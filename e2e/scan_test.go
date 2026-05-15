//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/rdegges/redline/e2e/fakellm"
	"github.com/rdegges/redline/internal/app"
	"github.com/rdegges/redline/internal/config"
	"github.com/rdegges/redline/internal/emb"
	"github.com/rdegges/redline/internal/log"
	"github.com/rdegges/redline/internal/report/normalize"
)

func openDBRaw(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
}

var updateGolden = flag.Bool("update", false, "regenerate golden files")

func fixtureRoot() string {
	cwd, _ := os.Getwd()
	return filepath.Join(filepath.Dir(cwd), "testdata", "fixture-site")
}

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	root := fixtureRoot()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" {
			p = "/index.html"
		}
		bytes, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		body := strings.ReplaceAll(string(bytes), "%HOST%", "http://"+r.Host)
		switch filepath.Ext(p) {
		case ".xml":
			w.Header().Set("Content-Type", "application/xml")
		case ".txt":
			w.Header().Set("Content-Type", "text/plain")
		default:
			w.Header().Set("Content-Type", "text/html")
		}
		_, _ = io.WriteString(w, body)
	}))
	return srv
}

func runScan(t *testing.T, dbPath, outDir, host string) *app.ScanResult {
	t.Helper()
	cfg, err := config.Load(filepath.Join(filepath.Dir(mustCwd(t)), "testdata", "prompts", "acme-minimal.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	opts := app.ScanOptions{
		Site:           host,
		Prompts:        cfg,
		PromptsPath:    "acme-minimal.yaml",
		DBPath:         dbPath,
		OutputDir:      outDir,
		Formats:        []string{"json", "markdown"},
		LLMProvider:    "ollama",
		LLMModel:       "qwen3:30b",
		OllamaURL:      "http://127.0.0.1:0",
		EmbProvider:    "ollama",
		EmbModel:       "nomic-embed-text",
		FindDuplicates: false, // keep deterministic for golden tests
		DupThreshold:   0.92,
		MaxPages:       50,
		MaxDepth:       5,
		Concurrency:    2,
		JudgeConc:      2,
		Rate:           100,
		RespectCanon:   true,
		ThinThreshold:  30,
		MaxRetries:     2,
		MDTopN:         100,
		DryRun:         false,
		Resume:         true,
		LLMClient:      fakellm.BuildClient(host),
		EmbClient:      emb.NewFake(),
	}
	res, err := app.Scan(context.Background(), log.Discard(), opts)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return res
}

func mustCwd(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("cwd: %v", err)
	}
	return cwd
}

func TestE2E_FakeLLM_ScanProducesBothReports(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	dbPath := filepath.Join(t.TempDir(), "scan.db")
	outDir := filepath.Join(t.TempDir(), "out")
	res := runScan(t, dbPath, outDir, srv.URL)
	if res == nil {
		t.Fatal("no result")
	}
	jsonPath := filepath.Join(outDir, "report.json")
	mdPath := filepath.Join(outDir, "report.md")
	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("missing report.json: %v", err)
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Fatalf("missing report.md: %v", err)
	}
	// Sidecar SHA must exist.
	if _, err := os.Stat(filepath.Join(outDir, "copies", "report.json.sha256")); err != nil {
		t.Fatalf("missing report.json.sha256: %v", err)
	}
}

func TestE2E_GoldenJSONAndMarkdown(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	dbPath := filepath.Join(t.TempDir(), "scan.db")
	outDir := filepath.Join(t.TempDir(), "out")
	_ = runScan(t, dbPath, outDir, srv.URL)
	jsonBytes, _ := os.ReadFile(filepath.Join(outDir, "report.json"))
	mdBytes, _ := os.ReadFile(filepath.Join(outDir, "report.md"))

	// Replace the dynamic host portion of URLs with a placeholder so the
	// golden file is stable across test runs.
	jsonNorm := normalizeForGolden(jsonBytes, srv.URL, true)
	mdNorm := normalizeForGolden(mdBytes, srv.URL, false)

	goldenDir := filepath.Join(filepath.Dir(mustCwd(t)), "testdata", "golden")
	jsonGold := filepath.Join(goldenDir, "report.json")
	mdGold := filepath.Join(goldenDir, "report.md")

	if *updateGolden {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(jsonGold, jsonNorm, 0o644); err != nil {
			t.Fatalf("write json gold: %v", err)
		}
		if err := os.WriteFile(mdGold, mdNorm, 0o644); err != nil {
			t.Fatalf("write md gold: %v", err)
		}
		t.Logf("golden files written: %s and %s", jsonGold, mdGold)
		return
	}

	wantJSON, err := os.ReadFile(jsonGold)
	if err != nil {
		t.Fatalf("read golden json: %v (run with -update)", err)
	}
	wantMD, err := os.ReadFile(mdGold)
	if err != nil {
		t.Fatalf("read golden md: %v (run with -update)", err)
	}
	if !bytes.Equal(jsonNorm, wantJSON) {
		dumpDiff(t, "report.json", wantJSON, jsonNorm)
	}
	if !bytes.Equal(mdNorm, wantMD) {
		dumpDiff(t, "report.md", wantMD, mdNorm)
	}
}

func TestE2E_CrashResume_ByteIdentical(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	// First, an uninterrupted scan into DB-A.
	dbA := filepath.Join(t.TempDir(), "scan-a.db")
	outA := filepath.Join(t.TempDir(), "out-a")
	if runScan(t, dbA, outA, srv.URL) == nil {
		t.Fatal("first scan failed")
	}
	jsonA, _ := os.ReadFile(filepath.Join(outA, "report.json"))

	// Now simulate an interrupted scan into DB-B: run scan, then reset
	// some judge state to mimic a SIGKILL mid-judge, then re-run scan.
	dbB := filepath.Join(t.TempDir(), "scan-b.db")
	outBPartial := filepath.Join(t.TempDir(), "out-b-partial")
	if runScan(t, dbB, outBPartial, srv.URL) == nil {
		t.Fatal("partial scan setup failed")
	}
	// Reset the run + classifications to simulate a crash mid-judge:
	// status='running' and half the classifications back to 'pending'.
	resetForCrash(t, dbB)
	outB := filepath.Join(t.TempDir(), "out-b")
	if runScan(t, dbB, outB, srv.URL) == nil {
		t.Fatal("resume scan failed")
	}
	jsonB, _ := os.ReadFile(filepath.Join(outB, "report.json"))

	a := normalize.JSON(jsonA)
	b := normalize.JSON(jsonB)
	if !bytes.Equal(a, b) {
		dumpDiff(t, "crash-resume report.json", a, b)
	}
}

func resetForCrash(t *testing.T, dbPath string) {
	t.Helper()
	db, err := openDBRaw(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE runs SET status='running', completed_at=NULL`); err != nil {
		t.Fatalf("update runs: %v", err)
	}
	if _, err := db.Exec(`UPDATE classifications SET judge_status='pending',
		primary_label=NULL, secondary_labels='[]', confidence=NULL,
		rationale=NULL, affected_prompts='[]', suggested_action=NULL,
		findings_json='[]', edit_plan_json=NULL, page_summary_json=NULL,
		input_tokens=0, output_tokens=0, cache_hit_tokens=0, latency_ms=0,
		raw_response=NULL, error=NULL, judged_at=NULL
		WHERE page_url IN (SELECT page_url FROM classifications LIMIT 2)`); err != nil {
		t.Fatalf("reset classifications: %v", err)
	}
}

func normalizeForGolden(in []byte, host string, isJSON bool) []byte {
	// Replace the test-server's host with a stable placeholder.
	out := bytes.ReplaceAll(in, []byte(host), []byte("http://FIXTURE"))
	if isJSON {
		// Pretty-print so diffs are line-oriented.
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, out, "", "  "); err == nil {
			out = append(pretty.Bytes(), '\n')
		}
		return normalize.JSON(out)
	}
	return normalize.Markdown(out)
}

func dumpDiff(t *testing.T, name string, want, got []byte) {
	t.Helper()
	t.Errorf("%s differs from golden", name)
	const max = 4000
	w := want
	g := got
	if len(w) > max {
		w = w[:max]
	}
	if len(g) > max {
		g = g[:max]
	}
	t.Logf("expected (first %d bytes):\n%s", len(w), w)
	t.Logf("got (first %d bytes):\n%s", len(g), g)
}

// Sanity: ensure imports compile without race detector noise.
var _ = sync.Mutex{}
var _ = fmt.Sprintf
