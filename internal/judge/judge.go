// Package judge orchestrates the LLM call for every page. It validates
// the LLM's JSON response against the schema plus the six
// cross-field invariants, and persists the result through internal/store.
package judge

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rdegges/redline/internal/config"
	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/llm"
	logevt "github.com/rdegges/redline/internal/log"
	"github.com/rdegges/redline/internal/store"
)

//go:embed prompt.tmpl
var promptTemplate string

//go:embed response.schema.json
var responseSchema []byte

// PromptTemplate returns the embedded prompt template.
func PromptTemplate() string { return promptTemplate }

// ResponseSchema returns the embedded JSON schema bytes for use by
// provider clients in JSON-mode requests.
func ResponseSchema() []byte { return responseSchema }

// Judge runs the per-page judgment loop until all `pages` rows have a
// completed classifications row.
type Judge struct {
	Client      llm.LLMClient
	DB          *store.DB
	Logger      *slog.Logger
	RunID       string
	Concurrency int
	Cfg         *config.File
	Model       string
	Provider    string // "ollama" | "anthropic"; used for api_calls accounting
	ThinThresh  int
}

// Run drains the classifications queue for the given run.
func (j *Judge) Run(ctx context.Context) error {
	if j.Concurrency <= 0 {
		j.Concurrency = 2
	}
	// Seed pending rows for every fetched page.
	pages, err := j.DB.ListPagesByRun(ctx, j.RunID)
	if err != nil {
		return err
	}
	for _, p := range pages {
		if err := j.DB.UpsertPendingClassification(ctx, j.RunID, p.URL); err != nil {
			return err
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < j.Concurrency; i++ {
		wg.Add(1)
		workerID := fmt.Sprintf("judge-%d", i)
		go func(id string) {
			defer wg.Done()
			j.worker(ctx, id, pages)
		}(workerID)
	}
	wg.Wait()
	return nil
}

func (j *Judge) worker(ctx context.Context, workerID string, pages []store.Page) {
	pageByURL := map[string]store.Page{}
	for _, p := range pages {
		pageByURL[p.URL] = p
	}
	for ctx.Err() == nil {
		c, err := j.DB.ClaimClassification(ctx, j.RunID, workerID, 5*time.Minute)
		if err != nil {
			j.Logger.Warn("classification claim error",
				slog.String("event_type", logevt.JudgeFailedPermanent),
				slog.String("phase", "judge"),
				slog.String("error", err.Error()),
			)
			return
		}
		if c == nil {
			return
		}
		page, ok := pageByURL[c.PageURL]
		if !ok {
			_ = j.DB.FailClassification(ctx, j.RunID, c.PageURL, store.JudgeFailed, "page not found")
			continue
		}
		if page.IsEmptyShell || page.WordCount < j.ThinThresh {
			j.autoLabelThin(ctx, page)
			continue
		}
		j.judgePage(ctx, workerID, page)
	}
}

// judgePage calls the LLM with the per-page retry-with-reprompt schedule.
func (j *Judge) judgePage(ctx context.Context, workerID string, page store.Page) {
	start := time.Now()
	attempts := 3
	var (
		resp    *llm.JudgeResponse
		lastErr error
		attempt int
	)
	for attempt = 1; attempt <= attempts; attempt++ {
		req := llm.JudgeRequest{
			Model:              j.Model,
			PageURL:            page.URL,
			PageTitle:          page.Title,
			PageBody:           page.BodyText,
			Prompts:            j.Cfg.Prompts,
			CanonicalMessaging: j.Cfg.CanonicalMessaging,
			IncludeFewShot:     attempt == 3,
			TruncatedBody:      attempt >= 2,
		}
		if attempt >= 2 && len(page.BodyText) > 1000 {
			req.PageBody = page.BodyText[:1000]
		}
		r, err := j.Client.Judge(ctx, req)
		if err != nil {
			lastErr = err
			if errors.Is(err, errs.ErrAuthFailed) {
				_ = j.DB.FailClassification(ctx, j.RunID, page.URL, store.JudgeFailed, err.Error())
				return
			}
			continue
		}
		if err := j.validateInvariants(r, page, j.Cfg); err != nil {
			lastErr = err
			j.Logger.Warn("schema invalid",
				slog.String("event_type", logevt.JudgeSchemaInvalid),
				slog.String("phase", "judge"),
				slog.String("url", page.URL),
				slog.Int("attempt", attempt),
				slog.String("error", err.Error()),
			)
			continue
		}
		resp = r
		break
	}
	latency := time.Since(start)
	if resp == nil {
		_ = j.DB.FailClassification(ctx, j.RunID, page.URL, store.JudgeFailedSchema, fmt.Sprintf("3 schema-invalid retries: %v", lastErr))
		return
	}
	j.persist(ctx, page, resp, attempt, latency, workerID)
}

func (j *Judge) autoLabelThin(ctx context.Context, page store.Page) {
	body := page.BodyText
	if len(body) > 100 {
		body = body[:100]
	}
	findings := []llm.Finding{{
		ID:           "f1",
		Kind:         "thin_content",
		Severity:     "high",
		QuotedText:   body,
		LocationHint: "entire page",
		Issue:        fmt.Sprintf("Page extracted to fewer than %d words of substantive content.", j.ThinThresh),
		SuggestedFix: "Delete this page or replace it with substantive content.",
	}}
	c := buildClassification(j.RunID, page.URL, &llm.JudgeResponse{
		PrimaryLabel:    "Thin",
		Confidence:      1.0,
		SuggestedAction: "DELETE",
		PageSummary:     llm.PageSummary{CurrentFocus: "Empty or near-empty page", ShouldFocusOn: nil},
		Findings:        findings,
		Rationale:       "Auto-labeled Thin without an LLM call because the extracted body was below the threshold. This is a load-bearing optimization: empty pages and SPA shells should not consume LLM budget. Delete the page or fill it with real content.",
	})
	_ = j.DB.CompleteClassification(ctx, c)
}

// validateInvariants enforces the six cross-field invariants from.
// Where a violation is repairable (degenerate finding lists, out-of-range
// confidence, conflicting MANUAL_REVIEW + high confidence), the function
// auto-corrects in place and returns nil. Only structural violations that
// indicate the model misunderstood the schema (e.g., Aligned with findings,
// UPDATE without an edit_plan) return an error to trigger a retry.
func (j *Judge) validateInvariants(r *llm.JudgeResponse, page store.Page, cfg *config.File) error {
	// Confidence sanity: clamp to [0, 1]. Some models return 0.95 as 95
	// (percentage-shaped). Treat anything in (10, 100] as percentage and
	// divide by 100; anything else above 1 is just clamped.
	if r.Confidence > 10.0 && r.Confidence <= 100.0 {
		r.Confidence = r.Confidence / 100.0
	}
	if r.Confidence < 0 {
		r.Confidence = 0
	}
	if r.Confidence > 1 {
		r.Confidence = 1
	}
	// (1) Aligned implies empty findings, no edit_plan, KEEP action.
	if r.PrimaryLabel == "Aligned" {
		if len(r.Findings) != 0 || r.EditPlan != nil || r.SuggestedAction != "KEEP" || r.PageSummary.ShouldFocusOn != nil {
			return fmt.Errorf("aligned invariant violated")
		}
	}
	// (2) UPDATE/REWRITE require edit_plan and ≥1 finding.
	if r.SuggestedAction == "UPDATE" || r.SuggestedAction == "REWRITE" {
		if r.EditPlan == nil || len(r.Findings) == 0 {
			return fmt.Errorf("update/rewrite requires edit_plan and ≥1 finding")
		}
	}
	// (3) DELETE requires ≥1 finding and no edit_plan.
	if r.SuggestedAction == "DELETE" {
		if len(r.Findings) == 0 || r.EditPlan != nil {
			return fmt.Errorf("delete requires findings and no edit_plan")
		}
	}
	// (4) MANUAL_REVIEW requires confidence < 0.6. Auto-cap rather than
	// retry: the model has explicitly said "human should decide" via
	// suggested_action, so its self-reported confidence is the unreliable
	// field. Pegging to 0.59 preserves the model's intent and removes the
	// invariant tension without wasting two more retries (which only
	// change the prompt, not the sampler — won't recalibrate confidence).
	if r.SuggestedAction == "MANUAL_REVIEW" && r.Confidence >= 0.6 {
		r.Confidence = 0.59
	}
	// (5) Every quoted_text must be a substring of page.BodyText.
	filtered := r.Findings[:0]
	for _, f := range r.Findings {
		if f.QuotedText == "" || strings.Contains(page.BodyText, f.QuotedText) {
			filtered = append(filtered, f)
		}
	}
	if len(r.Findings) > 0 && len(filtered) == 0 {
		return fmt.Errorf("all quoted_text passages failed substring check")
	}
	r.Findings = filtered
	// (6) affected_prompts must be known ids.
	knownIDs := map[string]bool{}
	for _, p := range cfg.Prompts {
		knownIDs[p.ID] = true
	}
	keep := r.AffectedPrompts[:0]
	for _, id := range r.AffectedPrompts {
		if knownIDs[id] {
			keep = append(keep, id)
		}
	}
	r.AffectedPrompts = keep
	return nil
}

func (j *Judge) persist(ctx context.Context, page store.Page, resp *llm.JudgeResponse, attempts int, latency time.Duration, workerID string) {
	c := buildClassification(j.RunID, page.URL, resp)
	c.AttemptCount = attempts
	c.LatencyMs = int(latency.Milliseconds())
	c.InputTokens = resp.InputTokens
	c.OutputTokens = resp.OutputTokens
	c.CacheHitTokens = resp.CacheHitTokens
	if len(resp.RawResponse) > 0 {
		c.RawResponse = string(resp.RawResponse)
	}
	if err := j.DB.CompleteClassification(ctx, c); err != nil {
		j.Logger.Error("complete classification",
			slog.String("event_type", logevt.JudgeFailedPermanent),
			slog.String("phase", "judge"),
			slog.String("url", page.URL),
			slog.String("error", err.Error()),
		)
		return
	}
	// every attempt should produce an api_calls row.
	// We only have the final attempt's token counts here; record those
	// against the final attempt_number so the token aggregator (and the
	// report's total_input_tokens / total_output_tokens fields) work.
	provider := j.Provider
	if provider == "" {
		provider = "ollama"
	}
	if err := j.DB.InsertAPICall(ctx, store.APICall{
		RunID:         j.RunID,
		Provider:      provider,
		Operation:     "chat",
		Model:         j.Model,
		PageURL:       page.URL,
		AttemptNumber: attempts,
		InputTokens:   resp.InputTokens,
		CachedTokens:  resp.CacheHitTokens,
		OutputTokens:  resp.OutputTokens,
		HTTPStatus:    200,
		Succeeded:     true,
		LatencyMs:     int(latency.Milliseconds()),
	}); err != nil {
		j.Logger.Warn("api_calls insert",
			slog.String("event_type", logevt.JudgeSuccess),
			slog.String("phase", "judge"),
			slog.String("url", page.URL),
			slog.String("error", err.Error()),
		)
	}
	j.Logger.Debug("judge success",
		slog.String("event_type", logevt.JudgeSuccess),
		slog.String("phase", "judge"),
		slog.String("url", page.URL),
		slog.String("primary_label", resp.PrimaryLabel),
		slog.String("worker_id", workerID),
	)
}

func buildClassification(runID, pageURL string, r *llm.JudgeResponse) store.Classification {
	secondaryJSON, _ := json.Marshal(r.SecondaryLabels)
	if r.SecondaryLabels == nil {
		secondaryJSON = []byte("[]")
	}
	affectedJSON, _ := json.Marshal(r.AffectedPrompts)
	if r.AffectedPrompts == nil {
		affectedJSON = []byte("[]")
	}
	findingsJSON, _ := json.Marshal(r.Findings)
	if r.Findings == nil {
		findingsJSON = []byte("[]")
	}
	c := store.Classification{
		RunID:           runID,
		PageURL:         pageURL,
		PrimaryLabel:    r.PrimaryLabel,
		SecondaryLabels: string(secondaryJSON),
		Confidence:      r.Confidence,
		Rationale:       r.Rationale,
		AffectedPrompts: string(affectedJSON),
		SuggestedAction: r.SuggestedAction,
		FindingsJSON:    string(findingsJSON),
	}
	if r.EditPlan != nil {
		b, _ := json.Marshal(r.EditPlan)
		c.EditPlanJSON.Valid = true
		c.EditPlanJSON.String = string(b)
	}
	summaryJSON, _ := json.Marshal(r.PageSummary)
	c.PageSummaryJSON.Valid = true
	c.PageSummaryJSON.String = string(summaryJSON)
	return c
}
