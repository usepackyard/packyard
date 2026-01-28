package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
)

type AdminUserHandler struct {
	users      store.UserStore
	bcryptCost int
}

func NewAdminUserHandler(users store.UserStore, bcryptCost int) *AdminUserHandler {
	return &AdminUserHandler{users: users, bcryptCost: bcryptCost}
}

func (h *AdminUserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.users.List(r.Context())
	if err != nil {
		slog.Error("list users error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_list_users", "failed to list users")
		return
	}
	if users == nil {
		users = []model.User{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"users": users,
	})
}

func (h *AdminUserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email        string `json:"email"`
		Password     string `json:"password"`
		Name         string `json:"name"`
		IsSuperAdmin bool   `json:"is_super_admin"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email_and_password_required", "email and password required")
		return
	}

	existing, _ := h.users.GetByEmail(r.Context(), req.Email)
	if existing != nil {
		writeError(w, http.StatusConflict, "email_already_in_use", "email already in use")
		return
	}

	hash, err := auth.HashPassword(req.Password, h.bcryptCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed_hash_password", "failed to hash password")
		return
	}

	user := &model.User{
		Email:        req.Email,
		Password:     hash,
		Name:         req.Name,
		IsActive:     true,
		IsSuperAdmin: req.IsSuperAdmin,
	}

	if err := h.users.Create(r.Context(), user); err != nil {
		slog.Error("create user error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_create_user", "failed to create user")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"user": user,
	})
}

func (h *AdminUserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "invalid user id")
		return
	}

	currentUserID, _ := auth.UserIDFromContext(r.Context())
	if currentUserID == id {
		writeError(w, http.StatusBadRequest, "cannot_delete_self", "cannot delete your own account")
		return
	}

	if err := h.users.Delete(r.Context(), id); err != nil {
		slog.Error("delete user error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_user", "failed to delete user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SetPassword replaces the user's password hash. Body: {"password": "..."}.
// Used by external SaaS layers for password-reset flows. The caller is
// responsible for verifying the user initiated the reset (e.g. via an
// emailed one-time token on the SaaS side).
func (h *AdminUserHandler) SetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "invalid user id")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password_required", "password required")
		return
	}

	user, err := h.users.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("set password: lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}

	hash, err := auth.HashPassword(req.Password, h.bcryptCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed_hash_password", "failed to hash password")
		return
	}
	user.Password = hash
	if err := h.users.Update(r.Context(), user); err != nil {
		slog.Error("set password: update error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_update_user", "failed to update user")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetSuperAdmin toggles the IsSuperAdmin flag. Body: {"is_super_admin": true/false}.
// Refuses to revoke super-admin from yourself (prevents lockout).
func (h *AdminUserHandler) SetSuperAdmin(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "invalid user id")
		return
	}

	var req struct {
		IsSuperAdmin bool `json:"is_super_admin"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}

	currentUserID, _ := auth.UserIDFromContext(r.Context())
	if currentUserID == id && !req.IsSuperAdmin {
		writeError(w, http.StatusBadRequest, "cannot_revoke_self_super_admin", "cannot revoke your own super-admin role")
		return
	}

	user, err := h.users.GetByID(r.Context(), id)
	if err != nil {
		slog.Error("set super-admin: lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	user.IsSuperAdmin = req.IsSuperAdmin
	if err := h.users.Update(r.Context(), user); err != nil {
		slog.Error("set super-admin: update error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_update_user", "failed to update user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"user": user})
}
