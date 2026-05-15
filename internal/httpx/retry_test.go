package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
)

func TestDo_DoneFirstTry_StopsImmediately(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.Clock = clockwork.NewFakeClock()
	calls := 0
	err := Do(context.Background(), cfg, nil, func(_ context.Context, _ int) (Action, time.Duration, error) {
		calls++
		return ActionDone, 0, nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
}

func TestDo_AbortStopsRetries(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.Clock = clockwork.NewFakeClock()
	calls := 0
	body := func(_ context.Context, _ int) (Action, time.Duration, error) {
		calls++
		return ActionAbort, 0, fmt.Errorf("permanent")
	}
	err := Do(context.Background(), cfg, nil, body)
	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDo_RetriesUntilSuccess_FakeClock(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 4
	cfg.BaseDelay = 100 * time.Millisecond
	cfg.MaxDelay = time.Second
	clk := clockwork.NewFakeClock()
	cfg.Clock = clk

	attempts := 0
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- Do(context.Background(), cfg, nil, func(_ context.Context, n int) (Action, time.Duration, error) {
			attempts++
			if n < 3 {
				return ActionRetry, 0, fmt.Errorf("blip")
			}
			return ActionDone, 0, nil
		})
	}()
	// Advance the clock past every possible sleep until Do completes.
	for i := 0; i < 10; i++ {
		select {
		case err := <-doneCh:
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if attempts != 3 {
				t.Fatalf("attempts=%d want 3", attempts)
			}
			return
		case <-time.After(50 * time.Millisecond):
			clk.Advance(2 * time.Second)
		}
	}
	t.Fatal("Do never returned")
}

func TestDo_ExhaustionReturnsLastError(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 2
	cfg.BaseDelay = time.Nanosecond
	clk := clockwork.NewFakeClock()
	cfg.Clock = clk
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- Do(context.Background(), cfg, nil, func(_ context.Context, _ int) (Action, time.Duration, error) {
			return ActionRetry, 0, fmt.Errorf("nope")
		})
	}()
	for i := 0; i < 10; i++ {
		select {
		case err := <-doneCh:
			if err == nil || !errors.Is(err, err) {
				t.Fatalf("want err, got %v", err)
			}
			return
		case <-time.After(50 * time.Millisecond):
			clk.Advance(time.Minute)
		}
	}
	t.Fatal("Do never returned")
}

func TestDo_ContextCancellation_AbortsImmediately(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 5
	cfg.BaseDelay = time.Hour
	cfg.MaxDelay = time.Hour
	clk := clockwork.NewFakeClock()
	cfg.Clock = clk
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- Do(ctx, cfg, nil, func(_ context.Context, _ int) (Action, time.Duration, error) {
			return ActionRetry, 0, fmt.Errorf("blip")
		})
	}()
	// Let one attempt happen, then cancel before backoff completes.
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-doneCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Do did not honor cancellation")
	}
}

func TestIsRetryable_NetworkAndContext(t *testing.T) {
	if IsRetryable(nil) {
		t.Fatal("nil should not be retryable")
	}
	if IsRetryable(context.Canceled) {
		t.Fatal("Canceled is NOT retryable")
	}
	if !IsRetryable(context.DeadlineExceeded) {
		t.Fatal("DeadlineExceeded is retryable")
	}
	if !IsRetryable(fmt.Errorf("read tcp: connection refused")) {
		t.Fatal("connection refused is retryable")
	}
}

func TestIsRetryableStatus(t *testing.T) {
	cases := map[int]bool{
		200: false, 301: false, 400: false, 403: false, 404: false,
		408: true, 425: true, 429: true, 500: true, 502: true, 503: true,
		504: true, 522: true, 524: true, 529: true,
	}
	for code, want := range cases {
		if got := IsRetryableStatus(code); got != want {
			t.Errorf("code %d => %v want %v", code, got, want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := ParseRetryAfter("30"); got != 30*time.Second {
		t.Fatalf("seconds: %v", got)
	}
	if got := ParseRetryAfter(""); got != 0 {
		t.Fatalf("empty: %v", got)
	}
	httpDate := time.Now().Add(15 * time.Second).UTC().Format(http.TimeFormat)
	d := ParseRetryAfter(httpDate)
	if d <= 0 || d > 16*time.Second {
		t.Fatalf("http date: %v", d)
	}
}

func TestDo_RetryAfterOverrideUsesProvidedDelay(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 2
	cfg.BaseDelay = 10 * time.Millisecond
	cfg.MaxDelay = time.Second
	clk := clockwork.NewFakeClock()
	cfg.Clock = clk
	calls := 0
	doneCh := make(chan error, 1)
	go func() {
		doneCh <- Do(context.Background(), cfg, nil, func(_ context.Context, _ int) (Action, time.Duration, error) {
			calls++
			if calls == 1 {
				return ActionRetry, 25 * time.Millisecond, fmt.Errorf("blip")
			}
			return ActionDone, 0, nil
		})
	}()
	for i := 0; i < 10; i++ {
		select {
		case err := <-doneCh:
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			return
		case <-time.After(20 * time.Millisecond):
			clk.Advance(100 * time.Millisecond)
		}
	}
	t.Fatal("Do never returned")
}
