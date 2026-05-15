package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rdegges/redline/internal/store"
)

func TestSuggestNextAction_Statuses(t *testing.T) {
	dbPath := "/tmp/x.db"
	cases := []struct {
		name        string
		status      string
		errMsg      string
		heartbeat   time.Time
		wantContain string
	}{
		{
			name:        "completed run",
			status:      store.RunCompleted,
			wantContain: "Re-render the report",
		},
		{
			name:        "failed run with error message",
			status:      store.RunFailed,
			errMsg:      "ollama refused connection",
			wantContain: "ollama refused connection",
		},
		{
			name:        "failed run without error message",
			status:      store.RunFailed,
			wantContain: "Run failed.",
		},
		{
			name:        "aborted run",
			status:      store.RunAborted,
			wantContain: "--resume",
		},
		{
			name:        "paused on budget",
			status:      store.RunPausedBudget,
			wantContain: "--resume",
		},
		{
			name:        "paused by user",
			status:      store.RunPausedUser,
			wantContain: "--resume",
		},
		{
			name:        "paused on provider auth",
			status:      store.RunPausedProviderAuth,
			wantContain: "API key",
		},
		{
			name:        "running with fresh heartbeat",
			status:      store.RunRunning,
			heartbeat:   time.Now().Add(-30 * time.Second),
			wantContain: "No action needed",
		},
		{
			name:        "running with stale heartbeat",
			status:      store.RunRunning,
			heartbeat:   time.Now().Add(-10 * time.Minute),
			wantContain: "heartbeat is stale",
		},
		{
			name:        "unknown status",
			status:      "weird-state",
			wantContain: "weird-state",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &store.Run{
				ID:              "abc",
				Status:          tc.status,
				LastHeartbeatAt: tc.heartbeat,
			}
			if tc.errMsg != "" {
				r.Error = sql.NullString{Valid: true, String: tc.errMsg}
			}
			got := suggestNextAction(r, dbPath)
			if !strings.Contains(got, tc.wantContain) {
				t.Errorf("suggestNextAction = %q, want substring %q", got, tc.wantContain)
			}
		})
	}
}

func TestSuggestNextAction_NilRun(t *testing.T) {
	got := suggestNextAction(nil, "/tmp/x.db")
	if !strings.Contains(got, "No runs found") {
		t.Errorf("nil run = %q, want 'No runs found'", got)
	}
}

func TestWriteRunInspection_NoRunFound(t *testing.T) {
	var buf bytes.Buffer
	writeRunInspection(&buf, &runInspection{NextActionMessage: "No runs found in the database."})
	out := buf.String()
	for _, want := range []string{"(no run found)", "No runs found in the database."} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nGot:\n%s", want, out)
		}
	}
}

func TestWriteRunInspection_WithRun(t *testing.T) {
	var buf bytes.Buffer
	r := &store.Run{
		ID:              "01HXX",
		SiteURL:         "https://example.com",
		Status:          store.RunCompleted,
		LLMProvider:     "ollama",
		LLMModel:        "qwen3:30b",
		StartedAt:       time.Now().Add(-time.Hour),
		LastHeartbeatAt: time.Now().Add(-time.Minute),
		CompletedAt:     sql.NullTime{Valid: true, Time: time.Now().Add(-5 * time.Minute)},
	}
	writeRunInspection(&buf, &runInspection{
		Run:               r,
		URLsByStatus:      map[string]int{"fetched": 12, "failed": 1},
		LabelCounts:       map[string]int{"Aligned": 8, "Stale": 4},
		APICalls:          15,
		Retries:           2,
		NextActionMessage: "Run is complete.",
	})
	out := buf.String()
	for _, want := range []string{
		"Run inspection: 01HXX",
		"https://example.com",
		"status",
		"completed",
		"ollama / qwen3:30b",
		"urls:",
		"fetched",
		"12",
		"classifications by label:",
		"Aligned",
		"api_calls",
		"15",
		"retries",
		"Suggested next action:",
		"Run is complete.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nGot:\n%s", want, out)
		}
	}
	// urls map keys must be sorted
	if i, j := strings.Index(out, "failed"), strings.Index(out, "fetched"); i < 0 || j < 0 || i > j {
		t.Errorf("expected URL statuses sorted alphabetically; got\n%s", out)
	}
}

func TestSortedKeys_OrdersAlphabetically(t *testing.T) {
	got := sortedKeys(map[string]int{"banana": 1, "apple": 2, "cherry": 3})
	want := []string{"apple", "banana", "cherry"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortedKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestInspectRun_NoSuchRunID(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	// Bootstrap an empty DB so Open + Migrate succeed.
	db, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Close()

	ins, err := inspectRun(context.Background(), dbPath, "nope-not-here")
	if err != nil {
		t.Fatalf("inspectRun: %v", err)
	}
	if ins.Run != nil {
		t.Fatalf("expected nil run for missing ID, got %v", ins.Run)
	}
	if !strings.Contains(ins.NextActionMessage, "not found") {
		t.Errorf("NextActionMessage = %q, want 'not found'", ins.NextActionMessage)
	}
}

func TestInspectRun_LatestEmptyDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Close()

	ins, err := inspectRun(context.Background(), dbPath, "latest")
	if err != nil {
		t.Fatalf("inspectRun: %v", err)
	}
	if ins.Run != nil {
		t.Fatalf("expected nil run on empty DB")
	}
	if !strings.Contains(ins.NextActionMessage, "No runs found") {
		t.Errorf("NextActionMessage = %q, want 'No runs found'", ins.NextActionMessage)
	}
}

func TestWriteRunInspectionJSON_ErrorCase(t *testing.T) {
	var buf bytes.Buffer
	writeRunInspectionJSON(&buf, nil, errors.New("open db: no such file"))
	var rec runInspectionJSON
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("unmarshal: %v\nGot: %s", err, buf.String())
	}
	if rec.Kind != "run_inspection" {
		t.Errorf("kind = %q, want run_inspection", rec.Kind)
	}
	if !strings.Contains(rec.Error, "open db") {
		t.Errorf("error = %q, want substring 'open db'", rec.Error)
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("output not newline-terminated: %q", buf.String())
	}
}

func TestWriteRunInspectionJSON_NoRunFound(t *testing.T) {
	var buf bytes.Buffer
	writeRunInspectionJSON(&buf, &runInspection{NextActionMessage: "Run \"abc\" not found in the database."}, nil)
	var rec runInspectionJSON
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("unmarshal: %v\nGot: %s", err, buf.String())
	}
	if rec.Kind != "run_inspection" {
		t.Errorf("kind = %q, want run_inspection", rec.Kind)
	}
	if !strings.Contains(rec.Message, "not found") {
		t.Errorf("message = %q, want substring 'not found'", rec.Message)
	}
	if rec.RunID != "" {
		t.Errorf("run_id should be empty when no run found, got %q", rec.RunID)
	}
}

func TestWriteRunInspectionJSON_WithRun(t *testing.T) {
	var buf bytes.Buffer
	started := time.Date(2026, 5, 14, 16, 23, 15, 0, time.UTC)
	heartbeat := time.Date(2026, 5, 14, 16, 35, 11, 0, time.UTC)
	completed := time.Date(2026, 5, 14, 17, 0, 0, 0, time.UTC)
	r := &store.Run{
		ID:              "01HXYZ",
		SiteURL:         "https://example.com",
		Status:          store.RunCompleted,
		LLMProvider:     "ollama",
		LLMModel:        "qwen3:30b",
		StartedAt:       started,
		LastHeartbeatAt: heartbeat,
		CompletedAt:     sql.NullTime{Valid: true, Time: completed},
	}
	writeRunInspectionJSON(&buf, &runInspection{
		Run:               r,
		URLsByStatus:      map[string]int{"fetched": 12, "failed": 1},
		LabelCounts:       map[string]int{"Aligned": 8, "Stale": 4},
		APICalls:          15,
		Retries:           2,
		NextActionMessage: "Run is complete. Re-render the report at any time.",
	}, nil)
	var rec runInspectionJSON
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("unmarshal: %v\nGot: %s", err, buf.String())
	}
	if rec.RunID != "01HXYZ" {
		t.Errorf("run_id = %q, want 01HXYZ", rec.RunID)
	}
	if rec.Status != store.RunCompleted {
		t.Errorf("status = %q, want %q", rec.Status, store.RunCompleted)
	}
	if rec.StartedAt != "2026-05-14T16:23:15Z" {
		t.Errorf("started_at = %q, want RFC3339 UTC", rec.StartedAt)
	}
	if rec.CompletedAt != "2026-05-14T17:00:00Z" {
		t.Errorf("completed_at = %q, want RFC3339 UTC", rec.CompletedAt)
	}
	if rec.LLMModel != "qwen3:30b" {
		t.Errorf("llm_model = %q, want qwen3:30b", rec.LLMModel)
	}
	if rec.URLsByStatus["fetched"] != 12 || rec.URLsByStatus["failed"] != 1 {
		t.Errorf("urls_by_status missing entries: %v", rec.URLsByStatus)
	}
	if rec.ClassificationsByLabel["Aligned"] != 8 {
		t.Errorf("classifications_by_label.Aligned = %d, want 8", rec.ClassificationsByLabel["Aligned"])
	}
	if rec.APICalls != 15 || rec.Retries != 2 {
		t.Errorf("counters wrong: api_calls=%d retries=%d", rec.APICalls, rec.Retries)
	}
	if !strings.Contains(rec.SuggestedAction, "Re-render the report") {
		t.Errorf("suggested_action = %q", rec.SuggestedAction)
	}
}

func TestWriteRunInspectionJSON_NilInspection(t *testing.T) {
	var buf bytes.Buffer
	writeRunInspectionJSON(&buf, nil, nil)
	var rec runInspectionJSON
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("unmarshal: %v\nGot: %s", err, buf.String())
	}
	if rec.Message == "" {
		t.Errorf("expected fallback message for nil inspection, got empty")
	}
}

func TestInspectRun_WithRealRun(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.InsertRun(ctx, store.Run{
		ID:            "01HABC",
		SiteURL:       "https://acme.example",
		PromptsSHA256: "deadbeef",
		ConfigJSON:    "{}",
		LLMProvider:   "ollama",
		LLMModel:      "qwen3:30b",
		Version:       "0.0.0-test",
		Status:        store.RunRunning,
		PID:           42,
	}); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	db.Close()

	ins, err := inspectRun(ctx, dbPath, "latest")
	if err != nil {
		t.Fatalf("inspectRun: %v", err)
	}
	if ins.Run == nil {
		t.Fatalf("expected a run, got nil")
	}
	if ins.Run.ID != "01HABC" {
		t.Errorf("run ID = %q, want %q", ins.Run.ID, "01HABC")
	}
	if ins.Run.Status != store.RunRunning {
		t.Errorf("status = %q, want %q", ins.Run.Status, store.RunRunning)
	}
}
