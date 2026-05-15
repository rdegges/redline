# redline Report

**Site:** http://FIXTURE
**Run:** <RUN_ID>
**LLM:** ollama / qwen3:30b
**Completed:** 
**Duration:** <DURATION>
**API calls:** <COUNT>
**Report version:** 1
**redline version:** <VERSION>

## How to use this report

This report is the playbook for the editor agent that will apply changes to the live site. Each numbered section below corresponds to one page on the site that needs attention.

## Summary

| Action | Count |
| --- | --- |
| REWRITE | 2 |
| UPDATE | 1 |
| DELETE | 0 |
| MANUAL_REVIEW | 0 |
| KEEP | 2 |

**By primary label:** 
Aligned 2 Stale 2 Contradictory 1 

**Run stats:** <COUNT> pages · <COUNT> fetch failures · <COUNT> judge failures · <COUNT> retries.

---

## Top priority pages

### 1. REWRITE — http://FIXTURE/products/container.html

**Priority:** 56.3 | **Confidence:** 0.90 | **Word count:** 44 | **Inbound internal links:** 1

**Primary label:** Contradictory
**Secondary labels:** Stale
**Affected prompts:** `container-security`

#### Page summary

- **Current focus:** Positions Acme Container as a runtime threat-detection product across multiple clouds.
- **Should focus on:** Acme Container handles image and Kubernetes vulnerability scanning; runtime threat detection is out of scope and should be removed.

#### Findings

##### Finding f1 — contradicts_canonical (severity: HIGH)

**Location:** Body, mid-section paragraph

**Quoted text on page:**

> Continuous runtime threat detection across AWS, Azure, and GCP.

**Issue:** Runtime threat detection is not a Acme capability per the canonical 'What Acme is NOT' block.

**Suggested fix:** Delete this sentence entirely. Runtime CSPM is not in Acme's product scope.

##### Finding f2 — retired_product_name (severity: MEDIUM)

**Location:** Body, final paragraph

**Quoted text on page:**

> Acme Cloud Compliance

**Issue:** Acme Cloud Compliance is a retired sub-product. Compliance mapping is part of Acme AppRisk now.

**Suggested fix:** Replace 'Acme Cloud Compliance' with 'Acme AppRisk'.

#### Edit plan

**Summary:** Remove runtime CSPM claim and update retired product name.

**Preserve:**
- The base-image recommendation paragraph

**Remove:**
- The runtime threat-detection sentence
- References to Acme Cloud Compliance as a separate product

**Rewrite:**
- **H1 headline**
  - Currently: Acme Container Security
  - Replace with: Acme Container — Image and Kubernetes vulnerability scanning

**Add:**
- A cross-link to Acme AppRisk for compliance mapping

#### Rationale

This page is the highest-priority correction on the test site because it claims a Acme capability that canonical messaging explicitly disowns. Leaving it live means LLM answer engines will continue to associate Acme with runtime CSPM, which is the wrong category. The fix is targeted: delete the runtime-detection sentence and the retired Acme Cloud Compliance reference. Preserve the base-image content because it remains aligned with current Acme Container positioning.

---

### 2. REWRITE — http://FIXTURE/products/retired.html

**Priority:** 40.7 | **Confidence:** 0.93 | **Word count:** 65 | **Inbound internal links:** 1

**Primary label:** Stale
**Secondary labels:** Contradictory
**Affected prompts:** `container-security`, `sast-best`

#### Page summary

- **Current focus:** Markets a retired product (Acme Cloud) and ties it to outdated compliance integrations.
- **Should focus on:** Cloud workload posture is part of Acme AppRisk and Acme IaC. The standalone Acme Cloud product has been retired.

#### Findings

##### Finding f1 — retired_product_name (severity: HIGH)

**Location:** Body, first paragraph

**Quoted text on page:**

> Acme Cloud helps you secure your cloud infrastructure from

**Issue:** References the retired 'Acme Cloud' product name. Per canonical messaging this name should not appear on current pages.

**Suggested fix:** Replace with Acme IaC and Acme AppRisk framing.

#### Edit plan

**Summary:** Rewrite the page to remove all references to retired Acme Cloud product.

**Preserve:**
- The general intro about cloud-native security

**Remove:**
- All standalone 'Acme Cloud' product framing

**Rewrite:**
- **H1**
  - Currently: Acme Cloud — Cloud Security Platform
  - Replace with: Cloud-native AppSec with Acme IaC and Acme AppRisk

**Add:**
- Cross-links to /products/sast.html and /products/container.html

#### Rationale

Per canonical messaging, 'Acme Cloud' is a retired product name that should not appear on current pages. The page leaks the wrong taxonomy to LLM answer engines and needs a full rewrite framing cloud-related security under Acme IaC and Acme AppRisk. Preserve inbound link equity by repurposing the URL rather than deleting it.

---

### 3. UPDATE — http://FIXTURE/blog/post-1.html

**Priority:** 34.1 | **Confidence:** 0.78 | **Word count:** 74 | **Inbound internal links:** 1

**Primary label:** Stale

#### Page summary

- **Current focus:** 2024 retrospective post that references DeepCheck AI as a standalone product.
- **Should focus on:** AI code security framing should use 'Acme Code' (current name) and reference the AI Trust Platform for AISPM.

#### Findings

##### Finding f1 — retired_product_name (severity: MEDIUM)

**Location:** Body, first paragraph

**Quoted text on page:**

> Acme Code (formerly DeepCheck AI)

**Issue:** DeepCheck AI is the engine inside Acme Code, not a former product name.

**Suggested fix:** Remove the 'formerly DeepCheck AI' parenthetical.

#### Edit plan

**Summary:** Targeted update to remove the DeepCheck AI parenthetical.

**Remove:**
- The phrase 'formerly DeepCheck AI'

#### Rationale

The post is broadly aligned with current Acme positioning but contains one stale framing point about DeepCheck AI being a former product name. A targeted UPDATE that removes the parenthetical is sufficient; no other changes are needed.

---

