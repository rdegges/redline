package config

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdegges/redline/internal/errs"
)

func TestParse_Minimal_OK(t *testing.T) {
	raw := []byte(`
version: "1"
prompts:
  - id: sast-best
    text: "What is the best SAST tool?"
`)
	f, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.Prompts[0].ID != "sast-best" {
		t.Fatalf("id = %q", f.Prompts[0].ID)
	}
	if f.Prompts[0].Weight != 1.0 {
		t.Fatalf("default weight = %v, want 1.0", f.Prompts[0].Weight)
	}
	if f.SHA256 == "" {
		t.Fatal("expected sha256 to be populated")
	}
}

func TestParse_AllValidationRules(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{
			name: "missing-version",
			raw: `
prompts:
  - id: a
    text: x`,
		},
		{
			name: "wrong-version",
			raw: `version: "2"
prompts:
  - id: a
    text: x`,
		},
		{
			name: "empty-prompts",
			raw: `version: "1"
prompts: []`,
		},
		{
			name: "duplicate-id",
			raw: `version: "1"
prompts:
  - id: a
    text: x
  - id: a
    text: y`,
		},
		{
			name: "bad-id",
			raw: `version: "1"
prompts:
  - id: Bad-ID
    text: x`,
		},
		{
			name: "empty-text",
			raw: `version: "1"
prompts:
  - id: a
    text: ""`,
		},
		{
			name: "weight-out-of-range",
			raw: `version: "1"
prompts:
  - id: a
    text: x
    weight: 11`,
		},
		{
			name: "bad-yaml",
			raw:  "::: not yaml",
		},
		{
			name: "seed-not-absolute",
			raw: `version: "1"
prompts:
  - id: a
    text: x
seeds:
  - "/relative/path"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.raw))
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !errors.Is(err, errs.ErrInvalidConfig) {
				t.Fatalf("expected ErrInvalidConfig, got %v", err)
			}
		})
	}
}

func TestLoad_FileNotFound_ReturnsSentinel(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if !errors.Is(err, errs.ErrPromptsNotFound) {
		t.Fatalf("expected ErrPromptsNotFound, got %v", err)
	}
}

func TestSchema_IsValidJSON(t *testing.T) {
	s := Schema()
	if !strings.Contains(string(s), "\"$schema\"") {
		t.Fatal("expected $schema field")
	}
}

func TestValidateSeedsAgainstHost(t *testing.T) {
	f := &File{Seeds: []string{"https://example.com/a", "https://other.com/b"}}
	if err := f.ValidateSeedsAgainstHost("https://example.com"); err == nil {
		t.Fatal("expected host mismatch error")
	}
	f.Seeds = []string{"https://example.com/a"}
	if err := f.ValidateSeedsAgainstHost("https://example.com"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(`version: "1"
prompts:
  - id: a
    text: x`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		// Must never panic.
		_, _ = Parse(raw)
	})
}
