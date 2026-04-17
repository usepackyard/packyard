package cli

import (
	"strings"
	"testing"
)

func TestGeneratePassword_Shape(t *testing.T) {
	pw, err := GeneratePassword()
	if err != nil {
		t.Fatalf("GeneratePassword: %v", err)
	}
	// 4-4-4-8 groups joined with hyphens → 20 chars + 3 hyphens.
	if want := 23; len(pw) != want {
		t.Errorf("len = %d, want %d: %q", len(pw), want, pw)
	}
	if strings.Count(pw, "-") != 3 {
		t.Errorf("hyphen count = %d, want 3: %q", strings.Count(pw, "-"), pw)
	}
}

func TestGeneratePassword_DifferentEachCall(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		pw, err := GeneratePassword()
		if err != nil {
			t.Fatalf("GeneratePassword: %v", err)
		}
		if _, dup := seen[pw]; dup {
			t.Fatalf("duplicate password after %d draws: %q", i, pw)
		}
		seen[pw] = struct{}{}
	}
}

func TestGenerateSessionSecret_64Hex(t *testing.T) {
	s, err := GenerateSessionSecret()
	if err != nil {
		t.Fatalf("GenerateSessionSecret: %v", err)
	}
	if len(s) != 64 {
		t.Errorf("len = %d, want 64: %q", len(s), s)
	}
	for i, r := range s {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isHex {
			t.Errorf("non-hex char at %d: %q in %q", i, r, s)
			return
		}
	}
}
