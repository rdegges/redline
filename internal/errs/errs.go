// Package errs holds the sentinel errors used across redline so that
// callers can use errors.Is/As to classify failures, and the CLI can map
// them to deterministic exit codes.
package errs

import "errors"

// Sentinel errors. All errors returned from internal packages should
// wrap one of these via fmt.Errorf("...: %w", err) so the caller can
// route on errors.Is.
var (
	// Config / input.
	ErrInvalidConfig   = errors.New("invalid config")
	ErrPromptsNotFound = errors.New("prompts.yaml not found")

	// Auth.
	ErrAuthFailed = errors.New("auth failed")

	// Providers.
	ErrOllamaUnavailable  = errors.New("ollama unavailable")
	ErrOllamaModelMissing = errors.New("ollama model not pulled")
	ErrOllamaLoading      = errors.New("ollama model loading")
	ErrAPIUnavailable     = errors.New("api unavailable")

	// Run state.
	ErrDuplicateRun  = errors.New("duplicate run; pass --resume=true to continue")
	ErrRunFinalized  = errors.New("run already completed")
	ErrSchemaVersion = errors.New("db schema version mismatch")

	// Judge.
	ErrSchemaInvalid         = errors.New("llm response did not match schema")
	ErrJudgeAllRetriesFailed = errors.New("judge call exhausted retries")

	// Fetcher.
	ErrNonHTML         = errors.New("non-html content")
	ErrBodyTooLarge    = errors.New("response body exceeded cap")
	ErrCrawlDisallowed = errors.New("crawl disallowed by robots.txt")

	// Internal.
	ErrPanicked = errors.New("internal panic recovered")
)

// ExitCode returns the POSIX-ish exit code that maps to err.
// Returns 0 if err is nil, 1 for a generic error with no sentinel match.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	switch {
	case errors.Is(err, ErrInvalidConfig):
		return 65
	case errors.Is(err, ErrPromptsNotFound):
		return 66
	case errors.Is(err, ErrAuthFailed):
		return 77
	case errors.Is(err, ErrOllamaUnavailable),
		errors.Is(err, ErrAPIUnavailable):
		return 69
	case errors.Is(err, ErrOllamaModelMissing):
		return 78
	case errors.Is(err, ErrDuplicateRun):
		return 64
	case errors.Is(err, ErrSchemaVersion),
		errors.Is(err, ErrPanicked):
		return 70
	}
	return 1
}
