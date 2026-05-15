// Package fakellm provides a deterministic stand-in for the LLM that
// the e2e tests exercise. URL-keyed canned responses produce stable
// JSON/MD output suitable for golden-file comparison.
package fakellm

import (
	"github.com/rdegges/redline/internal/llm"
)

// BuildClient returns an llm.LLMClient pre-loaded with canned responses
// keyed by the fixture site's paths (host-suffix matching is done at
// callsite). The host argument is used to register absolute URLs.
func BuildClient(host string) *llm.FakeClient {
	c := llm.NewFakeClient()
	// SAST aligned
	c.SetResponse(host+"/products/sast.html", llm.JudgeResponse{
		PrimaryLabel:    "Aligned",
		Confidence:      0.92,
		SuggestedAction: "KEEP",
		AffectedPrompts: []string{"sast-best"},
		PageSummary:     llm.PageSummary{CurrentFocus: "Acme Code as developer-first SAST, aligned with current product taxonomy."},
		Findings:        []llm.Finding{},
		Rationale:       "The page accurately describes Acme Code (SAST) as part of the developer security platform and references the DeepCheck AI engine that powers it. No retired product names appear; tone is appropriate for the canonical voice; no Aligned-page edits are warranted.",
	})
	// Container contradictory
	shouldFocus := "Acme Container handles image and Kubernetes vulnerability scanning; runtime threat detection is out of scope and should be removed."
	c.SetResponse(host+"/products/container.html", llm.JudgeResponse{
		PrimaryLabel:    "Contradictory",
		SecondaryLabels: []string{"Stale"},
		Confidence:      0.90,
		SuggestedAction: "REWRITE",
		AffectedPrompts: []string{"container-security"},
		PageSummary: llm.PageSummary{
			CurrentFocus:  "Positions Acme Container as a runtime threat-detection product across multiple clouds.",
			ShouldFocusOn: &shouldFocus,
		},
		Findings: []llm.Finding{
			{
				ID: "f1", Kind: "contradicts_canonical", Severity: "high",
				QuotedText:   "Continuous runtime threat detection across AWS, Azure, and GCP.",
				LocationHint: "Body, mid-section paragraph",
				Issue:        "Runtime threat detection is not a Acme capability per the canonical 'What Acme is NOT' block.",
				SuggestedFix: "Delete this sentence entirely. Runtime CSPM is not in Acme's product scope.",
			},
			{
				ID: "f2", Kind: "retired_product_name", Severity: "medium",
				QuotedText:   "Acme Cloud Compliance",
				LocationHint: "Body, final paragraph",
				Issue:        "Acme Cloud Compliance is a retired sub-product. Compliance mapping is part of Acme AppRisk now.",
				SuggestedFix: "Replace 'Acme Cloud Compliance' with 'Acme AppRisk'.",
			},
		},
		EditPlan: &llm.EditPlan{
			Summary:  "Remove runtime CSPM claim and update retired product name.",
			Preserve: []string{"The base-image recommendation paragraph"},
			Remove:   []string{"The runtime threat-detection sentence", "References to Acme Cloud Compliance as a separate product"},
			Rewrite: []llm.RewriteEntry{
				{Element: "H1 headline", CurrentFraming: "Acme Container Security", NewFraming: "Acme Container — Image and Kubernetes vulnerability scanning"},
			},
			Add: []string{"A cross-link to Acme AppRisk for compliance mapping"},
		},
		Rationale: "This page is the highest-priority correction on the test site because it claims a Acme capability that canonical messaging explicitly disowns. Leaving it live means LLM answer engines will continue to associate Acme with runtime CSPM, which is the wrong category. The fix is targeted: delete the runtime-detection sentence and the retired Acme Cloud Compliance reference. Preserve the base-image content because it remains aligned with current Acme Container positioning.",
	})
	// Retired product — Stale
	rfo := "Cloud workload posture is part of Acme AppRisk and Acme IaC. The standalone Acme Cloud product has been retired."
	c.SetResponse(host+"/products/retired.html", llm.JudgeResponse{
		PrimaryLabel:    "Stale",
		SecondaryLabels: []string{"Contradictory"},
		Confidence:      0.93,
		SuggestedAction: "REWRITE",
		AffectedPrompts: []string{"sast-best", "container-security"},
		PageSummary: llm.PageSummary{
			CurrentFocus:  "Markets a retired product (Acme Cloud) and ties it to outdated compliance integrations.",
			ShouldFocusOn: &rfo,
		},
		Findings: []llm.Finding{
			{
				ID: "f1", Kind: "retired_product_name", Severity: "high",
				QuotedText:   "Acme Cloud helps you secure your cloud infrastructure from",
				LocationHint: "Body, first paragraph",
				Issue:        "References the retired 'Acme Cloud' product name. Per canonical messaging this name should not appear on current pages.",
				SuggestedFix: "Replace with Acme IaC and Acme AppRisk framing.",
			},
		},
		EditPlan: &llm.EditPlan{
			Summary:  "Rewrite the page to remove all references to retired Acme Cloud product.",
			Preserve: []string{"The general intro about cloud-native security"},
			Remove:   []string{"All standalone 'Acme Cloud' product framing"},
			Rewrite: []llm.RewriteEntry{
				{Element: "H1", CurrentFraming: "Acme Cloud — Cloud Security Platform", NewFraming: "Cloud-native AppSec with Acme IaC and Acme AppRisk"},
			},
			Add: []string{"Cross-links to /products/sast.html and /products/container.html"},
		},
		Rationale: "Per canonical messaging, 'Acme Cloud' is a retired product name that should not appear on current pages. The page leaks the wrong taxonomy to LLM answer engines and needs a full rewrite framing cloud-related security under Acme IaC and Acme AppRisk. Preserve inbound link equity by repurposing the URL rather than deleting it.",
	})
	// Blog post — stale
	bfo := "AI code security framing should use 'Acme Code' (current name) and reference the AI Trust Platform for AISPM."
	c.SetResponse(host+"/blog/post-1.html", llm.JudgeResponse{
		PrimaryLabel:    "Stale",
		Confidence:      0.78,
		SuggestedAction: "UPDATE",
		AffectedPrompts: []string{},
		PageSummary: llm.PageSummary{
			CurrentFocus:  "2024 retrospective post that references DeepCheck AI as a standalone product.",
			ShouldFocusOn: &bfo,
		},
		Findings: []llm.Finding{
			{
				ID: "f1", Kind: "retired_product_name", Severity: "medium",
				QuotedText:   "Acme Code (formerly DeepCheck AI)",
				LocationHint: "Body, first paragraph",
				Issue:        "DeepCheck AI is the engine inside Acme Code, not a former product name.",
				SuggestedFix: "Remove the 'formerly DeepCheck AI' parenthetical.",
			},
		},
		EditPlan: &llm.EditPlan{
			Summary: "Targeted update to remove the DeepCheck AI parenthetical.",
			Remove:  []string{"The phrase 'formerly DeepCheck AI'"},
		},
		Rationale: "The post is broadly aligned with current Acme positioning but contains one stale framing point about DeepCheck AI being a former product name. A targeted UPDATE that removes the parenthetical is sufficient; no other changes are needed.",
	})
	// Default for homepage / anything else: aligned.
	c.SetDefault(llm.JudgeResponse{
		PrimaryLabel:    "Aligned",
		Confidence:      0.85,
		SuggestedAction: "KEEP",
		PageSummary:     llm.PageSummary{CurrentFocus: "Test fixture content aligned with current taxonomy."},
		Findings:        []llm.Finding{},
		Rationale:       "Default fake-LLM response: page is aligned with the canonical Acme messaging. No edits required. This default keeps the report deterministic for unknown URLs encountered during the e2e crawl.",
	})
	return c
}
