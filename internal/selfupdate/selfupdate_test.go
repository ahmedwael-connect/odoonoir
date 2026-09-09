package selfupdate

import "testing"

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"v0.24.0", "0.24.0"},
		{"0.24.0", "0.24.0"},
		{"v1.2.3-beta", "1.2.3"},
		{"v10.20.30", "10.20.30"},
		{"  v0.1.2  ", "0.1.2"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeVersion(tt.input)
		if got != tt.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
