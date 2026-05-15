package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rdegges/redline/internal/app"
	"github.com/rdegges/redline/internal/config"
	embpkg "github.com/rdegges/redline/internal/emb"
	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/llm"
	"github.com/rdegges/redline/internal/llm/anthropic"
	"github.com/rdegges/redline/internal/llm/ollama"
	logpkg "github.com/rdegges/redline/internal/log"
)

// scanRun is invoked by the cobra runner. It validates flags, loads the
// config, and dispatches to internal/app.Scan.
func scanRunImpl() error {
	if sflags.PrintSchema {
		_, err := os.Stdout.Write(config.Schema())
		return err
	}
	if sflags.Site == "" {
		return fmt.Errorf("%w: --site is required", errs.ErrInvalidConfig)
	}
	if sflags.Prompts == "" {
		return fmt.Errorf("%w: --prompts is required", errs.ErrInvalidConfig)
	}
	cfg, err := config.Load(sflags.Prompts)
	if err != nil {
		return err
	}
	if err := cfg.ValidateSeedsAgainstHost(sflags.Site); err != nil {
		return err
	}
	logger := newLogger()
	opts, err := buildScanOptions(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := signalContext()
	defer cancel()
	res, err := app.Scan(ctx, logger, opts)
	if err != nil {
		return err
	}
	printScanSummary(res, opts)
	return nil
}

func buildScanOptions(cfg *config.File) (app.ScanOptions, error) {
	timeout, _ := time.ParseDuration(sflags.OllamaTimeout)
	httpTimeout, _ := time.ParseDuration(sflags.HTTPTimeout)
	rbd, _ := time.ParseDuration(sflags.RetryBaseDelay)
	rmd, _ := time.ParseDuration(sflags.RetryMaxDelay)
	if rbd == 0 {
		rbd = time.Second
	}
	if rmd == 0 {
		rmd = 2 * time.Minute
	}
	if httpTimeout == 0 {
		httpTimeout = 60 * time.Second
	}
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	model := sflags.Model
	if model == "" {
		switch sflags.LLMProvider {
		case "anthropic":
			model = "claude-sonnet-4-6"
		default:
			model = defaultOllamaModel
		}
	}
	emb := sflags.EmbModel
	if emb == "" {
		switch sflags.EmbProvider {
		case "openai":
			emb = "text-embedding-3-small"
		case "voyage":
			emb = "voyage-3-large"
		default:
			emb = defaultEmbeddingModel
		}
	}
	jc := sflags.JudgeConc
	if jc <= 0 {
		switch sflags.LLMProvider {
		case "anthropic":
			jc = 4
		default:
			jc = 2
		}
	}
	opts := app.ScanOptions{
		Site:            sflags.Site,
		Prompts:         cfg,
		PromptsPath:     sflags.Prompts,
		DBPath:          global.DB,
		OutputDir:       sflags.Output,
		Formats:         sflags.Format,
		LLMProvider:     sflags.LLMProvider,
		LLMModel:        model,
		OllamaURL:       sflags.OllamaURL,
		OllamaTimeout:   timeout,
		OllamaKeepAlive: sflags.OllamaKeepAlive,
		EmbProvider:     sflags.EmbProvider,
		EmbModel:        emb,
		FindDuplicates:  sflags.FindDuplicates,
		DupThreshold:    sflags.DupThreshold,
		MaxPages:        sflags.MaxPages,
		MaxDepth:        sflags.MaxDepth,
		Concurrency:     sflags.Concurrency,
		JudgeConc:       jc,
		Rate:            sflags.Rate,
		UserAgent:       sflags.UserAgent,
		IgnoreRobots:    sflags.IgnoreRobots,
		RespectCanon:    sflags.RespectCanon,
		ThinThreshold:   sflags.ThinPageWords,
		MaxRetries:      sflags.MaxRetries,
		RetryBaseDelay:  rbd,
		RetryMaxDelay:   rmd,
		HTTPTimeout:     httpTimeout,
		MDTopN:          sflags.MDTopN,
		DryRun:          sflags.DryRun,
		Resume:          sflags.Resume,
	}
	// For dry runs, no real LLM call happens. For non-dry runs, attach
	// a real provider client.
	if !opts.DryRun {
		switch opts.LLMProvider {
		case "anthropic":
			opts.LLMClient = anthropic.New(os.Getenv("ANTHROPIC_API_KEY"), opts.OllamaTimeout)
		default:
			opts.LLMClient = ollama.New(opts.OllamaURL, opts.OllamaTimeout, opts.OllamaKeepAlive)
		}
	} else {
		opts.LLMClient = llm.NewFakeClient()
	}
	// Embedding client wiring (only needed when --find-duplicates is on).
	if opts.FindDuplicates && !opts.DryRun {
		switch opts.EmbProvider {
		case "openai":
			opts.EmbClient = embpkg.NewOpenAI(os.Getenv("OPENAI_API_KEY"), opts.EmbModel, opts.OllamaTimeout)
		case "voyage":
			opts.EmbClient = embpkg.NewVoyage(os.Getenv("VOYAGE_API_KEY"), opts.EmbModel, opts.OllamaTimeout)
		default:
			opts.EmbClient = embpkg.NewOllama(opts.OllamaURL, opts.EmbModel, opts.OllamaTimeout, opts.OllamaKeepAlive)
		}
	}
	return opts, nil
}

func newLogger() *slog.Logger {
	lvl := slog.LevelInfo
	switch global.LogLevel {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	format := logpkg.FormatText
	if global.LogFormat == "json" {
		format = logpkg.FormatJSON
	}
	return logpkg.NewLogger(logpkg.Options{Out: os.Stderr, Level: lvl, Format: format})
}

func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		cancel()
	}()
	return ctx, cancel
}

func printScanSummary(res *app.ScanResult, opts app.ScanOptions) {
	if res == nil || res.Report == nil {
		return
	}
	r := res.Report
	fmt.Println("redline scan complete.")
	fmt.Printf("  Site:                %s\n", r.Site)
	fmt.Printf("  Run:                 %s\n", res.RunID)
	fmt.Printf("  LLM:                 %s / %s\n", r.Provider.LLMProvider, r.Provider.LLMModel)
	if r.Provider.EmbeddingProvider != "" {
		fmt.Printf("  Embeddings:          %s / %s\n", r.Provider.EmbeddingProvider, r.Provider.EmbeddingModel)
	}
	fmt.Printf("  Pages crawled:       %d\n", r.Summary.PagesTotal)
	fmt.Printf("  Pages judged:        %d\n", r.Summary.PagesJudged)
	fmt.Printf("  Pages failed:        %d\n", r.Summary.PagesFailed)
	fmt.Printf("  Retries:             %d\n", r.Summary.RetriesTotal)
	fmt.Printf("  Duplicate clusters:  %d\n", r.Summary.DuplicateClustersCount)
	fmt.Printf("  API calls:           %d\n", r.Summary.TotalAPICalls)
	fmt.Printf("  Reports:             %s/report.json\n", opts.OutputDir)
	fmt.Printf("                       %s/report.md\n", opts.OutputDir)
}

// Verify untouched-imports warnings on tooling differences.
var _ = errors.Is
var _ = strings.ToLower
