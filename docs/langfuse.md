# Langfuse LLM Observability

kube-agent-helper integrates [Langfuse](https://langfuse.com) to trace every LLM turn: input/output tokens, tool calls, latency, and model metadata.

## How It Works

Each DiagnosticRun spawns an agent Job. The agent reads Langfuse credentials from a Kubernetes Secret and records one Langfuse **generation** per LLM turn. Tracing is best-effort — a Langfuse failure never aborts a diagnostic run.

## Setup Options

### Option A: Cloud Langfuse (external)

1. Create a project at [cloud.langfuse.com](https://cloud.langfuse.com) and copy the API keys.
2. Create a Kubernetes Secret:

```bash
kubectl create secret generic langfuse-credentials \
  --from-literal=publicKey=pk-lf-... \
  --from-literal=secretKey=sk-lf-... \
  -n kube-agent-helper
```

3. Reference the secret in Helm values:

```yaml
langfuse:
  secretName: langfuse-credentials
```

### Option B: Self-hosted Langfuse (in-cluster)

kube-agent-helper can deploy Langfuse v2 + PostgreSQL inside the cluster.

**Step 1 — Enable and deploy:**

```yaml
langfuse:
  selfHosted:
    enabled: true
    postgresPassword: "change-me"
    nextauthSecret: "change-me"
    salt: "change-me"
```

```bash
helm upgrade --install kah ./deploy/helm -f values.yaml -n kube-agent-helper
```

**Step 2 — Access the UI and create a project:**

```bash
kubectl port-forward svc/kah-langfuse 3000:3000 -n kube-agent-helper
```

Open http://localhost:3000, register an account, create a project, and copy the API keys.

**Step 3 — Write the keys back and redeploy:**

```yaml
langfuse:
  selfHosted:
    enabled: true
    credentials:
      publicKey: pk-lf-...
      secretKey: sk-lf-...
```

```bash
helm upgrade kah ./deploy/helm -f values.yaml -n kube-agent-helper
```

The controller now reads `kah-langfuse-credentials` automatically. No restart required.

## Viewing Traces

Navigate to your Langfuse project → **Generations** to see each LLM turn with:

- Model name and token counts
- Turn number and stop reason
- Sanitized input (raw tool results are stripped before upload)

## Disabling Tracing

Leave `langfuse.secretName` empty and `langfuse.selfHosted.enabled: false` (the defaults). No Langfuse code runs in the agent when credentials are absent.
