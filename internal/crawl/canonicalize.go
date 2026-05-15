package crawl

import (
	"net/url"
	"sort"
	"strings"
)

// trackingParams are stripped from query strings during canonicalization.
var trackingParams = map[string]bool{
	"gclid": true, "fbclid": true, "mc_eid": true, "mc_cid": true,
	"ref": true, "_ga": true, "_gl": true, "igshid": true, "yclid": true,
}

// Canonicalize returns the canonical form of rawURL:
//   - scheme + host lowercased
//   - default ports stripped (:80, :443)
//   - fragment removed
//   - query params sorted alphabetically; tracking params and utm_* stripped
//   - dot segments resolved
//   - single trailing slash removed from non-root paths
func Canonicalize(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", err
	}
	if !u.IsAbs() {
		return "", &url.Error{Op: "canonicalize", URL: rawURL, Err: errNotAbs}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	// Strip default ports.
	switch {
	case u.Scheme == "http" && strings.HasSuffix(u.Host, ":80"):
		u.Host = strings.TrimSuffix(u.Host, ":80")
	case u.Scheme == "https" && strings.HasSuffix(u.Host, ":443"):
		u.Host = strings.TrimSuffix(u.Host, ":443")
	}
	u.Fragment = ""
	// Resolve dot segments by re-parsing the cleaned path.
	if u.Path != "" {
		u.Path = cleanPath(u.Path)
	}
	if u.Path == "" {
		u.Path = "/"
	}
	// Strip trailing slash from non-root paths.
	if len(u.Path) > 1 && strings.HasSuffix(u.Path, "/") {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	// Sort and filter query.
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			lk := strings.ToLower(k)
			if trackingParams[lk] || strings.HasPrefix(lk, "utm_") {
				q.Del(k)
			}
		}
		// Use the standard encoder (already sorts keys alphabetically).
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		for i, k := range keys {
			if i > 0 {
				b.WriteByte('&')
			}
			for j, v := range q[k] {
				if j > 0 {
					b.WriteByte('&')
				}
				b.WriteString(url.QueryEscape(k))
				if v != "" {
					b.WriteByte('=')
					b.WriteString(url.QueryEscape(v))
				}
			}
		}
		u.RawQuery = b.String()
	}
	return u.String(), nil
}

// SameHost reports whether candidate is on the same host as origin.
func SameHost(origin, candidate string) bool {
	o, err1 := url.Parse(origin)
	c, err2 := url.Parse(candidate)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.EqualFold(o.Host, c.Host)
}

// errNotAbs is a sentinel for url.Error.Err when rawURL isn't absolute.
var errNotAbs = &canonErr{"not absolute"}

type canonErr struct{ msg string }

func (e *canonErr) Error() string { return e.msg }

// cleanPath resolves "." and ".." segments without touching trailing slashes.
func cleanPath(p string) string {
	if p == "" {
		return ""
	}
	hadTrail := strings.HasSuffix(p, "/")
	parts := strings.Split(p, "/")
	stack := make([]string, 0, len(parts))
	for _, seg := range parts {
		switch seg {
		case "", ".":
			// Keep leading empty (root) but drop interior empties.
			continue
		case "..":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, seg)
		}
	}
	out := "/" + strings.Join(stack, "/")
	if hadTrail && out != "/" {
		out += "/"
	}
	return out
}
