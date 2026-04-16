DROP INDEX IF EXISTS idx_fixes_finding_id;
ALTER TABLE fixes DROP COLUMN before_snapshot;
ALTER TABLE fixes DROP COLUMN finding_id;
