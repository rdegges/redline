package emb

import (
	"context"
	"hash/fnv"
	"math"
)

// FakeClient returns deterministic embeddings derived from input text.
// Used by tests + e2e dry run.
type FakeClient struct {
	ProviderName string
	ModelName    string
	DimCount     int
}

// NewFake returns a FakeClient with sensible defaults.
func NewFake() *FakeClient {
	return &FakeClient{ProviderName: "ollama", ModelName: "nomic-embed-text", DimCount: 8}
}

// Embed hashes text into a stable pseudo-vector.
func (f *FakeClient) Embed(_ context.Context, text string) ([]float32, error) {
	v := make([]float32, f.DimCount)
	for i := 0; i < f.DimCount; i++ {
		h := fnv.New64a()
		_, _ = h.Write([]byte{byte(i)})
		_, _ = h.Write([]byte(text))
		v[i] = float32(math.Sin(float64(h.Sum64() % 1000)))
	}
	return normalize(v), nil
}

func (f *FakeClient) Provider() string { return f.ProviderName }
func (f *FakeClient) Model() string    { return f.ModelName }
func (f *FakeClient) Dims() int        { return f.DimCount }

func normalize(v []float32) []float32 {
	var sumSq float32
	for _, x := range v {
		sumSq += x * x
	}
	if sumSq == 0 {
		return v
	}
	mag := float32(math.Sqrt(float64(sumSq)))
	for i := range v {
		v[i] /= mag
	}
	return v
}
