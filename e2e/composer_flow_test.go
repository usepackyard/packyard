// Package e2e contains end-to-end tests that drive the real HTTP server
// through its full middleware chain. These tests are coarser and slower
// than the unit tests in internal/*; they catch integration bugs that
// unit tests miss (routing, middleware ordering, wire format).
package e2e

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/composer"
	"github.com/usepackyard/packyard/internal/config"
	"github.com/usepackyard/packyard/internal/model"
	"github.com/usepackyard/packyard/internal/server"
	"github.com/usepackyard/packyard/internal/storage"
	"github.com/usepackyard/packyard/internal/store"
	"github.com/usepackyard/packyard/internal/testutil"
)

const sessionSecret = "dev-session-secret-at-least-32chars!"

type fixture struct {
	baseURL    string
	stores     *store.Stores
	adminToken string // plaintext Bearer admin token
}

// start boots a full Packyard server in multi mode on a random port,
// seeds a super-admin user, mints an admin Bearer token, and returns
// the fixture for tests.
func start(t *testing.T) *fixture {
	t.Helper()

	stores := testutil.NewStores(t)
	strg, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}

	cfg := &config.Config{
		Mode:       "multi",
		BaseURL:    "", // set after server starts
		BcryptCost: 4, // fast for tests
		Session:    config.SessionConfig{Secret: sessionSecret, MaxAge: 3600},
		Storage:    config.StorageConfig{Type: "local"},
	}
	cache := composer.NewCache(stores.Packages, stores.Orgs, cfg.BaseURL, cfg.Mode)
	mux := server.NewMux(cfg, stores, strg, cache, nil)
	ts := httptest.NewServer(server.Wrap(cfg, mux))
	t.Cleanup(ts.Close)

	// Now that we have a URL, rebuild with the real BaseURL embedded in
	// Composer dist URLs.
	cfg.BaseURL = ts.URL
	cache2 := composer.NewCache(stores.Packages, stores.Orgs, cfg.BaseURL, cfg.Mode)
	ts.Config.Handler = server.Wrap(cfg, server.NewMux(cfg, stores, strg, cache2, nil))

	// Seed a super-admin user (mirrors what cmd/server/main.go does on first run).
	hash, err := auth.HashPassword("admin-password", 4)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	admin := &model.User{
		Email:        "super@packyard",
		Password:     hash,
		Name:         "Super",
		IsActive:     true,
		IsSuperAdmin: true,
	}
	if err := stores.Users.Create(context.Background(), admin); err != nil {
		t.Fatalf("seed super-admin: %v", err)
	}

	// Mint an admin Bearer token directly via the store (skips the UI flow).
	// The plaintext is what an external service would store in env.
	plaintext := "adm_e2e0000000000000000000000000000000000000000000000000000000000"
	hashed := sha256Hex(plaintext)
	tok := &model.AdminToken{
		Name:        "e2e",
		TokenHash:   hashed,
		TokenPrefix: plaintext[:12],
		CreatedBy:   admin.ID,
		IsActive:    true,
	}
	if err := stores.AdminTokens.Create(context.Background(), tok); err != nil {
		t.Fatalf("seed admin token: %v", err)
	}

	return &fixture{baseURL: ts.URL, stores: stores, adminToken: plaintext}
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestE2E_FullComposerFlow(t *testing.T) {
	f := start(t)

	// --- 1. Create an org via /api/admin/orgs using the admin token ---
	adminPostJSON(t, f.adminToken, f.baseURL+"/api/admin/orgs",
		`{"slug":"acme","name":"Acme"}`, http.StatusCreated)

	// --- 2. Add an owner to that org via /api/admin/orgs/{slug}/members ---
	adminPostJSON(t, f.adminToken, f.baseURL+"/api/admin/orgs/acme/members",
		`{"email":"owner@acme","password":"owner-pass","name":"Owner","role":"owner"}`,
		http.StatusCreated)

	// --- 3. Owner logs in (session cookie) ---
	loginResp, loginBody := doJSON(t, "POST", f.baseURL+"/api/auth/login",
		map[string]string{"email": "owner@acme", "password": "owner-pass"}, nil)
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login: status=%d body=%s", loginResp.StatusCode, loginBody)
	}
	var session *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == "packyard_session" {
			session = c
			break
		}
	}
	if session == nil {
		t.Fatal("login did not set packyard_session cookie")
	}

	// --- 4. Create a package in the org ---
	createResp, createBody := doJSON(t, "POST", f.baseURL+"/api/orgs/acme/packages",
		map[string]string{"name": "e2e/hello", "type": "library"}, session)
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create package: status=%d body=%s", createResp.StatusCode, createBody)
	}
	var pkgResp struct {
		Package struct {
			ID string `json:"id"`
		} `json:"package"`
	}
	if err := json.Unmarshal(createBody, &pkgResp); err != nil {
		t.Fatalf("decode package: %v", err)
	}

	// --- 5. Upload a version zip ---
	zipBytes := makeZip(t, `{"name":"e2e/hello","version":"1.0.0"}`)
	var multipartBody bytes.Buffer
	mw := multipart.NewWriter(&multipartBody)
	fw, _ := mw.CreateFormFile("file", "hello.zip")
	fw.Write(zipBytes)
	mw.Close()

	uploadURL := f.baseURL + "/api/orgs/acme/packages/" + pkgResp.Package.ID + "/versions"
	req, _ := http.NewRequest("POST", uploadURL, &multipartBody)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.AddCookie(session)
	uploadResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	uploadBody, _ := io.ReadAll(uploadResp.Body)
	uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("upload version: status=%d body=%s", uploadResp.StatusCode, uploadBody)
	}

	// --- 6. Mint a Composer client token ---
	tokResp, tokBody := doJSON(t, "POST", f.baseURL+"/api/orgs/acme/tokens",
		map[string]string{"name": "CI"}, session)
	if tokResp.StatusCode != http.StatusCreated {
		t.Fatalf("create token: status=%d body=%s", tokResp.StatusCode, tokBody)
	}
	if tokResp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("token create should set Cache-Control: no-store, got %q", tokResp.Header.Get("Cache-Control"))
	}
	var tok struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(tokBody, &tok); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if tok.Password == "" {
		t.Fatal("token creation should return a generated password")
	}

	// --- 7. Composer endpoints work while org is active ---
	assertComposerOK(t, f.baseURL, "acme", tok.Token, tok.Password, "e2e/hello", zipBytes)

	// --- 7a. The dist fetch just done must have been recorded ---
	// The counter + event write is fire-and-forget from Dist, so poll briefly.
	t.Run("download is recorded in stats", func(t *testing.T) {
		statsURL := f.baseURL + "/api/orgs/acme/packages/stats"
		var resp statsResponseE2E
		testutil.Eventually(t, 2*time.Second, "total_downloads should reach 1", func() bool {
			resp = fetchStats(t, statsURL, session)
			return resp.TotalDownloads >= 1
		})
		if len(resp.TopPackages) != 1 || resp.TopPackages[0].PackageName != "e2e/hello" {
			t.Errorf("top_packages = %+v, want [{e2e/hello, …}]", resp.TopPackages)
		}
		if len(resp.RecentDownloads) == 0 {
			t.Fatal("recent_downloads empty")
		}
		if resp.RecentDownloads[0].Version != "1.0.0" {
			t.Errorf("recent[0].Version = %q, want 1.0.0", resp.RecentDownloads[0].Version)
		}
	})

	// --- 8. Lifecycle: suspend → 402 → reactivate → 200 → archive → 404 ---
	t.Run("suspend returns 402", func(t *testing.T) {
		adminPutJSON(t, f.adminToken, f.baseURL+"/api/admin/orgs/acme/status",
			`{"status":"suspended"}`, http.StatusOK)

		assertComposerStatus(t, f.baseURL, "acme", tok.Token, tok.Password, http.StatusPaymentRequired)
	})

	t.Run("reactivate restores access", func(t *testing.T) {
		adminPutJSON(t, f.adminToken, f.baseURL+"/api/admin/orgs/acme/status",
			`{"status":"active"}`, http.StatusOK)

		assertComposerStatus(t, f.baseURL, "acme", tok.Token, tok.Password, http.StatusOK)
	})

	t.Run("archive returns 404", func(t *testing.T) {
		adminPutJSON(t, f.adminToken, f.baseURL+"/api/admin/orgs/acme/status",
			`{"status":"archived"}`, http.StatusOK)

		assertComposerStatus(t, f.baseURL, "acme", tok.Token, tok.Password, http.StatusNotFound)
	})

	// --- 9. Negative tests ---
	t.Run("admin endpoint rejects bad token", func(t *testing.T) {
		req, _ := http.NewRequest("GET", f.baseURL+"/api/admin/orgs", nil)
		req.Header.Set("Authorization", "Bearer bogus-token")
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("bad bearer: status=%d, want 401", resp.StatusCode)
		}
	})

	t.Run("admin endpoint rejects missing auth", func(t *testing.T) {
		req, _ := http.NewRequest("GET", f.baseURL+"/api/admin/orgs", nil)
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("no auth: status=%d, want 401", resp.StatusCode)
		}
	})

	t.Run("CSRF defense on admin POST", func(t *testing.T) {
		req, _ := http.NewRequest("POST", f.baseURL+"/api/admin/orgs",
			strings.NewReader(`{"slug":"x","name":"X"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+f.adminToken)
		// Deliberately omit X-Requested-With.
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("admin POST without X-Requested-With: status=%d, want 403", resp.StatusCode)
		}
	})

	t.Run("cross-tenant token misuse blocked", func(t *testing.T) {
		// First reactivate acme so its token is otherwise valid.
		adminPutJSON(t, f.adminToken, f.baseURL+"/api/admin/orgs/acme/status",
			`{"status":"active"}`, http.StatusOK)
		// Provision a second org.
		adminPostJSON(t, f.adminToken, f.baseURL+"/api/admin/orgs",
			`{"slug":"beta","name":"Beta"}`, http.StatusCreated)

		// Use the acme token against /beta/... — must reject with 401.
		req, _ := http.NewRequest("GET", f.baseURL+"/beta/packages.json", nil)
		req.SetBasicAuth(tok.Token, tok.Password)
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("cross-tenant token: status=%d, want 401", resp.StatusCode)
		}
	})

	t.Run("security headers", func(t *testing.T) {
		resp, _ := http.DefaultClient.Get(f.baseURL + "/healthz")
		resp.Body.Close()
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("missing X-Content-Type-Options: nosniff")
		}
		if resp.Header.Get("X-Frame-Options") != "DENY" {
			t.Errorf("missing X-Frame-Options: DENY")
		}
	})
}

// ---------- helpers ----------

func doJSON(t *testing.T, method, url string, body any, cookie *http.Cookie) (*http.Response, []byte) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if cookie != nil {
		req.AddCookie(cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func adminPostJSON(t *testing.T, token, url, body string, wantStatus int) {
	t.Helper()
	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("admin POST %s: status=%d, want %d; body=%s", url, resp.StatusCode, wantStatus, b)
	}
}

func adminPutJSON(t *testing.T, token, url, body string, wantStatus int) {
	t.Helper()
	req, err := http.NewRequest("PUT", url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin PUT %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("admin PUT %s: status=%d, want %d; body=%s", url, resp.StatusCode, wantStatus, b)
	}
}

func makeZip(t *testing.T, composerJSON string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("composer.json")
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	w.Write([]byte(composerJSON))
	if err := zw.Close(); err != nil {
		t.Fatalf("zip.Close: %v", err)
	}
	return buf.Bytes()
}

// assertComposerOK walks /{slug}/packages.json, /{slug}/p2/{name}.json and
// /{slug}/dist/{name}/{ver} for an active org and verifies all three
// respond 200 with sane content. Multi-mode URL shape (Phase C).
func assertComposerOK(t *testing.T, base, slug, token, password, pkgName string, expectedZip []byte) {
	t.Helper()

	// /{slug}/packages.json.
	pkgsReq, _ := http.NewRequest("GET", base+"/"+slug+"/packages.json", nil)
	pkgsReq.SetBasicAuth(token, password)
	pkgsResp, _ := http.DefaultClient.Do(pkgsReq)
	pkgsData, _ := io.ReadAll(pkgsResp.Body)
	pkgsResp.Body.Close()
	if pkgsResp.StatusCode != http.StatusOK {
		t.Fatalf("packages.json: status=%d body=%s", pkgsResp.StatusCode, pkgsData)
	}
	if !strings.Contains(string(pkgsData), pkgName) {
		t.Errorf("packages.json should list %s: %s", pkgName, pkgsData)
	}
	// metadata-url must be tenant-prefixed (regression for Phase C URL shape).
	wantMetaURL := `"metadata-url":"/` + slug + `/p2/%package%.json"`
	if !strings.Contains(string(pkgsData), wantMetaURL) {
		t.Errorf("packages.json should embed slug-prefixed metadata-url; got: %s", pkgsData)
	}

	// /{slug}/p2/{name}.json.
	provReq, _ := http.NewRequest("GET", base+"/"+slug+"/p2/"+pkgName+".json", nil)
	provReq.SetBasicAuth(token, password)
	provResp, _ := http.DefaultClient.Do(provReq)
	provData, _ := io.ReadAll(provResp.Body)
	provResp.Body.Close()
	if provResp.StatusCode != http.StatusOK {
		t.Fatalf("p2: status=%d body=%s", provResp.StatusCode, provData)
	}
	// dist URL inside provider JSON must be tenant-prefixed.
	wantDistFragment := "/" + slug + "/dist/" + pkgName + "/1.0.0"
	if !strings.Contains(string(provData), wantDistFragment) {
		t.Errorf("provider JSON should embed slug-prefixed dist URL; got: %s", provData)
	}

	// /{slug}/dist/{name}/{ver}.
	distReq, _ := http.NewRequest("GET", base+"/"+slug+"/dist/"+pkgName+"/1.0.0", nil)
	distReq.SetBasicAuth(token, password)
	distResp, _ := http.DefaultClient.Do(distReq)
	distData, _ := io.ReadAll(distResp.Body)
	distResp.Body.Close()
	if distResp.StatusCode != http.StatusOK {
		t.Fatalf("dist: status=%d body=%s", distResp.StatusCode, distData)
	}
	if !bytes.Equal(distData, expectedZip) {
		t.Error("dist bytes don't match uploaded zip")
	}
}

// assertComposerStatus checks that /{slug}/packages.json returns the
// given status. Used to verify suspended (402) and archived (404)
// lifecycle behavior on the multi-mode tenant URLs.
func assertComposerStatus(t *testing.T, base, slug, token, password string, want int) {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/"+slug+"/packages.json", nil)
	req.SetBasicAuth(token, password)
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Errorf("packages.json: status=%d, want %d", resp.StatusCode, want)
	}
}

// statsResponseE2E mirrors the /packages/stats handler shape.
type statsResponseE2E struct {
	TotalDownloads   int64 `json:"total_downloads"`
	DownloadsLast7d  int64 `json:"downloads_last_7d"`
	DownloadsLast30d int64 `json:"downloads_last_30d"`
	TopPackages      []struct {
		PackageID   string `json:"package_id"`
		PackageName string `json:"package_name"`
		Count       int64  `json:"count"`
	} `json:"top_packages"`
	RecentDownloads []struct {
		PackageName string `json:"package_name"`
		Version     string `json:"version"`
	} `json:"recent_downloads"`
}

// fetchStats GETs /packages/stats as the authenticated session user and
// decodes the response.
func fetchStats(t *testing.T, url string, session *http.Cookie) statsResponseE2E {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.AddCookie(session)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stats GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("stats: status=%d body=%s", resp.StatusCode, b)
	}
	var out statsResponseE2E
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("stats decode: %v", err)
	}
	return out
}

// itoa is stdlib-free to keep imports minimal.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

