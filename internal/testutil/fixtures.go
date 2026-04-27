package testutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/store"
)

// MakeOrg inserts an organization and returns the populated record (ID set).
func MakeOrg(t *testing.T, stores *store.Stores, slug, name string) *model.Organization {
	t.Helper()
	org := &model.Organization{Slug: slug, Name: name}
	if err := stores.Orgs.Create(context.Background(), org); err != nil {
		t.Fatalf("create org %q: %v", slug, err)
	}
	return org
}

// MakeUser inserts a user with a bcrypt-hashed password and returns the record.
// Use bcrypt cost 4 in tests — fast and not exposed to anything real.
func MakeUser(t *testing.T, stores *store.Stores, email, password string) *model.User {
	t.Helper()
	hash, err := auth.HashPassword(password, 4)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u := &model.User{
		Email: email, Password: hash, Name: email, IsActive: true,
	}
	if err := stores.Users.Create(context.Background(), u); err != nil {
		t.Fatalf("create user %q: %v", email, err)
	}
	return u
}

// MakeMember adds a user as a member of an org with the given role and permissions.
func MakeMember(t *testing.T, stores *store.Stores, orgID, userID int64, role string, perms ...string) *model.OrgMember {
	t.Helper()
	m := &model.OrgMember{
		OrgID: orgID, UserID: userID, Role: role,
		Permissions: model.JSONStringSlice(perms),
	}
	if err := stores.Orgs.AddMember(context.Background(), m); err != nil {
		t.Fatalf("add member: %v", err)
	}
	return m
}

// MakeToken creates an API token. Returns the plaintext value (use as the
// Composer Basic-auth username) and the persisted record.
func MakeToken(t *testing.T, stores *store.Stores, orgID int64) (string, *model.APIToken) {
	t.Helper()
	plaintext := "tok_test_" + hex.EncodeToString([]byte(t.Name()))
	hash := sha256.Sum256([]byte(plaintext))
	tok := &model.APIToken{
		OrgID:       orgID,
		Name:        "test-token",
		TokenHash:   hex.EncodeToString(hash[:]),
		TokenPrefix: plaintext[:8],
		IsActive:    true,
	}
	if err := stores.Tokens.Create(context.Background(), tok); err != nil {
		t.Fatalf("create token: %v", err)
	}
	return plaintext, tok
}

// MakePackage inserts a package and returns the populated record.
func MakePackage(t *testing.T, stores *store.Stores, orgID int64, name string) *model.Package {
	t.Helper()
	p := &model.Package{
		OrgID: orgID, Name: name, Type: "library",
	}
	if err := stores.Packages.Create(context.Background(), p); err != nil {
		t.Fatalf("create package %q: %v", name, err)
	}
	return p
}

// MakeDefaultUploadSource attaches the default upload+from_zip source
// to a package, mirroring what the AdminPackageHandler.Create flow
// auto-provisions. Tests that hit the upload handler or expect the
// "every package has a source" invariant call this after
// MakePackage — tests exercising the "no source yet" transitional
// states skip it.
func MakeDefaultUploadSource(t *testing.T, stores *store.Stores, packageID int64) *model.PackageSource {
	t.Helper()
	src := &model.PackageSource{
		PackageID:      packageID,
		Provider:       "upload",
		MetadataSource: "from_zip",
		VersionSource:  "composer_json",
	}
	if err := stores.Sources.Create(context.Background(), src); err != nil {
		t.Fatalf("create default upload source: %v", err)
	}
	return src
}

// SourceConfigJSON returns the serialized provider config used by git-backed
// package sources. Tests use this instead of old flat repo fields.
func SourceConfigJSON(t *testing.T, owner, repo, strategy, assetPattern string) string {
	t.Helper()
	raw, err := json.Marshal(provider.SourceConfig{
		Owner:        owner,
		Repo:         repo,
		Strategy:     strategy,
		AssetPattern: assetPattern,
	})
	if err != nil {
		t.Fatalf("marshal source config: %v", err)
	}
	return string(raw)
}

// MakeVersion inserts a version row for an existing package.
func MakeVersion(t *testing.T, stores *store.Stores, packageID int64, version, sha1Hex string, fileSize int64) *model.Version {
	t.Helper()
	v := &model.Version{
		PackageID:         packageID,
		Version:           version,
		VersionNormalized: version + ".0",
		DistType:          "zip",
		DistSHA1:          sha1Hex,
		StoragePath:       "test/" + version + ".zip",
		FileSize:          fileSize,
		ComposerJSON:      `{"name":"test/pkg","version":"` + version + `"}`,
		CreatedAt:         time.Now(),
	}
	if err := stores.Packages.CreateVersion(context.Background(), v); err != nil {
		t.Fatalf("create version: %v", err)
	}
	return v
}
