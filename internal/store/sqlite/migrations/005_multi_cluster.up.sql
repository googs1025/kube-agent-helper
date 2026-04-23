ALTER TABLE diagnostic_runs   ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';
ALTER TABLE findings          ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';
ALTER TABLE fixes             ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';
ALTER TABLE events            ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';
ALTER TABLE metric_snapshots  ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';

CREATE INDEX IF NOT EXISTS idx_runs_cluster    ON diagnostic_runs(cluster_name);
CREATE INDEX IF NOT EXISTS idx_fixes_cluster   ON fixes(cluster_name);
CREATE INDEX IF NOT EXISTS idx_events_cluster  ON events(cluster_name);
CREATE INDEX IF NOT EXISTS idx_metrics_cluster ON metric_snapshots(cluster_name);