package handler

import (
	"log/slog"
	"net/http"

	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
)

// AdminGlobalPackageHandler serves /api/admin/packages — a cross-org
// package browser used by super-admins. Each row carries its OrgID so the
// UI can group/route by tenant.
type AdminGlobalPackageHandler struct {
	orgs     store.OrgStore
	packages store.PackageStore
	storage  storage.Storage
	cache    *composer.Cache
}

func NewAdminGlobalPackageHandler(orgs store.OrgStore, packages store.PackageStore, strg storage.Storage, cache *composer.Cache) *AdminGlobalPackageHandler {
	return &AdminGlobalPackageHandler{orgs: orgs, packages: packages, storage: strg, cache: cache}
}

// List returns every package across every org. For typical tenant counts this
// is a small dataset; if it grows we'll add pagination + filtering.
func (h *AdminGlobalPackageHandler) List(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.orgs.List(r.Context())
	if err != nil {
		slog.Error("admin packages: list orgs error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	all := make([]model.Package, 0)
	for _, o := range orgs {
		pkgs, err := h.packages.List(r.Context(), o.ID)
		if err != nil {
			slog.Error("admin packages: list error", "error", err, "org_id", o.ID)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		all = append(all, pkgs...)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"packages": all})
}

// Delete force-deletes a package by ID, regardless of org. Drops version
// files from storage and invalidates the cache for the owning org.
func (h *AdminGlobalPackageHandler) Delete(w http.ResponseWriter, r *http.Request) {
	publicID, err := pathPublicID(r, "id", pid.Package)
	if err != nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	pkg, err := h.packages.GetByPublicIDGlobal(r.Context(), publicID)
	if err != nil {
		slog.Error("admin packages: get error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if pkg == nil {
		writeError(w, http.StatusNotFound, "package_not_found", "package not found")
		return
	}

	versions, err := h.packages.ListVersions(r.Context(), pkg.OrgID, pkg.ID)
	if err != nil {
		slog.Error("admin packages: list versions error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	for _, v := range versions {
		if err := h.storage.Delete(r.Context(), v.StoragePath); err != nil {
			slog.Error("admin packages: storage delete error", "error", err, "version_id", v.ID)
		}
	}

	if err := h.packages.Delete(r.Context(), pkg.OrgID, pkg.ID); err != nil {
		slog.Error("admin packages: delete error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_package", "failed to delete package")
		return
	}

	h.cache.Invalidate(r.Context(), pkg.OrgID)
	w.WriteHeader(http.StatusNoContent)
}
