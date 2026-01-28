package composer

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ComposerJSON represents the relevant fields from a composer.json file.
type ComposerJSON struct {
	Name        string                       `json:"name"`
	Version     string                       `json:"version"`
	Type        string                       `json:"type"`
	Description string                       `json:"description,omitempty"`
	Homepage    string                       `json:"homepage,omitempty"`
	Require     map[string]string            `json:"require,omitempty"`
	Autoload    map[string]json.RawMessage   `json:"autoload,omitempty"`
	Extra       map[string]json.RawMessage   `json:"extra,omitempty"`
	Authors     []map[string]string          `json:"authors,omitempty"`
	License     json.RawMessage              `json:"license,omitempty"`
	RawJSON     string                       `json:"-"`
}

// Zip-bomb protection limits.
const (
	maxZipEntries       = 10000    // reject archives with more than this many entries
	maxComposerJSONSize = 1 << 20  // 1 MB — composer.json should be tiny
)

// ParseZIP extracts and parses composer.json from a ZIP file.
// It looks for composer.json at the root level or one directory deep.
func ParseZIP(path string) (*ComposerJSON, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	if len(r.File) > maxZipEntries {
		return nil, fmt.Errorf("zip has too many entries (%d > %d)", len(r.File), maxZipEntries)
	}

	for _, f := range r.File {
		name := filepath.Clean(f.Name)
		// Match composer.json at root or one level deep (e.g., "plugin-name/composer.json").
		if name == "composer.json" || filepath.Base(name) == "composer.json" && strings.Count(name, string(filepath.Separator)) <= 1 {
			// Reject absurd declared sizes before opening the reader.
			if f.UncompressedSize64 > maxComposerJSONSize {
				return nil, fmt.Errorf("composer.json too large (%d bytes)", f.UncompressedSize64)
			}

			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("open composer.json in zip: %w", err)
			}
			defer rc.Close()

			// Cap actual decompressed read, even if the header lied.
			data, err := io.ReadAll(io.LimitReader(rc, maxComposerJSONSize+1))
			if err != nil {
				return nil, fmt.Errorf("read composer.json: %w", err)
			}
			if int64(len(data)) > maxComposerJSONSize {
				return nil, fmt.Errorf("composer.json exceeds %d bytes after decompression", maxComposerJSONSize)
			}

			var cj ComposerJSON
			if err := json.Unmarshal(data, &cj); err != nil {
				return nil, fmt.Errorf("parse composer.json: %w", err)
			}

			cj.RawJSON = string(data)

			if cj.Name == "" {
				return nil, fmt.Errorf("composer.json missing required field: name")
			}

			return &cj, nil
		}
	}

	// Not found — give the caller enough signal to diagnose. Common cause
	// is a production-build drop (WordPress plugin release asset) that
	// excluded composer.json; we list a few archive entries so the user
	// can see what was inside and nudge them toward the hybrid strategy.
	return nil, fmt.Errorf(
		"composer.json not found in zip archive (found: %s). "+
			"hint: if this is a production build without dev metadata, "+
			"configure the source to use strategy \"source_archive\" "+
			"so composer.json is pulled from the tagged source tree",
		summarizeZipEntries(r.File, 5),
	)
}

// summarizeZipEntries returns a short, human-readable list of the first
// `limit` entry names in a zip, with "... and N more" appended when the
// archive has more. Used to decorate the "composer.json not found" error
// so the user can see what the archive actually contains.
func summarizeZipEntries(files []*zip.File, limit int) string {
	if len(files) == 0 {
		return "(empty archive)"
	}
	n := len(files)
	if n > limit {
		n = limit
	}
	names := make([]string, 0, n)
	for i := 0; i < n; i++ {
		names = append(names, files[i].Name)
	}
	s := strings.Join(names, ", ")
	if len(files) > limit {
		s += fmt.Sprintf(", ... and %d more", len(files)-limit)
	}
	return s
}

// SaveTempFile writes an io.Reader to a temporary file and returns the path.
func SaveTempFile(r io.Reader, maxSize int64) (string, error) {
	tmp, err := os.CreateTemp("", "packyard-upload-*.zip")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	limited := io.LimitReader(r, maxSize+1)
	n, err := io.Copy(tmp, limited)
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if n > maxSize {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("file exceeds maximum size of %d bytes", maxSize)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}

	return tmp.Name(), nil
}

// NormalizeVersion converts a Composer version string to its normalized form.
// Examples:
//   - "2.0.0" → "2.0.0.0"
//   - "2.0.0-beta.14" → "2.0.0.0-beta14"
//   - "1.0" → "1.0.0.0"
func NormalizeVersion(version string) string {
	version = strings.TrimPrefix(version, "v")

	// Split off stability suffix.
	var stability string
	parts := regexp.MustCompile(`[-]`).Split(version, 2)
	main := parts[0]
	if len(parts) > 1 {
		stability = parts[1]
	}

	// Pad version to 4 segments.
	segments := strings.Split(main, ".")
	for len(segments) < 4 {
		segments = append(segments, "0")
	}
	segments = segments[:4]

	// Normalize each segment to a number.
	for i, seg := range segments {
		if n, err := strconv.Atoi(seg); err == nil {
			segments[i] = strconv.Itoa(n)
		}
	}

	normalized := strings.Join(segments, ".")

	if stability != "" {
		// Remove dots from stability suffix: "beta.14" → "beta14"
		stability = strings.ReplaceAll(stability, ".", "")
		normalized += "-" + stability
	}

	return normalized
}
