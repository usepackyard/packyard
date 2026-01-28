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

func TestAdminMemberHandler_Add_NewUser(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	req := httptest.NewRequest("POST", "/api/members", strings.NewReader(
		`{"email":"new@example.com","password":"secret123","name":"New","role":"member","permissions":["packages:read"]}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.Add(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminMemberHandler_Add_RejectsInvalidRole(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	req := httptest.NewRequest("POST", "/api/members", strings.NewReader(
		`{"email":"x@example.com","role":"superadmin"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.Add(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminMemberHandler_Add_ExistingUser(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	testutil.MakeUser(t, stores, "existing@example.com", "p")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	// User exists but not yet a member of this org. Add should succeed
	// without requiring a new password.
	req := httptest.NewRequest("POST", "/api/members", strings.NewReader(
		`{"email":"existing@example.com","role":"member"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.Add(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminMemberHandler_Add_AlreadyMember(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	user := testutil.MakeUser(t, stores, "u@example.com", "p")
	testutil.MakeMember(t, stores, org.ID, user.ID, "member")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	req := httptest.NewRequest("POST", "/api/members", strings.NewReader(
		`{"email":"u@example.com","role":"member"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.Add(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestAdminMemberHandler_Add_NewUserRequiresPassword(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	req := httptest.NewRequest("POST", "/api/members", strings.NewReader(
		`{"email":"new@example.com","role":"member"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.Add(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (new user needs password)", rec.Code)
	}
}

func TestAdminMemberHandler_List(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	user := testutil.MakeUser(t, stores, "u@example.com", "p")
	testutil.MakeMember(t, stores, org.ID, user.ID, "owner")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	req := httptest.NewRequest("GET", "/api/members", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "u@example.com") {
		t.Errorf("response missing seeded member: %s", rec.Body.String())
	}
}

func TestAdminMemberHandler_Update_RoleAndPermissions(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	user := testutil.MakeUser(t, stores, "u@example.com", "p")
	testutil.MakeMember(t, stores, org.ID, user.ID, "member")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/members/{id}", h.Update)

	req := httptest.NewRequest("PUT", "/api/members/"+itoa(int(user.ID)),
		strings.NewReader(`{"role":"owner","permissions":["packages:write"]}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"role":"owner"`) {
		t.Errorf("response should reflect new role: %s", rec.Body.String())
	}
}

func TestAdminMemberHandler_Update_RejectsBadRole(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	user := testutil.MakeUser(t, stores, "u@example.com", "p")
	testutil.MakeMember(t, stores, org.ID, user.ID, "member")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/members/{id}", h.Update)

	req := httptest.NewRequest("PUT", "/api/members/"+itoa(int(user.ID)),
		strings.NewReader(`{"role":"superadmin"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminMemberHandler_Update_NotFound(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/members/{id}", h.Update)

	req := httptest.NewRequest("PUT", "/api/members/9999",
		strings.NewReader(`{"role":"owner"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminMemberHandler_Remove(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	target := testutil.MakeUser(t, stores, "target@example.com", "p")
	actor := testutil.MakeUser(t, stores, "actor@example.com", "p")
	testutil.MakeMember(t, stores, org.ID, target.ID, "member")
	testutil.MakeMember(t, stores, org.ID, actor.ID, "owner")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/members/{id}", h.Remove)

	req := httptest.NewRequest("DELETE", "/api/members/"+itoa(int(target.ID)), nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), actor.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminMemberHandler_Remove_PreventsSelfRemove(t *testing.T) {
	stores := testutil.NewStores(t)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	user := testutil.MakeUser(t, stores, "self@example.com", "p")
	testutil.MakeMember(t, stores, org.ID, user.ID, "owner")
	h := handler.NewAdminMemberHandler(stores.Orgs, stores.Users, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/members/{id}", h.Remove)

	req := httptest.NewRequest("DELETE", "/api/members/"+itoa(int(user.ID)), nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), org, nil))
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (self-remove blocked)", rec.Code)
	}
}
