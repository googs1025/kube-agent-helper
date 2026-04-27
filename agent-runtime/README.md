# agent-runtime

The agent-runtime is the container image spawned as a Kubernetes Job for each DiagnosticRun. It runs a multi-turn LLM agentic loop that diagnoses cluster health using MCP tools.

## Architecture

```
controller
    │  creates Job
    ▼
agent-runtime Pod
    ├── k8s-mcp-server (Go binary)   — exposes Kubernetes tools over stdio MCP
    └── runtime/main.py              — Python orchestrator
            │
            ├── skill_loader.py      — loads DiagnosticSkill definitions
            ├── orchestrator.py      — agentic loop (LLM ↔ MCP tools)
            ├── mcp_client.py        — calls k8s-mcp-server via MCP protocol
            ├── tracer.py            — optional Langfuse LLM tracing
            └── reporter.py          — posts findings back to controller API
```

## What It Does

1. Reads DiagnosticSkill CRs injected via environment variables.
2. Builds a system prompt from the skills and target namespaces.
3. Runs a streaming agentic loop (up to `MAX_TURNS` turns, default 10):
   - Calls the LLM (Anthropic API or compatible proxy).
   - Executes MCP tool calls against the cluster (kubectl_get, events_list, prometheus_alerts, etc.).
   - Parses structured finding JSON from the LLM output.
4. Posts findings to the controller API on completion.

## Environment Variables

| Variable | Description | Default |
|---|---|---|
| `ANTHROPIC_API_KEY` | Anthropic API key | required |
| `ANTHROPIC_BASE_URL` | API base URL (proxy support) | `https://api.anthropic.com` |
| `MODEL` | Model name | `claude-sonnet-4-6` |
| `MAX_TURNS` | Max LLM turns per run | `10` |
| `CONTROLLER_URL` | Controller API endpoint for posting findings | required |
| `RUN_NAME` | DiagnosticRun name | required |
| `RUN_NAMESPACE` | DiagnosticRun namespace | required |
| `TARGET_NAMESPACES` | Comma-separated namespaces to diagnose | required |
| `LANGFUSE_PUBLIC_KEY` | Langfuse public key (optional) | — |
| `LANGFUSE_SECRET_KEY` | Langfuse secret key (optional) | — |
| `LANGFUSE_HOST` | Langfuse server URL (optional) | `https://cloud.langfuse.com` |

## MCP Tools

The Go binary (`k8s-mcp-server`) exposes the following tools:

| Tool | Description |
|---|---|
| `kubectl_get` | List Kubernetes resources (max 200 items) |
| `kubectl_describe` | Describe a specific resource |
| `kubectl_logs` | Fetch pod logs |
| `events_list` | List cluster events (max 200, newest first) |
| `prometheus_alerts` | Query firing Prometheus alerts |
| `prometheus_query` | Run a PromQL query |
| `node_status_summary` | Summarize node conditions |
| `top_nodes` / `top_pods` | Resource usage metrics |

## Dependencies

- `anthropic` — LLM client
- `langfuse>=2.0.0,<3.0.0` — LLM tracing (optional)
- `requests`, `pyyaml` — HTTP and config utilities

## Build

```bash
# From repo root
docker build -f agent-runtime/Dockerfile -t kube-agent-helper/agent-runtime:dev .
```
