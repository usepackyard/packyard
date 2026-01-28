package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/store"
	"github.com/usepackyard/packyard/internal/testutil"
)

const testSessionSecret = "test-secret-of-at-least-32-characters!"

func newAuthHandler(t *testing.T) (*handler.AdminAuthHandler, *config.Config, *store.Stores) {
	t.Helper()
	stores := testutil.NewStores(t)
	cfg := &config.Config{
		BaseURL: "http://localhost:9090",
		Session: config.SessionConfig{Secret: testSessionSecret, MaxAge: 3600},
	}
	h := handler.NewAdminAuthHandler(stores.Users, stores.Sessions, stores.Orgs, cfg, 4)
	return h, cfg, stores
}

func TestAdminAuthHandler_Login_Success(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	testutil.MakeUser(t, stores, "admin@example.com", "secret123")

	rec := testutil.DoJSON(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login",
		map[string]string{"email": "admin@example.com", "password": "secret123"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	var session *http.Cookie
	for _, c := range cookies {
		if c.Name == "packyard_session" {
			session = c
			break
		}
	}
	if session == nil {
		t.Fatal("expected packyard_session cookie")
	}
	if !strings.Contains(session.Value, ".") {
		t.Errorf("cookie value should be id.hmac, got %q", session.Value)
	}
	if !session.HttpOnly {
		t.Error("session cookie should be HttpOnly")
	}
	if session.SameSite != http.SameSiteStrictMode {
		t.Errorf("session cookie should be SameSite=Strict, got %v", session.SameSite)
	}
}

func TestAdminAuthHandler_Login_WrongPassword(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	testutil.MakeUser(t, stores, "u@example.com", "right-password")

	rec := testutil.DoJSON(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login",
		map[string]string{"email": "u@example.com", "password": "wrong-password"})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid credentials") {
		t.Errorf("expected uniform 'invalid credentials' message, got: %s", rec.Body.String())
	}
}

func TestAdminAuthHandler_Login_UnknownEmailGivesSameMessage(t *testing.T) {
	h, _, _ := newAuthHandler(t)
	rec := testutil.DoJSON(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login",
		map[string]string{"email": "nobody@example.com", "password": "anything"})

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid credentials") {
		t.Errorf("should not distinguish missing user from wrong password: %s", rec.Body.String())
	}
}

func TestAdminAuthHandler_Login_DisabledUser(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "disabled@example.com", "p")
	user.IsActive = false
	if err := stores.Users.Update(context.Background(), user); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	rec := testutil.DoJSON(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login",
		map[string]string{"email": "disabled@example.com", "password": "p"})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAdminAuthHandler_Login_InvalidJSON(t *testing.T) {
	h, _, _ := newAuthHandler(t)
	rec := testutil.DoJSON(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login", "not-json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminAuthHandler_ListOrgs_NotAuthenticated(t *testing.T) {
	h, _, _ := newAuthHandler(t)
	rec := testutil.DoJSON(t, http.HandlerFunc(h.ListOrgs), "GET", "/api/orgs", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAdminAuthHandler_Login_MissingFields(t *testing.T) {
	h, _, _ := newAuthHandler(t)
	rec := testutil.DoJSON(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login",
		map[string]string{"email": "", "password": ""})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminAuthHandler_Logout_Idempotent(t *testing.T) {
	h, _, _ := newAuthHandler(t)
	rec := testutil.DoJSON(t, http.HandlerFunc(h.Logout), "POST", "/api/auth/logout", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "packyard_session" && c.MaxAge < 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected clearing packyard_session cookie")
	}
}

func TestAdminAuthHandler_Me_NotAuthenticated(t *testing.T) {
	h, _, _ := newAuthHandler(t)
	rec := testutil.DoJSON(t, http.HandlerFunc(h.Me), "GET", "/api/auth/me", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not authenticated") {
		t.Errorf("expected uniform 'not authenticated' message, got: %s", rec.Body.String())
	}
}

func TestAdminAuthHandler_Me_ValidSession(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "u@example.com", "p")

	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminAuthHandler_Me_StaleSession(t *testing.T) {
	h, _, _ := newAuthHandler(t)

	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), 99999))
	rec := httptest.NewRecorder()
	h.Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not authenticated") {
		t.Errorf("expected uniform message, got: %s", rec.Body.String())
	}
}

func TestAdminAuthHandler_UpdateMe_UpdatesLanguage(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "u@example.com", "p")

	req := httptest.NewRequest("PUT", "/api/auth/me",
		strings.NewReader(`{"language":"mk"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	h.UpdateMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"language":"mk"`) {
		t.Errorf("response should carry new language: %s", rec.Body.String())
	}
	got, err := stores.Users.GetByID(context.Background(), user.ID)
	if err != nil || got == nil {
		t.Fatalf("get user: %v", err)
	}
	if got.Language != "mk" {
		t.Errorf("DB language = %q, want mk", got.Language)
	}
}

func TestAdminAuthHandler_UpdateMe_RejectsUnsupportedLanguage(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "u@example.com", "p")

	req := httptest.NewRequest("PUT", "/api/auth/me",
		strings.NewReader(`{"language":"zz"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	h.UpdateMe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	got, _ := stores.Users.GetByID(context.Background(), user.ID)
	if got.Language == "zz" {
		t.Error("rejected language should not have been persisted")
	}
}

func TestAdminAuthHandler_UpdateMe_UpdatesName(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "u@example.com", "p")

	req := httptest.NewRequest("PUT", "/api/auth/me",
		strings.NewReader(`{"name":"New Name"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	h.UpdateMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	got, _ := stores.Users.GetByID(context.Background(), user.ID)
	if got.Name != "New Name" {
		t.Errorf("name = %q, want New Name", got.Name)
	}
}

func TestAdminAuthHandler_UpdateMe_RejectsEmptyName(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "u@example.com", "p")

	req := httptest.NewRequest("PUT", "/api/auth/me",
		strings.NewReader(`{"name":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	h.UpdateMe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminAuthHandler_UpdateMe_NotAuthenticated(t *testing.T) {
	h, _, _ := newAuthHandler(t)

	req := httptest.NewRequest("PUT", "/api/auth/me",
		strings.NewReader(`{"language":"mk"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.UpdateMe(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAdminAuthHandler_ListOrgs(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "u@example.com", "p")
	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	testutil.MakeMember(t, stores, org.ID, user.ID, "owner")

	req := httptest.NewRequest("GET", "/api/orgs", nil)
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	h.ListOrgs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "acme") {
		t.Errorf("body should list 'acme': %s", rec.Body.String())
	}
}

// --- ChangePassword ---

func TestChangePassword_Success(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "pw@example.com", "old-pass-123")

	// Create an extra session that should be revoked after password change.
	stores.Sessions.Create(context.Background(), &model.Session{
		ID: "other-session", UserID: user.ID,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	ctx := auth.SetUserIDForTest(context.Background(), user.ID)
	ctx = auth.SetSessionIDForTest(ctx, "current-session")

	req := httptest.NewRequest("PUT", "/api/auth/password",
		strings.NewReader(`{"current_password":"old-pass-123","new_password":"new-pass-456"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	// Verify new password works and old doesn't.
	updated, _ := stores.Users.GetByID(context.Background(), user.ID)
	if !auth.CheckPassword(updated.Password, "new-pass-456") {
		t.Error("new password should validate")
	}
	if auth.CheckPassword(updated.Password, "old-pass-123") {
		t.Error("old password should no longer validate")
	}

	// Other session should have been revoked.
	if s, _ := stores.Sessions.GetByID(context.Background(), "other-session"); s != nil {
		t.Error("other session was not revoked")
	}
}

func TestChangePassword_WrongCurrent(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "pw2@example.com", "real-pass")

	req := httptest.NewRequest("PUT", "/api/auth/password",
		strings.NewReader(`{"current_password":"wrong","new_password":"new-pass-456"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(context.Background(), user.ID))
	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "wrong_password") {
		t.Errorf("body should contain wrong_password: %s", rec.Body.String())
	}
}

func TestChangePassword_TooShort(t *testing.T) {
	h, _, stores := newAuthHandler(t)
	user := testutil.MakeUser(t, stores, "pw3@example.com", "real-pass")

	req := httptest.NewRequest("PUT", "/api/auth/password",
		strings.NewReader(`{"current_password":"real-pass","new_password":"short"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(context.Background(), user.ID))
	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "password_too_short") {
		t.Errorf("body should contain password_too_short: %s", rec.Body.String())
	}
}

func TestChangePassword_Unauthenticated(t *testing.T) {
	h, _, _ := newAuthHandler(t)

	req := httptest.NewRequest("PUT", "/api/auth/password",
		strings.NewReader(`{"current_password":"x","new_password":"12345678"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ChangePassword(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
