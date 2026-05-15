package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Page mirrors the pages table.
type Page struct {
	URL             string
	FirstRunID      string
	FinalURL        string
	Title           string
	MetaDescription string
	BodyText        string
	WordCount       int
	IsEmptyShell    bool
	Truncated       bool
	StatusCode      int
	ContentType     string
	LastModified    sql.NullString
	PublishedDate   sql.NullString
	FetchedAt       sql.NullTime
	RawBodySHA256   string
	BodySizeBytes   int
	FetchError      string
}

// UpsertPage inserts or replaces a pages row keyed by URL.
func (db *DB) UpsertPage(ctx context.Context, p Page) error {
	_, err := db.ExecContext(ctx, `
        INSERT INTO pages(
            url, first_run_id, final_url, title, meta_description, body_text,
            word_count, is_empty_shell, truncated, status_code, content_type,
            last_modified, published_date, fetched_at, raw_body_sha256,
            body_size_bytes, fetch_error)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), ?, ?, NULLIF(?, ''))
        ON CONFLICT(url) DO UPDATE SET
            final_url=excluded.final_url, title=excluded.title,
            meta_description=excluded.meta_description, body_text=excluded.body_text,
            word_count=excluded.word_count, is_empty_shell=excluded.is_empty_shell,
            truncated=excluded.truncated, status_code=excluded.status_code,
            content_type=excluded.content_type, last_modified=excluded.last_modified,
            published_date=excluded.published_date, fetched_at=datetime('now'),
            raw_body_sha256=excluded.raw_body_sha256, body_size_bytes=excluded.body_size_bytes,
            fetch_error=excluded.fetch_error`,
		p.URL, p.FirstRunID, p.FinalURL, p.Title, p.MetaDescription, p.BodyText,
		p.WordCount, boolInt(p.IsEmptyShell), boolInt(p.Truncated), p.StatusCode,
		p.ContentType, sqlNullStr(p.LastModified), sqlNullStr(p.PublishedDate),
		p.RawBodySHA256, p.BodySizeBytes, p.FetchError,
	)
	if err != nil {
		return fmt.Errorf("upsert page: %w", err)
	}
	return nil
}

// GetPage returns the pages row for url, or nil if not present.
func (db *DB) GetPage(ctx context.Context, url string) (*Page, error) {
	row := db.QueryRowContext(ctx, `
        SELECT url, first_run_id, final_url, COALESCE(title,''),
               COALESCE(meta_description,''), COALESCE(body_text,''),
               word_count, is_empty_shell, truncated, COALESCE(status_code,0),
               COALESCE(content_type,''), last_modified, published_date,
               fetched_at, COALESCE(raw_body_sha256,''), body_size_bytes,
               COALESCE(fetch_error,'')
        FROM pages WHERE url = ?`, url)
	var p Page
	var emptyShell, truncated int
	var fetchedAt sql.NullString
	if err := row.Scan(
		&p.URL, &p.FirstRunID, &p.FinalURL, &p.Title,
		&p.MetaDescription, &p.BodyText, &p.WordCount,
		&emptyShell, &truncated, &p.StatusCode, &p.ContentType,
		&p.LastModified, &p.PublishedDate, &fetchedAt,
		&p.RawBodySHA256, &p.BodySizeBytes, &p.FetchError,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.IsEmptyShell = emptyShell != 0
	p.Truncated = truncated != 0
	if fetchedAt.Valid {
		t, _ := parseSQLiteTime(fetchedAt.String)
		p.FetchedAt = sql.NullTime{Valid: true, Time: t}
	}
	return &p, nil
}

// ListPagesByRun returns all pages associated with a run, ordered by URL.
func (db *DB) ListPagesByRun(ctx context.Context, runID string) ([]Page, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT p.url, p.first_run_id, p.final_url, COALESCE(p.title,''),
               COALESCE(p.meta_description,''), COALESCE(p.body_text,''),
               p.word_count, p.is_empty_shell, p.truncated,
               COALESCE(p.status_code,0), COALESCE(p.content_type,''),
               p.last_modified, p.published_date, p.fetched_at,
               COALESCE(p.raw_body_sha256,''), p.body_size_bytes,
               COALESCE(p.fetch_error,'')
        FROM pages p
        INNER JOIN urls u ON u.url = p.url
        WHERE u.run_id = ? AND u.status = 'fetched'
        ORDER BY p.url ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Page
	for rows.Next() {
		var p Page
		var emptyShell, truncated int
		var fetchedAt sql.NullString
		if err := rows.Scan(
			&p.URL, &p.FirstRunID, &p.FinalURL, &p.Title,
			&p.MetaDescription, &p.BodyText, &p.WordCount,
			&emptyShell, &truncated, &p.StatusCode, &p.ContentType,
			&p.LastModified, &p.PublishedDate, &fetchedAt,
			&p.RawBodySHA256, &p.BodySizeBytes, &p.FetchError,
		); err != nil {
			return nil, err
		}
		p.IsEmptyShell = emptyShell != 0
		p.Truncated = truncated != 0
		if fetchedAt.Valid {
			t, _ := parseSQLiteTime(fetchedAt.String)
			p.FetchedAt = sql.NullTime{Valid: true, Time: t}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// InboundLinkCounts returns inbound internal link counts per page URL.
func (db *DB) InboundLinkCounts(ctx context.Context, runID string) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT l.to_url, COUNT(*)
        FROM links l
        INNER JOIN urls u ON u.url = l.from_url
        WHERE u.run_id = ? AND l.is_internal = 1
        GROUP BY l.to_url`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var url string
		var n int
		if err := rows.Scan(&url, &n); err != nil {
			return nil, err
		}
		out[url] = n
	}
	return out, rows.Err()
}

// InsertLink writes one row to the links graph. INSERT OR IGNORE so
// repeated discoveries don't duplicate edges.
func (db *DB) InsertLink(ctx context.Context, fromURL, toURL, anchor string, internal bool) error {
	_, err := db.ExecContext(ctx, `
        INSERT OR IGNORE INTO links(from_url, to_url, anchor, is_internal)
        VALUES (?, ?, ?, ?)`, fromURL, toURL, anchor, boolInt(internal))
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var _ = time.Time{}
