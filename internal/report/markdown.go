package report

import (
	"bytes"
	"fmt"
	"strings"
)

// RenderMarkdown emits the human/agent-readable report.
// maxTopN caps the per-page detailed sections; 0 means all.
func RenderMarkdown(r *Report, maxTopN int) []byte {
	var b bytes.Buffer
	fmt.Fprintln(&b, "# redline Report")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "**Site:** %s\n", r.Site)
	fmt.Fprintf(&b, "**Run:** %s\n", r.RunID)
	if r.Provider.EmbeddingProvider != "" {
		fmt.Fprintf(&b, "**LLM:** %s / %s\n", r.Provider.LLMProvider, r.Provider.LLMModel)
		fmt.Fprintf(&b, "**Embeddings:** %s / %s\n", r.Provider.EmbeddingProvider, r.Provider.EmbeddingModel)
	} else {
		fmt.Fprintf(&b, "**LLM:** %s / %s\n", r.Provider.LLMProvider, r.Provider.LLMModel)
	}
	fmt.Fprintf(&b, "**Completed:** %s\n", r.CompletedAt)
	fmt.Fprintf(&b, "**Duration:** %ds\n", r.DurationSeconds)
	fmt.Fprintf(&b, "**API calls:** %d\n", r.Summary.TotalAPICalls)
	fmt.Fprintf(&b, "**Report version:** %s\n", r.RedlineReportVersion)
	fmt.Fprintf(&b, "**redline version:** %s\n", r.RedlineVersion)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## How to use this report")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "This report is the playbook for the editor agent that will apply changes to the live site. Each numbered section below corresponds to one page on the site that needs attention.")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Summary")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Action | Count |")
	fmt.Fprintln(&b, "| --- | --- |")
	for _, action := range []string{"REWRITE", "UPDATE", "DELETE", "MANUAL_REVIEW", "KEEP"} {
		fmt.Fprintf(&b, "| %s | %d |\n", action, r.Summary.ByAction[action])
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "**By primary label:** ")
	for _, lbl := range []string{"Aligned", "Stale", "OffBrand", "Contradictory", "Redundant", "Thin", "Zombie", "Unclear"} {
		if r.Summary.ByLabel[lbl] > 0 {
			fmt.Fprintf(&b, "%s %d ", lbl, r.Summary.ByLabel[lbl])
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "**Run stats:** %d pages · %d fetch failures · %d judge failures · %d retries.\n",
		r.Summary.PagesTotal, r.Summary.FetchFailures, r.Summary.JudgeFailures, r.Summary.RetriesTotal)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "---")
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Top priority pages")
	fmt.Fprintln(&b)

	limit := len(r.Pages)
	if maxTopN > 0 && maxTopN < limit {
		limit = maxTopN
	}
	rendered := 0
	for i := 0; i < limit; i++ {
		p := r.Pages[i]
		if p.PrimaryLabel == "Aligned" {
			continue
		}
		rendered++
		fmt.Fprintf(&b, "### %d. %s — %s\n", rendered, p.SuggestedAction, p.URL)
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "**Priority:** %.1f | **Confidence:** %.2f | **Word count:** %d | **Inbound internal links:** %d\n",
			p.Priority, p.Confidence, p.Metadata.WordCount, p.Metadata.InboundInternalLinks)
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "**Primary label:** %s\n", p.PrimaryLabel)
		if len(p.SecondaryLabels) > 0 {
			fmt.Fprintf(&b, "**Secondary labels:** %s\n", strings.Join(p.SecondaryLabels, ", "))
		}
		if len(p.AffectedPrompts) > 0 {
			fmt.Fprintf(&b, "**Affected prompts:** `%s`\n", strings.Join(p.AffectedPrompts, "`, `"))
		}
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "#### Page summary")
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "- **Current focus:** %s\n", p.PageSummary.CurrentFocus)
		if p.PageSummary.ShouldFocusOn != nil {
			fmt.Fprintf(&b, "- **Should focus on:** %s\n", *p.PageSummary.ShouldFocusOn)
		}
		fmt.Fprintln(&b)
		if len(p.Findings) > 0 {
			fmt.Fprintln(&b, "#### Findings")
			fmt.Fprintln(&b)
			for _, f := range p.Findings {
				fmt.Fprintf(&b, "##### Finding %s — %s (severity: %s)\n", f.ID, f.Kind, strings.ToUpper(f.Severity))
				fmt.Fprintln(&b)
				fmt.Fprintf(&b, "**Location:** %s\n", f.LocationHint)
				fmt.Fprintln(&b)
				fmt.Fprintln(&b, "**Quoted text on page:**")
				fmt.Fprintln(&b)
				for _, line := range strings.Split(f.QuotedText, "\n") {
					fmt.Fprintf(&b, "> %s\n", line)
				}
				fmt.Fprintln(&b)
				fmt.Fprintf(&b, "**Issue:** %s\n\n", f.Issue)
				fmt.Fprintf(&b, "**Suggested fix:** %s\n\n", f.SuggestedFix)
			}
		}
		if p.EditPlan != nil {
			fmt.Fprintln(&b, "#### Edit plan")
			fmt.Fprintln(&b)
			fmt.Fprintf(&b, "**Summary:** %s\n\n", p.EditPlan.Summary)
			renderListBlock(&b, "Preserve", p.EditPlan.Preserve)
			renderListBlock(&b, "Remove", p.EditPlan.Remove)
			if len(p.EditPlan.Rewrite) > 0 {
				fmt.Fprintln(&b, "**Rewrite:**")
				for _, rw := range p.EditPlan.Rewrite {
					fmt.Fprintf(&b, "- **%s**\n  - Currently: %s\n  - Replace with: %s\n", rw.Element, rw.CurrentFraming, rw.NewFraming)
				}
				fmt.Fprintln(&b)
			}
			renderListBlock(&b, "Add", p.EditPlan.Add)
		}
		if p.Rationale != "" {
			fmt.Fprintln(&b, "#### Rationale")
			fmt.Fprintln(&b)
			fmt.Fprintln(&b, p.Rationale)
			fmt.Fprintln(&b)
		}
		fmt.Fprintln(&b, "---")
		fmt.Fprintln(&b)
	}

	if len(r.DuplicateClusters) > 0 {
		fmt.Fprintln(&b, "## Duplicate clusters")
		fmt.Fprintln(&b)
		for _, c := range r.DuplicateClusters {
			fmt.Fprintf(&b, "### Cluster `%s` (%d pages, avg similarity %.2f)\n", c.ClusterID, c.Size, c.AvgSimilarity)
			fmt.Fprintln(&b)
			fmt.Fprintf(&b, "**Canonical (keep):** %s\n", c.CanonicalURL)
			fmt.Fprintln(&b)
			fmt.Fprintln(&b, "**Redundant:**")
			for _, m := range c.MemberURLs {
				if m == c.CanonicalURL {
					continue
				}
				fmt.Fprintf(&b, "- %s\n", m)
			}
			fmt.Fprintln(&b)
		}
	}

	return b.Bytes()
}

func renderListBlock(b *bytes.Buffer, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "**%s:**\n", title)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
	fmt.Fprintln(b)
}
