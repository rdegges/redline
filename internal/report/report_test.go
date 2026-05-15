package report

import (
	"context"
	"strings"
	"testing"

	"github.com/rdegges/redline/internal/store"
)

func TestRenderMarkdown_BasicHeader(t *testing.T) {
	r := minimalReport()
	md := RenderMarkdown(r, 100)
	out := string(md)
	for _, want := range []string{"# redline Report", "**Site:**", "## Summary"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in MD output", want)
		}
	}
}

func TestComputePriority_DamageReachConfidence(t *testing.T) {
	p := ComputePriority("Contradictory", 1.0, 10, 10)
	if p != 100.0 {
		t.Fatalf("max priority = %v, want 100", p)
	}
	if got := ComputePriority("Aligned", 1.0, 10, 10); got != 0 {
		t.Fatalf("Aligned should be 0, got %v", got)
	}
}

func TestBuild_DeterministicTwice(t *testing.T) {
	in := minimalInput()
	r1 := Build(context.Background(), in)
	r2 := Build(context.Background(), in)
	b1, _ := MarshalJSONDeterministic(r1)
	b2, _ := MarshalJSONDeterministic(r2)
	if string(b1) != string(b2) {
		t.Fatal("two Build calls produced different JSON")
	}
}

func minimalReport() *Report {
	return &Report{
		RedlineReportVersion: "1",
		RedlineVersion:       "0.0.0-test",
		RunID:                "r1",
		Site:                 "http://x",
		Provider:             Provider{LLMProvider: "ollama", LLMModel: "qwen3:30b"},
		CompletedAt:          "2026-05-13T00:00:00Z",
		Summary:              Summary{ByLabel: map[string]int{}, ByAction: map[string]int{}},
	}
}

func minimalInput() BuildInput {
	return BuildInput{
		Run: store.Run{ID: "r1", SiteURL: "http://x", LLMProvider: "ollama", LLMModel: "qwen3:30b"},
	}
}
