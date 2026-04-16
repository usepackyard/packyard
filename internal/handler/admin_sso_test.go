package handler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/middleware"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
	"github.com/usepackyard/packyard/internal/testutil"
)

func TestAdminSSO_CreateAndLogin(t *testing.T) {
	stores := testutil.NewStores(t)
	cfg := testSSOConfig()
	user := testutil.MakeUser(t, stores, "user@example.com", "secret123")
	adminToken := makeAdminToken(t, stores)
	mux := newAdminSSOMux(stores, cfg)

	rec := doAdminJSON(t, mux, adminToken, "POST", "/api/admin/sso-tickets", map[string]any{
		"user_id":     user.PublicID,
		"redirect_to": "/packages",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body=%s", rec.Code, rec.Body.String())
	}

	var create struct {
		Ticket     string `json:"ticket"`
		RedirectTo string `json:"redirect_to"`
	}
	testutil.DecodeJSON(t, rec, &create)
	if create.Ticket == "" {
		t.Fatal("expected plaintext ticket")
	}
	if create.RedirectTo != "/packages" {
		t.Fatalf("redirect_to = %q, want /packages", create.RedirectTo)
	}

	loginReq := httptest.NewRequest("GET", "/sso/login?ticket="+create.Ticket, nil)
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusFound {
		t.Fatalf("login status = %d; body=%s", loginRec.Code, loginRec.Body.String())
	}
	if got := loginRec.Header().Get("Location"); got != "/packages" {
		t.Fatalf("location = %q, want /packages", got)
	}

	var sessionCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "packyard_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected packyard_session cookie")
	}
	parts := strings.SplitN(sessionCookie.Value, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("cookie value = %q, want session.hmac", sessionCookie.Value)
	}
	sess, err := stores.Sessions.GetByID(context.Background(), parts[0])
	if err != nil {
		t.Fatalf("lookup session: %v", err)
	}
	if sess == nil || sess.UserID != user.ID {
		t.Fatalf("session = %+v, want user_id=%d", sess, user.ID)
	}

	replayReq := httptest.NewRequest("GET", "/sso/login?ticket="+create.Ticket, nil)
	replayRec := httptest.NewRecorder()
	mux.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, want 401", replayRec.Code)
	}
}

func TestAdminSSO_CreateRejectsInvalidRedirect(t *testing.T) {
	stores := testutil.NewStores(t)
	cfg := testSSOConfig()
	user := testutil.MakeUser(t, stores, "user@example.com", "secret123")
	adminToken := makeAdminToken(t, stores)
	mux := newAdminSSOMux(stores, cfg)

	rec := doAdminJSON(t, mux, adminToken, "POST", "/api/admin/sso-tickets", map[string]any{
		"user_id":     user.PublicID,
		"redirect_to": "https://evil.example",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func newAdminSSOMux(stores *store.Stores, cfg *config.Config) http.Handler {
	h := handler.NewAdminSSOHandler(stores.Users, stores.SSOTickets, stores.Sessions, cfg)
	bearerAdminMw := auth.BearerAdminAuth(stores.AdminTokens)
	sessionAuthMw := auth.SessionAuth(stores.Sessions, cfg.Session.Secret)
	requireSuperAdminMw := auth.RequireSuperAdmin(stores.Users)

	adminAuthEither := func(h http.HandlerFunc) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				bearerAdminMw(http.HandlerFunc(h)).ServeHTTP(w, r)
				return
			}
			sessionAuthMw(requireSuperAdminMw(http.HandlerFunc(h))).ServeHTTP(w, r)
		})
	}
	adminWrite := func(h http.HandlerFunc) http.Handler {
		return middleware.RequireCSRFHeader(adminAuthEither(h))
	}

	mux := http.NewServeMux()
	mux.Handle("POST /api/admin/sso-tickets", adminWrite(h.Create))
	mux.Handle("GET /sso/login", http.HandlerFunc(h.Login))
	return mux
}

func doAdminJSON(t *testing.T, h http.Handler, adminToken, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(buf))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func makeAdminToken(t *testing.T, stores *store.Stores) string {
	t.Helper()

	creator := testutil.MakeUser(t, stores, "admin@example.com", "secret123")
	plaintext := "adm_test_token_plaintext"
	hash := sha256.Sum256([]byte(plaintext))
	tok := &model.AdminToken{
		Name:        "website",
		TokenHash:   hex.EncodeToString(hash[:]),
		TokenPrefix: plaintext[:12],
		CreatedBy:   creator.ID,
		IsActive:    true,
	}
	if err := stores.AdminTokens.Create(context.Background(), tok); err != nil {
		t.Fatalf("create admin token: %v", err)
	}
	return plaintext
}

func testSSOConfig() *config.Config {
	return &config.Config{
		BaseURL: "http://app.packyard.test",
		Session: config.SessionConfig{
			Secret: "0123456789abcdef0123456789abcdef",
			MaxAge: 3600,
		},
	}
}
