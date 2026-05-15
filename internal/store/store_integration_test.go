//go:build integration

package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/rdegges/redline/internal/errs"
)

func openTempDB(t *testing.T) *DB {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(context.Background(), p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newRun(id, site string) Run {
	return Run{
		ID:            id,
		SiteURL:       site,
		PromptsSHA256: "sha-" + id,
		ConfigJSON:    `{}`,
		LLMProvider:   "ollama",
		LLMModel:      "qwen3:30b",
		Version:       "0.0.0-test",
		Status:        RunRunning,
		PID:           0,
	}
}

func TestRuns_Insert_Succeeds(t *testing.T) {
	db := openTempDB(t)
	if err := db.InsertRun(context.Background(), newRun("r1", "http://x")); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestRuns_Insert_DuplicateActiveReturnsErrDuplicate(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	if err := db.InsertRun(ctx, newRun("r1", "http://x")); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := db.InsertRun(ctx, Run{
		ID: "r2", SiteURL: "http://x", PromptsSHA256: "sha-r1",
		ConfigJSON: "{}", LLMProvider: "ollama", LLMModel: "qwen3:30b",
		Version: "v", Status: RunRunning,
	})
	if !errors.Is(err, errs.ErrDuplicateRun) {
		t.Fatalf("expected ErrDuplicateRun, got %v", err)
	}
}

func TestRuns_FindActive_FreshHeartbeat(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	if err := db.InsertRun(ctx, newRun("r1", "http://x")); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := db.FindActiveRun(ctx, "http://x", "sha-r1", 90*time.Second)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil || got.ID != "r1" {
		t.Fatalf("expected r1, got %+v", got)
	}
}

func TestRuns_UpdateStatus_CompletesRun(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	if err := db.UpdateRunStatus(ctx, "r1", RunCompleted, ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := db.LatestRun(ctx, "http://x")
	if got.Status != RunCompleted {
		t.Fatalf("status = %s, want %s", got.Status, RunCompleted)
	}
}

func TestURLs_EnqueueAndClaim(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	if err := db.EnqueueURL(ctx, URL{URL: "http://x/", RunID: "r1", DiscoveredVia: "homepage"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	got, err := db.ClaimURL(ctx, "r1", "w1", time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got == nil || got.URL != "http://x/" {
		t.Fatalf("expected claimed url, got %+v", got)
	}
	if got.Status != URLClaimed {
		t.Fatalf("status = %s", got.Status)
	}
}

func TestURLs_ClaimEmptyReturnsNil(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	got, err := db.ClaimURL(ctx, "r1", "w1", time.Minute)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestURLs_RecoverStaleClaims(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	_ = db.EnqueueURL(ctx, URL{URL: "http://x/", RunID: "r1", DiscoveredVia: "homepage"})
	_, _ = db.ClaimURL(ctx, "r1", "w1", -time.Minute) // already expired
	n, err := db.RecoverStaleURLClaims(ctx, "r1")
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered=%d want 1", n)
	}
}

func TestURLAliases_LookupCanonical(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertURLAlias(ctx, "http://x/old", "http://x/new", "canonical_tag")
	canon, ok, err := db.LookupCanonical(ctx, "http://x/old")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !ok || canon != "http://x/new" {
		t.Fatalf("got (%q, %v)", canon, ok)
	}
	_, ok, err = db.LookupCanonical(ctx, "http://x/missing")
	if err != nil || ok {
		t.Fatalf("expected miss, got (ok=%v, err=%v)", ok, err)
	}
}

func TestPages_UpsertIdempotent(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	p := Page{URL: "http://x/", FirstRunID: "r1", FinalURL: "http://x/", Title: "T", WordCount: 10}
	if err := db.UpsertPage(ctx, p); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	p.Title = "T2"
	if err := db.UpsertPage(ctx, p); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	got, err := db.GetPage(ctx, "http://x/")
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.Title != "T2" {
		t.Fatalf("Title = %s", got.Title)
	}
}

func TestClassifications_Lifecycle(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	_ = db.UpsertPage(ctx, Page{URL: "http://x/", FirstRunID: "r1", FinalURL: "http://x/"})
	if err := db.UpsertPendingClassification(ctx, "r1", "http://x/"); err != nil {
		t.Fatalf("upsert pending: %v", err)
	}
	got, err := db.ClaimClassification(ctx, "r1", "w1", time.Minute)
	if err != nil || got == nil {
		t.Fatalf("claim: %v %v", got, err)
	}
	if err := db.CompleteClassification(ctx, Classification{
		RunID: "r1", PageURL: "http://x/", PrimaryLabel: "Aligned",
		SecondaryLabels: "[]", AffectedPrompts: "[]", SuggestedAction: "KEEP",
		FindingsJSON: "[]", Confidence: 0.9, Rationale: "ok",
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	counts, err := db.CountClassificationsByLabel(ctx, "r1")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if counts["Aligned"] != 1 {
		t.Fatalf("counts=%v", counts)
	}
}

func TestEmbeddings_RoundTripFloat32Exact(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	_ = db.UpsertPage(ctx, Page{URL: "http://x/", FirstRunID: "r1", FinalURL: "http://x/"})
	vec := []float32{0.0, 1.0, -1.5, 3.14159, 1e-10}
	e := Embedding{RunID: "r1", PageURL: "http://x/", Provider: "ollama", Model: "nomic-embed-text", Dims: len(vec), Vector: vec}
	if err := db.UpsertEmbedding(ctx, e); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	rows, err := db.ListEmbeddings(ctx, "r1", "ollama", "nomic-embed-text")
	if err != nil || len(rows) != 1 {
		t.Fatalf("list: %v %v", rows, err)
	}
	for i := range vec {
		if rows[0].Vector[i] != vec[i] {
			t.Fatalf("vec[%d] = %v want %v", i, rows[0].Vector[i], vec[i])
		}
	}
}

func TestDuplicates_CheckConstraint(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	_ = db.UpsertPage(ctx, Page{URL: "http://x/a", FirstRunID: "r1", FinalURL: "http://x/a"})
	_ = db.UpsertPage(ctx, Page{URL: "http://x/b", FirstRunID: "r1", FinalURL: "http://x/b"})
	if err := db.InsertDuplicate(ctx, DuplicatePair{RunID: "r1", PageURLA: "http://x/b", PageURLB: "http://x/a", Similarity: 0.95, ClusterID: "cl_001"}); err == nil {
		t.Fatalf("expected lexicographic-order error")
	}
	if err := db.InsertDuplicate(ctx, DuplicatePair{RunID: "r1", PageURLA: "http://x/a", PageURLB: "http://x/b", Similarity: 0.95, ClusterID: "cl_001"}); err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestAPICalls_AttemptNumberPreserved(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	for i := 1; i <= 3; i++ {
		if err := db.InsertAPICall(ctx, APICall{
			RunID: "r1", Provider: "ollama", Operation: "chat",
			AttemptNumber: i, Succeeded: i == 3,
		}); err != nil {
			t.Fatalf("insert attempt %d: %v", i, err)
		}
	}
	retries, _ := db.CountRetries(ctx, "r1")
	if retries != 2 {
		t.Fatalf("retries=%d want 2", retries)
	}
}

func TestEvents_InsertAndList(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	if err := db.InsertEvent(ctx, "r1", "crawl", "info", "fetch.success", "http://x/", "ok", "w1", map[string]any{"latency_ms": 100}); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	rows, err := db.ListEvents(ctx, "r1", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("list events: %v %v", rows, err)
	}
	if rows[0].EventType != "fetch.success" {
		t.Fatalf("got %q", rows[0].EventType)
	}
}

func TestMigrate_IdempotentOnReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	ctx := context.Background()
	db1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	_ = db1.Close()
	db2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer db2.Close()
	row := db2.QueryRow(`SELECT COUNT(*) FROM schema_migrations`)
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if n != 1 {
		t.Fatalf("migrations rows = %d, want 1", n)
	}
}

func TestReports_StoreAndGet(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	body := []byte(`{"hello": "world"}`)
	if err := db.StoreReport(ctx, "r1", "json", body); err != nil {
		t.Fatalf("store: %v", err)
	}
	got, err := db.GetReport(ctx, "r1", "json")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("mismatch")
	}
}

// Sanity: ensure schema_migrations is populated.
func TestMigrate_RecordsVersion(t *testing.T) {
	db := openTempDB(t)
	row := db.QueryRow(`SELECT version FROM schema_migrations`)
	var v int
	if err := row.Scan(&v); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if v != 1 {
		t.Fatalf("version=%d want 1", v)
	}
}

// Ensure sql.NullString fields stay clean.
var _ = sql.NullString{}
