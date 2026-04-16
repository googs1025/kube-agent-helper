ALTER TABLE fixes ADD COLUMN finding_id TEXT NOT NULL DEFAULT '';
ALTER TABLE fixes ADD COLUMN before_snapshot TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_fixes_finding_id ON fixes(finding_id);
