ALTER TABLE diagnostic_runs   DROP COLUMN cluster_name;
ALTER TABLE findings          DROP COLUMN cluster_name;
ALTER TABLE fixes             DROP COLUMN cluster_name;
ALTER TABLE events            DROP COLUMN cluster_name;
ALTER TABLE metric_snapshots  DROP COLUMN cluster_name;
