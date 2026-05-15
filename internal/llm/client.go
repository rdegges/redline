// Package llm defines the provider-agnostic LLM client interface used
// by the judge. The Ollama implementation lives in internal/llm/ollama
// and the Anthropic implementation in internal/llm/anthropic; both
// satisfy LLMClient and produce a JudgeResponse matching the schema.
package llm

import (
	"context"
	"time"

	"github.com/rdegges/redline/internal/config"
)

// JudgeRequest is the per-page input handed to a provider client.
type JudgeRequest struct {
	Model              string
	PageURL            string
	PageTitle          string
	PageBody           string
	Prompts            []config.Prompt
	CanonicalMessaging []config.MessagingBlock
	IncludeFewShot     bool // set on retry attempt 3
	TruncatedBody      bool // set on retry attempt 2 (stripped down)
}

// JudgeResponse mirrors the parsed schema in the schema.
type JudgeResponse struct {
	PrimaryLabel    string      `json:"primary_label"`
	SecondaryLabels []string    `json:"secondary_labels,omitempty"`
	Confidence      float64     `json:"confidence"`
	AffectedPrompts []string    `json:"affected_prompts,omitempty"`
	SuggestedAction string      `json:"suggested_action"`
	PageSummary     PageSummary `json:"page_summary"`
	Findings        []Finding   `json:"findings"`
	EditPlan        *EditPlan   `json:"edit_plan,omitempty"`
	Rationale       string      `json:"rationale"`

	// Metadata captured from the underlying call; not part of the LLM
	// response per se.
	Model          string        `json:"-"`
	InputTokens    int           `json:"-"`
	OutputTokens   int           `json:"-"`
	CacheHitTokens int           `json:"-"`
	Latency        time.Duration `json:"-"`
	RawResponse    []byte        `json:"-"`
}

// PageSummary mirrors the schema.
type PageSummary struct {
	CurrentFocus  string  `json:"current_focus"`
	ShouldFocusOn *string `json:"should_focus_on"`
}

// Finding is one atomic issue identified on a page.
type Finding struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	QuotedText   string `json:"quoted_text"`
	LocationHint string `json:"location_hint"`
	Issue        string `json:"issue"`
	SuggestedFix string `json:"suggested_fix"`
}

// EditPlan is the structured edit recipe for UPDATE/REWRITE pages.
type EditPlan struct {
	Summary  string         `json:"summary"`
	Preserve []string       `json:"preserve,omitempty"`
	Remove   []string       `json:"remove,omitempty"`
	Rewrite  []RewriteEntry `json:"rewrite,omitempty"`
	Add      []string       `json:"add,omitempty"`
}

// RewriteEntry is one element-level rewrite within an EditPlan.
type RewriteEntry struct {
	Element        string `json:"element"`
	CurrentFraming string `json:"current_framing"`
	NewFraming     string `json:"new_framing"`
}

// LLMClient is implemented by every provider (Ollama, Anthropic, …).
type LLMClient interface {
	Judge(ctx context.Context, req JudgeRequest) (*JudgeResponse, error)
}
