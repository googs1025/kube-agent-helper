# Dashboard Create CRD & Skill Forms Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add "+ 创建 Run / Skill" buttons to the dashboard that create real K8s CRDs via the controller's HTTP API, plus summary stats bars on each list page.

**Architecture:** HTTP server gains a `client.Client` (injected from `mgr.GetClient()`). `POST /api/runs` creates a `DiagnosticRun` CR; `POST /api/skills` creates a `DiagnosticSkill` CR. Existing reconcilers pick them up automatically. Frontend uses `@base-ui/react/dialog` for modal forms, SWR `mutate()` to refresh lists on success. Stats are computed from already-fetched SWR data — no new API endpoint needed.

**Tech Stack:** Go + controller-runtime `client.Client`, fake client for tests; Next.js 14 App Router, `@base-ui/react/dialog`, SWR, Tailwind CSS, TypeScript

---

## File Map

**Backend:**
- Modify: `internal/controller/httpserver/server.go` — add `k8sClient client.Client` field, `New(store, k8sClient)`, `POST /api/runs`, `POST /api/skills` handlers
- Modify: `internal/controller/httpserver/server_test.go` — update `New(fs)` → `New(fs, nil)` in existing tests; add POST tests with fake k8s client
- Modify: `cmd/controller/main.go` — pass `mgr.GetClient()` to `httpserver.New()`
- Modify: `deploy/helm/templates/rbac.yaml` — add `create` verb to diagnosticruns + diagnosticskills

**Frontend:**
- Modify: `dashboard/src/lib/types.ts` — add `CreateRunRequest`, `CreateSkillRequest`
- Modify: `dashboard/src/lib/api.ts` — add `createRun()`, `createSkill()`
- Create: `dashboard/src/components/ui/dialog.tsx` — Dialog primitive using `@base-ui/react/dialog`
- Create: `dashboard/src/components/tag-input.tsx` — reusable chip tag input (Enter/comma adds, × removes)
- Create: `dashboard/src/components/create-run-dialog.tsx` — create run form
- Create: `dashboard/src/components/create-skill-dialog.tsx` — create skill form
- Modify: `dashboard/src/app/page.tsx` — stats bar + "+ 创建 Run" button
- Modify: `dashboard/src/app/skills/page.tsx` — stats bar + "+ 创建 Skill" button

---

## Task 1: RBAC + k8s client injection

**Files:**
- Modify: `deploy/helm/templates/rbac.yaml`
- Modify: `internal/controller/httpserver/server.go`
- Modify: `cmd/controller/main.go`

- [ ] **Step 1: Add `create` verb to RBAC**

In `deploy/helm/templates/rbac.yaml`, change line 14:
```yaml
- apiGroups: ["k8sai.io"]
  resources: ["diagnosticruns","diagnosticskills","modelconfigs","diagnosticfixes"]
  verbs: ["get","list","watch","create","update","patch"]
```

- [ ] **Step 2: Add k8s client field and update `New()` in server.go**

Replace the `Server` struct and `New` function:
```go
import (
    // existing imports +
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type Server struct {
    store     store.Store
    k8sClient client.Client
    mux       *http.ServeMux
}

func New(s store.Store, k8sClient client.Client) *Server {
    srv := &Server{store: s, k8sClient: k8sClient, mux: http.NewServeMux()}
    srv.mux.HandleFunc("/internal/runs/", srv.handleInternal)
    srv.mux.HandleFunc("/api/runs", srv.handleAPIRuns)
    srv.mux.HandleFunc("/api/runs/", srv.handleAPIRunDetail)
    srv.mux.HandleFunc("/api/skills", srv.handleAPISkills)
    srv.mux.HandleFunc("/api/fixes", srv.handleAPIFixes)
    srv.mux.HandleFunc("/api/fixes/", srv.handleAPIFixDetail)
    return srv
}
```

- [ ] **Step 3: Update `cmd/controller/main.go` to pass k8s client**

Change line 129:
```go
httpSrv := httpserver.New(st, mgr.GetClient())
```

- [ ] **Step 4: Update existing server tests — change `New(fs)` → `New(fs, nil)`**

In `internal/controller/httpserver/server_test.go`, replace every `httpserver.New(fs)` with `httpserver.New(fs, nil)`:
```go
srv := httpserver.New(fs, nil)
```
(3 places: TestPostFindings, TestGetFindings, TestGetSkills)

- [ ] **Step 5: Run existing tests to confirm nothing broke**

```bash
cd /path/to/kube-agent-helper
go test ./internal/controller/httpserver/... -v -count=1
```
Expected: all 3 existing tests PASS

- [ ] **Step 6: Commit**

```bash
git add deploy/helm/templates/rbac.yaml \
        internal/controller/httpserver/server.go \
        internal/controller/httpserver/server_test.go \
        cmd/controller/main.go
git commit -m "feat(httpserver): inject k8s client for CRD creation"
```

---

## Task 2: `POST /api/runs` handler + tests

**Files:**
- Modify: `internal/controller/httpserver/server.go`
- Modify: `internal/controller/httpserver/server_test.go`

- [ ] **Step 1: Write the failing test first**

Add to `server_test.go`. Add required imports at top:
```go
import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"

    v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
    "github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
    "github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func newFakeK8sClient() client.Client {
    scheme := runtime.NewScheme()
    _ = v1alpha1.AddToScheme(scheme)
    return fake.NewClientBuilder().WithScheme(scheme).Build()
}

func TestPostRun(t *testing.T) {
    fs := &fakeStore{}
    srv := httpserver.New(fs, newFakeK8sClient())

    body, _ := json.Marshal(map[string]interface{}{
        "namespace":      "default",
        "target":         map[string]interface{}{"scope": "namespace", "namespaces": []string{"default"}},
        "modelConfigRef": "anthropic-credentials",
    })
    req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    assert.Equal(t, http.StatusCreated, rr.Code)

    var resp map[string]interface{}
    require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
    assert.NotEmpty(t, resp["metadata"])
}

func TestPostRunMissingModelConfig(t *testing.T) {
    fs := &fakeStore{}
    srv := httpserver.New(fs, newFakeK8sClient())

    body, _ := json.Marshal(map[string]interface{}{
        "namespace": "default",
        "target":    map[string]interface{}{"scope": "namespace"},
        // modelConfigRef missing
    })
    req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    assert.Equal(t, http.StatusBadRequest, rr.Code)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/controller/httpserver/... -run TestPostRun -v
```
Expected: FAIL — "not implemented" returns 501

- [ ] **Step 3: Add `POST /api/runs` handler to server.go**

Add the following imports if not present (merge with existing imports block):
```go
import (
    // existing imports, plus:
    "fmt"
    "math/rand"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
)
```

Replace the `case http.MethodPost` branch inside `handleAPIRuns`:
```go
case http.MethodPost:
    s.handleAPIRunsPost(w, r)
```

Add the new handler method:
```go
func (s *Server) handleAPIRunsPost(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Name           string            `json:"name"`
        Namespace      string            `json:"namespace"`
        Target         struct {
            Scope         string            `json:"scope"`
            Namespaces    []string          `json:"namespaces"`
            LabelSelector map[string]string `json:"labelSelector"`
        } `json:"target"`
        Skills         []string          `json:"skills"`
        ModelConfigRef string            `json:"modelConfigRef"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "bad json", http.StatusBadRequest)
        return
    }
    if req.ModelConfigRef == "" {
        http.Error(w, "modelConfigRef is required", http.StatusBadRequest)
        return
    }
    if req.Namespace == "" {
        req.Namespace = "default"
    }
    if req.Name == "" {
        req.Name = fmt.Sprintf("run-%s-%s", time.Now().Format("20060102"), randSuffix(4))
    }

    cr := &v1alpha1.DiagnosticRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      req.Name,
            Namespace: req.Namespace,
        },
        Spec: v1alpha1.DiagnosticRunSpec{
            Target: v1alpha1.TargetSpec{
                Scope:         req.Target.Scope,
                Namespaces:    req.Target.Namespaces,
                LabelSelector: req.Target.LabelSelector,
            },
            Skills:         req.Skills,
            ModelConfigRef: req.ModelConfigRef,
        },
    }

    if err := s.k8sClient.Create(r.Context(), cr); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    _ = json.NewEncoder(w).Encode(cr)
}

func randSuffix(n int) string {
    const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
    b := make([]byte, n)
    for i := range b {
        b[i] = chars[rand.Intn(len(chars))]
    }
    return string(b)
}
```

- [ ] **Step 4: Run tests to confirm they pass**

```bash
go test ./internal/controller/httpserver/... -run TestPostRun -v
```
Expected: PASS

- [ ] **Step 5: Run all httpserver tests**

```bash
go test ./internal/controller/httpserver/... -v -count=1
```
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/controller/httpserver/server.go \
        internal/controller/httpserver/server_test.go
git commit -m "feat(httpserver): POST /api/runs creates DiagnosticRun CR"
```

---

## Task 3: `POST /api/skills` handler + tests

**Files:**
- Modify: `internal/controller/httpserver/server.go`
- Modify: `internal/controller/httpserver/server_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `server_test.go`:
```go
func TestPostSkill(t *testing.T) {
    fs := &fakeStore{}
    srv := httpserver.New(fs, newFakeK8sClient())

    body, _ := json.Marshal(map[string]interface{}{
        "name":        "my-analyst",
        "namespace":   "default",
        "dimension":   "health",
        "description": "Analyzes pod health",
        "prompt":      "You are a health analyst...",
        "tools":       []string{"kubectl_get"},
        "enabled":     true,
        "priority":    100,
    })
    req := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    assert.Equal(t, http.StatusCreated, rr.Code)
    var resp map[string]interface{}
    require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
    assert.NotEmpty(t, resp["metadata"])
}

func TestPostSkillMissingFields(t *testing.T) {
    fs := &fakeStore{}
    srv := httpserver.New(fs, newFakeK8sClient())

    body, _ := json.Marshal(map[string]interface{}{
        "namespace": "default",
        // name, dimension, prompt, tools missing
    })
    req := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    assert.Equal(t, http.StatusBadRequest, rr.Code)
}
```

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/controller/httpserver/... -run TestPostSkill -v
```
Expected: FAIL — 405 Method Not Allowed

- [ ] **Step 3: Add `POST /api/skills` handler to server.go**

Replace `handleAPISkills` to handle both GET and POST:
```go
func (s *Server) handleAPISkills(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        skills, err := s.store.ListSkills(r.Context())
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        if skills == nil {
            skills = make([]*store.Skill, 0)
        }
        writeJSON(w, skills)
    case http.MethodPost:
        s.handleAPISkillsPost(w, r)
    default:
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    }
}

func (s *Server) handleAPISkillsPost(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Name         string   `json:"name"`
        Namespace    string   `json:"namespace"`
        Dimension    string   `json:"dimension"`
        Description  string   `json:"description"`
        Prompt       string   `json:"prompt"`
        Tools        []string `json:"tools"`
        RequiresData []string `json:"requiresData"`
        Enabled      bool     `json:"enabled"`
        Priority     int      `json:"priority"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "bad json", http.StatusBadRequest)
        return
    }
    if req.Name == "" || req.Dimension == "" || req.Prompt == "" || len(req.Tools) == 0 {
        http.Error(w, "name, dimension, prompt, and tools are required", http.StatusBadRequest)
        return
    }
    if req.Namespace == "" {
        req.Namespace = "default"
    }
    priority := req.Priority
    if priority == 0 {
        priority = 100
    }

    cr := &v1alpha1.DiagnosticSkill{
        ObjectMeta: metav1.ObjectMeta{
            Name:      req.Name,
            Namespace: req.Namespace,
        },
        Spec: v1alpha1.DiagnosticSkillSpec{
            Dimension:    req.Dimension,
            Description:  req.Description,
            Prompt:       req.Prompt,
            Tools:        req.Tools,
            RequiresData: req.RequiresData,
            Enabled:      req.Enabled,
            Priority:     &priority,
        },
    }

    if err := s.k8sClient.Create(r.Context(), cr); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    _ = json.NewEncoder(w).Encode(cr)
}
```

- [ ] **Step 4: Run all httpserver tests**

```bash
go test ./internal/controller/httpserver/... -v -count=1
```
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/controller/httpserver/server.go \
        internal/controller/httpserver/server_test.go
git commit -m "feat(httpserver): POST /api/skills creates DiagnosticSkill CR"
```

---

## Task 4: Frontend types + API client

**Files:**
- Modify: `dashboard/src/lib/types.ts`
- Modify: `dashboard/src/lib/api.ts`

- [ ] **Step 1: Add request types to `types.ts`**

Append to end of `dashboard/src/lib/types.ts`:
```typescript
export interface CreateRunRequest {
  name?: string;
  namespace: string;
  target: {
    scope: "namespace" | "cluster";
    namespaces?: string[];
    labelSelector?: Record<string, string>;
  };
  skills?: string[];
  modelConfigRef: string;
}

export interface CreateSkillRequest {
  name: string;
  namespace: string;
  dimension: "health" | "security" | "cost" | "reliability";
  description: string;
  prompt: string;
  tools: string[];
  requiresData?: string[];
  enabled: boolean;
  priority?: number;
}
```

- [ ] **Step 2: Add mutate functions to `api.ts`**

Append to `dashboard/src/lib/api.ts`:
```typescript
import type { DiagnosticRun, Finding, Skill, CreateRunRequest, CreateSkillRequest } from "./types";

export async function createRun(body: CreateRunRequest): Promise<void> {
  const res = await fetch("/api/runs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
}

export async function createSkill(body: CreateSkillRequest): Promise<void> {
  const res = await fetch("/api/skills", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }
}
```

- [ ] **Step 3: Build to confirm no TypeScript errors**

```bash
cd dashboard && npm run build 2>&1 | tail -15
```
Expected: build succeeds, no type errors

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/lib/types.ts dashboard/src/lib/api.ts
git commit -m "feat(dashboard): add createRun/createSkill API client functions"
```

---

## Task 5: Dialog UI component

**Files:**
- Create: `dashboard/src/components/ui/dialog.tsx`

- [ ] **Step 1: Create dialog.tsx using @base-ui/react/dialog**

Create `dashboard/src/components/ui/dialog.tsx`:
```typescript
"use client";

import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { cn } from "@/lib/utils";

function DialogRoot(props: DialogPrimitive.Root.Props) {
  return <DialogPrimitive.Root {...props} />;
}

function DialogTrigger(props: DialogPrimitive.Trigger.Props) {
  return <DialogPrimitive.Trigger {...props} />;
}

function DialogPortal(props: DialogPrimitive.Portal.Props) {
  return <DialogPrimitive.Portal {...props} />;
}

function DialogBackdrop({ className, ...props }: DialogPrimitive.Backdrop.Props) {
  return (
    <DialogPrimitive.Backdrop
      className={cn(
        "fixed inset-0 z-50 bg-black/50 backdrop-blur-sm",
        "data-[starting-style]:opacity-0 data-[ending-style]:opacity-0 transition-opacity duration-200",
        className
      )}
      {...props}
    />
  );
}

function DialogPopup({ className, ...props }: DialogPrimitive.Popup.Props) {
  return (
    <DialogPrimitive.Popup
      className={cn(
        "fixed left-1/2 top-1/2 z-50 w-full max-w-lg -translate-x-1/2 -translate-y-1/2",
        "rounded-xl border bg-white shadow-xl",
        "data-[starting-style]:opacity-0 data-[starting-style]:scale-95",
        "data-[ending-style]:opacity-0 data-[ending-style]:scale-95",
        "transition-all duration-200",
        className
      )}
      {...props}
    />
  );
}

function DialogTitle({ className, ...props }: DialogPrimitive.Title.Props) {
  return (
    <DialogPrimitive.Title
      className={cn("text-lg font-semibold text-gray-900", className)}
      {...props}
    />
  );
}

function DialogClose(props: DialogPrimitive.Close.Props) {
  return <DialogPrimitive.Close {...props} />;
}

export {
  DialogRoot,
  DialogTrigger,
  DialogPortal,
  DialogBackdrop,
  DialogPopup,
  DialogTitle,
  DialogClose,
};
```

- [ ] **Step 2: Verify build**

```bash
cd dashboard && npm run build 2>&1 | tail -10
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/components/ui/dialog.tsx
git commit -m "feat(dashboard): add Dialog component using @base-ui/react"
```

---

## Task 6: TagInput component

**Files:**
- Create: `dashboard/src/components/tag-input.tsx`

- [ ] **Step 1: Create TagInput**

Create `dashboard/src/components/tag-input.tsx`:
```typescript
"use client";

import { useState, KeyboardEvent } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

interface TagInputProps {
  value: string[];
  onChange: (tags: string[]) => void;
  placeholder?: string;
  className?: string;
  /** If provided, only these values are selectable (shows as chips to click) */
  suggestions?: string[];
}

export function TagInput({ value, onChange, placeholder, className, suggestions }: TagInputProps) {
  const [input, setInput] = useState("");

  function addTag(tag: string) {
    const t = tag.trim();
    if (t && !value.includes(t)) {
      onChange([...value, t]);
    }
    setInput("");
  }

  function removeTag(tag: string) {
    onChange(value.filter((v) => v !== tag));
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addTag(input);
    } else if (e.key === "Backspace" && input === "" && value.length > 0) {
      onChange(value.slice(0, -1));
    }
  }

  const remaining = suggestions?.filter((s) => !value.includes(s));

  return (
    <div className={cn("space-y-2", className)}>
      <div className="flex min-h-9 flex-wrap items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-2 py-1.5 focus-within:ring-2 focus-within:ring-blue-500/20 focus-within:border-blue-400">
        {value.map((tag) => (
          <span
            key={tag}
            className="inline-flex items-center gap-1 rounded-full bg-blue-50 border border-blue-200 px-2 py-0.5 text-xs text-blue-700"
          >
            {tag}
            <button
              type="button"
              onClick={() => removeTag(tag)}
              className="text-blue-400 hover:text-blue-700"
            >
              <X size={10} />
            </button>
          </span>
        ))}
        {!suggestions && (
          <input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={onKeyDown}
            onBlur={() => input && addTag(input)}
            placeholder={value.length === 0 ? placeholder : ""}
            className="flex-1 min-w-[80px] border-none bg-transparent text-sm outline-none placeholder:text-gray-400"
          />
        )}
      </div>
      {remaining && remaining.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {remaining.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => addTag(s)}
              className="rounded-full border border-gray-200 px-2 py-0.5 text-xs text-gray-500 hover:border-blue-300 hover:text-blue-600"
            >
              + {s}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Build to verify**

```bash
cd dashboard && npm run build 2>&1 | tail -10
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/components/tag-input.tsx
git commit -m "feat(dashboard): add TagInput chip component"
```

---

## Task 7: CreateRunDialog component

**Files:**
- Create: `dashboard/src/components/create-run-dialog.tsx`

- [ ] **Step 1: Create the component**

Create `dashboard/src/components/create-run-dialog.tsx`:
```typescript
"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DialogRoot, DialogTrigger, DialogPortal, DialogBackdrop,
  DialogPopup, DialogTitle, DialogClose,
} from "@/components/ui/dialog";
import { TagInput } from "@/components/tag-input";
import { createRun } from "@/lib/api";
import type { CreateRunRequest } from "@/lib/types";

interface Props {
  onCreated: () => void;
}

export function CreateRunDialog({ onCreated }: Props) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [name, setName] = useState("");
  const [namespace, setNamespace] = useState("kube-agent-helper");
  const [scope, setScope] = useState<"namespace" | "cluster">("namespace");
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [labelSelector, setLabelSelector] = useState<string[]>([]);
  const [skills, setSkills] = useState<string[]>([]);
  const [modelConfigRef, setModelConfigRef] = useState("anthropic-credentials");

  function parseLabelSelector(tags: string[]): Record<string, string> {
    const result: Record<string, string> = {};
    for (const tag of tags) {
      const [k, v] = tag.split("=");
      if (k && v !== undefined) result[k] = v;
    }
    return result;
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!modelConfigRef) { setError("modelConfigRef 不能为空"); return; }

    const body: CreateRunRequest = {
      name: name || undefined,
      namespace,
      target: {
        scope,
        namespaces: scope === "namespace" ? namespaces : undefined,
        labelSelector: labelSelector.length > 0 ? parseLabelSelector(labelSelector) : undefined,
      },
      skills: skills.length > 0 ? skills : undefined,
      modelConfigRef,
    };

    setLoading(true);
    try {
      await createRun(body);
      setOpen(false);
      onCreated();
      // reset
      setName(""); setNamespaces([]); setLabelSelector([]); setSkills([]);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "创建失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <DialogRoot open={open} onOpenChange={setOpen}>
      <DialogTrigger render={
        <Button size="sm">
          <Plus className="size-4" />
          创建 Run
        </Button>
      } />
      <DialogPortal>
        <DialogBackdrop />
        <DialogPopup>
          <form onSubmit={handleSubmit} className="p-6 space-y-4 max-h-[85vh] overflow-y-auto">
            <DialogTitle>新建 DiagnosticRun</DialogTitle>

            {error && (
              <div className="rounded-lg bg-red-50 border border-red-200 px-3 py-2 text-sm text-red-700">
                {error}
              </div>
            )}

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">
                Name <span className="font-normal normal-case text-gray-400">（留空自动生成）</span>
              </label>
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="run-20260415"
                className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Namespace *</label>
              <input
                required
                value={namespace}
                onChange={(e) => setNamespace(e.target.value)}
                placeholder="kube-agent-helper"
                className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Scope *</label>
              <div className="flex gap-2">
                {(["namespace", "cluster"] as const).map((s) => (
                  <button
                    key={s}
                    type="button"
                    onClick={() => setScope(s)}
                    className={`rounded-lg px-4 py-1.5 text-sm font-medium transition-colors ${
                      scope === s ? "bg-blue-600 text-white" : "border border-gray-200 text-gray-600 hover:border-blue-300"
                    }`}
                  >
                    {s}
                  </button>
                ))}
              </div>
              <p className="text-xs text-gray-400">
                <strong className="text-gray-500">namespace</strong> — 只扫描指定 namespace &nbsp;·&nbsp;
                <strong className="text-gray-500">cluster</strong> — 扫描整个集群
              </p>
            </div>

            {scope === "namespace" && (
              <div className="space-y-1.5">
                <label className="text-xs font-medium uppercase tracking-wide text-gray-500">
                  Namespaces <span className="font-normal normal-case text-gray-400">（留空 = 全部）</span>
                </label>
                <TagInput
                  value={namespaces}
                  onChange={setNamespaces}
                  placeholder="输入 namespace，回车添加"
                />
              </div>
            )}

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">
                Label Selector <span className="font-normal normal-case text-gray-400">（可选）</span>
              </label>
              <p className="text-xs text-gray-400">
                只诊断带指定 label 的资源，如 <code className="bg-gray-100 px-1 rounded">app=nginx</code>，留空 = 不过滤
              </p>
              <TagInput
                value={labelSelector}
                onChange={setLabelSelector}
                placeholder="输入 key=value，回车添加"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">
                Skills <span className="font-normal normal-case text-gray-400">（留空 = 全部启用的 skill）</span>
              </label>
              <TagInput
                value={skills}
                onChange={setSkills}
                placeholder="输入 skill 名称，回车添加"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">ModelConfigRef *</label>
              <input
                required
                value={modelConfigRef}
                onChange={(e) => setModelConfigRef(e.target.value)}
                placeholder="anthropic-credentials"
                className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20"
              />
              <p className="text-xs text-gray-400">引用集群中 ModelConfig CR 的名称</p>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <DialogClose render={
                <Button type="button" variant="outline" disabled={loading}>取消</Button>
              } />
              <Button type="submit" disabled={loading}>
                {loading ? "创建中..." : "创建 Run"}
              </Button>
            </div>
          </form>
        </DialogPopup>
      </DialogPortal>
    </DialogRoot>
  );
}
```

- [ ] **Step 2: Build**

```bash
cd dashboard && npm run build 2>&1 | tail -10
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/components/create-run-dialog.tsx
git commit -m "feat(dashboard): add CreateRunDialog component"
```

---

## Task 8: CreateSkillDialog component

**Files:**
- Create: `dashboard/src/components/create-skill-dialog.tsx`

- [ ] **Step 1: Create the component**

Create `dashboard/src/components/create-skill-dialog.tsx`:
```typescript
"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DialogRoot, DialogTrigger, DialogPortal, DialogBackdrop,
  DialogPopup, DialogTitle, DialogClose,
} from "@/components/ui/dialog";
import { TagInput } from "@/components/tag-input";
import { createSkill } from "@/lib/api";
import type { CreateSkillRequest } from "@/lib/types";

const AVAILABLE_TOOLS = ["kubectl_get", "kubectl_describe", "events_list", "logs_get"];
const DIMENSIONS = ["health", "security", "cost", "reliability"] as const;

interface Props {
  onCreated: () => void;
}

export function CreateSkillDialog({ onCreated }: Props) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [name, setName] = useState("");
  const [namespace, setNamespace] = useState("kube-agent-helper");
  const [dimension, setDimension] = useState<CreateSkillRequest["dimension"]>("health");
  const [description, setDescription] = useState("");
  const [prompt, setPrompt] = useState("");
  const [tools, setTools] = useState<string[]>([]);
  const [requiresData, setRequiresData] = useState<string[]>([]);
  const [enabled, setEnabled] = useState(true);
  const [priority, setPriority] = useState(100);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (tools.length === 0) { setError("至少选择一个 Tool"); return; }

    const body: CreateSkillRequest = {
      name, namespace, dimension, description, prompt, tools,
      requiresData: requiresData.length > 0 ? requiresData : undefined,
      enabled, priority,
    };

    setLoading(true);
    try {
      await createSkill(body);
      setOpen(false);
      onCreated();
      setName(""); setDescription(""); setPrompt(""); setTools([]); setRequiresData([]);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "创建失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <DialogRoot open={open} onOpenChange={setOpen}>
      <DialogTrigger render={
        <Button size="sm">
          <Plus className="size-4" />
          创建 Skill
        </Button>
      } />
      <DialogPortal>
        <DialogBackdrop />
        <DialogPopup>
          <form onSubmit={handleSubmit} className="p-6 space-y-4 max-h-[85vh] overflow-y-auto">
            <DialogTitle>新建 DiagnosticSkill</DialogTitle>

            {error && (
              <div className="rounded-lg bg-red-50 border border-red-200 px-3 py-2 text-sm text-red-700">
                {error}
              </div>
            )}

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-xs font-medium uppercase tracking-wide text-gray-500">
                  Name * <span className="font-normal normal-case text-gray-400">（小写+连字符）</span>
                </label>
                <input
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="my-security-analyst"
                  pattern="[a-z0-9][a-z0-9\-]*"
                  className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20"
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Namespace *</label>
                <input
                  required
                  value={namespace}
                  onChange={(e) => setNamespace(e.target.value)}
                  placeholder="kube-agent-helper"
                  className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20"
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Dimension *</label>
              <div className="flex gap-2 flex-wrap">
                {DIMENSIONS.map((d) => (
                  <button
                    key={d}
                    type="button"
                    onClick={() => setDimension(d)}
                    className={`rounded-lg px-4 py-1.5 text-sm font-medium capitalize transition-colors ${
                      dimension === d ? "bg-blue-600 text-white" : "border border-gray-200 text-gray-600 hover:border-blue-300"
                    }`}
                  >
                    {d}
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Description *</label>
              <input
                required
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="分析 Pod 的健康状态"
                className="w-full rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Prompt *</label>
              <textarea
                required
                rows={4}
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder="你是一个 K8s 健康分析专家..."
                className="w-full resize-y rounded-lg border border-gray-200 px-3 py-1.5 text-sm outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-500/20"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">Tools * （至少一个）</label>
              <TagInput
                value={tools}
                onChange={setTools}
                suggestions={AVAILABLE_TOOLS}
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-medium uppercase tracking-wide text-gray-500">
                RequiresData <span className="font-normal normal-case text-gray-400">（可选）</span>
              </label>
              <p className="text-xs text-gray-400">
                声明 skill 需要的外部数据源，如 <code className="bg-gray-100 px-1 rounded">workflows</code>、<code className="bg-gray-100 px-1 rounded">logs</code>
              </p>
              <TagInput
                value={requiresData}
                onChange={setRequiresData}
                placeholder="输入数据源，回车添加"
              />
            </div>

            <div className="flex items-center justify-between">
              <label className="flex items-center gap-2 cursor-pointer">
                <button
                  type="button"
                  role="switch"
                  aria-checked={enabled}
                  onClick={() => setEnabled(!enabled)}
                  className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                    enabled ? "bg-blue-600" : "bg-gray-200"
                  }`}
                >
                  <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow transition-transform ${
                    enabled ? "translate-x-4" : "translate-x-1"
                  }`} />
                </button>
                <span className="text-sm text-gray-700">Enabled</span>
              </label>
              <label className="flex items-center gap-2">
                <span className="text-xs text-gray-500">Priority</span>
                <input
                  type="number"
                  value={priority}
                  onChange={(e) => setPriority(Number(e.target.value))}
                  className="w-16 rounded-lg border border-gray-200 px-2 py-1 text-center text-sm outline-none focus:border-blue-400"
                />
              </label>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <DialogClose render={
                <Button type="button" variant="outline" disabled={loading}>取消</Button>
              } />
              <Button type="submit" disabled={loading}>
                {loading ? "创建中..." : "创建 Skill"}
              </Button>
            </div>
          </form>
        </DialogPopup>
      </DialogPortal>
    </DialogRoot>
  );
}
```

- [ ] **Step 2: Build**

```bash
cd dashboard && npm run build 2>&1 | tail -10
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/components/create-skill-dialog.tsx
git commit -m "feat(dashboard): add CreateSkillDialog component"
```

---

## Task 9: Wire up Runs page with button + stats bar

**Files:**
- Modify: `dashboard/src/app/page.tsx`

- [ ] **Step 1: Update page.tsx**

Replace the entire `dashboard/src/app/page.tsx`:
```typescript
"use client";

import Link from "next/link";
import { useRuns, useSkills } from "@/lib/api";
import { PhaseBadge } from "@/components/phase-badge";
import { CreateRunDialog } from "@/components/create-run-dialog";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

function formatTime(iso: string | null): string {
  if (!iso) return "-";
  return new Date(iso).toLocaleString();
}

function duration(start: string | null, end: string | null): string {
  if (!start) return "-";
  const s = new Date(start).getTime();
  const e = end ? new Date(end).getTime() : Date.now();
  const sec = Math.round((e - s) / 1000);
  if (sec < 60) return `${sec}s`;
  return `${Math.floor(sec / 60)}m ${sec % 60}s`;
}

export default function RunsPage() {
  const { data: runs, error, isLoading, mutate } = useRuns();
  const { data: skills } = useSkills();

  const total = runs?.length ?? 0;
  const running = runs?.filter((r) => r.Status === "Running").length ?? 0;
  const succeeded = runs?.filter((r) => r.Status === "Succeeded").length ?? 0;
  const failed = runs?.filter((r) => r.Status === "Failed").length ?? 0;
  const enabledSkills = skills?.filter((s) => s.Enabled).length ?? 0;

  if (isLoading) return <p className="text-gray-500">Loading runs...</p>;
  if (error) return <p className="text-red-600">Failed to load runs.</p>;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Diagnostic Runs</h1>
        <CreateRunDialog onCreated={() => mutate()} />
      </div>

      {/* Stats bar */}
      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div className="rounded-lg border bg-white px-4 py-3">
          <p className="text-xs text-gray-500 uppercase tracking-wide">Total Runs</p>
          <p className="mt-1 text-2xl font-semibold">{total}</p>
        </div>
        <div className="rounded-lg border bg-white px-4 py-3">
          <p className="text-xs text-gray-500 uppercase tracking-wide">Running</p>
          <p className="mt-1 text-2xl font-semibold text-blue-600">{running}</p>
        </div>
        <div className="rounded-lg border bg-white px-4 py-3">
          <p className="text-xs text-gray-500 uppercase tracking-wide">Succeeded</p>
          <p className="mt-1 text-2xl font-semibold text-green-600">{succeeded}</p>
        </div>
        <div className="rounded-lg border bg-white px-4 py-3">
          <p className="text-xs text-gray-500 uppercase tracking-wide">Failed</p>
          <p className="mt-1 text-2xl font-semibold text-red-600">{failed}</p>
        </div>
      </div>

      {runs && runs.length === 0 ? (
        <p className="text-gray-500">No runs yet.</p>
      ) : (
        <div className="rounded-lg border bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>ID</TableHead>
                <TableHead>Phase</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Duration</TableHead>
                <TableHead>Target</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs?.map((run) => {
                let target = "-";
                try {
                  const t = JSON.parse(run.TargetJSON);
                  target = t.namespaces?.join(", ") || t.scope || "-";
                } catch { /* ignore */ }
                return (
                  <TableRow key={run.ID}>
                    <TableCell>
                      <Link href={`/runs/${run.ID}`} className="font-mono text-sm text-blue-600 hover:underline">
                        {run.ID.slice(0, 8)}...
                      </Link>
                    </TableCell>
                    <TableCell><PhaseBadge phase={run.Status} /></TableCell>
                    <TableCell className="text-sm text-gray-600">{formatTime(run.CreatedAt)}</TableCell>
                    <TableCell className="text-sm text-gray-600">{duration(run.StartedAt, run.CompletedAt)}</TableCell>
                    <TableCell className="text-sm text-gray-600">{target}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}

      <p className="mt-3 text-xs text-gray-400">{enabledSkills} skill{enabledSkills !== 1 ? "s" : ""} active</p>
    </div>
  );
}
```

- [ ] **Step 2: Build**

```bash
cd dashboard && npm run build 2>&1 | tail -10
```
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/app/page.tsx
git commit -m "feat(dashboard): add CreateRun button and stats bar to runs page"
```

---

## Task 10: Wire up Skills page with button + stats bar

**Files:**
- Modify: `dashboard/src/app/skills/page.tsx`

- [ ] **Step 1: Update skills/page.tsx**

Replace the entire `dashboard/src/app/skills/page.tsx`:
```typescript
"use client";

import { useSkills } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { CreateSkillDialog } from "@/components/create-skill-dialog";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";

export default function SkillsPage() {
  const { data: skills, error, isLoading, mutate } = useSkills();

  const total = skills?.length ?? 0;
  const enabled = skills?.filter((s) => s.Enabled).length ?? 0;
  const crSkills = skills?.filter((s) => s.Source === "cr").length ?? 0;
  const builtinSkills = skills?.filter((s) => s.Source === "builtin").length ?? 0;

  if (isLoading) return <p className="text-gray-500">Loading skills...</p>;
  if (error) return <p className="text-red-600">Failed to load skills.</p>;

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Skills</h1>
        <CreateSkillDialog onCreated={() => mutate()} />
      </div>

      {/* Stats bar */}
      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div className="rounded-lg border bg-white px-4 py-3">
          <p className="text-xs text-gray-500 uppercase tracking-wide">Total</p>
          <p className="mt-1 text-2xl font-semibold">{total}</p>
        </div>
        <div className="rounded-lg border bg-white px-4 py-3">
          <p className="text-xs text-gray-500 uppercase tracking-wide">Enabled</p>
          <p className="mt-1 text-2xl font-semibold text-green-600">{enabled}</p>
        </div>
        <div className="rounded-lg border bg-white px-4 py-3">
          <p className="text-xs text-gray-500 uppercase tracking-wide">Builtin</p>
          <p className="mt-1 text-2xl font-semibold text-gray-600">{builtinSkills}</p>
        </div>
        <div className="rounded-lg border bg-white px-4 py-3">
          <p className="text-xs text-gray-500 uppercase tracking-wide">Custom (CR)</p>
          <p className="mt-1 text-2xl font-semibold text-blue-600">{crSkills}</p>
        </div>
      </div>

      {skills && skills.length === 0 ? (
        <p className="text-gray-500">No skills registered.</p>
      ) : (
        <div className="rounded-lg border bg-white">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Dimension</TableHead>
                <TableHead>Source</TableHead>
                <TableHead>Enabled</TableHead>
                <TableHead>Priority</TableHead>
                <TableHead>Tools</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {skills?.map((skill) => {
                let tools: string[] = [];
                try { tools = JSON.parse(skill.ToolsJSON); } catch { /* ignore */ }
                return (
                  <TableRow key={skill.ID}>
                    <TableCell className="font-mono text-sm font-medium">{skill.Name}</TableCell>
                    <TableCell><Badge variant="outline" className="capitalize">{skill.Dimension}</Badge></TableCell>
                    <TableCell><Badge variant={skill.Source === "cr" ? "default" : "secondary"}>{skill.Source}</Badge></TableCell>
                    <TableCell>{skill.Enabled ? <span className="text-green-600">Yes</span> : <span className="text-gray-400">No</span>}</TableCell>
                    <TableCell className="text-sm text-gray-600">{skill.Priority}</TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {tools.map((tool) => (
                          <Badge key={tool} variant="outline" className="text-xs">{tool}</Badge>
                        ))}
                      </div>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Full build + lint**

```bash
cd dashboard && npm run lint && npm run build 2>&1 | tail -15
```
Expected: no lint errors, build PASS

- [ ] **Step 3: Run Go tests**

```bash
cd /path/to/kube-agent-helper && go test ./... -count=1 -timeout=60s 2>&1 | tail -20
```
Expected: all tests PASS

- [ ] **Step 4: Final commit**

```bash
git add dashboard/src/app/skills/page.tsx
git commit -m "feat(dashboard): add CreateSkill button and stats bar to skills page"
```
