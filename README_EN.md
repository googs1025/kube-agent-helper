# kube-agent-helper

> Kubernetes-native AI diagnostic operator with auto-fix capabilities

**kube-agent-helper** is an AI agent that runs inside your Kubernetes cluster, diagnoses workload issues, and generates actionable fix suggestions. Declare a `DiagnosticRun` CR, and the controller spins up an isolated agent Pod that calls Claude via MCP tools, writes structured findings, and optionally produces `DiagnosticFix` CRs with patches or new resource manifests.

[![CI](https://github.com/googs1025/kube-agent-helper/actions/workflows/ci.yml/badge.svg)](https://github.com/googs1025/kube-agent-helper/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

[中文](README.md)

## Features

- **CRD-driven diagnostics** — declare tasks with `DiagnosticRun`; extend with `DiagnosticSkill` CRs
- **4 CRDs** — `DiagnosticRun`, `DiagnosticSkill`, `ModelConfig`, `DiagnosticFix`
- **10 built-in skills** — health, security, cost, reliability, config-drift, alert-responder, network, node, rollout, storage
- **14 MCP tools** — kubectl, Prometheus, logs, network policies, PVC, node status, and more
- **Claude-powered agentic loop** — multi-turn reasoning over live cluster data
- **Fix generation** — click "Generate Fix" on any finding; a short-lived Pod produces a patch or full resource manifest via LLM
- **Before/After diff** — Fix detail page shows resource diff before applying
- **Human-in-the-loop approval** — Fixes go through `PendingApproval → Approved → Applying → Succeeded` with optional auto-rollback
- **Symptom-driven entry** — `/diagnose` page: arrive from a monitoring alert, select symptoms, get targeted diagnosis
- **Dashboard** — Next.js web UI with Chinese/English toggle, dark/light theme, stats, create dialogs
- **Per-run output language** — `spec.outputLanguage: zh|en` controls finding language
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
│  Controller (Go)                                                            │
│                                                                             │
│  ┌─────────────────┐  ┌───────────────┐  ┌──────────────────────────────┐  │
│  │ Run Reconciler   │  │ Fix Reconciler │  │ HTTP Server                  │  │
│  │ Skill Reconciler │  │               │  │  /api/runs  /api/skills       │  │
│  │ ModelConfig Ctrl  │  │ apply patch   │  │  /api/fixes /api/findings     │  │
│  │ Pod status capture│  │ auto-rollback │  │  /api/k8s/resources           │  │
│  └────────┬──────────┘  └───────────────┘  └──────────────────────────────┘  │
│           │ Translator                                                      │
│           ▼                                                                 │
│  ┌──────────────────────────────────────────────────┐                      │
│  │ SQLite (diagnostic_runs, findings, skills, fixes) │                      │
│  └──────────────────────────────────────────────────┘                      │
└────────────┬────────────────────────────────────┬───────────────────────────┘
             │ creates Job                        │ creates Job
             ▼                                    ▼
┌──────────────────────┐        ┌──────────────────────────────┐
│  Diagnostic Agent Pod │        │  Fix Generator Pod            │
│  python -m runtime.main│       │  python -m runtime.fix_main   │
│  Multi-turn LLM loop   │       │  Single LLM call → patch JSON │
│  ┌──────────────────┐ │       └──────────────────────────────┘
│  │ k8s-mcp-server   │ │
│  │ (14 MCP tools)   │ │
│  └──────────────────┘ │
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

### 4. Symptom-driven diagnosis (recommended)

Open Dashboard → click **Diagnose** → select namespace, resource, check symptoms (e.g. high CPU, pod crash-looping) → submit.

The system maps symptoms to skills, triggers a DiagnosticRun, and displays findings sorted by severity.

### 5. Create a run via kubectl

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
  timeoutSeconds: 600     # optional, nil = no timeout
  outputLanguage: en      # optional: zh | en
```

```bash
kubectl apply -f the-above.yaml
kubectl get diagnosticrun cluster-health-check -w
```

### 6. Generate a Fix

From the dashboard: open a completed Run, click "Generate Fix" on any finding.

Or via API:

```bash
curl -X POST http://localhost:8080/api/findings/<finding-id>/generate-fix
```

Review the Before/After diff in the dashboard, then Approve or Reject.

## Built-in Skills

| Skill | Dimension | Description |
|-------|-----------|-------------|
| `pod-health-analyst` | health | Detects CrashLoopBackOff, OOMKilled, pending pods, high restarts |
| `pod-security-analyst` | security | Checks privileged containers, missing securityContext |
| `pod-cost-analyst` | cost | Finds over-provisioned resource requests, zombie deployments |
| `reliability-analyst` | reliability | Analyzes probe config, PDB, replica counts |
| `config-drift-analyst` | reliability | Detects selector/label mismatches, broken ConfigMap/Secret refs |
| `alert-responder` | health | Triages firing Prometheus alerts to root cause |
| `network-troubleshooter` | reliability | Diagnoses Service connectivity and NetworkPolicy blocks |
| `node-health-analyst` | reliability | Detects node pressure (memory/disk/PID), capacity issues |
| `rollout-analyst` | health | Diagnoses stuck rollouts and failing new pod versions |
| `storage-analyst` | reliability | Diagnoses PVC Pending/Lost and volume mount failures |

Custom skills: create a `DiagnosticSkill` CR or place a `.md` file in `skills/`.

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

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/runs` | List diagnostic runs |
| POST | `/api/runs` | Create a DiagnosticRun CR |
| GET | `/api/runs/:id` | Get run details |
| GET | `/api/runs/:id/findings` | List findings |
| GET | `/api/skills` | List registered skills |
| POST | `/api/skills` | Create a DiagnosticSkill CR |
| GET | `/api/fixes` | List fixes |
| GET | `/api/fixes/:id` | Get fix details |
| PATCH | `/api/fixes/:id/approve` | Approve a fix |
| PATCH | `/api/fixes/:id/reject` | Reject a fix |
| POST | `/api/findings/:id/generate-fix` | Trigger fix generation |
| GET | `/api/k8s/resources` | List cluster resources for autocomplete |

## Development

```bash
make test        # unit tests
make envtest     # integration tests (requires kubebuilder)
make build       # build binaries
make image       # build Docker images
cd dashboard && npm run dev  # dashboard dev server
```

## Roadmap

- [x] **Phase 1** — Operator MVP: 4 CRDs, single-run Job, 5 built-in skills
- [x] **Phase 2** — Dashboard, Skill Registry UI, i18n (zh/en), dark mode
- [x] **Phase 3** — DiagnosticFix: LLM patches, Before/After diff, HITL approval, auto-rollback
- [x] **Phase 3.5** — 5 new MCP tools, 10 built-in skills, symptom-driven /diagnose page
- [ ] **Phase 4** — Real-time event streaming, vector case memory (RAG), multi-cluster

## References

- [kagent](https://github.com/kagent-dev/kagent) — Kubernetes-native agent orchestration
- [ci-agent](https://github.com/googs1025/ci-agent) — GitHub CI pipeline AI analyzer

## License

Apache License 2.0 — see [LICENSE](LICENSE).
