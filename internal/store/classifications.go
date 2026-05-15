package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Classification mirrors the classifications row.
type Classification struct {
	RunID           string
	PageURL         string
	JudgeStatus     string
	AttemptCount    int
	PrimaryLabel    string
	SecondaryLabels string // JSON-encoded []string
	Confidence      float64
	Rationale       string
	AffectedPrompts string // JSON-encoded []string
	SuggestedAction string
	FindingsJSON    string // JSON-encoded []Finding
	EditPlanJSON    sql.NullString
	PageSummaryJSON sql.NullString
	InputTokens     int
	OutputTokens    int
	CacheHitTokens  int
	LatencyMs       int
	RawResponse     string
	Error           string
	JudgedAt        sql.NullTime
}

const (
	JudgePending      = "pending"
	JudgeClaimed      = "claimed"
	JudgeJudging      = "judging"
	JudgeJudged       = "judged"
	JudgeFailed       = "failed"
	JudgeFailedSchema = "failed_schema"
)

// UpsertPendingClassification ensures a classifications row exists in
// 'pending' state for (runID, pageURL).
func (db *DB) UpsertPendingClassification(ctx context.Context, runID, pageURL string) error {
	_, err := db.ExecContext(ctx, `
        INSERT OR IGNORE INTO classifications(run_id, page_url, judge_status)
        VALUES (?, ?, 'pending')`, runID, pageURL)
	return err
}

// ClaimClassification atomically claims one 'pending' or stale row.
func (db *DB) ClaimClassification(ctx context.Context, runID, workerID string, ttl time.Duration) (*Classification, error) {
	expires := time.Now().Add(ttl).UTC().Format("2006-01-02 15:04:05")
	row := db.QueryRowContext(ctx, `
        UPDATE classifications
        SET judge_status='claimed', claimed_by=?, claim_expires_at=?,
            attempt_count = attempt_count + 1
        WHERE (run_id, page_url) = (
            SELECT run_id, page_url FROM classifications
            WHERE run_id=? AND judge_status='pending'
            ORDER BY page_url LIMIT 1
        )
        RETURNING run_id, page_url, judge_status, attempt_count,
                  COALESCE(primary_label,''), secondary_labels, COALESCE(confidence,0),
                  COALESCE(rationale,''), affected_prompts, COALESCE(suggested_action,''),
                  findings_json, edit_plan_json, page_summary_json,
                  input_tokens, output_tokens, cache_hit_tokens, latency_ms,
                  COALESCE(raw_response,''), COALESCE(error,''), judged_at`,
		workerID, expires, runID,
	)
	c, err := scanClassification(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

// CompleteClassification commits a successful judgment.
func (db *DB) CompleteClassification(ctx context.Context, c Classification) error {
	_, err := db.ExecContext(ctx, `
        UPDATE classifications SET judge_status='judged',
            primary_label=?, secondary_labels=?, confidence=?, rationale=?,
            affected_prompts=?, suggested_action=?,
            findings_json=?, edit_plan_json=?, page_summary_json=?,
            input_tokens=?, output_tokens=?, cache_hit_tokens=?, latency_ms=?,
            raw_response=?, error=NULL, judged_at=datetime('now'),
            claimed_by=NULL, claim_expires_at=NULL
        WHERE run_id=? AND page_url=?`,
		c.PrimaryLabel, c.SecondaryLabels, c.Confidence, c.Rationale,
		c.AffectedPrompts, c.SuggestedAction,
		c.FindingsJSON, sqlNullStr(c.EditPlanJSON), sqlNullStr(c.PageSummaryJSON),
		c.InputTokens, c.OutputTokens, c.CacheHitTokens, c.LatencyMs,
		c.RawResponse, c.RunID, c.PageURL,
	)
	if err != nil {
		return fmt.Errorf("complete classification: %w", err)
	}
	return nil
}

// FailClassification marks the classification as failed (or failed_schema).
func (db *DB) FailClassification(ctx context.Context, runID, pageURL, status, errStr string) error {
	_, err := db.ExecContext(ctx, `
        UPDATE classifications SET judge_status=?, error=?,
            primary_label='Unclear', confidence=0, suggested_action='MANUAL_REVIEW',
            judged_at=datetime('now'), claimed_by=NULL, claim_expires_at=NULL
        WHERE run_id=? AND page_url=?`, status, errStr, runID, pageURL)
	return err
}

// RecoverStaleClassificationClaims resets stale 'claimed'/'judging' rows.
func (db *DB) RecoverStaleClassificationClaims(ctx context.Context, runID string) (int64, error) {
	res, err := db.ExecContext(ctx, `
        UPDATE classifications SET judge_status='pending', claimed_by=NULL, claim_expires_at=NULL
        WHERE run_id=? AND judge_status IN ('claimed','judging')
          AND claim_expires_at IS NOT NULL AND claim_expires_at <= datetime('now')`, runID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListClassifications returns every classification for runID ordered by URL.
func (db *DB) ListClassifications(ctx context.Context, runID string) ([]Classification, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT run_id, page_url, judge_status, attempt_count,
               COALESCE(primary_label,''), secondary_labels, COALESCE(confidence,0),
               COALESCE(rationale,''), affected_prompts, COALESCE(suggested_action,''),
               findings_json, edit_plan_json, page_summary_json,
               input_tokens, output_tokens, cache_hit_tokens, latency_ms,
               COALESCE(raw_response,''), COALESCE(error,''), judged_at
        FROM classifications WHERE run_id=? ORDER BY page_url ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Classification
	for rows.Next() {
		c, err := scanClassification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// CountClassificationsByLabel returns counts grouped by primary_label.
func (db *DB) CountClassificationsByLabel(ctx context.Context, runID string) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT COALESCE(primary_label, ''), COUNT(*)
        FROM classifications WHERE run_id=? AND judge_status IN ('judged','failed','failed_schema')
        GROUP BY primary_label`, runID)
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

func scanClassification(row interface{ Scan(...any) error }) (*Classification, error) {
	var c Classification
	var judgedAt sql.NullString
	if err := row.Scan(
		&c.RunID, &c.PageURL, &c.JudgeStatus, &c.AttemptCount,
		&c.PrimaryLabel, &c.SecondaryLabels, &c.Confidence,
		&c.Rationale, &c.AffectedPrompts, &c.SuggestedAction,
		&c.FindingsJSON, &c.EditPlanJSON, &c.PageSummaryJSON,
		&c.InputTokens, &c.OutputTokens, &c.CacheHitTokens, &c.LatencyMs,
		&c.RawResponse, &c.Error, &judgedAt,
	); err != nil {
		return nil, err
	}
	if judgedAt.Valid {
		t, _ := parseSQLiteTime(judgedAt.String)
		c.JudgedAt = sql.NullTime{Valid: true, Time: t}
	}
	return &c, nil
}
