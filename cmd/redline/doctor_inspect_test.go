package main

import (
	"bytes"
	"context"
	"database/sql"
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
