# Fix Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users click "Generate Fix" on any finding to spawn a short-lived agent Pod that produces a patch + explanation, create a DiagnosticFix CR, and display a Before/After diff in the dashboard.

**Architecture:** New `FixGeneratorTranslator` compiles finding+run into a simplified Job (reuses agent-runtime image with a new `runtime.fix_main` entrypoint). Job calls MCP `kubectl_get` once for the target resource, makes a single LLM call to produce a patch JSON, and POSTs back to `/internal/fixes` on the controller. The handler creates a DiagnosticFix CR + a store entry with the pre-patch YAML snapshot. Dashboard finding cards gain a "Generate Fix" button; Fix detail page gains a Before/After diff using `react-diff-viewer-continued`.

**Tech Stack:** Go (controller-runtime, sigs.k8s.io fake client), Python 3.12 (httpx, anthropic SDK), Next.js 16 App Router, SQLite migrations (golang-migrate/migrate), js-yaml + fast-json-patch + react-diff-viewer-continued (new npm deps).

---

### Task 1: Store layer — add FindingID + BeforeSnapshot

**Files:**
- Create: `internal/store/sqlite/migrations/003_fix_finding_snapshot.up.sql`
- Create: `internal/store/sqlite/migrations/003_fix_finding_snapshot.down.sql`
- Modify: `internal/store/store.go` (Fix struct)
- Modify: `internal/store/sqlite/sqlite.go` (CreateFix, GetFix, ListFixes, ListFixesByRun — column lists, scanFix)
- Modify: `internal/controller/reconciler/run_reconciler_test.go` (memStore — no-op stays no-op, no changes needed beyond interface match)
- Modify: `internal/controller/httpserver/server_test.go` (fakeStore — interface match)

- [ ] **Step 1: Create migration up SQL**

`internal/store/sqlite/migrations/003_fix_finding_snapshot.up.sql`:

```sql
ALTER TABLE fixes ADD COLUMN finding_id TEXT NOT NULL DEFAULT '';
ALTER TABLE fixes ADD COLUMN before_snapshot TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_fixes_finding_id ON fixes(finding_id);
```

- [ ] **Step 2: Create migration down SQL**

`internal/store/sqlite/migrations/003_fix_finding_snapshot.down.sql`:

```sql
DROP INDEX IF EXISTS idx_fixes_finding_id;
ALTER TABLE fixes DROP COLUMN before_snapshot;
ALTER TABLE fixes DROP COLUMN finding_id;
```

- [ ] **Step 3: Add fields to store.Fix struct**

In `internal/store/store.go`, locate `type Fix struct { ... }` and add two fields (alphabetical/logical placement — append after `Message`):

```go
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
	FindingID        string
	BeforeSnapshot   string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
```

- [ ] **Step 4: Update `CreateFix` to write the new columns**

In `internal/store/sqlite/sqlite.go`, replace the `CreateFix` method body:

```go
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
```

- [ ] **Step 5: Update `GetFix`, `ListFixes`, `ListFixesByRun` to SELECT the new columns**

In `internal/store/sqlite/sqlite.go`, find the three SELECT statements (in `GetFix`, `ListFixes`, `ListFixesByRun`) and add `finding_id, before_snapshot` before `created_at` in each:

```go
`SELECT id, run_id, finding_title, target_kind, target_namespace, target_name,
        strategy, approval_required, patch_type, patch_content, phase,
        approved_by, rollback_snapshot, message, finding_id, before_snapshot,
        created_at, updated_at
 FROM fixes ...`
```

All three SELECTs need the same change.

- [ ] **Step 6: Update `scanFix` to read the new columns**

Find `scanFix` in `internal/store/sqlite/sqlite.go`. It currently scans 16 columns. Adjust it to scan 18:

```go
func scanFix(s scanner) (*store.Fix, error) {
	var f store.Fix
	var phase string
	err := s.Scan(
		&f.ID, &f.RunID, &f.FindingTitle, &f.TargetKind, &f.TargetNamespace, &f.TargetName,
		&f.Strategy, &f.ApprovalRequired, &f.PatchType, &f.PatchContent,
		&phase, &f.ApprovedBy, &f.RollbackSnapshot, &f.Message,
		&f.FindingID, &f.BeforeSnapshot,
		&f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	f.Phase = store.FixPhase(phase)
	return &f, nil
}
```

(If `scanFix` has a slightly different shape in the actual file — e.g. uses an interface type name different from `scanner` — keep that shape and only change the field list.)

- [ ] **Step 7: Build to verify**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go build ./...`
Expected: Clean build.

- [ ] **Step 8: Run store tests (if any)**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/store/... -count=1`
Expected: Tests pass (existing tests should still pass since they don't touch the new columns).

- [ ] **Step 9: Commit**

```bash
git add internal/store/store.go internal/store/sqlite/
git commit -m "feat(store): add FindingID and BeforeSnapshot to Fix"
```

---

### Task 2: CRD — add FindingID to DiagnosticFix spec

**Files:**
- Modify: `internal/controller/api/v1alpha1/types.go`
- Modify: `deploy/helm/templates/crds/k8sai.io_diagnosticfixes.yaml`
- Modify: `deploy/crds/k8sai.io_diagnosticfixes.yaml` (if it exists as a duplicate)

- [ ] **Step 1: Add `FindingID` field to `DiagnosticFixSpec`**

In `internal/controller/api/v1alpha1/types.go`, find `type DiagnosticFixSpec struct { ... }` and add the field at the end:

```go
type DiagnosticFixSpec struct {
	DiagnosticRunRef string    `json:"diagnosticRunRef"`
	FindingTitle     string    `json:"findingTitle"`
	Target           FixTarget `json:"target"`
	// +kubebuilder:validation:Enum=auto;dry-run
	// +kubebuilder:default=auto
	Strategy string `json:"strategy"`
	// +kubebuilder:default=true
	ApprovalRequired bool           `json:"approvalRequired"`
	Patch            FixPatch       `json:"patch"`
	Rollback         RollbackConfig `json:"rollback,omitempty"`
	// +optional
	FindingID string `json:"findingID,omitempty"`
}
```

Preserve the exact indentation and existing comment markers from the file. Only add the last `// +optional` + `FindingID` lines.

- [ ] **Step 2: Add `findingID` to CRD YAML**

Open `deploy/helm/templates/crds/k8sai.io_diagnosticfixes.yaml`. Find the `spec.properties:` block. Add a `findingID` property (placement: append to the existing property list, matching indentation):

```yaml
              findingID:
                type: string
```

If the file `deploy/crds/k8sai.io_diagnosticfixes.yaml` also exists as an identical copy of the chart template, update it identically.

- [ ] **Step 3: Build to verify**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go build ./...`
Expected: Clean build. String field requires no deepcopy regeneration.

- [ ] **Step 4: Commit**

```bash
git add internal/controller/api/v1alpha1/types.go deploy/helm/templates/crds/k8sai.io_diagnosticfixes.yaml deploy/crds/k8sai.io_diagnosticfixes.yaml
git commit -m "feat(crd): add optional spec.findingID to DiagnosticFix"
```

(The `deploy/crds/...` path may or may not exist; if git complains it's not tracked, drop it from the `git add`.)

---

### Task 3: FixGeneratorTranslator — compile finding+run into a Job

**Files:**
- Create: `internal/controller/translator/fix_generator.go`
- Create: `internal/controller/translator/fix_generator_test.go`

- [ ] **Step 1: Write the failing test — minimal shape**

`internal/controller/translator/fix_generator_test.go`:

```go
package translator_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func TestFixGenerator_Compile_ProducesJob(t *testing.T) {
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run-abc", Namespace: "default", UID: "run-uid-1"},
		Spec: k8saiV1.DiagnosticRunSpec{
			ModelConfigRef: "creds",
			OutputLanguage: "zh",
		},
	}
	finding := &store.Finding{
		ID:                "finding-1",
		RunID:             "run-uid-1",
		Dimension:         "reliability",
		Severity:          "medium",
		Title:             "Dashboard Deployment has no health probes",
		Description:       "No readiness/liveness probes configured.",
		ResourceKind:      "Deployment",
		ResourceNamespace: "kube-agent-helper",
		ResourceName:      "kah-dashboard",
		Suggestion:        "Add readiness and liveness probes.",
	}

	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{
		AgentImage:    "kube-agent-helper/agent-runtime:dev",
		ControllerURL: "http://kah.kube-agent-helper.svc:8080",
	})

	job, err := fg.Compile(run, finding)
	assert.NoError(t, err)
	assert.NotNil(t, job)

	// Job spec invariants
	assert.Equal(t, "fix-gen-finding-1", job.Name)
	assert.Equal(t, "default", job.Namespace)
	assert.Equal(t, int32(0), *job.Spec.BackoffLimit)
	assert.Equal(t, int64(120), *job.Spec.ActiveDeadlineSeconds)
	assert.Equal(t, "run-run-abc", job.Spec.Template.Spec.ServiceAccountName)

	// Container
	c := job.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "kube-agent-helper/agent-runtime:dev", c.Image)
	assert.Equal(t, []string{"python", "-m", "runtime.fix_main"}, c.Command)

	envs := map[string]string{}
	for _, e := range c.Env {
		envs[e.Name] = e.Value
	}
	assert.Equal(t, "http://kah.kube-agent-helper.svc:8080", envs["CONTROLLER_URL"])
	assert.Equal(t, "zh", envs["OUTPUT_LANGUAGE"])
	assert.Contains(t, envs, "FIX_INPUT_JSON")

	// FIX_INPUT_JSON body is a valid JSON with the right keys
	var input map[string]any
	err = json.Unmarshal([]byte(envs["FIX_INPUT_JSON"]), &input)
	assert.NoError(t, err)
	assert.Equal(t, "finding-1", input["findingID"])
	assert.Equal(t, "run-uid-1", input["runID"])
	assert.Equal(t, "Dashboard Deployment has no health probes", input["title"])
	target, _ := input["target"].(map[string]any)
	assert.Equal(t, "Deployment", target["kind"])
	assert.Equal(t, "kube-agent-helper", target["namespace"])
	assert.Equal(t, "kah-dashboard", target["name"])
}

func TestFixGenerator_Compile_DefaultsOutputLanguageToEn(t *testing.T) {
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default", UID: "u"},
		Spec:       k8saiV1.DiagnosticRunSpec{ModelConfigRef: "creds"},
	}
	finding := &store.Finding{ID: "f", RunID: "u", Title: "t", ResourceKind: "Pod", ResourceNamespace: "default", ResourceName: "p"}
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "img", ControllerURL: "http://x"})
	job, err := fg.Compile(run, finding)
	assert.NoError(t, err)
	var lang string
	for _, e := range job.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "OUTPUT_LANGUAGE" {
			lang = e.Value
		}
	}
	assert.Equal(t, "en", lang, "default output language should be 'en'")
}

// Silence unused import warning if batchv1 isn't referenced directly elsewhere
var _ = batchv1.Job{}
```

- [ ] **Step 2: Run the test to confirm failure**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/translator/... -run TestFixGenerator -v`
Expected: FAIL — `NewFixGenerator` undefined.

- [ ] **Step 3: Create `internal/controller/translator/fix_generator.go`**

```go
package translator

import (
	"encoding/json"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// FixGeneratorConfig configures the short-lived Job that asks the LLM
// to propose a patch for a single finding.
type FixGeneratorConfig struct {
	AgentImage       string
	ControllerURL    string
	AnthropicBaseURL string
	Model            string
}

type FixGenerator struct {
	cfg FixGeneratorConfig
}

func NewFixGenerator(cfg FixGeneratorConfig) *FixGenerator {
	return &FixGenerator{cfg: cfg}
}

// Compile produces a single Kubernetes Job that runs the fix-generator
// entry point in the agent runtime container.
func (g *FixGenerator) Compile(run *k8saiV1.DiagnosticRun, finding *store.Finding) (*batchv1.Job, error) {
	if finding == nil || finding.ID == "" {
		return nil, fmt.Errorf("finding is required")
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}

	outputLang := run.Spec.OutputLanguage
	if outputLang == "" {
		outputLang = "en"
	}

	input := map[string]any{
		"findingID":   finding.ID,
		"runID":       finding.RunID,
		"title":       finding.Title,
		"description": finding.Description,
		"suggestion":  finding.Suggestion,
		"dimension":   finding.Dimension,
		"severity":    finding.Severity,
		"target": map[string]string{
			"kind":      finding.ResourceKind,
			"namespace": finding.ResourceNamespace,
			"name":      finding.ResourceName,
		},
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal fix input: %w", err)
	}

	backoff := int32(0)
	deadline := int64(120)
	ttl := int32(600)
	saName := fmt.Sprintf("run-%s", run.Name) // reuse per-run SA

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("fix-gen-%s", finding.ID),
			Namespace: run.Namespace,
			Labels: map[string]string{
				"finding-id": finding.ID,
				"run-id":     string(run.UID),
				"component":  "fix-generator",
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			BackoffLimit:            &backoff,
			ActiveDeadlineSeconds:   &deadline,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: saName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "fix-generator",
						Image:   g.cfg.AgentImage,
						Command: []string{"python", "-m", "runtime.fix_main"},
						Env: []corev1.EnvVar{
							{Name: "FIX_INPUT_JSON", Value: string(inputJSON)},
							{Name: "CONTROLLER_URL", Value: g.cfg.ControllerURL},
							{Name: "MCP_SERVER_PATH", Value: "/usr/local/bin/k8s-mcp-server"},
							{Name: "OUTPUT_LANGUAGE", Value: outputLang},
							{Name: "ANTHROPIC_BASE_URL", Value: g.cfg.AnthropicBaseURL},
							{Name: "MODEL", Value: g.cfg.Model},
							{
								Name: "ANTHROPIC_API_KEY",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeySelector: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: run.Spec.ModelConfigRef,
										},
										Key: "apiKey",
									},
								},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("50m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					}},
				},
			},
		},
	}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/translator/... -v -count=1`
Expected: All tests pass (existing + 2 new).

- [ ] **Step 5: Commit**

```bash
git add internal/controller/translator/fix_generator.go internal/controller/translator/fix_generator_test.go
git commit -m "feat(translator): add FixGenerator that compiles finding into a Job"
```

---

### Task 4: POST /api/findings/{id}/generate-fix handler (TDD)

**Files:**
- Modify: `internal/controller/httpserver/server.go`
- Modify: `internal/controller/httpserver/server_test.go`

- [ ] **Step 1: Add FixGenerator field to Server struct (wire-up)**

In `internal/controller/httpserver/server.go`, at the top where the `Server` struct is defined, add a new field and update `New`:

```go
type Server struct {
	store        store.Store
	k8sClient    client.Client
	fixGenerator *translator.FixGenerator
	mux          *http.ServeMux
}

func New(s store.Store, k8sClient client.Client, fg *translator.FixGenerator) *Server {
	srv := &Server{store: s, k8sClient: k8sClient, fixGenerator: fg, mux: http.NewServeMux()}
	srv.mux.HandleFunc("/internal/runs/", srv.handleInternal)
	srv.mux.HandleFunc("/internal/fixes", srv.handleInternalFixCallback)
	srv.mux.HandleFunc("/api/runs", srv.handleAPIRuns)
	srv.mux.HandleFunc("/api/runs/", srv.handleAPIRunDetail)
	srv.mux.HandleFunc("/api/skills", srv.handleAPISkills)
	srv.mux.HandleFunc("/api/fixes", srv.handleAPIFixes)
	srv.mux.HandleFunc("/api/fixes/", srv.handleAPIFixDetail)
	srv.mux.HandleFunc("/api/findings/", srv.handleAPIFindingAction)
	return srv
}
```

Add import: `"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"`.

- [ ] **Step 2: Update all callers of `httpserver.New(...)` to pass `nil` for the new arg (or a real FixGenerator)**

- `cmd/controller/main.go` — construct a `FixGenerator` via `translator.NewFixGenerator(...)` and pass it.
- `internal/controller/httpserver/server_test.go` — update every `httpserver.New(fs, fc)` call to `httpserver.New(fs, fc, nil)` (the `nil` means the handler returns 500 on generate-fix — OK for tests that don't exercise that path).

In `cmd/controller/main.go`, after the existing `tr := translator.New(...)` block, add:

```go
fg := translator.NewFixGenerator(translator.FixGeneratorConfig{
    AgentImage:       agentImage,
    ControllerURL:    controllerURL,
    AnthropicBaseURL: anthropicBaseURL,
    Model:            model,
})
```

Then change `httpserver.New(st, mgr.GetClient())` to `httpserver.New(st, mgr.GetClient(), fg)`.

- [ ] **Step 3: Write the failing test — happy path**

Append to `internal/controller/httpserver/server_test.go`:

```go
func TestGenerateFix_CreatesJob(t *testing.T) {
	// Seed the store with a run and a finding
	fs := &fakeStore{}
	run := &store.DiagnosticRun{ID: "run-uid-1", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhaseSucceeded}
	fs.runs = append(fs.runs, run)
	finding := &store.Finding{
		ID: "finding-1", RunID: "run-uid-1",
		Title: "Test finding", ResourceKind: "Deployment",
		ResourceNamespace: "ns", ResourceName: "nginx",
	}
	fs.findings["run-uid-1"] = []*store.Finding{finding}

	// fake K8s client to receive Job creation
	fc := newFakeK8sClient()

	// Seed a DiagnosticRun CR in the fake client so handler can look up spec
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run-abc", Namespace: "default", UID: "run-uid-1"},
		Spec:       v1alpha1.DiagnosticRunSpec{ModelConfigRef: "creds", OutputLanguage: "en"},
	}
	_ = fc.Create(context.Background(), cr)

	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{
		AgentImage: "agent:test", ControllerURL: "http://x",
	})
	srv := httpserver.New(fs, fc, fg)

	req := httptest.NewRequest(http.MethodPost, "/api/findings/finding-1/generate-fix", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	// The Job should now exist in the fake client
	var jobList batchv1.JobList
	err := fc.List(context.Background(), &jobList)
	assert.NoError(t, err)
	assert.Len(t, jobList.Items, 1)
	assert.Equal(t, "fix-gen-finding-1", jobList.Items[0].Name)
}

func TestGenerateFix_ReturnsExistingFix(t *testing.T) {
	fs := &fakeStore{}
	finding := &store.Finding{ID: "finding-2", RunID: "run-uid-1", Title: "t",
		ResourceKind: "Deployment", ResourceNamespace: "ns", ResourceName: "nginx"}
	fs.findings["run-uid-1"] = []*store.Finding{finding}
	fs.fixes = append(fs.fixes, &store.Fix{
		ID: "existing-fix-uid", RunID: "run-uid-1", FindingID: "finding-2", FindingTitle: "t",
	})
	fc := newFakeK8sClient()
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default", UID: "run-uid-1"},
		Spec:       v1alpha1.DiagnosticRunSpec{ModelConfigRef: "creds"},
	}
	_ = fc.Create(context.Background(), cr)
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	req := httptest.NewRequest(http.MethodPost, "/api/findings/finding-2/generate-fix", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"fixID":"existing-fix-uid"`)

	// No new Job should have been created
	var jobList batchv1.JobList
	_ = fc.List(context.Background(), &jobList)
	assert.Len(t, jobList.Items, 0)
}

func TestGenerateFix_FindingNotFound(t *testing.T) {
	fs := &fakeStore{}
	fc := newFakeK8sClient()
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	req := httptest.NewRequest(http.MethodPost, "/api/findings/does-not-exist/generate-fix", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

Add helpers to `fakeStore` (it may not expose `fixes` or `findings` as slice yet — inspect the file and adapt). If `fakeStore` is a struct with no-op methods, convert it to a small in-memory store:

```go
type fakeStore struct {
	runs     []*store.DiagnosticRun
	findings map[string][]*store.Finding
	skills   []*store.Skill
	fixes    []*store.Fix
}

func newFakeStore() *fakeStore {
	return &fakeStore{findings: map[string][]*store.Finding{}}
}
```

Update existing `fakeStore` methods so `CreateFinding` appends to `m.findings[f.RunID]`, `ListFindings` reads from it, `ListFixesByRun` filters `m.fixes`, `CreateFix` appends to `m.fixes`. If the existing file uses a different pattern, match the existing style.

- [ ] **Step 4: Run tests to confirm failures**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/... -run 'TestGenerateFix' -v`
Expected: FAIL — handler or method undefined.

- [ ] **Step 5: Implement the handler in `server.go`**

Add after `handleAPIFixDetail`:

```go
// POST /api/findings/{findingID}/generate-fix
func (s *Server) handleAPIFindingAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// expected: ["api", "findings", "{id}", "generate-fix"]
	if len(parts) != 4 || parts[3] != "generate-fix" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	findingID := parts[2]
	if findingID == "" {
		http.Error(w, "missing finding id", http.StatusBadRequest)
		return
	}
	if s.fixGenerator == nil {
		http.Error(w, "fix generator not configured", http.StatusInternalServerError)
		return
	}

	// 1. Find the finding in the store (scan all runs' findings — store has no
	//    global-by-id index; findings are small so this is fine).
	finding, err := s.findFindingByID(r.Context(), findingID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if finding == nil {
		http.NotFound(w, r)
		return
	}

	// 2. Idempotency: if a fix already exists, return it.
	fixes, err := s.store.ListFixesByRun(r.Context(), finding.RunID)
	if err == nil {
		for _, f := range fixes {
			if f.FindingID == findingID {
				writeJSON(w, map[string]string{"fixID": f.ID})
				return
			}
		}
	}

	// 3. Fetch DiagnosticRun CR (need namespace + spec.modelConfigRef)
	var runCR v1alpha1.DiagnosticRun
	if err := s.findRunByUID(r.Context(), finding.RunID, &runCR); err != nil {
		http.Error(w, "run CR not found: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. Compile Job
	job, err := s.fixGenerator.Compile(&runCR, finding)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.k8sClient.Create(r.Context(), job); err != nil {
		if errors.IsAlreadyExists(err) {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "already-generating"})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "generating"})
}

func (s *Server) findFindingByID(ctx context.Context, id string) (*store.Finding, error) {
	// Walk all recent runs' findings. Cheap for small clusters; future work:
	// add a ListFindingByID method to the store.
	runs, err := s.store.ListRuns(ctx, store.ListOpts{Limit: 200})
	if err != nil {
		return nil, err
	}
	for _, r := range runs {
		fs, err := s.store.ListFindings(ctx, r.ID)
		if err != nil {
			continue
		}
		for _, f := range fs {
			if f.ID == id {
				return f, nil
			}
		}
	}
	return nil, nil
}

func (s *Server) findRunByUID(ctx context.Context, uid string, out *v1alpha1.DiagnosticRun) error {
	var list v1alpha1.DiagnosticRunList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		return err
	}
	for _, r := range list.Items {
		if string(r.UID) == uid {
			*out = r
			return nil
		}
	}
	return fmt.Errorf("no DiagnosticRun with UID %s", uid)
}
```

Ensure these imports exist in the file:
- `"k8s.io/apimachinery/pkg/api/errors"` (for `errors.IsAlreadyExists`)

- [ ] **Step 6: Run tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/... -v -count=1`
Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/controller/httpserver/server.go internal/controller/httpserver/server_test.go cmd/controller/main.go
git commit -m "feat(httpserver): POST /api/findings/{id}/generate-fix spawns fix Job"
```

---

### Task 5: POST /internal/fixes callback handler (TDD)

**Files:**
- Modify: `internal/controller/httpserver/server.go`
- Modify: `internal/controller/httpserver/server_test.go`

- [ ] **Step 1: Write the failing test**

Append to `server_test.go`:

```go
func TestInternalFixes_CreatesCRAndStoreEntry(t *testing.T) {
	fs := newFakeStore()
	fc := newFakeK8sClient()
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	body := `{
	  "findingID":"f-1",
	  "diagnosticRunRef":"run-uid-1",
	  "findingTitle":"t",
	  "target":{"kind":"Deployment","namespace":"ns","name":"nginx"},
	  "patch":{"type":"strategic-merge","content":"{\"spec\":{\"replicas\":2}}"},
	  "beforeSnapshot":"YXBpVmVyc2lvbjogdjE=",
	  "explanation":"Scale up to reduce risk."
	}`
	req := httptest.NewRequest(http.MethodPost, "/internal/fixes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// CR exists in fake k8s client
	var cr v1alpha1.DiagnosticFixList
	_ = fc.List(context.Background(), &cr)
	assert.Len(t, cr.Items, 1)
	assert.Equal(t, "f-1", cr.Items[0].Spec.FindingID)
	assert.Equal(t, "run-uid-1", cr.Items[0].Spec.DiagnosticRunRef)
	assert.Equal(t, "Deployment", cr.Items[0].Spec.Target.Kind)
	assert.Equal(t, "strategic-merge", cr.Items[0].Spec.Patch.Type)

	// Store has matching entry
	assert.Len(t, fs.fixes, 1)
	assert.Equal(t, "f-1", fs.fixes[0].FindingID)
	assert.Equal(t, "YXBpVmVyc2lvbjogdjE=", fs.fixes[0].BeforeSnapshot)
	assert.Equal(t, "Scale up to reduce risk.", fs.fixes[0].Message)
}

func TestInternalFixes_RejectsMissingFields(t *testing.T) {
	fs := newFakeStore()
	fc := newFakeK8sClient()
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	body := `{"findingID":"f-1"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/fixes", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/... -run TestInternalFixes -v`
Expected: FAIL.

- [ ] **Step 3: Implement `handleInternalFixCallback`**

Add to `server.go`:

```go
// POST /internal/fixes — called by fix-generator Pod after producing a patch
func (s *Server) handleInternalFixCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		FindingID        string `json:"findingID"`
		DiagnosticRunRef string `json:"diagnosticRunRef"`
		FindingTitle     string `json:"findingTitle"`
		Target           struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"target"`
		Patch struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"patch"`
		BeforeSnapshot string `json:"beforeSnapshot"`
		Explanation    string `json:"explanation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.FindingID == "" || req.DiagnosticRunRef == "" ||
		req.Target.Kind == "" || req.Target.Namespace == "" || req.Target.Name == "" ||
		req.Patch.Content == "" {
		http.Error(w, "findingID, diagnosticRunRef, target{kind,namespace,name}, patch.content are required",
			http.StatusBadRequest)
		return
	}
	if req.Patch.Type == "" {
		req.Patch.Type = "strategic-merge"
	}

	// Create DiagnosticFix CR in the same namespace as the target.
	// (Fix CRs are namespaced; we use the target's namespace for locality.)
	name := fmt.Sprintf("fix-%s", req.FindingID)
	cr := &v1alpha1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: req.Target.Namespace,
		},
		Spec: v1alpha1.DiagnosticFixSpec{
			DiagnosticRunRef: req.DiagnosticRunRef,
			FindingTitle:     req.FindingTitle,
			FindingID:        req.FindingID,
			Target: v1alpha1.FixTarget{
				Kind:      req.Target.Kind,
				Namespace: req.Target.Namespace,
				Name:      req.Target.Name,
			},
			Strategy:         "dry-run",
			ApprovalRequired: true,
			Patch: v1alpha1.FixPatch{
				Type:    req.Patch.Type,
				Content: req.Patch.Content,
			},
			Rollback: v1alpha1.RollbackConfig{
				Enabled:               true,
				SnapshotBefore:        true,
				AutoRollbackOnFailure: true,
			},
		},
	}
	if err := s.k8sClient.Create(r.Context(), cr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Persist to store. Use the CR's UID so dashboard /api/fixes/{id} lines up.
	_ = s.store.CreateFix(r.Context(), &store.Fix{
		ID:               string(cr.UID),
		RunID:            req.DiagnosticRunRef,
		FindingID:        req.FindingID,
		FindingTitle:     req.FindingTitle,
		TargetKind:       req.Target.Kind,
		TargetNamespace: req.Target.Namespace,
		TargetName:      req.Target.Name,
		Strategy:         "dry-run",
		ApprovalRequired: true,
		PatchType:        req.Patch.Type,
		PatchContent:     req.Patch.Content,
		Phase:            store.FixPhasePendingApproval,
		Message:          req.Explanation,
		BeforeSnapshot:   req.BeforeSnapshot,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(cr)
}
```

Note: the existing `handleInternal` already handles `/internal/runs/{id}/findings`. The new `/internal/fixes` must be registered in the `New()` mux in Task 4 (already added above).

- [ ] **Step 4: Run tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/... -v -count=1`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/httpserver/server.go internal/controller/httpserver/server_test.go
git commit -m "feat(httpserver): POST /internal/fixes creates DiagnosticFix CR + store row"
```

---

### Task 6: GET /api/runs/{runID}/findings response includes fixID

**Files:**
- Modify: `internal/controller/httpserver/server.go`
- Modify: `internal/controller/httpserver/server_test.go`

- [ ] **Step 1: Write the failing test**

Append to `server_test.go`:

```go
func TestGetFindings_IncludesFixID(t *testing.T) {
	fs := newFakeStore()
	finding := &store.Finding{ID: "f-1", RunID: "run-1", Title: "t"}
	fs.findings["run-1"] = []*store.Finding{finding}
	fs.fixes = append(fs.fixes, &store.Fix{
		ID: "fix-uid-1", RunID: "run-1", FindingID: "f-1",
	})
	fc := newFakeK8sClient()
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-1/findings", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"FixID":"fix-uid-1"`)
}
```

- [ ] **Step 2: Run to confirm failure**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/... -run TestGetFindings_IncludesFixID -v`
Expected: FAIL — response doesn't include FixID.

- [ ] **Step 3: Modify the findings response**

Find the block in `server.go` that returns findings (inside `handleAPIRunDetail`, the `len(parts) == 4 && parts[3] == "findings"` branch). Replace it with:

```go
	if len(parts) == 4 && parts[3] == "findings" {
		findings, err := s.store.ListFindings(r.Context(), runID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if findings == nil {
			findings = make([]*store.Finding, 0)
		}
		// Join: map findingID -> fixID (if any)
		fixes, _ := s.store.ListFixesByRun(r.Context(), runID)
		fixByFinding := make(map[string]string, len(fixes))
		for _, f := range fixes {
			if f.FindingID != "" {
				fixByFinding[f.FindingID] = f.ID
			}
		}

		type findingWithFix struct {
			*store.Finding
			FixID string
		}
		out := make([]findingWithFix, 0, len(findings))
		for _, f := range findings {
			out = append(out, findingWithFix{Finding: f, FixID: fixByFinding[f.ID]})
		}
		writeJSON(w, out)
		return
	}
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/... -v -count=1`
Expected: All tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/httpserver/server.go internal/controller/httpserver/server_test.go
git commit -m "feat(httpserver): include FixID in findings response for UI linkage"
```

---

### Task 7: Extract MCP client helper from orchestrator.py

**Files:**
- Create: `agent-runtime/runtime/mcp_client.py`
- Modify: `agent-runtime/runtime/orchestrator.py`

- [ ] **Step 1: Create `agent-runtime/runtime/mcp_client.py`**

Copy the three private helpers from `orchestrator.py` (`_discover_tools`, `_mcp_to_anthropic_tool`, `_call_mcp_tool`) into a new module. Export them as public functions. New file content:

```python
"""MCP stdio client for the in-cluster k8s-mcp-server.

Extracted from orchestrator.py so fix_main.py can reuse the same helpers
without circular imports.
"""
import json
import os
import subprocess

MCP_SERVER_PATH = os.environ.get("MCP_SERVER_PATH", "/usr/local/bin/k8s-mcp-server")


def discover_tools() -> list:
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


def call_mcp_tool(name: str, args: dict) -> str:
    """Call a tool on k8s-mcp-server via MCP stdio protocol.

    Returns the raw text result string, or an error-prefixed string.
    """
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

- [ ] **Step 2: Update `orchestrator.py` to import from `mcp_client`**

In `agent-runtime/runtime/orchestrator.py`:
- Remove the three private helpers `_discover_tools`, `_mcp_to_anthropic_tool`, `_call_mcp_tool` (and the `MCP_SERVER_PATH` module-level constant if present).
- Add import: `from .mcp_client import discover_tools, call_mcp_tool`
- Replace call sites:
  - `tools = _discover_tools()` → `tools = discover_tools()`
  - `result = _call_mcp_tool(block["name"], block["input"])` → `result = call_mcp_tool(block["name"], block["input"])`

- [ ] **Step 3: Verify Python parses**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && python3 -c 'import ast; ast.parse(open("agent-runtime/runtime/orchestrator.py").read()); ast.parse(open("agent-runtime/runtime/mcp_client.py").read())'`
Expected: No output.

- [ ] **Step 4: Commit**

```bash
git add agent-runtime/runtime/mcp_client.py agent-runtime/runtime/orchestrator.py
git commit -m "refactor(agent): extract MCP client helpers into mcp_client.py"
```

---

### Task 8: Create `runtime/fix_main.py` entry point

**Files:**
- Create: `agent-runtime/runtime/fix_main.py`

- [ ] **Step 1: Create the file**

```python
"""Fix generator entry point — single LLM call to propose a patch for one finding.

Called as: python -m runtime.fix_main
Reads env var FIX_INPUT_JSON (finding + target), fetches current target YAML
via MCP, asks the LLM for a patch JSON, POSTs the result to the controller.
"""
import base64
import json
import os
import sys

import anthropic
import httpx

from .mcp_client import call_mcp_tool


CONTROLLER_URL = os.environ["CONTROLLER_URL"]
OUTPUT_LANG = os.environ.get("OUTPUT_LANGUAGE", "en")
MODEL = os.environ.get("MODEL", "claude-sonnet-4-6")


def main() -> int:
    finding = json.loads(os.environ["FIX_INPUT_JSON"])

    target = finding["target"]
    print(f"[info] generating fix for finding {finding['findingID']} on "
          f"{target['kind']}/{target['namespace']}/{target['name']}")

    # 1. Fetch current target resource via MCP
    raw = call_mcp_tool("kubectl_get", {
        "kind": target["kind"],
        "namespace": target["namespace"],
        "name": target["name"],
        "apiVersion": "",
    })
    if not raw or raw.startswith("tool error") or raw.startswith("{\"error\""):
        print(f"[error] failed to fetch target: {raw[:200]}", file=sys.stderr)
        return 1
    # raw is usually a JSON document (single object) from the MCP server.
    # Pretty-print it for display + as the "before" snapshot.
    try:
        obj = json.loads(raw)
        current_yaml = json.dumps(obj, indent=2)
    except json.JSONDecodeError:
        current_yaml = raw

    # 2. Single LLM call to produce patch
    prompt = build_prompt(finding, current_yaml, OUTPUT_LANG)
    client = anthropic.Anthropic()
    resp = client.messages.create(
        model=MODEL,
        max_tokens=2048,
        messages=[{"role": "user", "content": prompt}],
    )
    text = _extract_text(resp)
    try:
        parsed = parse_patch_json(text)
    except (json.JSONDecodeError, ValueError) as e:
        print(f"[error] invalid patch JSON from LLM: {e}\nraw:\n{text}", file=sys.stderr)
        return 2

    # 3. POST callback
    payload = {
        "findingID": finding["findingID"],
        "diagnosticRunRef": finding["runID"],
        "findingTitle": finding.get("title", ""),
        "target": target,
        "patch": {
            "type": parsed.get("type", "strategic-merge"),
            "content": parsed.get("content", ""),
        },
        "beforeSnapshot": base64.b64encode(current_yaml.encode("utf-8")).decode(),
        "explanation": parsed.get("explanation", ""),
    }
    r = httpx.post(f"{CONTROLLER_URL}/internal/fixes", json=payload, timeout=30)
    r.raise_for_status()
    print(f"[info] fix created: {r.json().get('metadata', {}).get('name', '')}")
    return 0


def build_prompt(finding: dict, current_yaml: str, lang: str) -> str:
    lang_clause = (
        "Write the `explanation` field in Simplified Chinese (简体中文)."
        if lang == "zh"
        else "Write the `explanation` field in English."
    )
    return f"""You are a Kubernetes fix suggestion generator.

## Finding
Title: {finding.get('title', '')}
Description: {finding.get('description', '')}
Suggestion: {finding.get('suggestion', '')}

## Current target resource (JSON)
```
{current_yaml}
```

## Instructions
Output a single JSON object with this exact schema:
{{"type": "strategic-merge" | "json-patch", "content": "<patch body as a JSON string>", "explanation": "<1-3 sentences>"}}

- Prefer strategic-merge for typical Deployment/StatefulSet/Service changes.
- Use json-patch only when the edit cannot be expressed as strategic-merge.
- The `content` field must itself be a valid JSON string (you are allowed to double-encode).
- {lang_clause}
- Output ONLY the JSON object. No prose, no code fences.
"""


def parse_patch_json(raw: str) -> dict:
    """Tolerate code fences and stray whitespace around the JSON body."""
    s = raw.strip()
    if s.startswith("```"):
        # strip leading fence (optionally with language tag) and trailing fence
        s = s.lstrip("`")
        if s.startswith("json"):
            s = s[4:]
        # drop trailing fence if present
        if s.endswith("```"):
            s = s[:-3]
    s = s.strip()
    result = json.loads(s)
    if not isinstance(result, dict):
        raise ValueError("expected JSON object")
    if "content" not in result:
        raise ValueError("missing 'content' field")
    return result


def _extract_text(response) -> str:
    """Concatenate all text blocks from an Anthropic Messages response."""
    out = []
    for block in response.content:
        if getattr(block, "type", "") == "text":
            out.append(block.text)
    return "".join(out)


if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 2: Verify Python parses**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && python3 -c 'import ast; ast.parse(open("agent-runtime/runtime/fix_main.py").read())'`
Expected: No output.

- [ ] **Step 3: Commit**

```bash
git add agent-runtime/runtime/fix_main.py
git commit -m "feat(agent): add fix_main.py entry point for fix generation"
```

---

### Task 9: Frontend — types + i18n for Fix generation

**Files:**
- Modify: `dashboard/src/lib/types.ts`
- Modify: `dashboard/src/lib/api.ts`
- Modify: `dashboard/src/i18n/zh.json`
- Modify: `dashboard/src/i18n/en.json`

- [ ] **Step 1: Update types**

In `dashboard/src/lib/types.ts`:

1. Add `FixID` to `Finding`:

```typescript
export interface Finding {
  ID: string;
  RunID: string;
  Dimension: string;
  Severity: "critical" | "high" | "medium" | "low";
  Title: string;
  Description: string;
  ResourceKind: string;
  ResourceNamespace: string;
  ResourceName: string;
  Suggestion: string;
  CreatedAt: string;
  FixID?: string;
}
```

2. Add `FindingID` and `BeforeSnapshot` to `Fix`:

```typescript
export interface Fix {
  ID: string;
  RunID: string;
  FindingID: string;
  FindingTitle: string;
  TargetKind: string;
  TargetNamespace: string;
  TargetName: string;
  Strategy: string;
  ApprovalRequired: boolean;
  PatchType: string;
  PatchContent: string;
  Phase: "PendingApproval" | "Approved" | "Applying" | "Succeeded" | "Failed" | "RolledBack" | "DryRunComplete";
  ApprovedBy: string;
  RollbackSnapshot: string;
  BeforeSnapshot: string;
  Message: string;
  CreatedAt: string;
  UpdatedAt: string;
}
```

- [ ] **Step 2: Add `generateFix` helper to `api.ts`**

Append to `dashboard/src/lib/api.ts`:

```typescript
export async function generateFix(findingID: string): Promise<{ fixID?: string; status?: string }> {
  const res = await fetch(`/api/findings/${findingID}/generate-fix`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
  return res.json();
}
```

- [ ] **Step 3: Add i18n keys**

In `dashboard/src/i18n/zh.json`, inside the existing `runs.findings` object, add:

```json
"generateFix": "生成修复建议",
"generating": "生成中...",
"viewFix": "查看修复建议"
```

Inside the existing `fixes.detail` object, add:

```json
"diffTitle": "资源变更预览",
"diffUnavailable": "json-patch 类型的预览暂不支持"
```

Mirror the additions in `dashboard/src/i18n/en.json`:

```json
// inside runs.findings:
"generateFix": "Generate Fix",
"generating": "Generating...",
"viewFix": "View Fix"

// inside fixes.detail:
"diffTitle": "Resource Change Preview",
"diffUnavailable": "Preview unavailable for json-patch"
```

- [ ] **Step 4: Build dashboard to verify**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Clean build.

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/lib/types.ts dashboard/src/lib/api.ts dashboard/src/i18n/
git commit -m "feat(dashboard): add Fix generation types, api helper, and i18n keys"
```

---

### Task 10: Finding card — Generate Fix / View Fix button

**Files:**
- Modify: `dashboard/src/app/runs/[id]/page.tsx`

- [ ] **Step 1: Add state and handler**

Open `dashboard/src/app/runs/[id]/page.tsx`. At the top of the `RunDetailPage` component body (after `const { data: findings, ... } = useFindings(id);`), add:

```tsx
const [generating, setGenerating] = useState<Record<string, boolean>>({});

async function handleGenerate(findingID: string) {
  setGenerating((g) => ({ ...g, [findingID]: true }));
  try {
    await generateFix(findingID);
    // SWR will pick up the new FixID on the next poll (5s); no explicit mutate needed
  } catch (err) {
    console.error("generateFix failed:", err);
  } finally {
    // Keep the "Generating..." label until SWR sees the new FixID
    setTimeout(() => setGenerating((g) => ({ ...g, [findingID]: false })), 60000);
  }
}
```

Update the imports at the top of the file:

```tsx
import { use, useState } from "react";
import Link from "next/link";
import { useRun, useFindings, generateFix } from "@/lib/api";
import { useI18n } from "@/i18n/context";
import { PhaseBadge } from "@/components/phase-badge";
import { SeverityBadge } from "@/components/severity-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
```

- [ ] **Step 2: Add button UI inside each finding Card**

Find the `<CardContent>` block inside the finding card rendering. After the existing `{f.Suggestion && ...}` block, add a new action row:

```tsx
<div className="mt-3 flex justify-end">
  {f.FixID ? (
    <Link href={`/fixes/${f.FixID}`} className="text-sm text-blue-600 hover:underline dark:text-blue-400">
      {t("runs.findings.viewFix")} →
    </Link>
  ) : (
    <Button size="sm" variant="outline" onClick={() => handleGenerate(f.ID)} disabled={generating[f.ID]}>
      {generating[f.ID] ? t("runs.findings.generating") : t("runs.findings.generateFix")}
    </Button>
  )}
</div>
```

- [ ] **Step 3: Build to verify**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Clean build.

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/app/runs/[id]/page.tsx
git commit -m "feat(dashboard): add Generate Fix / View Fix button on finding cards"
```

---

### Task 11: Install npm deps for diff viewer

**Files:**
- Modify: `dashboard/package.json`
- Modify: `dashboard/package-lock.json`

- [ ] **Step 1: Install the three packages**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm install js-yaml fast-json-patch react-diff-viewer-continued
npm install --save-dev @types/js-yaml
```

- [ ] **Step 2: Verify build still passes**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add dashboard/package.json dashboard/package-lock.json
git commit -m "feat(dashboard): add js-yaml, fast-json-patch, react-diff-viewer-continued"
```

---

### Task 12: ResourceDiff component + Fix detail page integration

**Files:**
- Create: `dashboard/src/components/resource-diff.tsx`
- Modify: `dashboard/src/app/fixes/[id]/page.tsx`

- [ ] **Step 1: Create `ResourceDiff` component**

`dashboard/src/components/resource-diff.tsx`:

```tsx
"use client";

import ReactDiffViewer, { DiffMethod } from "react-diff-viewer-continued";
import { useTheme } from "@/theme/context";

interface Props {
  before: string; // raw YAML or JSON text
  after: string;  // raw YAML or JSON text
}

export function ResourceDiff({ before, after }: Props) {
  const { theme } = useTheme();
  return (
    <ReactDiffViewer
      oldValue={before}
      newValue={after}
      splitView
      useDarkTheme={theme === "dark"}
      compareMethod={DiffMethod.LINES}
    />
  );
}
```

- [ ] **Step 2: Add a `computeAfter` helper — apply strategic-merge patch client-side**

Add to `dashboard/src/lib/utils.ts` (append to the file):

```typescript
import jsYaml from "js-yaml";
import * as jsonpatch from "fast-json-patch";

/**
 * Apply a Fix's patch client-side to the beforeSnapshot for preview.
 * Returns a pretty-printed JSON string, or empty string on failure.
 */
export function computeAfter(
  beforeSnapshot: string,
  patchType: string,
  patchContent: string,
): string {
  if (!beforeSnapshot) return "";
  let beforeObj: unknown;
  try {
    const decoded = atob(beforeSnapshot);
    // beforeSnapshot is base64-encoded JSON (pretty-printed) or YAML
    try {
      beforeObj = JSON.parse(decoded);
    } catch {
      beforeObj = jsYaml.load(decoded);
    }
  } catch {
    return "";
  }

  let afterObj: unknown;
  try {
    if (patchType === "json-patch") {
      const ops = JSON.parse(patchContent);
      afterObj = jsonpatch.applyPatch(
        JSON.parse(JSON.stringify(beforeObj)),
        ops,
      ).newDocument;
    } else {
      // strategic-merge: deep-merge patch into before
      const patchObj = JSON.parse(patchContent);
      afterObj = deepMerge(
        JSON.parse(JSON.stringify(beforeObj)),
        patchObj,
      );
    }
  } catch {
    return "";
  }

  return JSON.stringify(afterObj, null, 2);
}

export function decodeBefore(beforeSnapshot: string): string {
  if (!beforeSnapshot) return "";
  try {
    const decoded = atob(beforeSnapshot);
    try {
      // Re-pretty-print if JSON
      return JSON.stringify(JSON.parse(decoded), null, 2);
    } catch {
      return decoded;
    }
  } catch {
    return "";
  }
}

function deepMerge<T extends Record<string, unknown>>(a: T, b: T): T {
  for (const key of Object.keys(b)) {
    const aVal = (a as Record<string, unknown>)[key];
    const bVal = (b as Record<string, unknown>)[key];
    if (
      aVal && bVal &&
      typeof aVal === "object" && typeof bVal === "object" &&
      !Array.isArray(aVal) && !Array.isArray(bVal)
    ) {
      (a as Record<string, unknown>)[key] = deepMerge(
        aVal as Record<string, unknown>,
        bVal as Record<string, unknown>,
      );
    } else {
      (a as Record<string, unknown>)[key] = bVal;
    }
  }
  return a;
}
```

If `dashboard/src/lib/utils.ts` already has `cn()` from shadcn and lacks the imports, add them at the top. Keep the existing `cn()` export intact.

- [ ] **Step 3: Add Before/After card to Fix detail page**

Open `dashboard/src/app/fixes/[id]/page.tsx`. Add imports:

```tsx
import { ResourceDiff } from "@/components/resource-diff";
import { computeAfter, decodeBefore } from "@/lib/utils";
```

Above the existing `<Card>` that renders "Patch Content", add:

```tsx
{fix.BeforeSnapshot && (
  <Card className="mb-4">
    <CardHeader>
      <CardTitle className="text-base">{t("fixes.detail.diffTitle")}</CardTitle>
    </CardHeader>
    <CardContent>
      {(() => {
        if (fix.PatchType === "json-patch") {
          // Preview unavailable for json-patch in v1
          return <p className="text-sm text-gray-500 dark:text-gray-400">{t("fixes.detail.diffUnavailable")}</p>;
        }
        const before = decodeBefore(fix.BeforeSnapshot);
        const after = computeAfter(fix.BeforeSnapshot, fix.PatchType, fix.PatchContent);
        if (!after) {
          return <p className="text-sm text-gray-500 dark:text-gray-400">{t("fixes.detail.diffUnavailable")}</p>;
        }
        return <ResourceDiff before={before} after={after} />;
      })()}
    </CardContent>
  </Card>
)}
```

- [ ] **Step 4: Build to verify**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Clean build.

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/components/resource-diff.tsx dashboard/src/lib/utils.ts dashboard/src/app/fixes/[id]/page.tsx
git commit -m "feat(dashboard): add Before/After diff viewer on Fix detail page"
```

---

### Task 13: Rebuild + deploy + end-to-end verify

- [ ] **Step 1: Build controller image**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
eval $(minikube docker-env)
docker build -t kube-agent-helper/controller:dev -f Dockerfile .
```

- [ ] **Step 2: Build agent-runtime image (includes new fix_main.py)**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
eval $(minikube docker-env)
docker build -t kube-agent-helper/agent-runtime:dev -f agent-runtime/Dockerfile .
```

- [ ] **Step 3: Build dashboard image**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
eval $(minikube docker-env)
docker build -t kube-agent-helper/dashboard:dev -f dashboard/Dockerfile dashboard/
```

- [ ] **Step 4: Apply updated CRDs**

```bash
kubectl apply -f /Users/zhenyu.jiang/kube-agent-helper/deploy/helm/templates/crds/k8sai.io_diagnosticfixes.yaml
```

- [ ] **Step 5: Rollout restart controller + dashboard**

```bash
kubectl rollout restart deploy/kah-controller deploy/kah-dashboard -n kube-agent-helper
kubectl rollout status deploy/kah-controller -n kube-agent-helper --timeout=60s
kubectl rollout status deploy/kah-dashboard -n kube-agent-helper --timeout=60s
```

- [ ] **Step 6: Re-establish port-forward**

```bash
kill $(pgrep -f 'port-forward svc/kah') 2>/dev/null
nohup kubectl port-forward svc/kah -n kube-agent-helper 8080:8080 &>/tmp/port-forward.log &
sleep 3
```

- [ ] **Step 7: End-to-end test**

1. Open `http://localhost:3000` → navigate to a completed Run with findings
2. Click "生成修复建议" on one finding
3. Within 30-60 seconds the button should flip to "查看修复建议"
4. Click it → Fix detail page renders with:
   - Phase: `待审批`
   - "资源变更预览" card showing Before/After diff
   - "补丁内容" card with raw patch
5. Verify in cluster: `kubectl get diagnosticfix -n <target-ns>` shows the new Fix CR
6. Verify via API: `curl -s http://localhost:8080/api/fixes | python3 -m json.tool` shows the Fix with `BeforeSnapshot` populated

Expected: all steps succeed, diff highlights the changed lines.

---

## Self-Review

**Spec coverage:**
- §1 Overview → overall plan
- §2 User flow → Tasks 4, 5, 10 (trigger), 8 (Pod), 10 (polling for FixID), 12 (detail page)
- §3.1 DiagnosticFix spec FindingID → Task 2
- §3.2 BeforeSnapshot in store-only → Tasks 1 (store) and 5 (callback persists it)
- §3.3 Store FindingID + BeforeSnapshot → Task 1
- §3.4 CRD YAML → Task 2
- §4 Finding → Fix linkage (FixID in findings response) → Task 6
- §5.1 FixGeneratorTranslator → Task 3
- §5.2 HTTP endpoints (generate-fix, /internal/fixes) → Tasks 4 + 5
- §5.3 Reconciler unchanged → (no task — by design)
- §6 Agent runtime fix_main.py → Task 8 (also Task 7 for mcp_client extraction)
- §7.1 Finding card button → Task 10
- §7.2 Before/After diff block → Task 12
- §7.3 i18n keys → Task 9
- §8 Data flow summary → covered by Tasks 4–12 collectively
- §9 Error handling → handled inline in Task 4 (finding not found, existing fix), Task 5 (invalid callback body), Task 8 (MCP failure, LLM parse failure)
- §10 Non-goals → explicitly not implemented (patch editing, json-patch preview, batch, retry UI)
- §11 File structure → all files touched are in task "Files:" sections
- §12 Testing → Tasks 3, 4, 5, 6 have TDD; Task 13 has manual QA

**Type consistency check:**
- `FixGeneratorConfig` and `NewFixGenerator` — defined in Task 3, used in Task 4
- `generateFix()` — signature in Task 9 (`findingID → Promise<{fixID?, status?}>`), consumed in Task 10 (`handleGenerate(findingID)` calls it and doesn't rely on return value — OK)
- `Finding.FixID?` — added in Task 9, populated server-side in Task 6, read in Task 10
- `Fix.FindingID` and `Fix.BeforeSnapshot` — added in Task 1 (store) and Task 9 (TS), populated in Task 5 (callback), read in Task 12 (diff)
- `computeAfter` and `decodeBefore` — defined in Task 12, used in Task 12
- CRD `spec.findingID` — added in Task 2, set in Task 5 callback

All consistent.

**Placeholder scan:**
None found. Every step has concrete content or a concrete command. No TBD / TODO / "similar to Task N".
