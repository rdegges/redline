package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rdegges/redline/internal/config"
	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/llm"
)

func fixtureReq() llm.JudgeRequest {
	return llm.JudgeRequest{
		Model:              "qwen3:30b",
		PageURL:            "http://x/",
		PageTitle:          "T",
		PageBody:           "Body content.",
		Prompts:            []config.Prompt{{ID: "a", Text: "What is X?"}},
		CanonicalMessaging: []config.MessagingBlock{{Title: "Canonical", Body: "msg"}},
	}
}

func TestOllama_Chat_HappyPath(t *testing.T) {
	canned := llm.JudgeResponse{
		PrimaryLabel:    "Aligned",
		Confidence:      0.9,
		SuggestedAction: "KEEP",
		PageSummary:     llm.PageSummary{CurrentFocus: "ok"},
		Findings:        []llm.Finding{},
		Rationale:       "Long enough rationale to satisfy the schema minimum of fifty characters in length.",
	}
	cb, _ := json.Marshal(canned)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Format != "json" {
			t.Errorf("expected format=json, got %q", req.Format)
		}
		if req.KeepAlive == "" {
			t.Errorf("expected keep_alive set")
		}
		w.Header().Set("Content-Type", "application/json")
		resp := chatResponse{Model: req.Model, Message: chatMessage{Role: "assistant", Content: string(cb)}, Done: true}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second, "30m")
	got, err := c.Judge(context.Background(), fixtureReq())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.PrimaryLabel != "Aligned" {
		t.Fatalf("label: %s", got.PrimaryLabel)
	}
}

func TestOllama_ModelMissing_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second, "30m")
	_, err := c.Judge(context.Background(), fixtureReq())
	if !errors.Is(err, errs.ErrOllamaModelMissing) {
		t.Fatalf("expected ErrOllamaModelMissing, got %v", err)
	}
}

func TestOllama_ModelLoading_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"model is loading"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second, "30m")
	_, err := c.Judge(context.Background(), fixtureReq())
	if !errors.Is(err, errs.ErrOllamaLoading) {
		t.Fatalf("expected ErrOllamaLoading, got %v", err)
	}
}

func TestOllama_ConnectionRefused(t *testing.T) {
	c := New("http://127.0.0.1:1", 200*time.Millisecond, "30m")
	_, err := c.Judge(context.Background(), fixtureReq())
	if !errors.Is(err, errs.ErrOllamaUnavailable) {
		t.Fatalf("expected ErrOllamaUnavailable, got %v", err)
	}
}

func TestOllama_InvalidJSON_BubblesSchemaInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := chatResponse{Message: chatMessage{Content: "not-json"}, Done: true}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second, "30m")
	_, err := c.Judge(context.Background(), fixtureReq())
	if !errors.Is(err, errs.ErrSchemaInvalid) {
		t.Fatalf("expected ErrSchemaInvalid, got %v", err)
	}
}

func TestOllama_Tags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:30b"},{"name":"nomic-embed-text"}]}`))
	}))
	defer srv.Close()
	c := New(srv.URL, time.Second, "30m")
	got, err := c.Tags(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"qwen3:30b", "nomic-embed-text"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tags: %v", got)
	}
}
