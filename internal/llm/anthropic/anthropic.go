// Package anthropic implements llm.LLMClient against the Anthropic
// Messages API. Prompt caching is enabled.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/httpx"
	"github.com/rdegges/redline/internal/judge"
	"github.com/rdegges/redline/internal/llm"
)

// DefaultEndpoint is the Anthropic Messages API URL.
const DefaultEndpoint = "https://api.anthropic.com/v1/messages"

// Client is the Anthropic HTTP client.
type Client struct {
	Endpoint string
	APIKey   string
	HTTP     *http.Client
}

// New constructs a Client.
func New(apiKey string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &Client{Endpoint: DefaultEndpoint, APIKey: apiKey, HTTP: &http.Client{Timeout: timeout}}
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type         string         `json:"type"`
	Text         string         `json:"text,omitempty"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type messagesRequest struct {
	Model       string         `json:"model"`
	MaxTokens   int            `json:"max_tokens"`
	Temperature float64        `json:"temperature"`
	System      []contentBlock `json:"system"`
	Messages    []message      `json:"messages"`
}

type messagesResponse struct {
	ID      string         `json:"id"`
	Content []contentBlock `json:"content"`
	Usage   struct {
		InputTokens              int `json:"input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		OutputTokens             int `json:"output_tokens"`
	} `json:"usage"`
}

// Judge issues the Messages API call and parses the embedded JSON.
func (c *Client) Judge(ctx context.Context, jr llm.JudgeRequest) (*llm.JudgeResponse, error) {
	if c.APIKey == "" {
		return nil, errs.ErrAuthFailed
	}
	body := buildMessagesRequest(jr)
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.Endpoint, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	start := time.Now()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrAPIUnavailable, err)
	}
	defer httpx.DrainAndClose(resp)
	switch resp.StatusCode {
	case 401, 403:
		return nil, errs.ErrAuthFailed
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("anthropic: status %d", resp.StatusCode)
	}
	var mr messagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}
	var text string
	for _, b := range mr.Content {
		if b.Type == "text" {
			text += b.Text
		}
	}
	cleaned := stripCodeFences(text)
	var out llm.JudgeResponse
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrSchemaInvalid, err)
	}
	out.Model = jr.Model
	out.InputTokens = mr.Usage.InputTokens
	out.OutputTokens = mr.Usage.OutputTokens
	out.CacheHitTokens = mr.Usage.CacheReadInputTokens
	out.RawResponse = []byte(text)
	out.Latency = time.Since(start)
	return &out, nil
}

func buildMessagesRequest(jr llm.JudgeRequest) messagesRequest {
	sys := buildSystem(jr)
	canon := buildCanonical(jr)
	return messagesRequest{
		Model: jr.Model,
		// The v1 judge schema (findings + edit_plan + 50-2500 char
		// rationale) easily exceeds 1024 tokens; bump to 4096.
		MaxTokens:   4096,
		Temperature: 0.0,
		System: []contentBlock{
			{Type: "text", Text: sys, CacheControl: map[string]any{"type": "ephemeral"}},
			{Type: "text", Text: canon, CacheControl: map[string]any{"type": "ephemeral"}},
		},
		Messages: []message{
			{Role: "user", Content: []contentBlock{{Type: "text", Text: buildUserMsg(jr)}}},
		},
	}
}

func buildSystem(_ llm.JudgeRequest) string {
	var b strings.Builder
	b.WriteString("You are an expert website content auditor. For each page, return ONE JSON object that conforms EXACTLY to the schema below. ")
	b.WriteString("Output rules: no prose before or after the JSON; no markdown code fences (no ``` lines); start with `{` and end with `}`.\n\n")
	b.WriteString("# Required top-level keys\n\n")
	b.WriteString("`primary_label`, `confidence`, `suggested_action`, `page_summary`, `findings`, `rationale`. ")
	b.WriteString("Optional keys: `secondary_labels`, `affected_prompts`, `edit_plan`.\n\n")
	b.WriteString("Allowed `primary_label` values: Aligned, Stale, OffBrand, Contradictory, Redundant, Thin, Zombie, Unclear.\n")
	b.WriteString("Allowed `suggested_action` values: KEEP, UPDATE, REWRITE, DELETE, MANUAL_REVIEW.\n\n")
	b.WriteString("# Cross-field invariants (response is rejected if any is violated)\n\n")
	b.WriteString("1. If primary_label == \"Aligned\": findings = [], edit_plan = null, page_summary.should_focus_on = null, suggested_action = \"KEEP\".\n")
	b.WriteString("2. If suggested_action ∈ {UPDATE, REWRITE}: edit_plan is non-null AND findings has ≥1 item.\n")
	b.WriteString("3. If suggested_action == \"DELETE\": findings has ≥1 item AND edit_plan is null.\n")
	b.WriteString("4. If suggested_action == \"MANUAL_REVIEW\": confidence < 0.6.\n")
	b.WriteString("5. Every finding.quoted_text MUST be a literal substring of the PAGE BODY in the user message.\n")
	b.WriteString("6. Every affected_prompts entry must match an id from the PROMPTS list in the second system block.\n\n")
	b.WriteString("# JSON Schema (authoritative)\n\n")
	b.Write(judge.ResponseSchema())
	return b.String()
}

// stripCodeFences removes leading/trailing ```json ... ``` wrappers and
// any prose that landed outside the JSON object. Defensive: the system prompt
// instructs Claude not to use fences, but real-world responses sometimes
// add them anyway.
func stripCodeFences(s string) string {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "```") {
		if idx := strings.Index(t, "\n"); idx > 0 {
			t = t[idx+1:]
		}
		if end := strings.LastIndex(t, "```"); end >= 0 {
			t = t[:end]
		}
		t = strings.TrimSpace(t)
	}
	// Pull just the outer JSON object if the model added prose around it.
	if start := strings.Index(t, "{"); start > 0 {
		t = t[start:]
	}
	if end := strings.LastIndex(t, "}"); end >= 0 && end < len(t)-1 {
		t = t[:end+1]
	}
	return t
}

func buildCanonical(jr llm.JudgeRequest) string {
	var b strings.Builder
	b.WriteString("# CANONICAL MESSAGING\n")
	for _, m := range jr.CanonicalMessaging {
		b.WriteString("\n## " + m.Title + "\n" + m.Body + "\n")
	}
	b.WriteString("\n# PROMPTS\n")
	for _, p := range jr.Prompts {
		b.WriteString("- " + p.ID + ": " + p.Text + "\n")
	}
	return b.String()
}

func buildUserMsg(jr llm.JudgeRequest) string {
	return "PAGE URL: " + jr.PageURL + "\nPAGE TITLE: " + jr.PageTitle + "\n\nPAGE BODY:\n" + jr.PageBody
}
