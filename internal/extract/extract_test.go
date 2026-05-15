package extract

import (
	"net/url"
	"strings"
	"testing"
)

const navHTML = `<!DOCTYPE html><html><head><title>Hello</title>
<link rel="canonical" href="https://example.com/canon">
<meta name="description" content="A page about widgets.">
<meta property="article:modified_time" content="2026-01-02T03:04:05Z">
</head>
<body>
<nav>
  <a href="/products/sast">SAST</a>
  <a href="/products/container">Container</a>
  <a href="/products/iac">IaC</a>
  <a href="/products/apprisk">AppRisk</a>
  <a href="/products/aitrust">AI Trust</a>
</nav>
<main>
<h1>Widgets</h1>
<p>Widgets help you ship secure code. We sell widgets in many flavors.</p>
<p>Our widgets are best-in-class — try them today.</p>
</main>
<footer><a href="/legal">Legal</a></footer>
</body></html>`

func TestExtract_NavLinksVsBody(t *testing.T) {
	doc, err := Parse(strings.NewReader(navHTML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body, count, empty := ExtractBodyText(doc, 5)
	if empty {
		t.Fatalf("expected non-empty body, got count=%d", count)
	}
	// Body text must NOT include nav anchor labels.
	for _, label := range []string{"SAST", "Container", "IaC", "AppRisk", "AI Trust", "Legal"} {
		if strings.Contains(body, label) {
			t.Errorf("body should not include nav label %q: %q", label, body)
		}
	}
	if !strings.Contains(body, "Widgets help you ship secure code.") {
		t.Fatalf("body missing main paragraph: %q", body)
	}
	baseURL, _ := url.Parse("https://example.com/")
	links := ExtractLinks(doc, baseURL)
	if len(links) < 6 {
		t.Fatalf("expected ≥6 links (5 nav + 1 footer), got %d", len(links))
	}
	hrefs := map[string]bool{}
	for _, l := range links {
		hrefs[l.To] = true
	}
	for _, expect := range []string{
		"https://example.com/products/sast",
		"https://example.com/products/container",
		"https://example.com/legal",
	} {
		if !hrefs[expect] {
			t.Errorf("missing link %q", expect)
		}
	}
}

func TestExtract_FallbackWithoutMainOrArticle(t *testing.T) {
	html := `<html><body>
<nav><a href="/a">A</a></nav>
<p>Body text that should be extracted.</p>
<footer>Foot</footer>
</body></html>`
	doc, _ := Parse(strings.NewReader(html))
	body, _, empty := ExtractBodyText(doc, 5)
	if empty {
		t.Fatal("expected non-empty body")
	}
	if !strings.Contains(body, "Body text that should be extracted") {
		t.Fatalf("body: %q", body)
	}
	if strings.Contains(body, "Foot") {
		t.Fatalf("footer leaked: %q", body)
	}
}

func TestExtract_EmptyShellDetection(t *testing.T) {
	html := `<html><body><div id="root"></div></body></html>`
	doc, _ := Parse(strings.NewReader(html))
	_, count, empty := ExtractBodyText(doc, 50)
	if !empty {
		t.Fatalf("expected empty shell, count=%d", count)
	}
}

func TestExtract_Metadata(t *testing.T) {
	doc, _ := Parse(strings.NewReader(navHTML))
	m := ExtractMetadata(doc, nil)
	if m.Title != "Hello" {
		t.Fatalf("title = %q", m.Title)
	}
	if m.CanonicalURL != "https://example.com/canon" {
		t.Fatalf("canonical = %q", m.CanonicalURL)
	}
	if m.MetaDescription != "A page about widgets." {
		t.Fatalf("desc = %q", m.MetaDescription)
	}
	if m.LastModified == "" {
		t.Fatal("expected last_modified")
	}
}

func FuzzExtract_NoPanicOnMalformed(f *testing.F) {
	f.Add([]byte("<html><body>x</body></html>"))
	f.Add([]byte("<<><<<<>>>>"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, raw []byte) {
		doc, err := Parse(strings.NewReader(string(raw)))
		if err != nil {
			return // some inputs may legitimately fail to parse
		}
		_, _, _ = ExtractBodyText(doc, 50)
		baseURL, _ := url.Parse("https://example.com/")
		_ = ExtractLinks(doc, baseURL)
		_ = ExtractMetadata(doc, nil)
	})
}
