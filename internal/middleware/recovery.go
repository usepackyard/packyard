package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				attrs := []any{"error", err, "path", r.URL.Path}
				// Stack traces leak code paths and file locations — only
				// emit them when debug logging is explicitly enabled.
				if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
					attrs = append(attrs, "stack", string(debug.Stack()))
				}
				slog.Error("panic recovered", attrs...)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
