package main

import "github.com/spf13/cobra"

// scanFlags holds every flag exposed by `scan` (and selectively by the
// other stage subcommands). One struct, populated lazily
// during command execution.
type scanFlags struct {
	Site            string
	Prompts         string
	Output          string
	Format          []string
	LLMProvider     string
	Model           string
	OllamaURL       string
	OllamaTimeout   string
	OllamaKeepAlive string
	EmbProvider     string
	EmbModel        string
	FindDuplicates  bool
	DupThreshold    float64
	MinPriority     float64
	MaxPages        int
	MaxDepth        int
	Include         []string
	Exclude         []string
	RespectCanon    bool
	IgnoreRobots    bool
	UserAgent       string
	Rate            float64
	Concurrency     int
	JudgeConc       int
	MaxRetries      int
	RetryBaseDelay  string
	RetryMaxDelay   string
	JudgePromptPath string
	Resume          bool
	CheckpointEvery int
	MDTopN          int
	ThinPageWords   int
	EmptyShellWarn  float64
	DryRun          bool
	ConfirmLocal    bool
	AutoPull        bool
	HTTPTimeout     string
	LogFile         string
	PrintSchema     bool
}

var sflags = &scanFlags{}

func addScanFlags(c *cobra.Command) {
	c.Flags().StringVar(&sflags.Site, "site", "", "Root URL to crawl (required)")
	c.Flags().StringVar(&sflags.Prompts, "prompts", "", "Path to prompts.yaml (required)")
	c.Flags().StringVar(&sflags.Output, "output", "./redline-report", "Output directory")
	c.Flags().StringSliceVar(&sflags.Format, "format", []string{"json", "markdown"}, "Output formats")
	c.Flags().StringVar(&sflags.LLMProvider, "llm-provider", "ollama", "ollama|anthropic")
	c.Flags().StringVar(&sflags.Model, "model", "", "Judge model (default per provider)")
	c.Flags().StringVar(&sflags.OllamaURL, "ollama-url", "http://localhost:11434", "Ollama HTTP endpoint")
	c.Flags().StringVar(&sflags.OllamaTimeout, "ollama-timeout", "5m", "Per-request Ollama timeout")
	c.Flags().StringVar(&sflags.OllamaKeepAlive, "ollama-keepalive", "30m", "Ollama keep_alive")
	c.Flags().StringVar(&sflags.EmbProvider, "embedding-provider", "ollama", "ollama|openai|voyage")
	c.Flags().StringVar(&sflags.EmbModel, "embedding-model", "", "Embedding model (default per provider)")
	c.Flags().BoolVar(&sflags.FindDuplicates, "find-duplicates", true, "Run Pass 2 (embeddings + dedup)")
	c.Flags().Float64Var(&sflags.DupThreshold, "duplicate-threshold", 0.92, "Cosine similarity for duplicates")
	c.Flags().IntVar(&sflags.MaxPages, "max-pages", 5000, "Max pages to crawl (0 = unlimited)")
	c.Flags().IntVar(&sflags.MaxDepth, "max-depth", 6, "Max BFS depth (0 = unlimited)")
	c.Flags().StringSliceVar(&sflags.Include, "include", nil, "Regex(es) URLs must match")
	c.Flags().StringSliceVar(&sflags.Exclude, "exclude", nil, "Regex(es) URLs to skip")
	c.Flags().BoolVar(&sflags.RespectCanon, "respect-canonical", true, "Honor <link rel=canonical>")
	c.Flags().BoolVar(&sflags.IgnoreRobots, "ignore-robots", false, "Ignore robots.txt")
	c.Flags().StringVar(&sflags.UserAgent, "user-agent", "", "Override User-Agent")
	c.Flags().Float64Var(&sflags.Rate, "rate", 2.0, "Max requests per second to the target host")
	c.Flags().IntVar(&sflags.Concurrency, "concurrency", 4, "Crawl worker concurrency")
	c.Flags().IntVar(&sflags.JudgeConc, "judge-concurrency", 0, "Judge worker concurrency (provider-specific default)")
	c.Flags().IntVar(&sflags.MaxRetries, "max-retries", 5, "Max retry attempts")
	c.Flags().StringVar(&sflags.RetryBaseDelay, "retry-base-delay", "1s", "Base delay for exponential backoff")
	c.Flags().StringVar(&sflags.RetryMaxDelay, "retry-max-delay", "2m", "Cap on backoff per attempt")
	c.Flags().StringVar(&sflags.JudgePromptPath, "judge-prompt", "", "Override the built-in judge prompt template")
	c.Flags().BoolVar(&sflags.Resume, "resume", true, "Resume from existing DB")
	c.Flags().IntVar(&sflags.CheckpointEvery, "checkpoint-every", 10, "DB events flush interval (items)")
	c.Flags().IntVar(&sflags.MDTopN, "md-top-n", 100, "Pages in detailed MD section (0 = all)")
	c.Flags().IntVar(&sflags.ThinPageWords, "thin-page-words", 50, "Word threshold for auto-Thin label")
	c.Flags().Float64Var(&sflags.EmptyShellWarn, "empty-shell-warn-ratio", 0.30, "Ratio of empty shells to trigger warning")
	c.Flags().BoolVar(&sflags.DryRun, "dry-run", false, "Crawl + estimate only; no LLM or embedding")
	c.Flags().BoolVar(&sflags.ConfirmLocal, "confirm-local", false, "Force confirmation prompt for local runs")
	c.Flags().BoolVar(&sflags.AutoPull, "auto-pull", false, "Auto-pull missing Ollama models")
	c.Flags().StringVar(&sflags.HTTPTimeout, "http-timeout", "60s", "Per-request HTTP timeout")
	c.Flags().StringVar(&sflags.LogFile, "log-file", "", "Path to also stream JSON logs to")
	c.Flags().BoolVar(&sflags.PrintSchema, "print-schema", false, "Print the embedded prompts.yaml JSON Schema and exit")
}

func addCrawlFlags(c *cobra.Command) {
	c.Flags().StringVar(&sflags.Site, "site", "", "Root URL to crawl (required)")
	c.Flags().StringVar(&sflags.Prompts, "prompts", "", "Path to prompts.yaml")
	c.Flags().IntVar(&sflags.MaxPages, "max-pages", 5000, "Max pages to crawl")
	c.Flags().IntVar(&sflags.MaxDepth, "max-depth", 6, "Max BFS depth")
	c.Flags().Float64Var(&sflags.Rate, "rate", 2.0, "Max requests per second")
	c.Flags().IntVar(&sflags.Concurrency, "concurrency", 4, "Worker concurrency")
	c.Flags().StringSliceVar(&sflags.Include, "include", nil, "Include regex")
	c.Flags().StringSliceVar(&sflags.Exclude, "exclude", nil, "Exclude regex")
	c.Flags().BoolVar(&sflags.RespectCanon, "respect-canonical", true, "Honor canonical")
	c.Flags().BoolVar(&sflags.IgnoreRobots, "ignore-robots", false, "Ignore robots")
	c.Flags().StringVar(&sflags.UserAgent, "user-agent", "", "Override UA")
	c.Flags().IntVar(&sflags.MaxRetries, "max-retries", 5, "Retry attempts")
	c.Flags().BoolVar(&sflags.Resume, "resume", true, "Resume")
}

func addJudgeFlags(c *cobra.Command) {
	c.Flags().StringVar(&sflags.Site, "site", "", "Site (for run lookup)")
	c.Flags().StringVar(&sflags.Prompts, "prompts", "", "Prompts file")
	c.Flags().StringVar(&sflags.LLMProvider, "llm-provider", "ollama", "ollama|anthropic")
	c.Flags().StringVar(&sflags.Model, "model", "", "Judge model")
	c.Flags().StringVar(&sflags.OllamaURL, "ollama-url", "http://localhost:11434", "Ollama URL")
	c.Flags().IntVar(&sflags.JudgeConc, "judge-concurrency", 0, "Judge concurrency")
	c.Flags().IntVar(&sflags.MaxRetries, "max-retries", 5, "Retry attempts")
}

func addEmbedFlags(c *cobra.Command) {
	c.Flags().StringVar(&sflags.Site, "site", "", "Site (for run lookup)")
	c.Flags().StringVar(&sflags.EmbProvider, "embedding-provider", "ollama", "ollama|openai|voyage")
	c.Flags().StringVar(&sflags.EmbModel, "embedding-model", "", "Embedding model")
	c.Flags().Float64Var(&sflags.DupThreshold, "duplicate-threshold", 0.92, "Cosine threshold")
	c.Flags().IntVar(&sflags.Concurrency, "concurrency", 4, "Concurrency")
}

func addReportFlags(c *cobra.Command) {
	c.Flags().StringVar(&sflags.Output, "output", "", "Output path or directory")
	c.Flags().StringSliceVar(&sflags.Format, "format", []string{"markdown"}, "Output formats")
	c.Flags().StringSliceVar(&sflags.Include, "filter-label", nil, "Only include these labels")
	c.Flags().Float64Var(&sflags.MinPriority, "min-priority", 0, "Minimum priority")
	c.Flags().IntVar(&sflags.MDTopN, "top", 0, "Top N by priority (0 = all)")
}
