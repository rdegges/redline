package report

import (
	"context"
	"fmt"
	"sort"

	"github.com/rdegges/redline/internal/config"
	"github.com/rdegges/redline/internal/store"
)

// BuildInput captures everything Build needs to assemble a Report.
type BuildInput struct {
	Run                  store.Run
	Pages                []store.Page
	Classifications      []store.Classification
	Duplicates           []store.DuplicatePair
	InboundLinks         map[string]int
	APICalls             int
	Retries              int
	InputTokens          int
	CachedTokens         int
	OutputTokens         int
	FailureRows          []FailureEntry
	RedlineVersion       string
	ConfigPromptCount    int
	ConfigMessagingCount int
}

// Build assembles a Report from DB-derived inputs. The function is pure;
// every collection is explicitly sorted before being returned.
func Build(_ context.Context, in BuildInput) *Report {
	pageByURL := map[string]store.Page{}
	for _, p := range in.Pages {
		pageByURL[p.URL] = p
	}
	classByURL := map[string]store.Classification{}
	for _, c := range in.Classifications {
		classByURL[c.PageURL] = c
	}
	maxInbound := 0
	for _, n := range in.InboundLinks {
		if n > maxInbound {
			maxInbound = n
		}
	}
	// Group duplicates by cluster id.
	clusterByID := map[string]*ClusterEntry{}
	pageClusterID := map[string]string{}
	for _, d := range in.Duplicates {
		ce, ok := clusterByID[d.ClusterID]
		if !ok {
			ce = &ClusterEntry{ClusterID: d.ClusterID}
			clusterByID[d.ClusterID] = ce
		}
		ce.AvgSimilarity = ((ce.AvgSimilarity * float64(len(ce.MemberURLs))) + d.Similarity) / float64(len(ce.MemberURLs)+1)
		members := map[string]bool{}
		for _, m := range ce.MemberURLs {
			members[m] = true
		}
		for _, m := range []string{d.PageURLA, d.PageURLB} {
			if !members[m] {
				ce.MemberURLs = append(ce.MemberURLs, m)
				pageClusterID[m] = d.ClusterID
			}
		}
	}
	// Sort and finalize clusters.
	clusters := make([]ClusterEntry, 0, len(clusterByID))
	for _, c := range clusterByID {
		sort.Strings(c.MemberURLs)
		c.Size = len(c.MemberURLs)
		// canonical URL: highest word count then lex-lowest.
		canon := c.MemberURLs[0]
		for _, m := range c.MemberURLs[1:] {
			if pageByURL[m].WordCount > pageByURL[canon].WordCount {
				canon = m
			}
		}
		c.CanonicalURL = canon
		clusters = append(clusters, *c)
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].ClusterID < clusters[j].ClusterID })

	// Build per-page entries.
	pageEntries := make([]PageEntry, 0, len(in.Pages))
	byLabel := map[string]int{}
	byAction := map[string]int{}
	judged := 0
	failed := 0
	unclear := 0
	for _, p := range in.Pages {
		c := classByURL[p.URL]
		findings := FindingsFromJSON(c.FindingsJSON)
		sortFindingsInPlace(findings)
		secondary := SortSecondaryLabels(SecondaryFromJSON(c.SecondaryLabels))
		affected := AffectedFromJSON(c.AffectedPrompts)
		priority := ComputePriority(c.PrimaryLabel, c.Confidence, in.InboundLinks[p.URL], maxInbound)
		entry := PageEntry{
			URL:             p.URL,
			FinalURL:        p.FinalURL,
			Title:           p.Title,
			PrimaryLabel:    c.PrimaryLabel,
			SecondaryLabels: secondary,
			Confidence:      c.Confidence,
			SuggestedAction: c.SuggestedAction,
			Priority:        priority,
			AffectedPrompts: affected,
			PageSummary:     PageSummaryFromJSON(c),
			Findings:        findings,
			EditPlan:        EditPlanFromJSON(c),
			Rationale:       c.Rationale,
			Metadata: PageMetadata{
				WordCount:            p.WordCount,
				LastModified:         nullStringValue(p.LastModified.String, p.LastModified.Valid),
				PublishedDate:        nullStringValue(p.PublishedDate.String, p.PublishedDate.Valid),
				InboundInternalLinks: in.InboundLinks[p.URL],
				DuplicateClusterID:   pageClusterID[p.URL],
				JudgeAttempts:        c.AttemptCount,
				InputTokens:          c.InputTokens,
				CacheHitTokens:       c.CacheHitTokens,
				OutputTokens:         c.OutputTokens,
				LatencyMs:            c.LatencyMs,
			},
		}
		if c.JudgedAt.Valid {
			entry.Metadata.JudgedAt = c.JudgedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		pageEntries = append(pageEntries, entry)
		if c.PrimaryLabel != "" {
			byLabel[c.PrimaryLabel]++
		}
		if c.SuggestedAction != "" {
			byAction[c.SuggestedAction]++
		}
		switch c.JudgeStatus {
		case store.JudgeJudged:
			judged++
		case store.JudgeFailed, store.JudgeFailedSchema:
			failed++
		}
		if c.PrimaryLabel == "Unclear" {
			unclear++
		}
	}
	sort.Slice(pageEntries, func(i, j int) bool {
		if pageEntries[i].Priority != pageEntries[j].Priority {
			return pageEntries[i].Priority > pageEntries[j].Priority
		}
		return pageEntries[i].URL < pageEntries[j].URL
	})

	// Pages with duplicates.
	pagesWithDups := map[string]bool{}
	for _, c := range clusters {
		for _, m := range c.MemberURLs {
			pagesWithDups[m] = true
		}
	}

	provider := Provider{LLMProvider: in.Run.LLMProvider, LLMModel: in.Run.LLMModel}
	if in.Run.EmbeddingProvider.Valid {
		provider.EmbeddingProvider = in.Run.EmbeddingProvider.String
	}
	if in.Run.EmbeddingModel.Valid {
		provider.EmbeddingModel = in.Run.EmbeddingModel.String
	}

	completed := ""
	durationSec := 0
	if in.Run.CompletedAt.Valid {
		completed = in.Run.CompletedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		durationSec = int(in.Run.CompletedAt.Time.Sub(in.Run.StartedAt).Seconds())
	}

	failures := append([]FailureEntry{}, in.FailureRows...)
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Phase != failures[j].Phase {
			return failures[i].Phase < failures[j].Phase
		}
		return failures[i].URL < failures[j].URL
	})

	r := &Report{
		RedlineReportVersion: ReportVersion,
		RedlineVersion:       in.RedlineVersion,
		RunID:                in.Run.ID,
		Site:                 in.Run.SiteURL,
		Provider:             provider,
		StartedAt:            in.Run.StartedAt.UTC().Format("2006-01-02T15:04:05Z"),
		CompletedAt:          completed,
		DurationSeconds:      durationSec,
		Summary: Summary{
			PagesTotal:             len(in.Pages),
			PagesJudged:            judged,
			PagesFailed:            failed,
			PagesUnclear:           unclear,
			ByLabel:                byLabel,
			ByAction:               byAction,
			DuplicateClustersCount: len(clusters),
			PagesWithDuplicates:    len(pagesWithDups),
			TotalAPICalls:          in.APICalls,
			TotalInputTokens:       in.InputTokens,
			TotalCachedTokens:      in.CachedTokens,
			TotalOutputTokens:      in.OutputTokens,
			RetriesTotal:           in.Retries,
			FetchFailures:          len(failures),
		},
		ConfigSnapshot: ConfigSnapshot{
			PromptsCount:            in.ConfigPromptCount,
			CanonicalMessagingCount: in.ConfigMessagingCount,
			PromptsYAMLSHA256:       in.Run.PromptsSHA256,
		},
		Pages:             pageEntries,
		DuplicateClusters: clusters,
		Failures:          failures,
	}
	return r
}

func nullStringValue(s string, valid bool) string {
	if !valid {
		return ""
	}
	return s
}

// ConfigFromFile copies the loaded config metadata into BuildInput counts.
func ConfigFromFile(cfg *config.File) (promptCount, messagingCount int) {
	if cfg == nil {
		return 0, 0
	}
	return len(cfg.Prompts), len(cfg.CanonicalMessaging)
}

var _ = fmt.Sprintf
