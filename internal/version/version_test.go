package version

import "testing"

func TestInfo_DefaultsArePopulated(t *testing.T) {
	v, c, d := Info()
	if v == "" || c == "" || d == "" {
		t.Fatalf("Info returned empty fields: %q %q %q", v, c, d)
	}
}

func TestInfo_ReturnsPackageVars(t *testing.T) {
	Version = "1.2.3"
	Commit = "abcdef0"
	Date = "2026-01-01T00:00:00Z"
	t.Cleanup(func() {
		Version = "0.0.0-dev"
		Commit = "unknown"
		Date = "unknown"
	})
	v, c, d := Info()
	if v != "1.2.3" || c != "abcdef0" || d != "2026-01-01T00:00:00Z" {
		t.Fatalf("unexpected Info output: %q %q %q", v, c, d)
	}
}
