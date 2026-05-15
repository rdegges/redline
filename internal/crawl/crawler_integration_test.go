//go:build integration

package crawl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rdegges/redline/internal/httpx"
	geoplog "github.com/rdegges/redline/internal/log"
	"github.com/rdegges/redline/internal/store"
)

func fixtureHandler() http.Handler {
	pages := map[string]string{
		"/":                        homepageHTML,
		"/products/sast.html":      pageHTML("SAST", "Acme Code is a developer-first SAST tool that runs in the IDE and PR pipeline."),
		"/products/container.html": pageHTML("Container", "Acme Container scans images at the registry, plus build time."),
		"/products/retired.html":   pageHTML("Acme Cloud", "Acme Cloud helps protect runtime workloads."),
		"/blog/post-1.html":        pageHTML("Post 1", "A blog post about AI-generated code security."),
		"/blog/post-2.html":        pageHTML("Post 2", "A second blog post about supply chain."),
		"/private/secret.html":     pageHTML("Secret", "Restricted content"),
		"/robots.txt":              "User-agent: *\nDisallow: /private/\nSitemap: %SITEMAP%\n",
		"/sitemap.xml":             sitemapXML,
	}
	mux := http.NewServeMux()
	for path, body := range pages {
		p := path
		b := body
		mux.HandleFunc(p, func(w http.ResponseWriter, r *http.Request) {
			if p == "/robots.txt" {
				b = strings.ReplaceAll(b, "%SITEMAP%", "http://"+r.Host+"/sitemap.xml")
				w.Header().Set("Content-Type", "text/plain")
			} else if p == "/sitemap.xml" {
				b = strings.ReplaceAll(b, "%HOST%", "http://"+r.Host)
				w.Header().Set("Content-Type", "application/xml")
			} else {
				w.Header().Set("Content-Type", "text/html")
			}
			_, _ = w.Write([]byte(b))
		})
	}
	return mux
}

const homepageHTML = `<!DOCTYPE html><html><head><title>Home</title></head>
<body><nav>
<a href="/products/sast.html">SAST</a>
<a href="/products/container.html">Container</a>
<a href="/products/retired.html">Retired</a>
<a href="/blog/post-1.html">Blog 1</a>
<a href="/blog/post-2.html">Blog 2</a>
</nav><main>
<p>The Acme developer security platform secures code, dependencies, containers, and IaC. We help teams ship faster while staying secure. Our AI-powered fix engine suggests one-click remediation. Join the thousands of engineering teams who trust Acme to protect their software supply chain end to end.</p>
<p>Whether you're a developer in your IDE or a CISO running an application security program, Acme meets you where you work. Try the free tier today and see how developer-first security can change your workflow forever, no expensive professional services required.</p>
</main></body></html>`

func pageHTML(title, body string) string {
	return fmt.Sprintf(`<!DOCTYPE html><html><head><title>%s</title></head>
<body><nav><a href="/">Home</a></nav><main>
<h1>%s</h1>
<p>%s This is a longer paragraph of content that ensures the page has substantive body text suitable for content auditing. We need at least fifty words for the page to escape the empty-shell heuristic threshold.</p>
<p>Acme gives developers and AppSec teams the tools to find and fix vulnerabilities directly in the IDE, in pull requests, and across CI. Our AI Trust Platform extends to AI-generated code.</p>
</main></body></html>`, title, title, body)
}

const sitemapXML = `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%HOST%/products/sast.html</loc></url>
  <url><loc>%HOST%/products/container.html</loc></url>
  <url><loc>%HOST%/products/retired.html</loc></url>
  <url><loc>%HOST%/blog/post-1.html</loc></url>
  <url><loc>%HOST%/blog/post-2.html</loc></url>
</urlset>`

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(context.Background(), p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.InsertRun(context.Background(), store.Run{
		ID: "run1", SiteURL: "http://placeholder", PromptsSHA256: "sha",
		ConfigJSON: "{}", LLMProvider: "ollama", LLMModel: "qwen3:30b",
		Version: "test", Status: store.RunRunning,
	}); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	return db
}

func TestCrawler_FixtureSite_DiscoversAllPages(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler())
	defer srv.Close()
	// Update the run's site_url so the unique-active index doesn't trip.
	db := newTestDB(t)
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `UPDATE runs SET site_url=? WHERE id='run1'`, srv.URL)

	cfg := Config{
		Site:           srv.URL,
		MaxPages:       20,
		MaxDepth:       4,
		Concurrency:    2,
		Rate:           50,
		Exclude:        DefaultExcludes(),
		ThinThreshold:  5,
		MaxRetries:     2,
		RetryBaseDelay: time.Millisecond,
		RetryMaxDelay:  10 * time.Millisecond,
	}
	cr := &Crawler{
		Cfg:    cfg,
		Client: httpx.NewClient(time.Second, 50, "redline-test"),
		DB:     db,
		Logger: geoplog.Discard(),
		RunID:  "run1",
	}
	got, err := cr.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Expect homepage + 5 product/blog pages = 6.
	if got < 6 {
		t.Fatalf("pages fetched = %d, want >=6", got)
	}
	// Private should be disallowed.
	pages, err := db.ListPagesByRun(ctx, "run1")
	if err != nil {
		t.Fatalf("list pages: %v", err)
	}
	for _, p := range pages {
		if strings.Contains(p.URL, "/private/") {
			t.Errorf("private/ should be excluded by robots: %s", p.URL)
		}
	}
}

func TestCrawler_RespectsMaxPages(t *testing.T) {
	srv := httptest.NewServer(fixtureHandler())
	defer srv.Close()
	db := newTestDB(t)
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, `UPDATE runs SET site_url=? WHERE id='run1'`, srv.URL)
	cfg := Config{
		Site:          srv.URL,
		MaxPages:      2,
		MaxDepth:      4,
		Concurrency:   2,
		Rate:          50,
		Exclude:       DefaultExcludes(),
		ThinThreshold: 5,
		MaxRetries:    1,
	}
	cr := &Crawler{
		Cfg: cfg, Client: httpx.NewClient(time.Second, 50, "ua"), DB: db,
		Logger: geoplog.Discard(), RunID: "run1",
	}
	got, _ := cr.Run(ctx)
	// Soft cap: workers may finish one in-flight fetch past the threshold
	//. Allow a small overshoot but not "everything".
	if got > 2+cfg.Concurrency {
		t.Fatalf("max-pages not honored: fetched %d (cap 2+concurrency)", got)
	}
}

func TestCrawler_CanonicalizesTrackingParams(t *testing.T) {
	// quick canonicalization round-trip via parsing
	u := "http://example.com/a?utm_source=foo&b=1"
	canon, err := Canonicalize(u)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if strings.Contains(canon, "utm_source") {
		t.Fatalf("utm_source not stripped: %s", canon)
	}
	parsed, _ := url.Parse(canon)
	if parsed.Path != "/a" {
		t.Fatalf("path: %s", parsed.Path)
	}
}
