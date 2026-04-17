# kube-agent-helper

> Kubernetes-native AI diagnostic operator with auto-fix capabilities

**kube-agent-helper** is an AI agent that runs inside your Kubernetes cluster, diagnoses workload issues, and generates actionable fix suggestions. Declare a `DiagnosticRun` CR, and the controller spins up an isolated agent Pod that calls Claude via MCP tools, writes structured findings, and optionally produces `DiagnosticFix` CRs with patches or new resource manifests.

[![CI](https://github.com/googs1025/kube-agent-helper/actions/workflows/ci.yml/badge.svg)](https://github.com/googs1025/kube-agent-helper/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## Features

- **CRD-driven diagnostics** — declare tasks with `DiagnosticRun`; extend with `DiagnosticSkill` CRs
- **4 CRDs** — `DiagnosticRun`, `DiagnosticSkill`, `ModelConfig`, `DiagnosticFix`
- **5 built-in skills** — health, security, cost, reliability, config-drift analysis
- **Claude-powered agentic loop** ��� multi-turn reasoning over live cluster data via MCP
- **Fix generation** — click "Generate Fix" on any finding; a short-lived Pod produces a patch or full resource manifest via LLM
- **Before/After diff** — Fix detail page shows resource diff before applying
- **Human-in-the-loop approval** — Fixes go through `PendingApproval → Approved → Applying → Succeeded` with optional auto-rollback on failure
- **Dashboard** — Next.js web UI with Chinese/English toggle, dark/light theme, stats, create dialogs
- **Per-run output language** — `spec.outputLanguage: zh|en` controls whether findings are in Chinese or English
- **Pod status capture** — reconciler detects ImagePullBackOff, CrashLoopBackOff and writes them to `status.message`
- **Optional timeout** — `spec.timeoutSeconds` on DiagnosticRun; nil = no timeout
- **Minimal RBAC** — Translator auto-generates least-privilege ServiceAccount per run
- **SQLite persistence** — findings and fixes stored locally; no external database required

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  User                                                                       │
│  kubectl apply  │  Dashboard (Next.js :3000)  │  REST API (:8080)           │
└────────┬─────────────────┬──────────────────────────┬───────────────────────┘
         │ CR              │ /api/*                    │
         ▼                 ▼                           │
┌─────────────────────────────────────────────────────────────────────────────┐
│  Controller (Go binary)                                                     │
│                                                                             │
│  ┌─────────────────┐  ┌──────────────┐  ┌──────────────────────────────┐  │
│  │ Run Reconciler   │  │ Fix Reconciler│  │ HTTP Server                  │  │
│  │ Skill Reconciler │  │              │  │  /api/runs (CRUD)            │  │
│  │ ModelConfig Ctrl  │  │ apply patch  │  │  /api/skills                 │  │
│  │                   │  │ create resource│ │  /api/fixes (list/approve)   │  │
│  │ Pod status capture│  │ auto-rollback│  │  /api/findings/*/generate-fix│  │
│  │ timeout check     │  │              │  │  /internal/runs/*/findings   │  │
│  └────────┬──────────┘  └──────────────┘  │  /internal/fixes             │  │
│           │                                └──────────────────────────────┘  │
│           │ Translator                                                      │
│           ▼                                                                 │
│  ┌─────────────────────────────────────────────────┐                       │
│  │ SQLite (diagnostic_runs, findings, skills, fixes)│                       │
│  └─────────────────────────────────────────────────┘                       │
└────────────┬────────────────────────────────────┬───────────────────────────┘
             │ creates Job                        │ creates Job
             ▼                                    ▼
┌──────────────────────┐        ┌──────────────────────────────┐
│  Diagnostic Agent Pod │        │  Fix Generator Pod            │
│  python -m runtime.main│       │  python -m runtime.fix_main   │
│                        │       │                                │
│  Multi-turn LLM loop   │       │  Single LLM call → patch JSON │
│  ┌──────────────────┐ │       │  kubectl_get → before snapshot │
│  │ k8s-mcp-server   │ │       │  POST /internal/fixes          │
│  │ (9 read-only     │ │       └──────────────────────────────┘
│  │  MCP tools)      │ │
│  └──────────────────┘ │
│                        │
│  POST findings → ctrl  │
└────────────────────────┘
```

## Quick Start

### Prerequisites

- Kubernetes cluster (minikube, kind, or any cloud cluster)
- `helm` >= 3.14
- An Anthropic API key (or compatible proxy)

### 1. Create the API key secret

```bash
kubectl create namespace kube-agent-helper
kubectl create secret generic anthropic-credentials \
  -n kube-agent-helper \
  --from-literal=apiKey=sk-ant-...
```

### 2. Install with Helm

```bash
helm install kah deploy/helm \
  --namespace kube-agent-helper
```

With a custom proxy and model:

```bash
helm install kah deploy/helm \
  --namespace kube-agent-helper \
  --set anthropic.baseURL=https://my-proxy.example.com \
  --set anthropic.model=claude-3-5-sonnet-20241022
```

### 3. Access the Dashboard

```bash
kubectl port-forward svc/kah -n kube-agent-helper 8080:8080 &
kubectl port-forward svc/kah-dashboard -n kube-agent-helper 3000:3000 &
open http://localhost:3000
```

The dashboard supports Chinese (default) and English, with dark/light theme toggle.

### 4. Run a diagnostic

From the dashboard: click "Create Run" and fill in the form.

Or via kubectl:

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: cluster-health-check
  namespace: kube-agent-helper
spec:
  target:
    scope: namespace
    namespaces:
      - default
  modelConfigRef: "anthropic-credentials"
  timeoutSeconds: 600        # optional, nil = no timeout
  outputLanguage: zh          # optional, zh|en, controls finding language
```

```bash
kubectl apply -f the-above.yaml
kubectl get diagnosticrun cluster-health-check -w
kubectl get diagnosticrun cluster-health-check -o jsonpath='{.status.findings}' | jq .
```

### 5. Generate a Fix

From the dashboard: open a completed Run, click "Generate Fix" on any finding.

Or via API:

```bash
curl -X POST http://localhost:8080/api/findings/<finding-id>/generate-fix
```

The fix generator Pod reads the target resource, asks the LLM for a patch, and creates a `DiagnosticFix` CR. Review the Before/After diff in the dashboard, then Approve or Reject.

## CRDs

| CRD | Purpose |
|-----|---------|
| `DiagnosticRun` | Declares a diagnostic task; controller creates an agent Job |
| `DiagnosticSkill` | Declares a diagnostic skill (dimension, prompt, tools) |
| `ModelConfig` | LLM provider configuration (API key secret reference) |
| `DiagnosticFix` | A proposed fix (patch or new resource) with approval workflow |

### DiagnosticFix Lifecycle

```
DryRunComplete → [user approves] → Approved → Applying → Succeeded
                                                      → Failed → (auto-rollback) → RolledBack
                 [user rejects]  → Failed
```

Strategies: `dry-run` (review only), `auto` (apply patch), `create` (create new resource).

## Built-in Skills

| Skill | Dimension | Description |
|-------|-----------|-------------|
| `pod-health-analyst` | health | Detects CrashLoopBackOff, OOMKilled, pending pods, high restarts |
| `pod-security-analyst` | security | Checks privileged containers, missing securityContext, image policies |
| `pod-cost-analyst` | cost | Finds over-provisioned resource requests, zombie deployments |
| `reliability-analyst` | reliability | Analyzes probe configuration, PDB, HPA, replica counts |
| `config-drift-analyst` | reliability | Detects selector/label mismatches, broken ConfigMap/Secret refs, orphan Services |

Custom skills: create a `DiagnosticSkill` CR or place a `SKILL.md` file in `skills/`.

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/runs` | List diagnostic runs |
| POST | `/api/runs` | Create a DiagnosticRun CR |
| GET | `/api/runs/:id` | Get run details |
| GET | `/api/runs/:id/findings` | List findings (includes `FixID` if fix exists) |
| GET | `/api/skills` | List registered skills |
| POST | `/api/skills` | Create a DiagnosticSkill CR |
| GET | `/api/fixes` | List fixes |
| GET | `/api/fixes/:id` | Get fix details (includes `BeforeSnapshot`) |
| PATCH | `/api/fixes/:id/approve` | Approve a fix (triggers reconciler to apply) |
| PATCH | `/api/fixes/:id/reject` | Reject a fix |
| POST | `/api/findings/:id/generate-fix` | Trigger fix generation for a finding |

## Development

```bash
# Run all unit tests
make test

# Run integration tests (requires kubebuilder binaries)
make envtest

# Build binaries
make build

# Build Docker images
make image

# Run dashboard dev server
cd dashboard && npm run dev
```

## Project Structure

```
cmd/controller/          Go controller binary entrypoint
internal/
  controller/
    api/v1alpha1/        CRD type definitions
    reconciler/          Run, Skill, Fix, ModelConfig reconcilers
    translator/          Run → Job compiler, FixGenerator → Job compiler
    httpserver/           REST API handlers
    registry/            Skill registry (hot-reload from store)
  store/                 Store interface + SQLite implementation
  k8sclient/             Kubernetes client wrapper
  mcptools/              MCP tool implementations
agent-runtime/
  runtime/
    main.py              Diagnostic agent entrypoint (multi-turn LLM)
    fix_main.py          Fix generator entrypoint (single LLM call)
    orchestrator.py      Agentic loop with httpx SSE streaming
    mcp_client.py        MCP stdio client helpers
    skill_loader.py      SKILL.md file parser
dashboard/
  src/
    app/                 Next.js pages (runs, skills, fixes)
    components/          UI components (dialogs, badges, diff viewer)
    i18n/                zh.json + en.json dictionaries
    theme/               Dark/light theme context
    lib/                 API hooks (SWR), types, utils
skills/                  Built-in SKILL.md files
deploy/helm/             Helm chart (CRDs, RBAC, deployments)
```

## Roadmap

- [x] **Phase 1** — Operator MVP: 4 CRDs, single-run Job, 5 built-in skills
- [x] **Phase 2** — Dashboard (Next.js), Skill Registry UI, i18n (zh/en), dark mode
- [x] **Phase 3** — DiagnosticFix: LLM-generated patches, Before/After diff, HITL approval, auto-rollback
- [ ] **Phase 4** — Real-time event streaming, vector case memory (RAG), multi-cluster

## References

- [kagent](https://github.com/kagent-dev/kagent) — Kubernetes-native agent orchestration framework
- [ci-agent](https://github.com/googs1025/ci-agent) — GitHub CI pipeline AI analyzer

## License

Apache License 2.0 — see [LICENSE](LICENSE).
