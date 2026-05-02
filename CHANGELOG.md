# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-05-02

First public release of **KubeDoctor** (formerly `kube-agent-helper`).

### Added

#### Operator core (Phase 1)
- 5 CRDs: `DiagnosticRun`, `DiagnosticSkill`, `ModelConfig`, `DiagnosticFix`, `ClusterConfig`.
- Controller binary (`cmd/controller`) with reconcilers for each CRD.
- Translator: compiles `DiagnosticRun` → ServiceAccount + RoleBinding + ConfigMap + Job.
- SkillRegistry with dual-source loading (`skills/*.md` + `DiagnosticSkill` CR).
- SQLite-backed Store for findings, fixes, skills, events, metrics.
- 10 built-in Skills covering health, security, cost, reliability, network, node,
  rollout, storage, config-drift, alert-response.

#### MCP tools
- 16 MCP tools exposed via `k8s-mcp-server` (stdio transport, mcp-go):
  `kubectl_get/describe/logs/explain/rollout_status`, `events_list/events_history`,
  `top_pods/top_nodes`, `prometheus_query/prometheus_alerts/metric_history`,
  `network_policy_check`, `node_status_summary`, `pvc_status`, `list_api_resources`.
- Audit wrapper with parameter whitelist sanitization for every tool call.
- Output sanitizer (`internal/sanitize`) and trimmer (`internal/trimmer`) to
  redact Secrets and cap response size before returning to the agent.

#### Dashboard (Phase 2)
- Next.js 14 dashboard with i18n (zh/en) and dark/light theme.
- Pages: runs, findings, skills, fixes, diagnose, events, CRD YAML viewer.
- Skill registry UI (enable/disable, priority, source badge).

#### Fix workflow (Phase 3)
- `DiagnosticFix` CR with strategies `auto` / `dry-run` / `create`.
- LLM-driven patch / manifest generation via short-lived FixGenerator Job.
- Before/After diff viewer in Dashboard.
- Human-in-the-loop approval flow:
  `PendingApproval → Approved → Applying → Succeeded | Failed → RolledBack`.
- Automatic rollback on health-check failure with snapshot restore.

#### Symptom-driven diagnosis (Phase 3.5)
- `/diagnose` page: select namespace + symptoms → controller picks matching skills.
- Schedule presets: one-shot / hourly / daily 08:00 / weekly Mon 08:00 / custom cron.

#### Phase 4 — Production hardening
- **Scheduled diagnostics**: `spec.schedule` (cron) + `historyLimit` on
  `DiagnosticRun`; controller creates child runs and prunes history.
- **EventCollector**: background K8s Warning event watcher + Prometheus metric
  scraper, persisted to SQLite (`events_history` and `metric_history` tools).
- **Multi-cluster support**: `ClusterConfig` CR registers remote clusters via
  kubeconfig Secret; `DiagnosticRun.spec.clusterRef` targets a specific cluster.
- **Per-run model proxy**: `ModelConfig.spec.baseURL` allows each Run to use a
  different LLM endpoint.
- **Retry + Fallback chain**: `ModelConfig.spec.retries` for transient errors
  (5xx / 429 / network) plus `DiagnosticRun.spec.fallbackModelConfigRefs` for
  multi-model fallback with full message-history preservation across switches.
- **Notification webhooks**: generic JSON, Slack, DingTalk, Feishu with HMAC
  signing and 5-minute deduplication window.
- **Output language control**: `DiagnosticRun.spec.outputLanguage: zh|en`.
- **Langfuse integration**: optional self-hosted or cloud LLM observability.
- **Grafana dashboard ConfigMap** auto-provisioned via sidecar pattern.
- **Prometheus `/metrics` endpoint** + ServiceMonitor template.

#### Tooling
- `kah` CLI for local interaction with the controller HTTP API.
- Helm Chart with full `VALUES.md` reference.
- Comprehensive CI: unit + envtest + e2e + kind smoke + helm lint + skill lint
  + CRD consistency + Go/Python coverage reporting.

### Known Limitations

- Single-instance controller (SQLite, no HA — use `replicaCount: 1`).
- Anthropic-only LLM provider (proxy-compatible endpoints supported via `baseURL`).
- No multi-tenancy / RBAC for Dashboard users (intended for trusted ops teams).
- Streaming MCP tool responses not supported (request/response only).

[Unreleased]: https://github.com/googs1025/kube-agent-helper/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/googs1025/kube-agent-helper/releases/tag/v0.1.0
