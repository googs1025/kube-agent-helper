# Design: Dashboard Create CRD & Skill Forms

**Date:** 2026-04-15
**Status:** Approved

---

## Overview

Add "create" capability to the Next.js dashboard so users can create `DiagnosticRun` and `DiagnosticSkill` K8s CRDs directly from the UI, without needing kubectl.

---

## Architecture

### Approach: HTTP Server → K8s Client → CRD

The HTTP server (`internal/controller/httpserver/server.go`) receives POST requests from the dashboard, uses an in-cluster `k8s.io/client-go` client to create CRs, and returns the created object. The existing reconcilers (`run_reconciler.go`, `skill_reconciler.go`) pick up the new CRs and process them as usual — identical to `kubectl apply`.

```
Browser → POST /api/runs
           ↓
      HTTP Server (k8s client injected)
           ↓
      client.Create(&DiagnosticRun{...})
           ↓
      K8s API Server
           ↓
      run_reconciler → store.CreateRun → diagnostic pipeline
```

---

## Backend Changes

### 1. Inject k8s client into HTTP Server

`Server` struct gains a `k8sClient sigs.k8s.io/controller-runtime/pkg/client.Client` field.
In `cmd/controller/main.go`, the already-initialized `mgr.GetClient()` is passed to `httpserver.New()`.

### 2. `POST /api/runs`

**Request body:**
```json
{
  "name": "run-20260415",          // optional, auto-generated if empty
  "namespace": "kube-agent-helper",
  "target": {
    "scope": "namespace",
    "namespaces": ["default", "kube-system"],
    "labelSelector": {"app": "nginx"}  // optional
  },
  "skills": ["cost-analyst"],         // optional, empty = all enabled
  "modelConfigRef": "anthropic-credentials"
}
```

**Behaviour:**
- If `name` is empty, generate `run-<yyyymmdd>-<random4>` (e.g. `run-20260415-a3f2`)
- Create `DiagnosticRun` CR in given namespace
- Return `201 Created` with the created CR as JSON

### 3. `POST /api/skills`

**Request body:**
```json
{
  "name": "my-security-analyst",
  "namespace": "kube-agent-helper",
  "dimension": "security",
  "description": "...",
  "prompt": "...",
  "tools": ["kubectl_get", "kubectl_describe"],
  "requiresData": [],               // optional
  "enabled": true,
  "priority": 100                   // optional, default 100
}
```

**Behaviour:**
- Create `DiagnosticSkill` CR in given namespace
- skill_reconciler upserts into store automatically
- Return `201 Created` with created CR as JSON

### 4. RBAC

Add to Helm `rbac.yaml` — ClusterRole rules for the controller ServiceAccount:
```yaml
- apiGroups: ["k8sai.io"]
  resources: ["diagnosticruns", "diagnosticskills"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```
(Already has `watch`; need to confirm `create` is present.)

---

## Frontend Changes

### 1. API client (`src/lib/api.ts`)

Add two mutate functions:
```ts
createRun(body: CreateRunRequest): Promise<DiagnosticRun>
createSkill(body: CreateSkillRequest): Promise<DiagnosticSkill>
```

### 2. Runs page (`src/app/page.tsx`)

- Header row: `<h1>` + right-aligned `<Button>+ 创建 Run</Button>`
- Button opens `<CreateRunDialog>` modal
- On success: call `mutate()` (SWR revalidate) to refresh list

### 3. Skills page (`src/app/skills/page.tsx`)

- Header row: `<h1>` + right-aligned `<Button>+ 创建 Skill</Button>`
- Button opens `<CreateSkillDialog>` modal
- On success: call `mutate()` to refresh list

### 4. `<CreateRunDialog>` component (`src/components/create-run-dialog.tsx`)

Fields (all in a shadcn `<Dialog>`):

| Field | Type | Required | Notes |
|---|---|---|---|
| name | text input | No | placeholder "auto-generated" |
| namespace | text input | Yes | default "kube-agent-helper" |
| scope | toggle buttons | Yes | namespace \| cluster |
| namespaces | tag input | Cond. | shown when scope=namespace |
| labelSelector | tag input (key=value) | No | Enter to add, × to remove |
| skills | tag input | No | from existing skill names |
| modelConfigRef | text input | Yes | — |

### 5. `<CreateSkillDialog>` component (`src/components/create-skill-dialog.tsx`)

Fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| name | text input | Yes | K8s name format validation |
| namespace | text input | Yes | default "kube-agent-helper" |
| dimension | button group | Yes | health/security/cost/reliability |
| description | text input | Yes | — |
| prompt | textarea | Yes | resizable |
| tools | tag chips | Yes | fixed list: kubectl_get, kubectl_describe, events_list, logs_get |
| requiresData | tag input | No | free text, Enter to add |
| enabled | toggle | No | default true |
| priority | number input | No | default 100 |

### 6. Shared `<TagInput>` component (`src/components/tag-input.tsx`)

Reusable chip input: Enter/comma to add, × to remove. Used for namespaces, labelSelector, skills, requiresData.

---

## Types (`src/lib/types.ts`)

Add:
```ts
export interface CreateRunRequest {
  name?: string;
  namespace: string;
  target: { scope: string; namespaces?: string[]; labelSelector?: Record<string, string> };
  skills?: string[];
  modelConfigRef: string;
}

export interface CreateSkillRequest {
  name: string;
  namespace: string;
  dimension: string;
  description: string;
  prompt: string;
  tools: string[];
  requiresData?: string[];
  enabled: boolean;
  priority?: number;
}
```

---

## Error Handling

- Backend validation: missing required fields → `400 Bad Request` with message
- K8s conflict (name already exists) → `409 Conflict`
- Frontend: show inline error banner inside dialog on failure; keep dialog open so user can fix

---

## Out of Scope

- Creating `DiagnosticFix` CRs from UI (fixes are generated from run findings, not manually authored)
- Editing or deleting existing CRs from UI
- ModelConfig CR creation