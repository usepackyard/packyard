package composer

import (
	"encoding/json"
	"fmt"

	"github.com/usepackyard/packyard/internal/model"
)

// PackagesJSON generates the Composer v2 repository root response.
type PackagesJSON struct {
	MetadataURL       string   `json:"metadata-url"`
	AvailablePackages []string `json:"available-packages"`
}

// BuildPackagesJSON generates the root packages.json for Composer v2.
// orgSlug is empty in single mode (URLs stay tenant-less for self-hosters)
// and set in multi mode so the metadata-url and dist URLs are scoped per
// tenant: /{slug}/p2/... and /{slug}/dist/...
func BuildPackagesJSON(packages []model.Package, orgSlug string) ([]byte, error) {
	names := make([]string, 0, len(packages))
	for _, p := range packages {
		if len(p.Versions) > 0 {
			names = append(names, p.Name)
		}
	}

	prefix := ""
	if orgSlug != "" {
		prefix = "/" + orgSlug
	}
	result := PackagesJSON{
		MetadataURL:       prefix + "/p2/%package%.json",
		AvailablePackages: names,
	}

	return json.Marshal(result)
}

// ProviderJSON generates the per-package provider response for Composer v2.
// orgSlug behaves like in BuildPackagesJSON.
func BuildProviderJSON(pkg model.Package, baseURL, orgSlug string) ([]byte, error) {
	type distInfo struct {
		Type   string `json:"type"`
		URL    string `json:"url"`
		Shasum string `json:"shasum"`
	}

	type versionEntry struct {
		Name              string            `json:"name"`
		Version           string            `json:"version"`
		VersionNormalized string            `json:"version_normalized"`
		Type              string            `json:"type"`
		Dist              distInfo          `json:"dist"`
		Require           map[string]string `json:"require,omitempty"`
	}

	prefix := ""
	if orgSlug != "" {
		prefix = "/" + orgSlug
	}
	entries := make([]versionEntry, 0, len(pkg.Versions))
	for _, v := range pkg.Versions {
		entry := versionEntry{
			Name:              pkg.Name,
			Version:           v.Version,
			VersionNormalized: v.VersionNormalized,
			Type:              pkg.Type,
			Dist: distInfo{
				Type:   v.DistType,
				URL:    fmt.Sprintf("%s%s/dist/%s/%s", baseURL, prefix, pkg.Name, v.Version),
				Shasum: v.DistSHA1,
			},
		}

		if v.RequireJSON != "" {
			var req map[string]string
			if err := json.Unmarshal([]byte(v.RequireJSON), &req); err == nil {
				entry.Require = req
			}
		}

		entries = append(entries, entry)
	}

	result := map[string]interface{}{
		"packages": map[string]interface{}{
			pkg.Name: entries,
		},
	}

	return json.Marshal(result)
}
