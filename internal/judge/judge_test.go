//go:build integration

package judge

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rdegges/redline/internal/config"
	"github.com/rdegges/redline/internal/llm"
	geoplog "github.com/rdegges/redline/internal/log"
	"github.com/rdegges/redline/internal/store"
)

func openDB(t *testing.T) *store.DB {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(context.Background(), p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.InsertRun(context.Background(), store.Run{
		ID: "r1", SiteURL: "http://x", PromptsSHA256: "sha",
		ConfigJSON: "{}", LLMProvider: "ollama", LLMModel: "qwen3:30b",
		Version: "v", Status: store.RunRunning,
	}); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	return db
}

func TestJudge_AlignedPage_PersistsKEEP(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	p := store.Page{URL: "http://x/", FirstRunID: "r1", FinalURL: "http://x/", BodyText: "Body text exceeds threshold.", WordCount: 60, Title: "T"}
	_ = db.UpsertPage(ctx, p)
	// Manually add a urls row so ListPagesByRun returns it.
	_ = db.EnqueueURL(ctx, store.URL{URL: p.URL, RunID: "r1", DiscoveredVia: "homepage"})
	_ = db.MarkURLFetched(ctx, p.URL, p.FinalURL, 200)

	fake := llm.NewFakeClient()
	fake.SetDefault(llm.JudgeResponse{
		PrimaryLabel:    "Aligned",
		Confidence:      0.91,
		SuggestedAction: "KEEP",
		PageSummary:     llm.PageSummary{CurrentFocus: "ok"},
		Findings:        []llm.Finding{},
		Rationale:       "Long enough rationale to satisfy the schema minimum length of fifty characters with margin.",
	})
	cfg := &config.File{Prompts: []config.Prompt{{ID: "a", Text: "x"}}}
	j := &Judge{Client: fake, DB: db, Logger: geoplog.Discard(), RunID: "r1", Concurrency: 1, Cfg: cfg, Model: "m", ThinThresh: 5}
	if err := j.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	cs, _ := db.ListClassifications(ctx, "r1")
	if len(cs) != 1 || cs[0].PrimaryLabel != "Aligned" {
		t.Fatalf("classifications: %+v", cs)
	}
}

func TestJudge_ThinPage_AutoLabeledWithoutLLM(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	p := store.Page{URL: "http://x/", FirstRunID: "r1", FinalURL: "http://x/", BodyText: "tiny", WordCount: 1, IsEmptyShell: true}
	_ = db.UpsertPage(ctx, p)
	_ = db.EnqueueURL(ctx, store.URL{URL: p.URL, RunID: "r1", DiscoveredVia: "homepage"})
	_ = db.MarkURLFetched(ctx, p.URL, p.FinalURL, 200)

	fake := llm.NewFakeClient() // no SetDefault — will fail if called
	cfg := &config.File{Prompts: []config.Prompt{{ID: "a", Text: "x"}}}
	j := &Judge{Client: fake, DB: db, Logger: geoplog.Discard(), RunID: "r1", Concurrency: 1, Cfg: cfg, Model: "m", ThinThresh: 50}
	if err := j.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	cs, _ := db.ListClassifications(ctx, "r1")
	if len(cs) != 1 || cs[0].PrimaryLabel != "Thin" {
		t.Fatalf("expected Thin auto-label, got %+v", cs)
	}
}

func TestJudge_InvariantViolation_Retries(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	p := store.Page{URL: "http://x/", FirstRunID: "r1", FinalURL: "http://x/", BodyText: "body content with enough length", WordCount: 60}
	_ = db.UpsertPage(ctx, p)
	_ = db.EnqueueURL(ctx, store.URL{URL: p.URL, RunID: "r1", DiscoveredVia: "homepage"})
	_ = db.MarkURLFetched(ctx, p.URL, p.FinalURL, 200)

	// Aligned label but findings — violates invariant (1).
	fake := llm.NewFakeClient()
	fake.SetDefault(llm.JudgeResponse{
		PrimaryLabel:    "Aligned",
		Confidence:      0.9,
		SuggestedAction: "KEEP",
		Findings:        []llm.Finding{{ID: "f1", Kind: "thin_content", Severity: "low", QuotedText: "body", LocationHint: "L", Issue: "i", SuggestedFix: "s"}},
		PageSummary:     llm.PageSummary{CurrentFocus: "ok"},
		Rationale:       "Long enough rationale to satisfy the schema minimum length of fifty characters with margin.",
	})
	cfg := &config.File{Prompts: []config.Prompt{{ID: "a", Text: "x"}}}
	j := &Judge{Client: fake, DB: db, Logger: geoplog.Discard(), RunID: "r1", Concurrency: 1, Cfg: cfg, Model: "m", ThinThresh: 5}
	_ = j.Run(ctx)
	cs, _ := db.ListClassifications(ctx, "r1")
	if cs[0].JudgeStatus != store.JudgeFailedSchema {
		t.Fatalf("expected failed_schema after retries, got %s", cs[0].JudgeStatus)
	}
}
