package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
	"github.com/usepackyard/packyard/internal/store"
)

// AdminTokenHandler manages org-scoped Composer API tokens. Each token is
// bound to one org_id at creation; the Composer auth flow uses the token
// as the HTTP Basic username and a per-token generated password.
type AdminTokenHandler struct {
	tokens     store.TokenStore
	bcryptCost int
}

func NewAdminTokenHandler(tokens store.TokenStore, bcryptCost int) *AdminTokenHandler {
	return &AdminTokenHandler{tokens: tokens, bcryptCost: bcryptCost}
}

func (h *AdminTokenHandler) List(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}
	tokens, err := h.tokens.List(r.Context(), org.ID)
	if err != nil {
		slog.Error("list tokens error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_list_tokens", "failed to list tokens")
		return
	}
	if tokens == nil {
		tokens = []model.APIToken{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"tokens": tokens})
}

func (h *AdminTokenHandler) Create(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}

	var req struct {
		Name      string  `json:"name"`
		ExpiresAt *string `json:"expires_at,omitempty"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name_required", "name is required")
		return
	}

	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		writeError(w, http.StatusInternalServerError, "failed_generate_token", "failed to generate token")
		return
	}
	tokenStr := hex.EncodeToString(rawToken)

	rawPassword := make([]byte, 16)
	if _, err := rand.Read(rawPassword); err != nil {
		writeError(w, http.StatusInternalServerError, "failed_generate_token", "failed to generate token")
		return
	}
	passwordStr := hex.EncodeToString(rawPassword)

	passwordHash, err := auth.HashPassword(passwordStr, h.bcryptCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed_hash_password", "failed to hash password")
		return
	}

	hash := sha256.Sum256([]byte(tokenStr))
	tokenHash := hex.EncodeToString(hash[:])

	userID, _ := auth.UserIDFromContext(r.Context())
	var createdBy *int64
	if userID != 0 {
		createdBy = &userID
	}

	token := &model.APIToken{
		OrgID:        org.ID,
		Name:         req.Name,
		TokenHash:    tokenHash,
		PasswordHash: passwordHash,
		TokenPrefix:  tokenStr[:8],
		IsActive:     true,
		CreatedBy:    createdBy,
	}
	if err := h.tokens.Create(r.Context(), token); err != nil {
		slog.Error("create token error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_create_token", "failed to create token")
		return
	}

	// Return the raw token and password only once. Prevent intermediate
	// caches/proxies from retaining the plaintext credentials.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token":     tokenStr,
		"password":  passwordStr,
		"api_token": token,
	})
}

func (h *AdminTokenHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}
	publicID, err := pathPublicID(r, "id", pid.APIToken)
	if err != nil {
		writeError(w, http.StatusNotFound, "token_not_found", "token not found")
		return
	}
	tok, err := h.tokens.GetByPublicID(r.Context(), org.ID, publicID)
	if err != nil {
		slog.Error("delete token: lookup error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if tok == nil {
		writeError(w, http.StatusNotFound, "token_not_found", "token not found")
		return
	}
	if err := h.tokens.Delete(r.Context(), org.ID, tok.ID); err != nil {
		slog.Error("delete token error", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_token", "failed to delete token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
