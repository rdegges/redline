package httpx

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// Client is the shared HTTP client used by the crawler and provider
// clients. It bundles a stdlib *http.Client with a per-host token-bucket
// rate limiter.
type Client struct {
	HTTP    *http.Client
	Limiter *rate.Limiter
	UA      string
	MaxBody int64 // bytes; default 5 MiB
}

// NewClient returns a Client with sensible defaults.
func NewClient(timeout time.Duration, qps float64, ua string) *Client {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if qps <= 0 {
		qps = 2.0
	}
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	return &Client{
		HTTP:    &http.Client{Transport: tr, Timeout: timeout},
		Limiter: rate.NewLimiter(rate.Limit(qps), 1),
		UA:      ua,
		MaxBody: 5 * 1024 * 1024,
	}
}

// Do issues an HTTP request honoring the rate limiter and decoding gzip.
// Caller is responsible for closing the body.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := c.Limiter.Wait(ctx); err != nil {
		return nil, err
	}
	if c.UA != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.UA)
	}
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gr, gerr := gzip.NewReader(resp.Body)
		if gerr != nil {
			DrainAndClose(resp)
			return nil, fmt.Errorf("gunzip: %w", gerr)
		}
		resp.Body = &gzippedBody{r: gr, orig: resp.Body}
		resp.Header.Del("Content-Encoding")
	}
	return resp, nil
}

// ReadBody reads up to c.MaxBody bytes. Returns (body, truncated, err).
func (c *Client) ReadBody(resp *http.Response) ([]byte, bool, error) {
	if resp.Body == nil {
		return nil, false, nil
	}
	limited := io.LimitReader(resp.Body, c.MaxBody+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	if int64(len(buf)) > c.MaxBody {
		return buf[:c.MaxBody], true, nil
	}
	return buf, false, nil
}

type gzippedBody struct {
	r    *gzip.Reader
	orig io.ReadCloser
}

func (g *gzippedBody) Read(p []byte) (int, error) { return g.r.Read(p) }
func (g *gzippedBody) Close() error {
	_ = g.r.Close()
	return g.orig.Close()
}
