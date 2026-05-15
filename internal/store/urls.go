package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// URL is the application-level representation of a urls row.
type URL struct {
	URL            string
	RunID          string
	DiscoveredVia  string
	DiscoveredFrom sql.NullString
	Depth          int
	Status         string
	ClaimedBy      sql.NullString
	ClaimExpiresAt sql.NullTime
	AttemptCount   int
	NextRetryAt    sql.NullTime
	LastError      sql.NullString
	DiscoveredAt   time.Time
	FetchedAt      sql.NullTime
	FinalURL       sql.NullString
	StatusCode     sql.NullInt64
}

// URL statuses (CHECK constraint in migration).
const (
	URLDiscovered = "discovered"
	URLClaimed    = "claimed"
	URLFetching   = "fetching"
	URLFetched    = "fetched"
	URLFailed     = "failed"
	URLSkipped    = "skipped"
)

// EnqueueURL inserts a urls row with status='discovered'. INSERT OR
// IGNORE so callers can naively call this for every discovered link.
func (db *DB) EnqueueURL(ctx context.Context, u URL) error {
	_, err := db.ExecContext(ctx, `
        INSERT OR IGNORE INTO urls(url, run_id, discovered_via, discovered_from, depth, status)
        VALUES (?, ?, ?, ?, ?, 'discovered')`,
		u.URL, u.RunID, u.DiscoveredVia, sqlNullStr(u.DiscoveredFrom), u.Depth,
	)
	if err != nil {
		return fmt.Errorf("enqueue url: %w", err)
	}
	return nil
}

// ClaimURL atomically pops a `discovered` URL (or a `failed` URL whose
// next_retry_at has passed) for the given worker, returning nil if the
// queue is empty.
func (db *DB) ClaimURL(ctx context.Context, runID, workerID string, ttl time.Duration) (*URL, error) {
	expires := time.Now().Add(ttl).UTC().Format("2006-01-02 15:04:05")
	row := db.QueryRowContext(ctx, `
        UPDATE urls
        SET status='claimed', claimed_by=?, claim_expires_at=?, attempt_count = attempt_count + 1
        WHERE url = (
            SELECT url FROM urls
            WHERE run_id = ?
              AND (status='discovered'
                   OR (status='failed' AND next_retry_at IS NOT NULL AND next_retry_at <= datetime('now')))
            ORDER BY depth ASC, discovered_at ASC LIMIT 1
        )
        RETURNING url, run_id, discovered_via, discovered_from, depth, status,
                  claimed_by, claim_expires_at, attempt_count, next_retry_at,
                  last_error, discovered_at, fetched_at, final_url, status_code`,
		workerID, expires, runID,
	)
	u, err := scanURL(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return u, err
}

// RecoverStaleURLClaims resets claimed/fetching rows whose claim has
// expired back to 'discovered' so another worker can pick them up.
func (db *DB) RecoverStaleURLClaims(ctx context.Context, runID string) (int64, error) {
	res, err := db.ExecContext(ctx, `
        UPDATE urls SET status='discovered', claimed_by=NULL, claim_expires_at=NULL
        WHERE run_id = ?
          AND status IN ('claimed','fetching')
          AND claim_expires_at IS NOT NULL
          AND claim_expires_at <= datetime('now')`, runID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// MarkURLFetched transitions a urls row to 'fetched'.
func (db *DB) MarkURLFetched(ctx context.Context, url, finalURL string, code int) error {
	_, err := db.ExecContext(ctx, `
        UPDATE urls SET status='fetched', fetched_at=datetime('now'),
            final_url=?, status_code=?, claimed_by=NULL, claim_expires_at=NULL,
            last_error=NULL, next_retry_at=NULL
        WHERE url = ?`, finalURL, code, url)
	return err
}

// MarkURLFailed transitions a urls row to 'failed' with last_error.
func (db *DB) MarkURLFailed(ctx context.Context, url, lastErr string, nextRetryAt sql.NullTime) error {
	var next any
	if nextRetryAt.Valid {
		next = nextRetryAt.Time.UTC().Format("2006-01-02 15:04:05")
	}
	_, err := db.ExecContext(ctx, `
        UPDATE urls SET status='failed', last_error=?, next_retry_at=?,
            claimed_by=NULL, claim_expires_at=NULL
        WHERE url = ?`, lastErr, next, url)
	return err
}

// MarkURLSkipped sets status='skipped' with an explanatory error string.
func (db *DB) MarkURLSkipped(ctx context.Context, url, reason string) error {
	_, err := db.ExecContext(ctx, `
        UPDATE urls SET status='skipped', last_error=?, claimed_by=NULL, claim_expires_at=NULL
        WHERE url = ?`, reason, url)
	return err
}

// CountURLsByStatus is used by report.summary and progress logging.
func (db *DB) CountURLsByStatus(ctx context.Context, runID string) (map[string]int, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT status, COUNT(*) FROM urls WHERE run_id = ? GROUP BY status`, runID)
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

// InsertURLAlias records a canonical/alias mapping. INSERT OR IGNORE so
// the first alias wins.
func (db *DB) InsertURLAlias(ctx context.Context, alias, canonical, kind string) error {
	_, err := db.ExecContext(ctx, `
        INSERT OR IGNORE INTO url_aliases(alias_url, canonical_url, alias_type)
        VALUES (?, ?, ?)`, alias, canonical, kind)
	return err
}

// LookupCanonical returns the canonical URL for alias, if known.
func (db *DB) LookupCanonical(ctx context.Context, alias string) (string, bool, error) {
	row := db.QueryRowContext(ctx,
		`SELECT canonical_url FROM url_aliases WHERE alias_url = ?`, alias)
	var canon string
	if err := row.Scan(&canon); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return canon, true, nil
}

func scanURL(row interface{ Scan(...any) error }) (*URL, error) {
	var u URL
	var discoveredAt string
	var fetchedAt, claimExpires, nextRetry sql.NullString
	if err := row.Scan(
		&u.URL, &u.RunID, &u.DiscoveredVia, &u.DiscoveredFrom, &u.Depth, &u.Status,
		&u.ClaimedBy, &claimExpires, &u.AttemptCount, &nextRetry,
		&u.LastError, &discoveredAt, &fetchedAt, &u.FinalURL, &u.StatusCode,
	); err != nil {
		return nil, err
	}
	u.DiscoveredAt, _ = parseSQLiteTime(discoveredAt)
	if fetchedAt.Valid {
		t, _ := parseSQLiteTime(fetchedAt.String)
		u.FetchedAt = sql.NullTime{Valid: true, Time: t}
	}
	if claimExpires.Valid {
		t, _ := parseSQLiteTime(claimExpires.String)
		u.ClaimExpiresAt = sql.NullTime{Valid: true, Time: t}
	}
	if nextRetry.Valid {
		t, _ := parseSQLiteTime(nextRetry.String)
		u.NextRetryAt = sql.NullTime{Valid: true, Time: t}
	}
	return &u, nil
}

func sqlNullStr(s sql.NullString) any {
	if s.Valid {
		return s.String
	}
	return nil
}
