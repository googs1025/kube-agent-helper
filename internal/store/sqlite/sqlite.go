package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type SQLiteStore struct {
	db *sql.DB
}

func New(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func runMigrations(db *sql.DB) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	driver, err := migratesqlite.WithInstance(db, &migratesqlite.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) CreateRun(ctx context.Context, run *store.DiagnosticRun) error {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	run.CreatedAt = time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO diagnostic_runs (id, target_json, skills_json, status, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		run.ID, run.TargetJSON, run.SkillsJSON, string(run.Status), run.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) GetRun(ctx context.Context, id string) (*store.DiagnosticRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, target_json, skills_json, status, message, started_at, completed_at, created_at
		 FROM diagnostic_runs WHERE id = ?`, id)
	r, err := scanRun(row)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	return r, err
}

func (s *SQLiteStore) UpdateRunStatus(ctx context.Context, id string, phase store.Phase, msg string) error {
	now := time.Now()
	var (
		result sql.Result
		err    error
	)
	switch phase {
	case store.PhaseRunning:
		result, err = s.db.ExecContext(ctx,
			`UPDATE diagnostic_runs SET status=?, message=?, started_at=? WHERE id=?`,
			string(phase), msg, now, id)
	case store.PhaseSucceeded, store.PhaseFailed:
		result, err = s.db.ExecContext(ctx,
			`UPDATE diagnostic_runs SET status=?, message=?, completed_at=? WHERE id=?`,
			string(phase), msg, now, id)
	default:
		result, err = s.db.ExecContext(ctx,
			`UPDATE diagnostic_runs SET status=?, message=? WHERE id=?`,
			string(phase), msg, id)
	}
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) ListRuns(ctx context.Context, opts store.ListOpts) ([]*store.DiagnosticRun, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, target_json, skills_json, status, message, started_at, completed_at, created_at
		 FROM diagnostic_runs ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]*store.DiagnosticRun, 0)
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *SQLiteStore) CreateFinding(ctx context.Context, f *store.Finding) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	f.CreatedAt = time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO findings
		 (id, run_id, dimension, severity, title, description,
		  resource_kind, resource_namespace, resource_name, suggestion, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		f.ID, f.RunID, f.Dimension, f.Severity, f.Title, f.Description,
		f.ResourceKind, f.ResourceNamespace, f.ResourceName, f.Suggestion, f.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) ListFindings(ctx context.Context, runID string) ([]*store.Finding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, dimension, severity, title, description,
		        resource_kind, resource_namespace, resource_name, suggestion, created_at
		 FROM findings WHERE run_id = ? ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	findings := make([]*store.Finding, 0)
	for rows.Next() {
		f := &store.Finding{}
		if err := rows.Scan(&f.ID, &f.RunID, &f.Dimension, &f.Severity, &f.Title,
			&f.Description, &f.ResourceKind, &f.ResourceNamespace, &f.ResourceName,
			&f.Suggestion, &f.CreatedAt); err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	return findings, rows.Err()
}

func (s *SQLiteStore) UpsertSkill(ctx context.Context, sk *store.Skill) error {
	if sk.ID == "" {
		sk.ID = uuid.NewString()
	}
	sk.UpdatedAt = time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO skills (id, name, dimension, prompt, tools_json, requires_data_json, source, enabled, priority, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(name) DO UPDATE SET
		   dimension=excluded.dimension, prompt=excluded.prompt,
		   tools_json=excluded.tools_json, requires_data_json=excluded.requires_data_json,
		   source=excluded.source, enabled=excluded.enabled, priority=excluded.priority,
		   updated_at=excluded.updated_at`,
		sk.ID, sk.Name, sk.Dimension, sk.Prompt, sk.ToolsJSON, sk.RequiresDataJSON,
		sk.Source, sk.Enabled, sk.Priority, sk.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) ListSkills(ctx context.Context) ([]*store.Skill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, dimension, prompt, tools_json, requires_data_json,
		        source, enabled, priority, updated_at
		 FROM skills ORDER BY priority ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	skills := make([]*store.Skill, 0)
	for rows.Next() {
		sk := &store.Skill{}
		if err := rows.Scan(&sk.ID, &sk.Name, &sk.Dimension, &sk.Prompt,
			&sk.ToolsJSON, &sk.RequiresDataJSON, &sk.Source, &sk.Enabled,
			&sk.Priority, &sk.UpdatedAt); err != nil {
			return nil, err
		}
		skills = append(skills, sk)
	}
	return skills, rows.Err()
}

func (s *SQLiteStore) GetSkill(ctx context.Context, name string) (*store.Skill, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, dimension, prompt, tools_json, requires_data_json,
		        source, enabled, priority, updated_at
		 FROM skills WHERE name = ?`, name)
	sk := &store.Skill{}
	err := row.Scan(&sk.ID, &sk.Name, &sk.Dimension, &sk.Prompt,
		&sk.ToolsJSON, &sk.RequiresDataJSON, &sk.Source, &sk.Enabled,
		&sk.Priority, &sk.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	return sk, err
}

func (s *SQLiteStore) DeleteSkill(ctx context.Context, name string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM skills WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ── Fix methods ──────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateFix(ctx context.Context, f *store.Fix) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	f.CreatedAt = time.Now()
	f.UpdatedAt = f.CreatedAt
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fixes (id, run_id, finding_title, target_kind, target_namespace, target_name,
		  strategy, approval_required, patch_type, patch_content, phase, message,
		  finding_id, before_snapshot, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.ID, f.RunID, f.FindingTitle, f.TargetKind, f.TargetNamespace, f.TargetName,
		f.Strategy, f.ApprovalRequired, f.PatchType, f.PatchContent,
		string(f.Phase), f.Message, f.FindingID, f.BeforeSnapshot, f.CreatedAt, f.UpdatedAt)
	return err
}

func (s *SQLiteStore) GetFix(ctx context.Context, id string) (*store.Fix, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, run_id, finding_title, target_kind, target_namespace, target_name,
		        strategy, approval_required, patch_type, patch_content, phase,
		        approved_by, rollback_snapshot, message, finding_id, before_snapshot,
		        created_at, updated_at
		 FROM fixes WHERE id = ?`, id)
	return scanFix(row)
}

func (s *SQLiteStore) ListFixes(ctx context.Context, opts store.ListOpts) ([]*store.Fix, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, finding_title, target_kind, target_namespace, target_name,
		        strategy, approval_required, patch_type, patch_content, phase,
		        approved_by, rollback_snapshot, message, finding_id, before_snapshot,
		        created_at, updated_at
		 FROM fixes ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fixes := make([]*store.Fix, 0)
	for rows.Next() {
		f, err := scanFix(rows)
		if err != nil {
			return nil, err
		}
		fixes = append(fixes, f)
	}
	return fixes, rows.Err()
}

func (s *SQLiteStore) ListFixesByRun(ctx context.Context, runID string) ([]*store.Fix, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, finding_title, target_kind, target_namespace, target_name,
		        strategy, approval_required, patch_type, patch_content, phase,
		        approved_by, rollback_snapshot, message, finding_id, before_snapshot,
		        created_at, updated_at
		 FROM fixes WHERE run_id = ? ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fixes := make([]*store.Fix, 0)
	for rows.Next() {
		f, err := scanFix(rows)
		if err != nil {
			return nil, err
		}
		fixes = append(fixes, f)
	}
	return fixes, rows.Err()
}

func (s *SQLiteStore) UpdateFixPhase(ctx context.Context, id string, phase store.FixPhase, msg string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE fixes SET phase=?, message=?, updated_at=? WHERE id=?`,
		string(phase), msg, time.Now(), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) UpdateFixApproval(ctx context.Context, id string, approvedBy string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE fixes SET approved_by=?, phase=?, updated_at=? WHERE id=?`,
		approvedBy, string(store.FixPhaseApproved), time.Now(), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) UpdateFixSnapshot(ctx context.Context, id string, snapshot string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE fixes SET rollback_snapshot=?, updated_at=? WHERE id=?`,
		snapshot, time.Now(), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func scanFix(s scanner) (*store.Fix, error) {
	f := &store.Fix{}
	var phase string
	err := s.Scan(&f.ID, &f.RunID, &f.FindingTitle, &f.TargetKind, &f.TargetNamespace,
		&f.TargetName, &f.Strategy, &f.ApprovalRequired, &f.PatchType, &f.PatchContent,
		&phase, &f.ApprovedBy, &f.RollbackSnapshot, &f.Message,
		&f.FindingID, &f.BeforeSnapshot,
		&f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	f.Phase = store.FixPhase(phase)
	return f, nil
}

// scanner unifies *sql.Row and *sql.Rows
type scanner interface {
	Scan(dest ...any) error
}

func scanRun(s scanner) (*store.DiagnosticRun, error) {
	r := &store.DiagnosticRun{}
	var startedAt, completedAt sql.NullTime
	err := s.Scan(&r.ID, &r.TargetJSON, &r.SkillsJSON, &r.Status, &r.Message,
		&startedAt, &completedAt, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	if startedAt.Valid {
		r.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}
	return r, nil
}
