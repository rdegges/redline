package store

import "context"

// APICall mirrors an api_calls row.
type APICall struct {
	RunID         string
	Provider      string
	Operation     string
	Model         string
	PageURL       string
	AttemptNumber int
	InputTokens   int
	CachedTokens  int
	OutputTokens  int
	HTTPStatus    int
	Succeeded     bool
	Error         string
	LatencyMs     int
	RequestID     string
}

// InsertAPICall writes one api_calls row. One row per attempt.
func (db *DB) InsertAPICall(ctx context.Context, a APICall) error {
	_, err := db.ExecContext(ctx, `
        INSERT INTO api_calls(
            run_id, provider, operation, model, page_url, attempt_number,
            input_tokens, cached_tokens, output_tokens,
            http_status, succeeded, error, latency_ms, request_id)
        VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, NULLIF(?, 0), ?, NULLIF(?, ''), ?, NULLIF(?, ''))`,
		a.RunID, a.Provider, a.Operation, a.Model, a.PageURL, a.AttemptNumber,
		a.InputTokens, a.CachedTokens, a.OutputTokens,
		a.HTTPStatus, boolInt(a.Succeeded), a.Error, a.LatencyMs, a.RequestID,
	)
	return err
}

// SumAPICallTokens returns aggregate token counts for runID.
func (db *DB) SumAPICallTokens(ctx context.Context, runID string) (in, cached, out int, err error) {
	row := db.QueryRowContext(ctx, `
        SELECT COALESCE(SUM(input_tokens),0), COALESCE(SUM(cached_tokens),0),
               COALESCE(SUM(output_tokens),0)
        FROM api_calls WHERE run_id = ?`, runID)
	err = row.Scan(&in, &cached, &out)
	return
}

// CountAPICalls returns the number of rows for the given run.
func (db *DB) CountAPICalls(ctx context.Context, runID string) (int, error) {
	row := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_calls WHERE run_id=?`, runID)
	var n int
	err := row.Scan(&n)
	return n, err
}

// CountRetries returns the count of api_calls rows that were retries
// (attempt_number > 1).
func (db *DB) CountRetries(ctx context.Context, runID string) (int, error) {
	row := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_calls WHERE run_id=? AND attempt_number > 1`, runID)
	var n int
	err := row.Scan(&n)
	return n, err
}
