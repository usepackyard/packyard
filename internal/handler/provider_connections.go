package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/credentials"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/store"
)

type ProviderConnectionHandler struct {
	connections store.ProviderConnectionStore
	cfg         *config.Config
}

func NewProviderConnectionHandler(connections store.ProviderConnectionStore, cfg *config.Config) *ProviderConnectionHandler {
	return &ProviderConnectionHandler{connections: connections, cfg: cfg}
}

type providerConnectionResponse struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Provider    string      `json:"provider"`
	AuthType    string      `json:"auth_type"`
	TokenPrefix string      `json:"token_prefix,omitempty"`
	Config      interface{} `json:"config,omitempty"`
	SourceCount int64       `json:"source_count"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

func connectionResponse(conn *model.ProviderConnection, sourceCount int64) providerConnectionResponse {
	cfg, _ := provider.ParseConnectionConfig(conn.Provider, conn.Config)
	return providerConnectionResponse{
		ID:          conn.PublicID,
		Name:        conn.Name,
		Provider:    conn.Provider,
		AuthType:    conn.AuthType,
		TokenPrefix: conn.TokenPrefix,
		Config:      cfg,
		SourceCount: sourceCount,
		CreatedAt:   conn.CreatedAt,
		UpdatedAt:   conn.UpdatedAt,
	}
}

func (h *ProviderConnectionHandler) List(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}
	conns, err := h.connections.List(r.Context(), org.ID)
	if err != nil {
		slog.Error("list provider connections", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_list_provider_connections", "failed to list provider connections")
		return
	}
	if conns == nil {
		conns = []model.ProviderConnection{}
	}
	out := make([]providerConnectionResponse, 0, len(conns))
	for i := range conns {
		count, err := h.connections.CountSources(r.Context(), conns[i].ID)
		if err != nil {
			slog.Error("count provider connection sources", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		out = append(out, connectionResponse(&conns[i], count))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"connections": out})
}

func (h *ProviderConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}
	conn, ok := h.decodeAndBuild(w, r, nil)
	if !ok {
		return
	}
	conn.OrgID = org.ID
	if userID, ok := auth.UserIDFromContext(r.Context()); ok && userID != 0 {
		conn.CreatedBy = &userID
	}
	if err := h.connections.Create(r.Context(), conn); err != nil {
		slog.Error("create provider connection", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_create_provider_connection", "failed to create provider connection")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{"connection": connectionResponse(conn, 0)})
}

func (h *ProviderConnectionHandler) Update(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}
	publicID, err := pathPublicID(r, "id", pid.ProviderConnection)
	if err != nil {
		writeError(w, http.StatusNotFound, "provider_connection_not_found", "provider connection not found")
		return
	}
	existing, err := h.connections.GetByPublicID(r.Context(), org.ID, publicID)
	if err != nil {
		slog.Error("lookup provider connection", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "provider_connection_not_found", "provider connection not found")
		return
	}
	conn, ok := h.decodeAndBuild(w, r, existing)
	if !ok {
		return
	}
	conn.ID = existing.ID
	conn.PublicID = existing.PublicID
	conn.OrgID = existing.OrgID
	conn.CreatedBy = existing.CreatedBy
	conn.CreatedAt = existing.CreatedAt
	if existing.Provider != conn.Provider {
		count, err := h.connections.CountSources(r.Context(), existing.ID)
		if err != nil {
			slog.Error("count provider connection sources", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
		if count > 0 {
			writeError(w, http.StatusConflict, "provider_connection_in_use", "provider connection is in use")
			return
		}
	}
	if err := h.connections.Update(r.Context(), conn); err != nil {
		slog.Error("update provider connection", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_update_provider_connection", "failed to update provider connection")
		return
	}
	count, err := h.connections.CountSources(r.Context(), conn.ID)
	if err != nil {
		slog.Error("count provider connection sources", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"connection": connectionResponse(conn, count)})
}

func (h *ProviderConnectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	org := auth.OrgFromContext(r.Context())
	if org == nil {
		writeError(w, http.StatusInternalServerError, "missing_org_context", "missing org context")
		return
	}
	publicID, err := pathPublicID(r, "id", pid.ProviderConnection)
	if err != nil {
		writeError(w, http.StatusNotFound, "provider_connection_not_found", "provider connection not found")
		return
	}
	conn, err := h.connections.GetByPublicID(r.Context(), org.ID, publicID)
	if err != nil {
		slog.Error("lookup provider connection", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if conn == nil {
		writeError(w, http.StatusNotFound, "provider_connection_not_found", "provider connection not found")
		return
	}
	count, err := h.connections.CountSources(r.Context(), conn.ID)
	if err != nil {
		slog.Error("count provider connection sources", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "provider_connection_in_use", "provider connection is in use")
		return
	}
	if err := h.connections.Delete(r.Context(), org.ID, conn.ID); err != nil {
		slog.Error("delete provider connection", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_delete_provider_connection", "failed to delete provider connection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ProviderConnectionHandler) decodeAndBuild(w http.ResponseWriter, r *http.Request, existing *model.ProviderConnection) (*model.ProviderConnection, bool) {
	var req struct {
		Name     string          `json:"name"`
		Provider string          `json:"provider"`
		AuthType string          `json:"auth_type"`
		Token    string          `json:"token"`
		Config   json.RawMessage `json:"config"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return nil, false
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name_required", "name is required")
		return nil, false
	}
	switch req.Provider {
	case provider.ProviderGitHub, provider.ProviderGitLab:
	default:
		writeError(w, http.StatusBadRequest, "unsupported_provider", "unsupported provider")
		return nil, false
	}
	if req.AuthType == "" {
		if req.Token != "" {
			req.AuthType = model.ProviderAuthToken
		} else {
			req.AuthType = model.ProviderAuthNone
		}
	}
	switch req.AuthType {
	case model.ProviderAuthNone, model.ProviderAuthToken:
	default:
		writeError(w, http.StatusBadRequest, "invalid_auth_type", "auth_type must be none or token")
		return nil, false
	}

	connCfg := provider.ConnectionConfig{}
	if len(req.Config) > 0 && string(req.Config) != "null" {
		if err := json.Unmarshal(req.Config, &connCfg); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_connection_config", "invalid connection config")
			return nil, false
		}
	}
	configRaw, err := provider.MarshalConnectionConfig(req.Provider, connCfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_connection_config", "invalid connection config")
		return nil, false
	}

	conn := &model.ProviderConnection{
		Name:     req.Name,
		Provider: req.Provider,
		AuthType: req.AuthType,
		Config:   configRaw,
	}
	if existing != nil {
		conn.SecretEncrypted = existing.SecretEncrypted
		conn.TokenPrefix = existing.TokenPrefix
	}
	if req.AuthType == model.ProviderAuthNone {
		conn.SecretEncrypted = ""
		conn.TokenPrefix = ""
		return conn, true
	}
	if req.Token == "" && existing == nil {
		writeError(w, http.StatusBadRequest, "provider_token_required", "token is required")
		return nil, false
	}
	if req.Token != "" {
		encrypted, err := credentials.EncryptString(req.Token, h.cfg.CredentialsKey)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_credentials_key", "PACKYARD_CREDENTIALS_KEY is required and must be 64 hex characters")
			return nil, false
		}
		conn.SecretEncrypted = encrypted
		conn.TokenPrefix = credentials.TokenPrefix(req.Token)
	}
	return conn, true
}
