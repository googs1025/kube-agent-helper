# Multi-Cluster Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow a single kube-agent-helper controller to register remote clusters via a `ClusterConfig` CRD, run diagnostics against them, and filter all data by cluster in the Dashboard.

**Architecture:** A new `ClusterConfig` CRD stores kubeconfig secret references for remote clusters. A `ClusterClientRegistry` maintains a `map[clusterName]client.Client` updated by a new reconciler. `DiagnosticRun.spec.clusterRef` selects the target cluster; the RunReconciler resolves the client from the registry and creates Jobs on that cluster. All store tables gain a `cluster_name` column (default `"local"`) so the HTTP API and Dashboard can filter by cluster. The Dashboard gets a global `ClusterContext` and a nav dropdown.

**Tech Stack:** Go 1.22, controller-runtime, SQLite (golang-migrate), Next.js 14, SWR, TypeScript, `k8s.io/client-go/tools/clientcmd`

**Out of scope:** EventCollector for remote clusters; cross-cluster ScheduledRun; Fix apply on remote clusters (FixReconciler stays local-only in this iteration).

---

## File Map

### New files
| File | Responsibility |
|------|---------------|
| `internal/store/sqlite/migrations/005_multi_cluster.up.sql` | Add `cluster_name` column to 5 tables |
| `internal/store/sqlite/migrations/005_multi_cluster.down.sql` | Drop `cluster_name` columns |
| `internal/controller/registry/cluster_registry.go` | `ClusterClientRegistry` — thread-safe map of cluster clients |
| `internal/controller/registry/cluster_registry_test.go` | Unit tests for registry |
| `internal/controller/reconciler/clusterconfig_reconciler.go` | `ClusterConfigReconciler` — builds clients from kubeconfig secrets |
| `dashboard/src/cluster/context.tsx` | `ClusterContext` + `useCluster` hook |
| `dashboard/src/components/cluster-toggle.tsx` | Nav dropdown for cluster selection |
| `dashboard/src/app/clusters/page.tsx` | ClusterConfig management page with setup guide |
| `docs/examples/clusterconfig/local-only.yaml` | Example: local cluster (no ClusterConfig needed) |
| `docs/examples/clusterconfig/remote-with-sa-token.yaml` | Example: remote cluster with SA token kubeconfig |
| `docs/examples/diagnosticrun/cross-cluster-run.yaml` | Example: DiagnosticRun targeting remote cluster |

### Modified files
| File | Change |
|------|--------|
| `internal/store/store.go` | Add `ClusterName` field to structs; add `ClusterName` to `ListOpts` and `ListEventsOpts` |
| `internal/store/sqlite/sqlite.go` | Update all INSERT/SELECT/WHERE for `cluster_name` |
| `internal/controller/api/v1alpha1/types.go` | Add `ClusterConfig` CRD; add `ClusterRef` to `DiagnosticRunSpec` |
| `internal/controller/api/v1alpha1/zz_generated.deepcopy.go` | Add deepcopy for `ClusterConfig`, `ClusterConfigList` |
| `internal/controller/reconciler/run_reconciler.go` | Inject `ClusterClientRegistry`; use target client; set `ClusterName` on store run |
| `internal/controller/httpserver/server.go` | Add `/api/clusters` (GET+POST); thread `?cluster=` param to store queries |
| `cmd/controller/main.go` | Instantiate `ClusterClientRegistry`; register `ClusterConfigReconciler` |
| `dashboard/src/lib/api.ts` | Add `useClusterConfigs()`, `createClusterConfig()`; add `cluster?` param to all list hooks |
| `dashboard/src/app/layout.tsx` | Add `ClusterToggle` + nav link to clusters; wrap in `ClusterProvider` |
| `dashboard/src/i18n/en.json` | Add cluster + clusters page keys |
| `dashboard/src/i18n/zh.json` | Add cluster + clusters page keys |
| `docs/crd-user-guide.md` | Add ClusterConfig section with multi-cluster setup guide |
| `README.md` | Add multi-cluster feature; update CRD count to 5 |
| `README_EN.md` | Same in English |

---

## Task 1: DB Migration — Add `cluster_name` Column

**Files:**
- Create: `internal/store/sqlite/migrations/005_multi_cluster.up.sql`
- Create: `internal/store/sqlite/migrations/005_multi_cluster.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- internal/store/sqlite/migrations/005_multi_cluster.up.sql
ALTER TABLE diagnostic_runs   ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';
ALTER TABLE findings          ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';
ALTER TABLE fixes             ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';
ALTER TABLE events            ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';
ALTER TABLE metric_snapshots  ADD COLUMN cluster_name TEXT NOT NULL DEFAULT 'local';

CREATE INDEX IF NOT EXISTS idx_runs_cluster    ON diagnostic_runs(cluster_name);
CREATE INDEX IF NOT EXISTS idx_fixes_cluster   ON fixes(cluster_name);
CREATE INDEX IF NOT EXISTS idx_events_cluster  ON events(cluster_name);
CREATE INDEX IF NOT EXISTS idx_metrics_cluster ON metric_snapshots(cluster_name);
```

- [ ] **Step 2: Write the down migration**

```sql
-- internal/store/sqlite/migrations/005_multi_cluster.down.sql
-- SQLite >=3.35 supports DROP COLUMN; older versions require table rebuild.
-- Using pragma to check version at runtime is fragile; accept that down
-- migration is a no-op on SQLite <3.35 and document this.
ALTER TABLE diagnostic_runs   DROP COLUMN cluster_name;
ALTER TABLE findings          DROP COLUMN cluster_name;
ALTER TABLE fixes             DROP COLUMN cluster_name;
ALTER TABLE events            DROP COLUMN cluster_name;
ALTER TABLE metric_snapshots  DROP COLUMN cluster_name;
```

- [ ] **Step 3: Verify migration applies cleanly**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
go test ./internal/store/sqlite/... -run TestMigrations -v
```

Expected: PASS (migration framework auto-applies numbered migrations on `New()`).

- [ ] **Step 4: Commit**

```bash
git add internal/store/sqlite/migrations/005_multi_cluster.up.sql \
        internal/store/sqlite/migrations/005_multi_cluster.down.sql
git commit -m "feat(store): migration 005 — add cluster_name to all tables"
```

---

## Task 2: Store Interface — Add `ClusterName` to Structs and Opts

**Files:**
- Modify: `internal/store/store.go`

- [ ] **Step 1: Add `ClusterName` to `DiagnosticRun`, `Finding`, `Fix`, `Event`, `MetricSnapshot` and `ClusterName` filter to `ListOpts` and `ListEventsOpts`**

Replace the struct and opts section in `internal/store/store.go`:

```go
type DiagnosticRun struct {
	ID          string
	Name        string // K8s CR name, populated at API layer (not persisted in SQLite)
	ClusterName string // "local" for in-cluster; ClusterConfig.Name for remote
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
	ClusterName       string
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

type Fix struct {
	ID               string
	Name             string
	RunID            string
	ClusterName      string
	FindingTitle     string
	TargetKind       string
	TargetNamespace  string
	TargetName       string
	Strategy         string
	ApprovalRequired bool
	PatchType        string
	PatchContent     string
	Phase            FixPhase
	ApprovedBy       string
	RollbackSnapshot string
	Message          string
	FindingID        string
	BeforeSnapshot   string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Event struct {
	ID          int64
	UID         string
	ClusterName string
	Namespace   string
	Kind        string
	Name        string
	Reason      string
	Message     string
	Type        string
	Count       int32
	FirstTime   time.Time
	LastTime    time.Time
	CreatedAt   time.Time
}

type ListEventsOpts struct {
	ClusterName  string
	Namespace    string
	Name         string
	Type         string
	SinceMinutes int
	Limit        int
}

type MetricSnapshot struct {
	ID          int64
	ClusterName string
	Query       string
	LabelsJSON  string
	Value       float64
	Ts          time.Time
	CreatedAt   time.Time
}

type ListOpts struct {
	ClusterName string // "" = all clusters
	Limit       int
	Offset      int
}
```

- [ ] **Step 2: Build to check for compile errors**

```bash
go build ./internal/store/...
```

Expected: compile errors in sqlite.go — fix in next task.

- [ ] **Step 3: Commit**

```bash
git add internal/store/store.go
git commit -m "feat(store): add ClusterName to all store types and list opts"
```

---

## Task 3: SQLite Implementation — Update All Queries

**Files:**
- Modify: `internal/store/sqlite/sqlite.go`

- [ ] **Step 1: Update `CreateRun` to include `cluster_name`**

Find the INSERT in `CreateRun` and update it:

```go
func (s *SQLiteStore) CreateRun(ctx context.Context, run *store.DiagnosticRun) error {
	if run.ID == "" {
		run.ID = uuid.New().String()
	}
	if run.ClusterName == "" {
		run.ClusterName = "local"
	}
	now := time.Now().UTC()
	run.CreatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO diagnostic_runs
		 (id, cluster_name, target_json, skills_json, status, message, started_at, completed_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.ClusterName, run.TargetJSON, run.SkillsJSON,
		run.Status, run.Message, run.StartedAt, run.CompletedAt, now,
	)
	return err
}
```

- [ ] **Step 2: Update the `scanRun` helper (or inline scan) to read `cluster_name`**

Find where `diagnostic_runs` rows are scanned and update the SELECT + Scan:

```go
// In ListRuns and GetRun, update SELECT to include cluster_name:
const selectRunCols = `id, cluster_name, target_json, skills_json, status, message,
                        started_at, completed_at, created_at`

// Scan order must match SELECT:
func scanRun(row interface{ Scan(...interface{}) error }) (*store.DiagnosticRun, error) {
	var r store.DiagnosticRun
	err := row.Scan(
		&r.ID, &r.ClusterName, &r.TargetJSON, &r.SkillsJSON,
		&r.Status, &r.Message, &r.StartedAt, &r.CompletedAt, &r.CreatedAt,
	)
	return &r, err
}
```

- [ ] **Step 3: Update `ListRuns` to filter by `ClusterName`**

```go
func (s *SQLiteStore) ListRuns(ctx context.Context, opts store.ListOpts) ([]*store.DiagnosticRun, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 50
	}
	args := []interface{}{}
	where := ""
	if opts.ClusterName != "" {
		where = "WHERE cluster_name = ?"
		args = append(args, opts.ClusterName)
	}
	args = append(args, limit, opts.Offset)
	query := fmt.Sprintf(
		"SELECT %s FROM diagnostic_runs %s ORDER BY created_at DESC LIMIT ? OFFSET ?",
		selectRunCols, where,
	)
	rows, err := s.db.QueryContext(ctx, query, args...)
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
```

- [ ] **Step 4: Update `CreateFinding` and `ListFindings`**

```go
func (s *SQLiteStore) CreateFinding(ctx context.Context, f *store.Finding) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	if f.ClusterName == "" {
		f.ClusterName = "local"
	}
	f.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO findings
		 (id, run_id, cluster_name, dimension, severity, title, description,
		  resource_kind, resource_namespace, resource_name, suggestion, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.RunID, f.ClusterName, f.Dimension, f.Severity, f.Title, f.Description,
		f.ResourceKind, f.ResourceNamespace, f.ResourceName, f.Suggestion, f.CreatedAt,
	)
	return err
}

// ListFindings: add cluster_name to SELECT
func (s *SQLiteStore) ListFindings(ctx context.Context, runID string) ([]*store.Finding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, cluster_name, dimension, severity, title, description,
		        resource_kind, resource_namespace, resource_name, suggestion, created_at
		 FROM findings WHERE run_id = ? ORDER BY created_at ASC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var findings []*store.Finding
	for rows.Next() {
		var f store.Finding
		if err := rows.Scan(&f.ID, &f.RunID, &f.ClusterName, &f.Dimension, &f.Severity,
			&f.Title, &f.Description, &f.ResourceKind, &f.ResourceNamespace,
			&f.ResourceName, &f.Suggestion, &f.CreatedAt); err != nil {
			return nil, err
		}
		findings = append(findings, &f)
	}
	return findings, rows.Err()
}
```

- [ ] **Step 5: Update `CreateFix`, `scanFix`, `ListFixes`, `ListFixesByRun`**

```go
func (s *SQLiteStore) CreateFix(ctx context.Context, f *store.Fix) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	if f.ClusterName == "" {
		f.ClusterName = "local"
	}
	now := time.Now().UTC()
	f.CreatedAt, f.UpdatedAt = now, now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fixes
		 (id, run_id, cluster_name, finding_title, target_kind, target_namespace, target_name,
		  strategy, approval_required, patch_type, patch_content, phase, approved_by,
		  rollback_snapshot, message, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.RunID, f.ClusterName, f.FindingTitle, f.TargetKind, f.TargetNamespace, f.TargetName,
		f.Strategy, f.ApprovalRequired, f.PatchType, f.PatchContent, f.Phase, f.ApprovedBy,
		f.RollbackSnapshot, f.Message, now, now,
	)
	return err
}

// scanFix: add cluster_name to scan (update SELECT cols + Scan call wherever fixes are read)
// The scan must include cluster_name right after id, run_id:
// id, run_id, cluster_name, finding_title, ...
```

- [ ] **Step 6: Update `UpsertEvent` and `ListEvents`**

```go
func (s *SQLiteStore) UpsertEvent(ctx context.Context, e *store.Event) error {
	if e.ClusterName == "" {
		e.ClusterName = "local"
	}
	// ... existing INSERT OR REPLACE with cluster_name added to columns and VALUES
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO events
		 (uid, cluster_name, namespace, kind, name, reason, message, type, count, first_time, last_time, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(uid) DO UPDATE SET
		   count=excluded.count, last_time=excluded.last_time, message=excluded.message`,
		e.UID, e.ClusterName, e.Namespace, e.Kind, e.Name, e.Reason, e.Message, e.Type,
		e.Count, e.FirstTime.Unix(), e.LastTime.Unix(), time.Now().Unix(),
	)
	return err
}

func (s *SQLiteStore) ListEvents(ctx context.Context, opts store.ListEventsOpts) ([]*store.Event, error) {
	args := []interface{}{}
	clauses := []string{}
	if opts.ClusterName != "" {
		clauses = append(clauses, "cluster_name = ?")
		args = append(args, opts.ClusterName)
	}
	if opts.Namespace != "" {
		clauses = append(clauses, "namespace = ?")
		args = append(args, opts.Namespace)
	}
	if opts.Name != "" {
		clauses = append(clauses, "name LIKE ?")
		args = append(args, "%"+opts.Name+"%")
	}
	if opts.SinceMinutes > 0 {
		clauses = append(clauses, "last_time >= ?")
		args = append(args, time.Now().Add(-time.Duration(opts.SinceMinutes)*time.Minute).Unix())
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	limit := opts.Limit
	if limit == 0 {
		limit = 100
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT id, uid, cluster_name, namespace, kind, name, reason, message, type,
		                    count, first_time, last_time, created_at
		             FROM events %s ORDER BY last_time DESC LIMIT ?`, where),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []*store.Event
	for rows.Next() {
		var ev store.Event
		var firstUnix, lastUnix, createdUnix int64
		if err := rows.Scan(&ev.ID, &ev.UID, &ev.ClusterName, &ev.Namespace, &ev.Kind, &ev.Name,
			&ev.Reason, &ev.Message, &ev.Type, &ev.Count,
			&firstUnix, &lastUnix, &createdUnix); err != nil {
			return nil, err
		}
		ev.FirstTime = time.Unix(firstUnix, 0)
		ev.LastTime = time.Unix(lastUnix, 0)
		ev.CreatedAt = time.Unix(createdUnix, 0)
		events = append(events, &ev)
	}
	return events, rows.Err()
}
```

- [ ] **Step 7: Update `InsertMetricSnapshot` and `QueryMetricHistory`**

```go
func (s *SQLiteStore) InsertMetricSnapshot(ctx context.Context, snap *store.MetricSnapshot) error {
	if snap.ClusterName == "" {
		snap.ClusterName = "local"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO metric_snapshots (cluster_name, query, labels_json, value, ts, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		snap.ClusterName, snap.Query, snap.LabelsJSON, snap.Value,
		snap.Ts.Unix(), time.Now().Unix(),
	)
	return err
}

func (s *SQLiteStore) QueryMetricHistory(ctx context.Context, query string, sinceMinutes int) ([]*store.MetricSnapshot, error) {
	since := time.Now().Add(-time.Duration(sinceMinutes) * time.Minute).Unix()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, cluster_name, query, labels_json, value, ts, created_at
		 FROM metric_snapshots WHERE query = ? AND ts >= ? ORDER BY ts ASC`,
		query, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snaps []*store.MetricSnapshot
	for rows.Next() {
		var s store.MetricSnapshot
		var tsUnix, createdUnix int64
		if err := rows.Scan(&s.ID, &s.ClusterName, &s.Query, &s.LabelsJSON, &s.Value,
			&tsUnix, &createdUnix); err != nil {
			return nil, err
		}
		s.Ts = time.Unix(tsUnix, 0)
		s.CreatedAt = time.Unix(createdUnix, 0)
		snaps = append(snaps, &s)
	}
	return snaps, rows.Err()
}
```

- [ ] **Step 8: Build and run store tests**

```bash
go build ./...
go test ./internal/store/... -v
```

Expected: all tests PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/store/sqlite/sqlite.go
git commit -m "feat(store): update SQLite impl for cluster_name column"
```

---

## Task 4: ClusterConfig CRD + DiagnosticRun.ClusterRef

**Files:**
- Modify: `internal/controller/api/v1alpha1/types.go`
- Modify: `internal/controller/api/v1alpha1/zz_generated.deepcopy.go`

- [ ] **Step 1: Add `ClusterConfig` CRD and `ClusterRef` to `DiagnosticRunSpec`**

In `internal/controller/api/v1alpha1/types.go`, add after `DiagnosticFixList`:

```go
// ── ClusterConfig ─────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterConfigSpec   `json:"spec,omitempty"`
	Status            ClusterConfigStatus `json:"status,omitempty"`
}

type ClusterConfigSpec struct {
	// KubeConfigRef is the reference to a Secret containing a kubeconfig for the remote cluster.
	KubeConfigRef SecretKeyRef `json:"kubeConfigRef"`
	// PrometheusURL is the Prometheus endpoint accessible from within the remote cluster (optional).
	// +optional
	PrometheusURL string `json:"prometheusURL,omitempty"`
	// +optional
	Description string `json:"description,omitempty"`
}

type ClusterConfigStatus struct {
	// +kubebuilder:validation:Enum=Connected;Error
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
type ClusterConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterConfig `json:"items"`
}
```

- [ ] **Step 2: Add `ClusterRef` to `DiagnosticRunSpec`**

In `DiagnosticRunSpec`, add before the closing brace:

```go
type DiagnosticRunSpec struct {
	Target         TargetSpec `json:"target"`
	Skills         []string   `json:"skills,omitempty"`
	ModelConfigRef string     `json:"modelConfigRef"`
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum=zh;en
	OutputLanguage string `json:"outputLanguage,omitempty"`
	// +optional
	Schedule string `json:"schedule,omitempty"`
	// +optional
	// +kubebuilder:default=10
	HistoryLimit *int32 `json:"historyLimit,omitempty"`
	// ClusterRef is the name of a ClusterConfig CR in the same namespace.
	// When empty, the local (controller) cluster is used.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`
}
```

- [ ] **Step 3: Register `ClusterConfig` in `init()`**

Update the `init()` at the bottom of `types.go`:

```go
func init() {
	SchemeBuilder.Register(
		&DiagnosticSkill{}, &DiagnosticSkillList{},
		&DiagnosticRun{}, &DiagnosticRunList{},
		&ModelConfig{}, &ModelConfigList{},
		&DiagnosticFix{}, &DiagnosticFixList{},
		&ClusterConfig{}, &ClusterConfigList{},
	)
}
```

- [ ] **Step 4: Add deepcopy for `ClusterConfig` and `ClusterConfigList`**

Append to `internal/controller/api/v1alpha1/zz_generated.deepcopy.go`:

```go
// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterConfig) DeepCopyInto(out *ClusterConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterConfig.
func (in *ClusterConfig) DeepCopy() *ClusterConfig {
	if in == nil {
		return nil
	}
	out := new(ClusterConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements the runtime.Object interface.
func (in *ClusterConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterConfigList) DeepCopyInto(out *ClusterConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterConfigList.
func (in *ClusterConfigList) DeepCopy() *ClusterConfigList {
	if in == nil {
		return nil
	}
	out := new(ClusterConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements the runtime.Object interface.
func (in *ClusterConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
```

Also add `ClusterRef` deepcopy to `DiagnosticRunSpec` — since `ClusterRef` is a plain `string`, the existing `DeepCopyInto` for `DiagnosticRunSpec` copies it automatically via `*out = *in`. No change needed there.

- [ ] **Step 5: Build**

```bash
go build ./internal/controller/api/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/controller/api/v1alpha1/types.go \
        internal/controller/api/v1alpha1/zz_generated.deepcopy.go
git commit -m "feat(crd): add ClusterConfig CRD and DiagnosticRun.spec.clusterRef"
```

---

## Task 5: ClusterClientRegistry + ClusterConfigReconciler + Wire into main.go

**Files:**
- Create: `internal/controller/registry/cluster_registry.go`
- Create: `internal/controller/registry/cluster_registry_test.go`
- Create: `internal/controller/reconciler/clusterconfig_reconciler.go`
- Modify: `cmd/controller/main.go`

- [ ] **Step 1: Write failing test for ClusterClientRegistry**

```go
// internal/controller/registry/cluster_registry_test.go
package registry_test

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
)

func TestClusterClientRegistry_SetGetDelete(t *testing.T) {
	reg := registry.NewClusterClientRegistry()

	fakeClient := fake.NewClientBuilder().Build()

	// Set
	reg.Set("prod", fakeClient)

	// Get existing
	got, ok := reg.Get("prod")
	if !ok {
		t.Fatal("expected to find 'prod' cluster")
	}
	if got != fakeClient {
		t.Fatal("expected same client instance")
	}

	// Get missing
	_, ok = reg.Get("staging")
	if ok {
		t.Fatal("expected 'staging' to be missing")
	}

	// Delete
	reg.Delete("prod")
	_, ok = reg.Get("prod")
	if ok {
		t.Fatal("expected 'prod' to be deleted")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/controller/registry/... -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `ClusterClientRegistry`**

```go
// internal/controller/registry/cluster_registry.go
package registry

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterClientRegistry maintains a thread-safe map of cluster name → k8s client.
// The local (in-cluster) client is not stored here; callers should fall back to
// the controller-runtime manager client when ClusterRef is empty.
type ClusterClientRegistry struct {
	mu      sync.RWMutex
	clients map[string]client.Client
}

func NewClusterClientRegistry() *ClusterClientRegistry {
	return &ClusterClientRegistry{
		clients: make(map[string]client.Client),
	}
}

// Get returns the client for the given cluster name.
func (r *ClusterClientRegistry) Get(name string) (client.Client, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.clients[name]
	return c, ok
}

// Set registers or updates the client for the given cluster name.
func (r *ClusterClientRegistry) Set(name string, c client.Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[name] = c
}

// Delete removes the client for the given cluster name.
func (r *ClusterClientRegistry) Delete(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, name)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/controller/registry/... -v
```

Expected: PASS.

- [ ] **Step 5: Implement `ClusterConfigReconciler`**

```go
// internal/controller/reconciler/clusterconfig_reconciler.go
package reconciler

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
)

type ClusterConfigReconciler struct {
	client.Client
	Registry *registry.ClusterClientRegistry
}

func (r *ClusterConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var cc k8saiV1.ClusterConfig
	if err := r.Get(ctx, req.NamespacedName, &cc); err != nil {
		if errors.IsNotFound(err) {
			// CR deleted — remove client from registry
			r.Registry.Delete(req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the kubeconfig secret
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name:      cc.Spec.KubeConfigRef.Name,
		Namespace: cc.Namespace,
	}, &secret); err != nil {
		return ctrl.Result{}, r.setStatus(ctx, &cc, "Error", "kubeconfig secret not found: "+err.Error())
	}

	kubeconfigData, ok := secret.Data[cc.Spec.KubeConfigRef.Key]
	if !ok {
		return ctrl.Result{}, r.setStatus(ctx, &cc, "Error",
			"key "+cc.Spec.KubeConfigRef.Key+" not found in secret")
	}

	// Build REST config from kubeconfig bytes
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return ctrl.Result{}, r.setStatus(ctx, &cc, "Error", "invalid kubeconfig: "+err.Error())
	}

	// Build the cluster client using the same scheme as the manager
	clusterClient, err := client.New(restCfg, client.Options{Scheme: r.Scheme()})
	if err != nil {
		return ctrl.Result{}, r.setStatus(ctx, &cc, "Error", "failed to build client: "+err.Error())
	}

	r.Registry.Set(cc.Name, clusterClient)
	logger.Info("registered cluster client", "cluster", cc.Name)

	return ctrl.Result{}, r.setStatus(ctx, &cc, "Connected", "")
}

func (r *ClusterConfigReconciler) setStatus(ctx context.Context, cc *k8saiV1.ClusterConfig, phase, msg string) error {
	cc.Status.Phase = phase
	cc.Status.Message = msg
	return r.Status().Update(ctx, cc)
}

func (r *ClusterConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.ClusterConfig{}).
		Complete(r)
}
```

- [ ] **Step 6: Wire into `cmd/controller/main.go`**

In `main.go`, after the existing reconciler registrations, add:

```go
// After: import the registry package
import "github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"

// In main(), after creating the manager:
clusterRegistry := registry.NewClusterClientRegistry()

// Register ClusterConfigReconciler (alongside the other reconcilers):
if err := (&reconciler.ClusterConfigReconciler{
    Client:   mgr.GetClient(),
    Registry: clusterRegistry,
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "ClusterConfig")
    os.Exit(1)
}

// Pass clusterRegistry to DiagnosticRunReconciler (next task updates that struct)
```

- [ ] **Step 7: Build**

```bash
go build ./...
```

Expected: PASS (DiagnosticRunReconciler change comes in Task 6).

- [ ] **Step 8: Commit**

```bash
git add internal/controller/registry/ \
        internal/controller/reconciler/clusterconfig_reconciler.go \
        cmd/controller/main.go
git commit -m "feat: add ClusterClientRegistry and ClusterConfigReconciler"
```

---

## Task 6: RunReconciler — Route by `ClusterRef`

**Files:**
- Modify: `internal/controller/reconciler/run_reconciler.go`
- Modify: `cmd/controller/main.go` (pass registry to RunReconciler)

- [ ] **Step 1: Add `Registry` field to `DiagnosticRunReconciler`**

```go
type DiagnosticRunReconciler struct {
	client.Client
	Store      store.Store
	Translator *translator.Translator
	Registry   *registry.ClusterClientRegistry // nil = local-only mode
}
```

- [ ] **Step 2: Resolve target cluster client in `Reconcile`**

In the Pending → Running block, before calling `Translator.Compile`, add:

```go
// Resolve target cluster client
targetClient := client.Client(r.Client) // default: local cluster
clusterName := "local"
if run.Spec.ClusterRef != "" {
    clusterName = run.Spec.ClusterRef
    if r.Registry == nil {
        return r.failRun(ctx, &run, "cluster registry not configured")
    }
    c, ok := r.Registry.Get(run.Spec.ClusterRef)
    if !ok {
        return r.failRun(ctx, &run, fmt.Sprintf("cluster %q not registered — create a ClusterConfig CR", run.Spec.ClusterRef))
    }
    targetClient = c
}
```

- [ ] **Step 3: Use `targetClient` when creating compiled objects**

Find the loop that creates objects returned by `Translator.Compile` and replace `r.Client` with `targetClient`:

```go
objs, err := r.Translator.Compile(ctx, &run)
if err != nil {
    return r.failRun(ctx, &run, fmt.Sprintf("compile: %v", err))
}
for _, obj := range objs {
    if err := targetClient.Create(ctx, obj); err != nil {
        if !apierrors.IsAlreadyExists(err) {
            return r.failRun(ctx, &run, fmt.Sprintf("create %T: %v", obj, err))
        }
    }
}
```

- [ ] **Step 4: Set `ClusterName` on the store run**

```go
storeRun := &store.DiagnosticRun{
    ID:          string(run.UID),
    ClusterName: clusterName,
    TargetJSON:  mustJSON(run.Spec.Target),
    SkillsJSON:  mustJSON(run.Spec.Skills),
    Status:      store.PhasePending,
}
```

- [ ] **Step 5: Update `main.go` to pass Registry to RunReconciler**

```go
if err := (&reconciler.DiagnosticRunReconciler{
    Client:     mgr.GetClient(),
    Store:      st,
    Translator: trans,
    Registry:   clusterRegistry,
}).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "DiagnosticRun")
    os.Exit(1)
}
```

- [ ] **Step 6: Build**

```bash
go build ./...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/controller/reconciler/run_reconciler.go cmd/controller/main.go
git commit -m "feat(reconciler): route DiagnosticRun Jobs to target cluster via ClusterRef"
```

---

## Task 7: HTTP API — `/api/clusters` + `?cluster=` Filter

**Files:**
- Modify: `internal/controller/httpserver/server.go`

- [ ] **Step 1: Add `/api/clusters` endpoint**

In `Server`, the `k8sClient` already exists. Add the handler to `NewServer` mux setup:

```go
mux.HandleFunc("/api/clusters", s.handleAPIClusters)
```

Implement the handler:

```go
func (s *Server) handleAPIClusters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()

	var list k8saiV1.ClusterConfigList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type clusterItem struct {
		Name          string `json:"name"`
		Phase         string `json:"phase"`
		PrometheusURL string `json:"prometheusURL,omitempty"`
		Description   string `json:"description,omitempty"`
	}

	// Always include local cluster
	items := []clusterItem{{Name: "local", Phase: "Connected", Description: "In-cluster (local)"}}
	for _, cc := range list.Items {
		items = append(items, clusterItem{
			Name:          cc.Name,
			Phase:         cc.Status.Phase,
			PrometheusURL: cc.Spec.PrometheusURL,
			Description:   cc.Spec.Description,
		})
	}
	writeJSON(w, items)
}
```

- [ ] **Step 2: Thread `?cluster=` to `ListRuns`**

In `handleAPIRuns` (GET branch), extract the cluster param and pass to store:

```go
cluster := r.URL.Query().Get("cluster")
runs, err := s.store.ListRuns(ctx, store.ListOpts{
    ClusterName: cluster,
    Limit:       limit,
    Offset:      offset,
})
```

- [ ] **Step 3: Thread `?cluster=` to `ListFixes`**

In `handleAPIFixes` (GET branch):

```go
cluster := r.URL.Query().Get("cluster")
fixes, err := s.store.ListFixes(ctx, store.ListOpts{
    ClusterName: cluster,
    Limit:       limit,
    Offset:      offset,
})
```

- [ ] **Step 4: Thread `?cluster=` to `ListEvents`**

In `handleAPIEvents`:

```go
opts := store.ListEventsOpts{
    ClusterName:  r.URL.Query().Get("cluster"),
    Namespace:    r.URL.Query().Get("namespace"),
    Name:         r.URL.Query().Get("name"),
    SinceMinutes: sinceMinutes,
    Limit:        limit,
}
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

Expected: PASS.

- [ ] **Step 6: Manual smoke test**

```bash
# Start controller locally (requires kubeconfig)
go run ./cmd/controller/main.go &
curl http://localhost:8080/api/clusters
# Expected: [{"name":"local","phase":"Connected","description":"In-cluster (local)"}]
```

- [ ] **Step 7: Commit**

```bash
git add internal/controller/httpserver/server.go
git commit -m "feat(api): add /api/clusters endpoint and ?cluster= filter to list APIs"
```

---

## Task 8: Dashboard — ClusterContext, Selector, API Hooks, i18n

**Files:**
- Create: `dashboard/src/cluster/context.tsx`
- Create: `dashboard/src/components/cluster-toggle.tsx`
- Modify: `dashboard/src/lib/api.ts`
- Modify: `dashboard/src/app/layout.tsx`
- Modify: `dashboard/src/i18n/en.json`
- Modify: `dashboard/src/i18n/zh.json`

- [ ] **Step 1: Add i18n keys**

In `dashboard/src/i18n/en.json`, add inside the top-level object:

```json
"cluster.label": "Cluster",
"cluster.local": "Local (in-cluster)"
```

In `dashboard/src/i18n/zh.json`:

```json
"cluster.label": "集群",
"cluster.local": "本地集群"
```

- [ ] **Step 2: Create `ClusterContext`**

```tsx
// dashboard/src/cluster/context.tsx
"use client";

import { createContext, useContext, useState, ReactNode } from "react";

interface ClusterContextValue {
  cluster: string;        // "local" or ClusterConfig.name
  setCluster: (c: string) => void;
}

const ClusterContext = createContext<ClusterContextValue>({
  cluster: "local",
  setCluster: () => {},
});

export function ClusterProvider({ children }: { children: ReactNode }) {
  const [cluster, setCluster] = useState<string>(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("kah-cluster") ?? "local";
    }
    return "local";
  });

  const handleSet = (c: string) => {
    setCluster(c);
    localStorage.setItem("kah-cluster", c);
  };

  return (
    <ClusterContext.Provider value={{ cluster, setCluster: handleSet }}>
      {children}
    </ClusterContext.Provider>
  );
}

export function useCluster() {
  return useContext(ClusterContext);
}
```

- [ ] **Step 3: Add `useClusterConfigs` to `dashboard/src/lib/api.ts`**

```ts
export interface ClusterItem {
  name: string;
  phase: string;
  prometheusURL?: string;
  description?: string;
}

export function useClusterConfigs() {
  return useSWR<ClusterItem[]>("/api/clusters", fetcher, { refreshInterval: 30000 });
}
```

- [ ] **Step 4: Update existing SWR hooks to accept `cluster` param**

Update each hook in `api.ts` to append `?cluster=` when provided:

```ts
export function useRuns(opts?: { cluster?: string }) {
  const params = opts?.cluster ? `?cluster=${opts.cluster}` : "";
  return useSWR<DiagnosticRun[]>(`/api/runs${params}`, fetcher, { refreshInterval: 5000 });
}

export function useFixes(opts?: { cluster?: string }) {
  const params = opts?.cluster ? `?cluster=${opts.cluster}` : "";
  return useSWR<Fix[]>(`/api/fixes${params}`, fetcher, { refreshInterval: 5000 });
}

export function useEvents(opts?: {
  namespace?: string;
  name?: string;
  since?: number;
  limit?: number;
  cluster?: string;
}) {
  const p = new URLSearchParams();
  if (opts?.cluster)   p.set("cluster",   opts.cluster);
  if (opts?.namespace) p.set("namespace", opts.namespace);
  if (opts?.name)      p.set("name",      opts.name);
  if (opts?.since)     p.set("since",     String(opts.since));
  if (opts?.limit)     p.set("limit",     String(opts.limit));
  const qs = p.toString() ? `?${p.toString()}` : "";
  return useSWR<KubeEvent[]>(`/api/events${qs}`, fetcher, { refreshInterval: 15000 });
}
```

- [ ] **Step 5: Create `ClusterToggle` nav component**

```tsx
// dashboard/src/components/cluster-toggle.tsx
"use client";

import { useCluster } from "@/cluster/context";
import { useClusterConfigs } from "@/lib/api";
import { useI18n } from "@/i18n/context";

export function ClusterToggle() {
  const { t } = useI18n();
  const { cluster, setCluster } = useCluster();
  const { data: clusters } = useClusterConfigs();

  if (!clusters || clusters.length <= 1) return null; // hide when only local

  return (
    <select
      className="rounded border border-gray-300 bg-white px-2 py-1 text-xs dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100"
      value={cluster}
      onChange={(e) => setCluster(e.target.value)}
      aria-label={t("cluster.label")}
    >
      {clusters.map((c) => (
        <option key={c.name} value={c.name}>
          {c.name === "local" ? t("cluster.local") : c.name}
          {c.phase === "Error" ? " ⚠" : ""}
        </option>
      ))}
    </select>
  );
}
```

- [ ] **Step 6: Update `layout.tsx` — add `ClusterProvider` and `ClusterToggle`**

```tsx
// Add imports:
import { ClusterProvider } from "@/cluster/context";
import { ClusterToggle } from "@/components/cluster-toggle";

// In Nav, add ClusterToggle next to ThemeToggle:
<div className="flex items-center gap-1">
  <ClusterToggle />
  <ThemeToggle />
  <LanguageToggle />
</div>

// Wrap ClientProviders children with ClusterProvider:
<ClientProviders>
  <ClusterProvider>
    <Nav />
    <ErrorBoundary>
      <main ...>{children}</main>
    </ErrorBoundary>
  </ClusterProvider>
</ClientProviders>
```

- [ ] **Step 7: Pass `cluster` from context to hooks in each page**

In `dashboard/src/app/page.tsx` (runs list), `dashboard/src/app/fixes/page.tsx`, and `dashboard/src/app/events/page.tsx`:

```tsx
import { useCluster } from "@/cluster/context";

// Inside the component:
const { cluster } = useCluster();
const { data: runs } = useRuns({ cluster });
```

- [ ] **Step 8: Build and type-check**

```bash
cd dashboard && npm run build
```

Expected: PASS with no TypeScript errors.

- [ ] **Step 9: Commit**

```bash
git add dashboard/src/cluster/ \
        dashboard/src/components/cluster-toggle.tsx \
        dashboard/src/lib/api.ts \
        dashboard/src/app/layout.tsx \
        dashboard/src/app/page.tsx \
        dashboard/src/app/fixes/page.tsx \
        dashboard/src/app/events/page.tsx \
        dashboard/src/i18n/en.json \
        dashboard/src/i18n/zh.json
git commit -m "feat(dashboard): cluster selector, ClusterContext, API cluster filter"
```

---

## Task 9: Dashboard — ClusterConfig Management Page

**Files:**
- Create: `dashboard/src/app/clusters/page.tsx`
- Modify: `dashboard/src/lib/api.ts` (add `createClusterConfig`)
- Modify: `dashboard/src/lib/types.ts` (add `ClusterItem` type)
- Modify: `dashboard/src/app/layout.tsx` (add nav link)
- Modify: `dashboard/src/i18n/en.json`
- Modify: `dashboard/src/i18n/zh.json`

- [ ] **Step 1: Add i18n keys for clusters page**

In `dashboard/src/i18n/zh.json`:

```json
"nav.clusters": "集群",
"clusters.title": "集群管理",
"clusters.loading": "加载中...",
"clusters.empty": "尚未注册远端集群。本地集群始终可用。",
"clusters.col.name": "集群名称",
"clusters.col.phase": "状态",
"clusters.col.prometheus": "Prometheus",
"clusters.col.description": "描述",
"clusters.create.title": "注册远端集群",
"clusters.create.name": "集群名称",
"clusters.create.namespace": "命名空间",
"clusters.create.secretName": "Kubeconfig Secret 名称",
"clusters.create.secretKey": "Secret 中的 Key",
"clusters.create.prometheus": "Prometheus URL (可选)",
"clusters.create.description": "描述 (可选)",
"clusters.create.cancel": "取消",
"clusters.create.submit": "注册",
"clusters.setup.title": "如何准备 Kubeconfig Secret",
"clusters.setup.step1": "1. 准备远端集群的 kubeconfig 文件",
"clusters.setup.step1.desc": "可以从 ~/.kube/config 导出目标 context，或使用远端集群的 ServiceAccount Token 构造 kubeconfig",
"clusters.setup.step2": "2. 创建 Kubernetes Secret",
"clusters.setup.step2.cmd": "kubectl create secret generic <secret-name> -n kube-agent-helper --from-file=kubeconfig=/path/to/kubeconfig.yaml",
"clusters.setup.step3": "3. 在上方表单中填入 Secret 名称和 Key，点击注册",
"clusters.setup.step3.desc": "控制器会自动读取 Secret 中的 kubeconfig，建立与远端集群的连接。连接成功后状态显示为 Connected。",
"clusters.setup.sa.title": "推荐：使用 ServiceAccount Token（生产环境）",
"clusters.setup.sa.desc": "在远端集群创建只读 ServiceAccount，用其 Token 构造 kubeconfig，权限最小化且可控过期。"
```

In `dashboard/src/i18n/en.json`:

```json
"nav.clusters": "Clusters",
"clusters.title": "Cluster Management",
"clusters.loading": "Loading...",
"clusters.empty": "No remote clusters registered. Local cluster is always available.",
"clusters.col.name": "Cluster Name",
"clusters.col.phase": "Status",
"clusters.col.prometheus": "Prometheus",
"clusters.col.description": "Description",
"clusters.create.title": "Register Remote Cluster",
"clusters.create.name": "Cluster Name",
"clusters.create.namespace": "Namespace",
"clusters.create.secretName": "Kubeconfig Secret Name",
"clusters.create.secretKey": "Key in Secret",
"clusters.create.prometheus": "Prometheus URL (optional)",
"clusters.create.description": "Description (optional)",
"clusters.create.cancel": "Cancel",
"clusters.create.submit": "Register",
"clusters.setup.title": "How to Prepare the Kubeconfig Secret",
"clusters.setup.step1": "1. Prepare the kubeconfig file for the remote cluster",
"clusters.setup.step1.desc": "Export the target context from ~/.kube/config, or create a kubeconfig using a ServiceAccount token from the remote cluster.",
"clusters.setup.step2": "2. Create a Kubernetes Secret",
"clusters.setup.step2.cmd": "kubectl create secret generic <secret-name> -n kube-agent-helper --from-file=kubeconfig=/path/to/kubeconfig.yaml",
"clusters.setup.step3": "3. Fill in the Secret name and key in the form above, then click Register",
"clusters.setup.step3.desc": "The controller will read the kubeconfig from the Secret and establish a connection to the remote cluster. Status shows Connected on success.",
"clusters.setup.sa.title": "Recommended: Use ServiceAccount Token (production)",
"clusters.setup.sa.desc": "Create a read-only ServiceAccount in the remote cluster and build the kubeconfig with its token for minimal privileges and controlled expiration."
```

- [ ] **Step 2: Add `createClusterConfig` API function**

In `dashboard/src/lib/api.ts`:

```ts
export async function createClusterConfig(body: {
  name: string;
  namespace: string;
  secretName: string;
  secretKey: string;
  prometheusURL?: string;
  description?: string;
}) {
  const res = await fetch("/api/clusters", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}
```

- [ ] **Step 3: Add `POST /api/clusters` handler to HTTP server**

In `internal/controller/httpserver/server.go`, update `handleAPIClusters`:

```go
func (s *Server) handleAPIClusters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleAPIClustersGet(w, r)
	case http.MethodPost:
		s.handleAPIClustersPost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPIClustersPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string `json:"name"`
		Namespace     string `json:"namespace"`
		SecretName    string `json:"secretName"`
		SecretKey     string `json:"secretKey"`
		PrometheusURL string `json:"prometheusURL"`
		Description   string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.SecretName == "" || body.SecretKey == "" {
		http.Error(w, "name, secretName, secretKey are required", http.StatusBadRequest)
		return
	}
	if body.Namespace == "" {
		body.Namespace = "kube-agent-helper"
	}

	cc := &k8saiV1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      body.Name,
			Namespace: body.Namespace,
		},
		Spec: k8saiV1.ClusterConfigSpec{
			KubeConfigRef: k8saiV1.SecretKeyRef{
				Name: body.SecretName,
				Key:  body.SecretKey,
			},
			PrometheusURL: body.PrometheusURL,
			Description:   body.Description,
		},
	}
	if err := s.k8sClient.Create(r.Context(), cc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"name": cc.Name, "namespace": cc.Namespace})
}
```

- [ ] **Step 4: Create the clusters page with setup guide**

```tsx
// dashboard/src/app/clusters/page.tsx
"use client";

import { useState } from "react";
import { useI18n } from "@/i18n/context";
import { useClusterConfigs, createClusterConfig, useK8sNamespaces } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

function CreateDialog({ onClose }: { onClose: () => void }) {
  const { t } = useI18n();
  const { data: namespaces } = useK8sNamespaces();
  const [form, setForm] = useState({
    name: "",
    namespace: "kube-agent-helper",
    secretName: "",
    secretKey: "kubeconfig",
    prometheusURL: "",
    description: "",
  });
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await createClusterConfig({
        ...form,
        prometheusURL: form.prometheusURL || undefined,
        description: form.description || undefined,
      });
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed");
    } finally {
      setSubmitting(false);
    }
  };

  const inputClass =
    "w-full rounded border border-gray-300 bg-white px-3 py-1.5 text-sm dark:bg-gray-800 dark:border-gray-600 dark:text-gray-100";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl dark:bg-gray-900"
      >
        <h2 className="mb-4 text-lg font-semibold">{t("clusters.create.title")}</h2>
        {error && <p className="mb-3 text-sm text-red-500">{error}</p>}

        <div className="grid grid-cols-2 gap-3 mb-4">
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.name")}
            </label>
            <input
              className={inputClass}
              required
              placeholder="prod-cluster"
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.namespace")}
            </label>
            <select
              className={inputClass}
              value={form.namespace}
              onChange={(e) => setForm({ ...form, namespace: e.target.value })}
            >
              {(namespaces || []).map((ns) => (
                <option key={ns.name} value={ns.name}>{ns.name}</option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.secretName")}
            </label>
            <input
              className={inputClass}
              required
              placeholder="prod-kubeconfig"
              value={form.secretName}
              onChange={(e) => setForm({ ...form, secretName: e.target.value })}
            />
          </div>
          <div>
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.secretKey")}
            </label>
            <input
              className={inputClass}
              value={form.secretKey}
              onChange={(e) => setForm({ ...form, secretKey: e.target.value })}
            />
          </div>
          <div className="col-span-2">
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.prometheus")}
            </label>
            <input
              className={inputClass}
              placeholder="http://prometheus.monitoring:9090"
              value={form.prometheusURL}
              onChange={(e) => setForm({ ...form, prometheusURL: e.target.value })}
            />
          </div>
          <div className="col-span-2">
            <label className="block text-xs mb-1 text-gray-600 dark:text-gray-400">
              {t("clusters.create.description")}
            </label>
            <input
              className={inputClass}
              placeholder="Production cluster (us-east-1)"
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
            />
          </div>
        </div>

        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded px-4 py-1.5 text-sm text-gray-600 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
          >
            {t("clusters.create.cancel")}
          </button>
          <button
            type="submit"
            disabled={submitting || !form.name || !form.secretName}
            className="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {t("clusters.create.submit")}
          </button>
        </div>
      </form>
    </div>
  );
}

export default function ClustersPage() {
  const { t } = useI18n();
  const { data: clusters, isLoading, mutate } = useClusterConfigs();
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div className="space-y-8">
      {/* Cluster List */}
      <div>
        <div className="mb-6 flex items-center justify-between">
          <h1 className="text-2xl font-bold">{t("clusters.title")}</h1>
          <button
            onClick={() => setShowCreate(true)}
            className="rounded bg-blue-600 px-4 py-1.5 text-sm text-white hover:bg-blue-700"
          >
            + {t("clusters.create.title")}
          </button>
        </div>

        {isLoading && <p className="text-gray-500">{t("clusters.loading")}</p>}

        {!isLoading && (!clusters || clusters.length <= 1) && (
          <p className="text-gray-500">{t("clusters.empty")}</p>
        )}

        {clusters && clusters.length > 0 && (
          <div className="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 text-left text-xs font-medium text-gray-500 dark:bg-gray-800 dark:text-gray-400">
                <tr>
                  <th className="px-4 py-3">{t("clusters.col.name")}</th>
                  <th className="px-4 py-3">{t("clusters.col.phase")}</th>
                  <th className="px-4 py-3">{t("clusters.col.prometheus")}</th>
                  <th className="px-4 py-3">{t("clusters.col.description")}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {clusters.map((c) => (
                  <tr key={c.name} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                    <td className="px-4 py-3 font-medium">{c.name}</td>
                    <td className="px-4 py-3">
                      <Badge className={
                        c.phase === "Connected"
                          ? "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300"
                          : "bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300"
                      }>
                        {c.phase || "—"}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 text-xs font-mono text-gray-500">
                      {c.prometheusURL || <span className="italic text-gray-400">—</span>}
                    </td>
                    <td className="px-4 py-3 text-gray-500">{c.description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Setup Guide */}
      <Card>
        <CardHeader>
          <CardTitle>{t("clusters.setup.title")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <p className="font-medium text-sm">{t("clusters.setup.step1")}</p>
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("clusters.setup.step1.desc")}</p>
          </div>
          <div>
            <p className="font-medium text-sm">{t("clusters.setup.step2")}</p>
            <pre className="mt-1 overflow-x-auto rounded bg-gray-900 p-3 text-xs text-gray-100 dark:bg-gray-950">
              {t("clusters.setup.step2.cmd")}
            </pre>
          </div>
          <div>
            <p className="font-medium text-sm">{t("clusters.setup.step3")}</p>
            <p className="text-sm text-gray-500 dark:text-gray-400">{t("clusters.setup.step3.desc")}</p>
          </div>
          <div className="rounded-lg border border-blue-200 bg-blue-50 p-4 dark:border-blue-900 dark:bg-blue-950/30">
            <p className="font-medium text-sm text-blue-800 dark:text-blue-300">{t("clusters.setup.sa.title")}</p>
            <p className="text-sm text-blue-600 dark:text-blue-400 mt-1">{t("clusters.setup.sa.desc")}</p>
            <pre className="mt-2 overflow-x-auto rounded bg-gray-900 p-3 text-xs text-gray-100 dark:bg-gray-950">{`# On the REMOTE cluster:
kubectl create sa kah-reader -n kube-system
kubectl create clusterrolebinding kah-reader \\
  --clusterrole=view --serviceaccount=kube-system:kah-reader

TOKEN=$(kubectl create token kah-reader -n kube-system --duration=8760h)
CA=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
SERVER=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.server}')

cat > /tmp/remote-kubeconfig.yaml <<EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: \${CA}
    server: \${SERVER}
  name: remote
contexts:
- context: {cluster: remote, user: kah-reader}
  name: remote
current-context: remote
users:
- name: kah-reader
  user: {token: \${TOKEN}}
EOF

# Back on the LOCAL cluster:
kubectl create secret generic remote-kubeconfig \\
  -n kube-agent-helper \\
  --from-file=kubeconfig=/tmp/remote-kubeconfig.yaml`}</pre>
          </div>
        </CardContent>
      </Card>

      {showCreate && (
        <CreateDialog
          onClose={() => {
            setShowCreate(false);
            mutate();
          }}
        />
      )}
    </div>
  );
}
```

- [ ] **Step 5: Add nav link in `layout.tsx`**

In the `<div className="flex flex-1 gap-6 text-sm">` block, add before the About link:

```tsx
<Link href="/clusters" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.clusters")}</Link>
```

- [ ] **Step 6: Build and type-check**

```bash
cd dashboard && npm run build
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add dashboard/src/app/clusters/ \
        dashboard/src/lib/api.ts \
        dashboard/src/app/layout.tsx \
        dashboard/src/i18n/en.json \
        dashboard/src/i18n/zh.json \
        internal/controller/httpserver/server.go
git commit -m "feat(dashboard): ClusterConfig management page with setup guide"
```

---

## Task 10: User Documentation — CRD Guide, Examples, README

**Files:**
- Create: `docs/examples/clusterconfig/local-only.yaml`
- Create: `docs/examples/clusterconfig/remote-with-sa-token.yaml`
- Create: `docs/examples/diagnosticrun/cross-cluster-run.yaml`
- Modify: `docs/crd-user-guide.md`
- Modify: `README.md`
- Modify: `README_EN.md`

- [ ] **Step 1: Create example YAML — local-only (no-op reference)**

```yaml
# docs/examples/clusterconfig/local-only.yaml
# 本地集群无需 ClusterConfig — 默认可用。
# 当 DiagnosticRun.spec.clusterRef 为空时，诊断运行在本地集群。
#
# 此文件仅为参考，说明不指定 clusterRef 即为本地模式。
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: local-health-check
  namespace: kube-agent-helper
spec:
  target:
    scope: namespace
    namespaces:
      - default
  modelConfigRef: "anthropic-credentials"
  # clusterRef: ""  ← 省略或留空 = 本地集群
```

- [ ] **Step 2: Create example YAML — remote cluster with SA token**

```yaml
# docs/examples/clusterconfig/remote-with-sa-token.yaml
#
# 前置步骤：
#   1. 在远端集群创建只读 ServiceAccount:
#      kubectl create sa kah-reader -n kube-system
#      kubectl create clusterrolebinding kah-reader \
#        --clusterrole=view --serviceaccount=kube-system:kah-reader
#
#   2. 导出 kubeconfig:
#      TOKEN=$(kubectl create token kah-reader -n kube-system --duration=8760h)
#      # 用 TOKEN + CA + SERVER 构造 kubeconfig 文件（见 Dashboard 集群页面中的完整脚本）
#
#   3. 创建 Secret:
#      kubectl create secret generic prod-kubeconfig \
#        -n kube-agent-helper \
#        --from-file=kubeconfig=/tmp/prod-kubeconfig.yaml
#
---
apiVersion: k8sai.io/v1alpha1
kind: ClusterConfig
metadata:
  name: prod
  namespace: kube-agent-helper
spec:
  kubeConfigRef:
    name: prod-kubeconfig        # ← Secret 名称
    key: kubeconfig              # ← Secret 中存放 kubeconfig 的 key
  prometheusURL: "http://prometheus.monitoring:9090"   # 可选
  description: "Production cluster (us-east-1)"
```

- [ ] **Step 3: Create example YAML — cross-cluster DiagnosticRun**

```yaml
# docs/examples/diagnosticrun/cross-cluster-run.yaml
#
# 在远端集群 "prod" 上运行诊断（需先创建对应的 ClusterConfig）
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: prod-health-check
  namespace: kube-agent-helper
spec:
  clusterRef: "prod"             # ← 指向 ClusterConfig.metadata.name
  target:
    scope: namespace
    namespaces:
      - default
      - production
  modelConfigRef: "anthropic-credentials"
  outputLanguage: zh
```

- [ ] **Step 4: Update CRD user guide — add ClusterConfig section**

In `docs/crd-user-guide.md`, add a new section after the ModelConfig section. Locate the CRD table at the top and update it to include `ClusterConfig`:

```markdown
| CRD | 作用 | 你需要写吗？ |
|-----|------|-------------|
| `ModelConfig` | 配置 LLM 模型和 API Key | 一次性配置 |
| `ClusterConfig` | 注册远端集群（kubeconfig） | 多集群时配置 |
| `DiagnosticRun` | 触发一次诊断任务 | **每次诊断都要写** |
| `DiagnosticSkill` | 自定义诊断技能 | 可选（内置 10 个） |
| `DiagnosticFix` | 修复提案（系统自动生成） | 一般不需要手写 |
```

Then add the full ClusterConfig section:

```markdown
---

## 多集群诊断

### 概述

默认情况下，kube-agent-helper 诊断的是控制器所在的本地集群。如果你管理多个集群（如 staging + production），可以通过 `ClusterConfig` CRD 注册远端集群，然后在 `DiagnosticRun` 中通过 `spec.clusterRef` 指定目标集群。

```
用户 → ClusterConfig CR → Controller 读取 kubeconfig Secret → 建立远端连接
     → DiagnosticRun CR (clusterRef: prod) → Agent Job 在远端集群运行
```

### 第一步：准备远端集群的 kubeconfig

**方式 A：直接使用现有 kubeconfig 文件**

```bash
kubectl create secret generic prod-kubeconfig \
  -n kube-agent-helper \
  --from-file=kubeconfig=$HOME/.kube/prod-config
```

**方式 B：使用 ServiceAccount Token（推荐生产环境）**

在远端集群执行：

```bash
# 创建只读 ServiceAccount
kubectl create sa kah-reader -n kube-system
kubectl create clusterrolebinding kah-reader \
  --clusterrole=view --serviceaccount=kube-system:kah-reader

# 获取连接信息
TOKEN=$(kubectl create token kah-reader -n kube-system --duration=8760h)
CA=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
SERVER=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.server}')

# 构造 kubeconfig
cat > /tmp/prod-kubeconfig.yaml <<EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: ${CA}
    server: ${SERVER}
  name: prod
contexts:
- context:
    cluster: prod
    user: kah-reader
  name: prod
current-context: prod
users:
- name: kah-reader
  user:
    token: ${TOKEN}
EOF
```

回到本地集群：

```bash
kubectl create secret generic prod-kubeconfig \
  -n kube-agent-helper \
  --from-file=kubeconfig=/tmp/prod-kubeconfig.yaml
```

### 第二步：创建 ClusterConfig CR

```yaml
apiVersion: k8sai.io/v1alpha1
kind: ClusterConfig
metadata:
  name: prod
  namespace: kube-agent-helper
spec:
  kubeConfigRef:
    name: prod-kubeconfig     # Secret 名称
    key: kubeconfig           # Secret 中的 key
  prometheusURL: "http://prometheus.monitoring:9090"  # 可选
  description: "生产集群"
```

```bash
kubectl apply -f the-above.yaml
# 验证连接状态
kubectl get clusterconfig prod -n kube-agent-helper
# NAME   PHASE       AGE
# prod   Connected   10s
```

`Phase: Connected` 表示控制器已成功连接远端集群。如果显示 `Error`，检查 Secret 内容和网络可达性。

### 第三步：在 DiagnosticRun 中指定目标集群

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: prod-health-check
  namespace: kube-agent-helper
spec:
  clusterRef: "prod"           # ← 对应 ClusterConfig.metadata.name
  target:
    scope: namespace
    namespaces:
      - default
  modelConfigRef: "anthropic-credentials"
  outputLanguage: zh
```

省略 `clusterRef` 或留空 = 在本地集群运行（向后兼容）。

### Dashboard 集群管理

打开 Dashboard → **集群** 页面，可以：
- 查看已注册集群及连接状态
- 一键注册新集群（填入 Secret 名称即可）
- 查看完整的 kubeconfig 准备指南和脚本

在导航栏右侧的集群下拉框切换当前查看的集群，所有页面（Run 列表、事件、修复）自动按集群过滤。

### ClusterConfig 字段说明

| 字段 | 说明 |
|------|------|
| `spec.kubeConfigRef.name` | 包含 kubeconfig 的 Secret 名称 |
| `spec.kubeConfigRef.key` | Secret 中 kubeconfig 数据的 key |
| `spec.prometheusURL` | 远端集群的 Prometheus 端点（可选） |
| `spec.description` | 集群描述（显示在 Dashboard） |
| `status.phase` | 连接状态：`Connected` 或 `Error` |
| `status.message` | 错误信息（仅在 `Error` 时有值） |
```

- [ ] **Step 5: Update README.md — add multi-cluster to features and CRD table**

In the features list of `README.md`, add:

```markdown
- **多集群支持** — 通过 `ClusterConfig` CRD 注册远端集群，`spec.clusterRef` 指定诊断目标集群
```

Update the CRD table:

```markdown
| CRD | 用途 |
|-----|------|
| `DiagnosticRun` | 声明诊断任务（一次性或定时），控制器创建 Agent Job |
| `DiagnosticSkill` | 声明诊断技能（维度、Prompt、工具列表） |
| `ModelConfig` | LLM 提供商配置（API Key Secret 引用、`baseURL` 代理端点） |
| `DiagnosticFix` | 修复提案（patch 或新资源），含审批流程 |
| `ClusterConfig` | 远端集群注册（kubeconfig Secret 引用），状态显示连接状态 |
```

Change "4 个 CRD" to "5 个 CRD" in the features list and architecture description.

- [ ] **Step 6: Update README_EN.md — same changes in English**

Same updates as Step 5 but in English:

```markdown
- **Multi-cluster support** — register remote clusters via `ClusterConfig` CRD, use `spec.clusterRef` to target diagnostics
```

- [ ] **Step 7: Commit**

```bash
git add docs/examples/clusterconfig/ \
        docs/examples/diagnosticrun/cross-cluster-run.yaml \
        docs/crd-user-guide.md \
        README.md README_EN.md
git commit -m "docs: add multi-cluster setup guide, CRD user guide, and example YAMLs"
```

---

## Self-Review

**Spec coverage:**
- ✅ `ClusterConfig` CRD with kubeconfig secret ref → Task 4
- ✅ `ClusterClientRegistry` thread-safe map → Task 5
- ✅ `ClusterConfigReconciler` builds clients from secrets → Task 5
- ✅ `DiagnosticRun.spec.clusterRef` → Task 4, Task 6
- ✅ RunReconciler routes Jobs to target cluster → Task 6
- ✅ Store `cluster_name` column on all 5 tables → Tasks 1–3
- ✅ HTTP `?cluster=` filter on runs, fixes, events → Task 7
- ✅ `/api/clusters` endpoint (GET + POST) → Tasks 7, 9
- ✅ Dashboard nav dropdown → Task 8
- ✅ Dashboard API hooks pass cluster param → Task 8
- ✅ Dashboard ClusterConfig management page with setup guide → Task 9
- ✅ User documentation: CRD guide, example YAMLs, README updates → Task 10
- ✅ ServiceAccount Token setup guide (production recommended) → Tasks 9, 10
- ⚠️ FixReconciler cross-cluster apply — intentionally deferred (noted in scope)
- ⚠️ EventCollector for remote clusters — intentionally deferred

**Placeholder scan:** None found.

**Type consistency:**
- `ClusterClientRegistry.Get/Set/Delete` defined in Task 5, used in Tasks 5 and 6 ✅
- `store.ListOpts.ClusterName` defined in Task 2, used in Tasks 3 and 7 ✅
- `ClusterConfig` defined in Task 4, registered in Task 5, listed in Tasks 7/9 ✅
- `useCluster()` from `@/cluster/context` defined in Task 8 step 2, consumed in steps 6–7 ✅
- `createClusterConfig()` defined in Task 9 step 2, used in Task 9 step 4 ✅
- `ClusterItem` type used in both `useClusterConfigs` (Task 8) and clusters page (Task 9) ✅
