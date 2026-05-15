// Package extract converts an HTML document into the two outputs the
// rest of the pipeline needs: body_text (for the LLM judge) and
// outbound_links (for crawler BFS). these are two distinct
// passes over the SAME parsed DOM. Conflating them is a load-bearing
// bug — see TestExtract_NavLinksVsBody in extract_test.go.
package extract

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

// Metadata is the per-page metadata extracted from the document head.
type Metadata struct {
	Title           string
	MetaDescription string
	CanonicalURL    string
	LastModified    string
	PublishedDate   string
}

// OutboundLink is a single <a href> edge.
type OutboundLink struct {
	From       string
	To         string
	Anchor     string
	IsInternal bool
}

// Parse parses HTML bytes into a goquery.Document. Returns an error
// only for I/O issues; malformed HTML is parsed leniently.
func Parse(r io.Reader) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(r)
}

// ExtractBodyText returns the visible text from the primary content
// region. The empty-shell flag is true when wordCount
// is below thinThreshold; pass 50 for the v1 default.
func ExtractBodyText(doc *goquery.Document, thinThreshold int) (body string, wordCount int, isEmptyShell bool) {
	if thinThreshold <= 0 {
		thinThreshold = 50
	}
	region := selectPrimaryRegion(doc)
	if region == nil || region.Length() == 0 {
		return "", 0, true
	}
	// Strip non-content children before serializing text.
	region.Find("script, style, svg, noscript, template, [aria-hidden=true]").Remove()
	// Also strip nav/header/footer/aside in case the fallback path picked them up.
	region.Find("nav, header, footer, aside").Remove()
	text := strings.TrimSpace(region.Text())
	text = collapseWhitespace(text)
	count := countWords(text)
	return text, count, count < thinThreshold
}

func selectPrimaryRegion(doc *goquery.Document) *goquery.Selection {
	// Prefer <main> over <article>: a naive "first <article>" heuristic
	// picks small card elements on modern marketing sites and discards
	// the real body.
	if s := doc.Find("main").First(); s.Length() > 0 {
		return s
	}
	if s := doc.Find("article").First(); s.Length() > 0 {
		return s
	}
	body := doc.Find("body").First()
	if body.Length() == 0 {
		return nil
	}
	// Use a clone to avoid mutating the caller's document on the
	// nav-strip pass (the link extractor must still see the nav).
	clone := body.Clone()
	clone.Find("nav, header, footer, aside, script, style").Remove()
	return clone
}

// ExtractLinks walks the FULL DOM (not just the primary region) per
// baseURL is used to resolve relative hrefs.
func ExtractLinks(doc *goquery.Document, baseURL *url.URL) []OutboundLink {
	if doc == nil || baseURL == nil {
		return nil
	}
	// Honor a <base href> if present.
	if base, _ := doc.Find("base[href]").First().Attr("href"); base != "" {
		if b, err := url.Parse(base); err == nil {
			baseURL = baseURL.ResolveReference(b)
		}
	}
	seen := map[string]bool{}
	var out []OutboundLink
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		anchor := collapseWhitespace(strings.TrimSpace(s.Text()))
		if href == "" {
			return
		}
		if strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "tel:") {
			return
		}
		ref, err := url.Parse(href)
		if err != nil {
			return
		}
		abs := baseURL.ResolveReference(ref)
		abs.Fragment = ""
		key := abs.String()
		if seen[key] {
			return
		}
		seen[key] = true
		internal := abs.Host == baseURL.Host
		out = append(out, OutboundLink{
			From:       baseURL.String(),
			To:         abs.String(),
			Anchor:     anchor,
			IsInternal: internal,
		})
	})
	return out
}

// ExtractMetadata pulls title, meta description, canonical URL, dates.
func ExtractMetadata(doc *goquery.Document, headers http.Header) Metadata {
	m := Metadata{}
	m.Title = strings.TrimSpace(doc.Find("title").First().Text())
	if m.Title == "" {
		m.Title = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	m.MetaDescription = strings.TrimSpace(metaContent(doc, `meta[name="description"]`))
	if v, ok := doc.Find(`link[rel="canonical"]`).First().Attr("href"); ok {
		m.CanonicalURL = strings.TrimSpace(v)
	}
	for _, sel := range []string{
		`meta[property="article:modified_time"]`,
		`meta[name="last-modified"]`,
	} {
		if v := metaContent(doc, sel); v != "" {
			m.LastModified = strings.TrimSpace(v)
			break
		}
	}
	if m.LastModified == "" && headers != nil {
		if lm := headers.Get("Last-Modified"); lm != "" {
			if t, err := http.ParseTime(lm); err == nil {
				m.LastModified = t.UTC().Format(time.RFC3339)
			}
		}
	}
	for _, sel := range []string{
		`meta[property="article:published_time"]`,
		`time[datetime]`,
	} {
		if v := metaContent(doc, sel); v != "" {
			m.PublishedDate = strings.TrimSpace(v)
			break
		}
	}
	return m
}

func metaContent(doc *goquery.Document, selector string) string {
	s := doc.Find(selector).First()
	if v, ok := s.Attr("content"); ok && v != "" {
		return v
	}
	if v, ok := s.Attr("datetime"); ok && v != "" {
		return v
	}
	return strings.TrimSpace(s.Text())
}

func collapseWhitespace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

func countWords(s string) int {
	return len(strings.Fields(s))
}
