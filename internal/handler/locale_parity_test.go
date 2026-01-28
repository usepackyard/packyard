package handler_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/usepackyard/packyard/internal/i18n"
)

// TestLocaleParity guards the invariant that every translation key in
// the English catalog (the source of truth) has a matching key in each
// non-English locale. i18next falls back to en at runtime for missing
// keys, so drift isn't immediately visible — this test surfaces it.
//
// Scope: checks every `*.json` namespace under frontend/src/locales/en/
// against its sibling at frontend/src/locales/<lang>/. New locales are
// discovered automatically by directory listing. Extra keys in a
// non-English locale are allowed (they're just unused), but missing
// keys fail the test.
func TestLocaleParity(t *testing.T) {
	base := filepath.Join("..", "..", "frontend", "src", "locales")
	enDir := filepath.Join(base, "en")

	enNamespaces, err := listNamespaces(enDir)
	if err != nil {
		t.Fatalf("list en namespaces: %v", err)
	}
	if len(enNamespaces) == 0 {
		t.Fatal("no English namespace catalogs found")
	}

	locales, err := listLocales(base)
	if err != nil {
		t.Fatalf("list locales: %v", err)
	}

	for _, lang := range locales {
		if lang == "en" {
			continue
		}
		t.Run(lang, func(t *testing.T) {
			for _, ns := range enNamespaces {
				enPath := filepath.Join(enDir, ns)
				otherPath := filepath.Join(base, lang, ns)
				if _, err := os.Stat(otherPath); err != nil {
					t.Errorf("missing namespace file: %s", otherPath)
					continue
				}
				enKeys := loadFlatKeys(t, enPath)
				otherKeys := loadFlatKeys(t, otherPath)
				missing := diffKeys(enKeys, otherKeys)
				if len(missing) > 0 {
					sort.Strings(missing)
					t.Errorf("%s: missing keys (present in en):\n  %s",
						ns, strings.Join(missing, "\n  "))
				}
			}
		})
	}
}

// TestLocaleManifestMatchesCatalogs ensures every code in the central
// languages.json manifest has a matching catalog directory in
// frontend/src/locales/, and vice-versa. Catches the case where someone
// adds a locale to the manifest but forgets to create translations, or
// adds a catalog directory without registering it in the manifest.
func TestLocaleManifestMatchesCatalogs(t *testing.T) {
	base := filepath.Join("..", "..", "frontend", "src", "locales")
	catalogLocales, err := listLocales(base)
	if err != nil {
		t.Fatalf("list catalog locales: %v", err)
	}
	manifestCodes := i18n.Codes()

	catalogSet := make(map[string]bool, len(catalogLocales))
	for _, c := range catalogLocales {
		catalogSet[c] = true
	}
	manifestSet := make(map[string]bool, len(manifestCodes))
	for _, c := range manifestCodes {
		manifestSet[c] = true
	}

	for _, code := range manifestCodes {
		if !catalogSet[code] {
			t.Errorf("manifest lists %q but no catalog directory exists at frontend/src/locales/%s/", code, code)
		}
	}
	for _, code := range catalogLocales {
		if !manifestSet[code] {
			t.Errorf("catalog directory frontend/src/locales/%s/ exists but %q is not in languages.json manifest", code, code)
		}
	}
}

func listNamespaces(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func listLocales(base string) ([]string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// loadFlatKeys flattens nested JSON objects to dot-delimited keys. Only
// leaf values count — interior objects don't (their keys are implicit
// in the leaf paths). Arrays are treated as leaves.
func loadFlatKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make(map[string]bool)
	flatten("", raw, out)
	return out
}

func flatten(prefix string, v interface{}, out map[string]bool) {
	switch m := v.(type) {
	case map[string]interface{}:
		for k, child := range m {
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			flatten(next, child, out)
		}
	default:
		out[prefix] = true
	}
}

func diffKeys(src, dst map[string]bool) []string {
	var missing []string
	for k := range src {
		if !dst[k] {
			missing = append(missing, k)
		}
	}
	return missing
}

// Sanity check that the flatten helper correctly walks nested maps —
// easier to debug locally than staring at a cryptic parity failure.
func TestLocaleParity_FlattenSanity(t *testing.T) {
	input := map[string]interface{}{
		"a": "1",
		"b": map[string]interface{}{
			"c": "2",
			"d": map[string]interface{}{"e": "3"},
		},
	}
	out := make(map[string]bool)
	flatten("", input, out)
	want := []string{"a", "b.c", "b.d.e"}
	for _, k := range want {
		if !out[k] {
			t.Errorf("missing key %q in %v", k, out)
		}
	}
	if len(out) != len(want) {
		t.Errorf("got %d keys (%v), want %d (%v)", len(out), out, len(want), want)
	}
}
