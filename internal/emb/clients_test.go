package emb

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rdegges/redline/internal/errs"
)

func TestFakeClient_DeterministicAndUnit(t *testing.T) {
	f := NewFake()
	v1, _ := f.Embed(context.Background(), "hello")
	v2, _ := f.Embed(context.Background(), "hello")
	if len(v1) != f.DimCount || len(v2) != f.DimCount {
		t.Fatal("wrong dims")
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("non-deterministic at %d", i)
		}
	}
	var sq float64
	for _, x := range v1 {
		sq += float64(x) * float64(x)
	}
	if math.Abs(sq-1.0) > 0.001 {
		t.Fatalf("expected unit length, got %v", sq)
	}
}

func TestOllamaClient_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req embedRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !req.Truncate {
			t.Errorf("expected truncate=true")
		}
		_ = json.NewEncoder(w).Encode(embedResponse{Embeddings: [][]float32{{0.1, 0.2, 0.3}}})
	}))
	defer srv.Close()
	c := NewOllama(srv.URL, "nomic-embed-text", time.Second, "30m")
	v, err := c.Embed(context.Background(), "hello")
	if err != nil || len(v) != 3 {
		t.Fatalf("v=%v err=%v", v, err)
	}
}

func TestOllamaClient_ModelMissing404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer srv.Close()
	c := NewOllama(srv.URL, "x", time.Second, "0")
	_, err := c.Embed(context.Background(), "h")
	if !errors.Is(err, errs.ErrOllamaModelMissing) {
		t.Fatalf("got %v", err)
	}
}

func TestOpenAIClient_AuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer srv.Close()
	c := NewOpenAI("key", "m", time.Second)
	c.Endpoint = srv.URL
	_, err := c.Embed(context.Background(), "h")
	if !errors.Is(err, errs.ErrAuthFailed) {
		t.Fatalf("got %v", err)
	}
}

func TestVoyageClient_Happy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(voyageResp{Data: []struct {
			Embedding []float32 `json:"embedding"`
		}{{Embedding: []float32{0.5}}}})
	}))
	defer srv.Close()
	c := NewVoyage("key", "voyage-3-large", time.Second)
	c.Endpoint = srv.URL
	v, err := c.Embed(context.Background(), "h")
	if err != nil || len(v) != 1 {
		t.Fatalf("v=%v err=%v", v, err)
	}
}
