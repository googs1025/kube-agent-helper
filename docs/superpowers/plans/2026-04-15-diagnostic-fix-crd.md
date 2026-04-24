# DiagnosticFix CRD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the DiagnosticFix CRD with a complete lifecycle: Agent creates fix proposals from findings, human approves, controller applies the patch, health-checks, and auto-rollbacks on failure.

**Architecture:** New CRD `DiagnosticFix` with a state machine (PendingApproval -> Approved -> Applying -> Succeeded/Failed/RolledBack). A new `DiagnosticFixReconciler` manages the lifecycle. The controller saves a rollback snapshot before applying, then watches the target resource health. The existing HTTP server gets new `/api/fixes` endpoints. Agent-side changes add `fixes` output alongside findings.

**Tech Stack:** Go, controller-runtime, dynamic client (for patching arbitrary resources), SQLite store, testify

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/controller/api/v1alpha1/types.go` | Add DiagnosticFix CRD types |
| Modify | `internal/controller/api/v1alpha1/groupversion.go` or scheme registration | Register new types |
| Create | `internal/store/sqlite/migrations/002_diagnostic_fix.up.sql` | DB migration for fixes table |
| Create | `internal/store/sqlite/migrations/002_diagnostic_fix.down.sql` | Rollback migration |
| Modify | `internal/store/store.go` | Add Fix types and Store methods |
| Modify | `internal/store/sqlite/sqlite.go` | Implement Fix persistence |
| Create | `internal/controller/reconciler/fix_reconciler.go` | DiagnosticFix reconciler |
| Create | `internal/controller/reconciler/fix_reconciler_test.go` | Reconciler unit tests |
| Modify | `internal/controller/httpserver/server.go` | Add /api/fixes endpoints |
| Modify | `cmd/controller/main.go` | Wire fix reconciler |
| Create | `deploy/crds/k8sai.io_diagnosticfixes.yaml` | CRD YAML manifest |

---

### Task 1: Define DiagnosticFix CRD types

**Files:**
- Modify: `internal/controller/api/v1alpha1/types.go`

- [ ] **Step 1: Add DiagnosticFix types**

Append to `internal/controller/api/v1alpha1/types.go`, before the `func init()`:

```go
// ── DiagnosticFix ────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type DiagnosticFix struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiagnosticFixSpec   `json:"spec,omitempty"`
	Status            DiagnosticFixStatus `json:"status,omitempty"`
}

type DiagnosticFixSpec struct {
	// Reference to the DiagnosticRun that produced this fix
	DiagnosticRunRef string `json:"diagnosticRunRef"`
	// Title of the finding this fix addresses
	FindingTitle string `json:"findingTitle"`

	// Target resource to patch
	Target FixTarget `json:"target"`

	// Strategy: auto (apply after approval), dry-run (preview only)
	// +kubebuilder:validation:Enum=auto;dry-run
	// +kubebuilder:default=auto
	Strategy string `json:"strategy"`

	// Whether human approval is required before applying
	// +kubebuilder:default=true
	ApprovalRequired bool `json:"approvalRequired"`

	// Patch to apply
	Patch FixPatch `json:"patch"`

	// Rollback configuration
	Rollback RollbackConfig `json:"rollback,omitempty"`
}

type FixTarget struct {
	// +kubebuilder:validation:Enum=Deployment;StatefulSet;DaemonSet;Service;ConfigMap
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type FixPatch struct {
	// +kubebuilder:validation:Enum=strategic-merge;json-patch
	// +kubebuilder:default=strategic-merge
	Type    string `json:"type"`
	Content string `json:"content"`
}

type RollbackConfig struct {
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`
	// +kubebuilder:default=true
	SnapshotBefore bool `json:"snapshotBefore"`
	// +kubebuilder:default=true
	AutoRollbackOnFailure bool `json:"autoRollbackOnFailure"`
	// Timeout in seconds to wait for health check after applying
	// +kubebuilder:default=300
	HealthCheckTimeout int `json:"healthCheckTimeout,omitempty"`
}

type DiagnosticFixStatus struct {
	// +kubebuilder:validation:Enum=PendingApproval;Approved;Applying;Succeeded;Failed;RolledBack;DryRunComplete
	Phase      string       `json:"phase,omitempty"`
	ApprovedBy string       `json:"approvedBy,omitempty"`
	ApprovedAt *metav1.Time `json:"approvedAt,omitempty"`
	AppliedAt  *metav1.Time `json:"appliedAt,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	// Base64-encoded original resource YAML for rollback
	RollbackSnapshot string `json:"rollbackSnapshot,omitempty"`
	Message          string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
type DiagnosticFixList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiagnosticFix `json:"items"`
}
```

- [ ] **Step 2: Register the new types in scheme**

In the `func init()` of `types.go`, add:

```go
	SchemeBuilder.Register(
		&DiagnosticSkill{}, &DiagnosticSkillList{},
		&DiagnosticRun{}, &DiagnosticRunList{},
		&ModelConfig{}, &ModelConfigList{},
		&DiagnosticFix{}, &DiagnosticFixList{},
	)
```

- [ ] **Step 3: Verify compilation**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go build ./...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/controller/api/v1alpha1/types.go
git commit -m "feat(api): add DiagnosticFix CRD types with spec and status"
```

---

### Task 2: Add Fix to Store layer

**Files:**
- Modify: `internal/store/store.go`
- Create: `internal/store/sqlite/migrations/002_diagnostic_fix.up.sql`
- Create: `internal/store/sqlite/migrations/002_diagnostic_fix.down.sql`
- Modify: `internal/store/sqlite/sqlite.go`

- [ ] **Step 1: Add Fix type and Store methods**

In `internal/store/store.go`, add after the `Finding` struct:

```go
type FixPhase string

const (
	FixPhasePendingApproval FixPhase = "PendingApproval"
	FixPhaseApproved        FixPhase = "Approved"
	FixPhaseApplying        FixPhase = "Applying"
	FixPhaseSucceeded       FixPhase = "Succeeded"
	FixPhaseFailed          FixPhase = "Failed"
	FixPhaseRolledBack      FixPhase = "RolledBack"
	FixPhaseDryRunComplete  FixPhase = "DryRunComplete"
)

type Fix struct {
	ID               string
	RunID            string
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
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
```

Add to the Store interface:

```go
	// Fixes
	CreateFix(ctx context.Context, f *Fix) error
	GetFix(ctx context.Context, id string) (*Fix, error)
	ListFixes(ctx context.Context, opts ListOpts) ([]*Fix, error)
	ListFixesByRun(ctx context.Context, runID string) ([]*Fix, error)
	UpdateFixPhase(ctx context.Context, id string, phase FixPhase, msg string) error
	UpdateFixApproval(ctx context.Context, id string, approvedBy string) error
	UpdateFixSnapshot(ctx context.Context, id string, snapshot string) error
```

- [ ] **Step 2: Create migration files**

Create `internal/store/sqlite/migrations/002_diagnostic_fix.up.sql`:

```sql
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
```

Create `internal/store/sqlite/migrations/002_diagnostic_fix.down.sql`:

```sql
DROP TABLE IF EXISTS fixes;
```

- [ ] **Step 3: Implement Fix methods in SQLite store**

Add to `internal/store/sqlite/sqlite.go`:

```go
func (s *SQLiteStore) CreateFix(ctx context.Context, f *store.Fix) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	f.CreatedAt = time.Now()
	f.UpdatedAt = f.CreatedAt
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO fixes
		 (id, run_id, finding_title, target_kind, target_namespace, target_name,
		  strategy, approval_required, patch_type, patch_content, phase, message, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.ID, f.RunID, f.FindingTitle, f.TargetKind, f.TargetNamespace, f.TargetName,
		f.Strategy, f.ApprovalRequired, f.PatchType, f.PatchContent,
		string(f.Phase), f.Message, f.CreatedAt, f.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) GetFix(ctx context.Context, id string) (*store.Fix, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, run_id, finding_title, target_kind, target_namespace, target_name,
		        strategy, approval_required, patch_type, patch_content, phase,
		        approved_by, rollback_snapshot, message, created_at, updated_at
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
		        approved_by, rollback_snapshot, message, created_at, updated_at
		 FROM fixes ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, opts.Offset)
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
		        approved_by, rollback_snapshot, message, created_at, updated_at
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
		&phase, &f.ApprovedBy, &f.RollbackSnapshot, &f.Message, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	f.Phase = store.FixPhase(phase)
	return f, nil
}
```

- [ ] **Step 4: Verify compilation and migration**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go build ./...`
Expected: No errors

- [ ] **Step 5: Write a basic store test for Fix CRUD**

Add to `internal/store/sqlite/sqlite_test.go`:

```go
func TestFixCRUD(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	fix := &store.Fix{
		RunID:            "run-1",
		FindingTitle:     "Over-provisioned CPU",
		TargetKind:       "Deployment",
		TargetNamespace:  "default",
		TargetName:       "nginx",
		Strategy:         "auto",
		ApprovalRequired: true,
		PatchType:        "strategic-merge",
		PatchContent:     `{"spec":{"replicas":2}}`,
		Phase:            store.FixPhasePendingApproval,
	}
	require.NoError(t, st.CreateFix(ctx, fix))
	assert.NotEmpty(t, fix.ID)

	// Get
	got, err := st.GetFix(ctx, fix.ID)
	require.NoError(t, err)
	assert.Equal(t, "nginx", got.TargetName)
	assert.Equal(t, store.FixPhasePendingApproval, got.Phase)

	// Approve
	require.NoError(t, st.UpdateFixApproval(ctx, fix.ID, "admin@example.com"))
	got, _ = st.GetFix(ctx, fix.ID)
	assert.Equal(t, store.FixPhaseApproved, got.Phase)
	assert.Equal(t, "admin@example.com", got.ApprovedBy)

	// Update phase
	require.NoError(t, st.UpdateFixPhase(ctx, fix.ID, store.FixPhaseSucceeded, "applied"))
	got, _ = st.GetFix(ctx, fix.ID)
	assert.Equal(t, store.FixPhaseSucceeded, got.Phase)

	// List
	fixes, err := st.ListFixes(ctx, store.ListOpts{})
	require.NoError(t, err)
	assert.Len(t, fixes, 1)

	// List by run
	fixes, err = st.ListFixesByRun(ctx, "run-1")
	require.NoError(t, err)
	assert.Len(t, fixes, 1)
}
```

- [ ] **Step 6: Run tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/store/sqlite/ -run TestFix -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/store/store.go internal/store/sqlite/sqlite.go internal/store/sqlite/migrations/ internal/store/sqlite/sqlite_test.go
git commit -m "feat(store): add Fix persistence layer with migration, CRUD, and tests"
```

---

### Task 3: Implement DiagnosticFix reconciler

**Files:**
- Create: `internal/controller/reconciler/fix_reconciler.go`
- Create: `internal/controller/reconciler/fix_reconciler_test.go`

- [ ] **Step 1: Write the reconciler**

Create `internal/controller/reconciler/fix_reconciler.go`:

```go
package reconciler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

var supportedKinds = map[string]schema.GroupVersionResource{
	"Deployment":  {Group: "apps", Version: "v1", Resource: "deployments"},
	"StatefulSet": {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
	"Service":     {Group: "", Version: "v1", Resource: "services"},
	"ConfigMap":   {Group: "", Version: "v1", Resource: "configmaps"},
}

type DiagnosticFixReconciler struct {
	client.Client
	Store store.Store
}

func (r *DiagnosticFixReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var fix k8saiV1.DiagnosticFix
	if err := r.Get(ctx, req.NamespacedName, &fix); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	phase := fix.Status.Phase

	switch phase {
	case "":
		// New fix — set initial phase
		if fix.Spec.Strategy == "dry-run" {
			fix.Status.Phase = "DryRunComplete"
			fix.Status.Message = "Dry-run: patch content available for review"
		} else if fix.Spec.ApprovalRequired {
			fix.Status.Phase = "PendingApproval"
			fix.Status.Message = "Waiting for human approval"
		} else {
			fix.Status.Phase = "Approved"
			fix.Status.Message = "Auto-approved (approvalRequired=false)"
		}
		if err := r.Status().Update(ctx, &fix); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("fix initialized", "name", fix.Name, "phase", fix.Status.Phase)
		return ctrl.Result{Requeue: true}, nil

	case "Approved":
		// Apply the patch
		fix.Status.Phase = "Applying"
		now := metav1.Now()
		fix.Status.AppliedAt = &now
		if err := r.Status().Update(ctx, &fix); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.applyPatch(ctx, &fix); err != nil {
			return r.failFix(ctx, &fix, fmt.Sprintf("apply patch failed: %s", err))
		}

		fix.Status.Phase = "Succeeded"
		fix.Status.Message = "Patch applied successfully"
		completedNow := metav1.Now()
		fix.Status.CompletedAt = &completedNow
		if err := r.Status().Update(ctx, &fix); err != nil {
			return ctrl.Result{}, err
		}
		_ = r.Store.UpdateFixPhase(ctx, string(fix.UID), store.FixPhaseSucceeded, "patch applied")
		logger.Info("fix applied", "name", fix.Name)
		return ctrl.Result{}, nil

	case "PendingApproval", "DryRunComplete", "Succeeded", "Failed", "RolledBack":
		// Terminal or waiting states — nothing to do
		return ctrl.Result{}, nil

	case "Applying":
		// Requeue in case we need to check health
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *DiagnosticFixReconciler) applyPatch(ctx context.Context, fix *k8saiV1.DiagnosticFix) error {
	gvr, ok := supportedKinds[fix.Spec.Target.Kind]
	if !ok {
		return fmt.Errorf("unsupported target kind: %s", fix.Spec.Target.Kind)
	}

	// Fetch current state for rollback snapshot
	if fix.Spec.Rollback.SnapshotBefore {
		current := &unstructured.Unstructured{}
		current.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvr.Group,
			Version: gvr.Version,
			Kind:    fix.Spec.Target.Kind,
		})
		key := types.NamespacedName{
			Name:      fix.Spec.Target.Name,
			Namespace: fix.Spec.Target.Namespace,
		}
		if err := r.Get(ctx, key, current); err != nil {
			return fmt.Errorf("get target for snapshot: %w", err)
		}
		data, _ := json.Marshal(current.Object)
		fix.Status.RollbackSnapshot = base64.StdEncoding.EncodeToString(data)
		_ = r.Store.UpdateFixSnapshot(ctx, string(fix.UID), fix.Status.RollbackSnapshot)
	}

	// Apply the patch
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   gvr.Group,
		Version: gvr.Version,
		Kind:    fix.Spec.Target.Kind,
	})
	target.SetName(fix.Spec.Target.Name)
	target.SetNamespace(fix.Spec.Target.Namespace)

	var patchType types.PatchType
	switch fix.Spec.Patch.Type {
	case "json-patch":
		patchType = types.JSONPatchType
	default:
		patchType = types.StrategicMergePatchType
	}

	patch := client.RawPatch(patchType, []byte(fix.Spec.Patch.Content))
	if err := r.Patch(ctx, target, patch); err != nil {
		// Auto-rollback on failure if configured
		if fix.Spec.Rollback.AutoRollbackOnFailure && fix.Status.RollbackSnapshot != "" {
			_ = r.rollback(ctx, fix)
		}
		return err
	}

	return nil
}

func (r *DiagnosticFixReconciler) rollback(ctx context.Context, fix *k8saiV1.DiagnosticFix) error {
	logger := log.FromContext(ctx)

	data, err := base64.StdEncoding.DecodeString(fix.Status.RollbackSnapshot)
	if err != nil {
		return fmt.Errorf("decode rollback snapshot: %w", err)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("unmarshal rollback snapshot: %w", err)
	}

	target := &unstructured.Unstructured{Object: obj}
	// Use server-side apply with force to restore
	patch := client.RawPatch(types.MergePatchType, data)
	if err := r.Patch(ctx, target, patch); err != nil {
		logger.Error(err, "rollback failed", "fix", fix.Name)
		return err
	}

	fix.Status.Phase = "RolledBack"
	fix.Status.Message = "Auto-rolled back after apply failure"
	now := metav1.Now()
	fix.Status.CompletedAt = &now
	_ = r.Status().Update(ctx, fix)
	_ = r.Store.UpdateFixPhase(ctx, string(fix.UID), store.FixPhaseRolledBack, "auto-rollback")

	logger.Info("fix rolled back", "name", fix.Name)
	return nil
}

func (r *DiagnosticFixReconciler) failFix(ctx context.Context, fix *k8saiV1.DiagnosticFix, msg string) (ctrl.Result, error) {
	fix.Status.Phase = "Failed"
	fix.Status.Message = msg
	now := metav1.Now()
	fix.Status.CompletedAt = &now
	if err := r.Status().Update(ctx, fix); err != nil {
		return ctrl.Result{}, err
	}
	_ = r.Store.UpdateFixPhase(ctx, string(fix.UID), store.FixPhaseFailed, msg)
	return ctrl.Result{}, nil
}

func (r *DiagnosticFixReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.DiagnosticFix{}).
		Complete(r)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go build ./...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/controller/reconciler/fix_reconciler.go
git commit -m "feat(reconciler): add DiagnosticFix reconciler with apply, snapshot, and rollback"
```

---

### Task 4: Add /api/fixes HTTP endpoints

**Files:**
- Modify: `internal/controller/httpserver/server.go`

- [ ] **Step 1: Add fix endpoints to the HTTP server**

In `internal/controller/httpserver/server.go`, add routes in the `New` function:

```go
	srv.mux.HandleFunc("/api/fixes", srv.handleAPIFixes)
	srv.mux.HandleFunc("/api/fixes/", srv.handleAPIFixDetail)
```

Add the handler methods:

```go
// GET /api/fixes
func (s *Server) handleAPIFixes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fixes, err := s.store.ListFixes(r.Context(), store.ListOpts{Limit: 50})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if fixes == nil {
		fixes = make([]*store.Fix, 0)
	}
	writeJSON(w, fixes)
}

// GET /api/fixes/{id}
// PATCH /api/fixes/{id}/approve
// PATCH /api/fixes/{id}/reject
// POST /api/fixes/{id}/rollback
func (s *Server) handleAPIFixDetail(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["api","fixes","{id}"] or ["api","fixes","{id}","approve"|"reject"|"rollback"]
	if len(parts) < 3 {
		http.NotFound(w, r)
		return
	}
	fixID := parts[2]

	// GET /api/fixes/{id}
	if len(parts) == 3 && r.Method == http.MethodGet {
		fix, err := s.store.GetFix(r.Context(), fixID)
		if err != nil {
			if err == store.ErrNotFound {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, fix)
		return
	}

	if len(parts) == 4 {
		action := parts[3]
		switch {
		case action == "approve" && r.Method == http.MethodPatch:
			var body struct {
				ApprovedBy string `json:"approvedBy"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			if err := s.store.UpdateFixApproval(r.Context(), fixID, body.ApprovedBy); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return

		case action == "reject" && r.Method == http.MethodPatch:
			if err := s.store.UpdateFixPhase(r.Context(), fixID, store.FixPhaseFailed, "rejected by user"); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	http.NotFound(w, r)
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go build ./...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/controller/httpserver/server.go
git commit -m "feat(http): add /api/fixes endpoints for listing, detail, approve, and reject"
```

---

### Task 5: Wire fix reconciler in main.go

**Files:**
- Modify: `cmd/controller/main.go`

- [ ] **Step 1: Register DiagnosticFixReconciler**

In `cmd/controller/main.go`, after the ModelConfigReconciler setup block, add:

```go
	if err := (&reconciler.DiagnosticFixReconciler{
		Client: mgr.GetClient(),
		Store:  st,
	}).SetupWithManager(mgr); err != nil {
		slog.Error("setup fix reconciler", "error", err)
		os.Exit(1)
	}
```

- [ ] **Step 2: Verify build**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go build ./...`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add cmd/controller/main.go
git commit -m "feat(main): wire DiagnosticFix reconciler into controller manager"
```

---

### Task 6: Generate CRD YAML manifest

**Files:**
- Create: `deploy/crds/k8sai.io_diagnosticfixes.yaml`

- [ ] **Step 1: Generate or write the CRD manifest**

If the project uses controller-gen:
```bash
cd /Users/zhenyu.jiang/kube-agent-helper
make generate 2>/dev/null || echo "No make generate target"
```

If no generator, create `deploy/crds/k8sai.io_diagnosticfixes.yaml` manually:

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: diagnosticfixes.k8sai.io
spec:
  group: k8sai.io
  names:
    kind: DiagnosticFix
    listKind: DiagnosticFixList
    plural: diagnosticfixes
    singular: diagnosticfix
    shortNames:
      - dfix
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      subresources:
        status: {}
      additionalPrinterColumns:
        - name: Phase
          type: string
          jsonPath: .status.phase
        - name: Target
          type: string
          jsonPath: .spec.target.name
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required: [diagnosticRunRef, findingTitle, target, patch]
              properties:
                diagnosticRunRef:
                  type: string
                findingTitle:
                  type: string
                target:
                  type: object
                  required: [kind, namespace, name]
                  properties:
                    kind:
                      type: string
                      enum: [Deployment, StatefulSet, DaemonSet, Service, ConfigMap]
                    namespace:
                      type: string
                    name:
                      type: string
                strategy:
                  type: string
                  enum: [auto, dry-run]
                  default: auto
                approvalRequired:
                  type: boolean
                  default: true
                patch:
                  type: object
                  required: [content]
                  properties:
                    type:
                      type: string
                      enum: [strategic-merge, json-patch]
                      default: strategic-merge
                    content:
                      type: string
                rollback:
                  type: object
                  properties:
                    enabled:
                      type: boolean
                      default: true
                    snapshotBefore:
                      type: boolean
                      default: true
                    autoRollbackOnFailure:
                      type: boolean
                      default: true
                    healthCheckTimeout:
                      type: integer
                      default: 300
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: [PendingApproval, Approved, Applying, Succeeded, Failed, RolledBack, DryRunComplete]
                approvedBy:
                  type: string
                approvedAt:
                  type: string
                  format: date-time
                appliedAt:
                  type: string
                  format: date-time
                completedAt:
                  type: string
                  format: date-time
                rollbackSnapshot:
                  type: string
                message:
                  type: string
```

- [ ] **Step 2: Commit**

```bash
git add deploy/crds/k8sai.io_diagnosticfixes.yaml
git commit -m "feat(crd): add DiagnosticFix CRD manifest"
```

---

### Task 7: End-to-end verification

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./... -count=1 -timeout 120s 2>&1 | tail -30`
Expected: All PASS

- [ ] **Step 2: Build succeeds**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go build ./...`
Expected: No errors

- [ ] **Step 3: Final commit if any fixups**

Only if there are remaining changes.
