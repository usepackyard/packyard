package composer

import (
	"encoding/json"
	"testing"

	"github.com/usepackyard/packyard/internal/model"
)

func TestBuildPackagesJSON_NoSlug_LegacyURL(t *testing.T) {
	// Empty slug produces unprefixed URLs — only seen during initial
	// seed when no org has been resolved yet. Asserts the fallback
	// path keeps producing a valid response shape.
	pkgs := []model.Package{
		{Name: "vendor/with-version", Versions: []model.Version{{Version: "1.0.0"}}},
		{Name: "vendor/empty"}, // no versions — should be excluded
		{Name: "vendor/another", Versions: []model.Version{{Version: "0.1.0"}}},
	}

	data, err := BuildPackagesJSON(pkgs, "")
	if err != nil {
		t.Fatalf("BuildPackagesJSON: %v", err)
	}

	var got PackagesJSON
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.MetadataURL != "/p2/%package%.json" {
		t.Errorf("MetadataURL = %q, want /p2/%%package%%.json", got.MetadataURL)
	}

	want := map[string]bool{"vendor/with-version": true, "vendor/another": true}
	if len(got.AvailablePackages) != len(want) {
		t.Fatalf("AvailablePackages = %v (want 2 entries excluding the empty package)", got.AvailablePackages)
	}
	for _, name := range got.AvailablePackages {
		if !want[name] {
			t.Errorf("unexpected package in list: %q", name)
		}
	}
}

func TestBuildPackagesJSON_WithSlug_TenantPrefixedURL(t *testing.T) {
	pkgs := []model.Package{
		{Name: "vendor/pkg", Versions: []model.Version{{Version: "1.0.0"}}},
	}

	data, err := BuildPackagesJSON(pkgs, "acme")
	if err != nil {
		t.Fatalf("BuildPackagesJSON: %v", err)
	}

	var got PackagesJSON
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	wantURL := "/acme/p2/%package%.json"
	if got.MetadataURL != wantURL {
		t.Errorf("MetadataURL = %q, want %q", got.MetadataURL, wantURL)
	}
}

func TestBuildProviderJSON_NoSlug_LegacyDistURL(t *testing.T) {
	pkg := model.Package{
		Name: "vendor/pkg",
		Type: "library",
		Versions: []model.Version{{
			Version:           "1.0.0",
			VersionNormalized: "1.0.0.0",
			DistType:          "zip",
			DistSHA1:          "deadbeefcafef00d",
			RequireJSON:       `{"php":">=8.0"}`,
		}},
	}

	data, err := BuildProviderJSON(pkg, "https://repo.example.com", "")
	if err != nil {
		t.Fatalf("BuildProviderJSON: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	pkgs := got["packages"].(map[string]any)
	versions := pkgs["vendor/pkg"].([]any)
	v := versions[0].(map[string]any)
	dist := v["dist"].(map[string]any)

	// Composer requires SHA-1 in the shasum field (regression test).
	if dist["shasum"] != "deadbeefcafef00d" {
		t.Errorf("shasum = %v, want deadbeefcafef00d", dist["shasum"])
	}
	wantURL := "https://repo.example.com/dist/vendor/pkg/1.0.0"
	if dist["url"] != wantURL {
		t.Errorf("dist.url = %v, want %s", dist["url"], wantURL)
	}

	require := v["require"].(map[string]any)
	if require["php"] != ">=8.0" {
		t.Errorf("require.php = %v", require["php"])
	}
}

func TestBuildProviderJSON_WithSlug_TenantPrefixedDistURL(t *testing.T) {
	pkg := model.Package{
		Name: "vendor/pkg",
		Type: "library",
		Versions: []model.Version{{
			Version:  "1.0.0",
			DistType: "zip",
			DistSHA1: "deadbeef",
		}},
	}

	data, err := BuildProviderJSON(pkg, "https://repo.example.com", "acme")
	if err != nil {
		t.Fatalf("BuildProviderJSON: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dist := got["packages"].(map[string]any)["vendor/pkg"].([]any)[0].(map[string]any)["dist"].(map[string]any)

	wantURL := "https://repo.example.com/acme/dist/vendor/pkg/1.0.0"
	if dist["url"] != wantURL {
		t.Errorf("dist.url = %v, want %s", dist["url"], wantURL)
	}
}
