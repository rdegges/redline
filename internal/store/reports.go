package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
)

// StoreReport persists rendered report bytes per (run_id, format).
func (db *DB) StoreReport(ctx context.Context, runID, format string, content []byte) error {
	sum := sha256.Sum256(content)
	_, err := db.ExecContext(ctx, `
        INSERT INTO reports(run_id, format, sha256, content)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(run_id, format) DO UPDATE SET
            sha256 = excluded.sha256,
            content = excluded.content,
            rendered_at = datetime('now')`,
		runID, format, hex.EncodeToString(sum[:]), content,
	)
	return err
}

// GetReport returns the stored report content for (runID, format).
func (db *DB) GetReport(ctx context.Context, runID, format string) ([]byte, error) {
	row := db.QueryRowContext(ctx,
		`SELECT content FROM reports WHERE run_id=? AND format=?`, runID, format)
	var c []byte
	if err := row.Scan(&c); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}
