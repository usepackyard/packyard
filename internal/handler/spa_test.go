package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/usepackyard/packyard/internal/handler"
)

func TestSPAHandler_ServesExistingFile(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":          {Data: []byte("<!doctype html><body>app</body>")},
		"assets/main.js":      {Data: []byte("console.log('hi')")},
		"favicon.svg":         {Data: []byte("<svg/>")},
	}
	h := handler.SPAHandler(fsys)

	req := httptest.NewRequest("GET", "/assets/main.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "console.log") {
		t.Errorf("body = %s, want main.js content", rec.Body.String())
	}
}

func TestSPAHandler_FallsBackToIndexForSPARoutes(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("INDEX_BODY_MARKER")},
	}
	h := handler.SPAHandler(fsys)

	// /packages/123 doesn't exist as a file — should serve index.html.
	req := httptest.NewRequest("GET", "/packages/123", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "INDEX_BODY_MARKER") {
		t.Errorf("body should be index.html for SPA routing: %s", rec.Body.String())
	}
}

func TestSPAHandler_RootServesIndex(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html": {Data: []byte("ROOT_INDEX")},
	}
	h := handler.SPAHandler(fsys)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ROOT_INDEX") {
		t.Errorf("body = %s", rec.Body.String())
	}
}
