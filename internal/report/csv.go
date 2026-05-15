package report

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

// RenderCSV emits the CSV.
func RenderCSV(r *Report) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.UseCRLF = true
	_ = w.Write([]string{
		"url", "primary_label", "secondary_labels", "suggested_action", "priority",
		"confidence", "affected_prompts", "rationale", "word_count", "last_modified",
		"inbound_internal_links", "duplicate_cluster_id", "title",
	})
	for _, p := range r.Pages {
		_ = w.Write([]string{
			p.URL,
			p.PrimaryLabel,
			strings.Join(p.SecondaryLabels, "|"),
			p.SuggestedAction,
			fmt.Sprintf("%.1f", p.Priority),
			fmt.Sprintf("%.2f", p.Confidence),
			strings.Join(p.AffectedPrompts, "|"),
			p.Rationale,
			fmt.Sprintf("%d", p.Metadata.WordCount),
			p.Metadata.LastModified,
			fmt.Sprintf("%d", p.Metadata.InboundInternalLinks),
			p.Metadata.DuplicateClusterID,
			p.Title,
		})
	}
	w.Flush()
	return b.Bytes()
}
