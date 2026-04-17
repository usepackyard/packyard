package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/i18n"
	"github.com/usepackyard/packyard/internal/store"
)

type AdminAuthHandler struct {
	users      store.UserStore
	sessions   store.SessionStore
	orgs       store.OrgStore
	cfg        *config.Config
	bcryptCost int
}

func NewAdminAuthHandler(users store.UserStore, sessions store.SessionStore, orgs store.OrgStore, cfg *config.Config, bcryptCost int) *AdminAuthHandler {
	return &AdminAuthHandler{users: users, sessions: sessions, orgs: orgs, cfg: cfg, bcryptCost: bcryptCost}
}

func (h *AdminAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
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

	user, err := h.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		slog.Error("login: get user by email failed", "error", err, "email", req.Email)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if user == nil || !auth.CheckPassword(user.Password, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid credentials")
		return
	}
	if !user.IsActive {
		writeError(w, http.StatusForbidden, "account_disabled", "account disabled")
		return
	}

	if err := auth.CreateSession(w, h.sessions, h.cfg.Session.Secret, user.ID, h.cfg.Session.MaxAge, cookieOptions(h.cfg)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed_create_session", "failed to create session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

func (h *AdminAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	auth.ClearSession(w, r, h.sessions, h.cfg.Session.Secret, cookieOptions(h.cfg))
	w.WriteHeader(http.StatusNoContent)
}

// cookieOptions derives the session-cookie attributes from runtime
// config: Secure from the BaseURL scheme, Domain + SameSite from the
// two optional env vars. All session-cookie writes in this package
// funnel through here so deployments opting into shared-parent-domain
// login get one consistent cookie shape.
func cookieOptions(cfg *config.Config) auth.CookieOptions {
	return auth.CookieOptions{
		Secure:   strings.HasPrefix(cfg.BaseURL, "https"),
		Domain:   cfg.Session.CookieDomain,
		SameSite: cfg.Session.CookieSameSite,
	}
}

func (h *AdminAuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not_authenticated", "not authenticated")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "not_authenticated", "not authenticated")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

// UpdateMe lets the authenticated user patch their own profile. Scope
// is intentionally narrow — name + preferred UI language. Email changes
// and password changes have security requirements (reverification,
// current-password check) that belong on dedicated endpoints.
func (h *AdminAuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not_authenticated", "not authenticated")
		return
	}

	var req struct {
		Name     *string `json:"name"`
		Language *string `json:"language"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "not_authenticated", "not authenticated")
		return
	}

	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeError(w, http.StatusBadRequest, "name_cannot_be_empty", "name cannot be empty")
			return
		}
		user.Name = trimmed
	}
	if req.Language != nil {
		if !i18n.IsSupported(*req.Language) {
			writeError(w, http.StatusBadRequest, "unsupported_language", "unsupported language")
			return
		}
		user.Language = *req.Language
	}

	if err := h.users.Update(r.Context(), user); err != nil {
		slog.Error("update self failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

// ChangePassword lets the authenticated user change their own password.
// Requires the current password for verification. On success, all other
// sessions for the user are revoked.
func (h *AdminAuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not_authenticated", "not authenticated")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	if req.CurrentPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password_required", "current password is required")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters")
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "not_authenticated", "not authenticated")
		return
	}

	if !auth.CheckPassword(user.Password, req.CurrentPassword) {
		writeError(w, http.StatusForbidden, "wrong_password", "current password is incorrect")
		return
	}

	hash, err := auth.HashPassword(req.NewPassword, h.bcryptCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed_hash_password", "failed to hash password")
		return
	}
	user.Password = hash
	if err := h.users.Update(r.Context(), user); err != nil {
		slog.Error("change password: update error", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if sessionID, ok := auth.SessionIDFromContext(r.Context()); ok {
		_ = h.sessions.DeleteOthersByUserID(r.Context(), userID, sessionID)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminAuthHandler) ListOrgs(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not_authenticated", "not authenticated")
		return
	}

	orgs, err := h.orgs.ListUserOrgs(r.Context(), userID)
	if err != nil {
		slog.Error("list user orgs error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"organizations": orgs,
	})
}
