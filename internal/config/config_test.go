package config

import (
	"net/http"
	"strings"
	"testing"
)

func TestParseCIDRs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string // string form of resulting CIDRs, in order
	}{
		{"empty", "", nil},
		{"whitespace only", "   ,  ", nil},
		{"single CIDR", "10.0.0.0/8", []string{"10.0.0.0/8"}},
		{"multiple", "10.0.0.0/8,127.0.0.1/32", []string{"10.0.0.0/8", "127.0.0.1/32"}},
		{"bare IPv4 → /32", "127.0.0.1", []string{"127.0.0.1/32"}},
		{"bare IPv6 → /128", "::1", []string{"::1/128"}},
		{"mixed bare and CIDR", "10.0.0.0/8, 127.0.0.1", []string{"10.0.0.0/8", "127.0.0.1/32"}},
		{"invalid skipped", "10.0.0.0/8, not-an-ip, 127.0.0.1", []string{"10.0.0.0/8", "127.0.0.1/32"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCIDRs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got: %v)", len(got), len(tt.want), got)
			}
			for i, n := range got {
				if n.String() != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, n.String(), tt.want[i])
				}
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"empty rejected", "", true},
		{"too short rejected", strings.Repeat("a", minSessionSecretLen-1), true},
		{"exactly minimum accepted", strings.Repeat("a", minSessionSecretLen), false},
		{"longer accepted", strings.Repeat("a", 64), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Session: SessionConfig{Secret: tt.secret}}
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate(): err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestLoad_BcryptCostClamp(t *testing.T) {
	t.Setenv("PACKYARD_BCRYPT_COST", "4") // way too low
	cfg := Load()
	if cfg.BcryptCost < minBcryptCost {
		t.Fatalf("BcryptCost = %d, want >= %d (clamp didn't fire)", cfg.BcryptCost, minBcryptCost)
	}
}

func TestLoad_BcryptCostHonoredWhenAboveFloor(t *testing.T) {
	t.Setenv("PACKYARD_BCRYPT_COST", "13")
	cfg := Load()
	if cfg.BcryptCost != 13 {
		t.Fatalf("BcryptCost = %d, want 13", cfg.BcryptCost)
	}
}

func TestParseSameSite(t *testing.T) {
	tests := []struct {
		in     string
		want   http.SameSite
		wantOK bool
	}{
		{"", http.SameSiteStrictMode, true},
		{"strict", http.SameSiteStrictMode, true},
		{"STRICT", http.SameSiteStrictMode, true},
		{"lax", http.SameSiteLaxMode, true},
		{"  lax  ", http.SameSiteLaxMode, true},
		{"none", http.SameSiteNoneMode, true},
		{"banana", http.SameSiteStrictMode, false}, // unknown falls back to strict
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := parseSameSite(tt.in)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("parseSameSite(%q) = (%v, %v), want (%v, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestValidate_CookieSameSiteNoneRequiresHTTPS(t *testing.T) {
	cfg := &Config{
		Session: SessionConfig{
			Secret:         strings.Repeat("a", minSessionSecretLen),
			CookieSameSite: http.SameSiteNoneMode,
		},
		BaseURL: "http://localhost:9090",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected SameSite=None + http BaseURL to fail Validate")
	}
	cfg.BaseURL = "https://packyard.test"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("SameSite=None + https should pass, got %v", err)
	}
}

func TestValidate_PublicURLScheme(t *testing.T) {
	cfg := &Config{
		Session:   SessionConfig{Secret: strings.Repeat("a", minSessionSecretLen)},
		BaseURL:   "https://packyard.test",
		PublicURL: "packyard.dev", // missing scheme
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected scheme-less PublicURL to fail Validate")
	}
	cfg.PublicURL = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty PublicURL should pass, got %v", err)
	}
	cfg.PublicURL = "https://packyard.dev"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("https PublicURL should pass, got %v", err)
	}
}

func TestLoad_CookieEnvDefaults(t *testing.T) {
	t.Setenv("PACKYARD_COOKIE_DOMAIN", "")
	t.Setenv("PACKYARD_COOKIE_SAMESITE", "")
	cfg := Load()
	if cfg.Session.CookieDomain != "" {
		t.Errorf("default CookieDomain should be empty, got %q", cfg.Session.CookieDomain)
	}
	if cfg.Session.CookieSameSite != http.SameSiteStrictMode {
		t.Errorf("default CookieSameSite should be Strict, got %v", cfg.Session.CookieSameSite)
	}
}

func TestLoad_CookieEnvOverrides(t *testing.T) {
	t.Setenv("PACKYARD_COOKIE_DOMAIN", ".packyard.test")
	t.Setenv("PACKYARD_COOKIE_SAMESITE", "lax")
	t.Setenv("PACKYARD_PUBLIC_URL", "https://example.test")
	cfg := Load()
	if cfg.Session.CookieDomain != ".packyard.test" {
		t.Errorf("CookieDomain = %q", cfg.Session.CookieDomain)
	}
	if cfg.Session.CookieSameSite != http.SameSiteLaxMode {
		t.Errorf("CookieSameSite = %v", cfg.Session.CookieSameSite)
	}
	if cfg.PublicURL != "https://example.test" {
		t.Errorf("PublicURL = %q", cfg.PublicURL)
	}
}
