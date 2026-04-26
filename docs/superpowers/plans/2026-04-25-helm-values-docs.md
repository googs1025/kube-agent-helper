# Helm Values Documentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task.

**Issue:** #34 - Helm values documentation

## Goal

Provide comprehensive Helm values documentation including a full reference table, deployment scenario examples, and enhanced inline comments in `values.yaml`. Make it easy for users to configure kube-agent-helper for any environment.

## Architecture

Two documentation artifacts:
1. `deploy/helm/VALUES.md` - standalone reference with tables and deployment scenarios
2. Enhanced `deploy/helm/values.yaml` - inline comments explaining every field

Both are maintained alongside the chart and linked from the project README.

## Tech Stack

- Markdown for VALUES.md
- YAML comments for values.yaml
- Helm template validation

## File Map

| File | Status |
|------|--------|
| `deploy/helm/VALUES.md` | New |
| `deploy/helm/values.yaml` | Modified |
| `README.md` | Modified |

## Tasks

### Task 1: Create deploy/helm/VALUES.md

- [ ] Create comprehensive values reference document
- [ ] Include full parameter table with Key, Type, Default, Description columns
- [ ] Add deployment scenario sections
- [ ] Cover all current and recently added values (metrics, grafana, notifications, multi-cluster)

**Files:** `deploy/helm/VALUES.md`

**Steps:**

Structure:

```markdown
# Kube Agent Helper Helm Values Reference

## Quick Start

helm install kah ./deploy/helm -f my-values.yaml

## Parameters

### Global

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `replicaCount` | int | `1` | Number of controller replicas |
| `image.repository` | string | `ghcr.io/kube-agent-helper` | Controller image repository |
| `image.tag` | string | `""` | Image tag (defaults to chart appVersion) |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `imagePullSecrets` | list | `[]` | Docker registry pull secrets |

### Controller

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `controller.logLevel` | string | `info` | Log level (debug, info, warn, error) |
| `controller.concurrency` | int | `5` | Max concurrent diagnostic runs |
| `controller.watchNamespaces` | list | `[]` | Namespaces to watch (empty = all) |

### LLM Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `llm.provider` | string | `openai` | LLM provider (openai, azure, anthropic) |
| `llm.model` | string | `gpt-4` | Model name |
| `llm.apiKeySecret` | string | `""` | Secret name containing API key |
| `llm.apiKeyField` | string | `api-key` | Key field in secret |
| `llm.endpoint` | string | `""` | Custom API endpoint |
| `llm.maxTokens` | int | `4096` | Maximum tokens per request |

### Agent Runtime

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `agent.image.repository` | string | `ghcr.io/kube-agent-helper-agent` | Agent image |
| `agent.image.tag` | string | `""` | Agent image tag |
| `agent.resources.limits.cpu` | string | `500m` | Agent CPU limit |
| `agent.resources.limits.memory` | string | `256Mi` | Agent memory limit |
| `agent.serviceAccount` | string | `""` | Agent service account |
| `agent.ttlSecondsAfterFinished` | int | `300` | Agent pod cleanup TTL |

### Database

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `database.path` | string | `/data/kah.db` | SQLite database path |
| `database.persistence.enabled` | bool | `true` | Enable PVC for database |
| `database.persistence.size` | string | `1Gi` | PVC size |
| `database.persistence.storageClass` | string | `""` | Storage class |

### Dashboard

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `dashboard.enabled` | bool | `true` | Enable dashboard deployment |
| `dashboard.replicaCount` | int | `1` | Dashboard replicas |
| `dashboard.image.repository` | string | `ghcr.io/kube-agent-helper-dashboard` | Dashboard image |

### Metrics & Monitoring

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `metrics.enabled` | bool | `true` | Enable prometheus metrics |
| `metrics.serviceMonitor.enabled` | bool | `false` | Create ServiceMonitor |
| `metrics.serviceMonitor.interval` | string | `30s` | Scrape interval |
| `grafana.dashboard.enabled` | bool | `false` | Create Grafana dashboard ConfigMap |

### Notifications

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `notifications.dedupTTL` | string | `5m` | Deduplication window |
| `notifications.webhook.enabled` | bool | `false` | Enable generic webhook |
| `notifications.webhook.url` | string | `""` | Webhook URL |
| `notifications.webhook.secret` | string | `""` | HMAC signing secret |
| `notifications.slack.enabled` | bool | `false` | Enable Slack notifications |
| `notifications.slack.webhookURL` | string | `""` | Slack webhook URL |
| `notifications.dingtalk.enabled` | bool | `false` | Enable DingTalk notifications |
| `notifications.feishu.enabled` | bool | `false` | Enable Feishu notifications |

### Multi-Cluster

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `multiCluster.enabled` | bool | `false` | Enable multi-cluster support |
| `multiCluster.clusterName` | string | `default` | This cluster's name |

### Ingress

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ingress.enabled` | bool | `false` | Enable ingress |
| `ingress.className` | string | `""` | Ingress class name |
| `ingress.hosts` | list | `[]` | Ingress host rules |
| `ingress.tls` | list | `[]` | TLS configuration |

### RBAC & Security

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `rbac.create` | bool | `true` | Create RBAC resources |
| `serviceAccount.create` | bool | `true` | Create service account |
| `serviceAccount.name` | string | `""` | Service account name |
| `podSecurityContext` | object | `{}` | Pod security context |
| `securityContext` | object | `{}` | Container security context |
```

Deployment scenarios:

```markdown
## Deployment Scenarios

### Minikube / Local Development

helm install kah ./deploy/helm \
  --set dashboard.enabled=true \
  --set database.persistence.enabled=false \
  --set llm.provider=openai \
  --set llm.apiKeySecret=openai-secret

### Production Cloud (AWS EKS)

helm install kah ./deploy/helm \
  --set replicaCount=2 \
  --set database.persistence.storageClass=gp3 \
  --set database.persistence.size=10Gi \
  --set metrics.serviceMonitor.enabled=true \
  --set grafana.dashboard.enabled=true \
  --set ingress.enabled=true \
  --set ingress.className=alb \
  --set notifications.slack.enabled=true \
  --set notifications.slack.webhookURL=https://hooks.slack.com/... \
  -f production-values.yaml

### Behind Corporate Proxy

helm install kah ./deploy/helm \
  --set llm.endpoint=https://internal-proxy.corp/openai \
  --set controller.env[0].name=HTTPS_PROXY \
  --set controller.env[0].value=http://proxy:8080 \
  --set controller.env[1].name=NO_PROXY \
  --set controller.env[1].value=.svc,.cluster.local

### Multi-Cluster Setup

# Hub cluster
helm install kah ./deploy/helm \
  --set multiCluster.enabled=true \
  --set multiCluster.clusterName=hub

# Spoke cluster
helm install kah-agent ./deploy/helm \
  --set multiCluster.enabled=true \
  --set multiCluster.clusterName=spoke-1 \
  --set dashboard.enabled=false
```

**Test:** `helm template ./deploy/helm -f deploy/helm/values.yaml` (verify no errors)

**Commit:** `docs: add comprehensive Helm VALUES.md reference`

### Task 2: Enhance values.yaml with inline comments

- [ ] Add section header comments for every group
- [ ] Add per-field comments explaining purpose, valid values, and examples
- [ ] Add cross-references to VALUES.md where appropriate
- [ ] Ensure all fields have sensible defaults

**Files:** `deploy/helm/values.yaml`

**Steps:**

```yaml
# =============================================================================
# Kube Agent Helper Helm Chart Values
# Full reference: VALUES.md
# =============================================================================

# -- Number of controller replicas. Use 1 for SQLite (no HA), 2+ with external DB.
replicaCount: 1

# -- Controller image configuration
image:
  # -- Container image repository
  repository: ghcr.io/kube-agent-helper
  # -- Image pull policy (Always, IfNotPresent, Never)
  pullPolicy: IfNotPresent
  # -- Image tag. Defaults to chart appVersion if empty.
  tag: ""

# -- Docker registry pull secrets
# Example:
#   imagePullSecrets:
#     - name: regcred
imagePullSecrets: []

# =============================================================================
# Controller Configuration
# =============================================================================
controller:
  # -- Log level: debug, info, warn, error
  logLevel: info
  # -- Maximum number of concurrent diagnostic runs
  concurrency: 5
  # -- Namespaces to watch. Empty list means all namespaces.
  # Example: ["default", "production"]
  watchNamespaces: []

# =============================================================================
# LLM Provider Configuration
# =============================================================================
llm:
  # -- LLM provider: openai, azure, anthropic
  provider: openai
  # -- Model name to use for diagnostics
  model: gpt-4
  # -- Name of Kubernetes Secret containing the API key
  apiKeySecret: ""
  # -- Field name within the secret
  apiKeyField: api-key
  # -- Custom API endpoint (for proxies or Azure)
  endpoint: ""
  # -- Maximum tokens per LLM request
  maxTokens: 4096

# ... (continue for all sections)
```

Ensure every field in the current values.yaml has a descriptive comment.

**Test:** `helm lint ./deploy/helm`

**Commit:** `docs: enhance values.yaml with comprehensive inline comments`

### Task 3: Add README links to VALUES.md

- [ ] Add link to `deploy/helm/VALUES.md` in project README.md
- [ ] Add link in chart README or NOTES.txt if present
- [ ] Reference VALUES.md from installation documentation

**Files:** `README.md`

**Steps:**

Add to the Helm/Installation section of README.md:

```markdown
## Helm Chart

For installation and configuration, see the [Helm Values Reference](deploy/helm/VALUES.md).

### Quick Install

helm repo add kah https://...
helm install kah kah/kube-agent-helper

### Configuration

See [deploy/helm/VALUES.md](deploy/helm/VALUES.md) for the complete list of
configurable parameters and deployment scenarios.
```

**Test:** Verify links resolve correctly in rendered markdown.

**Commit:** `docs: add VALUES.md links to README`
