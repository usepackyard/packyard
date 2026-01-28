package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders_AlwaysSet(t *testing.T) {
	mw := SecurityHeaders("http://localhost:9090")(ok())

	req := httptest.NewRequest("GET", "/anything", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if hsts := rec.Header().Get("Strict-Transport-Security"); hsts != "" {
		t.Errorf("HSTS should not be set on plain-HTTP baseURL, got %q", hsts)
	}
}

func TestSecurityHeaders_HSTSWhenHTTPS(t *testing.T) {
	mw := SecurityHeaders("https://repo.example.com")(ok())

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	hsts := rec.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Fatal("HSTS should be set when baseURL is https")
	}
}
