//go:build integration

package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestURLs_MarkFailedAndSkipped(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	_ = db.EnqueueURL(ctx, URL{URL: "http://x/a", RunID: "r1", DiscoveredVia: "homepage"})
	_, _ = db.ClaimURL(ctx, "r1", "w1", time.Minute)
	next := time.Now().Add(5 * time.Minute)
	if err := db.MarkURLFailed(ctx, "http://x/a", "boom", sql.NullTime{Valid: true, Time: next}); err != nil {
		t.Fatalf("MarkURLFailed: %v", err)
	}
	if err := db.MarkURLSkipped(ctx, "http://x/a", "skipped"); err != nil {
		t.Fatalf("MarkURLSkipped: %v", err)
	}
	counts, err := db.CountURLsByStatus(ctx, "r1")
	if err != nil {
		t.Fatalf("CountURLsByStatus: %v", err)
	}
	if counts[URLSkipped] != 1 {
		t.Fatalf("counts: %v", counts)
	}
}

func TestRuns_Heartbeat(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	if err := db.Heartbeat(ctx, "r1"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
}

func TestStore_CountEventTypes_RecoverStaleClassification(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	_ = db.UpsertPage(ctx, Page{URL: "http://x/a", FirstRunID: "r1", FinalURL: "http://x/a"})
	_ = db.UpsertPendingClassification(ctx, "r1", "http://x/a")
	_, _ = db.ClaimClassification(ctx, "r1", "w1", -time.Minute)
	n, err := db.RecoverStaleClassificationClaims(ctx, "r1")
	if err != nil {
		t.Fatalf("recover stale: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered=%d want 1", n)
	}
	_ = db.InsertEvent(ctx, "r1", "judge", "info", "judge.success", "http://x/a", "ok", "w1", nil)
	cs, err := db.CountEventTypes(ctx, "r1")
	if err != nil {
		t.Fatalf("CountEventTypes: %v", err)
	}
	if cs["judge.success"] != 1 {
		t.Fatalf("count: %+v", cs)
	}
}

func TestStore_MarkEmbeddingsPending(t *testing.T) {
	db := openTempDB(t)
	ctx := context.Background()
	_ = db.InsertRun(ctx, newRun("r1", "http://x"))
	_ = db.UpsertPage(ctx, Page{URL: "http://x/a", FirstRunID: "r1", FinalURL: "http://x/a"})
	if err := db.MarkEmbeddingsPending(ctx, "r1", "ollama", "nomic-embed-text", []string{"http://x/a"}); err != nil {
		t.Fatalf("MarkEmbeddingsPending: %v", err)
	}
}
