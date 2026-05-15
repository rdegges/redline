package crawl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rdegges/redline/internal/extract"
	"github.com/rdegges/redline/internal/httpx"
	logevt "github.com/rdegges/redline/internal/log"
	"github.com/rdegges/redline/internal/store"
	"github.com/temoto/robotstxt"
)

// Config bundles the crawl-relevant settings derived from CLI flags.
type Config struct {
	Site           string
	Seeds          []string
	MaxPages       int
	MaxDepth       int
	Concurrency    int
	Rate           float64
	IgnoreRobots   bool
	RespectCanon   bool
	UserAgent      string
	Include        []*regexp.Regexp
	Exclude        []*regexp.Regexp
	HTTPTimeout    time.Duration
	ThinThreshold  int
	MaxRetries     int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
}

// DefaultExcludes are the patterns described in the schema.
func DefaultExcludes() []*regexp.Regexp {
	patterns := []string{
		`/wp-admin/`, `/wp-content/uploads/`, `/wp-json/`,
		`/feed/?$`, `/page/\d+/?$`,
		`/tag/`, `/author/`, `/cdn-cgi/`, `/\.well-known/`,
		`\.(jpg|jpeg|png|gif|svg|webp|ico|css|js|woff2?|ttf|eot|map|pdf|zip|tar|gz|mp4|webm|mp3)(\?|$)`,
	}
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}

// Crawler runs the BFS + sitemap + robots discovery and persists pages.
type Crawler struct {
	Cfg          Config
	Client       *httpx.Client
	DB           *store.DB
	Logger       *slog.Logger
	Robots       *robotstxt.RobotsData
	RunID        string
	BaseURL      *url.URL
	pagesFetched int64
}

// Run executes the crawl until --max-pages is reached or the queue is
// exhausted. Returns the count of pages fetched + error.
func (c *Crawler) Run(ctx context.Context) (int, error) {
	if err := c.bootstrap(ctx); err != nil {
		return 0, err
	}
	if err := c.discoverInitialURLs(ctx); err != nil {
		return 0, fmt.Errorf("discover initial urls: %w", err)
	}

	concurrency := c.Cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		workerID := fmt.Sprintf("crawl-%d", i)
		go func(id string) {
			defer wg.Done()
			c.worker(ctx, id)
		}(workerID)
	}
	wg.Wait()
	return int(atomic.LoadInt64(&c.pagesFetched)), nil
}

func (c *Crawler) bootstrap(ctx context.Context) error {
	u, err := url.Parse(c.Cfg.Site)
	if err != nil {
		return err
	}
	c.BaseURL = u
	if !c.Cfg.IgnoreRobots {
		c.Robots = c.fetchRobots(ctx)
	}
	return nil
}

func (c *Crawler) fetchRobots(ctx context.Context) *robotstxt.RobotsData {
	robotsURL := c.BaseURL.Scheme + "://" + c.BaseURL.Host + "/robots.txt"
	req, _ := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
	resp, err := c.Client.Do(ctx, req)
	if err != nil {
		c.Logger.Warn("robots.txt unavailable",
			slog.String("event_type", logevt.DiscoverSitemapFailed),
			slog.String("phase", "crawl"),
			slog.String("url", robotsURL),
			slog.String("error", err.Error()),
		)
		return nil
	}
	defer httpx.DrainAndClose(resp)
	body, _, _ := c.Client.ReadBody(resp)
	rd, err := robotstxt.FromBytes(body)
	if err != nil {
		return nil
	}
	return rd
}

func (c *Crawler) discoverInitialURLs(ctx context.Context) error {
	// Enqueue homepage + seeds.
	if err := c.enqueueIfAllowed(ctx, c.Cfg.Site, "homepage", "", 0); err != nil {
		return err
	}
	for _, s := range c.Cfg.Seeds {
		if err := c.enqueueIfAllowed(ctx, s, "seed", c.Cfg.Site, 0); err != nil {
			return err
		}
	}
	// Try sitemap fan-out (robots.txt declared + standard fallbacks).
	sitemapURLs := c.candidateSitemapURLs()
	for _, sm := range sitemapURLs {
		c.discoverSitemap(ctx, sm, 0)
	}
	return nil
}

func (c *Crawler) candidateSitemapURLs() []string {
	out := []string{}
	if c.Robots != nil {
		out = append(out, c.Robots.Sitemaps...)
	}
	out = append(out, SitemapURLs(c.BaseURL.Scheme+"://"+c.BaseURL.Host)...)
	seen := map[string]bool{}
	uniq := make([]string, 0, len(out))
	for _, s := range out {
		if seen[s] {
			continue
		}
		seen[s] = true
		uniq = append(uniq, s)
	}
	return uniq
}

func (c *Crawler) discoverSitemap(ctx context.Context, sitemap string, depth int) {
	if depth > 5 {
		return
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", sitemap, nil)
	resp, err := c.Client.Do(ctx, req)
	if err != nil {
		c.Logger.Debug("sitemap unavailable",
			slog.String("event_type", logevt.DiscoverSitemapFailed),
			slog.String("phase", "crawl"),
			slog.String("url", sitemap),
		)
		return
	}
	if resp.StatusCode != 200 {
		httpx.DrainAndClose(resp)
		return
	}
	body, _, _ := c.Client.ReadBody(resp)
	resp.Body.Close()
	_ = ParseSitemap(bytes.NewReader(body), sitemap, func(e SitemapEntry) error {
		if e.IsIndex {
			c.discoverSitemap(ctx, e.Loc, depth+1)
			return nil
		}
		return c.enqueueIfAllowed(ctx, e.Loc, "sitemap_index", sitemap, 0)
	})
	c.Logger.Info("sitemap loaded",
		slog.String("event_type", logevt.DiscoverSitemapLoaded),
		slog.String("phase", "crawl"),
		slog.String("url", sitemap),
	)
}

func (c *Crawler) enqueueIfAllowed(ctx context.Context, raw, via, from string, depth int) error {
	canon, err := Canonicalize(raw)
	if err != nil {
		return nil
	}
	if !SameHost(c.Cfg.Site, canon) {
		return nil
	}
	if c.shouldExclude(canon) {
		return nil
	}
	if c.Robots != nil && !c.Cfg.IgnoreRobots {
		group := c.Robots.FindGroup(c.Cfg.UserAgent)
		if group != nil {
			parsed, _ := url.Parse(canon)
			if !group.Test(parsed.Path) {
				return nil
			}
		}
	}
	if c.Cfg.MaxDepth > 0 && depth > c.Cfg.MaxDepth {
		return nil
	}
	df := sql.NullString{}
	if from != "" {
		df = sql.NullString{Valid: true, String: from}
	}
	return c.DB.EnqueueURL(ctx, store.URL{
		URL:            canon,
		RunID:          c.RunID,
		DiscoveredVia:  via,
		DiscoveredFrom: df,
		Depth:          depth,
	})
}

func (c *Crawler) shouldExclude(canon string) bool {
	parsed, err := url.Parse(canon)
	if err != nil {
		return true
	}
	path := parsed.Path
	if len(c.Cfg.Include) > 0 {
		match := false
		for _, re := range c.Cfg.Include {
			if re.MatchString(path) {
				match = true
				break
			}
		}
		if !match {
			return true
		}
	}
	for _, re := range c.Cfg.Exclude {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func (c *Crawler) worker(ctx context.Context, workerID string) {
	for ctx.Err() == nil {
		if c.Cfg.MaxPages > 0 && int(atomic.LoadInt64(&c.pagesFetched)) >= c.Cfg.MaxPages {
			return
		}
		u, err := c.DB.ClaimURL(ctx, c.RunID, workerID, 5*time.Minute)
		if err != nil {
			c.Logger.Warn("claim error",
				slog.String("event_type", logevt.FetchFailedPermanent),
				slog.String("phase", "crawl"),
				slog.String("error", err.Error()),
			)
			return
		}
		if u == nil {
			return // queue empty
		}
		// Alias lookup before fetch.
		if canon, ok, _ := c.DB.LookupCanonical(ctx, u.URL); ok {
			_ = c.DB.MarkURLSkipped(ctx, u.URL, "aliased to "+canon)
			_ = c.enqueueIfAllowed(ctx, canon, "canonical_alias", u.URL, u.Depth)
			continue
		}
		c.fetchAndStore(ctx, workerID, *u)
	}
}

func (c *Crawler) fetchAndStore(ctx context.Context, workerID string, u store.URL) {
	c.Logger.Debug("fetch attempt",
		slog.String("event_type", logevt.FetchAttempt),
		slog.String("phase", "crawl"),
		slog.String("url", u.URL),
		slog.String("worker_id", workerID),
	)
	req, err := http.NewRequestWithContext(ctx, "GET", u.URL, nil)
	if err != nil {
		_ = c.DB.MarkURLFailed(ctx, u.URL, err.Error(), sql.NullTime{})
		return
	}
	resp, err := c.doWithRetry(ctx, req)
	if err != nil {
		c.Logger.Warn("fetch failed",
			slog.String("event_type", logevt.FetchFailedPermanent),
			slog.String("phase", "crawl"),
			slog.String("url", u.URL),
			slog.String("error", err.Error()),
		)
		_ = c.DB.MarkURLFailed(ctx, u.URL, err.Error(), sql.NullTime{})
		return
	}
	defer httpx.DrainAndClose(resp)
	finalURL := resp.Request.URL.String()
	canonicalFinal, _ := Canonicalize(finalURL)
	if canonicalFinal != u.URL && SameHost(c.Cfg.Site, canonicalFinal) {
		_ = c.DB.InsertURLAlias(ctx, u.URL, canonicalFinal, "http_redirect")
	}
	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode != 200 {
		_ = c.DB.MarkURLFailed(ctx, u.URL, fmt.Sprintf("status=%d", resp.StatusCode), sql.NullTime{})
		return
	}
	if !isHTML(contentType) {
		_ = c.DB.MarkURLSkipped(ctx, u.URL, "non-html: "+contentType)
		return
	}
	body, truncated, err := c.Client.ReadBody(resp)
	if err != nil {
		_ = c.DB.MarkURLFailed(ctx, u.URL, err.Error(), sql.NullTime{})
		return
	}
	doc, err := extract.Parse(bytes.NewReader(body))
	if err != nil {
		_ = c.DB.MarkURLFailed(ctx, u.URL, err.Error(), sql.NullTime{})
		return
	}
	md := extract.ExtractMetadata(doc, resp.Header)
	bodyText, wordCount, isShell := extract.ExtractBodyText(doc, c.Cfg.ThinThreshold)
	// Canonical-tag handling.
	if c.Cfg.RespectCanon && md.CanonicalURL != "" {
		if canon, err := Canonicalize(md.CanonicalURL); err == nil && canon != u.URL && SameHost(c.Cfg.Site, canon) {
			_ = c.DB.InsertURLAlias(ctx, u.URL, canon, "canonical_tag")
			_ = c.enqueueIfAllowed(ctx, canon, "canonical_tag", u.URL, u.Depth)
			_ = c.DB.MarkURLSkipped(ctx, u.URL, "canonical_tag -> "+canon)
			return
		}
	}
	page := store.Page{
		URL:             u.URL,
		FirstRunID:      c.RunID,
		FinalURL:        canonicalFinal,
		Title:           md.Title,
		MetaDescription: md.MetaDescription,
		BodyText:        bodyText,
		WordCount:       wordCount,
		IsEmptyShell:    isShell,
		Truncated:       truncated,
		StatusCode:      resp.StatusCode,
		ContentType:     contentType,
		RawBodySHA256:   hashBytes(body),
		BodySizeBytes:   len(body),
	}
	if md.LastModified != "" {
		page.LastModified.Valid = true
		page.LastModified.String = md.LastModified
	}
	if md.PublishedDate != "" {
		page.PublishedDate.Valid = true
		page.PublishedDate.String = md.PublishedDate
	}
	if err := c.DB.UpsertPage(ctx, page); err != nil {
		c.Logger.Error("upsert page",
			slog.String("event_type", logevt.FetchFailedPermanent),
			slog.String("phase", "crawl"),
			slog.String("url", u.URL),
			slog.String("error", err.Error()),
		)
		return
	}
	_ = c.DB.MarkURLFetched(ctx, u.URL, canonicalFinal, resp.StatusCode)
	atomic.AddInt64(&c.pagesFetched, 1)

	// Discover same-host links via BFS.
	base, _ := url.Parse(u.URL)
	links := extract.ExtractLinks(doc, base)
	for _, l := range links {
		if !l.IsInternal {
			continue
		}
		canon, err := Canonicalize(l.To)
		if err != nil {
			continue
		}
		_ = c.DB.InsertLink(ctx, u.URL, canon, l.Anchor, l.IsInternal)
		_ = c.enqueueIfAllowed(ctx, canon, "bfs_link", u.URL, u.Depth+1)
	}

	c.Logger.Debug("fetch success",
		slog.String("event_type", logevt.FetchSuccess),
		slog.String("phase", "crawl"),
		slog.String("url", u.URL),
		slog.Int("words", wordCount),
	)
}

func (c *Crawler) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	cfg := httpx.DefaultRetryConfig()
	if c.Cfg.MaxRetries > 0 {
		cfg.MaxAttempts = c.Cfg.MaxRetries
	}
	if c.Cfg.RetryBaseDelay > 0 {
		cfg.BaseDelay = c.Cfg.RetryBaseDelay
	}
	if c.Cfg.RetryMaxDelay > 0 {
		cfg.MaxDelay = c.Cfg.RetryMaxDelay
	}
	var (
		resp *http.Response
		last error
	)
	err := httpx.Do(ctx, cfg, nil, func(ctx context.Context, attempt int) (httpx.Action, time.Duration, error) {
		r, e := c.Client.Do(ctx, req.Clone(ctx))
		if e != nil {
			last = e
			if httpx.IsRetryable(e) {
				return httpx.ActionRetry, 0, e
			}
			return httpx.ActionAbort, 0, e
		}
		if r.StatusCode >= 500 || r.StatusCode == 408 || r.StatusCode == 425 || r.StatusCode == 429 {
			retryAfter := httpx.ParseRetryAfter(r.Header.Get("Retry-After"))
			httpx.DrainAndClose(r)
			last = fmt.Errorf("status %d", r.StatusCode)
			if httpx.IsRetryableStatus(r.StatusCode) {
				return httpx.ActionRetry, retryAfter, last
			}
			return httpx.ActionAbort, 0, last
		}
		resp = r
		return httpx.ActionDone, 0, nil
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, last
	}
	return resp, nil
}

func isHTML(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "text/html") || strings.HasPrefix(ct, "application/xhtml+xml") || ct == ""
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
