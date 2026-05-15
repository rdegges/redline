# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

`redline` is currently in alpha. Breaking changes may land in any release until v1.0.0.

## [Unreleased]

## [0.1.0-alpha] — 2026-05-14

Initial public alpha release. Built from scratch under the working name `geo-prune` (May 2026); renamed to `redline` shortly before the public release.

### Added

- `redline scan` — crawl a website, judge every page against canonical brand messaging + GEO target prompts, and write `report.json` + `report.md` to the output directory.
- `redline crawl` — run only the crawl stage.
- `redline judge` — run only the LLM judge stage against an existing DB.
- `redline embed` — run only the embedding + dedup stage.
- `redline report` — regenerate reports from an existing DB without re-scanning.
- `redline doctor` — pre-flight diagnostics: Go runtime version, DB readiness, Ollama reachability, configured-model presence. Pass `--run latest` (or `--run <id>`) to also inspect a scan run — status, URL-state counts, classification-label counts, API-call counts, and a next-action suggestion based on run status (resume / re-render / debug).
- `redline models list / recommend` — local Ollama model info.
- `redline version` — prints version, commit, build date, Go version, OS/arch.
- Local-first by default — Ollama (`qwen3:30b` judge + `nomic-embed-text` embeddings) requires no cloud API keys.
- Optional Anthropic LLM provider with prompt caching (`--llm-provider=anthropic`).
- Optional cloud embedding providers: OpenAI (`text-embedding-3-small`) and Voyage (`voyage-3-large`).
- Pause/resume across hours-long runs — state committed to SQLite at per-item granularity.
- Token-count tracking per API call (input / cached / output) surfaced in `report.json` and the `api_calls` SQLite table — cost computation is delegated to the user's provider dashboard, not maintained inside redline.
- Comprehensive crawler: sitemap-index recursion, RSS/Atom feed discovery, `robots.txt` sitemap directives, BFS link-following with depth and URL-canonicalization.
- Universal exponential-backoff retry policy across all HTTP calls.
- Eight-label judge rubric: Aligned / Stale / OffBrand / Contradictory / Redundant / Thin / Zombie / Unclear.
- Agent-actionable report output: per-page edit plans with verbatim quoted text, severity-rated findings, and structured `preserve` / `remove` / `rewrite` / `add` actions.
- Pass 2 duplicate detection (`--find-duplicates`) via cosine similarity over page embeddings.
- 80% test coverage gate enforced in CI.
- Cross-platform builds — darwin, linux, windows × amd64, arm64.

### Known limitations

- No bundled model pull — run `ollama pull qwen3:30b && ollama pull nomic-embed-text` directly. `redline doctor` reports whether the models are present.
- No JavaScript rendering — SPAs without server-side rendering will produce thin / empty extractions. `redline` warns when this is happening.
- No automatic fix-application — `redline` writes a report; a downstream agent applies the changes.

### Security

- See [`SECURITY.md`](./SECURITY.md) for the documented threat model: prompt injection from crawled pages, data egress when using cloud providers, API key exposure, SSRF surface, and SQLite content sensitivity.

[Unreleased]: https://github.com/rdegges/redline/compare/v0.1.0-alpha...HEAD
[0.1.0-alpha]: https://github.com/rdegges/redline/releases/tag/v0.1.0-alpha
