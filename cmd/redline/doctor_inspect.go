package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rdegges/redline/internal/store"
)

// runInspection is the post-mortem state for one redline run.
type runInspection struct {
	Run               *store.Run
	URLsByStatus      map[string]int
	LabelCounts       map[string]int
	APICalls          int
	Retries           int
	NextActionMessage string
}

// inspectRun loads the run identified by runArg ("latest" or an explicit
// ID) from dbPath and gathers the counters needed to suggest a next
// action. Returns an error only on I/O / DB failures; "no such run" is
// reported via NextActionMessage.
func inspectRun(ctx context.Context, dbPath, runArg string) (*runInspection, error) {
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db %s: %w", dbPath, err)
	}
	defer db.Close()

	var r *store.Run
	switch runArg {
	case "", "latest":
		r, err = db.LatestRun(ctx, "")
	default:
		r, err = db.GetRun(ctx, runArg)
	}
	if err != nil {
		return nil, fmt.Errorf("read run: %w", err)
	}
	if r == nil {
		msg := "No runs found in the database."
		if runArg != "" && runArg != "latest" {
			msg = fmt.Sprintf("Run %q not found in the database.", runArg)
		}
		return &runInspection{NextActionMessage: msg}, nil
	}

	urls, err := db.CountURLsByStatus(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("count urls: %w", err)
	}
	labels, err := db.CountClassificationsByLabel(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("count classifications: %w", err)
	}
	apiCalls, err := db.CountAPICalls(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("count api_calls: %w", err)
	}
	retries, err := db.CountRetries(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("count retries: %w", err)
	}

	return &runInspection{
		Run:               r,
		URLsByStatus:      urls,
		LabelCounts:       labels,
		APICalls:          apiCalls,
		Retries:           retries,
		NextActionMessage: suggestNextAction(r, dbPath),
	}, nil
}

// suggestNextAction maps a run's current status into a one-paragraph
// human-readable recommendation. Pure function so it's easy to test.
func suggestNextAction(r *store.Run, dbPath string) string {
	if r == nil {
		return "No runs found in the database."
	}
	switch r.Status {
	case store.RunCompleted:
		return "Run is complete. Re-render the report at any time with:\n  redline report --db " + dbPath
	case store.RunFailed:
		msg := "Run failed."
		if r.Error.Valid && r.Error.String != "" {
			msg += " Last error: " + r.Error.String
		}
		msg += "\nInspect the events table or re-run with --log-level=debug to capture more detail."
		return msg
	case store.RunAborted:
		return "Run was aborted before completion. Resume with:\n  redline scan --resume --db " + dbPath
	case store.RunPausedBudget:
		return "Run paused on budget. Resume by raising --max-pages and re-running:\n  redline scan --resume --db " + dbPath
	case store.RunPausedUser:
		return "Run paused by user. Resume with:\n  redline scan --resume --db " + dbPath
	case store.RunPausedProviderAuth:
		return "Run paused on provider auth failure. Verify your API key and resume with:\n  redline scan --resume --db " + dbPath
	case store.RunRunning:
		stale := time.Since(r.LastHeartbeatAt) > 5*time.Minute
		if stale {
			return "Run is marked 'running' but the heartbeat is stale (>5m).\n" +
				"It likely crashed mid-scan. Resume with:\n  redline scan --resume --db " + dbPath
		}
		return "Run is active. No action needed — let it finish, or Ctrl-C and resume later with `--resume`."
	default:
		return "Run is in state " + r.Status + ". Re-run with --log-level=debug to capture more detail."
	}
}

// writeRunInspection prints the inspection table beneath the doctor
// component table. Layout mirrors writeDoctorTable for visual consistency.
func writeRunInspection(out io.Writer, ins *runInspection) {
	if ins == nil {
		return
	}
	fmt.Fprintln(out, "-----------------------------------------")
	if ins.Run == nil {
		fmt.Fprintln(out, "Run inspection: (no run found)")
		fmt.Fprintln(out, "-----------------------------------------")
		fmt.Fprintln(out, ins.NextActionMessage)
		return
	}
	r := ins.Run
	fmt.Fprintf(out, "Run inspection: %s\n", r.ID)
	fmt.Fprintf(out, "  site                             %s\n", r.SiteURL)
	fmt.Fprintln(out, "-----------------------------------------")
	rows := []struct {
		k, v string
	}{
		{"status", r.Status},
		{"started", formatTimeAgo(r.StartedAt)},
		{"last heartbeat", formatTimeAgo(r.LastHeartbeatAt)},
		{"llm provider/model", r.LLMProvider + " / " + r.LLMModel},
	}
	if r.CompletedAt.Valid {
		rows = append(rows, struct{ k, v string }{"completed", formatTimeAgo(r.CompletedAt.Time)})
	}
	for _, row := range rows {
		fmt.Fprintf(out, "  %-32s %s\n", row.k, row.v)
	}
	if len(ins.URLsByStatus) > 0 {
		fmt.Fprintln(out, "  urls:")
		for _, k := range sortedKeys(ins.URLsByStatus) {
			fmt.Fprintf(out, "    %-30s %d\n", k, ins.URLsByStatus[k])
		}
	}
	if len(ins.LabelCounts) > 0 {
		fmt.Fprintln(out, "  classifications by label:")
		for _, k := range sortedKeys(ins.LabelCounts) {
			fmt.Fprintf(out, "    %-30s %d\n", k, ins.LabelCounts[k])
		}
	}
	fmt.Fprintf(out, "  %-32s %d\n", "api_calls", ins.APICalls)
	fmt.Fprintf(out, "  %-32s %d\n", "retries", ins.Retries)
	fmt.Fprintln(out, "-----------------------------------------")
	fmt.Fprintln(out, "Suggested next action:")
	for _, line := range strings.Split(ins.NextActionMessage, "\n") {
		fmt.Fprintf(out, "  %s\n", line)
	}
}

// runInspectionJSON is the wire format emitted by `doctor --json --run`.
// One NDJSON line, alongside the per-check lines from writeDoctorJSON.
type runInspectionJSON struct {
	Kind                   string         `json:"kind"`
	Error                  string         `json:"error,omitempty"`
	Message                string         `json:"message,omitempty"`
	RunID                  string         `json:"run_id,omitempty"`
	Site                   string         `json:"site,omitempty"`
	Status                 string         `json:"status,omitempty"`
	StartedAt              string         `json:"started_at,omitempty"`
	LastHeartbeatAt        string         `json:"last_heartbeat_at,omitempty"`
	CompletedAt            string         `json:"completed_at,omitempty"`
	LLMProvider            string         `json:"llm_provider,omitempty"`
	LLMModel               string         `json:"llm_model,omitempty"`
	URLsByStatus           map[string]int `json:"urls_by_status,omitempty"`
	ClassificationsByLabel map[string]int `json:"classifications_by_label,omitempty"`
	APICalls               int            `json:"api_calls,omitempty"`
	Retries                int            `json:"retries,omitempty"`
	SuggestedAction        string         `json:"suggested_action,omitempty"`
}

// writeRunInspectionJSON serializes a runInspection as a single NDJSON
// line. inspectErr is the error returned by inspectRun (if any), so
// callers can pass through the original failure without re-wrapping.
func writeRunInspectionJSON(out io.Writer, ins *runInspection, inspectErr error) {
	rec := runInspectionJSON{Kind: "run_inspection"}
	switch {
	case inspectErr != nil:
		rec.Error = inspectErr.Error()
	case ins == nil || ins.Run == nil:
		if ins != nil {
			rec.Message = ins.NextActionMessage
		} else {
			rec.Message = "No run inspection available."
		}
	default:
		r := ins.Run
		rec.RunID = r.ID
		rec.Site = r.SiteURL
		rec.Status = r.Status
		rec.StartedAt = formatRFC3339(r.StartedAt)
		rec.LastHeartbeatAt = formatRFC3339(r.LastHeartbeatAt)
		if r.CompletedAt.Valid {
			rec.CompletedAt = formatRFC3339(r.CompletedAt.Time)
		}
		rec.LLMProvider = r.LLMProvider
		rec.LLMModel = r.LLMModel
		rec.URLsByStatus = ins.URLsByStatus
		rec.ClassificationsByLabel = ins.LabelCounts
		rec.APICalls = ins.APICalls
		rec.Retries = ins.Retries
		rec.SuggestedAction = ins.NextActionMessage
	}
	b, err := json.Marshal(rec)
	if err != nil {
		// Marshal cannot realistically fail for this struct; degrade
		// to an error record rather than panicking.
		fmt.Fprintf(out, `{"kind":"run_inspection","error":%q}`+"\n", err.Error())
		return
	}
	fmt.Fprintln(out, string(b))
}

func formatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "(never)"
	}
	ago := time.Since(t).Truncate(time.Second)
	return t.UTC().Format("2006-01-02 15:04:05 UTC") + " (" + ago.String() + " ago)"
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Insertion-sort: tiny maps, no allocation pressure.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
