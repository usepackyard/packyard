package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/testutil"
)

func TestAdminBearerTokenHandler_Create_PlaintextOnceAndNoStore(t *testing.T) {
	stores := testutil.NewStores(t)
	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")
	h := handler.NewAdminBearerTokenHandler(stores.AdminTokens)

	req := httptest.NewRequest("POST", "/api/admin/tokens",
		strings.NewReader(`{"name":"CI"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), admin.ID))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"token":"adm_`) {
		t.Errorf("response should include plaintext token with adm_ prefix: %s", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control should be no-store, got %q", rec.Header().Get("Cache-Control"))
	}
}

func TestAdminBearerTokenHandler_Create_RequiresName(t *testing.T) {
	stores := testutil.NewStores(t)
	h := handler.NewAdminBearerTokenHandler(stores.AdminTokens)

	rec := testutil.DoJSON(t, http.HandlerFunc(h.Create), "POST", "/api/admin/tokens",
		map[string]string{"name": ""})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminBearerTokenHandler_List(t *testing.T) {
	stores := testutil.NewStores(t)
	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")
	h := handler.NewAdminBearerTokenHandler(stores.AdminTokens)

	// Mint two tokens via Create.
	for _, name := range []string{"CI", "Backup"} {
		req := httptest.NewRequest("POST", "/api/admin/tokens",
			strings.NewReader(`{"name":"`+name+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.SetUserIDForTest(req.Context(), admin.ID))
		h.Create(httptest.NewRecorder(), req)
	}

	rec := testutil.DoJSON(t, http.HandlerFunc(h.List), "GET", "/api/admin/tokens", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "CI") || !strings.Contains(body, "Backup") {
		t.Errorf("response should include both tokens: %s", body)
	}
	// Must not include the plaintext.
	if strings.Contains(body, `"token":"adm_`) {
		t.Errorf("List should not return plaintext tokens: %s", body)
	}
}

func TestAdminBearerTokenHandler_Delete(t *testing.T) {
	stores := testutil.NewStores(t)
	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")
	h := handler.NewAdminBearerTokenHandler(stores.AdminTokens)

	// Create one to delete.
	createReq := httptest.NewRequest("POST", "/api/admin/tokens",
		strings.NewReader(`{"name":"To-Delete"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq = createReq.WithContext(auth.SetUserIDForTest(createReq.Context(), admin.ID))
	h.Create(httptest.NewRecorder(), createReq)

	tokens, _ := stores.AdminTokens.List(context.Background())
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token before delete, got %d", len(tokens))
	}
	id := tokens[0].ID

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/admin/tokens/{id}", h.Delete)

	delReq := httptest.NewRequest("DELETE", "/api/admin/tokens/"+itoa(int(id)), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, delReq)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	tokens, _ = stores.AdminTokens.List(context.Background())
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens after delete, got %d", len(tokens))
	}
}
