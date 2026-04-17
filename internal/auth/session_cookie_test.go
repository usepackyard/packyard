package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usepackyard/packyard/internal/auth"
	"github.com/usepackyard/packyard/internal/testutil"
)

// setCookieHeader grabs the raw Set-Cookie header the handler emitted.
// We check the raw header rather than the parsed *http.Cookie because
// Go's parser drops the leading dot on Domain and doesn't populate
// SameSite at all — but what matters for real browsers is the wire
// representation.
func setCookieHeader(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	h := rec.Result().Header.Get("Set-Cookie")
	if h == "" {
		t.Fatal("no Set-Cookie header emitted")
	}
	return h
}

// CreateSession must emit Domain + SameSite exactly as CookieOptions
// specifies so deployments that want a parent-domain, shared-login
// cookie can configure it without forking the cookie-writing code.
func TestCreateSession_CookieAttributes(t *testing.T) {
	stores := testutil.NewStores(t)
	user := testutil.MakeUser(t, stores, "owner@example.com", "password")

	rec := httptest.NewRecorder()
	err := auth.CreateSession(rec, stores.Sessions, strings.Repeat("a", 32), user.ID, 3600, auth.CookieOptions{
		Secure:   true,
		Domain:   ".packyard.test",
		SameSite: http.SameSiteLaxMode,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	header := setCookieHeader(t, rec)
	for _, want := range []string{
		"packyard_session=",
		"Domain=packyard.test", // browsers treat ".packyard.test" and "packyard.test" identically; Go serialises without the leading dot
		"SameSite=Lax",
		"Secure",
		"HttpOnly",
		"Path=/",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("Set-Cookie missing %q\nfull header: %s", want, header)
		}
	}
}

// The zero-value CookieOptions should reproduce the historical
// behaviour: Strict SameSite, no Domain. This guarantees callers that
// forget to populate the struct don't accidentally emit a Default-mode
// cookie (which browsers treat unpredictably).
func TestCreateSession_ZeroOptions_DefaultsToStrictNoDomain(t *testing.T) {
	stores := testutil.NewStores(t)
	user := testutil.MakeUser(t, stores, "owner@example.com", "password")

	rec := httptest.NewRecorder()
	if err := auth.CreateSession(rec, stores.Sessions, strings.Repeat("a", 32), user.ID, 3600, auth.CookieOptions{}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	header := setCookieHeader(t, rec)
	if strings.Contains(header, "Domain=") {
		t.Errorf("zero CookieOptions should not emit Domain=; got: %s", header)
	}
	if !strings.Contains(header, "SameSite=Strict") {
		t.Errorf("zero CookieOptions should default to SameSite=Strict; got: %s", header)
	}
}

// ClearSession's Set-Cookie must mirror Domain from CookieOptions,
// otherwise the browser keeps the original cookie (two cookies with
// different Domains coexist) and the user appears still-logged-in.
func TestClearSession_MatchesDomain(t *testing.T) {
	stores := testutil.NewStores(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/logout", nil)
	auth.ClearSession(rec, req, stores.Sessions, strings.Repeat("a", 32), auth.CookieOptions{
		Secure:   true,
		Domain:   ".packyard.test",
		SameSite: http.SameSiteLaxMode,
	})
	header := setCookieHeader(t, rec)
	if !strings.Contains(header, "Domain=packyard.test") {
		t.Errorf("Clear must emit Domain= matching Create (else browser won't evict). Got: %s", header)
	}
	if !strings.Contains(header, "Max-Age=0") && !strings.Contains(header, "Expires=") {
		t.Errorf("Clear must emit Max-Age=0 or Expires in the past. Got: %s", header)
	}
}
