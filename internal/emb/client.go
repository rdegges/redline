// Package emb defines the EmbeddingClient interface plus Ollama / OpenAI /
// Voyage implementations. Used by internal/embed for Pass 2
// duplicate detection.
package emb

import "context"

// EmbeddingClient is implemented by every provider.
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	Provider() string
	Model() string
	Dims() int
}
