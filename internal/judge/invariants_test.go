package judge

import (
	"testing"

	"github.com/rdegges/redline/internal/config"
	"github.com/rdegges/redline/internal/llm"
	"github.com/rdegges/redline/internal/store"
)

func newJudge() *Judge { return &Judge{} }

func TestValidateInvariants_ManualReviewHighConfidence_AutoCaps(t *testing.T) {
	j := newJudge()
	r := &llm.JudgeResponse{
		PrimaryLabel:    "Unclear",
		Confidence:      0.92,
		SuggestedAction: "MANUAL_REVIEW",
	}
	if err := j.validateInvariants(r, store.Page{}, &config.File{}); err != nil {
		t.Fatalf("expected auto-correction, got error: %v", err)
	}
	if r.Confidence >= 0.6 {
		t.Fatalf("confidence not capped: %v", r.Confidence)
	}
}

func TestValidateInvariants_ConfidenceClampsOutOfRange(t *testing.T) {
	j := newJudge()
	cases := []struct {
		in, want float64
	}{
		{95.0, 0.95}, // percentage-style
		{-0.5, 0.0},  // negative
		{1.7, 1.0},   // > 1 but not percentage-shaped
	}
	for _, c := range cases {
		r := &llm.JudgeResponse{
			PrimaryLabel:    "Aligned",
			Confidence:      c.in,
			SuggestedAction: "KEEP",
		}
		if err := j.validateInvariants(r, store.Page{}, &config.File{}); err != nil {
			t.Fatalf("in=%v: %v", c.in, err)
		}
		if r.Confidence != c.want {
			t.Errorf("in=%v: got %v want %v", c.in, r.Confidence, c.want)
		}
	}
}

func TestValidateInvariants_AlignedWithFindings_StillRetries(t *testing.T) {
	j := newJudge()
	r := &llm.JudgeResponse{
		PrimaryLabel:    "Aligned",
		Confidence:      0.9,
		SuggestedAction: "KEEP",
		Findings:        []llm.Finding{{ID: "f1", QuotedText: "x"}},
	}
	if err := j.validateInvariants(r, store.Page{BodyText: "x"}, &config.File{}); err == nil {
		t.Fatal("expected structural invariant violation to error")
	}
}

func TestValidateInvariants_UpdateWithoutEditPlan_StillRetries(t *testing.T) {
	j := newJudge()
	r := &llm.JudgeResponse{
		PrimaryLabel:    "Stale",
		Confidence:      0.8,
		SuggestedAction: "UPDATE",
		Findings:        []llm.Finding{{ID: "f1", QuotedText: "x"}},
		EditPlan:        nil,
	}
	if err := j.validateInvariants(r, store.Page{BodyText: "x"}, &config.File{}); err == nil {
		t.Fatal("expected error for UPDATE without edit_plan")
	}
}
