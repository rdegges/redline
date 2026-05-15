package log

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

type captureSink struct {
	mu     sync.Mutex
	events []Event
}

func (c *captureSink) HandleEvent(_ context.Context, ev Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

func TestLogger_JSONFormat_WritesStructured(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{Out: &buf, Format: FormatJSON, Level: slog.LevelInfo})
	logger.Info("hello",
		slog.String("event_type", FetchSuccess),
		slog.String("url", "https://example.com"),
	)
	out := buf.String()
	if !strings.Contains(out, `"event_type":"fetch.success"`) {
		t.Fatalf("expected event_type field: %s", out)
	}
	if !strings.Contains(out, `"url":"https://example.com"`) {
		t.Fatalf("expected url field: %s", out)
	}
}

func TestLogger_TextFormat_DefaultsOK(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{Out: &buf, Level: slog.LevelInfo})
	logger.Info("ok", slog.String("event_type", RunStarted))
	if !strings.Contains(buf.String(), "ok") {
		t.Fatalf("expected message in text output: %s", buf.String())
	}
}

func TestLogger_RedactsAPIKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{Out: &buf, Format: FormatJSON, Level: slog.LevelInfo})
	logger.Info("call",
		slog.String("Authorization", "Bearer sk-abc12345678901234567890"),
		slog.String("note", "key sk-deadbeef0123456789012345 leaked"),
	)
	out := buf.String()
	if strings.Contains(out, "sk-abc12345678901234567890") {
		t.Fatalf("authorization key leaked: %s", out)
	}
	if strings.Contains(out, "sk-deadbeef0123456789012345") {
		t.Fatalf("note key leaked: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected REDACTED marker: %s", out)
	}
}

func TestLogger_DualSink_ForwardsToEventSink(t *testing.T) {
	var buf bytes.Buffer
	sink := &captureSink{}
	logger := NewLogger(Options{Out: &buf, Format: FormatJSON, Level: slog.LevelDebug, Sink: sink})
	logger.Info("running",
		slog.String("event_type", PhaseCrawlStarted),
		slog.String("phase", "crawl"),
		slog.String("url", "https://example.com/x"),
		slog.Int("attempt", 2),
	)
	if len(sink.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(sink.events))
	}
	ev := sink.events[0]
	if ev.EventType != PhaseCrawlStarted || ev.Phase != "crawl" || ev.URL != "https://example.com/x" || ev.Attempt != 2 {
		t.Fatalf("event missing fields: %+v", ev)
	}
}

func TestLogger_WithAttrs_PropagatesToSink(t *testing.T) {
	var buf bytes.Buffer
	sink := &captureSink{}
	logger := NewLogger(Options{Out: &buf, Format: FormatJSON, Level: slog.LevelInfo, Sink: sink}).
		With(slog.String("run_id", "01HZZ"), slog.String("phase", "crawl"))
	logger.Info("x", slog.String("event_type", FetchSuccess))
	if len(sink.events) != 1 {
		t.Fatalf("event count = %d", len(sink.events))
	}
	if sink.events[0].RunID != "01HZZ" {
		t.Fatalf("run_id not propagated: %+v", sink.events[0])
	}
}

func TestDiscard_ReturnsUsableLogger(t *testing.T) {
	l := Discard()
	l.Info("noop")
}
