package composer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SynthesizeInput is the data a caller must supply to build a valid
// composer.json from package metadata — used when the underlying zip
// doesn't ship one (WordPress plugin distributions, custom manual
// uploads). All non-Require fields are forwarded verbatim; Require is
// emitted only when non-empty so the stored JSON stays readable.
type SynthesizeInput struct {
	Name        string
	Type        string
	Description string
	Homepage    string
	Version     string
	Require     map[string]string
}

// Synthesize builds a ComposerJSON (struct + serialised RawJSON) from
// the given inputs. It's the single source of truth for "what does a
// synthesized composer.json look like" — both the GitHub sync pipeline
// (`internal/provider/sync.go` when metadata_source=manual) and the
// upload handler (when the same metadata mode is configured on an
// upload-provider source) funnel through here. If you change the
// shape, both call sites pick it up.
func Synthesize(input SynthesizeInput) (*ComposerJSON, error) {
	cj := &ComposerJSON{
		Name:        input.Name,
		Version:     input.Version,
		Type:        input.Type,
		Description: input.Description,
		Homepage:    input.Homepage,
	}
	if len(input.Require) > 0 {
		cj.Require = input.Require
	}
	raw, err := json.Marshal(synthesizedComposerJSON{
		Name:        cj.Name,
		Version:     cj.Version,
		Type:        cj.Type,
		Description: cj.Description,
		Homepage:    cj.Homepage,
		Require:     cj.Require,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal synthesized composer.json: %w", err)
	}
	cj.RawJSON = string(raw)
	return cj, nil
}

// ParseRequireJSON decodes a `require` block stored as a JSON object
// string (e.g. the `manual_require` column on package_sources) into a
// map. Empty / whitespace-only input returns (nil, nil) — no require.
// A malformed JSON payload returns an error the caller surfaces as
// 400 / 422 to the user.
func ParseRequireJSON(s string) (map[string]string, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil, nil
	}
	var req map[string]string
	if err := json.Unmarshal([]byte(trimmed), &req); err != nil {
		return nil, fmt.Errorf("require is not a valid JSON object: %w", err)
	}
	return req, nil
}

// MergeRequire applies `override` on top of `base` and returns the
// merged map. Override keys win; keys only in base are kept. Neither
// input is mutated. Returns nil when both are empty to keep RawJSON
// clean of an empty `require:{}` block.
func MergeRequire(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

// synthesizedComposerJSON is the serialised shape of a manually
// constructed composer.json. Separate from ComposerJSON so we can
// control field ordering and emit only non-empty fields via omitempty.
type synthesizedComposerJSON struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Type        string            `json:"type,omitempty"`
	Description string            `json:"description,omitempty"`
	Homepage    string            `json:"homepage,omitempty"`
	Require     map[string]string `json:"require,omitempty"`
}
