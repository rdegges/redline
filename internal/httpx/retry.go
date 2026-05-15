// Package httpx provides the shared HTTP client + universal retry wrapper
// used for every outbound call (target site, Ollama, Anthropic, OpenAI,
// Voyage). there is exactly one retry implementation; stage
// packages call into this package.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jonboulle/clockwork"
)

// RetryConfig captures the tunable parameters from --max-retries,
// --retry-base-delay, and --retry-max-delay.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Clock       clockwork.Clock // optional; defaults to clockwork.NewRealClock
	Rand        *rand.Rand      // optional; defaults to a fresh source
}

// DefaultRetryConfig matches the v1 defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{MaxAttempts: 5, BaseDelay: time.Second, MaxDelay: 2 * time.Minute}
}

// AttemptInfo describes one in-flight retry attempt.
type AttemptInfo struct {
	Attempt int
	Sleep   time.Duration
	Err     error
}

// Action returned by Do's body function.
type Action int

const (
	// ActionDone — call succeeded, stop retrying.
	ActionDone Action = iota
	// ActionRetry — call failed transiently, retry after backoff.
	ActionRetry
	// ActionAbort — call failed permanently, surface err immediately.
	ActionAbort
)

// Do runs body up to MaxAttempts times. The body returns the chosen
// Action plus an error. retryAfter (if non-zero) overrides the
// computed backoff for the next attempt; it honors any Retry-After hint from the response.
// onAttempt (optional) is called after every attempt for audit logging.
func Do(ctx context.Context, cfg RetryConfig, onAttempt func(AttemptInfo), body func(ctx context.Context, attempt int) (Action, time.Duration, error)) error {
	if cfg.Clock == nil {
		cfg.Clock = clockwork.NewRealClock()
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		action, retryAfter, err := body(ctx, attempt)
		lastErr = err
		if onAttempt != nil {
			onAttempt(AttemptInfo{Attempt: attempt, Err: err})
		}
		switch action {
		case ActionDone:
			return nil
		case ActionAbort:
			return err
		case ActionRetry:
			if attempt == cfg.MaxAttempts {
				return fmt.Errorf("exhausted %d attempts: %w", cfg.MaxAttempts, err)
			}
			sleep := backoffDelay(cfg, attempt, retryAfter)
			timer := cfg.Clock.NewTimer(sleep)
			select {
			case <-timer.Chan():
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		default:
			return fmt.Errorf("retry: unknown action %d", action)
		}
	}
	return lastErr
}

func backoffDelay(cfg RetryConfig, attempt int, retryAfter time.Duration) time.Duration {
	maxAllowed := cfg.MaxDelay
	if retryAfter > 0 {
		if retryAfter > maxAllowed {
			return maxAllowed
		}
		return retryAfter
	}
	// Exponential with full jitter: random(0, min(maxDelay, base*2^attempt))
	cap := time.Duration(int64(cfg.BaseDelay) << uint(attempt-1))
	if cap > maxAllowed || cap <= 0 {
		cap = maxAllowed
	}
	if cap <= 0 {
		return 0
	}
	return time.Duration(cfg.Rand.Int63n(int64(cap) + 1))
}

// IsRetryable reports whether err looks transient (network blip, 5xx,
// 429, etc.).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := err.Error()
	for _, sub := range []string{"connection reset", "connection refused", "EOF", "unexpected EOF", "broken pipe", "i/o timeout"} {
		if strings.Contains(msg, sub) {
			return true
		}
	}
	return false
}

// IsRetryableStatus returns true if the HTTP status code is retryable.
func IsRetryableStatus(code int) bool {
	switch code {
	case 408, 425, 429, 500, 502, 503, 504, 522, 524, 529:
		return true
	}
	return false
}

// ParseRetryAfter parses a Retry-After header (delta-seconds or HTTP-date).
func ParseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if n, err := strconv.Atoi(h); err == nil {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

// DrainAndClose drains the response body so the connection can be reused
// and then closes it.
func DrainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
