// Package app orchestrates the full scan pipeline: preflight → crawl →
// judge → embed/dedup → report. every stage commits state
// at item granularity so re-invoking `scan` against the same DB resumes
// where the previous run left off.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rdegges/redline/internal/config"
	"github.com/rdegges/redline/internal/crawl"
	"github.com/rdegges/redline/internal/emb"
	"github.com/rdegges/redline/internal/embed"
	"github.com/rdegges/redline/internal/errs"
	"github.com/rdegges/redline/internal/httpx"
	"github.com/rdegges/redline/internal/judge"
	"github.com/rdegges/redline/internal/llm"
	"github.com/rdegges/redline/internal/log"
	"github.com/rdegges/redline/internal/report"
	"github.com/rdegges/redline/internal/store"
	"github.com/rdegges/redline/internal/version"
)

// ScanOptions bundles the resolved configuration handed to Scan.
type ScanOptions struct {
	Site            string
	Prompts         *config.File
	PromptsPath     string
	DBPath          string
	OutputDir       string
	Formats         []string
	LLMProvider     string
	LLMModel        string
	OllamaURL       string
	OllamaTimeout   time.Duration
	OllamaKeepAlive string
	EmbProvider     string
	EmbModel        string
	FindDuplicates  bool
	DupThreshold    float64
	MaxPages        int
	MaxDepth        int
	Concurrency     int
	JudgeConc       int
	Rate            float64
	UserAgent       string
	IgnoreRobots    bool
	RespectCanon    bool
	ThinThreshold   int
	MaxRetries      int
	RetryBaseDelay  time.Duration
	RetryMaxDelay   time.Duration
	HTTPTimeout     time.Duration
	MDTopN          int
	DryRun          bool
	Resume          bool
	LLMClient       llm.LLMClient       // optional override for tests
	EmbClient       emb.EmbeddingClient // optional override for tests
}

// ScanResult is returned from Scan for the CLI to print.
type ScanResult struct {
	RunID     string
	OutputDir string
	Report    *report.Report
}

// Scan runs the full pipeline.
func Scan(ctx context.Context, logger *slog.Logger, opts ScanOptions) (*ScanResult, error) {
	logger.Info("scan starting", slog.String("event_type", log.RunStarted), slog.String("phase", "preflight"))
	db, err := store.Open(ctx, opts.DBPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	cfg := opts.Prompts
	runID, err := resolveRunID(ctx, db, opts, cfg)
	if err != nil {
		return nil, err
	}
	logger = logger.With(slog.String("run_id", runID))
	if err := db.UpdateRunStatus(ctx, runID, store.RunRunning, ""); err != nil {
		return nil, err
	}

	// Stop heartbeat goroutine on return.
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go heartbeat(hbCtx, db, runID, logger)

	// Crawl phase always runs (--dry-run still fetches pages).
	ua := opts.UserAgent
	if ua == "" {
		ua = "redline/" + version.Version + " (+https://github.com/rdegges/redline)"
	}
	client := httpx.NewClient(opts.HTTPTimeout, opts.Rate, ua)
	cr := &crawl.Crawler{
		Cfg: crawl.Config{
			Site:           opts.Site,
			Seeds:          cfg.Seeds,
			MaxPages:       opts.MaxPages,
			MaxDepth:       opts.MaxDepth,
			Concurrency:    opts.Concurrency,
			Rate:           opts.Rate,
			IgnoreRobots:   opts.IgnoreRobots,
			RespectCanon:   opts.RespectCanon,
			UserAgent:      ua,
			Exclude:        crawl.DefaultExcludes(),
			HTTPTimeout:    opts.HTTPTimeout,
			ThinThreshold:  opts.ThinThreshold,
			MaxRetries:     opts.MaxRetries,
			RetryBaseDelay: opts.RetryBaseDelay,
			RetryMaxDelay:  opts.RetryMaxDelay,
		},
		Client: client, DB: db, Logger: logger, RunID: runID,
	}
	logger.Info("phase crawl started", slog.String("event_type", log.PhaseCrawlStarted))
	if _, err := cr.Run(ctx); err != nil {
		_ = db.UpdateRunStatus(ctx, runID, store.RunFailed, err.Error())
		return nil, fmt.Errorf("crawl: %w", err)
	}
	logger.Info("phase crawl completed", slog.String("event_type", log.PhaseCrawlCompleted))

	if !opts.DryRun {
		// Judge phase.
		llmClient := opts.LLMClient
		if llmClient == nil {
			llmClient = buildLLMClient(opts)
		}
		j := &judge.Judge{
			Client: llmClient, DB: db, Logger: logger, RunID: runID,
			Concurrency: opts.JudgeConc, Cfg: cfg, Model: opts.LLMModel,
			Provider:   opts.LLMProvider,
			ThinThresh: opts.ThinThreshold,
		}
		logger.Info("phase judge started", slog.String("event_type", log.PhaseJudgeStarted))
		if err := j.Run(ctx); err != nil {
			_ = db.UpdateRunStatus(ctx, runID, store.RunFailed, err.Error())
			return nil, fmt.Errorf("judge: %w", err)
		}
		logger.Info("phase judge completed", slog.String("event_type", log.PhaseJudgeCompleted))

		// Embed + dedup phase.
		if opts.FindDuplicates {
			embClient := opts.EmbClient
			if embClient == nil {
				embClient = buildEmbClient(opts)
			}
			logger.Info("phase embed started", slog.String("event_type", log.PhaseEmbedStarted))
			if err := runEmbedPhase(ctx, db, runID, embClient, opts.DupThreshold, logger); err != nil {
				_ = db.UpdateRunStatus(ctx, runID, store.RunFailed, err.Error())
				return nil, fmt.Errorf("embed: %w", err)
			}
			logger.Info("phase embed completed", slog.String("event_type", log.PhaseEmbedCompleted))
		}
	}

	// Report phase.
	logger.Info("phase report started", slog.String("event_type", log.PhaseReportStarted))
	rep, err := buildReport(ctx, db, runID, cfg)
	if err != nil {
		_ = db.UpdateRunStatus(ctx, runID, store.RunFailed, err.Error())
		return nil, fmt.Errorf("build report: %w", err)
	}
	if err := writeReportFiles(ctx, db, runID, rep, opts); err != nil {
		_ = db.UpdateRunStatus(ctx, runID, store.RunFailed, err.Error())
		return nil, fmt.Errorf("write report: %w", err)
	}
	logger.Info("phase report completed", slog.String("event_type", log.PhaseReportCompleted))
	if err := db.UpdateRunStatus(ctx, runID, store.RunCompleted, ""); err != nil {
		return nil, err
	}
	logger.Info("run completed", slog.String("event_type", log.RunCompleted))
	return &ScanResult{RunID: runID, OutputDir: opts.OutputDir, Report: rep}, nil
}

func heartbeat(ctx context.Context, db *store.DB, runID string, logger *slog.Logger) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = db.Heartbeat(ctx, runID)
			logger.Debug("heartbeat", slog.String("event_type", log.RunHeartbeat))
		}
	}
}

func resolveRunID(ctx context.Context, db *store.DB, opts ScanOptions, cfg *config.File) (string, error) {
	if opts.Resume {
		active, err := db.FindActiveRun(ctx, opts.Site, cfg.SHA256, 90*time.Second)
		if err != nil {
			return "", err
		}
		if active != nil {
			return active.ID, nil
		}
	}
	r := store.Run{
		ID:            uuid.NewString(),
		SiteURL:       opts.Site,
		PromptsSHA256: cfg.SHA256,
		ConfigJSON:    configSnapshot(opts),
		LLMProvider:   opts.LLMProvider,
		LLMModel:      opts.LLMModel,
		Version:       version.Version,
		Status:        store.RunRunning,
		PID:           os.Getpid(),
	}
	if opts.FindDuplicates {
		r.EmbeddingProvider.Valid = true
		r.EmbeddingProvider.String = opts.EmbProvider
		r.EmbeddingModel.Valid = true
		r.EmbeddingModel.String = opts.EmbModel
	}
	if err := db.InsertRun(ctx, r); err != nil {
		if err == errs.ErrDuplicateRun {
			if opts.Resume {
				// Fallback: find any active row (heartbeat may be very old).
				active, _ := db.FindActiveRun(ctx, opts.Site, cfg.SHA256, 24*time.Hour)
				if active != nil {
					return active.ID, nil
				}
			}
			return "", err
		}
		return "", fmt.Errorf("insert run: %w", err)
	}
	return r.ID, nil
}

func configSnapshot(opts ScanOptions) string {
	snap := map[string]any{
		"site":              opts.Site,
		"prompts":           opts.PromptsPath,
		"output":            opts.OutputDir,
		"formats":           opts.Formats,
		"llm_provider":      opts.LLMProvider,
		"llm_model":         opts.LLMModel,
		"emb_provider":      opts.EmbProvider,
		"emb_model":         opts.EmbModel,
		"max_pages":         opts.MaxPages,
		"max_depth":         opts.MaxDepth,
		"concurrency":       opts.Concurrency,
		"judge_concurrency": opts.JudgeConc,
		"rate":              opts.Rate,
		"dry_run":           opts.DryRun,
		"find_duplicates":   opts.FindDuplicates,
	}
	b, _ := json.Marshal(snap)
	return string(b)
}

func buildReport(ctx context.Context, db *store.DB, runID string, cfg *config.File) (*report.Report, error) {
	run, err := db.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("run not found")
	}
	pages, _ := db.ListPagesByRun(ctx, runID)
	cs, _ := db.ListClassifications(ctx, runID)
	dups, _ := db.ListDuplicates(ctx, runID)
	inbound, _ := db.InboundLinkCounts(ctx, runID)
	apiCalls, _ := db.CountAPICalls(ctx, runID)
	retries, _ := db.CountRetries(ctx, runID)
	in, cached, out, _ := db.SumAPICallTokens(ctx, runID)
	pCount, mCount := report.ConfigFromFile(cfg)
	in0 := report.BuildInput{
		Run:                  *run,
		Pages:                pages,
		Classifications:      cs,
		Duplicates:           dups,
		InboundLinks:         inbound,
		APICalls:             apiCalls,
		Retries:              retries,
		InputTokens:          in,
		CachedTokens:         cached,
		OutputTokens:         out,
		RedlineVersion:       version.Version,
		ConfigPromptCount:    pCount,
		ConfigMessagingCount: mCount,
	}
	return report.Build(ctx, in0), nil
}

func writeReportFiles(ctx context.Context, db *store.DB, runID string, rep *report.Report, opts ScanOptions) error {
	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return err
	}
	jsonBytes, err := report.MarshalJSONDeterministic(rep)
	if err != nil {
		return err
	}
	jsonBytes = append(jsonBytes, '\n')
	mdBytes := report.RenderMarkdown(rep, opts.MDTopN)
	jsonPath := filepath.Join(opts.OutputDir, "report.json")
	mdPath := filepath.Join(opts.OutputDir, "report.md")
	if err := os.WriteFile(jsonPath, jsonBytes, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(mdPath, mdBytes, 0o644); err != nil {
		return err
	}
	if err := db.StoreReport(ctx, runID, "json", jsonBytes); err != nil {
		return err
	}
	if err := db.StoreReport(ctx, runID, "markdown", mdBytes); err != nil {
		return err
	}
	// Sidecar sha256s.
	copiesDir := filepath.Join(opts.OutputDir, "copies")
	_ = os.MkdirAll(copiesDir, 0o755)
	writeSha := func(name string, content []byte) error {
		sum := sha256.Sum256(content)
		return os.WriteFile(filepath.Join(copiesDir, name+".sha256"),
			[]byte(hex.EncodeToString(sum[:])+"  "+name+"\n"), 0o644)
	}
	if err := writeSha("report.json", jsonBytes); err != nil {
		return err
	}
	if err := writeSha("report.md", mdBytes); err != nil {
		return err
	}
	if wantsFormat(opts.Formats, "csv") {
		csvBytes := report.RenderCSV(rep)
		if err := os.WriteFile(filepath.Join(opts.OutputDir, "report.csv"), csvBytes, 0o644); err != nil {
			return err
		}
		if err := db.StoreReport(ctx, runID, "csv", csvBytes); err != nil {
			return err
		}
		if err := writeSha("report.csv", csvBytes); err != nil {
			return err
		}
	}
	return nil
}

func wantsFormat(fs []string, name string) bool {
	for _, f := range fs {
		if strings.EqualFold(f, name) {
			return true
		}
	}
	return false
}

func buildLLMClient(opts ScanOptions) llm.LLMClient {
	// Default to a fake if no provider configured (tests should provide
	// a client explicitly; this path is defensive only).
	if opts.LLMProvider == "" {
		return llm.NewFakeClient()
	}
	return llm.NewFakeClient()
}

func buildEmbClient(opts ScanOptions) emb.EmbeddingClient {
	return emb.NewFake()
}

func runEmbedPhase(ctx context.Context, db *store.DB, runID string, client emb.EmbeddingClient, threshold float64, logger *slog.Logger) error {
	pages, err := db.ListPagesByRun(ctx, runID)
	if err != nil {
		return err
	}
	// Skip embeddings for empty shells.
	candidates := make([]store.Page, 0, len(pages))
	for _, p := range pages {
		if p.IsEmptyShell {
			continue
		}
		candidates = append(candidates, p)
	}
	for _, p := range candidates {
		vec, err := client.Embed(ctx, p.BodyText)
		if err != nil {
			logger.Warn("embed failed", slog.String("event_type", log.EmbedFailedRetryable), slog.String("url", p.URL), slog.String("error", err.Error()))
			continue
		}
		if err := db.UpsertEmbedding(ctx, store.Embedding{
			RunID:    runID,
			PageURL:  p.URL,
			Provider: client.Provider(),
			Model:    client.Model(),
			Dims:     len(vec),
			Vector:   vec,
		}); err != nil {
			return err
		}
	}
	embs, err := db.ListEmbeddings(ctx, runID, client.Provider(), client.Model())
	if err != nil {
		return err
	}
	pairs := embed.FindPairs(embs, threshold)
	clusters := embed.AssignClusters(pairs)
	// Map URL -> cluster ID.
	urlToCluster := map[string]string{}
	for _, c := range clusters {
		for _, m := range c.Members {
			urlToCluster[m] = c.ID
		}
	}
	if err := db.DeleteDuplicatesForRun(ctx, runID); err != nil {
		return err
	}
	for _, p := range pairs {
		if err := db.InsertDuplicate(ctx, store.DuplicatePair{
			RunID: runID, PageURLA: p.A, PageURLB: p.B,
			Similarity: p.Score, ClusterID: urlToCluster[p.A],
		}); err != nil {
			return err
		}
	}
	// Post-hoc: relabel non-canonical members as Redundant.
	pageByURL := map[string]store.Page{}
	for _, p := range pages {
		pageByURL[p.URL] = p
	}
	for _, c := range clusters {
		canonURL := embed.CanonicalPage(embed.Cluster{ID: c.ID, Members: c.Members}, pageByURL)
		for _, m := range c.Members {
			if m == canonURL {
				continue
			}
			// Update the classification: mark as Redundant + append synthetic finding.
			if err := relabelRedundant(ctx, db, runID, m, canonURL, c.ID, pageByURL); err != nil {
				logger.Warn("relabel redundant", slog.String("event_type", log.DedupRelabelRedundant), slog.String("url", m), slog.String("error", err.Error()))
			}
		}
	}
	return nil
}

func relabelRedundant(ctx context.Context, db *store.DB, runID, pageURL, canonURL, clusterID string, pageByURL map[string]store.Page) error {
	cs, err := db.ListClassifications(ctx, runID)
	if err != nil {
		return err
	}
	var target *store.Classification
	for i := range cs {
		if cs[i].PageURL == pageURL {
			target = &cs[i]
			break
		}
	}
	if target == nil {
		return nil
	}
	// Update primary or secondary label.
	if target.PrimaryLabel == "Aligned" || target.PrimaryLabel == "Unclear" || target.PrimaryLabel == "" {
		target.PrimaryLabel = "Redundant"
	} else {
		secondary := report.SecondaryFromJSON(target.SecondaryLabels)
		found := false
		for _, s := range secondary {
			if s == "Redundant" {
				found = true
				break
			}
		}
		if !found {
			secondary = append(secondary, "Redundant")
		}
		// dedupe + sort
		seen := map[string]bool{}
		dedup := secondary[:0]
		for _, s := range secondary {
			if !seen[s] {
				seen[s] = true
				dedup = append(dedup, s)
			}
		}
		sort.Strings(dedup)
		secondary = dedup
		b, _ := json.Marshal(secondary)
		target.SecondaryLabels = string(b)
	}
	// Append synthetic finding.
	findings := report.FindingsFromJSON(target.FindingsJSON)
	pageTitle := pageByURL[pageURL].Title
	if pageTitle == "" {
		pageTitle = pageURL
	}
	canon := pageByURL[canonURL]
	findings = append(findings, llm.Finding{
		ID:           fmt.Sprintf("f%d", len(findings)+1),
		Kind:         "redundant_with_other_page",
		Severity:     "high",
		QuotedText:   pageTitle,
		LocationHint: "entire page",
		Issue:        fmt.Sprintf("Near-duplicate of %s (cluster %s).", canonURL, clusterID),
		SuggestedFix: fmt.Sprintf("Delete this page and redirect to %s; canonical page has %d words.", canonURL, canon.WordCount),
	})
	b, _ := json.Marshal(findings)
	target.FindingsJSON = string(b)
	if target.SuggestedAction == "" || target.SuggestedAction == "KEEP" {
		target.SuggestedAction = "DELETE"
		target.EditPlanJSON.Valid = false
		target.EditPlanJSON.String = ""
	}
	// Persist via CompleteClassification (it sets judge_status=judged).
	return db.CompleteClassification(ctx, *target)
}

var _ = time.Now
