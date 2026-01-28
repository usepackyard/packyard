package handler

import (
	"log/slog"
	"net/http"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

type AdminPackageHandler struct {
	packages store.PackageStore
	storage  storage.Storage
	cache    *composer.Cache
}

func NewAdminPackageHandler(packages store.PackageStore, storage storage.Storage, cache *composer.Cache) *AdminPackageHandler {
	return &AdminPackageHandler{packages: packages, storage: storage, cache: cache}
}

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
		req.Type = "wordpress-plugin"
	}

	existing, _ := h.packages.GetByName(r.Context(), org.ID, req.Name)
	if existing != nil {
		writeError(w, http.StatusConflict, "package_already_exists", "package already exists")
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

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"package": pkg,
	})
}

func (h *AdminPackageHandler) Get(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_id", "invalid package id")
		return
	}

	pkg, err := h.packages.GetByID(r.Context(), org.ID, id)
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

	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_package_id", "invalid package id")
		return
	}

	// Delete all version files from storage first.
	versions, err := h.packages.ListVersions(r.Context(), org.ID, id)
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

	if err := h.packages.Delete(r.Context(), org.ID, id); err != nil {
		slog.Error("delete package error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_package", "failed to delete package")
		return
	}

	h.cache.Invalidate(r.Context(), org.ID)
	w.WriteHeader(http.StatusNoContent)
}
