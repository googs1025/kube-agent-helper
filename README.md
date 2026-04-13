# kube-agent-helper

> Kubernetes-native AI diagnostic operator

**kube-agent-helper** is an AI agent that runs inside your Kubernetes cluster and diagnoses workload issues. Declare a `DiagnosticRun` CR, and the controller spins up an isolated agent Pod that calls Claude via MCP tools, then writes structured findings back to the API server.

[![CI](https://github.com/kube-agent-helper/kube-agent-helper/actions/workflows/ci.yml/badge.svg)](https://github.com/kube-agent-helper/kube-agent-helper/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## Features

- **CRD-driven** — declare diagnostic tasks with `DiagnosticRun`; extend with `DiagnosticSkill`
- **Declarative skill system** — skills are `SKILL.md` files loaded as CRs; GitOps-friendly
- **Claude-powered agentic loop** — multi-turn reasoning over live cluster data
- **MCP tool layer** — 9 read-only tools (`kubectl_get`, `kubectl_logs`, `events_list`, `top_pods`, …)
- **Minimal RBAC** — Translator auto-generates least-privilege ServiceAccount per run
- **SQLite persistence** — findings stored locally; no external database required

## Architecture

```
kubectl apply DiagnosticRun
        │
        ▼
┌──────────────┐    translates    ┌─────────────┐    stdio MCP    ┌──────────────────┐
│  Controller  │ ──────────────▶  │  Agent Pod  │ ─────────────▶  │ k8s-mcp-server   │
│  (Operator)  │                  │  (Python)   │                  │ (Go, in-cluster) │
└──────┬───────┘                  └──────┬──────┘                  └──────────────────┘
       │                                 │
       │ REST API                        │ findings JSON
       ▼                                 ▼
  /api/runs                       POST /api/runs/:id/findings
  /api/skills
```

## Quick Start

### Prerequisites

- Kubernetes cluster (minikube, kind, or any cloud cluster)
- `helm` ≥ 3.14
- An Anthropic API key (or compatible proxy)

### 1. Create the API key secret

```bash
kubectl create secret generic anthropic-credentials \
  --from-literal=apiKey=sk-ant-...
```

### 2. Install with Helm

```bash
helm install kube-agent-helper deploy/helm \
  --namespace kube-agent-helper --create-namespace
```

With a custom proxy:

```bash
helm install kube-agent-helper deploy/helm \
  --namespace kube-agent-helper --create-namespace \
  --set anthropic.baseURL=https://my-proxy.example.com/v1/messages \
  --set anthropic.model=claude-3-5-sonnet-20241022
```

### 3. Run a diagnostic

```yaml
apiVersion: diagnostics.kube-agent-helper.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: cluster-health-check
  namespace: kube-agent-helper
spec:
  targetNamespaces:
    - default
  skillNames:
    - pod-health-analyst
```

```bash
kubectl apply -f the-above.yaml

# Watch progress
kubectl get diagnosticrun cluster-health-check -w

# View findings
kubectl get diagnosticrun cluster-health-check -o jsonpath='{.status.findings}' | jq .
```

## Built-in Skills

| Skill | Dimension | Description |
|-------|-----------|-------------|
| `pod-health-analyst` | health | Detects CrashLoopBackOff, OOMKilled, pending pods |
| `pod-security-analyst` | security | Checks privileged containers, missing securityContext |
| `pod-cost-analyst` | cost | Finds over-provisioned resource requests |

Custom skills can be added by creating a `DiagnosticSkill` CR or placing a `SKILL.md` file in the `skills/` directory.

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
```

## Roadmap

- **Phase 1** (current) — Operator MVP: 3 CRDs, single-run Job, 3 built-in skills
- **Phase 2** — Skill Registry UI, multi-dimension parallel analysis, Dashboard
- **Phase 3** — Real-time event streaming, vector case memory (RAG)
- **Phase 4** — Production hardening: minimal RBAC sandbox, OIDC, HITL approval

## References

- [kagent](https://github.com/kagent-dev/kagent) — Kubernetes-native agent orchestration framework
- [ci-agent](https://github.com/googs1025/ci-agent) — GitHub CI pipeline AI analyzer

## License

Apache License 2.0 — see [LICENSE](LICENSE).
