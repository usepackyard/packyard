package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/testutil"
)

// -------- OrgStore --------

func TestOrgStore_CRUD(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	// Create.
	org := &model.Organization{Slug: "acme", Name: "Acme"}
	if err := stores.Orgs.Create(ctx, org); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if org.ID == 0 {
		t.Fatal("ID not set after Create")
	}

	// GetByID.
	got, err := stores.Orgs.GetByID(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.Slug != "acme" {
		t.Fatalf("GetByID returned %+v", got)
	}

	// GetBySlug.
	got, err = stores.Orgs.GetBySlug(ctx, "acme")
	if err != nil || got == nil {
		t.Fatalf("GetBySlug: %v / %+v", err, got)
	}

	// GetBySlug unknown → nil, no error.
	miss, err := stores.Orgs.GetBySlug(ctx, "missing")
	if err != nil {
		t.Fatalf("GetBySlug missing error: %v", err)
	}
	if miss != nil {
		t.Error("GetBySlug missing should return nil")
	}

	// List.
	testutil.MakeOrg(t, stores, "beta", "Beta")
	all, err := stores.Orgs.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List len = %d, want 2", len(all))
	}

	// Update.
	org.Name = "Acme Corp"
	if err := stores.Orgs.Update(ctx, org); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = stores.Orgs.GetByID(ctx, org.ID)
	if got.Name != "Acme Corp" {
		t.Errorf("Update did not persist: %+v", got)
	}

	// Delete.
	if err := stores.Orgs.Delete(ctx, org.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = stores.Orgs.GetByID(ctx, org.ID)
	if got != nil {
		t.Error("org still present after Delete")
	}
}

func TestOrgStore_UpdateStatus(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	// Newly created orgs must default to "active".
	if org.Status != "active" {
		t.Fatalf("new org status = %q, want %q", org.Status, "active")
	}

	for _, want := range []string{"suspended", "archived", "active"} {
		if err := stores.Orgs.UpdateStatus(ctx, org.ID, want); err != nil {
			t.Fatalf("UpdateStatus(%q): %v", want, err)
		}
		got, err := stores.Orgs.GetByID(ctx, org.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Status != want {
			t.Errorf("status = %q, want %q", got.Status, want)
		}
	}
}

func TestOrgStore_Members(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "acme", "Acme")
	user := testutil.MakeUser(t, stores, "u@example.com", "p")

	// AddMember.
	m := &model.OrgMember{OrgID: org.ID, UserID: user.ID, Role: "owner"}
	if err := stores.Orgs.AddMember(ctx, m); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	// GetMember.
	got, err := stores.Orgs.GetMember(ctx, org.ID, user.ID)
	if err != nil || got == nil {
		t.Fatalf("GetMember: %v / %+v", err, got)
	}
	if got.Role != "owner" {
		t.Errorf("Role = %q", got.Role)
	}

	// GetMember for non-member returns nil.
	other := testutil.MakeUser(t, stores, "other@example.com", "p")
	miss, err := stores.Orgs.GetMember(ctx, org.ID, other.ID)
	if err != nil {
		t.Fatalf("GetMember other: %v", err)
	}
	if miss != nil {
		t.Error("GetMember non-member should be nil")
	}

	// ListMembers includes the User relation.
	members, err := stores.Orgs.ListMembers(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("ListMembers len = %d", len(members))
	}

	// UpdateMember.
	m.Role = "member"
	m.Permissions = model.JSONStringSlice{"packages:read"}
	if err := stores.Orgs.UpdateMember(ctx, m); err != nil {
		t.Fatalf("UpdateMember: %v", err)
	}
	got, _ = stores.Orgs.GetMember(ctx, org.ID, user.ID)
	if got.Role != "member" || len(got.Permissions) != 1 {
		t.Errorf("UpdateMember did not persist: %+v", got)
	}

	// ListUserOrgs.
	orgs, err := stores.Orgs.ListUserOrgs(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListUserOrgs: %v", err)
	}
	if len(orgs) != 1 || orgs[0].ID != org.ID {
		t.Errorf("ListUserOrgs = %+v", orgs)
	}

	// RemoveMember.
	if err := stores.Orgs.RemoveMember(ctx, org.ID, user.ID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	got, _ = stores.Orgs.GetMember(ctx, org.ID, user.ID)
	if got != nil {
		t.Error("member still present after RemoveMember")
	}
}

// -------- UserStore --------

func TestUserStore_CRUD(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	u := testutil.MakeUser(t, stores, "a@example.com", "p")

	// GetByID.
	got, err := stores.Users.GetByID(ctx, u.ID)
	if err != nil || got == nil || got.Email != "a@example.com" {
		t.Fatalf("GetByID: %v / %+v", err, got)
	}

	// GetByEmail.
	got, err = stores.Users.GetByEmail(ctx, "a@example.com")
	if err != nil || got == nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	miss, _ := stores.Users.GetByEmail(ctx, "missing@example.com")
	if miss != nil {
		t.Error("GetByEmail missing should be nil")
	}

	// Count.
	testutil.MakeUser(t, stores, "b@example.com", "p")
	n, err := stores.Users.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Errorf("Count = %d, want 2", n)
	}

	// List.
	list, err := stores.Users.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("List len = %d", len(list))
	}

	// Update.
	u.Name = "Renamed"
	if err := stores.Users.Update(ctx, u); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = stores.Users.GetByID(ctx, u.ID)
	if got.Name != "Renamed" {
		t.Errorf("Update did not persist")
	}

	// Delete.
	if err := stores.Users.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = stores.Users.GetByID(ctx, u.ID)
	if got != nil {
		t.Error("user still present after Delete")
	}
}

func TestUserStore_IsSuperAdminAndListSuperAdmins(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	regular := testutil.MakeUser(t, stores, "regular@example.com", "p")
	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")

	// Default users are not super-admins.
	if regular.IsSuperAdmin {
		t.Errorf("regular user wrongly has IsSuperAdmin=true")
	}
	if admin.IsSuperAdmin {
		t.Errorf("new user wrongly has IsSuperAdmin=true before promotion")
	}

	// Promote admin.
	admin.IsSuperAdmin = true
	if err := stores.Users.Update(ctx, admin); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := stores.Users.GetByID(ctx, admin.ID)
	if !got.IsSuperAdmin {
		t.Errorf("promotion didn't persist: %+v", got)
	}

	// ListSuperAdmins returns only promoted users.
	supers, err := stores.Users.ListSuperAdmins(ctx)
	if err != nil {
		t.Fatalf("ListSuperAdmins: %v", err)
	}
	if len(supers) != 1 || supers[0].ID != admin.ID {
		t.Errorf("ListSuperAdmins = %+v, want 1 entry with ID=%d", supers, admin.ID)
	}
}

// -------- SessionStore --------

func TestSessionStore_CRUD(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	user := testutil.MakeUser(t, stores, "s@example.com", "p")

	sess := &model.Session{ID: "sess123", UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := stores.Sessions.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := stores.Sessions.GetByID(ctx, "sess123")
	if err != nil || got == nil {
		t.Fatalf("GetByID: %v / %+v", err, got)
	}
	if got.UserID != user.ID {
		t.Errorf("UserID = %d", got.UserID)
	}

	// GetByID unknown → nil, no error.
	miss, err := stores.Sessions.GetByID(ctx, "nope")
	if err != nil {
		t.Fatalf("GetByID unknown: %v", err)
	}
	if miss != nil {
		t.Error("GetByID unknown should be nil")
	}

	// Delete.
	if err := stores.Sessions.Delete(ctx, "sess123"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = stores.Sessions.GetByID(ctx, "sess123")
	if got != nil {
		t.Error("session still present after Delete")
	}
}

func TestSessionStore_DeleteExpired(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	user := testutil.MakeUser(t, stores, "s@example.com", "p")

	// One expired, one fresh.
	old := &model.Session{ID: "old", UserID: user.ID, ExpiresAt: time.Now().Add(-time.Hour)}
	fresh := &model.Session{ID: "fresh", UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := stores.Sessions.Create(ctx, old); err != nil {
		t.Fatalf("create old: %v", err)
	}
	if err := stores.Sessions.Create(ctx, fresh); err != nil {
		t.Fatalf("create fresh: %v", err)
	}

	if err := stores.Sessions.DeleteExpired(ctx); err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}

	oldGot, _ := stores.Sessions.GetByID(ctx, "old")
	if oldGot != nil {
		t.Error("expired session not deleted")
	}
	freshGot, _ := stores.Sessions.GetByID(ctx, "fresh")
	if freshGot == nil {
		t.Error("fresh session wrongly deleted")
	}
}

func TestSessionStore_DeleteByUserID(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	u := testutil.MakeUser(t, stores, "s@example.com", "p")
	other := testutil.MakeUser(t, stores, "o@example.com", "p")

	stores.Sessions.Create(ctx, &model.Session{ID: "u1", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})
	stores.Sessions.Create(ctx, &model.Session{ID: "u2", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})
	stores.Sessions.Create(ctx, &model.Session{ID: "other", UserID: other.ID, ExpiresAt: time.Now().Add(time.Hour)})

	if err := stores.Sessions.DeleteByUserID(ctx, u.ID); err != nil {
		t.Fatalf("DeleteByUserID: %v", err)
	}

	g, _ := stores.Sessions.GetByID(ctx, "u1")
	if g != nil {
		t.Error("u1 not deleted")
	}
	g, _ = stores.Sessions.GetByID(ctx, "other")
	if g == nil {
		t.Error("other user's session wrongly deleted")
	}
}

func TestSessionStore_DeleteOthersByUserID(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()
	u := testutil.MakeUser(t, stores, "sess@example.com", "p")
	other := testutil.MakeUser(t, stores, "other@example.com", "p")

	stores.Sessions.Create(ctx, &model.Session{ID: "keep", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})
	stores.Sessions.Create(ctx, &model.Session{ID: "kill1", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})
	stores.Sessions.Create(ctx, &model.Session{ID: "kill2", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})
	stores.Sessions.Create(ctx, &model.Session{ID: "untouched", UserID: other.ID, ExpiresAt: time.Now().Add(time.Hour)})

	if err := stores.Sessions.DeleteOthersByUserID(ctx, u.ID, "keep"); err != nil {
		t.Fatalf("DeleteOthersByUserID: %v", err)
	}

	if g, _ := stores.Sessions.GetByID(ctx, "keep"); g == nil {
		t.Error("kept session was deleted")
	}
	if g, _ := stores.Sessions.GetByID(ctx, "kill1"); g != nil {
		t.Error("kill1 not deleted")
	}
	if g, _ := stores.Sessions.GetByID(ctx, "kill2"); g != nil {
		t.Error("kill2 not deleted")
	}
	if g, _ := stores.Sessions.GetByID(ctx, "untouched"); g == nil {
		t.Error("other user's session wrongly deleted")
	}
}

// -------- PackageStore --------

func TestPackageStore_CRUD(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	otherOrg := testutil.MakeOrg(t, stores, "other", "Other")

	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	otherPkg := testutil.MakePackage(t, stores, otherOrg.ID, "vendor/theirs")

	// List is org-scoped.
	list, err := stores.Packages.List(ctx, org.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "vendor/pkg" {
		t.Errorf("List = %+v", list)
	}

	// GetByID org-scoped: other org can't see our package.
	miss, _ := stores.Packages.GetByID(ctx, otherOrg.ID, pkg.ID)
	if miss != nil {
		t.Errorf("cross-org GetByID leaked: %+v", miss)
	}

	// GetByIDGlobal ignores org (used by webhooks).
	global, err := stores.Packages.GetByIDGlobal(ctx, otherPkg.ID)
	if err != nil || global == nil {
		t.Fatalf("GetByIDGlobal: %v / %+v", err, global)
	}

	// GetByName org-scoped.
	got, _ := stores.Packages.GetByName(ctx, org.ID, "vendor/pkg")
	if got == nil {
		t.Error("GetByName should return the package")
	}

	// Versions.
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	versions, err := stores.Packages.ListVersions(ctx, org.ID, pkg.ID)
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Errorf("ListVersions len = %d", len(versions))
	}

	vg, err := stores.Packages.GetVersionByID(ctx, v.ID)
	if err != nil || vg == nil {
		t.Fatalf("GetVersionByID: %v / %+v", err, vg)
	}

	// ListAllWithVersions.
	all, err := stores.Packages.ListAllWithVersions(ctx, org.ID)
	if err != nil {
		t.Fatalf("ListAllWithVersions: %v", err)
	}
	if len(all) != 1 || len(all[0].Versions) != 1 {
		t.Errorf("ListAllWithVersions = %+v", all)
	}

	// DeleteVersion.
	if err := stores.Packages.DeleteVersion(ctx, v.ID); err != nil {
		t.Fatalf("DeleteVersion: %v", err)
	}

	// Delete package.
	if err := stores.Packages.Delete(ctx, org.ID, pkg.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = stores.Packages.GetByID(ctx, org.ID, pkg.ID)
	if got != nil {
		t.Error("package still present after Delete")
	}
}

// UpdateVersionCreatedAt is the backfill path used during re-sync when
// we discover a new upstream publish date. Only created_at should move;
// dist_sha1 / file_size / storage_path must stay put (Composer integrity).
func TestPackageStore_UpdateVersionCreatedAt(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha-immutable", 4096)

	// Snapshot immutable fields so we can prove they weren't touched.
	origSHA1 := v.DistSHA1
	origSize := v.FileSize
	origPath := v.StoragePath

	backdated := time.Date(2020, 3, 14, 9, 26, 0, 0, time.UTC)
	if err := stores.Packages.UpdateVersionCreatedAt(ctx, v.ID, backdated); err != nil {
		t.Fatalf("UpdateVersionCreatedAt: %v", err)
	}

	got, _ := stores.Packages.GetVersionByID(ctx, v.ID)
	if !got.CreatedAt.Equal(backdated) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, backdated)
	}
	if got.DistSHA1 != origSHA1 {
		t.Errorf("DistSHA1 changed: %q → %q", origSHA1, got.DistSHA1)
	}
	if got.FileSize != origSize {
		t.Errorf("FileSize changed: %d → %d", origSize, got.FileSize)
	}
	if got.StoragePath != origPath {
		t.Errorf("StoragePath changed: %q → %q", origPath, got.StoragePath)
	}
}

func TestPackageStore_IncrementDownload_IncrementsCounterAndStamp(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	v := testutil.MakeVersion(t, stores, pkg.ID, "1.0.0", "sha", 100)

	// Baseline: new version has zero downloads, no last_downloaded_at.
	got, _ := stores.Packages.GetVersionByID(ctx, v.ID)
	if got.DownloadCount != 0 {
		t.Errorf("initial DownloadCount = %d, want 0", got.DownloadCount)
	}
	if got.LastDownloadedAt != nil {
		t.Errorf("initial LastDownloadedAt = %v, want nil", got.LastDownloadedAt)
	}

	// Two increments must bump the counter and stamp the time.
	t1 := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 14, 10, 5, 0, 0, time.UTC)
	if err := stores.Packages.IncrementDownload(ctx, v.ID, t1); err != nil {
		t.Fatalf("IncrementDownload t1: %v", err)
	}
	if err := stores.Packages.IncrementDownload(ctx, v.ID, t2); err != nil {
		t.Fatalf("IncrementDownload t2: %v", err)
	}

	got, _ = stores.Packages.GetVersionByID(ctx, v.ID)
	if got.DownloadCount != 2 {
		t.Errorf("DownloadCount = %d, want 2", got.DownloadCount)
	}
	if got.LastDownloadedAt == nil || !got.LastDownloadedAt.Equal(t2) {
		t.Errorf("LastDownloadedAt = %v, want %v", got.LastDownloadedAt, t2)
	}
}

// -------- TokenStore --------

func TestTokenStore_CRUD(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	other := testutil.MakeOrg(t, stores, "other", "Other")
	plaintext, tok := testutil.MakeToken(t, stores, org.ID)

	// GetByHash finds it.
	hash := tok.TokenHash
	got, err := stores.Tokens.GetByHash(ctx, hash)
	if err != nil || got == nil {
		t.Fatalf("GetByHash: %v / %+v", err, got)
	}
	if got.OrgID != org.ID {
		t.Errorf("OrgID = %d", got.OrgID)
	}
	_ = plaintext

	// List is org-scoped.
	list, err := stores.Tokens.List(ctx, org.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List len = %d", len(list))
	}
	otherList, _ := stores.Tokens.List(ctx, other.ID)
	if len(otherList) != 0 {
		t.Errorf("other org list should be empty: %+v", otherList)
	}

	// UpdateLastUsed.
	if err := stores.Tokens.UpdateLastUsed(ctx, tok.ID); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}
	got, _ = stores.Tokens.GetByHash(ctx, hash)
	if got.LastUsedAt == nil {
		t.Error("LastUsedAt not set")
	}

	// Delete — org-scoped, wrong org should no-op.
	if err := stores.Tokens.Delete(ctx, other.ID, tok.ID); err != nil {
		t.Fatalf("Delete wrong org: %v", err)
	}
	got, _ = stores.Tokens.GetByHash(ctx, hash)
	if got == nil {
		t.Error("Delete with wrong org should not have removed token")
	}

	// Delete — correct org.
	if err := stores.Tokens.Delete(ctx, org.ID, tok.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = stores.Tokens.GetByHash(ctx, hash)
	if got != nil {
		t.Error("token still present after Delete")
	}
}

// -------- AdminTokenStore --------

func TestAdminTokenStore_CRUD(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	admin := testutil.MakeUser(t, stores, "admin@example.com", "p")

	tok := &model.AdminToken{
		Name:        "CI Token",
		TokenHash:   "abcd1234hash",
		TokenPrefix: "adm_12345",
		CreatedBy:   admin.ID,
		IsActive:    true,
	}
	if err := stores.AdminTokens.Create(ctx, tok); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tok.ID == 0 {
		t.Fatal("ID not set after Create")
	}

	got, err := stores.AdminTokens.GetByHash(ctx, "abcd1234hash")
	if err != nil || got == nil {
		t.Fatalf("GetByHash: %v / %+v", err, got)
	}
	if got.Name != "CI Token" {
		t.Errorf("Name = %q", got.Name)
	}

	// Unknown hash → nil, no error.
	miss, err := stores.AdminTokens.GetByHash(ctx, "nope")
	if err != nil {
		t.Fatalf("GetByHash unknown: %v", err)
	}
	if miss != nil {
		t.Error("unknown hash should return nil")
	}

	// List returns our token.
	list, err := stores.AdminTokens.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List len = %d", len(list))
	}

	// UpdateLastUsed sets the timestamp.
	if err := stores.AdminTokens.UpdateLastUsed(ctx, tok.ID); err != nil {
		t.Fatalf("UpdateLastUsed: %v", err)
	}
	got, _ = stores.AdminTokens.GetByHash(ctx, "abcd1234hash")
	if got.LastUsedAt == nil {
		t.Error("LastUsedAt not set after UpdateLastUsed")
	}

	// Delete.
	if err := stores.AdminTokens.Delete(ctx, tok.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = stores.AdminTokens.GetByHash(ctx, "abcd1234hash")
	if got != nil {
		t.Error("token still present after Delete")
	}
}

// -------- SourceStore --------

func TestSourceStore_CRUD(t *testing.T) {
	stores := testutil.NewStores(t)
	ctx := context.Background()

	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")

	src := &model.PackageSource{
		PackageID:     pkg.ID,
		Provider:      "github",
		RepoOwner:     "octo",
		RepoName:      "hello",
		Strategy:      "release_asset",
		AssetPattern:  "*.zip",
		WebhookSecret: "shhh",
	}
	if err := stores.Sources.Create(ctx, src); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := stores.Sources.GetByPackageID(ctx, pkg.ID)
	if err != nil || got == nil {
		t.Fatalf("GetByPackageID: %v / %+v", err, got)
	}

	gotByRepo, err := stores.Sources.GetByRepo(ctx, "github", "octo", "hello")
	if err != nil || gotByRepo == nil {
		t.Fatalf("GetByRepo: %v / %+v", err, gotByRepo)
	}
	missByRepo, _ := stores.Sources.GetByRepo(ctx, "github", "nobody", "nothing")
	if missByRepo != nil {
		t.Error("GetByRepo missing should be nil")
	}

	// Update.
	src.AssetPattern = "*.tgz"
	if err := stores.Sources.Update(ctx, src); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// UpdateLastSynced sets a timestamp.
	if err := stores.Sources.UpdateLastSynced(ctx, pkg.ID); err != nil {
		t.Fatalf("UpdateLastSynced: %v", err)
	}
	got, _ = stores.Sources.GetByPackageID(ctx, pkg.ID)
	if got.LastSyncedAt == nil {
		t.Error("LastSyncedAt not set")
	}

	// Delete.
	if err := stores.Sources.Delete(ctx, pkg.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = stores.Sources.GetByPackageID(ctx, pkg.ID)
	if got != nil {
		t.Error("source still present after Delete")
	}
}
