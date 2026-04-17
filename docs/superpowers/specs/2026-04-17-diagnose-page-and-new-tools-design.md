# Diagnose Page + New MCP Tools Design

> Date: 2026-04-17
> Status: Draft

## Background

Users come to the platform because they observed a symptom in monitoring/alerting (CPU high, memory high, slow requests, pod crash) on a specific resource (Pod/Deployment). The current dashboard is admin-oriented (Runs/Skills/Fixes management). We need a user-facing diagnostic page that lets users express "what's wrong with this thing" and get structured results.

## Goals

1. Add a `/diagnose` page — symptom-driven diagnostic entry for end users
2. Add a `/diagnose/[id]` result page — user-friendly findings view sorted by severity
3. Add 5 new MCP tools to close diagnostic gaps across 6 major symptom categories
4. Add 1 lightweight backend API for resource autocomplete
5. Keep existing admin pages and backend architecture unchanged

## Non-Goals

- User authentication / RBAC (future)
- Custom MCP tool plugin mechanism (deferred — not a real need yet)
- Changing DiagnosticRun CRD schema
- Modifying existing admin pages

---

## Part 1: New MCP Tools

### Current Tool Coverage

| Tool | Purpose |
|------|---------|
| `kubectl_get` | Get/list any K8s resource |
| `kubectl_describe` | Describe a resource + events |
| `kubectl_logs` | Pod container logs |
| `events_list` | List/filter K8s events |
| `top_pods` | Pod CPU/memory usage |
| `top_nodes` | Node CPU/memory usage |
| `list_api_resources` | List available API types |
| `prometheus_query` | Execute PromQL |
| `kubectl_explain` | OpenAPI schema |

### Gap Analysis by Symptom

| Symptom | Current Coverage | Missing |
|---------|-----------------|---------|
| CPU high | top_pods, describe, logs | HPA status, rollout history |
| Memory high / OOMKill | top_pods, describe, events | Node conditions/capacity |
| Requests slow / service down | kubectl_get (Endpoints) | NetworkPolicy analysis, active alerts |
| Pod won't start | describe, events, logs | PVC status, node scheduling capacity |
| Rollout stuck | events | Rollout status, ReplicaSet history |
| Node NotReady | top_nodes, events | Node conditions/capacity summary |
| Scaling issues | - | HPA status |

### New Tools (5)

#### 1. `kubectl_rollout_status`

**Purpose:** Show Deployment/StatefulSet rollout status including ReplicaSet history.

**Parameters:**
- `kind` (required): Deployment or StatefulSet
- `name` (required): Resource name
- `namespace` (required): Namespace

**Returns:** JSON with:
- Current rollout status (progressing / complete / degraded)
- Desired vs updated vs ready vs available replicas
- ReplicaSet list (name, replicas, image, creation time) — newest first
- Rollout conditions from the Deployment status

**Implementation:** Use typed client to get the Deployment, then list ReplicaSets with owner reference matching the Deployment. Sort by creation timestamp descending.

**Covers:** Rollout stuck, CPU high (new version regression), pod won't start after deploy.

#### 2. `node_status_summary`

**Purpose:** Show node conditions, capacity, allocated resources, and taints in one call.

**Parameters:**
- `name` (optional): Specific node name. If omitted, return summary for all nodes (limited to 20).
- `labelSelector` (optional): Filter nodes by label.

**Returns:** JSON array with per-node:
- `conditions`: Array of {type, status, reason, message} — highlight non-True Ready, True MemoryPressure/DiskPressure/PIDPressure
- `capacity`: CPU, memory, pods (allocatable)
- `allocated`: Sum of requests from all pods on this node (CPU, memory, pod count)
- `utilization_pct`: allocated / allocatable as percentage
- `taints`: Array of {key, value, effect}
- `unschedulable`: boolean

**Implementation:**
1. List Nodes (typed client)
2. For each node, list Pods with `fieldSelector=spec.nodeName=<node>`
3. Sum pod resource requests
4. Return aggregated view

**Covers:** Node NotReady, OOMKill (node memory pressure), pod scheduling failures, capacity planning.

#### 3. `prometheus_alerts`

**Purpose:** List active alerts from Prometheus/AlertManager.

**Parameters:**
- `state` (optional): Filter by state — "firing", "pending", or "all" (default "firing")
- `labelFilter` (optional): Filter alerts by label match, e.g. "namespace=production,severity=critical"

**Returns:** JSON array of alerts:
- `alertname`, `state`, `severity`
- `labels`: full label set
- `annotations`: summary, description, runbook_url
- `activeAt`: when the alert started firing
- `value`: current metric value (if available)

**Implementation:** Call Prometheus API `/api/v1/alerts` (already have Prometheus client in Deps). Filter by state and labels. Sort by severity (critical > warning > info) then activeAt descending.

**Covers:** All symptom scenarios — the user came from an alert, the LLM should also see what alerts are firing to correlate.

#### 4. `pvc_status`

**Purpose:** List PersistentVolumeClaims with their binding status, capacity, and associated PV/StorageClass.

**Parameters:**
- `namespace` (required): Namespace to check
- `name` (optional): Specific PVC name
- `labelSelector` (optional): Filter by label

**Returns:** JSON array with per-PVC:
- `name`, `namespace`, `phase` (Bound / Pending / Lost)
- `capacity`: requested vs actual
- `storageClass`: name, provisioner
- `volumeName`: bound PV name (if any)
- `accessModes`: ReadWriteOnce, etc.
- `events`: recent events for Pending PVCs (auto-fetched)

**Implementation:** List PVCs via typed client. For any PVC in Pending state, also fetch events filtered by involvedObject. Include StorageClass info via discovery.

**Covers:** Pod won't start (volume mount failure), storage issues.

#### 5. `network_policy_check`

**Purpose:** Analyze NetworkPolicies affecting a specific Pod and show which ingress/egress traffic is allowed or blocked.

**Parameters:**
- `namespace` (required): Namespace of the target Pod
- `podName` (required): Name of the Pod to analyze

**Returns:** JSON with:
- `pod_labels`: the Pod's labels
- `matching_policies`: array of NetworkPolicies whose podSelector matches this Pod
  - `name`: policy name
  - `policy_types`: ["Ingress"] / ["Egress"] / ["Ingress", "Egress"]
  - `ingress_rules`: array of {from, ports} — human-readable
  - `egress_rules`: array of {to, ports} — human-readable
- `default_deny`: whether there's a default-deny policy in this namespace
- `summary`: text description of effective access (e.g. "Only allows ingress from pods with label app=frontend on port 8080. All egress allowed.")

**Implementation:**
1. Get the Pod's labels
2. List all NetworkPolicies in the namespace
3. For each policy, evaluate if its podSelector matches the Pod labels
4. Compile matching rules into a readable summary

**Covers:** Requests slow / service unreachable, network debugging.

### Tool Registration

All 5 tools registered in `RegisterExtension()` in `internal/mcptools/register.go`. This keeps the Core (M5) / Extension (M6) pattern. These become **M7 tools**.

### Skill Prompt Updates

After adding tools, update the builtin skills to reference them:

| Skill | Add Tools |
|-------|-----------|
| `pod-health-analyst` | `kubectl_rollout_status`, `prometheus_alerts` |
| `pod-cost-analyst` | `node_status_summary` |
| `reliability-analyst` | `kubectl_rollout_status`, `pvc_status`, `node_status_summary` |
| `config-drift-analyst` | `network_policy_check` |

---

## Part 2: Diagnose Page (`/diagnose`)

### User Flow

```
User observes "payment-svc CPU high" in Grafana
     │
     ▼
/diagnose page
     │
     ├─ Select namespace: [production ▼]     (autocomplete)
     ├─ Select resource type: ○ Deployment ○ Pod ○ StatefulSet ...
     ├─ Select resource name: [payment-svc ▼]  (autocomplete from API)
     ├─ Select symptoms: ☑ CPU高  ☐ 内存高  ☐ 请求慢 ...
     ├─ Output language: ○ 中文  ○ English
     │
     └─ [开始诊断]
            │
            ├─ Frontend resolves resource labels via API
            ├─ Frontend maps symptoms → skills
            ├─ Frontend calls POST /api/runs with computed params
            │
            ▼
       /diagnose/[runId] — user-friendly result page
```

### Symptom → Skill Mapping (Frontend-only)

```typescript
const SYMPTOM_PRESETS: Record<string, {
  label_zh: string;
  label_en: string;
  skills: string[];
}> = {
  'cpu-high': {
    label_zh: 'CPU 利用率高',
    label_en: 'High CPU usage',
    skills: ['pod-health-analyst', 'pod-cost-analyst'],
  },
  'memory-high': {
    label_zh: '内存使用率高 / OOMKill',
    label_en: 'High memory / OOMKill',
    skills: ['pod-health-analyst', 'pod-cost-analyst'],
  },
  'request-slow': {
    label_zh: '请求延迟高 / 服务不通',
    label_en: 'Slow requests / service unreachable',
    skills: ['pod-health-analyst', 'config-drift-analyst'],
  },
  'pod-restart': {
    label_zh: 'Pod 频繁重启',
    label_en: 'Pod frequent restarts',
    skills: ['pod-health-analyst', 'reliability-analyst'],
  },
  'pod-not-start': {
    label_zh: 'Pod 启动失败',
    label_en: 'Pod failed to start',
    skills: ['pod-health-analyst', 'config-drift-analyst', 'reliability-analyst'],
  },
  'scaling-issue': {
    label_zh: '扩缩容异常',
    label_en: 'Scaling issues (HPA)',
    skills: ['pod-cost-analyst', 'reliability-analyst'],
  },
  'rollout-stuck': {
    label_zh: '滚动更新卡住',
    label_en: 'Rollout stuck',
    skills: ['pod-health-analyst', 'reliability-analyst'],
  },
  'full-check': {
    label_zh: '全面体检',
    label_en: 'Full health check',
    skills: [],  // empty = all enabled skills
  },
};
```

### Resource Autocomplete Flow

1. User selects namespace → frontend calls `GET /api/k8s/namespaces` to populate namespace dropdown
2. User selects resource type → frontend calls `GET /api/k8s/resources?namespace=X&kind=Deployment` to get resource list
3. User selects resource name → frontend calls `GET /api/k8s/resources?namespace=X&kind=Deployment&name=Y` to get the resource's labels
4. On submit, frontend uses those labels as `labelSelector` in the `CreateRunRequest`

### Label Resolution Logic

```typescript
async function resolveLabels(ns: string, kind: string, name: string): Promise<Record<string, string>> {
  const res = await fetch(`/api/k8s/resources?namespace=${ns}&kind=${kind}&name=${name}`);
  const data = await res.json();

  // Strategy: prefer well-known labels, fall back to matchLabels
  const labels = data.metadata?.labels || {};
  const matchLabels = data.spec?.selector?.matchLabels || {};

  // For Deployment/StatefulSet: use spec.selector.matchLabels (most precise)
  if (kind === 'Deployment' || kind === 'StatefulSet') {
    return matchLabels;
  }
  // For Pod: use pod labels directly, prefer app/app.kubernetes.io/name
  const appLabel = labels['app'] || labels['app.kubernetes.io/name'];
  if (appLabel) {
    return { app: appLabel };
  }
  return labels;
}
```

### CreateRunRequest Construction

```typescript
function buildDiagnoseRequest(form: DiagnoseForm): CreateRunRequest {
  const allSkills = [...new Set(
    form.symptoms.flatMap(s => SYMPTOM_PRESETS[s].skills)
  )];

  return {
    namespace: 'kube-agent-helper',      // Run CR lives in controller ns
    target: {
      scope: 'namespace',
      namespaces: [form.namespace],
      labelSelector: form.resolvedLabels, // from resolveLabels()
    },
    skills: allSkills.length > 0 ? allSkills : undefined,
    modelConfigRef: 'anthropic-credentials',
    outputLanguage: form.language,
  };
}
```

### Recent Diagnoses Section

Below the form, show the user's recent diagnoses. This reuses `GET /api/runs` and renders the latest 5 runs in a compact card format:

```
payment-svc (CPU高, 内存高)     3分钟前    ✅ 发现 3 个问题
order-svc (请求慢)              1小时前    ✅ 发现 1 个问题
全面体检 / production           昨天       ✅ 发现 7 个问题
```

Implementation: filter runs list, parse TargetJSON to extract resource info. The symptom info is not stored in the backend — use a naming convention for the Run name: `diagnose-{resource}-{symptoms}-{random}` (e.g. `diagnose-payment-svc-cpu-high-ab3d`). The diagnose page parses this prefix to display symptom labels. Runs created from the admin "Create Run" page won't have this prefix and won't appear in the recent diagnoses section (filter by `name.startsWith('diagnose-')`).

---

## Part 3: Diagnose Result Page (`/diagnose/[id]`)

### Design Principles

- **Severity-first ordering** — critical → high → medium → low
- **Hide internal concepts** — no "dimension" label visible to user
- **Highlight actionable findings** — suggestion and fix button prominent
- **Compact cards** — each finding is a card, not a section

### Page Layout

```
┌─ 诊断报告 ─────────────────────────────────────────────┐
│                                                         │
│  📋 payment-svc 诊断报告              状态: ● 诊断完成   │
│  症状: CPU 利用率高, 内存使用率高                         │
│  耗时: 45s  |  发现: 5 个问题                            │
│                                                         │
│  ── 发现 ──────────────────────────────────────────     │
│                                                         │
│  🔴 Critical ──────────────────────────────────────     │
│  ┌────────────────────────────────────────────────┐    │
│  │ HPA maxReplicas=2 已达上限，无法扩容             │    │
│  │ Pod: production/payment-svc-7d8f                │    │
│  │                                                 │    │
│  │ 当前副本 2/2，HPA 期望 5 个副本但受 maxReplicas  │    │
│  │ 限制无法扩容，导致现有 Pod CPU 持续 >90%。       │    │
│  │                                                 │    │
│  │ 💡 建议: 调高 HPA maxReplicas 至 10，同时检查   │    │
│  │    是否存在代码层面的 CPU 热点。                  │    │
│  │                                     [生成修复]   │    │
│  └────────────────────────────────────────────────┘    │
│                                                         │
│  🟡 Medium ────────────────────────────────────────     │
│  ┌────────────────────────────────────────────────┐    │
│  │ Pod payment-svc-7d8f 重启 12 次                 │    │
│  │ ...                                             │    │
│  └────────────────────────────────────────────────┘    │
│                                                         │
│  ┌────────────────────────────────────────────────┐    │
│  │ 未配置 PodDisruptionBudget                      │    │
│  │ ...                                             │    │
│  └────────────────────────────────────────────────┘    │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### Severity Grouping & Sorting

```typescript
const SEVERITY_ORDER = { critical: 0, high: 1, medium: 2, low: 3, info: 4 };

function sortFindings(findings: Finding[]): Finding[] {
  return [...findings].sort((a, b) =>
    (SEVERITY_ORDER[a.Severity] ?? 99) - (SEVERITY_ORDER[b.Severity] ?? 99)
  );
}

// Group consecutive findings by severity for section headers
function groupBySeverity(findings: Finding[]): Map<string, Finding[]> {
  const sorted = sortFindings(findings);
  const groups = new Map<string, Finding[]>();
  for (const f of sorted) {
    const group = groups.get(f.Severity) || [];
    group.push(f);
    groups.set(f.Severity, group);
  }
  return groups;
}
```

### Severity Visual Treatment

| Severity | Color | Icon | Badge Style |
|----------|-------|------|-------------|
| critical | red-600 | 🔴 | bg-red-100 text-red-800 border-red-300 |
| high | orange-500 | 🟠 | bg-orange-100 text-orange-800 border-orange-300 |
| medium | yellow-500 | 🟡 | bg-yellow-100 text-yellow-800 border-yellow-300 |
| low | blue-500 | 🔵 | bg-blue-100 text-blue-800 border-blue-300 |
| info | gray-400 | ⚪ | bg-gray-100 text-gray-600 border-gray-200 |

### Finding Card Components

Each finding card contains:
1. **Title** — bold, with severity badge
2. **Resource** — `Kind: Namespace/Name` in muted text
3. **Description** — the LLM's analysis
4. **Suggestion box** — light background, highlighted with 💡 icon
5. **Action button** — "Generate Fix" or "View Fix →" (reuse existing logic from `/runs/[id]`)

### Status Header

Shows diagnostic progress in real-time (poll every 5s like existing pages):
- `Pending` → "等待调度..."
- `Running` → "诊断中..." with spinner
- `Succeeded` → "诊断完成" with finding count
- `Failed` → "诊断失败" with error message

### i18n

Reuse existing i18n pattern from the project. All labels have zh/en variants.

---

## Part 4: Backend Addition

### New API Endpoint: `GET /api/k8s/resources`

A lightweight read-only proxy to support resource autocomplete on the diagnose page.

**Query Parameters:**
- `kind` (required): "Namespace", "Deployment", "Pod", "StatefulSet", "DaemonSet"
- `namespace` (optional): Required for non-Namespace kinds
- `name` (optional): If provided, return single resource with full metadata (for label resolution)

**Response:**

When `name` is omitted (list mode):
```json
[
  { "name": "payment-svc", "namespace": "production" },
  { "name": "order-svc", "namespace": "production" }
]
```

When `name` is provided (detail mode — for label resolution):
```json
{
  "metadata": {
    "name": "payment-svc",
    "namespace": "production",
    "labels": { "app": "payment-svc", "version": "v2" }
  },
  "spec": {
    "selector": {
      "matchLabels": { "app": "payment-svc" }
    },
    "replicas": 3
  }
}
```

When `kind=Namespace` (list namespaces):
```json
[
  { "name": "default" },
  { "name": "production" },
  { "name": "kube-system" }
]
```

**Implementation:** Add `handleAPIK8sResources` method to `httpserver.Server`. Use existing `k8sClient` to list/get resources. Filter out system namespaces (kube-system, kube-public, kube-node-lease) by default for namespace listing.

**Security:** Read-only, uses the same ServiceAccount as the controller (already has `view` ClusterRole).

### Route Registration

```go
srv.mux.HandleFunc("/api/k8s/resources", srv.handleAPIK8sResources)
```

---

## Part 5: Navigation Update

### New Navigation Structure

```
[🔍 Diagnose]  [📋 Runs]  [🧩 Skills]  [🔧 Fixes]  [About]
     ↑ 新增       ↑────── 原有，保持不变 ──────↑
```

- `/diagnose` is the first nav item (primary entry for users)
- Existing pages remain unchanged
- `/diagnose/[id]` is a sub-route, not in nav

---

## File Changes Summary

### New Files

| File | Purpose |
|------|---------|
| `internal/mcptools/rollout_status.go` | kubectl_rollout_status tool |
| `internal/mcptools/node_status.go` | node_status_summary tool |
| `internal/mcptools/prometheus_alerts.go` | prometheus_alerts tool |
| `internal/mcptools/pvc_status.go` | pvc_status tool |
| `internal/mcptools/network_policy.go` | network_policy_check tool |
| `dashboard/src/app/diagnose/page.tsx` | Diagnose form page |
| `dashboard/src/app/diagnose/[id]/page.tsx` | Diagnose result page |
| `dashboard/src/lib/symptoms.ts` | Symptom → skill mapping config |

### Modified Files

| File | Change |
|------|--------|
| `internal/mcptools/register.go` | Register 5 new tools in RegisterExtension |
| `internal/controller/httpserver/server.go` | Add /api/k8s/resources handler |
| `dashboard/src/app/layout.tsx` | Add Diagnose nav item |
| `dashboard/src/lib/api.ts` | Add k8s resource query + diagnose helpers |
| `dashboard/src/lib/types.ts` | Add DiagnoseForm type |
| `skills/pod-health-analyst.md` | Add rollout_status, prometheus_alerts to tools |
| `skills/pod-cost-analyst.md` | Add node_status_summary to tools |
| `skills/reliability-analyst.md` | Add rollout_status, pvc_status, node_status_summary to tools |
| `skills/config-drift-analyst.md` | Add network_policy_check to tools |

### Unchanged

- All existing pages (/, /skills, /fixes, /runs/[id], /fixes/[id], /about)
- CRD types (no schema change)
- Translator, Reconciler, Agent Runtime
- All existing API endpoints
