//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rdegges/redline/e2e/fakellm"
	"github.com/rdegges/redline/internal/app"
	"github.com/rdegges/redline/internal/config"
	"github.com/rdegges/redline/internal/emb"
	"github.com/rdegges/redline/internal/log"
)

func TestE2E_FindDuplicates_ExercisesEmbedPhase(t *testing.T) {
	srv := newFixtureServer(t)
	defer srv.Close()
	cfg, err := config.Load(filepath.Join(filepath.Dir(mustCwd(t)), "testdata", "prompts", "acme-minimal.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	dbPath := filepath.Join(t.TempDir(), "scan.db")
	outDir := filepath.Join(t.TempDir(), "out")
	opts := app.ScanOptions{
		Site: srv.URL, Prompts: cfg, PromptsPath: "p", DBPath: dbPath, OutputDir: outDir,
		Formats:     []string{"json", "markdown"},
		LLMProvider: "ollama", LLMModel: "qwen3:30b",
		EmbProvider: "ollama", EmbModel: "nomic-embed-text",
		FindDuplicates: true,
		DupThreshold:   0.5, // low threshold to ensure some clusters form
		MaxPages:       50, MaxDepth: 5, Concurrency: 2, JudgeConc: 2, Rate: 100,
		RespectCanon: true, ThinThreshold: 30, MaxRetries: 1,
		Resume:    true,
		LLMClient: fakellm.BuildClient(srv.URL),
		EmbClient: emb.NewFake(),
	}
	if _, err := app.Scan(context.Background(), log.Discard(), opts); err != nil {
		t.Fatalf("scan: %v", err)
	}
	// At least the report files should be present.
	if _, err := os.Stat(filepath.Join(outDir, "report.json")); err != nil {
		t.Fatalf("report.json: %v", err)
	}
}
