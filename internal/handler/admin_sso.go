package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
	"github.com/usepackyard/packyard/internal/store"
)

const (
	adminSSOAudience  = "app"
	adminSSOTicketTTL = 5 * time.Minute
)

type AdminSSOHandler struct {
	users    store.UserStore
	tickets  store.SSOTicketStore
	sessions store.SessionStore
	cfg      *config.Config
}

func NewAdminSSOHandler(users store.UserStore, tickets store.SSOTicketStore, sessions store.SessionStore, cfg *config.Config) *AdminSSOHandler {
	return &AdminSSOHandler{users: users, tickets: tickets, sessions: sessions, cfg: cfg}
}

func (h *AdminSSOHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID     string `json:"user_id"`
		RedirectTo string `json:"redirect_to"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}
	if _, err := pid.Parse(req.UserID, pid.User); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "invalid user id")
		return
	}

	redirectTo, ok := sanitizeSSORedirect(req.RedirectTo)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_payload", "redirect_to must be an absolute path")
		return
	}

	user, err := h.users.GetByPublicID(r.Context(), req.UserID)
	if err != nil {
		slog.Error("admin sso create: user lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if !user.IsActive {
		writeError(w, http.StatusForbidden, "account_disabled", "account disabled")
		return
	}

	plaintext, tokenHash, err := generateSSOTicket()
	if err != nil {
		slog.Error("admin sso create: ticket generation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_generate_token", "failed to generate token")
		return
	}

	ticket := &model.SSOTicket{
		TokenHash:  tokenHash,
		UserID:     user.ID,
		Audience:   adminSSOAudience,
		RedirectTo: redirectTo,
		ExpiresAt:  time.Now().Add(adminSSOTicketTTL),
	}
	if err := h.tickets.Create(r.Context(), ticket); err != nil {
		slog.Error("admin sso create: store failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed_create_token", "failed to create token")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"ticket":      plaintext,
		"redirect_to": ticket.RedirectTo,
		"expires_at":  ticket.ExpiresAt,
	})
}

func (h *AdminSSOHandler) Login(w http.ResponseWriter, r *http.Request) {
	rawTicket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if rawTicket == "" {
		http.Error(w, "ticket is required", http.StatusBadRequest)
		return
	}

	ticket, user, status, message := h.consume(r.Context(), rawTicket)
	if status != 0 {
		http.Error(w, message, status)
		return
	}

	secure := strings.HasPrefix(h.cfg.BaseURL, "https")
	if err := auth.CreateSession(w, h.sessions, h.cfg.Session.Secret, user.ID, h.cfg.Session.MaxAge, secure); err != nil {
		slog.Error("admin sso login: failed to create session", "error", err)
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, ticket.RedirectTo, http.StatusFound)
}

func (h *AdminSSOHandler) consume(ctx context.Context, rawTicket string) (*model.SSOTicket, *model.User, int, string) {
	hash := sha256.Sum256([]byte(rawTicket))
	ticket, err := h.tickets.Consume(ctx, hex.EncodeToString(hash[:]), adminSSOAudience, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, store.ErrSSOTicketNotFound), errors.Is(err, store.ErrSSOTicketAudienceInvalid):
			return nil, nil, http.StatusUnauthorized, "invalid ticket"
		case errors.Is(err, store.ErrSSOTicketExpired):
			return nil, nil, http.StatusUnauthorized, "ticket expired"
		case errors.Is(err, store.ErrSSOTicketConsumed):
			return nil, nil, http.StatusUnauthorized, "ticket already consumed"
		default:
			slog.Error("admin sso consume: consume failed", "error", err)
			return nil, nil, http.StatusInternalServerError, "internal error"
		}
	}

	user, err := h.users.GetByID(ctx, ticket.UserID)
	if err != nil {
		slog.Error("admin sso consume: user lookup failed", "error", err)
		return nil, nil, http.StatusInternalServerError, "internal error"
	}
	if user == nil {
		return nil, nil, http.StatusNotFound, "user not found"
	}
	if !user.IsActive {
		return nil, nil, http.StatusForbidden, "account disabled"
	}

	return ticket, user, 0, ""
}

func generateSSOTicket() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plaintext := "sso_" + hex.EncodeToString(buf)
	hash := sha256.Sum256([]byte(plaintext))
	return plaintext, hex.EncodeToString(hash[:]), nil
}

func sanitizeSSORedirect(raw string) (string, bool) {
	if raw == "" {
		return "/", true
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "", false
	}
	return raw, true
}
