package report

import (
	"strings"
	"testing"

	"github.com/rdegges/redline/internal/llm"
)

func TestRenderCSV_HeaderAndRow(t *testing.T) {
	r := &Report{
		Pages: []PageEntry{{
			URL:             "http://x/a",
			PrimaryLabel:    "Stale",
			SecondaryLabels: []string{"OffBrand"},
			SuggestedAction: "UPDATE",
			Priority:        42.5,
			Confidence:      0.85,
			AffectedPrompts: []string{"p1", "p2"},
			Rationale:       "stale",
			Metadata:        PageMetadata{WordCount: 100, LastModified: "2024-01-01", InboundInternalLinks: 3},
			Title:           "Title",
			PageSummary:     llm.PageSummary{CurrentFocus: "f"},
		}},
	}
	csv := string(RenderCSV(r))
	if !strings.Contains(csv, "url,primary_label") {
		t.Fatalf("missing header: %s", csv)
	}
	if !strings.Contains(csv, "http://x/a") || !strings.Contains(csv, "Stale") {
		t.Fatalf("missing row data: %s", csv)
	}
	if !strings.Contains(csv, "OffBrand") || !strings.Contains(csv, "p1|p2") {
		t.Fatalf("pipe-delimited columns missing: %s", csv)
	}
}
