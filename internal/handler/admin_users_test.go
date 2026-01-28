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

func TestAdminUserHandler_List(t *testing.T) {
	stores := testutil.NewStores(t)
	testutil.MakeUser(t, stores, "a@example.com", "p")
	testutil.MakeUser(t, stores, "b@example.com", "p")
	h := handler.NewAdminUserHandler(stores.Users, 4)

	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "a@example.com") || !strings.Contains(body, "b@example.com") {
		t.Errorf("body should list both users: %s", body)
	}
}

func TestAdminUserHandler_Create_HappyPath(t *testing.T) {
	stores := testutil.NewStores(t)
	h := handler.NewAdminUserHandler(stores.Users, 4)

	req := httptest.NewRequest("POST", "/api/users",
		strings.NewReader(`{"email":"new@example.com","password":"pw12345","name":"New"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	// Password must not appear in the response.
	if strings.Contains(rec.Body.String(), "pw12345") {
		t.Error("response leaked plaintext password")
	}
}

func TestAdminUserHandler_Create_MissingFields(t *testing.T) {
	stores := testutil.NewStores(t)
	h := handler.NewAdminUserHandler(stores.Users, 4)

	cases := []string{
		`{"email":"","password":"x"}`,
		`{"email":"a@b.c","password":""}`,
	}
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/users", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.Create(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestAdminUserHandler_Create_DuplicateEmail(t *testing.T) {
	stores := testutil.NewStores(t)
	testutil.MakeUser(t, stores, "dup@example.com", "p")
	h := handler.NewAdminUserHandler(stores.Users, 4)

	req := httptest.NewRequest("POST", "/api/users",
		strings.NewReader(`{"email":"dup@example.com","password":"p","name":"X"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestAdminUserHandler_Delete(t *testing.T) {
	stores := testutil.NewStores(t)
	user := testutil.MakeUser(t, stores, "victim@example.com", "p")
	other := testutil.MakeUser(t, stores, "actor@example.com", "p")
	h := handler.NewAdminUserHandler(stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/users/{id}", h.Delete)

	req := httptest.NewRequest("DELETE", "/api/users/"+itoa(int(user.ID)), nil)
	// Acting as a different user.
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), other.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	// Confirm gone.
	got, _ := stores.Users.GetByID(context.Background(), user.ID)
	if got != nil {
		t.Errorf("user not deleted: %+v", got)
	}
}

func TestAdminUserHandler_SetSuperAdmin_Promote(t *testing.T) {
	stores := testutil.NewStores(t)
	target := testutil.MakeUser(t, stores, "target@example.com", "p")
	actor := testutil.MakeUser(t, stores, "actor@example.com", "p")
	h := handler.NewAdminUserHandler(stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/admin/users/{id}/super-admin", h.SetSuperAdmin)

	req := httptest.NewRequest("PUT", "/api/admin/users/"+itoa(int(target.ID))+"/super-admin",
		strings.NewReader(`{"is_super_admin":true}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), actor.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	got, _ := stores.Users.GetByID(context.Background(), target.ID)
	if !got.IsSuperAdmin {
		t.Errorf("target should now be super-admin")
	}
}

func TestAdminUserHandler_SetSuperAdmin_PreventsSelfRevoke(t *testing.T) {
	stores := testutil.NewStores(t)
	actor := testutil.MakeUser(t, stores, "actor@example.com", "p")
	actor.IsSuperAdmin = true
	if err := stores.Users.Update(context.Background(), actor); err != nil {
		t.Fatalf("promote: %v", err)
	}
	h := handler.NewAdminUserHandler(stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/admin/users/{id}/super-admin", h.SetSuperAdmin)

	req := httptest.NewRequest("PUT", "/api/admin/users/"+itoa(int(actor.ID))+"/super-admin",
		strings.NewReader(`{"is_super_admin":false}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), actor.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (self-revoke blocked)", rec.Code)
	}
	got, _ := stores.Users.GetByID(context.Background(), actor.ID)
	if !got.IsSuperAdmin {
		t.Errorf("self-revoke should not have changed flag")
	}
}

func TestAdminUserHandler_SetSuperAdmin_NotFound(t *testing.T) {
	stores := testutil.NewStores(t)
	actor := testutil.MakeUser(t, stores, "actor@example.com", "p")
	h := handler.NewAdminUserHandler(stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/admin/users/{id}/super-admin", h.SetSuperAdmin)

	req := httptest.NewRequest("PUT", "/api/admin/users/9999/super-admin",
		strings.NewReader(`{"is_super_admin":true}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), actor.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminUserHandler_Delete_PreventsSelfDelete(t *testing.T) {
	stores := testutil.NewStores(t)
	user := testutil.MakeUser(t, stores, "self@example.com", "p")
	h := handler.NewAdminUserHandler(stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/users/{id}", h.Delete)

	req := httptest.NewRequest("DELETE", "/api/users/"+itoa(int(user.ID)), nil)
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (self-delete blocked)", rec.Code)
	}
	// Confirm user still exists.
	got, _ := stores.Users.GetByID(context.Background(), user.ID)
	if got == nil {
		t.Error("user wrongly deleted despite self-delete guard")
	}
}
