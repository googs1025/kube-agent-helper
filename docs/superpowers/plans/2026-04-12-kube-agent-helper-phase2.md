# kube-agent-helper Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add SkillRegistry (built-in + CR merge with dynamic Orchestrator prompt), a full Next.js 14 Dashboard (4 pages), and migrate the storage backend from SQLite to PostgreSQL with pgvector.

**Architecture:** SkillRegistry merges built-in SKILL.md files with DiagnosticSkill CRs at runtime — CR wins on name collision. Translator uses registry output to generate a dynamic Orchestrator prompt injected into the Agent Pod. Dashboard is a separate Next.js app that reads exclusively from the Controller's `/api/*` endpoints. PostgreSQL replaces SQLite via the same `Store` interface; `--db-driver` flag selects the backend.

**Tech Stack:** Go 1.25, controller-runtime v0.20.0; Next.js 14 App Router, shadcn/ui, Tailwind CSS; PostgreSQL 16 + pgvector; pgx/v5.

**Spec:** [`docs/superpowers/specs/2026-04-12-kube-agent-helper-phase1-phase2-design.md`](../specs/2026-04-12-kube-agent-helper-phase1-phase2-design.md)

**Prerequisite:** Phase 0 + Phase 1 plan complete (`2026-04-12-kube-agent-helper-phase0-phase1.md`).

---

## Natural Exit Points

- **After Task 16** — SkillRegistry + dynamic prompt complete, richer diagnostics without Dashboard
- **After Task 19** — Dashboard complete, full Phase 2 delivered
- **After Task 20** — PostgreSQL migration complete, production-ready storage

---

## File Map

```
internal/controller/registry/
  registry.go                    Task 14
  registry_test.go               Task 14
internal/controller/translator/
  translator.go                  Task 15  (modify: add dynamic prompt)
  translator_test.go             Task 15  (modify)
internal/store/
  postgres/
    postgres.go                  Task 20
    migrations/001_initial.sql   Task 20
cmd/controller/main.go           Task 15  (modify: wire registry)
dashboard/
  package.json                   Task 16
  next.config.ts                 Task 16
  tailwind.config.ts             Task 16
  app/
    layout.tsx                   Task 16
    page.tsx                     Task 16  (redirect → /runs)
    runs/
      page.tsx                   Task 17
      [id]/
        page.tsx                 Task 17
        findings/page.tsx        Task 17
    skills/
      page.tsx                   Task 18
    settings/
      page.tsx                   Task 18
  components/
    ui/                          Task 16  (shadcn installs here)
    RunsTable.tsx                Task 17
    FindingsTable.tsx            Task 17
    SkillsTable.tsx              Task 18
  lib/
    api.ts                       Task 16  (typed API client)
```

---

## Task 14: SkillRegistry — merge built-in + CR skills

**Files:**
- Create: `internal/controller/registry/registry.go`
- Create: `internal/controller/registry/registry_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/controller/registry/registry_test.go
package registry_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func builtinSkill(name, dim string) *store.Skill {
	return &store.Skill{Name: name, Dimension: dim, Prompt: "builtin prompt",
		ToolsJSON: "[]", Source: "builtin", Enabled: true, Priority: 100}
}

func crSkill(name, dim, prompt string) *store.Skill {
	return &store.Skill{Name: name, Dimension: dim, Prompt: prompt,
		ToolsJSON: "[]", Source: "cr", Enabled: true, Priority: 50}
}

func TestRegistry_CROverridesBuiltin(t *testing.T) {
	reg := registry.New(
		[]*store.Skill{builtinSkill("pod-health-analyst", "health")},
		[]*store.Skill{crSkill("pod-health-analyst", "health", "custom prompt")},
	)

	skills := reg.List()
	require.Len(t, skills, 1)
	assert.Equal(t, "custom prompt", skills[0].Prompt)
	assert.Equal(t, "cr", skills[0].Source)
}

func TestRegistry_MergeNonOverlapping(t *testing.T) {
	reg := registry.New(
		[]*store.Skill{
			builtinSkill("pod-health-analyst", "health"),
			builtinSkill("pod-security-analyst", "security"),
		},
		[]*store.Skill{
			crSkill("pod-custom-analyst", "cost", "custom"),
		},
	)

	skills := reg.List()
	assert.Len(t, skills, 3)
}

func TestRegistry_GetByName(t *testing.T) {
	reg := registry.New(
		[]*store.Skill{builtinSkill("pod-health-analyst", "health")},
		nil,
	)

	sk := reg.Get("pod-health-analyst")
	require.NotNil(t, sk)
	assert.Equal(t, "health", sk.Dimension)

	assert.Nil(t, reg.Get("nonexistent"))
}

func TestRegistry_DisabledSkillsExcluded(t *testing.T) {
	disabled := builtinSkill("disabled-skill", "health")
	disabled.Enabled = false

	reg := registry.New([]*store.Skill{disabled}, nil)
	assert.Empty(t, reg.List())
}

func TestRegistry_SortedByPriority(t *testing.T) {
	reg := registry.New([]*store.Skill{
		{Name: "b", Dimension: "health", Enabled: true, Priority: 200, ToolsJSON: "[]", Source: "builtin"},
		{Name: "a", Dimension: "health", Enabled: true, Priority: 50, ToolsJSON: "[]", Source: "builtin"},
	}, nil)

	skills := reg.List()
	require.Len(t, skills, 2)
	assert.Equal(t, "a", skills[0].Name) // lower priority number = higher priority
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/controller/registry/... -count=1 -v
```

Expected: FAIL — package not found.

- [ ] **Step 3: Write `internal/controller/registry/registry.go`**

```go
package registry

import (
	"sort"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// Registry merges built-in skills and CR-defined skills.
// CR skills override built-ins with the same name.
type Registry struct {
	skills map[string]*store.Skill // keyed by name, CR wins
}

// New creates a Registry. builtin and cr are the two sources; cr overrides builtin.
func New(builtin, cr []*store.Skill) *Registry {
	merged := make(map[string]*store.Skill, len(builtin)+len(cr))
	for _, s := range builtin {
		merged[s.Name] = s
	}
	for _, s := range cr {
		merged[s.Name] = s // CR overrides
	}
	return &Registry{skills: merged}
}

// List returns all enabled skills sorted by Priority ascending (lower = higher priority).
func (r *Registry) List() []*store.Skill {
	var out []*store.Skill
	for _, s := range r.skills {
		if s.Enabled {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority < out[j].Priority
	})
	return out
}

// Get returns a skill by name, or nil if not found / disabled.
func (r *Registry) Get(name string) *store.Skill {
	s, ok := r.skills[name]
	if !ok || !s.Enabled {
		return nil
	}
	return s
}

// Select returns the subset of skills matching the given names.
// If names is empty, returns all enabled skills.
func (r *Registry) Select(names []string) []*store.Skill {
	if len(names) == 0 {
		return r.List()
	}
	var out []*store.Skill
	for _, n := range names {
		if s := r.Get(n); s != nil {
			out = append(out, s)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/controller/registry/... -count=1 -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/controller/registry/
git commit --no-gpg-sign -m "feat(registry): SkillRegistry with CR-overrides-builtin merge and priority sort"
```

---

## Task 15: Dynamic Orchestrator prompt in Translator + wire Registry into Controller

**Files:**
- Modify: `internal/controller/translator/translator.go`
- Modify: `internal/controller/translator/translator_test.go`
- Modify: `cmd/controller/main.go`

- [ ] **Step 1: Add `BuildOrchestratorPrompt` to translator.go**

Add this method to the `Translator` struct (after the existing `Compile` method):

```go
// BuildOrchestratorPrompt generates a dynamic system prompt from the selected skills.
func BuildOrchestratorPrompt(skills []*store.Skill, targetNamespaces []string) string {
	ns := strings.Join(targetNamespaces, ", ")
	if ns == "" {
		ns = "all namespaces"
	}

	var sb strings.Builder
	sb.WriteString("You are a Kubernetes diagnostic orchestrator.\n\n")
	sb.WriteString(fmt.Sprintf("Target namespaces: %s\n\n", ns))
	sb.WriteString("Available diagnostic skills (run in order):\n\n")

	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("## %s (%s)\n", s.Name, s.Dimension))
		// Include first 500 chars of prompt as description
		preview := s.Prompt
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		sb.WriteString(preview)
		sb.WriteString("\n\n")
	}

	sb.WriteString(`## Output Format
For each issue found by any skill, output one finding JSON per line:
{"dimension":"<dim>","severity":"<critical|high|medium|low|info>","title":"<title>","description":"<detail>","resource_kind":"<kind>","resource_namespace":"<ns>","resource_name":"<name>","suggestion":"<actionable fix>"}

Run all skills sequentially. After all skills complete, output: FINDINGS_COMPLETE
`)
	return sb.String()
}
```

Add `"strings"` to imports.

- [ ] **Step 2: Inject orchestrator prompt into ConfigMap**

In `buildConfigMap`, add a key `_orchestrator_prompt` containing the dynamic prompt:

```go
func (t *Translator) buildConfigMap(name, runID string, skills []*store.Skill, targetNS []string) *corev1.ConfigMap {
	data := make(map[string]string, len(skills)+1)
	for _, s := range skills {
		key := s.Name + ".md"
		data[key] = fmt.Sprintf("---\nname: %s\ndimension: %s\ntools: %s\n---\n\n%s\n",
			s.Name, s.Dimension, s.ToolsJSON, s.Prompt)
	}
	data["_orchestrator_prompt"] = BuildOrchestratorPrompt(skills, targetNS)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"run-id": runID},
		},
		Data: data,
	}
}
```

Update `Compile` to pass `run.Spec.Target.Namespaces` to `buildConfigMap`:
```go
cm := t.buildConfigMap(cmName, runID, selected, run.Spec.Target.Namespaces)
```

- [ ] **Step 3: Update Python runtime to read orchestrator prompt**

In `agent-runtime/runtime/orchestrator.py`, replace `build_prompt(skills)` with reading from the ConfigMap file:

```python
SKILLS_DIR = os.environ.get("SKILLS_DIR", "/workspace/skills")

def load_orchestrator_prompt() -> str:
    """Load the pre-built orchestrator prompt from ConfigMap."""
    path = os.path.join(SKILLS_DIR, "_orchestrator_prompt")
    if os.path.exists(path):
        with open(path) as f:
            return f.read()
    # Fallback: build dynamically from loaded skills
    return build_prompt([])


def run_agent(skills: List[Skill]) -> List[dict]:
    client = anthropic.Anthropic()
    tools = _discover_tools()
    prompt = load_orchestrator_prompt()
    # ... rest of function unchanged
```

- [ ] **Step 4: Update translator test to pass targetNS**

In `translator_test.go`, update the `buildConfigMap` call verification:

```go
assert.Contains(t, cm.Data, "_orchestrator_prompt")
assert.Contains(t, cm.Data["_orchestrator_prompt"], "Target namespaces")
```

- [ ] **Step 5: Wire Registry into cmd/controller/main.go**

Replace the existing skills loading in `main()`:

```go
import "github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"

// After loadBuiltinSkills:
builtinSkills, _ := st.ListSkills(context.Background())

// CR skills come from the reconciler at runtime; we build registry on startup
// with builtin-only, then Reconciler updates the store as CRs are applied.
// For Phase 2, rebuild the registry on every reconcile by reading the store.
reg := registry.New(builtinSkills, nil) // CR skills loaded dynamically by SkillReconciler
_ = reg // used by translator below

tr := translator.New(translator.Config{
    AgentImage:    agentImage,
    ControllerURL: controllerURL,
}, builtinSkills) // translator uses store.ListSkills at compile time
```

Note: The Translator already calls `Store.ListSkills` or uses the skills passed at construction. For Phase 2, update `Compile` to accept a `*registry.Registry` parameter and call `reg.Select(run.Spec.Skills)` instead of `t.selectSkills(...)`.

Update `Compile` signature:
```go
func (t *Translator) Compile(ctx context.Context, run *k8saiV1.DiagnosticRun, reg *registry.Registry) ([]client.Object, error) {
    selected := reg.Select(run.Spec.Skills)
    // ... rest unchanged
}
```

Update `DiagnosticRunReconciler` to hold a `*registry.Registry` and pass it to `Compile`:
```go
type DiagnosticRunReconciler struct {
    client.Client
    Store      store.Store
    Translator *translator.Translator
    Registry   *registry.Registry
}

// In Reconcile, replace:
objects, err := r.Translator.Compile(ctx, &run)
// with:
objects, err := r.Translator.Compile(ctx, &run, r.Registry)
```

And in `main.go`:
```go
reconciler := &reconcilerPkg.DiagnosticRunReconciler{
    Client:     mgr.GetClient(),
    Store:      st,
    Translator: tr,
    Registry:   reg,
}
```

- [ ] **Step 6: Build and test**

```bash
go build ./...
go test ./internal/... -count=1 -timeout=60s
```

- [ ] **Step 7: Commit**

```bash
git add internal/controller/translator/ internal/controller/registry/ \
         cmd/controller/main.go agent-runtime/
git commit --no-gpg-sign -m "feat(registry+translator): dynamic orchestrator prompt + SkillRegistry wired into Controller"
```

---

## Task 16: Dashboard scaffold — Next.js 14 + shadcn/ui + API client

**Files:**
- Create: `dashboard/package.json`
- Create: `dashboard/next.config.ts`
- Create: `dashboard/tailwind.config.ts`
- Create: `dashboard/app/layout.tsx`
- Create: `dashboard/app/page.tsx`
- Create: `dashboard/lib/api.ts`

- [ ] **Step 1: Scaffold Next.js project**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
npx create-next-app@latest dashboard \
  --typescript --tailwind --eslint --app --no-src-dir \
  --import-alias "@/*"
cd dashboard
npx shadcn@latest init --defaults
npx shadcn@latest add table badge button card tabs
```

- [ ] **Step 2: Write `dashboard/lib/api.ts`**

```typescript
const BASE = process.env.NEXT_PUBLIC_CONTROLLER_URL ?? "http://localhost:8080";

export interface DiagnosticRun {
  id: string;
  target_json: string;
  skills_json: string;
  status: "Pending" | "Running" | "Succeeded" | "Failed";
  message: string;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
}

export interface Finding {
  id: string;
  run_id: string;
  dimension: string;
  severity: "critical" | "high" | "medium" | "low" | "info";
  title: string;
  description: string;
  resource_kind: string;
  resource_namespace: string;
  resource_name: string;
  suggestion: string;
  created_at: string;
}

export interface Skill {
  id: string;
  name: string;
  dimension: string;
  prompt: string;
  tools_json: string;
  source: string;
  enabled: boolean;
  priority: number;
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`, { cache: "no-store" });
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return res.json();
}

export const api = {
  runs: {
    list: () => fetchJSON<DiagnosticRun[]>("/api/runs"),
    get: (id: string) => fetchJSON<DiagnosticRun>(`/api/runs/${id}`),
    findings: (id: string) => fetchJSON<Finding[]>(`/api/runs/${id}/findings`),
  },
  skills: {
    list: () => fetchJSON<Skill[]>("/api/skills"),
  },
};
```

- [ ] **Step 3: Write `dashboard/app/layout.tsx`**

```tsx
import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import Link from "next/link";

const inter = Inter({ subsets: ["latin"] });

export const metadata: Metadata = {
  title: "kube-agent-helper",
  description: "Kubernetes AI Diagnostic Dashboard",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={inter.className}>
        <nav className="border-b px-6 py-3 flex gap-6 items-center">
          <span className="font-semibold text-lg">kube-agent-helper</span>
          <Link href="/runs" className="text-sm text-muted-foreground hover:text-foreground">
            Runs
          </Link>
          <Link href="/skills" className="text-sm text-muted-foreground hover:text-foreground">
            Skills
          </Link>
          <Link href="/settings" className="text-sm text-muted-foreground hover:text-foreground">
            Settings
          </Link>
        </nav>
        <main className="container mx-auto px-6 py-8">{children}</main>
      </body>
    </html>
  );
}
```

- [ ] **Step 4: Write `dashboard/app/page.tsx`**

```tsx
import { redirect } from "next/navigation";

export default function Home() {
  redirect("/runs");
}
```

- [ ] **Step 5: Verify dev server starts**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm run dev
```

Expected: server starts on http://localhost:3000, redirects to /runs.

Stop the dev server (Ctrl+C).

- [ ] **Step 6: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/
git commit --no-gpg-sign -m "feat(dashboard): Next.js 14 scaffold with shadcn/ui, API client, nav layout"
```

---

## Task 17: Dashboard — /runs and /runs/[id] pages

**Files:**
- Create: `dashboard/app/runs/page.tsx`
- Create: `dashboard/app/runs/[id]/page.tsx`
- Create: `dashboard/components/RunsTable.tsx`
- Create: `dashboard/components/FindingsTable.tsx`

- [ ] **Step 1: Write `dashboard/components/RunsTable.tsx`**

```tsx
"use client";
import { DiagnosticRun } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import Link from "next/link";

const statusVariant: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  Pending: "secondary",
  Running: "default",
  Succeeded: "outline",
  Failed: "destructive",
};

export function RunsTable({ runs }: { runs: DiagnosticRun[] }) {
  if (runs.length === 0) {
    return <p className="text-muted-foreground text-sm">No runs yet. Use <code>kubectl apply</code> to create a DiagnosticRun.</p>;
  }
  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b text-left text-muted-foreground">
          <th className="py-2 pr-4">ID</th>
          <th className="py-2 pr-4">Status</th>
          <th className="py-2 pr-4">Skills</th>
          <th className="py-2">Created</th>
        </tr>
      </thead>
      <tbody>
        {runs.map((r) => {
          const skills = (() => { try { return JSON.parse(r.skills_json) as string[]; } catch { return []; } })();
          return (
            <tr key={r.id} className="border-b hover:bg-muted/50">
              <td className="py-2 pr-4">
                <Link href={`/runs/${r.id}`} className="font-mono text-xs underline">
                  {r.id.slice(0, 8)}…
                </Link>
              </td>
              <td className="py-2 pr-4">
                <Badge variant={statusVariant[r.status] ?? "secondary"}>{r.status}</Badge>
              </td>
              <td className="py-2 pr-4 text-xs">{skills.join(", ") || "all"}</td>
              <td className="py-2 text-xs text-muted-foreground">
                {new Date(r.created_at).toLocaleString()}
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}
```

- [ ] **Step 2: Write `dashboard/app/runs/page.tsx`**

```tsx
import { api } from "@/lib/api";
import { RunsTable } from "@/components/RunsTable";

export const dynamic = "force-dynamic";

export default async function RunsPage() {
  const runs = await api.runs.list().catch(() => []);
  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold">Diagnostic Runs</h1>
        <p className="text-sm text-muted-foreground">
          Trigger via <code className="bg-muted px-1 rounded">kubectl apply -f run.yaml</code>
        </p>
      </div>
      <RunsTable runs={runs} />
    </div>
  );
}
```

- [ ] **Step 3: Write `dashboard/components/FindingsTable.tsx`**

```tsx
"use client";
import { Finding } from "@/lib/api";
import { Badge } from "@/components/ui/badge";

const severityVariant: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  critical: "destructive",
  high: "destructive",
  medium: "default",
  low: "secondary",
  info: "outline",
};

export function FindingsTable({ findings }: { findings: Finding[] }) {
  if (findings.length === 0) {
    return <p className="text-muted-foreground text-sm">No findings.</p>;
  }
  return (
    <div className="space-y-3">
      {findings.map((f) => (
        <div key={f.id} className="border rounded-lg p-4">
          <div className="flex items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-2 mb-1">
                <Badge variant={severityVariant[f.severity] ?? "secondary"}>{f.severity}</Badge>
                <span className="text-xs text-muted-foreground">{f.dimension}</span>
              </div>
              <p className="font-medium text-sm">{f.title}</p>
              <p className="text-xs text-muted-foreground mt-1">
                {f.resource_kind}/{f.resource_namespace}/{f.resource_name}
              </p>
            </div>
          </div>
          {f.description && (
            <p className="text-sm mt-2 text-muted-foreground">{f.description}</p>
          )}
          {f.suggestion && (
            <p className="text-sm mt-2">
              <span className="font-medium">Suggestion:</span> {f.suggestion}
            </p>
          )}
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 4: Write `dashboard/app/runs/[id]/page.tsx`**

```tsx
import { api } from "@/lib/api";
import { FindingsTable } from "@/components/FindingsTable";
import { Badge } from "@/components/ui/badge";
import Link from "next/link";
import { notFound } from "next/navigation";

export const dynamic = "force-dynamic";

export default async function RunDetailPage({ params }: { params: { id: string } }) {
  const [run, findings] = await Promise.all([
    api.runs.get(params.id).catch(() => null),
    api.runs.findings(params.id).catch(() => []),
  ]);

  if (!run) notFound();

  const criticalCount = findings.filter((f) => f.severity === "critical").length;
  const highCount = findings.filter((f) => f.severity === "high").length;

  return (
    <div>
      <div className="mb-6">
        <Link href="/runs" className="text-sm text-muted-foreground hover:text-foreground">
          ← All Runs
        </Link>
        <h1 className="text-2xl font-semibold mt-2">
          Run <code className="font-mono text-lg">{run.id.slice(0, 8)}…</code>
        </h1>
        <div className="flex gap-3 mt-2">
          <Badge>{run.status}</Badge>
          {criticalCount > 0 && <Badge variant="destructive">{criticalCount} critical</Badge>}
          {highCount > 0 && <Badge variant="destructive">{highCount} high</Badge>}
        </div>
        {run.message && (
          <p className="text-sm text-muted-foreground mt-2">{run.message}</p>
        )}
      </div>
      <h2 className="text-lg font-medium mb-4">Findings ({findings.length})</h2>
      <FindingsTable findings={findings} />
    </div>
  );
}
```

- [ ] **Step 5: Build to verify no type errors**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm run build
```

Expected: BUILD successful.

- [ ] **Step 6: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/app/runs/ dashboard/components/RunsTable.tsx dashboard/components/FindingsTable.tsx
git commit --no-gpg-sign -m "feat(dashboard): /runs list page + /runs/[id] detail with findings"
```

---

## Task 18: Dashboard — /skills and /settings pages

**Files:**
- Create: `dashboard/app/skills/page.tsx`
- Create: `dashboard/components/SkillsTable.tsx`
- Create: `dashboard/app/settings/page.tsx`

- [ ] **Step 1: Write `dashboard/components/SkillsTable.tsx`**

```tsx
"use client";
import { Skill } from "@/lib/api";
import { Badge } from "@/components/ui/badge";

const dimensionColors: Record<string, string> = {
  health: "bg-green-100 text-green-800",
  security: "bg-red-100 text-red-800",
  cost: "bg-yellow-100 text-yellow-800",
  reliability: "bg-blue-100 text-blue-800",
};

export function SkillsTable({ skills }: { skills: Skill[] }) {
  if (skills.length === 0) {
    return <p className="text-muted-foreground text-sm">No skills loaded.</p>;
  }
  return (
    <div className="space-y-3">
      {skills.map((s) => {
        const tools = (() => { try { return JSON.parse(s.tools_json) as string[]; } catch { return []; } })();
        return (
          <div key={s.id} className="border rounded-lg p-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <span className="font-medium text-sm">{s.name}</span>
                <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${dimensionColors[s.dimension] ?? "bg-gray-100"}`}>
                  {s.dimension}
                </span>
                <Badge variant={s.source === "cr" ? "default" : "outline"} className="text-xs">
                  {s.source}
                </Badge>
              </div>
              <Badge variant={s.enabled ? "outline" : "secondary"}>
                {s.enabled ? "enabled" : "disabled"}
              </Badge>
            </div>
            {tools.length > 0 && (
              <p className="text-xs text-muted-foreground mt-2">
                Tools: {tools.join(", ")}
              </p>
            )}
          </div>
        );
      })}
    </div>
  );
}
```

- [ ] **Step 2: Write `dashboard/app/skills/page.tsx`**

```tsx
import { api } from "@/lib/api";
import { SkillsTable } from "@/components/SkillsTable";

export const dynamic = "force-dynamic";

export default async function SkillsPage() {
  const skills = await api.skills.list().catch(() => []);
  const builtinCount = skills.filter((s) => s.source === "builtin").length;
  const crCount = skills.filter((s) => s.source === "cr").length;

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-semibold">Skills</h1>
        <p className="text-sm text-muted-foreground mt-1">
          {builtinCount} built-in · {crCount} custom (DiagnosticSkill CR)
        </p>
      </div>
      <SkillsTable skills={skills} />
    </div>
  );
}
```

- [ ] **Step 3: Write `dashboard/app/settings/page.tsx`**

```tsx
export default function SettingsPage() {
  return (
    <div>
      <h1 className="text-2xl font-semibold mb-6">Settings</h1>
      <div className="border rounded-lg p-6 max-w-lg">
        <h2 className="font-medium mb-4">ModelConfig</h2>
        <p className="text-sm text-muted-foreground mb-4">
          Model configuration is managed via Kubernetes CRs. Use <code className="bg-muted px-1 rounded">kubectl</code> to create or update ModelConfig resources.
        </p>
        <pre className="bg-muted rounded p-4 text-xs overflow-auto">{`apiVersion: k8sai.io/v1alpha1
kind: ModelConfig
metadata:
  name: claude-default
  namespace: kube-agent-helper
spec:
  provider: anthropic
  model: claude-sonnet-4-6
  apiKeyRef:
    name: anthropic-credentials
    key: apiKey
  maxTurns: 20`}</pre>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Build**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper/dashboard
npm run build
```

- [ ] **Step 5: Commit**

```bash
cd /Users/zhenyu.jiang/kube-agent-helper
git add dashboard/app/skills/ dashboard/app/settings/ dashboard/components/SkillsTable.tsx
git commit --no-gpg-sign -m "feat(dashboard): /skills page + /settings page — Phase 2 Dashboard complete"
```

---

## Task 19: Wire Dashboard into Helm chart + add CORS to HTTP server

**Files:**
- Modify: `internal/controller/httpserver/server.go` (add CORS headers)
- Modify: `deploy/helm/Chart.yaml` (bump version)
- Create: `deploy/helm/templates/dashboard.yaml`
- Modify: `deploy/helm/values.yaml`

- [ ] **Step 1: Add CORS middleware to httpserver/server.go**

Wrap the `mux` in a CORS handler in `New()`:

```go
func New(s store.Store) *Server {
	srv := &Server{store: s, mux: http.NewServeMux()}
	srv.mux.HandleFunc("/internal/runs/", srv.handleInternal)
	srv.mux.HandleFunc("/api/runs", srv.handleAPIRuns)
	srv.mux.HandleFunc("/api/runs/", srv.handleAPIRunDetail)
	srv.mux.HandleFunc("/api/skills", srv.handleAPISkills)
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}
```

- [ ] **Step 2: Add dashboard deployment to helm values.yaml**

```yaml
dashboard:
  enabled: true
  image: ghcr.io/kube-agent-helper/dashboard:latest
  pullPolicy: IfNotPresent
  port: 3000
```

- [ ] **Step 3: Create `deploy/helm/templates/dashboard.yaml`**

```yaml
{{- if .Values.dashboard.enabled }}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-dashboard
  namespace: {{ .Release.Namespace }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ .Release.Name }}-dashboard
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}-dashboard
    spec:
      containers:
      - name: dashboard
        image: {{ .Values.dashboard.image }}
        imagePullPolicy: {{ .Values.dashboard.pullPolicy }}
        env:
        - name: NEXT_PUBLIC_CONTROLLER_URL
          value: "http://{{ .Release.Name }}.{{ .Release.Namespace }}.svc.cluster.local:8080"
        ports:
        - containerPort: {{ .Values.dashboard.port }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}-dashboard
  namespace: {{ .Release.Namespace }}
spec:
  selector:
    app: {{ .Release.Name }}-dashboard
  ports:
  - port: {{ .Values.dashboard.port }}
    targetPort: {{ .Values.dashboard.port }}
{{- end }}
```

- [ ] **Step 4: Build and test**

```bash
go build ./...
go test ./internal/... -count=1 -timeout=60s
cd dashboard && npm run build && cd ..
```

- [ ] **Step 5: Commit**

```bash
git add internal/controller/httpserver/server.go \
         deploy/helm/templates/dashboard.yaml \
         deploy/helm/values.yaml
git commit --no-gpg-sign -m "feat: CORS headers + Dashboard Helm deployment — Phase 2 complete"
```

---

## Task 20: PostgreSQL Store implementation

**Files:**
- Create: `internal/store/postgres/postgres.go`
- Create: `internal/store/postgres/migrations/001_initial.sql`
- Modify: `cmd/controller/main.go` (add `--db-driver` flag)

- [ ] **Step 1: Add pgx dependency**

```bash
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/stdlib
go get github.com/golang-migrate/migrate/v4/database/pgx/v5
```

- [ ] **Step 2: Write `internal/store/postgres/migrations/001_initial.sql`**

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "vector";

CREATE TABLE IF NOT EXISTS diagnostic_runs (
    id           TEXT PRIMARY KEY,
    target_json  JSONB NOT NULL DEFAULT '{}',
    skills_json  JSONB NOT NULL DEFAULT '[]',
    status       TEXT NOT NULL DEFAULT 'Pending',
    message      TEXT NOT NULL DEFAULT '',
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS findings (
    id                  TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    run_id              TEXT NOT NULL REFERENCES diagnostic_runs(id),
    dimension           TEXT NOT NULL,
    severity            TEXT NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    resource_kind       TEXT NOT NULL DEFAULT '',
    resource_namespace  TEXT NOT NULL DEFAULT '',
    resource_name       TEXT NOT NULL DEFAULT '',
    suggestion          TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS skills (
    id                  TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name                TEXT NOT NULL UNIQUE,
    dimension           TEXT NOT NULL,
    prompt              TEXT NOT NULL,
    tools_json          JSONB NOT NULL DEFAULT '[]',
    requires_data_json  JSONB NOT NULL DEFAULT '[]',
    source              TEXT NOT NULL DEFAULT 'builtin',
    enabled             BOOLEAN NOT NULL DEFAULT true,
    priority            INTEGER NOT NULL DEFAULT 100,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS case_memory (
    id         TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    embedding  vector(1536),
    finding_id TEXT REFERENCES findings(id),
    outcome    TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

- [ ] **Step 3: Write `internal/store/postgres/postgres.go`**

```go
package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type PGStore struct {
	db *sql.DB
}

func New(dsn string) (*PGStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &PGStore{db: db}, nil
}

func runMigrations(db *sql.DB) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func (s *PGStore) Close() error { return s.db.Close() }

func (s *PGStore) CreateRun(ctx context.Context, run *store.DiagnosticRun) error {
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	run.CreatedAt = time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO diagnostic_runs (id, target_json, skills_json, status, created_at)
		 VALUES ($1, $2::jsonb, $3::jsonb, $4, $5)`,
		run.ID, run.TargetJSON, run.SkillsJSON, string(run.Status), run.CreatedAt,
	)
	return err
}

func (s *PGStore) GetRun(ctx context.Context, id string) (*store.DiagnosticRun, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, target_json::text, skills_json::text, status, message,
		        started_at, completed_at, created_at
		 FROM diagnostic_runs WHERE id = $1`, id)
	return scanRun(row)
}

func (s *PGStore) UpdateRunStatus(ctx context.Context, id string, phase store.Phase, msg string) error {
	now := time.Now()
	switch phase {
	case store.PhaseRunning:
		_, err := s.db.ExecContext(ctx,
			`UPDATE diagnostic_runs SET status=$1, message=$2, started_at=$3 WHERE id=$4`,
			string(phase), msg, now, id)
		return err
	case store.PhaseSucceeded, store.PhaseFailed:
		_, err := s.db.ExecContext(ctx,
			`UPDATE diagnostic_runs SET status=$1, message=$2, completed_at=$3 WHERE id=$4`,
			string(phase), msg, now, id)
		return err
	default:
		_, err := s.db.ExecContext(ctx,
			`UPDATE diagnostic_runs SET status=$1, message=$2 WHERE id=$3`,
			string(phase), msg, id)
		return err
	}
}

func (s *PGStore) ListRuns(ctx context.Context, opts store.ListOpts) ([]*store.DiagnosticRun, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, target_json::text, skills_json::text, status, message,
		        started_at, completed_at, created_at
		 FROM diagnostic_runs ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
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

func (s *PGStore) CreateFinding(ctx context.Context, f *store.Finding) error {
	if f.ID == "" {
		f.ID = uuid.NewString()
	}
	f.CreatedAt = time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO findings
		 (id, run_id, dimension, severity, title, description,
		  resource_kind, resource_namespace, resource_name, suggestion, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		f.ID, f.RunID, f.Dimension, f.Severity, f.Title, f.Description,
		f.ResourceKind, f.ResourceNamespace, f.ResourceName, f.Suggestion, f.CreatedAt,
	)
	return err
}

func (s *PGStore) ListFindings(ctx context.Context, runID string) ([]*store.Finding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, run_id, dimension, severity, title, description,
		        resource_kind, resource_namespace, resource_name, suggestion, created_at
		 FROM findings WHERE run_id = $1 ORDER BY created_at ASC`, runID)
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

func (s *PGStore) UpsertSkill(ctx context.Context, sk *store.Skill) error {
	if sk.ID == "" {
		sk.ID = uuid.NewString()
	}
	sk.UpdatedAt = time.Now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO skills (id, name, dimension, prompt, tools_json, requires_data_json,
		                     source, enabled, priority, updated_at)
		 VALUES ($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7,$8,$9,$10)
		 ON CONFLICT(name) DO UPDATE SET
		   dimension=$3, prompt=$4, tools_json=$5::jsonb, requires_data_json=$6::jsonb,
		   source=$7, enabled=$8, priority=$9, updated_at=$10`,
		sk.ID, sk.Name, sk.Dimension, sk.Prompt, sk.ToolsJSON, sk.RequiresDataJSON,
		sk.Source, sk.Enabled, sk.Priority, sk.UpdatedAt,
	)
	return err
}

func (s *PGStore) ListSkills(ctx context.Context) ([]*store.Skill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, dimension, prompt, tools_json::text, requires_data_json::text,
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

func (s *PGStore) GetSkill(ctx context.Context, name string) (*store.Skill, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, dimension, prompt, tools_json::text, requires_data_json::text,
		        source, enabled, priority, updated_at
		 FROM skills WHERE name = $1`, name)
	sk := &store.Skill{}
	err := row.Scan(&sk.ID, &sk.Name, &sk.Dimension, &sk.Prompt,
		&sk.ToolsJSON, &sk.RequiresDataJSON, &sk.Source, &sk.Enabled,
		&sk.Priority, &sk.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sk, err
}

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

- [ ] **Step 4: Add `--db-driver` flag to cmd/controller/main.go**

Add to the flag definitions:
```go
var dbDriver string
flag.StringVar(&dbDriver, "db-driver", "sqlite", "DB driver: sqlite or postgres")
```

Replace the `sqlitestore.New(dbPath)` call with:

```go
var st store.Store
switch dbDriver {
case "postgres":
    pgst, err := postgres.New(dbPath) // dbPath used as DSN for postgres
    if err != nil {
        slog.Error("open postgres", "error", err)
        os.Exit(1)
    }
    st = pgst
default:
    sqlst, err := sqlitestore.New(dbPath)
    if err != nil {
        slog.Error("open sqlite", "error", err)
        os.Exit(1)
    }
    st = sqlst
}
defer st.Close()
```

Add import:
```go
postgres "github.com/kube-agent-helper/kube-agent-helper/internal/store/postgres"
```

- [ ] **Step 5: Build**

```bash
go build ./...
```

- [ ] **Step 6: Commit**

```bash
git add internal/store/postgres/ cmd/controller/main.go go.mod go.sum
git commit --no-gpg-sign -m "feat(store): PostgreSQL implementation with pgvector + --db-driver flag"
```

---

## Phase 2 Done ✓

At this point:
- `SkillRegistry` merges built-in and CR skills, CR wins on name collision
- Translator generates a dynamic Orchestrator prompt including all selected skills
- Dashboard accessible at port 3000: `/runs`, `/runs/[id]`, `/skills`, `/settings`
- PostgreSQL backend available via `--db-driver postgres --db <dsn>`

**Deployment with PostgreSQL:**
```bash
helm install kube-agent-helper ./deploy/helm \
  --set controller.dbDriver=postgres \
  --set controller.dbPath="postgres://user:pass@postgres:5432/kah?sslmode=disable"
```
