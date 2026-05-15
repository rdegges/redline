# redline

[![CI](https://github.com/rdegges/redline/actions/workflows/ci.yml/badge.svg)](https://github.com/rdegges/redline/actions/workflows/ci.yml)
[![CodeQL](https://github.com/rdegges/redline/actions/workflows/codeql.yml/badge.svg)](https://github.com/rdegges/redline/actions/workflows/codeql.yml)
[![Latest release](https://img.shields.io/github/v/release/rdegges/redline?include_prereleases&sort=semver)](https://github.com/rdegges/redline/releases)
[![Go reference](https://pkg.go.dev/badge/github.com/rdegges/redline.svg)](https://pkg.go.dev/github.com/rdegges/redline)
[![Go report card](https://goreportcard.com/badge/github.com/rdegges/redline)](https://goreportcard.com/report/github.com/rdegges/redline)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

> ⚠️ **Alpha quality.** `redline` is under active development. The core scan pipeline works, tests are green, and the report format is stable — but JavaScript rendering isn't supported, some niche subcommands aren't implemented yet, and breaking changes may land in any release until v1.0.0. Pin a specific tag in production-y use.

`redline` is a single-binary command-line tool that audits a website's content against your canonical brand messaging and **Generative Engine Optimization (GEO)** target prompts. It crawls the site, uses a local LLM (Ollama by default) to judge every page, and produces an **agent-actionable report** — each finding tied to a verbatim quote, a location hint, and a suggested fix — so a downstream LLM can apply UPDATE / REWRITE / DELETE edits across the site. Both `report.json` and `report.md` are written on every successful scan.

Other SEO/GEO tools tell you what to **add** to your site. `redline` tells you what's already on your site that no longer matches what you're trying to say — and hands the per-page fix-up plan to the next agent in your pipeline.

## Why redline

- **Messaging-coherence audit, not just monitoring.** Every other GEO tool tells you how LLMs *currently mention* your brand. `redline` tells you *which pages on your own site* are dragging that mention quality down — and what to fix.
- **Agent-actionable output.** The report.md is designed to be eaten by another LLM. Every finding includes verbatim quoted text, a location hint, and a structured edit plan (`preserve` / `remove` / `rewrite` / `add`). Plug it into an editor agent and let it apply the fixes.
- **Local-first.** Default invocation uses Ollama on your machine. No cloud API keys required. No page content leaves localhost. Cloud providers (Anthropic, OpenAI, Voyage) are opt-in for users who want speed or model quality.

## What the report looks like

Excerpt from a real scan — every non-`Aligned` page gets a section like this in `report.md`:

```markdown
### 1. REWRITE — https://example.com/products/container

**Priority:** 56.3 | **Confidence:** 0.90 | **Word count:** 44

**Primary label:** Contradictory
**Secondary labels:** Stale

#### Page summary

- **Current focus:** Positions Container Security as a runtime threat-detection product.
- **Should focus on:** Container handles image and Kubernetes vulnerability scanning;
  runtime threat detection is out of scope.

#### Findings

##### Finding f1 — contradicts_canonical (severity: HIGH)

**Quoted text on page:**
> Continuous runtime threat detection across AWS, Azure, and GCP.

**Suggested fix:** Delete this sentence entirely. Runtime CSPM is not in scope.

#### Edit plan

**Remove:** The runtime threat-detection sentence
**Rewrite:** H1 headline → "Container Security: Image and Kubernetes vulnerability scanning"
**Add:** A cross-link to AppRisk for compliance mapping
```

The `quoted_text` field is **guaranteed to be a literal substring of the page body**, so a downstream editor agent can locate it with a string match.

The full `report.json` is the machine-readable equivalent — validates against an embedded JSON Schema, deterministic ordering, every collection explicitly sorted at the boundary.

## Quickstart

`redline` is **local-first**. The default invocation needs zero cloud API keys; it talks to a local Ollama server.

### 1. Install redline

Via Homebrew:

```bash
brew install rdegges/tap/redline
```

Via `go install`:

```bash
go install github.com/rdegges/redline/cmd/redline@latest
```

Or grab a pre-built binary from the [releases page](https://github.com/rdegges/redline/releases).

### 2. Install Ollama (one-time)

```bash
# macOS
brew install ollama
ollama serve         # leave running in another terminal

# Linux
curl -fsSL https://ollama.com/install.sh | sh
ollama serve
```

Minimum supported version: **0.5.0**.

### 3. Pull the default models (one-time)

```bash
ollama pull qwen3:30b           # judge model (~18GB)
ollama pull nomic-embed-text    # embedding model (~275MB)
```

### 4. Author a `prompts.yaml`

```yaml
version: "1"

prompts:
  - id: category-leader
    text: "Who are the leading vendors in [your category]?"
    weight: 1.5
  - id: best-for-developers
    text: "What is the best developer-first [category] tool?"
    weight: 1.5

canonical_messaging:
  - title: "What we are (2026)"
    body: |
      [Your brand] is the [category] platform for [primary ICP].
      We help [who] [do what] by [how].
  - title: "What we are NOT"
    body: |
      We are not a [adjacent category] tool, not a [another category]
      product. Retired names like "[old product name]" should be flagged
      as STALE.
```

Working templates for three verticals (SaaS product, dev tool, security vendor) live in [`examples/`](./examples/).

### 5. Run the scan

```bash
redline scan --site https://example.com --prompts prompts.yaml
```

The default invocation writes both `report.json` and `report.md` to `./redline-report/`. Add `--format csv` to also produce `report.csv`.

If something's off, run the pre-flight diagnostic:

```bash
redline doctor
```

## Where redline fits in the landscape

| Tool | Crawls site | Brand-msg input | Per-page edit plan | Machine-readable output | Hosting | License |
|---|---|---|---|---|---|---|
| **`redline`** | ✅ sitemap + BFS | ✅ explicit prompts + canon | ✅ UPDATE/REWRITE/DELETE | ✅ JSON + Markdown | Local-first / self-host | MIT (OSS) |
| Profound | Partial | Implicit | ❌ generates new content | Unclear | Cloud only | Proprietary |
| AthenaHQ | ✅ | Implicit | Partial | ❌ dashboards/BI | Cloud only | Proprietary |
| Peec.ai | ❌ tracks LLM outputs | ❌ | ❌ | ✅ CSV + API | Cloud only | Proprietary |
| Otterly | Partial (crawlability) | ❌ prompt library | Partial | Unclear | Cloud only | Proprietary |
| Writer.com | ❌ governs new content | ✅ voice / style guide | ❌ drafts, not edits | Partial | Cloud only | Proprietary |
| MarketMuse | ✅ content inventory | Partial (topic, not voice) | ✅ optimize briefs | Unclear | Cloud only | Proprietary |
| Adobe LLM Optimizer | ✅ crawl + index | Implicit | ✅ auto-suggest | ✅ A2A + MCP | Cloud only | Proprietary |

**Honest read:** the closest direct competitors to `redline` are **Adobe LLM Optimizer** and **AthenaHQ** — both crawl your site and emit page-level GEO recommendations, but they're cloud-only SaaS at mid-to-enterprise price points. **Profound / Peec.ai / Otterly** are a different category: they monitor how LLMs currently mention your brand rather than auditing your site for fixes. **Writer.com** is the most orthogonal — it governs new content generation against a brand voice; it doesn't crawl and rewrite existing pages. `redline`'s distinct wedge is the combination of (1) crawl-the-whole-site, (2) user-supplied prompts + canonical messaging as first-class inputs, (3) structured agent-consumable edit plans, and (4) MIT-licensed local-first execution.

## Command reference

```
redline scan       Crawl, judge, embed, and write a report (full pipeline)
redline crawl      Run only the crawl stage (no LLM calls)
redline judge      Run only the LLM judge stage against an existing DB
redline embed      Run only the embedding + dedup stage
redline report     Regenerate a report from an existing DB
redline doctor     Diagnose runtime, DB, and provider readiness (use `--run` to also inspect a scan run)
redline models     Local Ollama model info (list, recommend)
redline version    Print version information
```

### Selected flags

| Flag | Default | Description |
|---|---|---|
| `--site URL` | required | Root URL to crawl |
| `--prompts PATH` | required | Path to `prompts.yaml` |
| `--output DIR` | `./redline-report` | Output directory |
| `--format` | `json,markdown` | Add `csv` to also emit a CSV |
| `--llm-provider` | `ollama` | `ollama` (default) or `anthropic` |
| `--model` | `qwen3:30b` | Judge model (provider default) |
| `--ollama-url` | `http://localhost:11434` | Ollama HTTP endpoint |
| `--find-duplicates` | `true` | Run Pass 2 (embedding similarity dedup) |
| `--max-pages` | `5000` | Crawl page cap (`0` = unlimited) — bounds cost for cloud runs |
| `--dry-run` | `false` | Crawl + estimate only; no LLM/embed calls |
| `--resume` | `true` | Resume an interrupted run against the same DB |

Run `redline <cmd> --help` for the full flag set on any subcommand.

## Cloud (Anthropic) mode

```bash
export ANTHROPIC_API_KEY=sk-...
export OPENAI_API_KEY=sk-...      # only if --embedding-provider=openai

redline scan \
  --site https://example.com \
  --prompts prompts.yaml \
  --llm-provider anthropic \
  --model claude-sonnet-4-6 \
  --embedding-provider openai
```

Cloud spend is bounded by `--max-pages` (default 5000). For finer cost control, use your provider's account-level spending limits (Anthropic Console / OpenAI usage settings) — `redline` does not maintain its own pricing tables. Token counts per call are recorded in `api_calls` and surfaced in the report.

## Pause / resume

Long scans (4–8 hours against a 2000+ page site) are designed to survive Ctrl-C, machine sleep, network blips, and Ollama restarts. State is committed to the SQLite DB at per-item granularity. Re-invoke the same command against the same `--db` path; already-fetched URLs and already-judged pages are skipped automatically.

## Reports

Every successful scan writes:

- `report.json` — primary machine-readable artifact, validates against the embedded JSON Schema.
- `report.md` — agent-actionable playbook with quoted findings, edit plans, and rationales for every non-`Aligned` page.
- `report.summary.txt` — 10-line stdout summary, also printed to stderr.
- `copies/report.json.sha256`, `copies/report.md.sha256` — sidecar checksums.

`--format csv` adds `report.csv` (RFC 4180, pipe-delimited multi-value columns).

## Architecture

`redline` is a five-stage pipeline (crawl → extract → judge → embed/dedup → report) backed by a single SQLite database for state, resume, retries, and audit. Every stage is idempotent — re-running a stage on the same DB doesn't duplicate work.

For a deeper dive — package layout, lifecycle of a scan, load-bearing design decisions, where to start reading code — see [`docs/architecture.md`](./docs/architecture.md).

## Troubleshooting

- `ollama unavailable` / connection refused → Start Ollama: `ollama serve`.
- `ollama model not pulled` → `ollama pull qwen3:30b` and `ollama pull nomic-embed-text`.
- `Configured prompts.yaml didn't validate` → `redline scan --print-schema` shows the embedded JSON Schema.
- A run aborted mid-scan? Inspect what stage it stopped at with `redline doctor --run latest` — it summarizes counts and suggests the right resume command. Re-running `redline scan` with the same `--db` path picks up where it left off automatically. Add `--json` for scriptable NDJSON output: the per-check lines are followed by a single `{"kind":"run_inspection",...}` record covering the run's state.

## Contributing

PRs welcome — see [`CONTRIBUTING.md`](./CONTRIBUTING.md) for local dev setup (Go via asdf, optional Ollama for live tests), the test/lint/coverage make targets, the conventional-commits style, and the PR workflow.

For questions and general discussion, use [GitHub Discussions](https://github.com/rdegges/redline/discussions). For bug reports and feature requests, use the [issue templates](https://github.com/rdegges/redline/issues/new/choose).

## Security

`redline` processes third-party page content through LLMs. That combination has inherent risk — prompt injection, data egress when using cloud providers, etc. See [`SECURITY.md`](./SECURITY.md) for the full threat model, mitigations, and how to report vulnerabilities privately.

## License

[MIT](./LICENSE) — Copyright © 2026 Randall Degges. Built with [Cobra](https://github.com/spf13/cobra), [modernc.org/sqlite](https://gitlab.com/cznic/sqlite), [PuerkitoBio/goquery](https://github.com/PuerkitoBio/goquery), and a healthy respect for the [Ollama](https://ollama.com/) project.
