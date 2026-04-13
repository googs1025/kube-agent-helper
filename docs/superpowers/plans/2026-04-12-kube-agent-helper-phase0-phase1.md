# kube-agent-helper Phase 0 + Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Operator MVP — `kubectl apply -f run.yaml` triggers a complete Kubernetes diagnostic run across 3 built-in skills, with findings persisted to SQLite and a REST API for retrieval.

**Architecture:** Vertical slice (Path B). Single Go binary `cmd/controller` runs controller-runtime manager + HTTP server in the same process. Python Agent Pod is spawned as a Kubernetes Job, reads SKILL.md files from a ConfigMap, calls k8s-mcp-server (embedded in the agent image) via MCP stdio, and POSTs findings back to the Controller's internal HTTP endpoint.

**Tech Stack:** Go 1.25, controller-runtime v0.20.0, k8s v0.35.3, modernc.org/sqlite (no cgo), golang-migrate, controller-gen; Python 3.12 + anthropic SDK; SKILL.md × 3.

**Spec:** [`docs/superpowers/specs/2026-04-12-kube-agent-helper-phase1-phase2-design.md`](../specs/2026-04-12-kube-agent-helper-phase1-phase2-design.md)

---

## Natural Exit Points

- **After Task 8** — Phase 0 complete: one diagnosis runs end-to-end (pod-health only)
- **After Task 13** — Phase 1 complete: all 3 skills, full REST API, Helm chart, tests

---

## File Map

```
cmd/controller/main.go                            Task 8
internal/
  store/
    store.go                                      Task 2  (interface + domain types)
    sqlite/
      sqlite.go                                   Task 2  (SQLite implementation)
      migrations/001_initial.sql                  Task 2
  controller/
    api/v1alpha1/
      groupversion.go                             Task 3
      types.go                                    Task 3
      zz_generated.deepcopy.go                    Task 3  (controller-gen output)
    reconciler/
      run_reconciler.go                           Task 4
      skill_reconciler.go                         Task 9
      modelconfig_reconciler.go                   Task 9
    translator/
      translator.go                               Task 5
      translator_test.go                          Task 5
    httpserver/
      server.go                                   Task 7
      server_test.go                              Task 7
  agent/
    runtime.go                                    Task 6
skills/
  pod-health-analyst/SKILL.md                     Task 8
  pod-security-analyst/SKILL.md                   Task 10
  pod-cost-analyst/SKILL.md                       Task 10
agent-runtime/
  Dockerfile                                      Task 6
  runtime/main.py                                 Task 6
  runtime/skill_loader.py                         Task 6
  runtime/orchestrator.py                         Task 6
  runtime/reporter.py                             Task 6
  requirements.txt                                Task 6
deploy/
  crds/                                           Task 3  (controller-gen output)
  helm/
    Chart.yaml                                    Task 12
    values.yaml                                   Task 12
    templates/deployment.yaml                     Task 12
    templates/rbac.yaml                           Task 12
    templates/crds/                               Task 12
```

**Merged from kube-agent-helper-mcp (Task 1):**
```
cmd/k8s-mcp-server/main.go
internal/k8sclient/
internal/sanitize/
internal/trimmer/
internal/audit/
internal/mcptools/
test/envtest/
test/integration/
go.mod  go.sum
```

---

## Task 1: Merge k8s-mcp-server into main repo

**Files:**
- Create: `go.mod`, `go.sum`, `cmd/k8s-mcp-server/`, `internal/k8sclient/`, `internal/sanitize/`, `internal/trimmer/`, `internal/audit/`, `internal/mcptools/`, `test/`

- [ ] **Step 1: Copy all source files from the mcp worktree**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
cp -r /Users/zhenyu.jiang/kube-agent-helper-mcp/cmd .
cp -r /Users/zhenyu.jiang/kube-agent-helper-mcp/internal .
cp -r /Users/zhenyu.jiang/kube-agent-helper-mcp/test .
cp /Users/zhenyu.jiang/kube-agent-helper-mcp/go.mod .
cp /Users/zhenyu.jiang/kube-agent-helper-mcp/go.sum .
cp /Users/zhenyu.jiang/kube-agent-helper-mcp/Makefile .
cp /Users/zhenyu.jiang/kube-agent-helper-mcp/.golangci.yml .
```

- [ ] **Step 2: Verify build and tests pass**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
go build ./...
go test ./internal/... -count=1 -timeout=60s
```

Expected: BUILD OK, all packages PASS.

- [ ] **Step 3: Commit**

```bash
git add .
git commit --no-gpg-sign -m "chore: merge k8s-mcp-server into monorepo"
```

---

## Task 2: Store interface + SQLite implementation + migrations

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/sqlite/sqlite.go`
- Create: `internal/store/sqlite/migrations/001_initial.sql`
- Create: `internal/store/sqlite/sqlite_test.go`

- [ ] **Step 1: Add dependencies**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
go get modernc.org/sqlite
go get github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/sqlite3
go get github.com/golang-migrate/migrate/v4/source/iofs
```

- [ ] **Step 2: Write `internal/store/store.go`**

```go
package store

import (
	"context"
	"time"
)

type Phase string

const (
	PhasePending   Phase = "Pending"
	PhaseRunning   Phase = "Running"
	PhaseSucceeded Phase = "Succeeded"
	PhaseFailed    Phase = "Failed"
)

type DiagnosticRun struct {
	ID          string
	TargetJSON  string
	SkillsJSON  string
	Status      Phase
	Message     string
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
}

type Finding struct {
	ID                string
	RunID             string
	Dimension         string
	Severity          string
	Title             string
	Description       string
	ResourceKind      string
	ResourceNamespace string
	ResourceName      string
	Suggestion        string
	CreatedAt         time.Time
}

type Skill struct {
	ID               string
	Name             string
	Dimension        string
	Prompt           string
	ToolsJSON        string
	RequiresDataJSON string
	Source           string // builtin | cr
	Enabled          bool
	Priority         int
	UpdatedAt        time.Time
}

type ListOpts struct {
	Limit  int
	Offset int
}

// Store is the persistence interface. Both SQLite and PostgreSQL implement it.
type Store interface {
	// Runs
	CreateRun(ctx context.Context, run *DiagnosticRun) error
	GetRun(ctx context.Context, id string) (*DiagnosticRun, error)
	UpdateRunStatus(ctx context.Context, id string, phase Phase, msg string) error
	ListRuns(ctx context.Context, opts ListOpts) ([]*DiagnosticRun, error)

	// Findings
	CreateFinding(ctx context.Context, f *Finding) error
	ListFindings(ctx context.Context, runID string) ([]*Finding, error)

	// Skills
	UpsertSkill(ctx context.Context, s *Skill) error
	ListSkills(ctx context.Context) ([]*Skill, error)
	GetSkill(ctx context.Context, name string) (*Skill, error)

	Close() error
}
```

- [ ] **Step 3: Write `internal/store/sqlite/migrations/001_initial.sql`**

```sql
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
```

- [ ] **Step 4: Write `internal/store/sqlite/sqlite.go`**

```go
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
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
	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite3", driver)
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
	return scanRun(row)
}

func (s *SQLiteStore) UpdateRunStatus(ctx context.Context, id string, phase store.Phase, msg string) error {
	now := time.Now()
	switch phase {
	case store.PhaseRunning:
		_, err := s.db.ExecContext(ctx,
			`UPDATE diagnostic_runs SET status=?, message=?, started_at=? WHERE id=?`,
			string(phase), msg, now, id)
		return err
	case store.PhaseSucceeded, store.PhaseFailed:
		_, err := s.db.ExecContext(ctx,
			`UPDATE diagnostic_runs SET status=?, message=?, completed_at=? WHERE id=?`,
			string(phase), msg, now, id)
		return err
	default:
		_, err := s.db.ExecContext(ctx,
			`UPDATE diagnostic_runs SET status=?, message=? WHERE id=?`,
			string(phase), msg, id)
		return err
	}
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
	var runs []*store.DiagnosticRun
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
	var findings []*store.Finding
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
	var skills []*store.Skill
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
		return nil, nil
	}
	return sk, err
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
```

- [ ] **Step 5: Write `internal/store/sqlite/sqlite_test.go`**

```go
package sqlite_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
	sqlitestore "github.com/kube-agent-helper/kube-agent-helper/internal/store/sqlite"
)

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.db")
	require.NoError(t, err)
	f.Close()
	s, err := sqlitestore.New(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRun_CreateAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	run := &store.DiagnosticRun{
		TargetJSON: `{"namespaces":["default"]}`,
		SkillsJSON: `["pod-health-analyst"]`,
		Status:     store.PhasePending,
	}
	require.NoError(t, s.CreateRun(ctx, run))
	assert.NotEmpty(t, run.ID)

	got, err := s.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, store.PhasePending, got.Status)
}

func TestRun_UpdateStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	run := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	require.NoError(t, s.CreateRun(ctx, run))

	require.NoError(t, s.UpdateRunStatus(ctx, run.ID, store.PhaseRunning, ""))
	got, _ := s.GetRun(ctx, run.ID)
	assert.Equal(t, store.PhaseRunning, got.Status)
	assert.NotNil(t, got.StartedAt)

	require.NoError(t, s.UpdateRunStatus(ctx, run.ID, store.PhaseSucceeded, ""))
	got, _ = s.GetRun(ctx, run.ID)
	assert.Equal(t, store.PhaseSucceeded, got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestFinding_CreateAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	run := &store.DiagnosticRun{TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending}
	require.NoError(t, s.CreateRun(ctx, run))

	f := &store.Finding{
		RunID: run.ID, Dimension: "health", Severity: "critical",
		Title: "Pod crashing", ResourceKind: "Pod",
	}
	require.NoError(t, s.CreateFinding(ctx, f))

	list, err := s.ListFindings(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "critical", list[0].Severity)
}

func TestSkill_Upsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sk := &store.Skill{Name: "pod-health-analyst", Dimension: "health",
		Prompt: "You are...", ToolsJSON: "[]", Source: "builtin", Enabled: true, Priority: 100}
	require.NoError(t, s.UpsertSkill(ctx, sk))

	sk.Priority = 50
	require.NoError(t, s.UpsertSkill(ctx, sk))

	got, err := s.GetSkill(ctx, "pod-health-analyst")
	require.NoError(t, err)
	assert.Equal(t, 50, got.Priority)
}
```

- [ ] **Step 6: Run tests**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
go test ./internal/store/... -count=1 -v
```

Expected: all PASS.

- [ ] **Step 7: Add uuid dependency and commit**

```bash
go get github.com/google/uuid
git add internal/store/ go.mod go.sum
git commit --no-gpg-sign -m "feat(store): Store interface + SQLite implementation with migrations"
```

---

## Task 3: CRD Go types + controller-gen

**Files:**
- Create: `internal/controller/api/v1alpha1/groupversion.go`
- Create: `internal/controller/api/v1alpha1/types.go`
- Create: `internal/controller/api/v1alpha1/zz_generated.deepcopy.go` (generated)
- Create: `deploy/crds/*.yaml` (generated)
- Create: `hack/boilerplate.go.txt`

- [ ] **Step 1: Install controller-gen**

```bash
go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
go get sigs.k8s.io/controller-runtime@v0.20.0
```

- [ ] **Step 2: Create `hack/boilerplate.go.txt`**

```
/*
Copyright 2026 kube-agent-helper authors.

Licensed under the Apache License, Version 2.0.
*/
```

- [ ] **Step 3: Write `internal/controller/api/v1alpha1/groupversion.go`**

```go
// Package v1alpha1 contains the CRD types for kube-agent-helper.
// +groupName=k8sai.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "k8sai.io", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)
```

- [ ] **Step 4: Write `internal/controller/api/v1alpha1/types.go`**

```go
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ── DiagnosticSkill ────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type DiagnosticSkill struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiagnosticSkillSpec `json:"spec,omitempty"`
}

type DiagnosticSkillSpec struct {
	// +kubebuilder:validation:Enum=health;security;cost;reliability
	Dimension    string   `json:"dimension"`
	Description  string   `json:"description"`
	Prompt       string   `json:"prompt"`
	Tools        []string `json:"tools"`
	RequiresData []string `json:"requiresData,omitempty"`
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`
	// +kubebuilder:default=100
	Priority int `json:"priority,omitempty"`
}

// +kubebuilder:object:root=true
type DiagnosticSkillList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiagnosticSkill `json:"items"`
}

// ── DiagnosticRun ─────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type DiagnosticRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiagnosticRunSpec   `json:"spec,omitempty"`
	Status            DiagnosticRunStatus `json:"status,omitempty"`
}

type DiagnosticRunSpec struct {
	Target         TargetSpec `json:"target"`
	Skills         []string   `json:"skills,omitempty"`
	ModelConfigRef string     `json:"modelConfigRef"`
}

type TargetSpec struct {
	// +kubebuilder:validation:Enum=namespace;cluster
	Scope         string            `json:"scope"`
	Namespaces    []string          `json:"namespaces,omitempty"`
	LabelSelector map[string]string `json:"labelSelector,omitempty"`
}

type DiagnosticRunStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed
	Phase       string       `json:"phase,omitempty"`
	StartedAt   *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	ReportID    string       `json:"reportId,omitempty"`
	Message     string       `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
type DiagnosticRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiagnosticRun `json:"items"`
}

// ── ModelConfig ───────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type ModelConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ModelConfigSpec `json:"spec,omitempty"`
}

type ModelConfigSpec struct {
	// +kubebuilder:default=anthropic
	Provider string `json:"provider"`
	// +kubebuilder:default="claude-sonnet-4-6"
	Model     string          `json:"model"`
	APIKeyRef SecretKeyRef    `json:"apiKeyRef"`
	// +kubebuilder:default=20
	MaxTurns  int             `json:"maxTurns,omitempty"`
}

type SecretKeyRef struct {
	Name string           `json:"name"`
	Key  string           `json:"key"`
}

// +kubebuilder:object:root=true
type ModelConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&DiagnosticSkill{}, &DiagnosticSkillList{},
		&DiagnosticRun{}, &DiagnosticRunList{},
		&ModelConfig{}, &ModelConfigList{},
	)
	_ = corev1.SchemeBuilder // ensure corev1 is registered
}
```

- [ ] **Step 5: Generate deepcopy and CRD YAML**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
mkdir -p deploy/crds
controller-gen object:headerFile="hack/boilerplate.go.txt" \
  paths="./internal/controller/api/..."
controller-gen crd \
  paths="./internal/controller/api/..." \
  output:crd:artifacts:config=deploy/crds
```

Expected: `deploy/crds/k8sai.io_diagnosticruns.yaml` etc. created, `zz_generated.deepcopy.go` created.

- [ ] **Step 6: Verify build**

```bash
go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add internal/controller/api/ deploy/crds/ hack/ go.mod go.sum
git commit --no-gpg-sign -m "feat(crd): DiagnosticSkill, DiagnosticRun, ModelConfig types + generated CRD YAML"
```

---

## Task 4: DiagnosticRunReconciler (minimal state machine)

**Files:**
- Create: `internal/controller/reconciler/run_reconciler.go`

- [ ] **Step 1: Write `internal/controller/reconciler/run_reconciler.go`**

```go
package reconciler

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type DiagnosticRunReconciler struct {
	client.Client
	Store      store.Store
	Translator *translator.Translator
}

func (r *DiagnosticRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var run k8saiV1.DiagnosticRun
	if err := r.Get(ctx, req.NamespacedName, &run); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Already terminal — nothing to do.
	if run.Status.Phase == "Succeeded" || run.Status.Phase == "Failed" {
		return ctrl.Result{}, nil
	}

	// Phase: Pending → Running
	if run.Status.Phase == "" || run.Status.Phase == "Pending" {
		logger.Info("translating run", "name", run.Name)

		// Persist to store
		storeRun := &store.DiagnosticRun{
			ID:         string(run.UID),
			TargetJSON: mustJSON(run.Spec.Target),
			SkillsJSON: mustJSON(run.Spec.Skills),
			Status:     store.PhasePending,
		}
		if err := r.Store.CreateRun(ctx, storeRun); err != nil {
			logger.Error(err, "store.CreateRun failed")
		}

		// Translate to Job resources
		objects, err := r.Translator.Compile(ctx, &run)
		if err != nil {
			return r.failRun(ctx, &run, fmt.Sprintf("translate failed: %s", err))
		}

		// Apply all generated objects
		for _, obj := range objects {
			obj.SetNamespace(run.Namespace)
			if err := r.Create(ctx, obj); err != nil && !errors.IsAlreadyExists(err) {
				return r.failRun(ctx, &run, fmt.Sprintf("create %T: %s", obj, err))
			}
		}

		run.Status.Phase = "Running"
		run.Status.ReportID = string(run.UID)
		if err := r.Status().Update(ctx, &run); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Store.UpdateRunStatus(ctx, string(run.UID), store.PhaseRunning, ""); err != nil {
			logger.Error(err, "store.UpdateRunStatus failed")
		}
		logger.Info("run started", "name", run.Name)
	}

	return ctrl.Result{}, nil
}

func (r *DiagnosticRunReconciler) failRun(ctx context.Context, run *k8saiV1.DiagnosticRun, msg string) (ctrl.Result, error) {
	run.Status.Phase = "Failed"
	run.Status.Message = msg
	_ = r.Status().Update(ctx, run)
	_ = r.Store.UpdateRunStatus(ctx, string(run.UID), store.PhaseFailed, msg)
	return ctrl.Result{}, nil
}

func (r *DiagnosticRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.DiagnosticRun{}).
		Complete(r)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
```

Add `"encoding/json"` to the import block. The full import block for `run_reconciler.go`:

```go
import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
```

Add `"encoding/json"` to the import block.

- [ ] **Step 2: Verify build**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/controller/reconciler/
git commit --no-gpg-sign -m "feat(reconciler): DiagnosticRunReconciler Pending→Running state machine"
```

---

## Task 5: Translator — compile DiagnosticRun → Job + ConfigMap + SA + RoleBinding

**Files:**
- Create: `internal/controller/translator/translator.go`
- Create: `internal/controller/translator/translator_test.go`

- [ ] **Step 1: Write failing test first**

```go
// internal/controller/translator/translator_test.go
package translator_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func TestTranslator_Compile_ProducesExpectedObjects(t *testing.T) {
	skills := []*store.Skill{{
		Name:      "pod-health-analyst",
		Dimension: "health",
		Prompt:    "You are a health analyst.",
		ToolsJSON: `["kubectl_get","events_list"]`,
	}}
	tr := translator.New(translator.Config{
		AgentImage:    "ghcr.io/kube-agent-helper/agent-runtime:latest",
		ControllerURL: "http://controller.svc:8080",
	}, skills)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: "default", UID: "uid-123"},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health-analyst"},
			ModelConfigRef: "claude-default",
		},
	}

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)

	var job *batchv1.Job
	var cm *corev1.ConfigMap
	var sa *corev1.ServiceAccount
	var rb *rbacv1.RoleBinding

	for _, o := range objects {
		switch v := o.(type) {
		case *batchv1.Job:
			job = v
		case *corev1.ConfigMap:
			cm = v
		case *corev1.ServiceAccount:
			sa = v
		case *rbacv1.RoleBinding:
			rb = v
		}
	}

	require.NotNil(t, job, "expected Job")
	require.NotNil(t, cm, "expected ConfigMap")
	require.NotNil(t, sa, "expected ServiceAccount")
	require.NotNil(t, rb, "expected RoleBinding")

	assert.Contains(t, cm.Data, "pod-health-analyst.md")
	assert.Equal(t, sa.Name, rb.Subjects[0].Name)
	assert.Equal(t, "uid-123", job.Labels["run-id"])
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/controller/translator/... -count=1 -v
```

Expected: FAIL — package not found.

- [ ] **Step 3: Write `internal/controller/translator/translator.go`**

```go
package translator

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type Config struct {
	AgentImage    string
	ControllerURL string
}

type Translator struct {
	cfg    Config
	skills []*store.Skill
}

func New(cfg Config, skills []*store.Skill) *Translator {
	return &Translator{cfg: cfg, skills: skills}
}

// Compile produces all Kubernetes objects needed for one DiagnosticRun.
func (t *Translator) Compile(_ context.Context, run *k8saiV1.DiagnosticRun) ([]client.Object, error) {
	runID := string(run.UID)
	if runID == "" {
		runID = run.Name
	}

	// Select skills for this run
	selected := t.selectSkills(run.Spec.Skills)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no enabled skills found for run %s", run.Name)
	}

	saName := fmt.Sprintf("run-%s", run.Name)
	cmName := fmt.Sprintf("skill-bundle-%s", run.Name)
	namespaces := run.Spec.Target.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{run.Namespace}
	}

	sa := t.buildSA(saName, runID)
	cm := t.buildConfigMap(cmName, runID, selected)
	rb := t.buildRoleBinding(saName, runID, namespaces)
	job := t.buildJob(run, runID, saName, cmName, selected)

	return []client.Object{sa, cm, rb, job}, nil
}

func (t *Translator) selectSkills(names []string) []*store.Skill {
	if len(names) == 0 {
		var all []*store.Skill
		for _, s := range t.skills {
			if s.Enabled {
				all = append(all, s)
			}
		}
		return all
	}
	byName := make(map[string]*store.Skill, len(t.skills))
	for _, s := range t.skills {
		byName[s.Name] = s
	}
	var selected []*store.Skill
	for _, n := range names {
		if s, ok := byName[n]; ok && s.Enabled {
			selected = append(selected, s)
		}
	}
	return selected
}

func (t *Translator) buildSA(name, runID string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"run-id": runID},
		},
	}
}

func (t *Translator) buildConfigMap(name, runID string, skills []*store.Skill) *corev1.ConfigMap {
	data := make(map[string]string, len(skills))
	for _, s := range skills {
		key := s.Name + ".md"
		data[key] = fmt.Sprintf("---\nname: %s\ndimension: %s\ntools: %s\n---\n\n%s\n",
			s.Name, s.Dimension, s.ToolsJSON, s.Prompt)
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"run-id": runID},
		},
		Data: data,
	}
}

func (t *Translator) buildRoleBinding(saName, runID string, namespaces []string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   saName,
			Labels: map[string]string{"run-id": runID},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "view", // built-in read-only role
		},
		Subjects: []rbacv1.Subject{{
			Kind: "ServiceAccount",
			Name: saName,
		}},
	}
}

func (t *Translator) buildJob(run *k8saiV1.DiagnosticRun, runID, saName, cmName string, skills []*store.Skill) *batchv1.Job {
	ttl := int32(3600)
	backoff := int32(0)

	skillNames := make([]string, len(skills))
	for i, s := range skills {
		skillNames[i] = s.Name
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("agent-%s", run.Name),
			Labels: map[string]string{"run-id": runID},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			BackoffLimit:            &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: saName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{{
						Name: "skills",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
							},
						},
					}},
					Containers: []corev1.Container{{
						Name:  "agent",
						Image: t.cfg.AgentImage,
						Command: []string{"python", "-m", "runtime.main"},
						Env: []corev1.EnvVar{
							{Name: "RUN_ID", Value: runID},
							{Name: "TARGET_NAMESPACES", Value: joinStr(run.Spec.Target.Namespaces)},
							{Name: "CONTROLLER_URL", Value: t.cfg.ControllerURL},
							{Name: "MCP_SERVER_PATH", Value: "/usr/local/bin/k8s-mcp-server"},
							{Name: "SKILL_NAMES", Value: joinStr(skillNames)},
							{
								Name: "ANTHROPIC_API_KEY",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: run.Spec.ModelConfigRef,
										},
										Key: "apiKey",
									},
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "skills",
							MountPath: "/workspace/skills",
						}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					}},
				},
			},
		},
	}
}

func joinStr(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ","
		}
		result += s
	}
	return result
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/controller/translator/... -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/translator/
git commit --no-gpg-sign -m "feat(translator): compile DiagnosticRun → Job + ConfigMap + SA + RoleBinding"
```

---

## Task 6: Python Agent Runtime

**Files:**
- Create: `agent-runtime/Dockerfile`
- Create: `agent-runtime/requirements.txt`
- Create: `agent-runtime/runtime/__init__.py`
- Create: `agent-runtime/runtime/main.py`
- Create: `agent-runtime/runtime/skill_loader.py`
- Create: `agent-runtime/runtime/orchestrator.py`
- Create: `agent-runtime/runtime/reporter.py`

- [ ] **Step 1: Write `agent-runtime/requirements.txt`**

```
anthropic>=0.40.0
requests>=2.32.0
pyyaml>=6.0.0
```

- [ ] **Step 2: Write `agent-runtime/runtime/skill_loader.py`**

```python
"""Loads SKILL.md files from /workspace/skills/"""
import os
import re
from dataclasses import dataclass, field
from typing import List

SKILLS_DIR = os.environ.get("SKILLS_DIR", "/workspace/skills")


@dataclass
class Skill:
    name: str
    dimension: str
    tools: List[str]
    prompt: str
    requires_data: List[str] = field(default_factory=list)


def load_skills(skill_names: List[str]) -> List[Skill]:
    """Load only the requested skills from /workspace/skills/<name>.md"""
    skills = []
    for name in skill_names:
        path = os.path.join(SKILLS_DIR, f"{name}.md")
        if not os.path.exists(path):
            print(f"[warn] skill file not found: {path}")
            continue
        skill = _parse_skill_md(path)
        if skill:
            skills.append(skill)
    return skills


def _parse_skill_md(path: str) -> Skill | None:
    with open(path) as f:
        content = f.read()

    # Extract YAML frontmatter between --- markers
    match = re.match(r"^---\n(.*?)\n---\n(.*)", content, re.DOTALL)
    if not match:
        return None

    import yaml
    meta = yaml.safe_load(match.group(1))
    prompt_body = match.group(2).strip()

    import json
    tools_raw = meta.get("tools", "[]")
    if isinstance(tools_raw, str):
        tools = json.loads(tools_raw)
    else:
        tools = tools_raw

    return Skill(
        name=meta["name"],
        dimension=meta.get("dimension", "health"),
        tools=tools,
        prompt=prompt_body,
    )
```

- [ ] **Step 3: Write `agent-runtime/runtime/orchestrator.py`**

```python
"""Builds the orchestrator prompt and runs the agentic loop."""
import json
import os
import subprocess
from typing import List

import anthropic

from .skill_loader import Skill


MCP_SERVER_PATH = os.environ.get("MCP_SERVER_PATH", "/usr/local/bin/k8s-mcp-server")
TARGET_NAMESPACES = os.environ.get("TARGET_NAMESPACES", "default")


def build_prompt(skills: List[Skill]) -> str:
    skill_list = "\n".join(
        f"- **{s.name}** ({s.dimension}): {s.prompt[:200]}..."
        for s in skills
    )
    return f"""You are a Kubernetes diagnostic orchestrator.

Target namespaces: {TARGET_NAMESPACES}

Available diagnostic skills:
{skill_list}

Instructions:
1. For each skill, analyze the cluster in the target namespaces.
2. Use the available MCP tools to gather data.
3. For each issue found, output a finding JSON object on its own line:
   {{"dimension":"<dim>","severity":"<critical|high|medium|low|info>","title":"<title>","description":"<desc>","resource_kind":"<kind>","resource_namespace":"<ns>","resource_name":"<name>","suggestion":"<suggestion>"}}
4. After all skills complete, output: FINDINGS_COMPLETE
"""


def run_agent(skills: List[Skill]) -> List[dict]:
    """Run the agentic loop and return a list of findings."""
    client = anthropic.Anthropic()

    # Build MCP tool definitions by querying k8s-mcp-server
    tools = _discover_tools()

    prompt = build_prompt(skills)
    messages = [{"role": "user", "content": prompt}]

    findings = []
    max_turns = int(os.environ.get("MAX_TURNS", "20"))

    for _ in range(max_turns):
        response = client.messages.create(
            model=os.environ.get("MODEL", "claude-sonnet-4-6"),
            max_tokens=4096,
            tools=tools,
            messages=messages,
        )

        messages.append({"role": "assistant", "content": response.content})

        # Extract text blocks for finding detection
        for block in response.content:
            if hasattr(block, "text"):
                for line in block.text.split("\n"):
                    line = line.strip()
                    if line.startswith("{") and "dimension" in line:
                        try:
                            f = json.loads(line)
                            findings.append(f)
                        except json.JSONDecodeError:
                            pass

        if response.stop_reason == "end_turn":
            break

        if response.stop_reason == "tool_use":
            tool_results = []
            for block in response.content:
                if block.type == "tool_use":
                    result = _call_mcp_tool(block.name, block.input)
                    tool_results.append({
                        "type": "tool_result",
                        "tool_use_id": block.id,
                        "content": result,
                    })
            messages.append({"role": "user", "content": tool_results})

    return findings


def _discover_tools() -> list:
    """Query k8s-mcp-server for available tools via MCP initialize."""
    try:
        proc = subprocess.run(
            [MCP_SERVER_PATH, "--in-cluster"],
            input=json.dumps({"jsonrpc":"2.0","id":1,"method":"initialize",
                              "params":{"protocolVersion":"2024-11-05",
                                        "clientInfo":{"name":"agent","version":"0.1"},
                                        "capabilities":{}}}) + "\n" +
                  json.dumps({"jsonrpc":"2.0","method":"notifications/initialized"}) + "\n" +
                  json.dumps({"jsonrpc":"2.0","id":2,"method":"tools/list"}) + "\n",
            capture_output=True, text=True, timeout=10,
        )
        lines = [l for l in proc.stdout.split("\n") if l.strip()]
        for line in lines:
            parsed = json.loads(line)
            if parsed.get("id") == 2 and "result" in parsed:
                mcp_tools = parsed["result"].get("tools", [])
                return [_mcp_to_anthropic_tool(t) for t in mcp_tools]
    except Exception as e:
        print(f"[warn] tool discovery failed: {e}")
    return []


def _mcp_to_anthropic_tool(t: dict) -> dict:
    return {
        "name": t["name"],
        "description": t.get("description", ""),
        "input_schema": t.get("inputSchema", {"type": "object", "properties": {}}),
    }


def _call_mcp_tool(name: str, args: dict) -> str:
    """Call a tool on k8s-mcp-server via MCP stdio protocol."""
    request = json.dumps({
        "jsonrpc": "2.0", "id": 1, "method": "tools/call",
        "params": {"name": name, "arguments": args},
    })
    try:
        proc = subprocess.run(
            [MCP_SERVER_PATH, "--in-cluster"],
            input=json.dumps({"jsonrpc":"2.0","id":0,"method":"initialize",
                              "params":{"protocolVersion":"2024-11-05",
                                        "clientInfo":{"name":"agent","version":"0.1"},
                                        "capabilities":{}}}) + "\n" +
                  json.dumps({"jsonrpc":"2.0","method":"notifications/initialized"}) + "\n" +
                  request + "\n",
            capture_output=True, text=True, timeout=30,
        )
        for line in proc.stdout.split("\n"):
            if not line.strip():
                continue
            parsed = json.loads(line)
            if parsed.get("id") == 1 and "result" in parsed:
                content = parsed["result"].get("content", [])
                if content:
                    return content[0].get("text", "")
    except Exception as e:
        return f"tool error: {e}"
    return ""
```

- [ ] **Step 4: Write `agent-runtime/runtime/reporter.py`**

```python
"""POSTs findings back to the Controller."""
import json
import os

import requests

CONTROLLER_URL = os.environ.get("CONTROLLER_URL", "http://controller.kube-agent-helper.svc:8080")


def post_findings(run_id: str, findings: list[dict]) -> None:
    url = f"{CONTROLLER_URL}/internal/runs/{run_id}/findings"
    for f in findings:
        try:
            resp = requests.post(url, json=f, timeout=10)
            resp.raise_for_status()
        except Exception as e:
            print(f"[warn] failed to post finding: {e}")
    print(f"[info] posted {len(findings)} findings for run {run_id}")
```

- [ ] **Step 5: Write `agent-runtime/runtime/__init__.py`**

```python
```

(empty file)

- [ ] **Step 6: Write `agent-runtime/runtime/main.py`**

```python
"""Entry point for the Agent Pod."""
import os
import sys

from .orchestrator import run_agent
from .reporter import post_findings
from .skill_loader import load_skills


def main() -> None:
    run_id = os.environ["RUN_ID"]
    skill_names_raw = os.environ.get("SKILL_NAMES", "")
    skill_names = [s.strip() for s in skill_names_raw.split(",") if s.strip()]

    print(f"[info] run_id={run_id} skills={skill_names}")

    skills = load_skills(skill_names)
    if not skills:
        print("[error] no skills loaded — exiting")
        sys.exit(1)

    findings = run_agent(skills)
    print(f"[info] found {len(findings)} findings")

    post_findings(run_id, findings)
    print("[info] done")


if __name__ == "__main__":
    main()
```

- [ ] **Step 7: Write `agent-runtime/Dockerfile`**

```dockerfile
# Stage 1: build k8s-mcp-server binary
FROM golang:1.25-bookworm AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/k8s-mcp-server ./cmd/k8s-mcp-server
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/k8s-mcp-server ./cmd/k8s-mcp-server

# Stage 2: Python runtime
FROM python:3.12-slim
COPY --from=go-builder /usr/local/bin/k8s-mcp-server /usr/local/bin/k8s-mcp-server
RUN chmod +x /usr/local/bin/k8s-mcp-server

WORKDIR /app
COPY agent-runtime/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY agent-runtime/runtime ./runtime

CMD ["python", "-m", "runtime.main"]
```

- [ ] **Step 8: Write `internal/agent/runtime.go`**

```go
package agent

import (
	batchv1 "k8s.io/api/batch/v1"
	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// AgentRuntime generates the Job manifest for an Agent Pod.
type AgentRuntime interface {
	BuildJobSpec(run *k8saiV1.DiagnosticRun, skills []*store.Skill, model *k8saiV1.ModelConfig) (*batchv1.Job, error)
}
```

- [ ] **Step 9: Commit**

```bash
git add agent-runtime/ internal/agent/
git commit --no-gpg-sign -m "feat(agent): Python Agent runtime + Dockerfile with embedded k8s-mcp-server"
```

---

## Task 7: Internal HTTP Server — findings write-back endpoint

**Files:**
- Create: `internal/controller/httpserver/server.go`
- Create: `internal/controller/httpserver/server_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/controller/httpserver/server_test.go
package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type fakeStore struct {
	findings []*store.Finding
}

func (f *fakeStore) CreateFinding(_ context.Context, finding *store.Finding) error {
	f.findings = append(f.findings, finding)
	return nil
}
func (f *fakeStore) ListFindings(_ context.Context, runID string) ([]*store.Finding, error) {
	var out []*store.Finding
	for _, fi := range f.findings {
		if fi.RunID == runID {
			out = append(out, fi)
		}
	}
	return out, nil
}
func (f *fakeStore) CreateRun(_ context.Context, r *store.DiagnosticRun) error  { return nil }
func (f *fakeStore) GetRun(_ context.Context, id string) (*store.DiagnosticRun, error) { return nil, nil }
func (f *fakeStore) UpdateRunStatus(_ context.Context, id string, p store.Phase, msg string) error { return nil }
func (f *fakeStore) ListRuns(_ context.Context, opts store.ListOpts) ([]*store.DiagnosticRun, error) { return nil, nil }
func (f *fakeStore) UpsertSkill(_ context.Context, s *store.Skill) error       { return nil }
func (f *fakeStore) ListSkills(_ context.Context) ([]*store.Skill, error)       { return nil, nil }
func (f *fakeStore) GetSkill(_ context.Context, name string) (*store.Skill, error) { return nil, nil }
func (f *fakeStore) Close() error                                               { return nil }

func TestPostFindings(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs)

	body, _ := json.Marshal(map[string]interface{}{
		"dimension": "health", "severity": "critical",
		"title": "Pod crash", "resource_kind": "Pod",
		"resource_namespace": "default", "resource_name": "api-pod",
	})

	req := httptest.NewRequest(http.MethodPost, "/internal/runs/run-123/findings",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)
	require.Len(t, fs.findings, 1)
	assert.Equal(t, "run-123", fs.findings[0].RunID)
}

func TestGetFindings(t *testing.T) {
	fs := &fakeStore{}
	_ = fs.CreateFinding(context.Background(), &store.Finding{
		RunID: "run-abc", Dimension: "security", Severity: "high", Title: "Root container",
	})
	srv := httpserver.New(fs)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-abc/findings", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp []map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Len(t, resp, 1)
}
```

- [ ] **Step 2: Run test to confirm failure**

```bash
go test ./internal/controller/httpserver/... -count=1 -v
```

Expected: FAIL.

- [ ] **Step 3: Write `internal/controller/httpserver/server.go`**

```go
package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type Server struct {
	store store.Store
	mux   *http.ServeMux
}

func New(s store.Store) *Server {
	srv := &Server{store: s, mux: http.NewServeMux()}
	srv.mux.HandleFunc("/internal/runs/", srv.handleInternal)
	srv.mux.HandleFunc("/api/runs", srv.handleAPIRuns)
	srv.mux.HandleFunc("/api/runs/", srv.handleAPIRunDetail)
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	return srv.ListenAndServe()
}

// POST /internal/runs/{id}/findings
func (s *Server) handleInternal(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["internal","runs","{id}","findings"]
	if len(parts) != 4 || parts[3] != "findings" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	runID := parts[2]

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	f := &store.Finding{
		RunID:             runID,
		Dimension:         strVal(payload, "dimension"),
		Severity:          strVal(payload, "severity"),
		Title:             strVal(payload, "title"),
		Description:       strVal(payload, "description"),
		ResourceKind:      strVal(payload, "resource_kind"),
		ResourceNamespace: strVal(payload, "resource_namespace"),
		ResourceName:      strVal(payload, "resource_name"),
		Suggestion:        strVal(payload, "suggestion"),
	}
	if err := s.store.CreateFinding(r.Context(), f); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// GET /api/runs
func (s *Server) handleAPIRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runs, err := s.store.ListRuns(r.Context(), store.ListOpts{Limit: 50})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, runs)
}

// GET /api/runs/{id}  and  GET /api/runs/{id}/findings
func (s *Server) handleAPIRunDetail(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["api","runs","{id}"] or ["api","runs","{id}","findings"]
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	runID := parts[2]

	if len(parts) == 4 && parts[3] == "findings" {
		findings, err := s.store.ListFindings(r.Context(), runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, findings)
		return
	}

	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if run == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, run)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/controller/httpserver/... -count=1 -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/httpserver/
git commit --no-gpg-sign -m "feat(httpserver): internal findings endpoint + /api/runs REST API"
```

---

## Task 8: Wire controller binary + pod-health-analyst + Phase 0 smoke test

**Files:**
- Create: `cmd/controller/main.go`
- Create: `skills/pod-health-analyst.md`

- [ ] **Step 1: Write `skills/pod-health-analyst.md`**

```markdown
---
name: pod-health-analyst
dimension: health
tools: ["kubectl_get","kubectl_describe","kubectl_logs","events_list"]
requires_data: ["pods","events"]
---

You are a Kubernetes pod health specialist. Analyze all pods in the target namespaces.

## Instructions

1. List all pods using `kubectl_get` with kind=Pod for each target namespace.
2. For each pod that is NOT in Running or Succeeded state:
   - Use `kubectl_describe` to get details.
   - Use `events_list` to get related events.
   - If CrashLoopBackOff or OOMKilled, use `kubectl_logs` (previous=true) to get last crash logs.
3. Check for pods with high restart counts (>5 restarts).
4. For each issue found, output one finding JSON per line:
   {"dimension":"health","severity":"<critical|high|medium|low>","title":"<short title>","description":"<what you observed>","resource_kind":"Pod","resource_namespace":"<ns>","resource_name":"<pod-name>","suggestion":"<actionable fix>"}

## Severity Guide
- critical: Pod won't start or is OOMKilled repeatedly
- high: CrashLoopBackOff or probe failures preventing traffic
- medium: High restart count but currently running
- low: Completed/evicted pods leaving stale entries
```

- [ ] **Step 2: Write `cmd/controller/main.go`**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	sqlitestore "github.com/kube-agent-helper/kube-agent-helper/internal/store/sqlite"
)

var (
	dbPath        string
	httpAddr      string
	agentImage    string
	controllerURL string
	skillsDir     string
)

func main() {
	flag.StringVar(&dbPath, "db", "/data/kube-agent-helper.db", "SQLite database path")
	flag.StringVar(&httpAddr, "http-addr", ":8080", "HTTP server listen address")
	flag.StringVar(&agentImage, "agent-image", "ghcr.io/kube-agent-helper/agent-runtime:latest", "Agent Pod image")
	flag.StringVar(&controllerURL, "controller-url", "http://controller.kube-agent-helper.svc:8080", "Controller URL for Agent callbacks")
	flag.StringVar(&skillsDir, "skills-dir", "/skills", "Directory containing built-in SKILL.md files")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = k8saiV1.AddToScheme(scheme)

	// Open DB
	st, err := sqlitestore.New(dbPath)
	if err != nil {
		slog.Error("open db", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	// Load built-in skills from skillsDir into DB
	if err := loadBuiltinSkills(context.Background(), st, skillsDir); err != nil {
		slog.Warn("load builtin skills", "error", err)
	}

	// Manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{Scheme: scheme})
	if err != nil {
		slog.Error("new manager", "error", err)
		os.Exit(1)
	}

	// Load skills for translator
	skills, _ := st.ListSkills(context.Background())

	tr := translator.New(translator.Config{
		AgentImage:    agentImage,
		ControllerURL: controllerURL,
	}, skills)

	if err := (&reconciler.DiagnosticRunReconciler{
		Client:     mgr.GetClient(),
		Store:      st,
		Translator: tr,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("setup reconciler", "error", err)
		os.Exit(1)
	}

	// HTTP server as manager Runnable
	httpSrv := httpserver.New(st)
	if err := mgr.Add(&runnableHTTP{srv: httpSrv, addr: httpAddr}); err != nil {
		slog.Error("add http server", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	slog.Info("controller starting", "http", httpAddr)
	if err := mgr.Start(ctx); err != nil {
		slog.Error("manager stopped", "error", err)
		os.Exit(1)
	}
}

type runnableHTTP struct {
	srv  *httpserver.Server
	addr string
}

func (r *runnableHTTP) Start(ctx context.Context) error {
	return r.srv.Start(ctx, r.addr)
}

func loadBuiltinSkills(ctx context.Context, st store.Store, _ string) error {
	return st.UpsertSkill(ctx, &store.Skill{
		Name:      "pod-health-analyst",
		Dimension: "health",
		Prompt:    "You are a Kubernetes pod health specialist. See SKILL.md for full prompt.",
		ToolsJSON: `["kubectl_get","kubectl_describe","kubectl_logs","events_list"]`,
		Source:    "builtin",
		Enabled:   true,
		Priority:  100,
	})
}
```

This stub is replaced with the full SKILL.md file scanner in Task 9.

The `loadBuiltinSkills` Task 8 version (same as above):

```go
import (
    "context"
    "github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func loadBuiltinSkills(ctx context.Context, st store.Store, _ string) error {
	return st.UpsertSkill(ctx, &store.Skill{
		Name:      "pod-health-analyst",
		Dimension: "health",
		Prompt:    "You are a Kubernetes pod health specialist. See SKILL.md for full prompt.",
		ToolsJSON: `["kubectl_get","kubectl_describe","kubectl_logs","events_list"]`,
		Source:    "builtin",
		Enabled:   true,
		Priority:  100,
	})
}
```

- [ ] **Step 3: Verify full build**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
go build ./...
```

Expected: BUILD OK (both `cmd/k8s-mcp-server` and `cmd/controller`).

- [ ] **Step 4: Run all unit tests**

```bash
go test ./internal/... -count=1 -timeout=60s
```

Expected: all PASS.

- [ ] **Step 5: Commit Phase 0 complete**

```bash
git add cmd/controller/ skills/ go.mod go.sum
git commit --no-gpg-sign -m "feat: Phase 0 complete — controller binary + pod-health-analyst skill wired"
```

---

## Task 9: DiagnosticSkillReconciler + ModelConfigReconciler + builtin skill loader

**Files:**
- Create: `internal/controller/reconciler/skill_reconciler.go`
- Create: `internal/controller/reconciler/modelconfig_reconciler.go`
- Modify: `cmd/controller/main.go` (register new reconcilers, replace stub loadBuiltinSkills)

- [ ] **Step 1: Write `internal/controller/reconciler/skill_reconciler.go`**

```go
package reconciler

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type DiagnosticSkillReconciler struct {
	client.Client
	Store store.Store
}

func (r *DiagnosticSkillReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var skill k8saiV1.DiagnosticSkill
	if err := r.Get(ctx, req.NamespacedName, &skill); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	toolsJSON, _ := json.Marshal(skill.Spec.Tools)
	requiresJSON, _ := json.Marshal(skill.Spec.RequiresData)

	s := &store.Skill{
		Name:             skill.Name,
		Dimension:        skill.Spec.Dimension,
		Prompt:           skill.Spec.Prompt,
		ToolsJSON:        string(toolsJSON),
		RequiresDataJSON: string(requiresJSON),
		Source:           "cr",
		Enabled:          skill.Spec.Enabled,
		Priority:         skill.Spec.Priority,
	}
	if err := r.Store.UpsertSkill(ctx, s); err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("synced skill", "name", skill.Name)
	return ctrl.Result{}, nil
}

func (r *DiagnosticSkillReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.DiagnosticSkill{}).
		Complete(r)
}
```

Fix: replace the pseudo-code with real import. Add `"encoding/json"` to imports and use `json.Marshal` directly.

- [ ] **Step 2: Write `internal/controller/reconciler/modelconfig_reconciler.go`**

```go
package reconciler

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
)

// ModelConfigReconciler validates ModelConfig resources exist and their Secret refs are valid.
type ModelConfigReconciler struct {
	client.Client
}

func (r *ModelConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var mc k8saiV1.ModelConfig
	if err := r.Get(ctx, req.NamespacedName, &mc); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Validate the referenced Secret exists
	var secret corev1.Secret
	secretRef := client.ObjectKey{
		Namespace: mc.Namespace,
		Name:      mc.Spec.APIKeyRef.Name,
	}
	if err := r.Get(ctx, secretRef, &secret); err != nil {
		logger.Error(err, "apiKeyRef secret not found", "secret", secretRef.Name)
		// Not a hard failure — the run will fail when it tries to use the key
	} else {
		logger.Info("ModelConfig validated", "name", mc.Name, "model", mc.Spec.Model)
	}

	return ctrl.Result{}, nil
}

func (r *ModelConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.ModelConfig{}).
		Complete(r)
}
```

Add `corev1 "k8s.io/api/core/v1"` to imports.

- [ ] **Step 3: Replace stub loadBuiltinSkills in `cmd/controller/main.go`**

Replace the hardcoded stub with a real SKILL.md file scanner:

```go
import (
    "os"
    "path/filepath"
    "strings"
    // ... existing imports
)

func loadBuiltinSkills(ctx context.Context, st store.Store, dir string) error {
    entries, err := os.ReadDir(dir)
    if err != nil {
        return err
    }
    for _, e := range entries {
        if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
            continue
        }
        data, err := os.ReadFile(filepath.Join(dir, e.Name()))
        if err != nil {
            continue
        }
        sk := parseSkillMD(string(data))
        if sk == nil {
            continue
        }
        if err := st.UpsertSkill(ctx, sk); err != nil {
            return err
        }
    }
    return nil
}

func parseSkillMD(content string) *store.Skill {
    // Extract frontmatter between --- markers
    parts := strings.SplitN(content, "---", 3)
    if len(parts) < 3 {
        return nil
    }
    // Simple key: value parsing
    meta := map[string]string{}
    for _, line := range strings.Split(parts[1], "\n") {
        kv := strings.SplitN(strings.TrimSpace(line), ":", 2)
        if len(kv) == 2 {
            meta[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
        }
    }
    name := meta["name"]
    if name == "" {
        return nil
    }
    toolsJSON := meta["tools"]
    if toolsJSON == "" {
        toolsJSON = "[]"
    }
    return &store.Skill{
        Name:      name,
        Dimension: meta["dimension"],
        Prompt:    strings.TrimSpace(parts[2]),
        ToolsJSON: toolsJSON,
        Source:    "builtin",
        Enabled:   true,
        Priority:  100,
    }
}
```

Also register the two new reconcilers in `main()`:

```go
if err := (&reconciler.DiagnosticSkillReconciler{
    Client: mgr.GetClient(),
    Store:  st,
}).SetupWithManager(mgr); err != nil {
    slog.Error("setup skill reconciler", "error", err)
    os.Exit(1)
}

if err := (&reconciler.ModelConfigReconciler{
    Client: mgr.GetClient(),
}).SetupWithManager(mgr); err != nil {
    slog.Error("setup modelconfig reconciler", "error", err)
    os.Exit(1)
}
```

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test ./internal/... -count=1 -timeout=60s
```

- [ ] **Step 5: Commit**

```bash
git add internal/controller/reconciler/skill_reconciler.go \
         internal/controller/reconciler/modelconfig_reconciler.go \
         cmd/controller/main.go
git commit --no-gpg-sign -m "feat(reconciler): DiagnosticSkillReconciler + ModelConfigReconciler + real builtin skill loader"
```

---

## Task 10: pod-security-analyst + pod-cost-analyst SKILL.md

**Files:**
- Create: `skills/pod-security-analyst.md`
- Create: `skills/pod-cost-analyst.md`

- [ ] **Step 1: Write `skills/pod-security-analyst.md`**

```markdown
---
name: pod-security-analyst
dimension: security
tools: ["kubectl_get","kubectl_describe"]
requires_data: ["pods","serviceaccounts"]
---

You are a Kubernetes pod security specialist. Analyze all pods in the target namespaces for security misconfigurations.

## Instructions

1. List all pods using `kubectl_get` with kind=Pod for each target namespace.
2. For each pod, check its security context:
   - Use `kubectl_describe` to get the full pod spec.
3. Check for these security issues:
   - **root container**: `securityContext.runAsNonRoot` is false or missing, `securityContext.runAsUser` is 0
   - **privileged**: `securityContext.privileged: true`
   - **host access**: `hostPID: true`, `hostNetwork: true`, or `hostIPC: true`
   - **no resource limits**: any container missing `resources.limits`
   - **SA token auto-mount**: `automountServiceAccountToken` not set to false on non-API-accessing pods
4. For each issue found, output one finding JSON per line:
   {"dimension":"security","severity":"<critical|high|medium|low>","title":"<title>","description":"<detail>","resource_kind":"Pod","resource_namespace":"<ns>","resource_name":"<name>","suggestion":"<fix>"}

## Severity Guide
- critical: privileged container or host namespace access
- high: running as root
- medium: missing resource limits or SA token auto-mount
- low: missing read-only root filesystem
```

- [ ] **Step 2: Write `skills/pod-cost-analyst.md`**

```markdown
---
name: pod-cost-analyst
dimension: cost
tools: ["kubectl_get","top_pods","top_nodes","prometheus_query"]
requires_data: ["pods","nodes","metrics"]
---

You are a Kubernetes cost optimization specialist. Identify resource waste in the target namespaces.

## Instructions

1. List all pods using `kubectl_get` with kind=Pod.
2. Get actual CPU/memory usage using `top_pods` for each namespace.
3. Get node usage using `top_nodes`.
4. Compare requests vs actual usage:
   - If actual CPU < 20% of request for >3 pods: report over-provisioning
   - If actual memory < 30% of request for >3 pods: report memory over-provisioning
5. Find zombie resources:
   - Deployments with 0 replicas: use `kubectl_get` with kind=Deployment
6. Identify underutilized nodes (usage < 20% CPU):
   - Check top_nodes output
7. For each issue found, output one finding JSON per line:
   {"dimension":"cost","severity":"<high|medium|low>","title":"<title>","description":"<detail>","resource_kind":"<kind>","resource_namespace":"<ns>","resource_name":"<name>","suggestion":"<fix>"}

## Severity Guide
- high: >50% resource waste across multiple pods or zombie Deployment consuming quota
- medium: 20-50% over-provisioning or underutilized node
- low: minor over-provisioning, single pod
```

- [ ] **Step 3: Commit**

```bash
git add skills/
git commit --no-gpg-sign -m "feat(skills): pod-security-analyst + pod-cost-analyst SKILL.md"
```

---

## Task 11: REST API — /api/runs POST + /api/skills

**Files:**
- Modify: `internal/controller/httpserver/server.go`
- Modify: `internal/controller/httpserver/server_test.go`

- [ ] **Step 1: Add POST /api/runs and GET /api/skills to server.go**

In `New()`, add two more routes:
```go
srv.mux.HandleFunc("/api/skills", srv.handleAPISkills)
```

Add handler:
```go
// GET /api/skills
func (s *Server) handleAPISkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	skills, err := s.store.ListSkills(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, skills)
}
```

Add POST /api/runs handler (creates a DiagnosticRun entry in the store directly, without K8s; the reconciler is the authoritative source for K8s-triggered runs):
```go
// POST /api/runs — creates a store record (K8s CR creation is the authoritative trigger)
func (s *Server) handleAPIRunsPost(w http.ResponseWriter, r *http.Request) {
	// This is a lightweight record-only endpoint for Phase 1.
	// Full CR creation via k8s client comes in Phase 2.
	http.Error(w, "use kubectl apply to create DiagnosticRun CR", http.StatusNotImplemented)
}
```

Update `handleAPIRuns` to route by method:
```go
func (s *Server) handleAPIRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		runs, err := s.store.ListRuns(r.Context(), store.ListOpts{Limit: 50})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, runs)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
```

- [ ] **Step 2: Add test for /api/skills**

```go
func TestGetSkills(t *testing.T) {
	fs := &fakeStore{}
	_ = fs.UpsertSkill(context.Background(), &store.Skill{
		Name: "pod-health-analyst", Dimension: "health",
		Prompt: "You are...", Enabled: true,
	})
	srv := httpserver.New(fs)

	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/controller/httpserver/... -count=1 -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/controller/httpserver/
git commit --no-gpg-sign -m "feat(httpserver): GET /api/skills + method routing for /api/runs"
```

---

## Task 12: Helm chart

**Files:**
- Create: `deploy/helm/Chart.yaml`
- Create: `deploy/helm/values.yaml`
- Create: `deploy/helm/templates/deployment.yaml`
- Create: `deploy/helm/templates/serviceaccount.yaml`
- Create: `deploy/helm/templates/rbac.yaml`
- Create: `deploy/helm/templates/service.yaml`
- Create: `deploy/helm/templates/crds/` (copy from deploy/crds/)

- [ ] **Step 1: Write `deploy/helm/Chart.yaml`**

```yaml
apiVersion: v2
name: kube-agent-helper
description: Kubernetes AI diagnostic operator
type: application
version: 0.1.0
appVersion: "0.1.0"
```

- [ ] **Step 2: Write `deploy/helm/values.yaml`**

```yaml
replicaCount: 1

image:
  controller: ghcr.io/kube-agent-helper/controller:latest
  agent: ghcr.io/kube-agent-helper/agent-runtime:latest
  pullPolicy: IfNotPresent

controller:
  httpAddr: ":8080"
  dbPath: "/data/kube-agent-helper.db"
  controllerURL: "http://{{ .Release.Name }}.{{ .Release.Namespace }}.svc.cluster.local:8080"

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    memory: 256Mi

persistence:
  enabled: true
  size: 1Gi
  storageClass: ""

anthropic:
  secretName: "anthropic-credentials"
  secretKey: "apiKey"
```

- [ ] **Step 3: Write `deploy/helm/templates/deployment.yaml`**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-controller
  namespace: {{ .Release.Namespace }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app: {{ .Release.Name }}-controller
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}-controller
    spec:
      serviceAccountName: {{ .Release.Name }}-controller
      containers:
      - name: controller
        image: {{ .Values.image.controller }}
        imagePullPolicy: {{ .Values.image.pullPolicy }}
        args:
        - --http-addr={{ .Values.controller.httpAddr }}
        - --db={{ .Values.controller.dbPath }}
        - --agent-image={{ .Values.image.agent }}
        - --controller-url={{ .Values.controller.controllerURL }}
        - --skills-dir=/skills
        ports:
        - containerPort: 8080
        resources:
          {{- toYaml .Values.resources | nindent 10 }}
        volumeMounts:
        - name: data
          mountPath: /data
        - name: skills
          mountPath: /skills
      volumes:
      - name: data
        {{- if .Values.persistence.enabled }}
        persistentVolumeClaim:
          claimName: {{ .Release.Name }}-data
        {{- else }}
        emptyDir: {}
        {{- end }}
      - name: skills
        configMap:
          name: {{ .Release.Name }}-skills
```

- [ ] **Step 4: Write `deploy/helm/templates/rbac.yaml`**

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ .Release.Name }}-controller
  namespace: {{ .Release.Namespace }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ .Release.Name }}-controller
rules:
- apiGroups: ["k8sai.io"]
  resources: ["diagnosticruns","diagnosticskills","modelconfigs"]
  verbs: ["get","list","watch","update","patch"]
- apiGroups: ["k8sai.io"]
  resources: ["diagnosticruns/status"]
  verbs: ["update","patch"]
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["get","list","watch","create","delete"]
- apiGroups: [""]
  resources: ["configmaps","serviceaccounts","secrets"]
  verbs: ["get","list","watch","create","delete"]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["rolebindings"]
  verbs: ["get","list","watch","create","delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ .Release.Name }}-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ .Release.Name }}-controller
subjects:
- kind: ServiceAccount
  name: {{ .Release.Name }}-controller
  namespace: {{ .Release.Namespace }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}
  namespace: {{ .Release.Namespace }}
spec:
  selector:
    app: {{ .Release.Name }}-controller
  ports:
  - port: 8080
    targetPort: 8080
```

- [ ] **Step 5: Copy CRDs into Helm chart**

```bash
mkdir -p deploy/helm/templates/crds
cp deploy/crds/*.yaml deploy/helm/templates/crds/
```

- [ ] **Step 6: Commit**

```bash
git add deploy/helm/
git commit --no-gpg-sign -m "feat(helm): Helm chart for one-command cluster deployment"
```

---

## Task 13: Reconciler unit tests + Phase 1 verification

**Files:**
- Create: `internal/controller/reconciler/run_reconciler_test.go`

- [ ] **Step 1: Write `internal/controller/reconciler/run_reconciler_test.go`**

```go
package reconciler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type memStore struct{ runs map[string]*store.DiagnosticRun }

func newMemStore() *memStore { return &memStore{runs: map[string]*store.DiagnosticRun{}} }
func (m *memStore) CreateRun(_ context.Context, r *store.DiagnosticRun) error {
	if r.ID == "" { r.ID = "test-id" }
	m.runs[r.ID] = r; return nil
}
func (m *memStore) GetRun(_ context.Context, id string) (*store.DiagnosticRun, error) {
	return m.runs[id], nil
}
func (m *memStore) UpdateRunStatus(_ context.Context, id string, p store.Phase, msg string) error {
	if r, ok := m.runs[id]; ok { r.Status = p }; return nil
}
func (m *memStore) ListRuns(_ context.Context, _ store.ListOpts) ([]*store.DiagnosticRun, error) { return nil, nil }
func (m *memStore) CreateFinding(_ context.Context, _ *store.Finding) error { return nil }
func (m *memStore) ListFindings(_ context.Context, _ string) ([]*store.Finding, error) { return nil, nil }
func (m *memStore) UpsertSkill(_ context.Context, _ *store.Skill) error { return nil }
func (m *memStore) ListSkills(_ context.Context) ([]*store.Skill, error) { return nil, nil }
func (m *memStore) GetSkill(_ context.Context, _ string) (*store.Skill, error) { return nil, nil }
func (m *memStore) Close() error { return nil }

func TestRunReconciler_PendingToRunning(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = k8saiV1.AddToScheme(scheme)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-run", Namespace: "default", UID: "uid-1",
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health-analyst"},
			ModelConfigRef: "claude-default",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()

	skill := &store.Skill{
		Name: "pod-health-analyst", Dimension: "health",
		Prompt: "You are...", ToolsJSON: "[]", Enabled: true,
	}
	tr := translator.New(translator.Config{
		AgentImage: "agent:test", ControllerURL: "http://ctrl:8080",
	}, []*store.Skill{skill})

	ms := newMemStore()
	r := &reconciler.DiagnosticRunReconciler{
		Client:     fakeClient,
		Store:      ms,
		Translator: tr,
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "test-run", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(context.Background(),
		types.NamespacedName{Name: "test-run", Namespace: "default"}, &updated))
	assert.Equal(t, "Running", updated.Status.Phase)
}
```

- [ ] **Step 2: Run all tests**

```bash
go test ./... -count=1 -timeout=90s 2>&1 | grep -E "^(ok|FAIL|---)"
```

Expected: all `ok`.

- [ ] **Step 3: Final build verification**

```bash
go build ./cmd/k8s-mcp-server ./cmd/controller
```

- [ ] **Step 4: Commit Phase 1 complete**

```bash
git add internal/controller/reconciler/run_reconciler_test.go
git commit --no-gpg-sign -m "test(reconciler): DiagnosticRunReconciler unit test — Phase 1 complete"
```

---

## Phase 1 Done ✓

At this point:
- `go build ./cmd/controller` produces the Operator binary
- `go build ./cmd/k8s-mcp-server` produces the MCP server binary
- `helm install kube-agent-helper ./deploy/helm` deploys to cluster
- `kubectl apply -f run.yaml` triggers a full diagnostic run with 3 skills
- `GET /api/runs` returns persisted run history
- `GET /api/runs/{id}/findings` returns structured findings

**Next:** See `2026-04-12-kube-agent-helper-phase2.md` for SkillRegistry + Dashboard + PostgreSQL.
