package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/usepackyard/packyard/internal/provider/github"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/handler"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/provider"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/testutil"
)

type sourceCtx struct {
	org *model.Organization
	pkg *model.Package
}

func newSourceHandler(t *testing.T) (*handler.AdminSourceHandler, sourceCtx) {
	t.Helper()
	stores := testutil.NewStores(t)
	st, _ := storage.NewLocal(t.TempDir())
	c := composer.NewCache(stores.Packages, stores.Orgs, "http://test")
	cfg := &config.Config{BaseURL: "http://test"}
	h := handler.NewAdminSourceHandler(stores.Sources, stores.Packages, stores.Jobs, st, c, cfg)
	org := testutil.MakeOrg(t, stores, "default", "Default")
	pkg := testutil.MakePackage(t, stores, org.ID, "vendor/pkg")
	return h, sourceCtx{org, pkg}
}

func TestAdminSourceHandler_Set_NewSourceReturnsWebhookURLAndSecret(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)

	body := `{"provider":"github","repo_owner":"octo","repo_name":"hello","strategy":"release_asset","asset_pattern":"*.zip"}`
	req := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "webhook_url") {
		t.Errorf("response should include webhook_url: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "webhook_secret") {
		t.Errorf("new source should return webhook_secret once: %s", rec.Body.String())
	}
}

func TestAdminSourceHandler_Set_RequiresRepoFields(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)

	req := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(`{"provider":"github","repo_owner":"","repo_name":""}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminSourceHandler_Set_RejectsUnknownProvider(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)

	req := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(`{"provider":"bitbucket","repo_owner":"o","repo_name":"r"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminSourceHandler_Set_PackageNotFound(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)

	req := httptest.NewRequest("PUT", "/api/packages/9999/source",
		strings.NewReader(`{"repo_owner":"o","repo_name":"r"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminSourceHandler_Set_UpdateExisting(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)

	// First Set creates.
	req1 := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(`{"repo_owner":"old","repo_name":"r"}`))
	req1.Header.Set("Content-Type", "application/json")
	req1 = req1.WithContext(auth.SetOrgInContext(req1.Context(), ctx.org, nil))
	mux.ServeHTTP(httptest.NewRecorder(), req1)

	// Second Set updates.
	req2 := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(`{"repo_owner":"new","repo_name":"r"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2 = req2.WithContext(auth.SetOrgInContext(req2.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req2)

	if rec.Code != http.StatusOK {
		t.Fatalf("update Set status = %d; body=%s", rec.Code, rec.Body.String())
	}
	// Update path should NOT regenerate the webhook secret.
	if strings.Contains(rec.Body.String(), "webhook_secret") {
		t.Errorf("update should not return webhook_secret: %s", rec.Body.String())
	}
}

func TestAdminSourceHandler_Set_ValidatesStrategy(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)

	req := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(`{"provider":"github","repo_owner":"o","repo_name":"r","strategy":"clown"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAdminSourceHandler_Get_NotFound(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/packages/{id}/source", h.Get)

	req := httptest.NewRequest("GET", "/api/packages/"+ctx.pkg.PublicID+"/source", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminSourceHandler_Get_PackageInWrongOrg(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/packages/{id}/source", h.Get)

	// Use a totally different org slug (no member) — package not found.
	otherOrg := &model.Organization{ID: 9999}
	req := httptest.NewRequest("GET", "/api/packages/"+ctx.pkg.PublicID+"/source", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), otherOrg, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (cross-org isolation)", rec.Code)
	}
}

func TestAdminSourceHandler_GetAfterSet_ReturnsURL(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)
	mux.HandleFunc("GET /api/packages/{id}/source", h.Get)

	// Create.
	setReq := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(`{"repo_owner":"o","repo_name":"r"}`))
	setReq.Header.Set("Content-Type", "application/json")
	setReq = setReq.WithContext(auth.SetOrgInContext(setReq.Context(), ctx.org, nil))
	mux.ServeHTTP(httptest.NewRecorder(), setReq)

	// Read back.
	getReq := httptest.NewRequest("GET", "/api/packages/"+ctx.pkg.PublicID+"/source", nil)
	getReq = getReq.WithContext(auth.SetOrgInContext(getReq.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, getReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "webhook_url") {
		t.Errorf("response should include webhook_url: %s", rec.Body.String())
	}
	// webhook_secret must NOT be returned on Get (only once at create time).
	if strings.Contains(rec.Body.String(), "webhook_secret") {
		t.Errorf("Get should not leak webhook_secret: %s", rec.Body.String())
	}
}

func TestAdminSourceHandler_Sync_BadID(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/source/sync", h.Sync)

	// Malformed / wrong-prefix id returns 404 — same shape as a real
	// "no such package" response.
	req := httptest.NewRequest("POST", "/api/packages/not-a-pkg-id/source/sync", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminSourceHandler_Sync_PackageNotFound(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/source/sync", h.Sync)

	req := httptest.NewRequest("POST", "/api/packages/pkg_01JHZ8K3Y5WQ9V2N6TRB4XE7CM/source/sync", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAdminSourceHandler_Sync_SourceNotConfigured(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/packages/{id}/source/sync", h.Sync)

	// Package exists (created by newSourceHandler) but no source configured.
	req := httptest.NewRequest("POST", "/api/packages/"+ctx.pkg.PublicID+"/source/sync", nil)
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminSourceHandler_Delete(t *testing.T) {
	h, ctx := newSourceHandler(t)

	// Seed via the Set endpoint, then delete.
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)
	mux.HandleFunc("DELETE /api/packages/{id}/source", h.Delete)

	setReq := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(`{"repo_owner":"o","repo_name":"r"}`))
	setReq.Header.Set("Content-Type", "application/json")
	setReq = setReq.WithContext(auth.SetOrgInContext(setReq.Context(), ctx.org, nil))
	mux.ServeHTTP(httptest.NewRecorder(), setReq)

	delReq := httptest.NewRequest("DELETE", "/api/packages/"+ctx.pkg.PublicID+"/source", nil)
	delReq = delReq.WithContext(auth.SetOrgInContext(delReq.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, delReq)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
}

// stubProvider lets us register a fake provider just for preview tests
// without hitting real GitHub. Returns canned releases + assets.
type stubProvider struct {
	releases []provider.Release
	listErr  error
}

func (s *stubProvider) ListReleases(ctx context.Context, owner, repo string) ([]provider.Release, error) {
	return s.releases, s.listErr
}
func (s *stubProvider) DownloadAsset(ctx context.Context, url string) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}
func (s *stubProvider) DownloadSourceArchive(ctx context.Context, owner, repo, tag string) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}
func (s *stubProvider) ParseWebhook(body []byte) (*provider.WebhookEvent, error) {
	return nil, errors.New("not used")
}
func (s *stubProvider) ValidateWebhook(r *http.Request, secret string, body []byte) error {
	return errors.New("not used")
}

// registerStubProvider puts `stub` behind the given provider name for the
// test's lifetime; stores nothing outside the registry map and the test
// doesn't run in parallel, so mutation is safe here.
func registerStubProvider(t *testing.T, name string, stub *stubProvider) {
	t.Helper()
	provider.Register(name, func(token string) provider.Provider { return stub })
}

// Validation matrix for the new metadata_source / version_source / manual_require
// fields. These are orthogonal to the existing strategy/pattern checks so they get
// their own table-driven test; each row produces 400 with a specific rationale.
func TestAdminSourceHandler_Set_ValidatesMetadataAndVersionSources(t *testing.T) {
	h, ctx := newSourceHandler(t)

	cases := []struct {
		name string
		body string
	}{
		{"unknown metadata_source", `{"provider":"github","repo_owner":"o","repo_name":"r","metadata_source":"wat"}`},
		{"unknown version_source", `{"provider":"github","repo_owner":"o","repo_name":"r","version_source":"wat"}`},
		{"manual+source_archive rejected", `{"provider":"github","repo_owner":"o","repo_name":"r","strategy":"source_archive","metadata_source":"manual"}`},
		{"manual_require invalid JSON", `{"provider":"github","repo_owner":"o","repo_name":"r","metadata_source":"manual","manual_require":"not-json"}`},
		{"manual_require not an object", `{"provider":"github","repo_owner":"o","repo_name":"r","metadata_source":"manual","manual_require":"[1,2,3]"}`},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// Happy-path: manual metadata_source with a valid manual_require object
// succeeds. VersionSource is silently coerced to git_tag on the backend
// since composer.json isn't read in manual mode. We verify via the Get
// handler (which round-trips the same data the UI consumes).
func TestAdminSourceHandler_Set_ManualMetadata_CoercesVersionSource(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)
	mux.HandleFunc("GET /api/packages/{id}/source", h.Get)

	// Send metadata_source=manual with version_source=composer_json (nonsense
	// in manual mode) — backend must coerce to git_tag, not reject.
	body := `{"provider":"github","repo_owner":"o","repo_name":"r","strategy":"release_asset","metadata_source":"manual","version_source":"composer_json","manual_require":"{\"composer/installers\":\"^2.0\"}"}`
	setReq := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(body))
	setReq.Header.Set("Content-Type", "application/json")
	setReq = setReq.WithContext(auth.SetOrgInContext(setReq.Context(), ctx.org, nil))
	setRec := httptest.NewRecorder()
	mux.ServeHTTP(setRec, setReq)

	if setRec.Code != http.StatusCreated && setRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d; body=%s", setRec.Code, setRec.Body.String())
	}

	// Round-trip through GET to confirm persisted shape matches backend rules.
	getReq := httptest.NewRequest("GET", "/api/packages/"+ctx.pkg.PublicID+"/source", nil)
	getReq = getReq.WithContext(auth.SetOrgInContext(getReq.Context(), ctx.org, nil))
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d; body=%s", getRec.Code, getRec.Body.String())
	}

	var resp struct {
		Source struct {
			MetadataSource string `json:"metadata_source"`
			VersionSource  string `json:"version_source"`
			ManualRequire  string `json:"manual_require"`
		} `json:"source"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Source.MetadataSource != "manual" {
		t.Errorf("metadata_source = %q, want manual", resp.Source.MetadataSource)
	}
	if resp.Source.VersionSource != "git_tag" {
		t.Errorf("version_source = %q, want git_tag (coerced)", resp.Source.VersionSource)
	}
	if !strings.Contains(resp.Source.ManualRequire, "composer/installers") {
		t.Errorf("manual_require not persisted: %q", resp.Source.ManualRequire)
	}
}

// Upload provider: valid with only metadata+version configured; no
// repo fields needed. Mirrors what the frontend will send for the
// default package source created on `POST /packages`.
func TestAdminSourceHandler_Set_UploadProvider_HappyPath(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)

	body := `{"provider":"upload","metadata_source":"from_zip","version_source":"composer_json"}`
	req := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "webhook_url") {
		t.Errorf("upload provider should not emit webhook_url (response=%s)", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "webhook_secret") {
		t.Errorf("upload provider should not mint a webhook_secret (response=%s)", rec.Body.String())
	}
}

// Upload + version_source=git_tag is nonsense (no tags exist in the
// upload flow). Must be rejected outright.
func TestAdminSourceHandler_Set_UploadProvider_RejectsGitTagVersionSource(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)

	body := `{"provider":"upload","metadata_source":"from_zip","version_source":"git_tag"}`
	req := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// Upload + metadata=manual must coerce version_source to "manual" (the
// only meaningful value in that branch — the user types the version per
// upload). Mirrors GitHub's "manual metadata coerces to git_tag".
func TestAdminSourceHandler_Set_UploadProvider_ManualMetadataCoercesVersionSource(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/packages/{id}/source", h.Set)
	mux.HandleFunc("GET /api/packages/{id}/source", h.Get)

	// Intentionally send the wrong version_source; server must coerce.
	body := `{"provider":"upload","metadata_source":"manual","version_source":"composer_json","manual_require":"{\"composer/installers\":\"^2.0\"}"}`
	setReq := httptest.NewRequest("PUT", "/api/packages/"+ctx.pkg.PublicID+"/source",
		strings.NewReader(body))
	setReq.Header.Set("Content-Type", "application/json")
	setReq = setReq.WithContext(auth.SetOrgInContext(setReq.Context(), ctx.org, nil))
	setRec := httptest.NewRecorder()
	mux.ServeHTTP(setRec, setReq)
	if setRec.Code != http.StatusCreated {
		t.Fatalf("PUT status = %d; body=%s", setRec.Code, setRec.Body.String())
	}

	getReq := httptest.NewRequest("GET", "/api/packages/"+ctx.pkg.PublicID+"/source", nil)
	getReq = getReq.WithContext(auth.SetOrgInContext(getReq.Context(), ctx.org, nil))
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", getRec.Code)
	}

	var resp struct {
		Source struct {
			Provider       string `json:"provider"`
			MetadataSource string `json:"metadata_source"`
			VersionSource  string `json:"version_source"`
		} `json:"source"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Source.Provider != "upload" {
		t.Errorf("provider = %q, want upload", resp.Source.Provider)
	}
	if resp.Source.VersionSource != "manual" {
		t.Errorf("version_source = %q, want manual (coerced)", resp.Source.VersionSource)
	}
}

func TestAdminSourceHandler_PreviewReleases_HappyPath(t *testing.T) {
	h, ctx := newSourceHandler(t)

	registerStubProvider(t, "stub-preview", &stubProvider{
		releases: []provider.Release{
			{TagName: "v2.0.0-beta.47", Assets: []provider.Asset{
				{Name: "digital-license-manager-pro-v2.0.0-beta.47.zip", Size: 1024},
				{Name: "checksums.txt", Size: 256},
			}},
			{TagName: "v2.0.0-beta.46", Assets: []provider.Asset{
				{Name: "digital-license-manager-pro-v2.0.0-beta.46.zip", Size: 1024},
			}},
			{TagName: "v2.0.0-beta.45", Assets: []provider.Asset{
				{Name: "digital-license-manager-pro-v2.0.0-beta.45.zip", Size: 1024},
			}},
			// This fourth release must be trimmed — maxReleases = 3.
			{TagName: "v2.0.0-beta.44"},
		},
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sources/preview", h.PreviewReleases)

	req := httptest.NewRequest("POST", "/api/sources/preview",
		strings.NewReader(`{"provider":"stub-preview","owner":"o","repo":"r"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Releases []struct {
			Tag    string `json:"tag"`
			Assets []struct {
				Name string `json:"name"`
				Size int64  `json:"size"`
			} `json:"assets"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Releases) != 3 {
		t.Fatalf("len = %d, want 3 (capped)", len(resp.Releases))
	}
	if resp.Releases[0].Tag != "v2.0.0-beta.47" {
		t.Errorf("first tag = %q", resp.Releases[0].Tag)
	}
	if len(resp.Releases[0].Assets) != 2 {
		t.Errorf("first release assets len = %d, want 2", len(resp.Releases[0].Assets))
	}
	if resp.Releases[0].Assets[0].Name != "digital-license-manager-pro-v2.0.0-beta.47.zip" {
		t.Errorf("first asset name = %q", resp.Releases[0].Assets[0].Name)
	}
}

func TestAdminSourceHandler_PreviewReleases_RequiresOwnerAndRepo(t *testing.T) {
	h, ctx := newSourceHandler(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sources/preview", h.PreviewReleases)

	// Each body is missing owner, repo, or both. All should 400.
	for _, body := range []string{`{}`, `{"owner":"o"}`, `{"repo":"r"}`} {
		req := httptest.NewRequest("POST", "/api/sources/preview", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestAdminSourceHandler_PreviewReleases_UpstreamFailure(t *testing.T) {
	h, ctx := newSourceHandler(t)

	registerStubProvider(t, "stub-preview-fail", &stubProvider{
		listErr: errors.New("GitHub says 404"),
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sources/preview", h.PreviewReleases)

	req := httptest.NewRequest("POST", "/api/sources/preview",
		strings.NewReader(`{"provider":"stub-preview-fail","owner":"o","repo":"r"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}

// Per-call auth_token overrides the org-configured provider token. Verify by
// registering a stub whose factory records the token it was built with and
// asserting the request-scoped value was passed through.
func TestAdminSourceHandler_PreviewReleases_UsesAuthTokenFromBody(t *testing.T) {
	h, ctx := newSourceHandler(t)

	var capturedToken string
	provider.Register("stub-preview-token", func(token string) provider.Provider {
		capturedToken = token
		return &stubProvider{releases: []provider.Release{}}
	})

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sources/preview", h.PreviewReleases)

	req := httptest.NewRequest("POST", "/api/sources/preview",
		strings.NewReader(`{"provider":"stub-preview-token","owner":"o","repo":"r","auth_token":"ghp_adhoc_token"}`))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.SetOrgInContext(req.Context(), ctx.org, nil))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if capturedToken != "ghp_adhoc_token" {
		t.Errorf("provider built with token = %q, want ghp_adhoc_token", capturedToken)
	}
}

