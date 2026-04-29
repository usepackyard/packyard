package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/credentials"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

type AdminSourceHandler struct {
	sources     store.SourceStore
	connections store.ProviderConnectionStore
	packages    store.PackageStore
	jobs        store.JobStore
	storage     storage.Storage
	cache       *composer.Cache
	cfg         *config.Config
}

func NewAdminSourceHandler(sources store.SourceStore, connections store.ProviderConnectionStore, packages store.PackageStore, jobs store.JobStore, strg storage.Storage, cache *composer.Cache, cfg *config.Config) *AdminSourceHandler {
	return &AdminSourceHandler{
		sources:     sources,
		connections: connections,
		packages:    packages,
		jobs:        jobs,
		storage:     strg,
		cache:       cache,
		cfg:         cfg,
	}
}

type packageSourceResponse struct {
	ID             string      `json:"id"`
	Provider       string      `json:"provider"`
	ConnectionID   string      `json:"connection_id,omitempty"`
	Config         interface{} `json:"config,omitempty"`
	RepoKey        string      `json:"repo_key,omitempty"`
	RepoURL        string      `json:"repo_url,omitempty"`
	MetadataSource string      `json:"metadata_source"`
	VersionSource  string      `json:"version_source"`
	ManualRequire  string      `json:"manual_require,omitempty"`
	LastSyncedAt   interface{} `json:"last_synced_at,omitempty"`
	CreatedAt      interface{} `json:"created_at"`
	UpdatedAt      interface{} `json:"updated_at"`
}

func sourceResponse(src *model.PackageSource, conn *model.ProviderConnection) packageSourceResponse {
	var cfg interface{}
	var sourceCfg provider.SourceConfig
	if src.Provider != provider.ProviderUpload {
		parsed, err := provider.ParseSourceConfig(src.Provider, src.ProviderConfig)
		if err == nil {
			sourceCfg = parsed
			cfg = parsed
		}
	}
	connectionID := ""
	connectionConfig := ""
	if conn != nil {
		connectionID = conn.PublicID
		connectionConfig = conn.Config
	}
	return packageSourceResponse{
		ID:             src.PublicID,
		Provider:       src.Provider,
		ConnectionID:   connectionID,
		Config:         cfg,
		RepoKey:        src.RepoKey,
		RepoURL:        provider.ExternalRepoURL(src.Provider, connectionConfig, sourceCfg),
		MetadataSource: src.MetadataSource,
		VersionSource:  src.VersionSource,
		ManualRequire:  src.ManualRequire,
		LastSyncedAt:   src.LastSyncedAt,
		CreatedAt:      src.CreatedAt,
		UpdatedAt:      src.UpdatedAt,
	}
}

func (h *AdminSourceHandler) Get(w http.ResponseWriter, r *http.Request) {
	pkgPublicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	// Verify the package belongs to this org.
	pkg, err := h.packages.GetByPublicID(r.Context(), org.ID, pkgPublicID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	src, err := h.sources.GetByPackageID(r.Context(), pkg.ID)
	if err != nil {
		slog.Error("get source error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if src == nil {
		writeError(w, http.StatusNotFound, "no_source_configured", "no source configured")
		return
	}

	var conn *model.ProviderConnection
	if src.ConnectionID != nil {
		conn, err = h.connections.GetByID(r.Context(), org.ID, *src.ConnectionID)
		if err != nil {
			slog.Error("get source connection error", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"source":      sourceResponse(src, conn),
		"webhook_url": webhookURL(h.cfg.BaseURL, src),
	})
}

func (h *AdminSourceHandler) Set(w http.ResponseWriter, r *http.Request) {
	pkgPublicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	pkg, err := h.packages.GetByPublicID(r.Context(), org.ID, pkgPublicID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	var req struct {
		Provider       string          `json:"provider"`
		ConnectionID   string          `json:"connection_id"`
		Config         json.RawMessage `json:"config"`
		MetadataSource string          `json:"metadata_source"`
		VersionSource  string          `json:"version_source"`
		ManualRequire  string          `json:"manual_require"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}

	if req.Provider == "" {
		req.Provider = "upload"
	}
	switch req.Provider {
	case provider.ProviderUpload, provider.ProviderGitHub, provider.ProviderGitLab:
		// valid
	default:
		writeError(w, http.StatusBadRequest, "unsupported_provider", "unsupported provider")
		return
	}

	var conn *model.ProviderConnection
	var connectionConfig string
	var sourceConfig provider.SourceConfig
	var providerConfig string
	var repoKey string
	var connectionID *int64
	if req.Provider != provider.ProviderUpload {
		if len(req.Config) > 0 && string(req.Config) != "null" {
			if err := json.Unmarshal(req.Config, &sourceConfig); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_source_config", "invalid source config")
				return
			}
		}
		raw, _, err := provider.MarshalSourceConfig(req.Provider, sourceConfig)
		if err != nil {
			writeSourceConfigError(w, err)
			return
		}
		providerConfig = raw
		if req.ConnectionID != "" {
			conn, err = h.connections.GetByPublicID(r.Context(), org.ID, req.ConnectionID)
			if err != nil {
				slog.Error("source connection lookup error", "error", err)
				writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
				return
			}
			if conn == nil {
				writeError(w, http.StatusBadRequest, "provider_connection_not_found", "provider connection not found")
				return
			}
			if conn.Provider != req.Provider {
				writeError(w, http.StatusBadRequest, "provider_connection_mismatch", "provider connection does not match source provider")
				return
			}
			connectionID = &conn.ID
			connectionConfig = conn.Config
		}
		parsed, _ := provider.ParseSourceConfig(req.Provider, providerConfig)
		sourceConfig = parsed
		repoKey = provider.RepoKey(req.Provider, connectionConfig, sourceConfig)
	}

	// MetadataSource is shared across providers.
	if req.MetadataSource == "" {
		req.MetadataSource = "from_zip"
	}
	switch req.MetadataSource {
	case "from_zip", "manual":
		// valid
	default:
		writeError(w, http.StatusBadRequest, "invalid_metadata_source", "metadata_source must be from_zip or manual")
		return
	}

	// Validate ManualRequire parses when supplied. Empty = no require.
	if req.MetadataSource == "manual" {
		if _, err := composer.ParseRequireJSON(req.ManualRequire); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_manual_require", "manual_require must be a JSON object mapping package names to version constraints")
			return
		}
	}

	// Provider-specific validation + normalisation.
	switch req.Provider {
	case provider.ProviderGitHub, provider.ProviderGitLab:
		// manual metadata with source_archive is pointless — the
		// source zipball always has composer.json. Reject so users
		// don't footgun themselves.
		if req.MetadataSource == "manual" && sourceConfig.Strategy == provider.StrategySourceArchive {
			writeError(w, http.StatusBadRequest, "manual_metadata_requires_release_asset", "manual metadata only applies to release_asset strategy")
			return
		}
		if req.VersionSource == "" {
			req.VersionSource = "auto"
		}
		switch req.VersionSource {
		case "auto", "git_tag", "composer_json":
			// valid
		default:
			writeError(w, http.StatusBadRequest, "invalid_version_source", "version_source must be auto, git_tag, or composer_json")
			return
		}
		// manual metadata can only use git_tag — there's no
		// composer.json to read from. Silently coerce so the
		// frontend's disabled select matches backend behavior.
		if req.MetadataSource == "manual" {
			req.VersionSource = "git_tag"
		}

	case provider.ProviderUpload:
		// Upload sources don't have a repo, strategy, or asset
		// pattern; blank those to keep the row tidy even if the
		// client sent them.
		// Upload's version_source options are a narrower set.
		if req.VersionSource == "" {
			if req.MetadataSource == "manual" {
				req.VersionSource = "manual"
			} else {
				req.VersionSource = "composer_json"
			}
		}
		switch req.VersionSource {
		case "composer_json", "manual":
			// valid
		default:
			writeError(w, http.StatusBadRequest, "invalid_version_source", "version_source for upload must be composer_json or manual")
			return
		}
		// manual metadata on upload → user types the version per
		// upload. Coerce to keep the pair consistent.
		if req.MetadataSource == "manual" {
			req.VersionSource = "manual"
		}
	}

	existing, err := h.sources.GetByPackageID(r.Context(), pkg.ID)
	if err != nil {
		slog.Error("source lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	isNew := existing == nil
	var webhookSecret string

	// Git providers have inbound webhooks, so they carry a webhook secret.
	// Switching to upload clears it so stale secrets don't linger.
	if isNew {
		if isGitProvider(req.Provider) {
			secret, err := generateWebhookSecret()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed_generate_webhook_secret", "failed to generate webhook secret")
				return
			}
			webhookSecret = secret
		}

		src := &model.PackageSource{
			PackageID:      pkg.ID,
			ConnectionID:   connectionID,
			Provider:       req.Provider,
			ProviderConfig: providerConfig,
			RepoKey:        repoKey,
			MetadataSource: req.MetadataSource,
			VersionSource:  req.VersionSource,
			ManualRequire:  req.ManualRequire,
			WebhookSecret:  webhookSecret,
		}
		if err := h.sources.Create(r.Context(), src); err != nil {
			slog.Error("create source error", "error", err)
			writeError(w, http.StatusInternalServerError, "failed_create_source", "failed to create source")
			return
		}
	} else {
		wasGitProvider := isGitProvider(existing.Provider)
		existing.Provider = req.Provider
		existing.ConnectionID = connectionID
		existing.ProviderConfig = providerConfig
		existing.RepoKey = repoKey
		existing.MetadataSource = req.MetadataSource
		existing.VersionSource = req.VersionSource
		existing.ManualRequire = req.ManualRequire
		if isGitProvider(req.Provider) && (!wasGitProvider || existing.WebhookSecret == "") {
			secret, err := generateWebhookSecret()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed_generate_webhook_secret", "failed to generate webhook secret")
				return
			}
			existing.WebhookSecret = secret
			webhookSecret = secret
		}
		if req.Provider == provider.ProviderUpload {
			existing.WebhookSecret = ""
		}
		if err := h.sources.Update(r.Context(), existing); err != nil {
			slog.Error("update source error", "error", err)
			writeError(w, http.StatusInternalServerError, "failed_update_source", "failed to update source")
			return
		}
	}

	src, _ := h.sources.GetByPackageID(r.Context(), pkg.ID)
	if src.ConnectionID != nil && conn == nil {
		conn, _ = h.connections.GetByID(r.Context(), org.ID, *src.ConnectionID)
	}

	resp := map[string]interface{}{
		"source": sourceResponse(src, conn),
	}
	// Webhook URL only applies to git providers. Upload sources don't accept
	// webhooks, so don't advertise a URL the server won't route.
	if isGitProvider(src.Provider) {
		resp["webhook_url"] = webhookURL(h.cfg.BaseURL, src)
	}
	if webhookSecret != "" {
		resp["webhook_secret"] = webhookSecret
	}

	status := http.StatusOK
	if isNew {
		status = http.StatusCreated
	}
	writeJSON(w, status, resp)
}

func (h *AdminSourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pkgPublicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	// Verify the package belongs to this org.
	pkg, err := h.packages.GetByPublicID(r.Context(), org.ID, pkgPublicID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	// Every package has exactly one source, so "Delete" is really
	// "reset to the default upload+from_zip config" — the natural
	// user intent is "stop syncing from GitHub, switch me back to
	// uploading zips myself." Row stays, fields reset. Existing
	// versions stored on the package are untouched.
	existing, err := h.sources.GetByPackageID(r.Context(), pkg.ID)
	if err != nil {
		slog.Error("source lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if existing == nil {
		// Should never happen (Create inserts a default source), but
		// keep behaviour idempotent for callers that retry.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	existing.Provider = "upload"
	existing.ConnectionID = nil
	existing.ProviderConfig = ""
	existing.RepoKey = ""
	existing.MetadataSource = "from_zip"
	existing.VersionSource = "composer_json"
	existing.ManualRequire = ""
	existing.WebhookSecret = ""
	if err := h.sources.Update(r.Context(), existing); err != nil {
		slog.Error("reset source error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_reset_source", "failed to reset source")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Sync enqueues a sync job for this package and returns its ID. The
// actual sync runs in a worker (see internal/jobs). The client should
// poll GET .../sync/{job_id} for progress and final status.
//
// If a job is already queued or running for this package, responds with
// 409 + the existing job, letting the UI hook into the live run instead
// of scheduling a duplicate. Webhooks and manual triggers both go
// through this path for unified concurrency control.
func (h *AdminSourceHandler) Sync(w http.ResponseWriter, r *http.Request) {
	pkgPublicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	pkg, err := h.packages.GetByPublicID(r.Context(), org.ID, pkgPublicID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	// Require a configured source — can't sync without one.
	src, err := h.sources.GetByPackageID(r.Context(), pkg.ID)
	if err != nil || src == nil {
		writeError(w, http.StatusNotFound, "no_source_configured", "no source configured")
		return
	}
	if !isGitProvider(src.Provider) {
		writeError(w, http.StatusBadRequest, "source_not_syncable", "source is not syncable")
		return
	}

	// Enforce one-active-sync-per-package. The worker claim query would
	// eventually serialize these anyway, but we want the UI to see the
	// existing job rather than silently queue a duplicate.
	if existing, err := h.jobs.ActiveForPackage(r.Context(), pkg.ID); err != nil {
		slog.Error("active-for-package lookup", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	} else if existing != nil {
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"job":      existing,
			"existing": true,
		})
		return
	}

	job := &model.SyncJob{
		OrgID:     org.ID,
		PackageID: pkg.ID,
		Trigger:   "manual",
	}
	if err := h.jobs.Enqueue(r.Context(), job); err != nil {
		slog.Error("enqueue sync job", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_enqueue_sync", "failed to enqueue sync")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"job":      job,
		"existing": false,
	})
}

// RotateWebhookSecret mints a fresh webhook signing secret for this
// package's source and returns it once. The previous secret is
// invalidated immediately, so the operator must update the provider's
// webhook configuration with the new value or deliveries will start
// failing signature verification. Only valid for git providers.
func (h *AdminSourceHandler) RotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	pkgPublicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	pkg, err := h.packages.GetByPublicID(r.Context(), org.ID, pkgPublicID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	src, err := h.sources.GetByPackageID(r.Context(), pkg.ID)
	if err != nil {
		slog.Error("source lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if src == nil {
		writeError(w, http.StatusNotFound, "no_source_configured", "no source configured")
		return
	}
	if !isGitProvider(src.Provider) {
		writeError(w, http.StatusBadRequest, "source_not_syncable", "source is not syncable")
		return
	}

	secret, err := generateWebhookSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed_generate_webhook_secret", "failed to generate webhook secret")
		return
	}
	src.WebhookSecret = secret
	if err := h.sources.Update(r.Context(), src); err != nil {
		slog.Error("rotate webhook secret error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_update_source", "failed to update source")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"webhook_url":    webhookURL(h.cfg.BaseURL, src),
		"webhook_secret": secret,
	})
}

// GetSyncJob returns the state of a single sync job scoped to this org
// and package — used by the frontend's poll loop.
func (h *AdminSourceHandler) GetSyncJob(w http.ResponseWriter, r *http.Request) {
	pkgPublicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}
	jobPublicID, err := pathPublicID(r, "job_id", pid.SyncJob)
	if err != nil {
		writeError(w, http.StatusNotFound, "job_not_found", "job not found")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	// Confirm the package belongs to this org — same cross-tenant guard
	// as every other org-scoped admin handler.
	pkg, err := h.packages.GetByPublicID(r.Context(), org.ID, pkgPublicID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	job, err := h.jobs.GetByPublicID(r.Context(), org.ID, jobPublicID)
	if err != nil {
		slog.Error("get sync job", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if job == nil || job.PackageID != pkg.ID {
		writeError(w, http.StatusNotFound, "job_not_found", "job not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"job": job})
}

// ListSyncJobs returns the most recent sync jobs for this package.
// Powers the (future) sync history UI; endpoint lands with the rest of
// the plumbing so the API surface is stable.
func (h *AdminSourceHandler) ListSyncJobs(w http.ResponseWriter, r *http.Request) {
	pkgPublicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	pkg, err := h.packages.GetByPublicID(r.Context(), org.ID, pkgPublicID)
	if err != nil || pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	jobs, err := h.jobs.ListForPackage(r.Context(), org.ID, pkg.ID, 20)
	if err != nil {
		slog.Error("list sync jobs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if jobs == nil {
		jobs = []model.SyncJob{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": jobs})
}

func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func isGitProvider(providerName string) bool {
	return providerName == provider.ProviderGitHub || providerName == provider.ProviderGitLab
}

func webhookURL(baseURL string, src *model.PackageSource) string {
	if src == nil || !isGitProvider(src.Provider) {
		return ""
	}
	return baseURL + "/hooks/" + src.Provider + "/" + src.PublicID
}

func writeSourceConfigError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case msg == "repo owner and name are required":
		writeError(w, http.StatusBadRequest, "repo_owner_and_name_required", msg)
	case msg == "strategy must be release_asset or source_archive":
		writeError(w, http.StatusBadRequest, "invalid_strategy", msg)
	case msg == "invalid asset_pattern glob":
		writeError(w, http.StatusBadRequest, "invalid_asset_pattern", msg)
	default:
		writeError(w, http.StatusBadRequest, "invalid_source_config", msg)
	}
}

// releasePreview is a trimmed projection of a release — just what the
// source-config form needs to let users pick a sensible asset pattern
// without guessing what their release actually looks like.
type releasePreview struct {
	Tag    string                `json:"tag"`
	Assets []releasePreviewAsset `json:"assets"`
}

type releasePreviewAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// PreviewReleases serves POST /api/[orgs/{org}/]sources/preview with a
// JSON body {provider, connection_id, config}. Returns up to 3 recent
// releases with their asset filenames so the user can build a matching
// pattern before saving.
func (h *AdminSourceHandler) PreviewReleases(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	var req struct {
		Provider     string          `json:"provider"`
		ConnectionID string          `json:"connection_id"`
		Config       json.RawMessage `json:"config"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	providerName := req.Provider
	if providerName == "" {
		providerName = "github"
	}
	if !isGitProvider(providerName) {
		writeError(w, http.StatusBadRequest, "unsupported_provider", "unsupported provider")
		return
	}

	var sourceConfig provider.SourceConfig
	if len(req.Config) > 0 && string(req.Config) != "null" {
		if err := json.Unmarshal(req.Config, &sourceConfig); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_source_config", "invalid source config")
			return
		}
	}
	raw, _, err := provider.MarshalSourceConfig(providerName, sourceConfig)
	if err != nil {
		writeSourceConfigError(w, err)
		return
	}
	sourceConfig, _ = provider.ParseSourceConfig(providerName, raw)

	var token, connectionConfig string
	if req.ConnectionID != "" {
		conn, err := h.connections.GetByPublicID(r.Context(), org.ID, req.ConnectionID)
		if err != nil {
			slog.Error("preview connection lookup error", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		if conn == nil {
			writeError(w, http.StatusBadRequest, "provider_connection_not_found", "provider connection not found")
			return
		}
		if conn.Provider != providerName {
			writeError(w, http.StatusBadRequest, "provider_connection_mismatch", "provider connection does not match source provider")
			return
		}
		connectionConfig = conn.Config
		if conn.AuthType == model.ProviderAuthToken {
			token, err = credentials.DecryptString(conn.SecretEncrypted, h.cfg.CredentialsKey)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_credentials_key", "PACKYARD_CREDENTIALS_KEY is required and must be 64 hex characters")
				return
			}
		}
	}
	prov, err := provider.NewProvider(providerName, token, connectionConfig)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsupported_provider", err.Error())
		return
	}

	releases, err := prov.ListReleases(r.Context(), sourceConfig.Owner, sourceConfig.Repo)
	if err != nil {
		slog.Warn("source preview list releases failed", "provider", providerName, "owner", sourceConfig.Owner, "repo", sourceConfig.Repo, "error", err)
		// Surface upstream failures distinctly. 404 typically means either
		// wrong coordinates OR a private repo we can't see — phrase both.
		writeError(w, http.StatusBadGateway, "failed_list_releases",
			"failed to list releases: "+err.Error()+
				". Check that owner/repo are correct; for private repos, select a provider connection.")
		return
	}

	const maxReleases = 3
	if len(releases) > maxReleases {
		releases = releases[:maxReleases]
	}

	out := make([]releasePreview, 0, len(releases))
	for _, rel := range releases {
		rp := releasePreview{
			Tag:    rel.TagName,
			Assets: make([]releasePreviewAsset, 0, len(rel.Assets)),
		}
		for _, a := range rel.Assets {
			rp.Assets = append(rp.Assets, releasePreviewAsset{Name: a.Name, Size: a.Size})
		}
		out = append(out, rp)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"releases": out})
}
