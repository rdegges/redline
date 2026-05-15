package store

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
)

// Embedding mirrors the embeddings row plus a parsed []float32.
type Embedding struct {
	RunID    string
	PageURL  string
	Provider string
	Model    string
	Dims     int
	Vector   []float32
}

// UpsertEmbedding stores a vector keyed by (run, page, provider, model).
func (db *DB) UpsertEmbedding(ctx context.Context, e Embedding) error {
	if len(e.Vector) != e.Dims {
		return fmt.Errorf("dims mismatch: declared=%d actual=%d", e.Dims, len(e.Vector))
	}
	blob := encodeFloat32(e.Vector)
	_, err := db.ExecContext(ctx, `
        INSERT INTO embeddings(run_id, page_url, provider, model, dims, vector, embed_status)
        VALUES (?, ?, ?, ?, ?, ?, 'done')
        ON CONFLICT(run_id, page_url, provider, model) DO UPDATE SET
            dims = excluded.dims, vector = excluded.vector,
            embed_status = 'done', error = NULL`,
		e.RunID, e.PageURL, e.Provider, e.Model, e.Dims, blob,
	)
	return err
}

// ListEmbeddings returns all 'done' embeddings for (runID, provider, model).
// Sorted by page_url ASC for deterministic iteration.
func (db *DB) ListEmbeddings(ctx context.Context, runID, provider, model string) ([]Embedding, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT run_id, page_url, provider, model, dims, vector
        FROM embeddings
        WHERE run_id=? AND provider=? AND model=? AND embed_status='done'
        ORDER BY page_url ASC`, runID, provider, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Embedding
	for rows.Next() {
		var e Embedding
		var blob []byte
		if err := rows.Scan(&e.RunID, &e.PageURL, &e.Provider, &e.Model, &e.Dims, &blob); err != nil {
			return nil, err
		}
		e.Vector = decodeFloat32(blob, e.Dims)
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkEmbeddingsPending creates pending rows for every page in pages,
// keyed by the given provider/model. Idempotent.
func (db *DB) MarkEmbeddingsPending(ctx context.Context, runID, provider, model string, pages []string) error {
	for _, p := range pages {
		_, err := db.ExecContext(ctx, `
            INSERT OR IGNORE INTO embeddings(run_id, page_url, provider, model, dims, vector, embed_status)
            VALUES (?, ?, ?, ?, 0, x'', 'pending')`, runID, p, provider, model)
		if err != nil {
			return err
		}
	}
	return nil
}

func encodeFloat32(v []float32) []byte {
	buf := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func decodeFloat32(buf []byte, dims int) []float32 {
	out := make([]float32, dims)
	for i := 0; i < dims && (i+1)*4 <= len(buf); i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return out
}
