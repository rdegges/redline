#!/usr/bin/env bash
#
# snyk-full-scan.sh
# -----------------
# Comprehensive overnight geo-prune scan of https://snyk.io using local
# models only (no cloud API keys required).
#
# Prerequisites:
#   - geo-prune installed:    go install github.com/rdegges/geo-prune/cmd/geo-prune@latest
#   - Ollama installed:       brew install ollama  (or https://ollama.com)
#   - Ollama running:         ollama serve  (or launch Ollama.app)
#   - Default models pulled:  geo-prune models pull
#   - Machine plugged in and `caffeinate -i` (the script invokes this for you)
#
# Expected wall-time on an M4 Pro 48GB: 4-8 hours for ~2,500-3,500 pages.
#
# Resumable: if you Ctrl-C or the machine sleeps, just re-run the same
# script against the same --db path. Already-fetched URLs and already-judged
# pages are skipped automatically.
#
# Stop with: Ctrl-C (graceful pause; finishes in-flight workers then exits).
#
# Diagnose a partial run with:
#   geo-prune diagnose --db <db-path> --run latest

set -euo pipefail

# ---------------------------------------------------------------------------
# Paths — override via env var when you want a different location
# ---------------------------------------------------------------------------
SCAN_DATE="$(date +%Y%m%d)"
SCAN_ROOT="${SCAN_ROOT:-${HOME}/scans}"
SCAN_DIR="${SCAN_ROOT}/snyk-${SCAN_DATE}"
DB_PATH="${SCAN_ROOT}/snyk-${SCAN_DATE}.db"
LOG_PATH="${SCAN_ROOT}/snyk-${SCAN_DATE}.log"
PROMPTS_PATH="${PROMPTS_PATH:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/prompts/snyk-full.yaml}"

mkdir -p "${SCAN_ROOT}" "${SCAN_DIR}"

# ---------------------------------------------------------------------------
# Preflight — fail fast if the environment isn't ready
# ---------------------------------------------------------------------------
if ! command -v geo-prune >/dev/null 2>&1; then
  echo "error: geo-prune not on PATH. Install with:" >&2
  echo "  go install github.com/rdegges/geo-prune/cmd/geo-prune@latest" >&2
  exit 1
fi

if [[ ! -f "${PROMPTS_PATH}" ]]; then
  echo "error: prompts file not found at ${PROMPTS_PATH}" >&2
  echo "       set PROMPTS_PATH env var or run from the geo-prune repo." >&2
  exit 1
fi

# Doctor will fail with a clear message if Ollama is unreachable or
# the required models aren't pulled.
geo-prune doctor

# ---------------------------------------------------------------------------
# The scan
# ---------------------------------------------------------------------------
# Why these flags (see PRD §6.3.1 + the runbook in repo for full rationale):
#
#   --max-pages 0          unlimited — let the crawler find everything linked
#   --max-depth 8          snyk.io has deeply nested customer/article trees
#   --concurrency 6        crawler workers — network-bound, polite at 3 QPS
#   --judge-concurrency 2  local LLM is the bottleneck; 2 fits comfortably
#                          in 48GB with qwen3:30b resident
#   --rate 3.0             requests/sec to snyk.io — conservative, polite
#   --ollama-timeout 10m   defensive for long pages + first-load latency
#   --ollama-keepalive 4h  keep the model resident in RAM for the whole run
#   --find-duplicates      default true; surfaces redundant snyk.io pages
#   --max-retries 8        overnight runs hit transient blips
#   --retry-max-delay 5m   ceiling on individual backoff intervals
#   --log-format json      structured logs for post-hoc grep + agent debug
#   --format json,markdown,csv  all three for downstream tooling
# ---------------------------------------------------------------------------

echo ""
echo "Starting geo-prune scan of https://snyk.io"
echo "  prompts:  ${PROMPTS_PATH}"
echo "  db:       ${DB_PATH}"
echo "  output:   ${SCAN_DIR}"
echo "  log:      ${LOG_PATH}"
echo ""
echo "This will take 4-8 hours on an M4 Pro 48GB. Safe to Ctrl-C and resume."
echo ""

exec caffeinate -i geo-prune scan \
  --site https://snyk.io \
  --prompts "${PROMPTS_PATH}" \
  --db "${DB_PATH}" \
  --output "${SCAN_DIR}" \
  --max-pages 0 \
  --max-depth 8 \
  --concurrency 6 \
  --judge-concurrency 2 \
  --rate 3.0 \
  --ollama-timeout 10m \
  --ollama-keepalive 4h \
  --find-duplicates \
  --max-retries 8 \
  --retry-max-delay 5m \
  --log-format json \
  --log-file "${LOG_PATH}" \
  --format json,markdown,csv \
  --yes
