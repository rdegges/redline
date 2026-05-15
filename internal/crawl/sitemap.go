package crawl

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// SitemapEntry is one URL discovered via a sitemap.
type SitemapEntry struct {
	Loc          string
	LastModified string
	Source       string // sitemap URL it came from
	IsIndex      bool   // when true, Loc points to a nested sitemap
}

// ParseSitemap streams an XML sitemap or sitemap-index, yielding entries
// via callback. depth caps recursive sitemap-index processing.
// The reader is closed by the caller.
func ParseSitemap(r io.Reader, sourceURL string, onEntry func(SitemapEntry) error) error {
	d := xml.NewDecoder(r)
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity

	var (
		inLoc      bool
		inSitemap  bool
		inURL      bool
		inLastMod  bool
		curLoc     strings.Builder
		curLastMod strings.Builder
	)
	flush := func(isIndex bool) error {
		loc := strings.TrimSpace(curLoc.String())
		lastMod := strings.TrimSpace(curLastMod.String())
		curLoc.Reset()
		curLastMod.Reset()
		if loc == "" {
			return nil
		}
		return onEntry(SitemapEntry{Loc: loc, LastModified: lastMod, Source: sourceURL, IsIndex: isIndex})
	}

	for {
		tok, err := d.Token()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("parse sitemap: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "sitemap":
				inSitemap = true
			case "url":
				inURL = true
			case "loc":
				inLoc = true
			case "lastmod":
				inLastMod = true
			}
		case xml.CharData:
			if inLoc {
				curLoc.Write(t)
			} else if inLastMod {
				curLastMod.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "loc":
				inLoc = false
			case "lastmod":
				inLastMod = false
			case "url":
				if err := flush(false); err != nil {
					return err
				}
				inURL = false
			case "sitemap":
				if err := flush(true); err != nil {
					return err
				}
				inSitemap = false
			}
		}
		_ = inSitemap
		_ = inURL
	}
}

// MaybeGunzip wraps r in a gzip.Reader if header says so or URL ends .gz.
func MaybeGunzip(r io.ReadCloser, hdr http.Header, url string) (io.Reader, func() error, error) {
	encoding := strings.ToLower(hdr.Get("Content-Encoding"))
	if encoding == "gzip" || strings.HasSuffix(strings.ToLower(url), ".gz") {
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, r.Close, err
		}
		return gr, func() error { _ = gr.Close(); return r.Close() }, nil
	}
	return r, r.Close, nil
}

// SitemapURLs returns the candidate sitemap URL paths to probe when
// robots.txt doesn't declare one. step 2.
func SitemapURLs(host string) []string {
	host = strings.TrimRight(host, "/")
	return []string{
		host + "/sitemap.xml",
		host + "/sitemap_index.xml",
		host + "/sitemaps.xml",
		host + "/sitemap-index.xml",
	}
}

var _ = context.Background
