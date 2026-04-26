package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"strings"
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
	if run.ClusterName == "" {
		run.ClusterName = "local"
	}
	run.CreatedAt = time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO diagnostic_runs (id, cluster_name, target_json, skills_json, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		run.ID, run.ClusterName, run.TargetJSON, run.SkillsJSON, string(run.Status), run.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) GetRun(ctx context.Context, id string) (*store.DiagnosticRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, cluster_name, target_json, skills_json, status, message, started_at, completed_at, created_at
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
	query := `SELECT id, cluster_name, target_json, skills_json, status, message, started_at, completed_at, created_at
	          FROM diagnostic_runs WHERE 1=1`
	args := []interface{}{}
	if opts.ClusterName != "" {
		query += " AND cluster_name = ?"
		args = append(args, opts.ClusterName)
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, opts.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
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
	if f.ClusterName == "" {
		f.ClusterName = "local"
	}
	f.CreatedAt = time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO findings
		 (id, run_id, cluster_name, dimension, severity, title, description,
		  resource_kind, resource_namespace, resource_name, suggestion, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.ID, f.RunID, f.ClusterName, f.Dimension, f.Severity, f.Title, f.Description,
		f.ResourceKind, f.ResourceNamespace, f.ResourceName, f.Suggestion, f.CreatedAt,
	)
	return err
}

func (s *SQLiteStore) ListFindings(ctx context.Context, runID string) ([]*store.Finding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, cluster_name, dimension, severity, title, description,
		        resource_kind, resource_namespace, resource_name, suggestion, created_at
		 FROM findings WHERE run_id = ? ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	findings := make([]*store.Finding, 0)
	for rows.Next() {
		f := &store.Finding{}
		if err := rows.Scan(&f.ID, &f.RunID, &f.ClusterName, &f.Dimension, &f.Severity, &f.Title,
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
	if f.ClusterName == "" {
		f.ClusterName = "local"
	}
	f.CreatedAt = time.Now()
	f.UpdatedAt = f.CreatedAt
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fixes (id, cluster_name, run_id, finding_title, target_kind, target_namespace, target_name,
		  strategy, approval_required, patch_type, patch_content, phase, message,
		  finding_id, before_snapshot, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.ID, f.ClusterName, f.RunID, f.FindingTitle, f.TargetKind, f.TargetNamespace, f.TargetName,
		f.Strategy, f.ApprovalRequired, f.PatchType, f.PatchContent,
		string(f.Phase), f.Message, f.FindingID, f.BeforeSnapshot, f.CreatedAt, f.UpdatedAt)
	return err
}

func (s *SQLiteStore) GetFix(ctx context.Context, id string) (*store.Fix, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, cluster_name, run_id, finding_title, target_kind, target_namespace, target_name,
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
	query := `SELECT id, cluster_name, run_id, finding_title, target_kind, target_namespace, target_name,
	                 strategy, approval_required, patch_type, patch_content, phase,
	                 approved_by, rollback_snapshot, message, finding_id, before_snapshot,
	                 created_at, updated_at
	          FROM fixes WHERE 1=1`
	args := []interface{}{}
	if opts.ClusterName != "" {
		query += " AND cluster_name = ?"
		args = append(args, opts.ClusterName)
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, opts.Offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
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
		`SELECT id, cluster_name, run_id, finding_title, target_kind, target_namespace, target_name,
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
	err := s.Scan(&f.ID, &f.ClusterName, &f.RunID, &f.FindingTitle, &f.TargetKind, &f.TargetNamespace,
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
	err := s.Scan(&r.ID, &r.ClusterName, &r.TargetJSON, &r.SkillsJSON, &r.Status, &r.Message,
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

func (s *SQLiteStore) UpsertEvent(ctx context.Context, e *store.Event) error {
	if e.ClusterName == "" {
		e.ClusterName = "local"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (uid, cluster_name, namespace, kind, name, reason, message, type, count, first_time, last_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			count     = excluded.count,
			last_time = excluded.last_time,
			message   = excluded.message`,
		e.UID, e.ClusterName, e.Namespace, e.Kind, e.Name, e.Reason, e.Message,
		e.Type, e.Count, e.FirstTime.Unix(), e.LastTime.Unix(),
	)
	return err
}

func (s *SQLiteStore) ListEvents(ctx context.Context, opts store.ListEventsOpts) ([]*store.Event, error) {
	query := `SELECT id, uid, cluster_name, namespace, kind, name, reason, message, type, count, first_time, last_time, created_at
	          FROM events WHERE 1=1`
	args := []interface{}{}

	if opts.ClusterName != "" {
		query += " AND cluster_name = ?"
		args = append(args, opts.ClusterName)
	}
	if opts.Namespace != "" {
		query += " AND namespace = ?"
		args = append(args, opts.Namespace)
	}
	if opts.Name != "" {
		query += " AND name = ?"
		args = append(args, opts.Name)
	}
	if opts.Type != "" {
		query += " AND type = ?"
		args = append(args, opts.Type)
	}
	if opts.SinceMinutes > 0 {
		cutoff := time.Now().Add(-time.Duration(opts.SinceMinutes) * time.Minute).Unix()
		query += " AND last_time >= ?"
		args = append(args, cutoff)
	}
	query += " ORDER BY last_time DESC"
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*store.Event
	for rows.Next() {
		var ev store.Event
		var firstTS, lastTS int64
		var createdAt string
		if err := rows.Scan(&ev.ID, &ev.UID, &ev.ClusterName, &ev.Namespace, &ev.Kind, &ev.Name,
			&ev.Reason, &ev.Message, &ev.Type, &ev.Count, &firstTS, &lastTS, &createdAt); err != nil {
			return nil, err
		}
		ev.FirstTime = time.Unix(firstTS, 0)
		ev.LastTime = time.Unix(lastTS, 0)
		events = append(events, &ev)
	}
	return events, rows.Err()
}

func (s *SQLiteStore) InsertMetricSnapshot(ctx context.Context, snap *store.MetricSnapshot) error {
	if snap.ClusterName == "" {
		snap.ClusterName = "local"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO metric_snapshots (cluster_name, query, labels_json, value, ts) VALUES (?, ?, ?, ?, ?)`,
		snap.ClusterName, snap.Query, snap.LabelsJSON, snap.Value, snap.Ts.Unix(),
	)
	return err
}

func (s *SQLiteStore) QueryMetricHistory(ctx context.Context, query string, sinceMinutes int) ([]*store.MetricSnapshot, error) {
	cutoff := time.Now().Add(-time.Duration(sinceMinutes) * time.Minute).Unix()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, cluster_name, query, labels_json, value, ts, created_at
		 FROM metric_snapshots WHERE query = ? AND ts >= ?
		 ORDER BY ts DESC LIMIT 500`,
		query, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []*store.MetricSnapshot
	for rows.Next() {
		var snap store.MetricSnapshot
		var ts int64
		var createdAt string
		if err := rows.Scan(&snap.ID, &snap.ClusterName, &snap.Query, &snap.LabelsJSON, &snap.Value, &ts, &createdAt); err != nil {
			return nil, err
		}
		snap.Ts = time.Unix(ts, 0)
		snaps = append(snaps, &snap)
	}
	return snaps, rows.Err()
}

func (s *SQLiteStore) PurgeOldEvents(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM events WHERE last_time < ?`, before.Unix())
	return err
}

func (s *SQLiteStore) PurgeOldMetrics(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM metric_snapshots WHERE ts < ?`, before.Unix())
	return err
}

// ── Run log methods ──────────────────────────────────────────────────────────

func (s *SQLiteStore) AppendRunLog(ctx context.Context, log store.RunLog) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO run_logs (run_id, timestamp, type, message, data) VALUES (?, ?, ?, ?, ?)`,
		log.RunID, log.Timestamp, log.Type, log.Message, log.Data,
	)
	return err
}

func (s *SQLiteStore) ListRunLogs(ctx context.Context, runID string, afterID int64) ([]store.RunLog, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, timestamp, type, message, COALESCE(data,'')
		 FROM run_logs WHERE run_id = ? AND id > ? ORDER BY id ASC`, runID, afterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []store.RunLog
	for rows.Next() {
		var l store.RunLog
		if err := rows.Scan(&l.ID, &l.RunID, &l.Timestamp, &l.Type, &l.Message, &l.Data); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

// ── Paginated list methods ──────────────────────────────────────────────────

var allowedRunSort = map[string]bool{"created_at": true, "status": true}
var allowedFixSort = map[string]bool{"created_at": true, "updated_at": true, "phase": true}

func sanitizeSortOrder(order string) string {
	if order == "asc" || order == "ASC" {
		return "ASC"
	}
	return "DESC"
}

func (s *SQLiteStore) ListRunsPaginated(ctx context.Context, opts store.ListOpts) (store.PaginatedResult[*store.DiagnosticRun], error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 20
	}
	if opts.PageSize > 100 {
		opts.PageSize = 100
	}
	sortCol := "created_at"
	if allowedRunSort[opts.SortBy] {
		sortCol = opts.SortBy
	}
	sortOrder := sanitizeSortOrder(opts.SortOrder)

	where := " WHERE 1=1"
	args := []interface{}{}
	if opts.ClusterName != "" {
		where += " AND cluster_name = ?"
		args = append(args, opts.ClusterName)
	}
	if f := opts.Filters; f != nil {
		if v, ok := f["phase"]; ok && v != "" {
			where += " AND status = ?"
			args = append(args, v)
		}
		if v, ok := f["status"]; ok && v != "" {
			where += " AND status = ?"
			args = append(args, v)
		}
	}

	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM diagnostic_runs"+where, args...).Scan(&total)
	if err != nil {
		return store.PaginatedResult[*store.DiagnosticRun]{}, err
	}

	query := fmt.Sprintf(`SELECT id, cluster_name, target_json, skills_json, status, message, started_at, completed_at, created_at
	          FROM diagnostic_runs%s ORDER BY %s %s LIMIT ? OFFSET ?`, where, sortCol, sortOrder)
	args = append(args, opts.PageSize, (opts.Page-1)*opts.PageSize)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.PaginatedResult[*store.DiagnosticRun]{}, err
	}
	defer rows.Close()
	runs := make([]*store.DiagnosticRun, 0)
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return store.PaginatedResult[*store.DiagnosticRun]{}, err
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return store.PaginatedResult[*store.DiagnosticRun]{}, err
	}

	return store.PaginatedResult[*store.DiagnosticRun]{
		Items: runs, Total: total, Page: opts.Page, PageSize: opts.PageSize,
	}, nil
}

func (s *SQLiteStore) ListFixesPaginated(ctx context.Context, opts store.ListOpts) (store.PaginatedResult[*store.Fix], error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 20
	}
	if opts.PageSize > 100 {
		opts.PageSize = 100
	}
	sortCol := "created_at"
	if allowedFixSort[opts.SortBy] {
		sortCol = opts.SortBy
	}
	sortOrder := sanitizeSortOrder(opts.SortOrder)

	where := " WHERE 1=1"
	args := []interface{}{}
	if opts.ClusterName != "" {
		where += " AND cluster_name = ?"
		args = append(args, opts.ClusterName)
	}
	if f := opts.Filters; f != nil {
		if v, ok := f["phase"]; ok && v != "" {
			where += " AND phase = ?"
			args = append(args, v)
		}
		if v, ok := f["namespace"]; ok && v != "" {
			where += " AND target_namespace = ?"
			args = append(args, v)
		}
	}

	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM fixes"+where, args...).Scan(&total)
	if err != nil {
		return store.PaginatedResult[*store.Fix]{}, err
	}

	selectCols := `id, cluster_name, run_id, finding_title, target_kind, target_namespace, target_name,
	                 strategy, approval_required, patch_type, patch_content, phase,
	                 approved_by, rollback_snapshot, message, finding_id, before_snapshot,
	                 created_at, updated_at`
	query := fmt.Sprintf("SELECT %s FROM fixes%s ORDER BY %s %s LIMIT ? OFFSET ?",
		selectCols, where, sortCol, sortOrder)
	args = append(args, opts.PageSize, (opts.Page-1)*opts.PageSize)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.PaginatedResult[*store.Fix]{}, err
	}
	defer rows.Close()
	fixes := make([]*store.Fix, 0)
	for rows.Next() {
		f, err := scanFix(rows)
		if err != nil {
			return store.PaginatedResult[*store.Fix]{}, err
		}
		fixes = append(fixes, f)
	}
	if err := rows.Err(); err != nil {
		return store.PaginatedResult[*store.Fix]{}, err
	}

	return store.PaginatedResult[*store.Fix]{
		Items: fixes, Total: total, Page: opts.Page, PageSize: opts.PageSize,
	}, nil
}

func (s *SQLiteStore) ListEventsPaginated(ctx context.Context, opts store.ListEventsOpts, page, pageSize int) (store.PaginatedResult[*store.Event], error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	where := " WHERE 1=1"
	args := []interface{}{}
	if opts.ClusterName != "" {
		where += " AND cluster_name = ?"
		args = append(args, opts.ClusterName)
	}
	if opts.Namespace != "" {
		where += " AND namespace = ?"
		args = append(args, opts.Namespace)
	}
	if opts.Name != "" {
		where += " AND name = ?"
		args = append(args, opts.Name)
	}
	if opts.Type != "" {
		where += " AND type = ?"
		args = append(args, opts.Type)
	}
	if opts.SinceMinutes > 0 {
		cutoff := time.Now().Add(-time.Duration(opts.SinceMinutes) * time.Minute).Unix()
		where += " AND last_time >= ?"
		args = append(args, cutoff)
	}

	var total int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events"+where, args...).Scan(&total)
	if err != nil {
		return store.PaginatedResult[*store.Event]{}, err
	}

	query := fmt.Sprintf(`SELECT id, uid, cluster_name, namespace, kind, name, reason, message, type, count, first_time, last_time, created_at
	          FROM events%s ORDER BY last_time DESC LIMIT ? OFFSET ?`, where)
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return store.PaginatedResult[*store.Event]{}, err
	}
	defer rows.Close()

	events := make([]*store.Event, 0)
	for rows.Next() {
		var ev store.Event
		var firstTS, lastTS int64
		var createdAt string
		if err := rows.Scan(&ev.ID, &ev.UID, &ev.ClusterName, &ev.Namespace, &ev.Kind, &ev.Name,
			&ev.Reason, &ev.Message, &ev.Type, &ev.Count, &firstTS, &lastTS, &createdAt); err != nil {
			return store.PaginatedResult[*store.Event]{}, err
		}
		ev.FirstTime = time.Unix(firstTS, 0)
		ev.LastTime = time.Unix(lastTS, 0)
		events = append(events, &ev)
	}
	if err := rows.Err(); err != nil {
		return store.PaginatedResult[*store.Event]{}, err
	}

	return store.PaginatedResult[*store.Event]{
		Items: events, Total: total, Page: page, PageSize: pageSize,
	}, nil
}

// ── Notification config methods ───────────────────────────────────────────────

func (s *SQLiteStore) ListNotificationConfigs(ctx context.Context) ([]*store.NotificationConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, type, webhook_url, secret, events, enabled, created_at, updated_at
		 FROM notification_configs ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	configs := make([]*store.NotificationConfig, 0)
	for rows.Next() {
		cfg := &store.NotificationConfig{}
		var enabled int
		if err := rows.Scan(&cfg.ID, &cfg.Name, &cfg.Type, &cfg.WebhookURL, &cfg.Secret,
			&cfg.Events, &enabled, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		cfg.Enabled = enabled != 0
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

func (s *SQLiteStore) GetNotificationConfig(ctx context.Context, id string) (*store.NotificationConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, type, webhook_url, secret, events, enabled, created_at, updated_at
		 FROM notification_configs WHERE id = ?`, id)
	cfg := &store.NotificationConfig{}
	var enabled int
	err := row.Scan(&cfg.ID, &cfg.Name, &cfg.Type, &cfg.WebhookURL, &cfg.Secret,
		&cfg.Events, &enabled, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cfg.Enabled = enabled != 0
	return cfg, nil
}

func (s *SQLiteStore) CreateNotificationConfig(ctx context.Context, cfg *store.NotificationConfig) error {
	if cfg.ID == "" {
		cfg.ID = uuid.NewString()
	}
	now := time.Now()
	cfg.CreatedAt = now
	cfg.UpdatedAt = now
	enabledInt := 0
	if cfg.Enabled {
		enabledInt = 1
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notification_configs (id, name, type, webhook_url, secret, events, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cfg.ID, cfg.Name, cfg.Type, cfg.WebhookURL, cfg.Secret, cfg.Events, enabledInt, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

func (s *SQLiteStore) UpdateNotificationConfig(ctx context.Context, cfg *store.NotificationConfig) error {
	cfg.UpdatedAt = time.Now()
	enabledInt := 0
	if cfg.Enabled {
		enabledInt = 1
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE notification_configs SET name=?, type=?, webhook_url=?, secret=?, events=?, enabled=?, updated_at=? WHERE id=?`,
		cfg.Name, cfg.Type, cfg.WebhookURL, cfg.Secret, cfg.Events, enabledInt, cfg.UpdatedAt, cfg.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) DeleteNotificationConfig(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM notification_configs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ── Batch operations ──────────────────────────────────────────────────────────

// DeleteRuns 级联删除一批 run。
//
// 子表（findings / run_logs / fixes）通过 run_id FK 引用 diagnostic_runs，
// 直接删主表会触发 FOREIGN KEY constraint failed。这里在事务内先清子表再删主表，
// 保证幂等：任何 ID 不存在都不报错。
func (s *SQLiteStore) DeleteRuns(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	in := strings.Join(placeholders, ",")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"findings", "run_logs", "fixes"} {
		q := fmt.Sprintf("DELETE FROM %s WHERE run_id IN (%s)", table, in)
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	q := fmt.Sprintf("DELETE FROM diagnostic_runs WHERE id IN (%s)", in)
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("delete diagnostic_runs: %w", err)
	}
	return tx.Commit()
}

func (s *SQLiteStore) BatchUpdateFixPhase(ctx context.Context, ids []string, phase store.FixPhase, msg string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := []interface{}{string(phase), msg, time.Now()}
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf("UPDATE fixes SET phase=?, message=?, updated_at=? WHERE id IN (%s)",
		strings.Join(placeholders, ","))
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}
