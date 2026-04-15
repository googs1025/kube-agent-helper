CREATE TABLE IF NOT EXISTS fixes (
    id                TEXT PRIMARY KEY,
    run_id            TEXT NOT NULL,
    finding_title     TEXT NOT NULL,
    target_kind       TEXT NOT NULL,
    target_namespace  TEXT NOT NULL,
    target_name       TEXT NOT NULL,
    strategy          TEXT NOT NULL DEFAULT 'auto',
    approval_required INTEGER NOT NULL DEFAULT 1,
    patch_type        TEXT NOT NULL DEFAULT 'strategic-merge',
    patch_content     TEXT NOT NULL,
    phase             TEXT NOT NULL DEFAULT 'PendingApproval',
    approved_by       TEXT NOT NULL DEFAULT '',
    rollback_snapshot TEXT NOT NULL DEFAULT '',
    message           TEXT NOT NULL DEFAULT '',
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_fixes_run_id ON fixes(run_id);
CREATE INDEX idx_fixes_phase ON fixes(phase);
