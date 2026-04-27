package handler_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/usepackyard/packyard/internal/provider/github" // register provider

	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/pid"
	"github.com/usepackyard/packyard/internal/store"
	"github.com/usepackyard/packyard/internal/testutil"
)

func githubSig(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

const releasePayload = `{
  "action": "published",
  "release": {"tag_name": "1.0.0", "draft": false},
  "repository": {"owner": {"login": "octo"}, "name": "hello"}
}`

const draftPayload = `{
  "action": "published",
  "release": {"tag_name": "1.0.0", "draft": true},
  "repository": {"owner": {"login": "octo"}, "name": "hello"}
}`

func newWebhookHandlerSimple(t *testing.T) (*handler.WebhookHandler, *store.Stores) {
	t.Helper()
	stores := testutil.NewStores(t)
	cfg := &config.Config{}
	h := handler.NewWebhookHandler(stores.Sources, stores.Packages, stores.Jobs, cfg)
	return h, stores
}

func TestWebhookHandler_UnsupportedProvider(t *testing.T) {
	h, _ := newWebhookHandlerSimple(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{provider}/{source_id}", h.Handle)

	req := httptest.NewRequest("POST", "/hooks/bitbucket/"+pid.New(pid.PackageSource), strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestWebhookHandler_DraftRelease_Ignored(t *testing.T) {
	h, helpers := newWebhookHandlerSimple(t)

	org := testutil.MakeOrg(t, helpers, "default", "Default")
	pkg := testutil.MakePackage(t, helpers, org.ID, "octo/hello")
	src := &model.PackageSource{
		PackageID:      pkg.ID,
		Provider:       "github",
		ProviderConfig: testutil.SourceConfigJSON(t, "octo", "hello", "release_asset", "*.zip"),
		RepoKey:        "octo/hello",
		WebhookSecret:  "shhh",
	}
	if err := helpers.Sources.Create(context.Background(), src); err != nil {
		t.Fatalf("create source: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{provider}/{source_id}", h.Handle)

	req := httptest.NewRequest("POST", "/hooks/github/"+src.PublicID, strings.NewReader(draftPayload))
	req.Header.Set("X-Hub-Signature-256", githubSig("shhh", draftPayload))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ignored"`) {
		t.Errorf("expected 'ignored' status, got: %s", rec.Body.String())
	}
}

func TestWebhookHandler_NoSourceConfigured(t *testing.T) {
	h, _ := newWebhookHandlerSimple(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{provider}/{source_id}", h.Handle)

	req := httptest.NewRequest("POST", "/hooks/github/"+pid.New(pid.PackageSource), strings.NewReader(releasePayload))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestWebhookHandler_FailsClosed_NoSecret(t *testing.T) {
	// A source row without a WebhookSecret must fail closed, not be silently
	// accepted.
	h, helpers := newWebhookHandlerSimple(t)

	org := testutil.MakeOrg(t, helpers, "default", "Default")
	pkg := testutil.MakePackage(t, helpers, org.ID, "octo/hello")
	src := &model.PackageSource{
		PackageID:      pkg.ID,
		Provider:       "github",
		ProviderConfig: testutil.SourceConfigJSON(t, "octo", "hello", "release_asset", "*.zip"),
		RepoKey:        "octo/hello",
		// WebhookSecret intentionally empty
	}
	if err := helpers.Sources.Create(context.Background(), src); err != nil {
		t.Fatalf("create source: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{provider}/{source_id}", h.Handle)

	req := httptest.NewRequest("POST", "/hooks/github/"+src.PublicID, strings.NewReader(releasePayload))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (fail-closed); body=%s", rec.Code, rec.Body.String())
	}
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	h, helpers := newWebhookHandlerSimple(t)

	org := testutil.MakeOrg(t, helpers, "default", "Default")
	pkg := testutil.MakePackage(t, helpers, org.ID, "octo/hello")
	src := &model.PackageSource{
		PackageID:      pkg.ID,
		Provider:       "github",
		ProviderConfig: testutil.SourceConfigJSON(t, "octo", "hello", "release_asset", "*.zip"),
		RepoKey:        "octo/hello",
		WebhookSecret:  "shhh",
	}
	if err := helpers.Sources.Create(context.Background(), src); err != nil {
		t.Fatalf("create source: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{provider}/{source_id}", h.Handle)

	req := httptest.NewRequest("POST", "/hooks/github/"+src.PublicID, strings.NewReader(releasePayload))
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWebhookHandler_ValidSignatureSyncs(t *testing.T) {
	h, helpers := newWebhookHandlerSimple(t)

	org := testutil.MakeOrg(t, helpers, "default", "Default")
	pkg := testutil.MakePackage(t, helpers, org.ID, "octo/hello")
	src := &model.PackageSource{
		PackageID:      pkg.ID,
		Provider:       "github",
		ProviderConfig: testutil.SourceConfigJSON(t, "octo", "hello", "release_asset", "*.zip"),
		RepoKey:        "octo/hello",
		WebhookSecret:  "shhh",
	}
	if err := helpers.Sources.Create(context.Background(), src); err != nil {
		t.Fatalf("create source: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{provider}/{source_id}", h.Handle)

	req := httptest.NewRequest("POST", "/hooks/github/"+src.PublicID, strings.NewReader(releasePayload))
	req.Header.Set("X-Hub-Signature-256", githubSig("shhh", releasePayload))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Webhook enqueues a sync job and returns {"status":"queued"} — the
	// worker pool runs the sync asynchronously. We just confirm the
	// enqueue happened.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Errorf("expected queued status, got: %s", rec.Body.String())
	}
}
