package handler_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestErrorCodeRegistry guards the contract between backend error codes
// (emitted by writeError) and the frontend translation catalog at
// frontend/src/locales/en/errors.json. Every code used in Go must exist
// in the catalog so the UI can translate it; every code in the catalog
// must be used somewhere (unused keys rot silently). Catches drift
// introduced by refactors that rename/add/remove codes on one side only.
//
// Mechanism: AST walk over the handler package, pull the third argument
// of every `writeError(...)` call literal. Not a lint rule — a real test
// so it runs in CI and blocks merge on drift.
func TestErrorCodeRegistry(t *testing.T) {
	t.Helper()

	usedCodes := collectWriteErrorCodes(t)
	catalogCodes := loadCatalogCodes(t)

	var missingInCatalog, extraInCatalog []string
	for code := range usedCodes {
		if !catalogCodes[code] {
			missingInCatalog = append(missingInCatalog, code)
		}
	}
	// Codes that exist in the catalog without a matching writeError site
	// are dead entries — EXCEPT for keys the frontend uses as fallbacks
	// (e.g. "generic" is the final fallback when an unknown code arrives
	// from the server). Keep this list short; the goal is to catch
	// genuine drift, not whitelist every future ad-hoc entry.
	frontendOnly := map[string]bool{
		"generic": true,
	}
	for code := range catalogCodes {
		if !usedCodes[code] && !frontendOnly[code] {
			extraInCatalog = append(extraInCatalog, code)
		}
	}
	sort.Strings(missingInCatalog)
	sort.Strings(extraInCatalog)

	if len(missingInCatalog) > 0 {
		t.Errorf("codes used in handlers but missing from frontend/src/locales/en/errors.json:\n  %s",
			strings.Join(missingInCatalog, "\n  "))
	}
	if len(extraInCatalog) > 0 {
		t.Errorf("codes in errors.json catalog that no writeError call uses (dead entries):\n  %s",
			strings.Join(extraInCatalog, "\n  "))
	}
}

// collectWriteErrorCodes walks every .go file in internal/handler/ and
// pulls the literal code argument from each writeError(...) call. We
// ignore calls whose code argument isn't a plain string literal — those
// are either test fixtures or dynamic codes we can't check statically.
func collectWriteErrorCodes(t *testing.T) map[string]bool {
	t.Helper()
	codes := make(map[string]bool)
	fset := token.NewFileSet()
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(src, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok || name.Name != "writeError" {
				return true
			}
			// writeError(w, status, code, message) — code is args[2].
			if len(call.Args) < 3 {
				return true
			}
			lit, ok := call.Args[2].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			code, err := strconvUnquote(lit.Value)
			if err != nil {
				return true
			}
			codes[code] = true
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return codes
}

func strconvUnquote(s string) (string, error) {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1], nil
	}
	return s, nil
}

// loadCatalogCodes reads the English error catalog. The path is
// relative to this test file (internal/handler/), climbing up to the
// repo root then into the frontend. Uses the en catalog as the
// authoritative key set — other locales are free to lag (missing keys
// fall back to en at runtime).
func loadCatalogCodes(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "frontend", "src", "locales", "en", "errors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make(map[string]bool, len(raw))
	for k := range raw {
		out[k] = true
	}
	return out
}
