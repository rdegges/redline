# Contributing to redline

Thanks for thinking about contributing to `redline`. This document covers everything you need to know to make your first contribution — from local setup to PR conventions.

If you only want to **report a bug** or **request a feature**, jump straight to [github.com/rdegges/redline/issues/new/choose](https://github.com/rdegges/redline/issues/new/choose).

---

## Ground rules

- **One concern per PR.** Bundling rename + bug fix + feature in one PR makes review harder for everyone.
- **Tests pin behavior.** Every behavior change ships with a test that fails before your change and passes after.
- **Don't break the build.** CI must be green before merge. Run `make test` locally first.
- **Be kind.** This is a personal-time project and everyone's contribution time is volunteer.

---

## Local development setup

### Prerequisites

- **Go 1.24 or newer.** Recommended: install via [asdf](https://asdf-vm.com/) so you can match the version pinned in `.tool-versions`:
  ```bash
  asdf install golang $(cat .tool-versions | awk '/golang/ {print $2}')
  ```
- **Ollama**, if you want to run the integration tests or hand-test end-to-end against a real LLM. Install from [ollama.com](https://ollama.com/) and run `ollama serve` in a separate terminal.
  ```bash
  brew install ollama   # macOS
  # or: curl -fsSL https://ollama.com/install.sh | sh
  ollama serve
  ```
- **GNU Make**, for the convenience targets in the `Makefile`.

You do **not** need any cloud API keys for development. The default test path uses a deterministic fake LLM (`e2e/fakellm`) and a fixture site under `testdata/fixture-site/`.

### Clone and build

```bash
git clone https://github.com/rdegges/redline.git
cd redline
make build
./bin/redline --help
```

### Run the test suite

```bash
make test         # unit tests (fast — under a minute)
make test-int     # integration tests (-tags=integration)
make e2e          # end-to-end tests (-tags=e2e) against the fixture site
make coverage     # runs the coverage gate (fails if < 80%)
make lint         # golangci-lint
make fmt          # gofmt -s -w
make vuln         # govulncheck
```

`make test-ollama` runs a few live-Ollama integration tests that hit a real local Ollama server. They're gated behind `OLLAMA_LIVE=1` so CI doesn't depend on Ollama being available.

### Regenerating golden files

The e2e tests assert byte-for-byte against `testdata/golden/report.json` and `report.md`. When you change report rendering on purpose:

```bash
go test -tags=e2e ./e2e/... -update
```

Then run the e2e tests again without `-update` and confirm they pass.

---

## Code style

- **Format with gofmt.** CI fails on unformatted files. Run `make fmt` before pushing.
- **Lint with golangci-lint.** The active linters are listed in `.golangci.yml` (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`, `gofmt`, `misspell`, `unconvert`). Run `make lint` locally.
- **No new dependencies without a reason.** `redline` aims to stay small. If you add a new `go.mod` requirement, explain why in the PR description.
- **Comments explain WHY, not WHAT.** Code already says what it does. Comments should explain non-obvious constraints, invariants, or rationales — not narrate the line above.
- **No emojis in code or commit messages** unless explicitly relevant.
- **Internal packages stay internal.** The `internal/` tree is private to this repo on purpose; Go enforces this. Don't promote anything to `pkg/` without a discussion in an issue first.

---

## Commit conventions

`redline` follows [Conventional Commits](https://www.conventionalcommits.org/). Examples from the existing history:

```
feat(judge): persist api_calls row with cost on every successful judgment
fix(judge): clamp confidence to [0,1] with percentage recovery
test(judge): cover confidence clamping and MANUAL_REVIEW auto-cap
docs(readme): reposition around messaging-coherence + agent-handoff
chore(module): rename Go module path to github.com/rdegges/redline
```

**Types:** `feat`, `fix`, `docs`, `test`, `chore`, `refactor`, `perf`, `ci`.

**Scope (optional):** the package or area touched — `judge`, `crawl`, `report`, `cli`, `module`, etc.

**Subject:** imperative mood, lowercase, no trailing period, under ~70 chars.

**Body:** wrap at 72 chars. Explain the WHY, link to issues with `Fixes #N` or `Closes #N`.

---

## Pull request workflow

1. **Fork the repo** on GitHub and clone your fork locally.
2. **Create a topic branch** off `main`:
   ```bash
   git checkout -b feat/my-new-thing
   ```
3. **Write a failing test** that captures the behavior you want.
4. **Make the minimum change** to make that test pass.
5. **Run the full suite** locally: `make test test-int e2e lint`.
6. **Commit using conventional commits** (one logical change per commit; small + composable is welcome).
7. **Push to your fork and open a PR** against `main`. The PR template will prompt you for context.
8. **Respond to review comments** by pushing additional commits to the same branch. CI will re-run automatically.
9. **Squash-or-merge is the maintainer's call.** Don't worry about rewriting history.

### What makes a PR easy to merge

- Tests pass; coverage doesn't regress.
- The diff matches the PR title/description — no surprises.
- New behavior is documented (README, CHANGELOG, command help text).
- Backwards-incompatible changes are flagged explicitly in the PR body.

### What can slow a PR down

- Mixing unrelated changes (formatting + feature + bug fix in one PR).
- New dependencies without justification.
- Touching `internal/store/migrations/*.sql` without explaining the migration strategy.
- Modifying the LLM judge prompt template (`internal/judge/prompt.tmpl`) without updating golden files and explaining the diff.

---

## Asking questions

- **General questions:** open a [GitHub Discussion](https://github.com/rdegges/redline/discussions) (once enabled) or a low-priority issue tagged `question`.
- **Bug reports:** use the [bug report template](https://github.com/rdegges/redline/issues/new?template=bug_report.yml). The structured fields help maintainers reproduce faster than free-form prose.
- **Feature requests:** use the [feature request template](https://github.com/rdegges/redline/issues/new?template=feature_request.yml). Briefly describe the use case before proposing an implementation.
- **Security issues:** see [`SECURITY.md`](./SECURITY.md). Do **not** open public issues for vulnerabilities.

---

## Releasing (maintainers only)

Releases are tag-driven. To cut a release:

```bash
git tag v0.2.0
git push origin v0.2.0
```

The `release.yml` GitHub Actions workflow runs [GoReleaser](https://goreleaser.com/), which:

- Builds binaries for darwin/linux/windows × amd64/arm64
- Publishes tarballs + SHA256 checksums to GitHub Releases
- Updates the Homebrew formula in `rdegges/homebrew-tap`
- Generates a CHANGELOG entry from conventional commits since the last tag

Pre-release tags (`v0.2.0-alpha`, `v0.2.0-rc.1`, etc.) are recognized automatically and marked as pre-releases on GitHub.

---

## License

By contributing, you agree your contributions will be released under the same [MIT license](./LICENSE) the project uses.
