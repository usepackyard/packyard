package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

type AdminPackageHandler struct {
	packages    store.PackageStore
	sources     store.SourceStore
	connections store.ProviderConnectionStore
	storage     storage.Storage
	cache       *composer.Cache
}

func NewAdminPackageHandler(packages store.PackageStore, sources store.SourceStore, connections store.ProviderConnectionStore, storage storage.Storage, cache *composer.Cache) *AdminPackageHandler {
	return &AdminPackageHandler{packages: packages, sources: sources, connections: connections, storage: storage, cache: cache}
}

var (
	errInvalidMetadataSource              = errors.New("invalid metadata source")
	errInvalidManualRequire               = errors.New("invalid manual require")
	errInvalidUploadVersionSource         = errors.New("invalid upload version source")
	errInvalidVersionSource               = errors.New("invalid version source")
	errInvalidSourceConfig                = errors.New("invalid source config")
	errProviderConnectionNotFound         = errors.New("provider connection not found")
	errProviderConnectionMismatch         = errors.New("provider connection mismatch")
	errManualMetadataRequiresReleaseAsset = errors.New("manual metadata requires release asset")
	errUnsupportedProvider                = errors.New("unsupported provider")
	errGenerateWebhookSecret              = errors.New("generate webhook secret")
)

func (h *AdminPackageHandler) List(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	// Include each package's versions. The Version model hides heavy
	// fields (composer_json, require_json) via json:"-", so the only
	// visible addition is the small per-version metadata the dashboard
	// needs (download_count, file_size, dist_sha1, created_at).
	packages, err := h.packages.ListAllWithVersions(r.Context(), org.ID)
	if err != nil {
		slog.Error("list packages error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_list_packages", "failed to list packages")
		return
	}
	if packages == nil {
		packages = []model.Package{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"packages": packages,
	})
}

func (h *AdminPackageHandler) Create(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Description string `json:"description"`
		Homepage    string `json:"homepage"`
		Source      *struct {
			Provider       string          `json:"provider"`
			ConnectionID   string          `json:"connection_id"`
			Config         json.RawMessage `json:"config"`
			MetadataSource string          `json:"metadata_source"`
			VersionSource  string          `json:"version_source"`
			ManualRequire  string          `json:"manual_require"`
		} `json:"source"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	if err := composer.ValidatePackageName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_name", err.Error())
		return
	}
	if req.Type == "" {
		req.Type = "library"
	}

	existing, _ := h.packages.GetByName(r.Context(), org.ID, req.Name)
	if existing != nil {
		writeError(w, http.StatusConflict, "package_already_exists", "package already exists")
		return
	}

	source, err := h.sourceFromCreateRequest(r, org.ID, req.Source)
	if err != nil {
		writePackageSourceCreateError(w, err)
		return
	}

	pkg := &model.Package{
		OrgID:       org.ID,
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		Homepage:    req.Homepage,
	}

	if err := h.packages.Create(r.Context(), pkg); err != nil {
		slog.Error("create package error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_create_package", "failed to create package")
		return
	}

	source.PackageID = pkg.ID
	if err := h.sources.Create(r.Context(), source); err != nil {
		slog.Error("create default source error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_create_source", "failed to create package source")
		return
	}

	resp := map[string]interface{}{
		"package": pkg,
		"source":  source,
	}
	if source.WebhookSecret != "" {
		resp["webhook_secret"] = source.WebhookSecret
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *AdminPackageHandler) sourceFromCreateRequest(r *http.Request, orgID int64, req *struct {
	Provider       string          `json:"provider"`
	ConnectionID   string          `json:"connection_id"`
	Config         json.RawMessage `json:"config"`
	MetadataSource string          `json:"metadata_source"`
	VersionSource  string          `json:"version_source"`
	ManualRequire  string          `json:"manual_require"`
}) (*model.PackageSource, error) {
	if req == nil {
		return &model.PackageSource{
			Provider:       provider.ProviderUpload,
			MetadataSource: "from_zip",
			VersionSource:  "composer_json",
		}, nil
	}
	if req.Provider == "" {
		req.Provider = provider.ProviderUpload
	}
	if req.MetadataSource == "" {
		req.MetadataSource = "from_zip"
	}
	switch req.MetadataSource {
	case "from_zip", "manual":
	default:
		return nil, errInvalidMetadataSource
	}
	if req.MetadataSource == "manual" {
		if _, err := composer.ParseRequireJSON(req.ManualRequire); err != nil {
			return nil, errInvalidManualRequire
		}
	}

	src := &model.PackageSource{
		Provider:       req.Provider,
		MetadataSource: req.MetadataSource,
		ManualRequire:  req.ManualRequire,
	}

	switch req.Provider {
	case provider.ProviderUpload:
		if req.VersionSource == "" {
			if req.MetadataSource == "manual" {
				req.VersionSource = "manual"
			} else {
				req.VersionSource = "composer_json"
			}
		}
		if req.VersionSource != "composer_json" && req.VersionSource != "manual" {
			return nil, errInvalidUploadVersionSource
		}
		if req.MetadataSource == "manual" {
			req.VersionSource = "manual"
		}
		src.VersionSource = req.VersionSource
		return src, nil

	case provider.ProviderGitHub, provider.ProviderGitLab:
		var sourceConfig provider.SourceConfig
		if len(req.Config) > 0 && string(req.Config) != "null" {
			if err := json.Unmarshal(req.Config, &sourceConfig); err != nil {
				return nil, errInvalidSourceConfig
			}
		}
		raw, _, err := provider.MarshalSourceConfig(req.Provider, sourceConfig)
		if err != nil {
			return nil, err
		}
		sourceConfig, _ = provider.ParseSourceConfig(req.Provider, raw)

		var connectionConfig string
		if req.ConnectionID != "" {
			conn, err := h.connections.GetByPublicID(r.Context(), orgID, req.ConnectionID)
			if err != nil {
				return nil, err
			}
			if conn == nil {
				return nil, errProviderConnectionNotFound
			}
			if conn.Provider != req.Provider {
				return nil, errProviderConnectionMismatch
			}
			src.ConnectionID = &conn.ID
			connectionConfig = conn.Config
		}
		if req.MetadataSource == "manual" && sourceConfig.Strategy == provider.StrategySourceArchive {
			return nil, errManualMetadataRequiresReleaseAsset
		}
		if req.VersionSource == "" {
			req.VersionSource = "auto"
		}
		switch req.VersionSource {
		case "auto", "git_tag", "composer_json":
		default:
			return nil, errInvalidVersionSource
		}
		if req.MetadataSource == "manual" {
			req.VersionSource = "git_tag"
		}
		secret, err := generateWebhookSecret()
		if err != nil {
			return nil, errGenerateWebhookSecret
		}
		src.ProviderConfig = raw
		src.RepoKey = provider.RepoKey(req.Provider, connectionConfig, sourceConfig)
		src.VersionSource = req.VersionSource
		src.WebhookSecret = secret
		return src, nil

	default:
		return nil, errUnsupportedProvider
	}
}

func writePackageSourceCreateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errInvalidMetadataSource):
		writeError(w, http.StatusBadRequest, "invalid_metadata_source", "metadata_source must be from_zip or manual")
	case errors.Is(err, errInvalidManualRequire):
		writeError(w, http.StatusBadRequest, "invalid_manual_require", "manual_require must be a JSON object mapping package names to version constraints")
	case errors.Is(err, errInvalidUploadVersionSource), errors.Is(err, errInvalidVersionSource):
		writeError(w, http.StatusBadRequest, "invalid_version_source", "invalid version_source")
	case errors.Is(err, errInvalidSourceConfig):
		writeError(w, http.StatusBadRequest, "invalid_source_config", "invalid source config")
	case errors.Is(err, errProviderConnectionNotFound):
		writeError(w, http.StatusBadRequest, "provider_connection_not_found", "provider connection not found")
	case errors.Is(err, errProviderConnectionMismatch):
		writeError(w, http.StatusBadRequest, "provider_connection_mismatch", "provider connection does not match source provider")
	case errors.Is(err, errManualMetadataRequiresReleaseAsset):
		writeError(w, http.StatusBadRequest, "manual_metadata_requires_release_asset", "manual metadata only applies to release_asset strategy")
	case errors.Is(err, errUnsupportedProvider):
		writeError(w, http.StatusBadRequest, "unsupported_provider", "unsupported provider")
	case errors.Is(err, errGenerateWebhookSecret):
		writeError(w, http.StatusInternalServerError, "failed_generate_webhook_secret", "failed to generate webhook secret")
	default:
		switch err.Error() {
		case "repo owner and name are required", "strategy must be release_asset or source_archive", "invalid asset_pattern glob":
			writeSourceConfigError(w, err)
		default:
			slog.Error("build package source error", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
	}
}

func (h *AdminPackageHandler) Get(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	publicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	pkg, err := h.packages.GetByPublicID(r.Context(), org.ID, publicID)
	if err != nil {
		slog.Error("get package error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	versions, err := h.packages.ListVersions(r.Context(), org.ID, pkg.ID)
	if err != nil {
		slog.Error("list versions error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if versions == nil {
		versions = []model.Version{}
	}
	pkg.Versions = versions

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"package": pkg,
	})
}

func (h *AdminPackageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	publicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	pkg, err := h.packages.GetByPublicID(r.Context(), org.ID, publicID)
	if err != nil {
		slog.Error("lookup package for delete error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	// Delete all version files from storage first.
	versions, err := h.packages.ListVersions(r.Context(), org.ID, pkg.ID)
	if err != nil {
		slog.Error("list versions for delete error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	for _, v := range versions {
		if err := h.storage.Delete(r.Context(), v.StoragePath); err != nil {
			slog.Error("delete version file error", "error", err, "version_id", v.ID)
		}
	}

	if err := h.packages.Delete(r.Context(), org.ID, pkg.ID); err != nil {
		slog.Error("delete package error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_package", "failed to delete package")
		return
	}

	h.cache.Invalidate(r.Context(), org.ID)
	w.WriteHeader(http.StatusNoContent)
}
