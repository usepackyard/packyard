package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/testutil"
)

// captureOrgHandler records the org and member it sees in context.
type captured struct {
	org    *model.Organization
	member *model.OrgMember
	hit    bool
}

func captureHandler(c *captured) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.hit = true
		c.org = auth.OrgFromContext(r.Context())
		c.member = auth.MemberFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
}

func TestOrgMiddleware_RequiresMembership(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := &model.Organization{Slug: "acme", Name: "Acme"}
	if err := stores.Orgs.Create(ctx, org); err != nil {
		t.Fatalf("create org: %v", err)
	}
	user := &model.User{Email: "user@example.com", Password: "x", IsActive: true}
	if err := stores.Users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Build a small mux so {org} path-value resolution works.
	mux := http.NewServeMux()
	c := &captured{}
	mux.Handle("GET /api/orgs/{org}/x", auth.OrgMiddleware(stores.Orgs)(captureHandler(c)))

	t.Run("not authenticated", func(t *testing.T) {
		c.hit = false
		req := httptest.NewRequest("GET", "/api/orgs/acme/x", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("authenticated but not a member", func(t *testing.T) {
		c.hit = false
		req := httptest.NewRequest("GET", "/api/orgs/acme/x", nil)
		// Manually inject a user_id so OrgMiddleware sees authentication.
		ctx := auth.SetUserIDForTest(req.Context(), user.ID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("member", func(t *testing.T) {
		if err := stores.Orgs.AddMember(ctx, &model.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "owner"}); err != nil {
			t.Fatalf("add member: %v", err)
		}
		c.hit = false
		req := httptest.NewRequest("GET", "/api/orgs/acme/x", nil)
		ctx := auth.SetUserIDForTest(req.Context(), user.ID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		if c.org == nil || c.org.ID != org.ID {
			t.Errorf("org in context = %+v, want id=%d", c.org, org.ID)
		}
		if c.member == nil || c.member.Role != "owner" {
			t.Errorf("member in context = %+v, want role=owner", c.member)
		}
	})

	t.Run("unknown org", func(t *testing.T) {
		c.hit = false
		req := httptest.NewRequest("GET", "/api/orgs/missing/x", nil)
		ctx := auth.SetUserIDForTest(req.Context(), user.ID)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

func TestOrgMiddleware_SuspendedOrgReturns402(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	user := testutil.MakeUser(t, stores, "u@example.com", "p")
	testutil.MakeMember(t, stores, org.ID, user.ID, "owner")
	if err := stores.Orgs.UpdateStatus(ctx, org.ID, "suspended"); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /api/orgs/{org}/x", auth.OrgMiddleware(stores.Orgs)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})))

	req := httptest.NewRequest("GET", "/api/orgs/acme/x", nil)
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", rec.Code)
	}
}

func TestOrgMiddleware_ArchivedOrgReturns404(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	user := testutil.MakeUser(t, stores, "u@example.com", "p")
	testutil.MakeMember(t, stores, org.ID, user.ID, "owner")
	if err := stores.Orgs.UpdateStatus(ctx, org.ID, "archived"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /api/orgs/{org}/x", auth.OrgMiddleware(stores.Orgs)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})))

	req := httptest.NewRequest("GET", "/api/orgs/acme/x", nil)
	req = req.WithContext(auth.SetUserIDForTest(req.Context(), user.ID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (archived)", rec.Code)
	}
}

func TestAdminOrgFromSlug(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	testutil.MakeOrg(t, stores, "acme", "Acme")
	archived := testutil.MakeOrg(t, stores, "old", "Old")
	if err := stores.Orgs.UpdateStatus(ctx, archived.ID, "archived"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /api/admin/orgs/{slug}/x", auth.AdminOrgFromSlug(stores.Orgs)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			org := auth.OrgFromContext(r.Context())
			if org == nil {
				t.Error("org not in context")
			}
			w.WriteHeader(http.StatusOK)
		})))

	tests := []struct {
		name     string
		path     string
		wantCode int
	}{
		{"active org", "/api/admin/orgs/acme/x", http.StatusOK},
		{"unknown slug", "/api/admin/orgs/missing/x", http.StatusNotFound},
		{"archived org", "/api/admin/orgs/old/x", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
		})
	}
}

func TestRequirePermission(t *testing.T) {
	mw := auth.RequirePermission("packages:write")
	final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name   string
		member *model.OrgMember
		want   int
	}{
		{"no member in context → forbidden", nil, http.StatusForbidden},
		{"owner → bypass", &model.OrgMember{Role: "owner", Permissions: model.JSONStringSlice{}}, http.StatusOK},
		{"has explicit permission", &model.OrgMember{Role: "member", Permissions: model.JSONStringSlice{"packages:read", "packages:write"}}, http.StatusOK},
		{"lacks permission", &model.OrgMember{Role: "member", Permissions: model.JSONStringSlice{"packages:read"}}, http.StatusForbidden},
		{"empty permissions", &model.OrgMember{Role: "member"}, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/x", nil)
			ctx := req.Context()
			ctx = auth.SetOrgInContext(ctx, &model.Organization{ID: 1}, tt.member)
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			mw(final).ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
