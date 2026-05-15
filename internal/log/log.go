// Package log provides the structured slog handler used across redline.
// It supports a dual-sink pattern: every log record is written both to
// the configured stderr sink and (when a DB-backed event sink is provided)
// inserted into the events table.
package log

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// EventSink receives a structured copy of every log record (info level
// and above by default; debug when the underlying handler is in debug
// mode). It is the bridge between slog and the `events` SQLite table.
type EventSink interface {
	HandleEvent(ctx context.Context, ev Event)
}

// Event mirrors the row inserted into the `events` table.
type Event struct {
	Time      string // RFC3339Nano
	Level     string // debug/info/warn/error
	Phase     string
	EventType string
	URL       string
	Message   string
	RunID     string
	WorkerID  string
	Attempt   int
	LatencyMs int
	Error     string
	ErrorKind string
	Severity  string
	Payload   map[string]any
}

// Format selects the output format for the stderr sink.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// Options configures NewLogger.
type Options struct {
	Level  slog.Level
	Format Format
	Sink   EventSink // optional
	Out    io.Writer // defaults to os.Stderr when nil
}

// NewLogger builds a *slog.Logger with the redline dual-sink pipeline.
func NewLogger(opts Options) *slog.Logger {
	if opts.Out == nil {
		panic("log.NewLogger: Out is required")
	}
	hopts := &slog.HandlerOptions{
		Level:       opts.Level,
		ReplaceAttr: replaceAttr,
	}
	var base slog.Handler
	switch opts.Format {
	case FormatJSON:
		base = slog.NewJSONHandler(opts.Out, hopts)
	default:
		base = slog.NewTextHandler(opts.Out, hopts)
	}
	h := &dualHandler{base: base, sink: opts.Sink, level: opts.Level}
	return slog.New(h)
}

type dualHandler struct {
	mu    sync.Mutex
	base  slog.Handler
	sink  EventSink
	level slog.Level
	group string
	attrs []slog.Attr
}

func (h *dualHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.base.Enabled(ctx, l)
}

func (h *dualHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.base.Handle(ctx, r); err != nil {
		return err
	}
	if h.sink != nil {
		h.sink.HandleEvent(ctx, recordToEvent(r, h.attrs))
	}
	return nil
}

func (h *dualHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &dualHandler{base: h.base.WithAttrs(as), sink: h.sink, level: h.level, group: h.group, attrs: append(append([]slog.Attr{}, h.attrs...), as...)}
}

func (h *dualHandler) WithGroup(name string) slog.Handler {
	return &dualHandler{base: h.base.WithGroup(name), sink: h.sink, level: h.level, group: name, attrs: append([]slog.Attr{}, h.attrs...)}
}

func recordToEvent(r slog.Record, base []slog.Attr) Event {
	ev := Event{
		Time:    r.Time.UTC().Format("2006-01-02T15:04:05.000000000Z"),
		Level:   strings.ToLower(r.Level.String()),
		Message: r.Message,
		Payload: map[string]any{},
	}
	// Map level to severity (events.severity column).
	ev.Severity = ev.Level
	apply := func(a slog.Attr) {
		a = replaceAttr(nil, a)
		switch a.Key {
		case "phase":
			ev.Phase = a.Value.String()
		case "event_type":
			ev.EventType = a.Value.String()
		case "url":
			ev.URL = a.Value.String()
		case "run_id":
			ev.RunID = a.Value.String()
		case "worker_id":
			ev.WorkerID = a.Value.String()
		case "attempt":
			ev.Attempt = int(a.Value.Int64())
		case "latency_ms":
			ev.LatencyMs = int(a.Value.Int64())
		case "error":
			ev.Error = a.Value.String()
		case "error_kind":
			ev.ErrorKind = a.Value.String()
		default:
			ev.Payload[a.Key] = a.Value.Any()
		}
	}
	for _, a := range base {
		apply(a)
	}
	r.Attrs(func(a slog.Attr) bool { apply(a); return true })
	return ev
}

// Discard returns a logger that swallows everything; useful for tests
// where the logger is required but its output is not asserted.
func Discard() *slog.Logger {
	return NewLogger(Options{Out: io.Discard, Level: slog.LevelDebug})
}
