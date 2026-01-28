package main

import (
	"context"
	"testing"

	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/testutil"
)

func TestSeedDefaults_SingleMode_CreatesSuperAdminAndOrg(t *testing.T) {
	stores := testutil.NewStores(t)
	cfg := &config.Config{
		Mode:       "single",
		BcryptCost: 4,
		Admin:      config.AdminConfig{Email: "admin@test", Password: "secret123"},
	}

	if err := seedDefaults(stores, cfg); err != nil {
		t.Fatalf("seedDefaults: %v", err)
	}

	// The seeded user is a super-admin.
	u, err := stores.Users.GetByEmail(context.Background(), "admin@test")
	if err != nil || u == nil {
		t.Fatalf("admin not found: %v", err)
	}
	if !u.IsSuperAdmin {
		t.Error("seeded admin should be a super-admin")
	}

	// And a default org is provisioned.
	org, err := stores.Orgs.GetBySlug(context.Background(), "default")
	if err != nil || org == nil {
		t.Fatalf("default org not found: %v", err)
	}

	// And the admin is a member of it.
	member, _ := stores.Orgs.GetMember(context.Background(), org.ID, u.ID)
	if member == nil || member.Role != "owner" {
		t.Errorf("admin should be owner of default org, got %+v", member)
	}
}

func TestSeedDefaults_MultiMode_CreatesSuperAdminAndDefaultOrg(t *testing.T) {
	stores := testutil.NewStores(t)
	cfg := &config.Config{
		Mode:       "multi",
		BcryptCost: 4,
		Admin:      config.AdminConfig{Email: "admin@test", Password: "secret123"},
	}

	if err := seedDefaults(stores, cfg); err != nil {
		t.Fatalf("seedDefaults: %v", err)
	}

	u, err := stores.Users.GetByEmail(context.Background(), "admin@test")
	if err != nil || u == nil {
		t.Fatalf("admin not found: %v", err)
	}
	if !u.IsSuperAdmin {
		t.Error("seeded admin should be a super-admin")
	}

	// Multi mode also seeds a default org so the super-admin has an org to
	// land in on first login. Additional tenants come from the admin API.
	org, err := stores.Orgs.GetBySlug(context.Background(), "default")
	if err != nil || org == nil {
		t.Fatalf("default org not found: %v", err)
	}

	member, _ := stores.Orgs.GetMember(context.Background(), org.ID, u.ID)
	if member == nil || member.Role != "owner" {
		t.Errorf("admin should be owner of default org, got %+v", member)
	}
}

func TestSeedDefaults_Idempotent(t *testing.T) {
	stores := testutil.NewStores(t)
	cfg := &config.Config{
		Mode:       "single",
		BcryptCost: 4,
		Admin:      config.AdminConfig{Email: "admin@test", Password: "secret123"},
	}

	if err := seedDefaults(stores, cfg); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Second call must be a no-op (no panics, no duplicates).
	if err := seedDefaults(stores, cfg); err != nil {
		t.Fatalf("second call: %v", err)
	}

	count, _ := stores.Users.Count(context.Background())
	if count != 1 {
		t.Errorf("user count = %d, want 1 after idempotent re-seed", count)
	}
}
