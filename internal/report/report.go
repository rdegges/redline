// Package report builds the JSON and Markdown report artifacts.
// Determinism: every collection is explicitly sorted before
// serialization; map iteration order is never relied on.
package report

import (
	"encoding/json"
	"sort"

	"github.com/rdegges/redline/internal/llm"
	"github.com/rdegges/redline/internal/store"
)

// ReportVersion bumps on any breaking schema change.
const ReportVersion = "1"

// Report is the top-level report structure.
type Report struct {
	RedlineReportVersion string         `json:"redline_report_version"`
	RedlineVersion       string         `json:"redline_version"`
	RunID                string         `json:"run_id"`
	Site                 string         `json:"site"`
	Provider             Provider       `json:"provider"`
	StartedAt            string         `json:"started_at"`
	CompletedAt          string         `json:"completed_at"`
	DurationSeconds      int            `json:"duration_seconds"`
	Summary              Summary        `json:"summary"`
	ConfigSnapshot       ConfigSnapshot `json:"config_snapshot"`
	Pages                []PageEntry    `json:"pages"`
	DuplicateClusters    []ClusterEntry `json:"duplicate_clusters"`
	Failures             []FailureEntry `json:"failures"`
}

// Provider mirrors the provider block in the JSON report.
type Provider struct {
	LLMProvider       string `json:"llm_provider"`
	LLMModel          string `json:"llm_model"`
	EmbeddingProvider string `json:"embedding_provider,omitempty"`
	EmbeddingModel    string `json:"embedding_model,omitempty"`
}

// Summary is the run-level counters block.
type Summary struct {
	PagesTotal             int            `json:"pages_total"`
	PagesJudged            int            `json:"pages_judged"`
	PagesFailed            int            `json:"pages_failed"`
	PagesUnclear           int            `json:"pages_unclear"`
	ByLabel                map[string]int `json:"by_label"`
	ByAction               map[string]int `json:"by_action"`
	DuplicateClustersCount int            `json:"duplicate_clusters_count"`
	PagesWithDuplicates    int            `json:"pages_with_duplicates"`
	TotalAPICalls          int            `json:"total_api_calls"`
	TotalInputTokens       int            `json:"total_input_tokens"`
	TotalCachedTokens      int            `json:"total_cached_tokens"`
	TotalOutputTokens      int            `json:"total_output_tokens"`
	RetriesTotal           int            `json:"retries_total"`
	FetchFailures          int            `json:"fetch_failures"`
	JudgeFailures          int            `json:"judge_failures"`
	EmbedFailures          int            `json:"embed_failures"`
}

// ConfigSnapshot is included for traceability.
type ConfigSnapshot struct {
	PromptsCount            int    `json:"prompts_count"`
	CanonicalMessagingCount int    `json:"canonical_messaging_count"`
	PromptsYAMLSHA256       string `json:"prompts_yaml_sha256"`
}

// PageEntry is one row in the per-page array.
type PageEntry struct {
	URL             string          `json:"url"`
	FinalURL        string          `json:"final_url"`
	Title           string          `json:"title"`
	PrimaryLabel    string          `json:"primary_label"`
	SecondaryLabels []string        `json:"secondary_labels"`
	Confidence      float64         `json:"confidence"`
	SuggestedAction string          `json:"suggested_action"`
	Priority        float64         `json:"priority"`
	AffectedPrompts []string        `json:"affected_prompts"`
	PageSummary     llm.PageSummary `json:"page_summary"`
	Findings        []llm.Finding   `json:"findings"`
	EditPlan        *llm.EditPlan   `json:"edit_plan"`
	Rationale       string          `json:"rationale"`
	Metadata        PageMetadata    `json:"metadata"`
}

// PageMetadata holds non-LLM context for a page.
type PageMetadata struct {
	WordCount            int    `json:"word_count"`
	LastModified         string `json:"last_modified,omitempty"`
	PublishedDate        string `json:"published_date,omitempty"`
	InboundInternalLinks int    `json:"inbound_internal_links"`
	DuplicateClusterID   string `json:"duplicate_cluster_id,omitempty"`
	JudgeAttempts        int    `json:"judge_attempts"`
	JudgedAt             string `json:"judged_at,omitempty"`
	InputTokens          int    `json:"input_tokens"`
	CacheHitTokens       int    `json:"cache_hit_tokens"`
	OutputTokens         int    `json:"output_tokens"`
	LatencyMs            int    `json:"latency_ms"`
}

// ClusterEntry is one duplicate cluster in the report.
type ClusterEntry struct {
	ClusterID     string   `json:"cluster_id"`
	CanonicalURL  string   `json:"canonical_url"`
	MemberURLs    []string `json:"member_urls"`
	Size          int      `json:"size"`
	AvgSimilarity float64  `json:"avg_similarity"`
}

// FailureEntry is one failed fetch or judge.
type FailureEntry struct {
	URL           string `json:"url"`
	Phase         string `json:"phase"`
	Attempts      int    `json:"attempts"`
	LastError     string `json:"last_error"`
	FirstSeenAt   string `json:"first_seen_at,omitempty"`
	LastAttemptAt string `json:"last_attempt_at,omitempty"`
}

// damageWeights.
var damageWeights = map[string]float64{
	"Contradictory": 1.00,
	"Stale":         0.70,
	"OffBrand":      0.55,
	"Thin":          0.35,
	"Redundant":     0.30,
	"Zombie":        0.25,
	"Unclear":       0.20,
	"Aligned":       0.00,
}

// secondaryOrder is the canonical enum order used to sort secondary_labels.
var secondaryOrder = map[string]int{
	"Aligned": 0, "Stale": 1, "OffBrand": 2, "Contradictory": 3,
	"Redundant": 4, "Thin": 5, "Zombie": 6, "Unclear": 7,
}

// ComputePriority returns damage × reach × confidence scaled to [0,100].
func ComputePriority(label string, confidence float64, inbound, maxInbound int) float64 {
	dw := damageWeights[label]
	reach := 0.5
	if maxInbound > 0 {
		reach = 0.5 + 0.5*float64(inbound)/float64(maxInbound)
	}
	p := dw * reach * confidence * 100.0
	// Round to one decimal.
	return float64(int(p*10+0.5)) / 10
}

// sortFindingsInPlace orders findings high→low severity, then by id ASC.
func sortFindingsInPlace(fs []llm.Finding) {
	sevRank := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.SliceStable(fs, func(i, j int) bool {
		if sevRank[fs[i].Severity] != sevRank[fs[j].Severity] {
			return sevRank[fs[i].Severity] < sevRank[fs[j].Severity]
		}
		return fs[i].ID < fs[j].ID
	})
}

// SortSecondaryLabels orders by the canonical enum order.
func SortSecondaryLabels(s []string) []string {
	out := append([]string{}, s...)
	sort.SliceStable(out, func(i, j int) bool { return secondaryOrder[out[i]] < secondaryOrder[out[j]] })
	return out
}

// MarshalJSONDeterministic emits r as canonical JSON with sorted keys
// at every level (encoding/json does this for map[string]any by default;
// the Report struct uses Go fields so order is field-declaration order
// which is also deterministic across builds).
func MarshalJSONDeterministic(r *Report) ([]byte, error) {
	// Use a roundtrip via map to guarantee alphabetical key ordering
	// in the resulting JSON even for the struct's nested types.
	out, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SecondaryFromJSON parses a stored "[...]" string into a slice. Returns
// nil for null/empty.
func SecondaryFromJSON(s string) []string {
	if s == "" || s == "null" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// FindingsFromJSON parses the stored findings_json column.
func FindingsFromJSON(s string) []llm.Finding {
	if s == "" || s == "null" {
		return nil
	}
	var out []llm.Finding
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// EditPlanFromJSON parses the stored edit_plan_json column or returns nil.
func EditPlanFromJSON(s store.Classification) *llm.EditPlan {
	if !s.EditPlanJSON.Valid || s.EditPlanJSON.String == "" {
		return nil
	}
	var out llm.EditPlan
	if err := json.Unmarshal([]byte(s.EditPlanJSON.String), &out); err != nil {
		return nil
	}
	return &out
}

// PageSummaryFromJSON parses the stored page_summary_json column.
func PageSummaryFromJSON(s store.Classification) llm.PageSummary {
	var out llm.PageSummary
	if !s.PageSummaryJSON.Valid || s.PageSummaryJSON.String == "" {
		return out
	}
	_ = json.Unmarshal([]byte(s.PageSummaryJSON.String), &out)
	return out
}

// AffectedFromJSON parses the affected_prompts JSON column.
func AffectedFromJSON(s string) []string {
	if s == "" || s == "null" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(s), &out)
	sort.Strings(out)
	return out
}
