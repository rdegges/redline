// Package config loads and validates prompts.yaml.
package config

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"

	"github.com/rdegges/redline/internal/errs"
	"gopkg.in/yaml.v3"
)

//go:embed prompts.schema.json
var schemaJSON []byte

// Schema returns the embedded JSON Schema.
func Schema() []byte { return schemaJSON }

// Prompt is a single GEO target.
type Prompt struct {
	ID     string   `yaml:"id"`
	Text   string   `yaml:"text"`
	Weight float64  `yaml:"weight"`
	Tags   []string `yaml:"tags"`
}

// MessagingBlock is a canonical_messaging block.
type MessagingBlock struct {
	Title string `yaml:"title"`
	Body  string `yaml:"body"`
}

// File is the parsed prompts.yaml document.
type File struct {
	Version            string           `yaml:"version"`
	Prompts            []Prompt         `yaml:"prompts"`
	CanonicalMessaging []MessagingBlock `yaml:"canonical_messaging"`
	LabelOverrides     map[string]any   `yaml:"label_overrides"`
	Seeds              []string         `yaml:"seeds"`
	// SHA256 of the raw on-disk bytes; set by Load.
	SHA256 string `yaml:"-"`
}

// DefaultWeight is the prompt weight when not specified.
const DefaultWeight = 1.0

var idPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// Load reads, parses, and validates the file at path.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", errs.ErrPromptsNotFound, path)
		}
		return nil, fmt.Errorf("read prompts: %w", err)
	}
	return parse(raw)
}

func parse(raw []byte) (*File, error) {
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrInvalidConfig, err)
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	f.SHA256 = hex.EncodeToString(sum[:])
	for i := range f.Prompts {
		if f.Prompts[i].Weight == 0 {
			f.Prompts[i].Weight = DefaultWeight
		}
	}
	return &f, nil
}

// Parse is a public wrapper around parse for tests and `--print-schema`.
func Parse(raw []byte) (*File, error) { return parse(raw) }

func (f *File) validate() error {
	if f.Version != "1" {
		return fmt.Errorf("%w: version must be \"1\", got %q", errs.ErrInvalidConfig, f.Version)
	}
	if len(f.Prompts) == 0 {
		return fmt.Errorf("%w: prompts must be non-empty", errs.ErrInvalidConfig)
	}
	if len(f.Prompts) > 100 {
		return fmt.Errorf("%w: prompts has %d entries (max 100)", errs.ErrInvalidConfig, len(f.Prompts))
	}
	seen := map[string]bool{}
	for i, p := range f.Prompts {
		if !idPattern.MatchString(p.ID) {
			return fmt.Errorf("%w: prompt[%d].id %q must match %s", errs.ErrInvalidConfig, i, p.ID, idPattern)
		}
		if seen[p.ID] {
			return fmt.Errorf("%w: duplicate prompt id %q", errs.ErrInvalidConfig, p.ID)
		}
		seen[p.ID] = true
		if p.Text == "" {
			return fmt.Errorf("%w: prompt[%d].text is empty", errs.ErrInvalidConfig, i)
		}
		if len(p.Text) > 2000 {
			return fmt.Errorf("%w: prompt[%d].text > 2000 chars", errs.ErrInvalidConfig, i)
		}
		if p.Weight < 0 || p.Weight > 10 {
			return fmt.Errorf("%w: prompt[%d].weight %.2f out of range [0,10]", errs.ErrInvalidConfig, i, p.Weight)
		}
	}
	if len(f.CanonicalMessaging) > 50 {
		return fmt.Errorf("%w: canonical_messaging has %d entries (max 50)", errs.ErrInvalidConfig, len(f.CanonicalMessaging))
	}
	for i, b := range f.CanonicalMessaging {
		if b.Title == "" {
			return fmt.Errorf("%w: canonical_messaging[%d].title is empty", errs.ErrInvalidConfig, i)
		}
		if len(b.Body) > 20000 {
			return fmt.Errorf("%w: canonical_messaging[%d].body > 20000 chars", errs.ErrInvalidConfig, i)
		}
	}
	for i, s := range f.Seeds {
		u, err := url.Parse(s)
		if err != nil || !u.IsAbs() {
			return fmt.Errorf("%w: seed[%d] %q is not an absolute URL", errs.ErrInvalidConfig, i, s)
		}
	}
	return nil
}

// ValidateSeedsAgainstHost ensures every seed shares its host with site.
func (f *File) ValidateSeedsAgainstHost(site string) error {
	siteURL, err := url.Parse(site)
	if err != nil {
		return fmt.Errorf("%w: --site %q: %v", errs.ErrInvalidConfig, site, err)
	}
	for i, s := range f.Seeds {
		u, _ := url.Parse(s)
		if u.Host != siteURL.Host {
			return fmt.Errorf("%w: seed[%d] %q host does not match --site host %q",
				errs.ErrInvalidConfig, i, s, siteURL.Host)
		}
	}
	return nil
}
