package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/llm"
)

func req() llm.JudgeRequest {
	return llm.JudgeRequest{Model: "claude", PageURL: "u", PageTitle: "t", PageBody: "b"}
}

func TestAnthropic_HappyPath_CacheControlPresent(t *testing.T) {
	canned := llm.JudgeResponse{
		PrimaryLabel:    "Aligned",
		Confidence:      0.9,
		SuggestedAction: "KEEP",
		PageSummary:     llm.PageSummary{CurrentFocus: "ok"},
		Findings:        []llm.Finding{},
		Rationale:       "long enough rationale to satisfy minimum length of fifty characters total content",
	}
	cb, _ := json.Marshal(canned)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body messagesRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		hasCache := false
		for _, b := range body.System {
			if b.CacheControl != nil {
				hasCache = true
			}
		}
		if !hasCache {
			t.Errorf("expected cache_control on system blocks")
		}
		resp := messagesResponse{Content: []contentBlock{{Type: "text", Text: string(cb)}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	c := New("test-key", time.Second)
	c.Endpoint = srv.URL
	got, err := c.Judge(context.Background(), req())
	if err != nil || got.PrimaryLabel != "Aligned" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestAnthropic_401_AbortsWithAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()
	c := New("test-key", time.Second)
	c.Endpoint = srv.URL
	_, err := c.Judge(context.Background(), req())
	if !errors.Is(err, errs.ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
}

func TestAnthropic_MissingKey_ReturnsAuthFailed(t *testing.T) {
	c := New("", time.Second)
	_, err := c.Judge(context.Background(), req())
	if !errors.Is(err, errs.ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
}

func TestAnthropic_5xx_ReturnsAPIUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c := New("test-key", time.Second)
	c.Endpoint = srv.URL
	_, err := c.Judge(context.Background(), req())
	if err == nil {
		t.Fatal("expected error")
	}
}
