package log

// Event types. These are the canonical dotted identifiers
// used in slog records and persisted in the `events` table.
const (
	PreflightConfigLoaded    = "preflight.config_loaded"
	PreflightOllamaReachable = "preflight.ollama_reachable"
	PreflightOllamaModel     = "preflight.ollama_model_present"
	PreflightUserConfirmed   = "preflight.user_confirmed"
	PreflightUserAborted     = "preflight.user_aborted"

	PhaseCrawlStarted    = "phase.crawl.started"
	PhaseCrawlCompleted  = "phase.crawl.completed"
	PhaseJudgeStarted    = "phase.judge.started"
	PhaseJudgeCompleted  = "phase.judge.completed"
	PhaseEmbedStarted    = "phase.embed.started"
	PhaseEmbedCompleted  = "phase.embed.completed"
	PhaseDedupStarted    = "phase.dedup.started"
	PhaseDedupCompleted  = "phase.dedup.completed"
	PhaseReportStarted   = "phase.report.started"
	PhaseReportCompleted = "phase.report.completed"

	FetchAttempt          = "fetch.attempt"
	FetchSuccess          = "fetch.success"
	FetchRedirect         = "fetch.redirect"
	FetchCanonicalAlias   = "fetch.canonical_alias"
	FetchFailedRetryable  = "fetch.failed_retryable"
	FetchFailedPermanent  = "fetch.failed_permanent"
	FetchNonHTML          = "fetch.non_html"
	FetchBodyTruncated    = "fetch.body_truncated"
	FetchDisallowedRobots = "fetch.disallowed_robots"

	DiscoverSitemapLoaded   = "discover.sitemap_loaded"
	DiscoverSitemapFailed   = "discover.sitemap_failed"
	DiscoverSitemapEntryBad = "discover.sitemap_failed_entry"
	DiscoverSitemapTooLarge = "discover.sitemap_too_large"
	DiscoverFeedLoaded      = "discover.feed_loaded"
	DiscoverFeedFailed      = "discover.feed_failed"
	DiscoverBFSLinkEnqueued = "discover.bfs_link_enqueued"
	DiscoverURLSkipped      = "discover.url_skipped_excluded"

	JudgeAttempt            = "judge.attempt"
	JudgeSuccess            = "judge.success"
	JudgeFailedRetryable    = "judge.failed_retryable"
	JudgeFailedPermanent    = "judge.failed_permanent"
	JudgeSchemaInvalid      = "judge.schema_invalid"
	JudgeSchemaRepaired     = "judge.schema_repaired"
	JudgeOllamaModelLoading = "judge.ollama_model_loading"

	EmbedAttempt         = "embed.attempt"
	EmbedSuccess         = "embed.success"
	EmbedFailedRetryable = "embed.failed_retryable"
	EmbedFailedPermanent = "embed.failed_permanent"

	DedupClusterAssigned  = "dedup.cluster_assigned"
	DedupRelabelRedundant = "dedup.relabel_redundant"

	RetryScheduled = "retry.scheduled"
	RetryExhausted = "retry.exhausted"

	RunStarted   = "run.started"
	RunCompleted = "run.completed"
	RunHeartbeat = "run.heartbeat"
	RunPaused    = "run.paused"
	RunResumed   = "run.resumed"
	RunFailed    = "run.failed"
	RunAborted   = "run.aborted"

	PanicRecovered = "panic.recovered"

	WarnHighEmptyShellRatio = "warn.high_empty_shell_ratio"

	DBBusyRetry = "db.busy_retry"
)
