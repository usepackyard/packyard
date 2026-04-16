package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/usepackyard/packyard/internal/pid"
)

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// errorResponse is the stable wire envelope for every user-visible error
// from the admin/internal JSON API. `code` is a snake_case machine key
// that the frontend maps to a translated string; `message` is the
// English fallback for CLI consumers, logs, and locales missing a
// catalog entry. `error` is kept as an alias for `message` for
// backwards compatibility with external integrations that haven't
// migrated to reading `code`/`message` yet — can be removed once all
// consumers are on the new shape.
type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

// writeError emits a typed error envelope. `code` must be a stable
// snake_case identifier — see frontend/src/locales/en/errors.json for
// the catalog. `message` is the English prose; passing an empty string
// makes the frontend fall back to whatever the code resolves to.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Code: code, Message: message, Error: message})
}

// maxJSONBodySize caps request bodies for JSON admin/internal endpoints.
// Multipart uploads use their own limit via http.MaxBytesReader in the handler.
const maxJSONBodySize = 1 << 20 // 1 MB

func decodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) error {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodySize)
	return json.NewDecoder(r.Body).Decode(v)
}

func pathID(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}

// pathPublicID returns the prefixed-ULID path value for `name`, verifying
// that it carries the expected kind prefix (e.g. "pkg", "ver"). Returns
// the full "prefix_body" string on success; the caller hands that to a
// store lookup. Use this for every URL-facing id route param.
func pathPublicID(r *http.Request, name, prefix string) (string, error) {
	raw := r.PathValue(name)
	if _, err := pid.Parse(raw, prefix); err != nil {
		return "", err
	}
	return raw, nil
}

// isPublicIDError reports whether err came from pid.Parse (wrong prefix
// or malformed body). Callers use this to return 404 Not Found for a
// well-formed URL that doesn't match an existing kind.
func isPublicIDError(err error) bool {
	return errors.Is(err, pid.ErrWrongPrefix) || errors.Is(err, pid.ErrMalformed)
}
