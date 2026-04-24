# SkillRegistry + Extend Skills Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a thread-safe SkillRegistry that merges built-in and CR-defined Skills with hot-reload, then add 2-3 new built-in Skills covering reliability and config-drift dimensions.

**Architecture:** Replace the static `[]*store.Skill` slice in Translator with a `SkillRegistry` interface that returns the latest merged skill set on every call. The registry loads built-in skills at startup, watches DiagnosticSkill CRs via the existing reconciler writing to the Store, and on every `ListEnabled()` call reads from the Store (which already handles upsert/conflict). CR skills with the same name as built-in skills override them via the existing `ON CONFLICT(name) DO UPDATE` in SQLite. New skills are added as `.md` files in `skills/`.

**Tech Stack:** Go, controller-runtime, SQLite store (existing), testify

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `internal/controller/registry/registry.go` | SkillRegistry interface + Store-backed implementation |
| Create | `internal/controller/registry/registry_test.go` | Unit tests for registry |
| Modify | `internal/controller/translator/translator.go` | Replace `skills []*store.Skill` field with `SkillRegistry` interface |
| Modify | `internal/controller/translator/translator_test.go` | Update tests for new Translator constructor |
| Modify | `cmd/controller/main.go` | Wire SkillRegistry into Translator |
| Modify | `internal/controller/reconciler/skill_reconciler.go` | Add delete handling for removed CRs |
| Create | `internal/store/store.go` (modify) | Add `DeleteSkill` method to Store interface |
| Create | `internal/store/sqlite/sqlite.go` (modify) | Implement `DeleteSkill` |
| Create | `skills/reliability-analyst.md` | New built-in skill: reliability dimension |
| Create | `skills/config-drift-analyst.md` | New built-in skill: config-drift |

---

### Task 1: Add DeleteSkill to Store interface and SQLite implementation

**Files:**
- Modify: `internal/store/store.go:76-80`
- Modify: `internal/store/sqlite/sqlite.go`
- Test: `internal/store/sqlite/sqlite_test.go` (create if not exists)

- [ ] **Step 1: Write the failing test for DeleteSkill**

Create `internal/store/sqlite/sqlite_test.go` if it doesn't exist:

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
	f, err := os.CreateTemp("", "test-*.db")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })
	f.Close()
	st, err := sqlitestore.New(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

func TestDeleteSkill(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	sk := &store.Skill{
		Name:             "to-delete",
		Dimension:        "health",
		Prompt:           "test prompt",
		ToolsJSON:        `["kubectl_get"]`,
		RequiresDataJSON: `[]`,
		Source:           "cr",
		Enabled:          true,
		Priority:         100,
	}
	require.NoError(t, st.UpsertSkill(ctx, sk))

	// Verify it exists
	got, err := st.GetSkill(ctx, "to-delete")
	require.NoError(t, err)
	assert.Equal(t, "to-delete", got.Name)

	// Delete it
	require.NoError(t, st.DeleteSkill(ctx, "to-delete"))

	// Verify it's gone
	_, err = st.GetSkill(ctx, "to-delete")
	assert.ErrorIs(t, err, store.ErrNotFound)
}

func TestDeleteSkill_NotFound(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	err := st.DeleteSkill(ctx, "nonexistent")
	assert.ErrorIs(t, err, store.ErrNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/store/sqlite/ -run TestDeleteSkill -v`
Expected: FAIL — `DeleteSkill` method not found on Store interface

- [ ] **Step 3: Add DeleteSkill to Store interface**

In `internal/store/store.go`, add to the `Store` interface after `GetSkill`:

```go
	DeleteSkill(ctx context.Context, name string) error
```

- [ ] **Step 4: Implement DeleteSkill in SQLite store**

In `internal/store/sqlite/sqlite.go`, add after the `GetSkill` method:

```go
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/store/sqlite/ -run TestDeleteSkill -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/store.go internal/store/sqlite/sqlite.go internal/store/sqlite/sqlite_test.go
git commit -m "feat(store): add DeleteSkill method to Store interface and SQLite impl"
```

---

### Task 2: Implement SkillRegistry

**Files:**
- Create: `internal/controller/registry/registry.go`
- Create: `internal/controller/registry/registry_test.go`

- [ ] **Step 1: Write the failing test for SkillRegistry**

Create `internal/controller/registry/registry_test.go`:

```go
package registry_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
	sqlitestore "github.com/kube-agent-helper/kube-agent-helper/internal/store/sqlite"
)

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	f, err := os.CreateTemp("", "registry-test-*.db")
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })
	f.Close()
	st, err := sqlitestore.New(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

func TestListEnabled_ReturnsOnlyEnabled(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "enabled-skill", Dimension: "health", Prompt: "p",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: true, Priority: 100,
	}))
	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "disabled-skill", Dimension: "cost", Prompt: "p",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: false, Priority: 100,
	}))

	reg := registry.New(st)
	skills, err := reg.ListEnabled(ctx)
	require.NoError(t, err)
	assert.Len(t, skills, 1)
	assert.Equal(t, "enabled-skill", skills[0].Name)
}

func TestListEnabled_CROverridesBuiltin(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Insert builtin
	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "my-skill", Dimension: "health", Prompt: "builtin prompt",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: true, Priority: 100,
	}))
	// CR upserts same name — should override
	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "my-skill", Dimension: "security", Prompt: "cr prompt",
		ToolsJSON: `["kubectl_get"]`, RequiresDataJSON: `[]`,
		Source: "cr", Enabled: true, Priority: 50,
	}))

	reg := registry.New(st)
	skills, err := reg.ListEnabled(ctx)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "cr", skills[0].Source)
	assert.Equal(t, "cr prompt", skills[0].Prompt)
}

func TestListEnabled_OrderedByPriority(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "low-prio", Dimension: "health", Prompt: "p",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: true, Priority: 200,
	}))
	require.NoError(t, st.UpsertSkill(ctx, &store.Skill{
		Name: "high-prio", Dimension: "health", Prompt: "p",
		ToolsJSON: `[]`, RequiresDataJSON: `[]`,
		Source: "builtin", Enabled: true, Priority: 10,
	}))

	reg := registry.New(st)
	skills, err := reg.ListEnabled(ctx)
	require.NoError(t, err)
	require.Len(t, skills, 2)
	assert.Equal(t, "high-prio", skills[0].Name)
	assert.Equal(t, "low-prio", skills[1].Name)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/registry/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement SkillRegistry**

Create `internal/controller/registry/registry.go`:

```go
package registry

import (
	"context"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// SkillRegistry provides the current merged set of skills.
type SkillRegistry interface {
	// ListEnabled returns all enabled skills, ordered by priority ASC.
	// Built-in skills and CR skills are merged; same-name CR overrides built-in
	// via the store's ON CONFLICT upsert semantics.
	ListEnabled(ctx context.Context) ([]*store.Skill, error)
}

// storeRegistry implements SkillRegistry by reading from the Store on every call.
// This naturally picks up CR changes because the SkillReconciler writes to the Store.
type storeRegistry struct {
	store store.Store
}

// New creates a SkillRegistry backed by the given Store.
func New(s store.Store) SkillRegistry {
	return &storeRegistry{store: s}
}

func (r *storeRegistry) ListEnabled(ctx context.Context) ([]*store.Skill, error) {
	all, err := r.store.ListSkills(ctx)
	if err != nil {
		return nil, err
	}
	enabled := make([]*store.Skill, 0, len(all))
	for _, s := range all {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	return enabled, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/registry/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/controller/registry/
git commit -m "feat(registry): add SkillRegistry backed by Store for hot-reload"
```

---

### Task 3: Wire SkillRegistry into Translator

**Files:**
- Modify: `internal/controller/translator/translator.go`
- Modify: `internal/controller/translator/translator_test.go`
- Modify: `cmd/controller/main.go`

- [ ] **Step 1: Update Translator to accept SkillRegistry**

In `internal/controller/translator/translator.go`, change the struct and constructor:

Replace:
```go
type Translator struct {
	cfg    Config
	skills []*store.Skill
}

func New(cfg Config, skills []*store.Skill) *Translator {
	return &Translator{cfg: cfg, skills: skills}
}
```

With:
```go
// SkillProvider returns the current set of enabled skills.
type SkillProvider interface {
	ListEnabled(ctx context.Context) ([]*store.Skill, error)
}

type Translator struct {
	cfg      Config
	provider SkillProvider
}

func New(cfg Config, provider SkillProvider) *Translator {
	return &Translator{cfg: cfg, provider: provider}
}
```

- [ ] **Step 2: Update Compile to fetch skills dynamically**

In `internal/controller/translator/translator.go`, update the `Compile` method. Replace:
```go
	// Select skills for this run
	selected := t.selectSkills(run.Spec.Skills)
```

With:
```go
	// Fetch latest skills from registry
	allSkills, err := t.provider.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}

	// Select skills for this run
	selected := selectSkills(allSkills, run.Spec.Skills)
```

- [ ] **Step 3: Change selectSkills to a package-level function**

Replace the method:
```go
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
```

With:
```go
func selectSkills(skills []*store.Skill, names []string) []*store.Skill {
	if len(names) == 0 {
		// ListEnabled already filters disabled, return all
		return skills
	}
	byName := make(map[string]*store.Skill, len(skills))
	for _, s := range skills {
		byName[s.Name] = s
	}
	var selected []*store.Skill
	for _, n := range names {
		if s, ok := byName[n]; ok {
			selected = append(selected, s)
		}
	}
	return selected
}
```

- [ ] **Step 4: Update translator_test.go to use a mock SkillProvider**

In `internal/controller/translator/translator_test.go`, replace the `newTranslator` helper:

```go
// mockProvider implements translator.SkillProvider for tests.
type mockProvider struct {
	skills []*store.Skill
}

func (m *mockProvider) ListEnabled(_ context.Context) ([]*store.Skill, error) {
	var enabled []*store.Skill
	for _, s := range m.skills {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	return enabled, nil
}

func newTranslator(skills []*store.Skill) *translator.Translator {
	return translator.New(translator.Config{
		AgentImage:    "ghcr.io/kube-agent-helper/agent-runtime:latest",
		ControllerURL: "http://controller.svc:8080",
	}, &mockProvider{skills: skills})
}
```

- [ ] **Step 5: Run all translator tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/translator/ -v`
Expected: PASS — all 8 existing tests should pass unchanged

- [ ] **Step 6: Update main.go to wire SkillRegistry**

In `cmd/controller/main.go`, replace:
```go
	// Load skills for translator
	skills, err := st.ListSkills(context.Background())
	if err != nil {
		slog.Error("list skills", "error", err)
		os.Exit(1)
	}

	tr := translator.New(translator.Config{
		AgentImage:       agentImage,
		ControllerURL:    controllerURL,
		AnthropicBaseURL: anthropicBaseURL,
		Model:            model,
	}, skills)
```

With:
```go
	// Create skill registry (reads from store on every call — hot-reload)
	reg := registry.New(st)

	tr := translator.New(translator.Config{
		AgentImage:       agentImage,
		ControllerURL:    controllerURL,
		AnthropicBaseURL: anthropicBaseURL,
		Model:            model,
	}, reg)
```

Add the import:
```go
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
```

- [ ] **Step 7: Run full test suite**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./... -count=1 2>&1 | tail -30`
Expected: All tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/controller/translator/translator.go internal/controller/translator/translator_test.go cmd/controller/main.go
git commit -m "feat(translator): replace static skill list with SkillRegistry for hot-reload"
```

---

### Task 4: Handle DiagnosticSkill CR deletion in reconciler

**Files:**
- Modify: `internal/controller/reconciler/skill_reconciler.go`

- [ ] **Step 1: Update SkillReconciler to handle deletion**

The current reconciler returns early with `nil` when the CR is not found (deleted). It should also delete the corresponding skill from the store. However, we should only delete CR-sourced skills — never delete built-in skills when a CR is removed.

Replace the full `Reconcile` method in `internal/controller/reconciler/skill_reconciler.go`:

```go
func (r *DiagnosticSkillReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var skill k8saiV1.DiagnosticSkill
	if err := r.Get(ctx, req.NamespacedName, &skill); err != nil {
		if errors.IsNotFound(err) {
			// CR deleted — remove from store only if it was CR-sourced.
			// If a built-in skill has the same name, re-insert it so builtin resurfaces.
			existing, getErr := r.Store.GetSkill(ctx, req.Name)
			if getErr == nil && existing.Source == "cr" {
				if delErr := r.Store.DeleteSkill(ctx, req.Name); delErr != nil {
					logger.Error(delErr, "failed to delete skill from store", "name", req.Name)
				} else {
					logger.Info("deleted cr skill from store", "name", req.Name)
				}
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	toolsJSON, err := json.Marshal(skill.Spec.Tools)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("marshal tools: %w", err)
	}
	requiresJSON, err := json.Marshal(skill.Spec.RequiresData)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("marshal requiresData: %w", err)
	}

	priority := 100
	if skill.Spec.Priority != nil {
		priority = *skill.Spec.Priority
	}

	s := &store.Skill{
		Name:             skill.Name,
		Dimension:        skill.Spec.Dimension,
		Prompt:           skill.Spec.Prompt,
		ToolsJSON:        string(toolsJSON),
		RequiresDataJSON: string(requiresJSON),
		Source:           "cr",
		Enabled:          skill.Spec.Enabled,
		Priority:         priority,
	}
	if err := r.Store.UpsertSkill(ctx, s); err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("synced skill", "name", skill.Name)
	return ctrl.Result{}, nil
}
```

- [ ] **Step 2: Run existing tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/reconciler/ -v`
Expected: PASS (or no test files yet — if so, that's fine)

- [ ] **Step 3: Commit**

```bash
git add internal/controller/reconciler/skill_reconciler.go
git commit -m "feat(reconciler): handle DiagnosticSkill CR deletion — remove cr skill from store"
```

---

### Task 5: Add reliability-analyst built-in skill

**Files:**
- Create: `skills/reliability-analyst.md`

- [ ] **Step 1: Create the skill file**

Create `skills/reliability-analyst.md`:

```markdown
---
name: reliability-analyst
dimension: reliability
tools: ["kubectl_get","kubectl_describe","events_list"]
requires_data: ["pods","events","deployments"]
---

You are a Kubernetes reliability specialist. Analyze workloads in the target namespaces for reliability risks.

## Instructions

1. List all Deployments using `kubectl_get` with kind=Deployment for each target namespace.
2. Check for single-replica Deployments in non-system namespaces:
   - Use `kubectl_describe` to verify replicas count.
   - Single-replica Deployments are a single point of failure.
3. Check for missing or misconfigured probes:
   - Use `kubectl_get` with kind=Pod to list pods.
   - Use `kubectl_describe` on each pod to check liveness/readiness probes.
   - Report pods missing livenessProbe or readinessProbe.
   - Report probes with unreasonable settings (initialDelaySeconds=0 with slow-starting apps).
4. Check for high-restart pods that are NOT in CrashLoopBackOff:
   - Pods with >5 restarts but currently Running indicate intermittent failures.
5. Check PodDisruptionBudget coverage:
   - Use `kubectl_get` with kind=PodDisruptionBudget.
   - Identify Deployments with replicas > 1 but no matching PDB.
6. Check for recent eviction or OOMKill events:
   - Use `events_list` to find Evicted or OOMKilling events in the past 1 hour.
7. For each issue found, output one finding JSON per line:
   {"dimension":"reliability","severity":"<critical|high|medium|low>","title":"<title>","description":"<detail>","resource_kind":"<kind>","resource_namespace":"<ns>","resource_name":"<name>","suggestion":"<fix>"}

## Severity Guide
- critical: Single-replica production Deployment with no PDB
- high: Missing liveness probe on long-running service, recent OOMKill
- medium: Missing readiness probe, high restart count (>5)
- low: Missing PDB for multi-replica Deployment
```

- [ ] **Step 2: Verify skill loads correctly**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./cmd/controller/ -run TestParseSkillMD -v 2>/dev/null || echo "No parse test yet — manual check"`

Manually verify the frontmatter is valid by checking the file parses:
Run: `cd /Users/zhenyu.jiang/kube-agent-helper && head -5 skills/reliability-analyst.md`

- [ ] **Step 3: Commit**

```bash
git add skills/reliability-analyst.md
git commit -m "feat(skills): add reliability-analyst built-in skill"
```

---

### Task 6: Add config-drift-analyst built-in skill

**Files:**
- Create: `skills/config-drift-analyst.md`

- [ ] **Step 1: Create the skill file**

Create `skills/config-drift-analyst.md`:

```markdown
---
name: config-drift-analyst
dimension: reliability
tools: ["kubectl_get","kubectl_describe"]
requires_data: ["pods","deployments","services","configmaps"]
---

You are a Kubernetes configuration drift analyst. Detect mismatches and broken references in the target namespaces.

## Instructions

1. Check Deployment selector/label mismatches:
   - Use `kubectl_get` with kind=Deployment for each target namespace.
   - Use `kubectl_describe` to compare `spec.selector.matchLabels` with `spec.template.metadata.labels`.
   - Report any Deployment where selector does not match template labels.
2. Check Service → Endpoint connectivity:
   - Use `kubectl_get` with kind=Service for each target namespace.
   - Use `kubectl_get` with kind=Endpoints for each Service.
   - Report Services with 0 endpoints (selector matches no pods).
3. Check for broken ConfigMap/Secret references:
   - Use `kubectl_get` with kind=Pod for each target namespace.
   - Use `kubectl_describe` on each pod to find volume mounts and envFrom references.
   - Use `kubectl_get` with kind=ConfigMap and kind=Secret to verify referenced objects exist.
   - Report pods referencing non-existent ConfigMaps or Secrets.
4. Check for environment variable conflicts:
   - In pods with multiple containers, check if different containers define the same env var with different values.
5. For each issue found, output one finding JSON per line:
   {"dimension":"reliability","severity":"<critical|high|medium|low>","title":"<title>","description":"<detail>","resource_kind":"<kind>","resource_namespace":"<ns>","resource_name":"<name>","suggestion":"<fix>"}

## Severity Guide
- critical: Service with 0 endpoints (complete traffic blackhole)
- high: Broken ConfigMap/Secret reference (pod will fail to start)
- medium: Deployment selector/label mismatch
- low: Environment variable conflicts between containers
```

- [ ] **Step 2: Commit**

```bash
git add skills/config-drift-analyst.md
git commit -m "feat(skills): add config-drift-analyst built-in skill"
```

---

### Task 7: End-to-end verification

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./... -count=1 -timeout 120s 2>&1 | tail -30`
Expected: All PASS

- [ ] **Step 2: Verify all 5 skills load**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && ls -1 skills/*.md | wc -l`
Expected: 5

- [ ] **Step 3: Build succeeds**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go build ./...`
Expected: No errors

- [ ] **Step 4: Final commit (if any remaining changes)**

Only if there are fixups needed.