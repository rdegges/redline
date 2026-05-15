package log

import (
	"log/slog"
	"regexp"
	"strings"
)

// secretPatterns are matched against any string-valued attribute and
// replaced with REDACTED if they match.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{20,}`),
	regexp.MustCompile(`pa-[A-Za-z0-9_-]{20,}`),
}

// redactionKeys are attribute keys whose values must be wholesale redacted.
var redactionKeys = map[string]bool{
	"authorization":     true,
	"x-api-key":         true,
	"anthropic-api-key": true,
	"openai-api-key":    true,
	"voyage-api-key":    true,
	"api_key":           true,
}

// replaceAttr is the slog.HandlerOptions.ReplaceAttr function that
// scrubs secrets out of log records.
func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	if redactionKeys[strings.ToLower(a.Key)] {
		return slog.String(a.Key, "REDACTED")
	}
	if a.Value.Kind() == slog.KindString {
		s := a.Value.String()
		for _, re := range secretPatterns {
			if re.MatchString(s) {
				return slog.String(a.Key, re.ReplaceAllString(s, "REDACTED"))
			}
		}
	}
	return a
}
