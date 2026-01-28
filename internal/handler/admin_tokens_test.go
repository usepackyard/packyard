package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/testutil"
)

func TestAdminTokenHandler_Create_PlaintextOnceAndNoStore(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	user := testutil.MakeUser(t, stores, "owner@example.com", "p")

	h := handler.NewAdminTokenHandler(stores.Tokens, 4)

	req := httptest.NewRequest("POST", "/api/tokens",
		strings.NewReader(`{"name":"CI"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}

	// The plaintext token and password should appear in the response body.
	body := rec.Body.String()
	if !strings.Contains(body, `"token":"`) {
		t.Errorf("response should include plaintext token: %s", body)
	}
	if !strings.Contains(body, `"password":"`) {
		t.Errorf("response should include generated password: %s", body)
	}

	// Cache-Control should prevent intermediate caches from holding the token.
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want 'no-store'", got)
	}
}

func TestAdminTokenHandler_Create_RequiresName(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	h := handler.NewAdminTokenHandler(stores.Tokens, 4)

	req := httptest.NewRequest("POST", "/api/tokens",
		strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminTokenHandler_Delete_BadID(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	h := handler.NewAdminTokenHandler(stores.Tokens, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/tokens/{id}", h.Delete)

	req := httptest.NewRequest("DELETE", "/api/tokens/not-a-number", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminTokenHandler_Delete(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	_, tok := testutil.MakeToken(t, stores, org.ID)

	h := handler.NewAdminTokenHandler(stores.Tokens, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/tokens/{id}", h.Delete)

	req := httptest.NewRequest("DELETE", "/api/tokens/"+itoa(int(tok.ID)), nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestAdminTokenHandler_List(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	testutil.MakeToken(t, stores, org.ID)
	h := handler.NewAdminTokenHandler(stores.Tokens, 4)

	req := httptest.NewRequest("GET", "/api/tokens", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "test-token") {
		t.Errorf("response should list seeded token: %s", rec.Body.String())
	}
}
