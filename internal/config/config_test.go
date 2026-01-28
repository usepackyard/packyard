package config

import (
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
