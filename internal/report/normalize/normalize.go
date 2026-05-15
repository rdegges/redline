// Package normalize replaces dynamic fields in reports with stable
// placeholders so golden-file tests can compare byte-for-byte.
package normalize

import (
	"regexp"
)

// Replacement is the placeholder used for one or more JSON paths.
type Replacement struct {
	Pattern *regexp.Regexp
	Sub     string
}

// JSONReplacements is the ordered list of pattern→placeholder rules
// applied to JSON output before comparison.
var JSONReplacements = []Replacement{
	// ISO-8601 timestamps anywhere.
	{regexp.MustCompile(`"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z"`), `"<TIMESTAMP>"`},
	{regexp.MustCompile(`"\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?"`), `"<TIMESTAMP>"`},
	// Run ID (ULID/UUID-like).
	{regexp.MustCompile(`"run_id":\s*"[0-9a-zA-Z-]{4,}"`), `"run_id": "<RUN_ID>"`},
	{regexp.MustCompile(`"duration_seconds":\s*-?\d+`), `"duration_seconds": "<DURATION>"`},
	{regexp.MustCompile(`"total_input_tokens":\s*-?\d+`), `"total_input_tokens": "<TOKENS>"`},
	{regexp.MustCompile(`"total_cached_tokens":\s*-?\d+`), `"total_cached_tokens": "<TOKENS>"`},
	{regexp.MustCompile(`"total_output_tokens":\s*-?\d+`), `"total_output_tokens": "<TOKENS>"`},
	{regexp.MustCompile(`"input_tokens":\s*-?\d+`), `"input_tokens": "<TOKENS>"`},
	{regexp.MustCompile(`"output_tokens":\s*-?\d+`), `"output_tokens": "<TOKENS>"`},
	{regexp.MustCompile(`"cache_hit_tokens":\s*-?\d+`), `"cache_hit_tokens": "<TOKENS>"`},
	{regexp.MustCompile(`"judge_attempts":\s*\d+`), `"judge_attempts": "<ATTEMPTS>"`},
	{regexp.MustCompile(`"retries_total":\s*\d+`), `"retries_total": "<COUNT>"`},
	{regexp.MustCompile(`"fetch_failures":\s*\d+`), `"fetch_failures": "<COUNT>"`},
	{regexp.MustCompile(`"judge_failures":\s*\d+`), `"judge_failures": "<COUNT>"`},
	{regexp.MustCompile(`"embed_failures":\s*\d+`), `"embed_failures": "<COUNT>"`},
	{regexp.MustCompile(`"redline_version":\s*"[^"]+"`), `"redline_version": "<VERSION>"`},
	{regexp.MustCompile(`"latency_ms":\s*\d+`), `"latency_ms": "<LATENCY>"`},
	{regexp.MustCompile(`"prompts_yaml_sha256":\s*"[^"]+"`), `"prompts_yaml_sha256": "<SHA256>"`},
	{regexp.MustCompile(`"total_api_calls":\s*\d+`), `"total_api_calls": "<COUNT>"`},
}

// MarkdownReplacements is applied to MD output.
var MarkdownReplacements = []Replacement{
	{regexp.MustCompile(`\*\*Run:\*\* [0-9a-zA-Z-]+`), `**Run:** <RUN_ID>`},
	{regexp.MustCompile(`\*\*Completed:\*\* [^\n]+`), `**Completed:** <TIMESTAMP>`},
	{regexp.MustCompile(`\*\*Duration:\*\* -?\d+s`), `**Duration:** <DURATION>`},
	{regexp.MustCompile(`\*\*API calls:\*\* \d+`), `**API calls:** <COUNT>`},
	{regexp.MustCompile(`\*\*redline version:\*\* [^\n]+`), `**redline version:** <VERSION>`},
	{regexp.MustCompile(`\*\*Run stats:\*\* \d+ pages · \d+ fetch failures · \d+ judge failures · \d+ retries\.`), `**Run stats:** <COUNT> pages · <COUNT> fetch failures · <COUNT> judge failures · <COUNT> retries.`},
}

// JSON replaces dynamic fields in a JSON byte slice with stable
// placeholders.
func JSON(in []byte) []byte {
	out := in
	for _, r := range JSONReplacements {
		out = r.Pattern.ReplaceAll(out, []byte(r.Sub))
	}
	return out
}

// Markdown replaces dynamic fields in a Markdown byte slice.
func Markdown(in []byte) []byte {
	out := in
	for _, r := range MarkdownReplacements {
		out = r.Pattern.ReplaceAll(out, []byte(r.Sub))
	}
	return out
}
