package store

import (
	"context"
	"fmt"
)

// DuplicatePair mirrors a duplicates row.
type DuplicatePair struct {
	RunID      string
	PageURLA   string
	PageURLB   string
	Similarity float64
	ClusterID  string
}

// InsertDuplicate writes one duplicates row. Page URLs must satisfy
// page_url_a < page_url_b (CHECK constraint).
func (db *DB) InsertDuplicate(ctx context.Context, d DuplicatePair) error {
	if !(d.PageURLA < d.PageURLB) {
		return fmt.Errorf("page_url_a must be lex < page_url_b: %q vs %q", d.PageURLA, d.PageURLB)
	}
	_, err := db.ExecContext(ctx, `
        INSERT OR IGNORE INTO duplicates(run_id, page_url_a, page_url_b, similarity, cluster_id)
        VALUES (?, ?, ?, ?, ?)`,
		d.RunID, d.PageURLA, d.PageURLB, d.Similarity, d.ClusterID)
	return err
}

// ListDuplicates returns every duplicate pair for runID, ordered by cluster
// and then by URL.
func (db *DB) ListDuplicates(ctx context.Context, runID string) ([]DuplicatePair, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT run_id, page_url_a, page_url_b, similarity, cluster_id
        FROM duplicates WHERE run_id=?
        ORDER BY cluster_id ASC, page_url_a ASC, page_url_b ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DuplicatePair
	for rows.Next() {
		var d DuplicatePair
		if err := rows.Scan(&d.RunID, &d.PageURLA, &d.PageURLB, &d.Similarity, &d.ClusterID); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteDuplicatesForRun wipes the duplicates table for a run before
// the dedup pass re-computes clusters.
func (db *DB) DeleteDuplicatesForRun(ctx context.Context, runID string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM duplicates WHERE run_id = ?`, runID)
	return err
}
