CREATE TABLE IF NOT EXISTS diagnostic_runs (
    id           TEXT PRIMARY KEY,
    target_json  TEXT NOT NULL,
    skills_json  TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'Pending',
    message      TEXT NOT NULL DEFAULT '',
    started_at   DATETIME,
    completed_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS findings (
    id                  TEXT PRIMARY KEY,
    run_id              TEXT NOT NULL REFERENCES diagnostic_runs(id),
    dimension           TEXT NOT NULL,
    severity            TEXT NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    resource_kind       TEXT NOT NULL DEFAULT '',
    resource_namespace  TEXT NOT NULL DEFAULT '',
    resource_name       TEXT NOT NULL DEFAULT '',
    suggestion          TEXT NOT NULL DEFAULT '',
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS skills (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL UNIQUE,
    dimension           TEXT NOT NULL,
    prompt              TEXT NOT NULL,
    tools_json          TEXT NOT NULL DEFAULT '[]',
    requires_data_json  TEXT NOT NULL DEFAULT '[]',
    source              TEXT NOT NULL DEFAULT 'builtin',
    enabled             INTEGER NOT NULL DEFAULT 1,
    priority            INTEGER NOT NULL DEFAULT 100,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_findings_run_id ON findings(run_id, created_at);
