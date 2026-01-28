package composer

import (
	"strings"
	"testing"
)

func TestValidatePackageName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid Composer-style names.
		{"simple", "vendor/package", false},
		{"hyphens", "my-vendor/my-package", false},
		{"underscores", "my_vendor/my_package", false},
		{"dots", "my.vendor/my.package", false},
		{"digits", "vendor1/package2", false},
		{"realistic", "packyard/sample-package", false},

		// Invalid: structural problems.
		{"empty", "", true},
		{"no slash", "vendorpackage", true},
		{"two slashes", "vendor/sub/package", true},
		{"trailing slash", "vendor/", true},
		{"leading slash", "/package", true},

		// Invalid: case + character set.
		{"uppercase vendor", "Vendor/package", true},
		{"uppercase package", "vendor/Package", true},
		{"unicode", "véndor/päckage", true},
		{"space", "vendor name/package", true},

		// Invalid: path traversal attempts.
		{"dot dot", "vendor/..", true},
		{"escape", "vendor/../etc", true},
		{"backslash", "vendor\\package", true},

		// Invalid: too long.
		{"too long", strings.Repeat("a", 60) + "/" + strings.Repeat("b", 60), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePackageName(%q): got err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOrgSlug(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid.
		{"simple", "acme", false},
		{"hyphenated", "my-org", false},
		{"alphanum", "team42", false},
		{"min length", "abc", false},
		{"max length", "this-is-thirtytwo-character-org1", false},

		// Format violations.
		{"empty", "", true},
		{"too short", "ab", true},
		{"too long", strings.Repeat("a", 33), true},
		{"uppercase", "Acme", true},
		{"starts with digit", "1team", true},
		{"starts with hyphen", "-team", true},
		{"ends with hyphen", "team-", true},
		{"underscore", "my_team", true},
		{"dot", "my.team", true},
		{"slash", "vendor/pkg", true},
		{"path traversal", "../etc", true},
		{"unicode", "äcme", true},
		{"space", "my team", true},

		// Reserved subdomains.
		{"reserved www", "www", true},
		{"reserved api", "api", true},
		{"reserved admin", "admin", true},
		{"reserved app", "app", true},
		{"reserved repo", "repo", true},
		{"reserved billing", "billing", true},
		{"reserved auth", "auth", true},
		{"reserved healthz", "healthz", true},
		{"reserved root", "root", true},
		{"reserved default", "default", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOrgSlug(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateOrgSlug(%q): err=%v wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"semver", "1.0.0", false},
		{"v prefix", "v2.3.4", false},
		{"prerelease", "1.0.0-beta.1", false},
		{"build metadata", "1.0.0+abc123", false},
		{"branch dev", "dev-main", false},
		{"single number", "1", false},

		{"empty", "", true},
		{"slash", "1/0", true},
		{"backslash", "1\\0", true},
		{"path escape", "../../1.0.0", true},
		{"shell meta", "1.0.0; rm -rf /", true},
		{"newline", "1.0.0\n", true},
		{"too long", strings.Repeat("1", 65), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateVersion(%q): got err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}
