-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- One row per `redline scan` invocation
CREATE TABLE IF NOT EXISTS runs (
    id                  TEXT PRIMARY KEY,
    started_at          TEXT NOT NULL DEFAULT (datetime('now')),
    completed_at        TEXT,
    site_url            TEXT NOT NULL,
    prompts_yaml_sha256 TEXT NOT NULL,
    config_json         TEXT NOT NULL,
    llm_provider        TEXT NOT NULL,
    llm_model           TEXT NOT NULL,
    embedding_provider  TEXT,
    embedding_model     TEXT,
    redline_version   TEXT NOT NULL,
    status              TEXT NOT NULL CHECK (status IN (
                            'running','completed','failed',
                            'paused_budget','paused_user','paused_provider_auth','aborted'
                        )),
    error               TEXT,
    pid                 INTEGER,
    last_heartbeat_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_runs_started ON runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_status  ON runs(status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_one_active
    ON runs(site_url, prompts_yaml_sha256)
    WHERE status IN ('running','paused_user','paused_budget','paused_provider_auth');

CREATE TABLE IF NOT EXISTS urls (
    url                TEXT PRIMARY KEY,
    run_id             TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    discovered_via     TEXT NOT NULL,
    discovered_from    TEXT,
    depth              INTEGER NOT NULL DEFAULT 0,
    status             TEXT NOT NULL DEFAULT 'discovered' CHECK (status IN (
                          'discovered','claimed','fetching','fetched','failed','skipped'
                       )),
    claimed_by         TEXT,
    claim_expires_at   TEXT,
    attempt_count      INTEGER NOT NULL DEFAULT 0,
    next_retry_at      TEXT,
    last_error         TEXT,
    discovered_at      TEXT NOT NULL DEFAULT (datetime('now')),
    fetched_at         TEXT,
    final_url          TEXT,
    status_code        INTEGER
);

CREATE INDEX IF NOT EXISTS idx_urls_run_status ON urls(run_id, status);
CREATE INDEX IF NOT EXISTS idx_urls_next_retry ON urls(next_retry_at)
    WHERE status = 'failed' AND next_retry_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS url_aliases (
    alias_url     TEXT PRIMARY KEY,
    canonical_url TEXT NOT NULL,
    alias_type    TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS pages (
    url               TEXT PRIMARY KEY,
    first_run_id      TEXT NOT NULL REFERENCES runs(id),
    final_url         TEXT NOT NULL,
    title             TEXT,
    meta_description  TEXT,
    body_text         TEXT,
    word_count        INTEGER NOT NULL DEFAULT 0,
    is_empty_shell    INTEGER NOT NULL DEFAULT 0,
    truncated         INTEGER NOT NULL DEFAULT 0,
    status_code       INTEGER,
    content_type      TEXT,
    last_modified     TEXT,
    published_date    TEXT,
    discovered_at     TEXT NOT NULL DEFAULT (datetime('now')),
    fetched_at        TEXT,
    raw_body_sha256   TEXT,
    body_size_bytes   INTEGER NOT NULL DEFAULT 0,
    fetch_error       TEXT
);

CREATE INDEX IF NOT EXISTS idx_pages_run ON pages(first_run_id);
CREATE INDEX IF NOT EXISTS idx_pages_status ON pages(status_code);

CREATE TABLE IF NOT EXISTS links (
    from_url    TEXT NOT NULL REFERENCES pages(url) ON DELETE CASCADE,
    to_url      TEXT NOT NULL,
    anchor      TEXT,
    is_internal INTEGER NOT NULL,
    PRIMARY KEY (from_url, to_url)
);

CREATE INDEX IF NOT EXISTS idx_links_to ON links(to_url);

CREATE TABLE IF NOT EXISTS classifications (
    run_id             TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    page_url           TEXT NOT NULL REFERENCES pages(url) ON DELETE CASCADE,
    judge_status       TEXT NOT NULL DEFAULT 'pending' CHECK (judge_status IN (
                          'pending','claimed','judging','judged','failed','failed_schema'
                       )),
    claimed_by         TEXT,
    claim_expires_at   TEXT,
    attempt_count      INTEGER NOT NULL DEFAULT 0,
    primary_label      TEXT,
    secondary_labels   TEXT NOT NULL DEFAULT '[]',
    confidence         REAL,
    rationale          TEXT,
    affected_prompts   TEXT NOT NULL DEFAULT '[]',
    suggested_action   TEXT,
    findings_json      TEXT NOT NULL DEFAULT '[]',
    edit_plan_json     TEXT,
    page_summary_json  TEXT,
    input_tokens       INTEGER NOT NULL DEFAULT 0,
    output_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_hit_tokens   INTEGER NOT NULL DEFAULT 0,
    latency_ms         INTEGER NOT NULL DEFAULT 0,
    raw_response       TEXT,
    error              TEXT,
    judged_at          TEXT,
    PRIMARY KEY (run_id, page_url)
);

CREATE INDEX IF NOT EXISTS idx_classifications_status ON classifications(run_id, judge_status);
CREATE INDEX IF NOT EXISTS idx_classifications_label  ON classifications(primary_label);

CREATE TABLE IF NOT EXISTS embeddings (
    run_id       TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    page_url     TEXT NOT NULL REFERENCES pages(url) ON DELETE CASCADE,
    provider     TEXT NOT NULL,
    model        TEXT NOT NULL,
    dims         INTEGER NOT NULL,
    vector       BLOB NOT NULL,
    embed_status TEXT NOT NULL DEFAULT 'pending' CHECK (embed_status IN (
                    'pending','embedding','done','failed'
                 )),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    error        TEXT,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (run_id, page_url, provider, model)
);

CREATE INDEX IF NOT EXISTS idx_embeddings_status ON embeddings(run_id, embed_status);

CREATE TABLE IF NOT EXISTS duplicates (
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    page_url_a  TEXT NOT NULL,
    page_url_b  TEXT NOT NULL,
    similarity  REAL NOT NULL,
    cluster_id  TEXT NOT NULL,
    PRIMARY KEY (run_id, page_url_a, page_url_b),
    CHECK (page_url_a < page_url_b)
);

CREATE INDEX IF NOT EXISTS idx_duplicates_cluster ON duplicates(run_id, cluster_id);

CREATE TABLE IF NOT EXISTS api_calls (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id         TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    provider       TEXT NOT NULL,
    operation      TEXT NOT NULL,
    model          TEXT,
    page_url       TEXT,
    attempt_number INTEGER NOT NULL DEFAULT 1,
    input_tokens   INTEGER NOT NULL DEFAULT 0,
    cached_tokens  INTEGER NOT NULL DEFAULT 0,
    output_tokens  INTEGER NOT NULL DEFAULT 0,
    http_status    INTEGER,
    succeeded      INTEGER NOT NULL,
    error          TEXT,
    latency_ms     INTEGER NOT NULL DEFAULT 0,
    request_id     TEXT,
    called_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_api_calls_run       ON api_calls(run_id);
CREATE INDEX IF NOT EXISTS idx_api_calls_page      ON api_calls(page_url);
CREATE INDEX IF NOT EXISTS idx_api_calls_succeeded ON api_calls(succeeded);

CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id      TEXT REFERENCES runs(id) ON DELETE CASCADE,
    timestamp   TEXT NOT NULL DEFAULT (datetime('now', 'subsec')),
    phase       TEXT NOT NULL,
    severity    TEXT NOT NULL CHECK (severity IN ('debug','info','warn','error')),
    event_type  TEXT NOT NULL,
    url         TEXT,
    message     TEXT NOT NULL,
    payload_json TEXT,
    worker_id   TEXT
);

CREATE INDEX IF NOT EXISTS idx_events_run_phase_ts ON events(run_id, phase, timestamp);
CREATE INDEX IF NOT EXISTS idx_events_severity     ON events(severity);
CREATE INDEX IF NOT EXISTS idx_events_event_type   ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_url          ON events(url);

CREATE TABLE IF NOT EXISTS reports (
    run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    format      TEXT NOT NULL,
    rendered_at TEXT NOT NULL DEFAULT (datetime('now')),
    sha256      TEXT NOT NULL,
    content     BLOB NOT NULL,
    PRIMARY KEY (run_id, format)
);

INSERT OR IGNORE INTO schema_migrations(version) VALUES (1);
