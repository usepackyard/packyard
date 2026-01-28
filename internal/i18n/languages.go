// Package i18n holds the single source of truth for which dashboard locales
// Packyard supports. The backend uses it to validate profile updates; the
// frontend imports the same JSON manifest so the two lists cannot drift.
package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed languages.json
var manifestJSON []byte

// Language describes one supported UI locale.
type Language struct {
	Code   string `json:"code"`
	Native string `json:"native"`
}

var (
	languages []Language
	codeSet   map[string]struct{}
)

func init() {
	if err := json.Unmarshal(manifestJSON, &languages); err != nil {
		panic(fmt.Sprintf("i18n: parse languages.json: %v", err))
	}
	codeSet = make(map[string]struct{}, len(languages))
	for _, l := range languages {
		codeSet[l.Code] = struct{}{}
	}
}

// Supported returns the supported locales in manifest order.
func Supported() []Language {
	out := make([]Language, len(languages))
	copy(out, languages)
	return out
}

// IsSupported reports whether code is a known locale.
func IsSupported(code string) bool {
	_, ok := codeSet[code]
	return ok
}

// Codes returns just the language codes, in manifest order.
func Codes() []string {
	out := make([]string, len(languages))
	for i, l := range languages {
		out[i] = l.Code
	}
	return out
}
