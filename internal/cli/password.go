package cli

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
)

// GeneratePassword returns a 20-character Crockford-base32 password
// derived from crypto/rand. Format: XXXX-XXXX-XXXX-XXXX (hyphens for
// typo tolerance — people copy these off screens). 20 chars of this
// alphabet is ~92 bits of entropy, comfortably past the Packyard
// password-hashing threshold.
func GeneratePassword() (string, error) {
	buf := make([]byte, 13) // → 21 base32 chars, we'll trim to 20
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	raw := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
	raw = raw[:20]

	// Insert hyphens every 4 chars for readability. The user can
	// paste with or without them; login accepts both because we
	// strip hyphens when reading them back.
	return fmt.Sprintf("%s-%s-%s-%s", raw[:4], raw[4:8], raw[8:12], raw[12:20]), nil
}

// GenerateSessionSecret returns 64 hex characters (32 bytes of
// crypto/rand output). That's the exact shape the server's Validate()
// check accepts; anything shorter fails startup.
func GenerateSessionSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session secret: %w", err)
	}
	out := make([]byte, 64)
	const hexchars = "0123456789abcdef"
	for i, b := range buf {
		out[2*i] = hexchars[b>>4]
		out[2*i+1] = hexchars[b&0x0f]
	}
	return string(out), nil
}
