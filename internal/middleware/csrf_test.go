package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireCSRFHeader(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		headerSet  bool
		wantStatus int
	}{
		{"GET passes without header", "GET", false, http.StatusOK},
		{"HEAD passes without header", "HEAD", false, http.StatusOK},
		{"OPTIONS passes without header", "OPTIONS", false, http.StatusOK},
		{"POST without header rejected", "POST", false, http.StatusForbidden},
		{"PUT without header rejected", "PUT", false, http.StatusForbidden},
		{"DELETE without header rejected", "DELETE", false, http.StatusForbidden},
		{"PATCH without header rejected", "PATCH", false, http.StatusForbidden},
		{"POST with header passes", "POST", true, http.StatusOK},
	}

	mw := RequireCSRFHeader(ok())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/anything", nil)
			if tt.headerSet {
				req.Header.Set("X-Requested-With", "XMLHttpRequest")
			}
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
