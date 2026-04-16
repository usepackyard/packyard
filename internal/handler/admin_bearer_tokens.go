package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
	"github.com/usepackyard/packyard/internal/store"
)

// AdminBearerTokenHandler serves /api/admin/tokens — minting, listing, and
// revoking long-lived super-admin Bearer tokens for machine-to-machine use
// (CI, external automation, provisioning scripts). Distinct from
// AdminTokenHandler, which manages org-scoped Composer tokens.
type AdminBearerTokenHandler struct {
	tokens store.AdminTokenStore
}

func NewAdminBearerTokenHandler(tokens store.AdminTokenStore) *AdminBearerTokenHandler {
	return &AdminBearerTokenHandler{tokens: tokens}
}

func (h *AdminBearerTokenHandler) List(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.tokens.List(r.Context())
	if err != nil {
		slog.Error("admin tokens list error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_list_tokens", "failed to list tokens")
		return
	}
	if tokens == nil {
		tokens = []model.AdminToken{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tokens": tokens})
}

func (h *AdminBearerTokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name_required", "name is required")
		return
	}

	// 32 random bytes → 64 hex chars. Prefix with "adm_" so tokens are
	// recognizable and a naive log-grep can spot leakage.
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed_generate_token", "failed to generate token")
		return
	}
	plaintext := "adm_" + hex.EncodeToString(rawBytes)

	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	creator, _ := auth.UserIDFromContext(r.Context())

	token := &model.AdminToken{
		Name:        req.Name,
		TokenHash:   tokenHash,
		TokenPrefix: plaintext[:12], // "adm_" + 8 hex chars
		CreatedBy:   creator,
		IsActive:    true,
	}
	if err := h.tokens.Create(r.Context(), token); err != nil {
		slog.Error("admin bearer token create error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_create_token", "failed to create token")
		return
	}

	// Plaintext shown once. Don't let intermediate caches keep it.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token":       plaintext,
		"admin_token": token,
	})
}

func (h *AdminBearerTokenHandler) Delete(w http.ResponseWriter, r *http.Request) {
	publicID, err := pathPublicID(r, "id", pid.AdminToken)
	if err != nil {
		writeError(w, http.StatusNotFound, "token_not_found", "token not found")
		return
	}
	tok, err := h.tokens.GetByPublicID(r.Context(), publicID)
	if err != nil {
		slog.Error("admin bearer token delete: lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if tok == nil {
		writeError(w, http.StatusNotFound, "token_not_found", "token not found")
		return
	}
	if err := h.tokens.Delete(r.Context(), tok.ID); err != nil {
		slog.Error("admin bearer token delete error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_token", "failed to delete token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
