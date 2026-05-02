# KubeDoctor — Helm Values Reference

Complete reference for all configurable parameters in the `kube-agent-helper` Helm chart.

## Quick Start

```bash
helm install kah ./deploy/helm \
  --namespace kube-agent-helper \
  --create-namespace
```

Override values inline or with a file:

```bash
helm install kah ./deploy/helm \
  --namespace kube-agent-helper \
  -f my-values.yaml
```

---

## Parameters

### Global

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `replicaCount` | int | `1` | Number of controller replicas. Use 1 when backed by SQLite (no HA). |

### Image

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `image.controller` | string | `ghcr.io/kube-agent-helper/controller:latest` | Controller container image (repository + tag). |
| `image.agent` | string | `ghcr.io/kube-agent-helper/agent-runtime:latest` | Agent runtime image spawned by the Translator for each diagnostic Job. |
| `image.dashboard` | string | `ghcr.io/kube-agent-helper/dashboard:latest` | Dashboard (Next.js) container image. |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy applied to all containers (`Always`, `IfNotPresent`, `Never`). |

### Controller

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `controller.httpAddr` | string | `":8080"` | Listen address for the controller HTTP API. |
| `controller.dbPath` | string | `"/data/kube-agent-helper.db"` | Path to the SQLite database file inside the container. Must be on a persistent volume for durability. |
| `controller.controllerURL` | string | `""` | Base URL the agent Pods use to call back to the controller. If empty, defaults to `http://<release>.<namespace>.svc.cluster.local:8080`. |

### Controller Resources

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `resources.requests.cpu` | string | `"100m"` | CPU request for the controller container. |
| `resources.requests.memory` | string | `"128Mi"` | Memory request for the controller container. |
| `resources.limits.memory` | string | `"256Mi"` | Memory limit for the controller container. |

### Dashboard

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `dashboard.enabled` | bool | `true` | Deploy the Next.js dashboard. Set to `false` on spoke clusters in a multi-cluster setup. |
| `dashboard.replicas` | int | `1` | Number of dashboard replicas. |
| `dashboard.port` | int | `3000` | Container port for the dashboard process. |
| `dashboard.resources.requests.cpu` | string | `"50m"` | Dashboard CPU request. |
| `dashboard.resources.requests.memory` | string | `"64Mi"` | Dashboard memory request. |
| `dashboard.resources.limits.memory` | string | `"256Mi"` | Dashboard memory limit. |

### Persistence

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `persistence.enabled` | bool | `true` | Create a PersistentVolumeClaim for the SQLite database. When `false`, an `emptyDir` volume is used (data lost on pod restart). |
| `persistence.size` | string | `"1Gi"` | Requested storage size for the PVC. |
| `persistence.storageClass` | string | `""` | StorageClass name. Empty string uses the cluster default StorageClass. |

### Anthropic / LLM Provider

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `anthropic.secretName` | string | `"anthropic-credentials"` | Name of the Kubernetes Secret containing the Anthropic API key. Referenced by the default ModelConfig. |
| `anthropic.secretKey` | string | `"apiKey"` | Key within the Secret that holds the API key value. |
| `anthropic.baseURL` | string | `""` | Custom API endpoint for Anthropic (or a compatible proxy). Empty uses the default Anthropic API URL. |
| `anthropic.model` | string | `""` | Model name override (e.g. `claude-3-5-sonnet-20241022`). Empty uses the controller's built-in default. |

### Prometheus Integration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `prometheus.url` | string | `""` | Prometheus server URL for background metric scraping by EventCollector (e.g. `http://prometheus:9090`). |
| `prometheus.agentURL` | string | `""` | Prometheus URL injected into agent Pods. Defaults to `prometheus.url` if empty. Useful when agents need a different network path. |
| `prometheus.metricsQueries` | string | `""` | Comma-separated PromQL queries for the EventCollector to scrape periodically. |

### Ingress

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ingress.enabled` | bool | `false` | Create an Ingress resource for the dashboard. |
| `ingress.className` | string | `""` | Ingress class name (e.g. `nginx`, `alb`, `traefik`). |
| `ingress.host` | string | `"kube-agent-helper.local"` | Hostname for the Ingress rule. |
| `ingress.annotations` | object | `{}` | Extra annotations on the Ingress resource (e.g. cert-manager, ALB settings). |
| `ingress.tls` | list | `[]` | TLS configuration. Each entry has `hosts` (list) and `secretName`. |

---

## Deployment Scenarios

### Minikube / Local Development

Minimal install for local testing. Disable persistence to avoid StorageClass issues on minikube without a provisioner:

```bash
helm install kah ./deploy/helm \
  --namespace kube-agent-helper \
  --create-namespace \
  --set persistence.enabled=false \
  --set anthropic.baseURL=https://my-proxy.example.com
```

Or with a custom values file (`local-values.yaml`):

```yaml
replicaCount: 1

persistence:
  enabled: false

dashboard:
  enabled: true

anthropic:
  secretName: anthropic-credentials
  baseURL: "https://my-proxy.example.com"
  model: "claude-3-5-sonnet-20241022"
```

```bash
helm install kah ./deploy/helm \
  --namespace kube-agent-helper \
  --create-namespace \
  -f local-values.yaml
```

### Production Cloud (AWS EKS / GKE / AKS)

Production setup with persistent storage, Prometheus integration, and ingress:

```bash
helm install kah ./deploy/helm \
  --namespace kube-agent-helper \
  --create-namespace \
  --set persistence.size=10Gi \
  --set persistence.storageClass=gp3 \
  --set prometheus.url=http://prometheus-server.monitoring:9090 \
  --set ingress.enabled=true \
  --set ingress.className=alb \
  --set ingress.host=kah.internal.example.com
```

Or with a production values file (`production-values.yaml`):

```yaml
replicaCount: 1

persistence:
  enabled: true
  size: 10Gi
  storageClass: gp3

prometheus:
  url: "http://prometheus-server.monitoring:9090"
  metricsQueries: "up,node_cpu_seconds_total"

ingress:
  enabled: true
  className: alb
  host: kah.internal.example.com
  annotations:
    alb.ingress.kubernetes.io/scheme: internal
    alb.ingress.kubernetes.io/target-type: ip
  tls:
    - hosts:
        - kah.internal.example.com
      secretName: kah-tls

dashboard:
  enabled: true
  replicas: 2
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      memory: 512Mi

resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    memory: 512Mi
```

### Behind Corporate Proxy

When the controller must reach external LLM APIs through a corporate HTTP proxy, use `anthropic.baseURL` to point to an internal proxy or gateway:

```bash
helm install kah ./deploy/helm \
  --namespace kube-agent-helper \
  --create-namespace \
  --set anthropic.baseURL=https://internal-llm-proxy.corp.example.com \
  --set anthropic.model=claude-3-5-sonnet-20241022
```

If you have a ModelConfig CR pointing to a custom `spec.baseURL`, that value takes precedence over the Helm chart value for runs referencing that ModelConfig.

### Multi-Cluster Setup

Register remote clusters via `ClusterConfig` CRDs. On spoke clusters you may disable the dashboard and only run the controller:

**Hub cluster** (full install with dashboard):

```bash
helm install kah ./deploy/helm \
  --namespace kube-agent-helper \
  --create-namespace \
  --set dashboard.enabled=true
```

**Spoke cluster** (controller only, no dashboard):

```bash
helm install kah ./deploy/helm \
  --namespace kube-agent-helper \
  --create-namespace \
  --set dashboard.enabled=false
```

On the hub cluster, create `ClusterConfig` CRs to register spoke clusters:

```yaml
apiVersion: k8sai.io/v1alpha1
kind: ClusterConfig
metadata:
  name: spoke-1
  namespace: kube-agent-helper
spec:
  displayName: "Spoke Cluster 1"
  kubeconfigSecret:
    name: spoke-1-kubeconfig
    key: kubeconfig
```

Then target diagnostics at a remote cluster with `spec.clusterRef: spoke-1`.

---

## Useful Commands

```bash
# Render templates locally (dry-run)
helm template kah ./deploy/helm --namespace kube-agent-helper

# Lint the chart
helm lint ./deploy/helm

# Upgrade an existing release
helm upgrade kah ./deploy/helm --namespace kube-agent-helper -f my-values.yaml

# Show computed values
helm get values kah --namespace kube-agent-helper
```
