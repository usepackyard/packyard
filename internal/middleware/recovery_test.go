package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func panicHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		panic("boom")
	})
}

func TestRecovery_PanicReturns500(t *testing.T) {
	// Restore the default logger after the test.
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	req := httptest.NewRequest("GET", "/boom", nil)
	rec := httptest.NewRecorder()
	Recovery(panicHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	logged := buf.String()
	if !strings.Contains(logged, "panic recovered") {
		t.Errorf("log should mention 'panic recovered': %s", logged)
	}
	if strings.Contains(logged, "stack=") || strings.Contains(logged, "goroutine ") {
		t.Errorf("stack trace should NOT be logged at INFO level: %s", logged)
	}
}

func TestRecovery_StackLoggedAtDebug(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	req := httptest.NewRequest("GET", "/boom", nil)
	rec := httptest.NewRecorder()
	Recovery(panicHandler()).ServeHTTP(rec, req)

	logged := buf.String()
	if !strings.Contains(logged, "stack=") {
		t.Errorf("stack trace should be logged at DEBUG level: %s", logged)
	}
}
