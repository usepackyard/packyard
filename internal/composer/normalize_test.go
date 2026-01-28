package composer

import "testing"

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0", "1.0.0.0"},
		{"1.0.0", "1.0.0.0"},
		{"1.0.0.0", "1.0.0.0"},
		{"v2.3.4", "2.3.4.0"},
		{"2.0.0-beta.14", "2.0.0.0-beta14"},
		{"3.1-alpha.2", "3.1.0.0-alpha2"},
		{"v1.0.0-rc.1", "1.0.0.0-rc1"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeVersion(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
