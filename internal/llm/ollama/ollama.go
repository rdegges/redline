// Package ollama implements llm.LLMClient against an Ollama HTTP server.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/httpx"
	"github.com/rdegges/redline/internal/judge"
	"github.com/rdegges/redline/internal/llm"
)

// Client is the Ollama HTTP client.
type Client struct {
	URL       string
	HTTP      *http.Client
	KeepAlive string
	NumCtx    int
}

// New constructs a Client.
func New(url string, timeout time.Duration, keepAlive string) *Client {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &Client{
		URL:       strings.TrimRight(url, "/"),
		HTTP:      &http.Client{Timeout: timeout},
		KeepAlive: keepAlive,
		NumCtx:    16384,
	}
}

type chatRequest struct {
	Model     string         `json:"model"`
	Stream    bool           `json:"stream"`
	Format    string         `json:"format"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Messages  []chatMessage  `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model           string      `json:"model"`
	CreatedAt       string      `json:"created_at"`
	Message         chatMessage `json:"message"`
	Done            bool        `json:"done"`
	PromptEvalCount int         `json:"prompt_eval_count"`
	EvalCount       int         `json:"eval_count"`
}

// Judge sends the request to /api/chat with format=json and parses the
// embedded judge response. Schema-invalid bodies bubble up as
// errs.ErrSchemaInvalid.
func (c *Client) Judge(ctx context.Context, jr llm.JudgeRequest) (*llm.JudgeResponse, error) {
	system := buildSystemPrompt(jr)
	user := buildUserPrompt(jr)
	body := chatRequest{
		Model:     jr.Model,
		Stream:    false,
		Format:    "json",
		KeepAlive: c.KeepAlive,
		Options: map[string]any{
			"temperature": 0.0,
			"num_ctx":     c.NumCtx,
			// A full judge response (findings + edit_plan + 3-6 paragraph
			// rationale up to 2500 chars) frequently exceeds the typical
			// 1024-token default and gets truncated, producing
			// "unexpected end of JSON". 4096 gives ~4x headroom.
			"num_predict": 4096,
			"seed":        42,
		},
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.URL+"/api/chat", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrOllamaUnavailable, err)
	}
	defer httpx.DrainAndClose(resp)
	if resp.StatusCode == 404 {
		return nil, errs.ErrOllamaModelMissing
	}
	if resp.StatusCode == 500 || resp.StatusCode == 503 {
		// Check the model-loading marker.
		buf, _ := io.ReadAll(resp.Body)
		if bytes.Contains(buf, []byte("model is loading")) || bytes.Contains(buf, []byte("loading")) {
			return nil, errs.ErrOllamaLoading
		}
		return nil, fmt.Errorf("%w: status %d", errs.ErrOllamaUnavailable, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama chat: status %d", resp.StatusCode)
	}
	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}
	var out llm.JudgeResponse
	if err := json.Unmarshal([]byte(cr.Message.Content), &out); err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrSchemaInvalid, err)
	}
	out.Model = jr.Model
	out.InputTokens = cr.PromptEvalCount
	out.OutputTokens = cr.EvalCount
	out.RawResponse = []byte(cr.Message.Content)
	out.Latency = time.Since(start)
	return &out, nil
}

func buildSystemPrompt(jr llm.JudgeRequest) string {
	var b strings.Builder
	b.WriteString("You are an expert website content auditor. For each page, return ONE JSON object that conforms EXACTLY to the schema below — no other top-level keys, no prose outside the JSON.\n\n")
	b.WriteString("# Required output structure\n\n")
	b.WriteString("Your JSON object MUST contain these top-level keys: ")
	b.WriteString("`primary_label`, `confidence`, `suggested_action`, `page_summary`, `findings`, `rationale`. ")
	b.WriteString("Optional keys: `secondary_labels`, `affected_prompts`, `edit_plan`.\n\n")
	b.WriteString("Allowed `primary_label` values: Aligned, Stale, OffBrand, Contradictory, Redundant, Thin, Zombie, Unclear.\n")
	b.WriteString("Allowed `suggested_action` values: KEEP, UPDATE, REWRITE, DELETE, MANUAL_REVIEW.\n\n")
	b.WriteString("# Cross-field invariants (your output WILL be rejected if any is violated)\n\n")
	b.WriteString("1. If primary_label == \"Aligned\": findings = [], edit_plan = null, page_summary.should_focus_on = null, suggested_action = \"KEEP\".\n")
	b.WriteString("2. If suggested_action ∈ {UPDATE, REWRITE}: edit_plan is non-null AND findings has ≥1 item.\n")
	b.WriteString("3. If suggested_action == \"DELETE\": findings has ≥1 item AND edit_plan is null.\n")
	b.WriteString("4. If suggested_action == \"MANUAL_REVIEW\": confidence < 0.6.\n")
	b.WriteString("5. Every finding.quoted_text MUST be a literal substring of the PAGE BODY in the user message. Quote exactly.\n")
	b.WriteString("6. Every affected_prompts entry must match an id from the PROMPTS list below.\n\n")
	b.WriteString("# JSON Schema (authoritative)\n\n")
	b.Write(judge.ResponseSchema())
	b.WriteString("\n\n# CANONICAL MESSAGING (the source of truth for this brand)\n")
	for _, m := range jr.CanonicalMessaging {
		b.WriteString("\n## ")
		b.WriteString(m.Title)
		b.WriteString("\n")
		b.WriteString(m.Body)
		b.WriteString("\n")
	}
	b.WriteString("\n# PROMPTS (queries we want LLM answer engines to handle correctly)\n")
	for _, p := range jr.Prompts {
		b.WriteString("- ")
		b.WriteString(p.ID)
		b.WriteString(": ")
		b.WriteString(p.Text)
		b.WriteString("\n")
	}
	return b.String()
}

func buildUserPrompt(jr llm.JudgeRequest) string {
	var b strings.Builder
	b.WriteString("PAGE URL: ")
	b.WriteString(jr.PageURL)
	b.WriteString("\nPAGE TITLE: ")
	b.WriteString(jr.PageTitle)
	b.WriteString("\n\nPAGE BODY:\n")
	b.WriteString(jr.PageBody)
	return b.String()
}

// CheckReachable hits /api/version. Returns nil if Ollama is up.
func (c *Client) CheckReachable(ctx context.Context) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.URL+"/api/version", nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", errs.ErrOllamaUnavailable, err)
	}
	defer httpx.DrainAndClose(resp)
	var v struct {
		Version string `json:"version"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&v)
	return v.Version, nil
}

// Tags returns the list of model tags available in Ollama.
func (c *Client) Tags(ctx context.Context) ([]string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.URL+"/api/tags", nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errs.ErrOllamaUnavailable, err)
	}
	defer httpx.DrainAndClose(resp)
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		out = append(out, m.Name)
	}
	return out, nil
}
