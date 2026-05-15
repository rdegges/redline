package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rdegges/redline/internal/errs"
)

// Run is the application-level representation of a runs row.
type Run struct {
	ID                string
	StartedAt         time.Time
	CompletedAt       sql.NullTime
	SiteURL           string
	PromptsSHA256     string
	ConfigJSON        string
	LLMProvider       string
	LLMModel          string
	EmbeddingProvider sql.NullString
	EmbeddingModel    sql.NullString
	Version           string
	Status            string
	Error             sql.NullString
	PID               int
	LastHeartbeatAt   time.Time
}

// RunStatus enumerates legal run statuses; keep in sync with the CHECK
// constraint in migration 0001.
const (
	RunRunning            = "running"
	RunCompleted          = "completed"
	RunFailed             = "failed"
	RunPausedBudget       = "paused_budget"
	RunPausedUser         = "paused_user"
	RunPausedProviderAuth = "paused_provider_auth"
	RunAborted            = "aborted"
)

// InsertRun inserts a new run. If an active row already exists for the
// same (site_url, prompts_yaml_sha256), the partial unique index makes
// SQLite return a constraint error and we surface ErrDuplicateRun.
func (db *DB) InsertRun(ctx context.Context, r Run) error {
	_, err := db.ExecContext(ctx, `
        INSERT INTO runs(id, site_url, prompts_yaml_sha256, config_json,
            llm_provider, llm_model, embedding_provider, embedding_model,
            redline_version, status, pid)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.SiteURL, r.PromptsSHA256, r.ConfigJSON,
		r.LLMProvider, r.LLMModel,
		nullableString(r.EmbeddingProvider), nullableString(r.EmbeddingModel),
		r.Version, r.Status, r.PID,
	)
	if err != nil {
		// SQLite reports UNIQUE violations as "constraint failed".
		if isConstraint(err) {
			return errs.ErrDuplicateRun
		}
		return fmt.Errorf("insert run: %w", err)
	}
	return nil
}

// FindActiveRun returns the most recent active run for (site, promptsSHA256)
// whose heartbeat is within staleAfter, or nil if none.
func (db *DB) FindActiveRun(ctx context.Context, site, promptsSHA string, staleAfter time.Duration) (*Run, error) {
	row := db.QueryRowContext(ctx, `
        SELECT id, started_at, completed_at, site_url, prompts_yaml_sha256,
               config_json, llm_provider, llm_model, embedding_provider,
               embedding_model, redline_version, status, error, pid, last_heartbeat_at
        FROM runs
        WHERE site_url = ? AND prompts_yaml_sha256 = ?
          AND status IN ('running','paused_user','paused_budget','paused_provider_auth')
        ORDER BY started_at DESC LIMIT 1`,
		site, promptsSHA,
	)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if time.Since(r.LastHeartbeatAt) > staleAfter {
		return nil, nil
	}
	return r, nil
}

// GetRun returns the run row for id, or nil if not found.
func (db *DB) GetRun(ctx context.Context, id string) (*Run, error) {
	row := db.QueryRowContext(ctx, `
        SELECT id, started_at, completed_at, site_url, prompts_yaml_sha256,
               config_json, llm_provider, llm_model, embedding_provider,
               embedding_model, redline_version, status, error, pid, last_heartbeat_at
        FROM runs WHERE id = ?`, id)
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// LatestRun returns the most recent run for the given site (or any site
// if site is empty). Used by `report` and `diagnose` when --run is "latest".
func (db *DB) LatestRun(ctx context.Context, site string) (*Run, error) {
	var (
		row *sql.Row
	)
	if site == "" {
		row = db.QueryRowContext(ctx, `
            SELECT id, started_at, completed_at, site_url, prompts_yaml_sha256,
                   config_json, llm_provider, llm_model, embedding_provider,
                   embedding_model, redline_version, status, error, pid, last_heartbeat_at
            FROM runs ORDER BY started_at DESC LIMIT 1`)
	} else {
		row = db.QueryRowContext(ctx, `
            SELECT id, started_at, completed_at, site_url, prompts_yaml_sha256,
                   config_json, llm_provider, llm_model, embedding_provider,
                   embedding_model, redline_version, status, error, pid, last_heartbeat_at
            FROM runs WHERE site_url = ? ORDER BY started_at DESC LIMIT 1`, site)
	}
	r, err := scanRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// UpdateRunStatus transitions a run to status. completedAt is set when
// status is "completed" or "failed".
func (db *DB) UpdateRunStatus(ctx context.Context, id, status, errStr string) error {
	completedAt := sql.NullString{}
	switch status {
	case RunCompleted, RunFailed, RunAborted:
		completedAt = sql.NullString{Valid: true, String: time.Now().UTC().Format("2006-01-02 15:04:05")}
	}
	_, err := db.ExecContext(ctx, `
        UPDATE runs SET status = ?, error = NULLIF(?, ''),
            completed_at = COALESCE(?, completed_at)
        WHERE id = ?`,
		status, errStr, sqlNull(completedAt), id,
	)
	if err != nil {
		return fmt.Errorf("update run status: %w", err)
	}
	return nil
}

// Heartbeat updates last_heartbeat_at to now. Called by the heartbeat
// goroutine every 30s.
func (db *DB) Heartbeat(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx,
		`UPDATE runs SET last_heartbeat_at = datetime('now') WHERE id = ?`, id)
	return err
}

func scanRun(row interface {
	Scan(dest ...any) error
}) (*Run, error) {
	var (
		r           Run
		startedAt   string
		completedAt sql.NullString
		heartbeat   string
	)
	if err := row.Scan(
		&r.ID, &startedAt, &completedAt, &r.SiteURL, &r.PromptsSHA256,
		&r.ConfigJSON, &r.LLMProvider, &r.LLMModel,
		&r.EmbeddingProvider, &r.EmbeddingModel,
		&r.Version, &r.Status, &r.Error, &r.PID, &heartbeat,
	); err != nil {
		return nil, err
	}
	r.StartedAt, _ = parseSQLiteTime(startedAt)
	if completedAt.Valid {
		t, _ := parseSQLiteTime(completedAt.String)
		r.CompletedAt = sql.NullTime{Valid: true, Time: t}
	}
	r.LastHeartbeatAt, _ = parseSQLiteTime(heartbeat)
	return &r, nil
}

func parseSQLiteTime(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05.000",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %q", s)
}

func nullableString(s sql.NullString) any {
	if s.Valid {
		return s.String
	}
	return nil
}

func sqlNull(s sql.NullString) any {
	if s.Valid {
		return s.String
	}
	return nil
}

// isConstraint returns true if err looks like a SQLite constraint
// violation (UNIQUE, CHECK, FOREIGN KEY).
func isConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "constraint") || contains(msg, "UNIQUE")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
