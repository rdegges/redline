package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// EventRow mirrors an events row.
type EventRow struct {
	ID          int64
	RunID       sql.NullString
	Timestamp   string
	Phase       string
	Severity    string
	EventType   string
	URL         sql.NullString
	Message     string
	PayloadJSON sql.NullString
	WorkerID    sql.NullString
}

// InsertEvent persists a single events row. Used by the dual-sink log
// handler via internal/app's EventSink implementation.
func (db *DB) InsertEvent(ctx context.Context, runID, phase, severity, eventType, url, msg, workerID string, payload map[string]any) error {
	var payloadStr any
	if len(payload) > 0 {
		b, err := json.Marshal(payload)
		if err == nil {
			payloadStr = string(b)
		}
	}
	_, err := db.ExecContext(ctx, `
        INSERT INTO events(run_id, phase, severity, event_type, url, message, payload_json, worker_id)
        VALUES (NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), ?, ?, NULLIF(?, ''))`,
		runID, phase, severity, eventType, url, msg, payloadStr, workerID,
	)
	return err
}

// ListEvents returns events for a run ordered by timestamp ASC, id ASC.
func (db *DB) ListEvents(ctx context.Context, runID string, levelFilter string) ([]EventRow, error) {
	q := `SELECT id, run_id, timestamp, phase, severity, event_type, url,
                 message, payload_json, worker_id
          FROM events WHERE run_id = ?`
	args := []any{runID}
	if levelFilter != "" {
		q += ` AND severity = ?`
		args = append(args, levelFilter)
	}
	q += ` ORDER BY timestamp ASC, id ASC`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventRow
	for rows.Next() {
		var e EventRow
		if err := rows.Scan(&e.ID, &e.RunID, &e.Timestamp, &e.Phase, &e.Severity,
			&e.EventType, &e.URL, &e.Message, &e.PayloadJSON, &e.WorkerID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountEventTypes returns a map of event_type -> count.
func (db *DB) CountEventTypes(ctx context.Context, runID string) (map[string]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT event_type, COUNT(*) FROM events WHERE run_id=? GROUP BY event_type`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}
