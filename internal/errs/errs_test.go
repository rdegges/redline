package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCode_Nil_ReturnsZero(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Fatalf("ExitCode(nil) = %d, want 0", got)
	}
}

func TestExitCode_SentinelMapping(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{ErrInvalidConfig, 65},
		{ErrPromptsNotFound, 66},
		{ErrAuthFailed, 77},
		{ErrOllamaUnavailable, 69},
		{ErrAPIUnavailable, 69},
		{ErrOllamaModelMissing, 78},
		{ErrDuplicateRun, 64},
		{ErrSchemaVersion, 70},
		{ErrPanicked, 70},
		{errors.New("misc"), 1},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

func TestExitCode_WrappedSentinel(t *testing.T) {
	wrapped := fmt.Errorf("layer: %w", ErrAuthFailed)
	if got := ExitCode(wrapped); got != 77 {
		t.Fatalf("ExitCode(wrapped ErrAuthFailed) = %d, want 77", got)
	}
}
