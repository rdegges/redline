package crawl

import (
	"strings"
	"testing"
)

func TestParseSitemap_URLSet(t *testing.T) {
	xml := `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc><lastmod>2024-01-01</lastmod></url>
  <url><loc>https://example.com/b</loc></url>
</urlset>`
	var got []SitemapEntry
	err := ParseSitemap(strings.NewReader(xml), "src", func(e SitemapEntry) error {
		got = append(got, e)
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Loc != "https://example.com/a" || got[0].LastModified != "2024-01-01" {
		t.Fatalf("entry 0: %+v", got[0])
	}
}

func TestParseSitemap_Index(t *testing.T) {
	xml := `<?xml version="1.0"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>https://example.com/a.xml</loc></sitemap>
  <sitemap><loc>https://example.com/b.xml</loc></sitemap>
</sitemapindex>`
	var got []SitemapEntry
	_ = ParseSitemap(strings.NewReader(xml), "src", func(e SitemapEntry) error {
		got = append(got, e)
		return nil
	})
	if len(got) != 2 || !got[0].IsIndex {
		t.Fatalf("got: %+v", got)
	}
}

func TestParseSitemap_MalformedEntrySkipped(t *testing.T) {
	xml := `<?xml version="1.0"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://example.com/a</loc></url>
  <url></url>
  <url><loc>https://example.com/b</loc></url>
</urlset>`
	var got []SitemapEntry
	_ = ParseSitemap(strings.NewReader(xml), "src", func(e SitemapEntry) error {
		got = append(got, e)
		return nil
	})
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestSitemapURLs_CandidatesIncludeAllFallbacks(t *testing.T) {
	cands := SitemapURLs("https://example.com")
	want := []string{
		"https://example.com/sitemap.xml",
		"https://example.com/sitemap_index.xml",
		"https://example.com/sitemaps.xml",
		"https://example.com/sitemap-index.xml",
	}
	if len(cands) != len(want) {
		t.Fatalf("len=%d want=%d", len(cands), len(want))
	}
	for i := range cands {
		if cands[i] != want[i] {
			t.Errorf("%d: %s != %s", i, cands[i], want[i])
		}
	}
}
