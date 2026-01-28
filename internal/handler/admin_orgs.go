package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
)

// AdminOrgHandler serves /api/admin/orgs/* endpoints. All routes assume
// auth.RequireSuperAdmin (or BearerAdminAuth followed by RequireSuperAdmin)
// has gated the request.
type AdminOrgHandler struct {
	orgs     store.OrgStore
	packages store.PackageStore
}

func NewAdminOrgHandler(orgs store.OrgStore, packages store.PackageStore) *AdminOrgHandler {
	return &AdminOrgHandler{orgs: orgs, packages: packages}
}

func (h *AdminOrgHandler) List(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.orgs.List(r.Context())
	if err != nil {
		slog.Error("admin orgs list error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_list_organizations", "failed to list organizations")
		return
	}
	if orgs == nil {
		orgs = []model.Organization{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"organizations": orgs})
}

func (h *AdminOrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	req.Slug = strings.TrimSpace(req.Slug)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name_required", "name is required")
		return
	}
	if err := composer.ValidateOrgSlug(req.Slug); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_org_slug", err.Error())
		return
	}

	existing, _ := h.orgs.GetBySlug(r.Context(), req.Slug)
	if existing != nil {
		writeError(w, http.StatusConflict, "organization_slug_exists", "organization slug already exists")
		return
	}

	org := &model.Organization{Slug: req.Slug, Name: req.Name}
	if err := h.orgs.Create(r.Context(), org); err != nil {
		slog.Error("admin org create error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_create_organization", "failed to create organization")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"organization": org})
}

func (h *AdminOrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	org, err := h.orgs.GetBySlug(r.Context(), slug)
	if err != nil {
		slog.Error("admin org get error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if org == nil {
		writeError(w, http.StatusNotFound, "organization_not_found", "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"organization": org})
}

func (h *AdminOrgHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var req struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	switch req.Status {
	case model.OrgStatusActive, model.OrgStatusSuspended, model.OrgStatusArchived:
		// ok
	default:
		writeError(w, http.StatusBadRequest, "invalid_status", "status must be active, suspended, or archived")
		return
	}

	org, err := h.orgs.GetBySlug(r.Context(), slug)
	if err != nil {
		slog.Error("admin status: lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if org == nil {
		writeError(w, http.StatusNotFound, "organization_not_found", "organization not found")
		return
	}

	if err := h.orgs.UpdateStatus(r.Context(), org.ID, req.Status); err != nil {
		slog.Error("admin status update error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_update_status", "failed to update status")
		return
	}
	org.Status = req.Status
	writeJSON(w, http.StatusOK, map[string]interface{}{"organization": org})
}

func (h *AdminOrgHandler) Delete(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	force := r.URL.Query().Get("force") == "true"

	org, err := h.orgs.GetBySlug(r.Context(), slug)
	if err != nil {
		slog.Error("admin delete: lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if org == nil {
		writeError(w, http.StatusNotFound, "organization_not_found", "organization not found")
		return
	}

	if !force {
		// Refuse if any packages exist — operator must opt in to data loss.
		pkgs, err := h.packages.List(r.Context(), org.ID)
		if err != nil {
			slog.Error("admin delete: package list error", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		if len(pkgs) > 0 {
			writeError(w, http.StatusConflict, "organization_has_packages", "organization has packages — pass ?force=true to delete anyway")
			return
		}
	}

	if err := h.orgs.Delete(r.Context(), org.ID); err != nil {
		slog.Error("admin org delete error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_organization", "failed to delete organization")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
