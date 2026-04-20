-- events table: K8s Warning events, 7-day retention
CREATE TABLE IF NOT EXISTS events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uid        TEXT    NOT NULL UNIQUE,
    namespace  TEXT    NOT NULL DEFAULT '',
    kind       TEXT    NOT NULL DEFAULT '',
    name       TEXT    NOT NULL DEFAULT '',
    reason     TEXT    NOT NULL DEFAULT '',
    message    TEXT    NOT NULL DEFAULT '',
    type       TEXT    NOT NULL DEFAULT 'Warning',
    count      INTEGER NOT NULL DEFAULT 1,
    first_time INTEGER NOT NULL DEFAULT 0,
    last_time  INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_events_namespace_name ON events(namespace, name);
CREATE INDEX IF NOT EXISTS idx_events_last_time      ON events(last_time);
CREATE INDEX IF NOT EXISTS idx_events_type           ON events(type);

-- metric_snapshots table: Prometheus metric snapshots, 7-day retention
CREATE TABLE IF NOT EXISTS metric_snapshots (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    query       TEXT    NOT NULL,
    labels_json TEXT    NOT NULL DEFAULT '{}',
    value       REAL    NOT NULL,
    ts          INTEGER NOT NULL,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_metrics_query_ts ON metric_snapshots(query, ts);
