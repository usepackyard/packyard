package auth

import (
	"testing"
)

func TestSignVerifyCookie_RoundTrip(t *testing.T) {
	const secret = "this-is-32-characters-of-secret!"
	id := "abc123def456"

	signed := signCookie(id, secret)
	if signed == id {
		t.Fatal("signCookie returned bare id with no MAC")
	}
	got, ok := verifyCookie(signed, secret)
	if !ok {
		t.Fatal("verifyCookie rejected a freshly-signed cookie")
	}
	if got != id {
		t.Fatalf("verifyCookie returned id %q, want %q", got, id)
	}
}

func TestVerifyCookie_TamperDetection(t *testing.T) {
	const secret = "this-is-32-characters-of-secret!"
	signed := signCookie("session-xyz", secret)

	// Flip one byte of the MAC portion.
	dotIdx := -1
	for i, c := range signed {
		if c == '.' {
			dotIdx = i
			break
		}
	}
	if dotIdx == -1 {
		t.Fatal("malformed signed cookie — no dot separator")
	}

	bytes := []byte(signed)
	// Toggle a low-order bit on the first MAC byte (after the dot).
	bytes[dotIdx+1] ^= 0x01
	tampered := string(bytes)

	if _, ok := verifyCookie(tampered, secret); ok {
		t.Fatal("verifyCookie accepted a tampered MAC")
	}
}

func TestVerifyCookie_WrongSecret(t *testing.T) {
	signed := signCookie("session-xyz", "secret-A-padding-padding-padding")
	if _, ok := verifyCookie(signed, "secret-B-padding-padding-padding"); ok {
		t.Fatal("verifyCookie accepted MAC computed with a different secret")
	}
}

func TestVerifyCookie_MalformedInputs(t *testing.T) {
	const secret = "this-is-32-characters-of-secret!"
	cases := []string{
		"",
		"no-dot-at-all",
		".only-mac-no-id",
		"only-id-no-mac.",
		"id.not-hex",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if _, ok := verifyCookie(in, secret); ok {
				t.Fatalf("verifyCookie accepted malformed input %q", in)
			}
		})
	}
}
