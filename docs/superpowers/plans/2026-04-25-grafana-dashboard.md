# Grafana Dashboard Template Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task.

**Issue:** #30 - Grafana Dashboard Template

## Goal

Provide a production-ready Grafana dashboard JSON template for kube-agent-helper metrics, auto-provisioned via Helm and the Grafana sidecar. The dashboard covers 14 panels across 5 logical rows: Overview, Runs, Findings, LLM, and Events.

## Architecture

A static Grafana dashboard JSON file is embedded in a Helm ConfigMap with the `grafana_dashboard: "1"` label. The Grafana sidecar (standard in kube-prometheus-stack) detects the ConfigMap and auto-provisions the dashboard. No Grafana API or manual import required.

## Tech Stack

- Grafana Dashboard JSON Model (schema v38+)
- Helm ConfigMap template
- Prometheus datasource UID variable

## File Map

| File | Status |
|------|--------|
| `deploy/grafana/kube-agent-helper-dashboard.json` | New |
| `deploy/helm/templates/grafana-dashboard-configmap.yaml` | New |
| `deploy/helm/values.yaml` | Modified |
| `docs/grafana-dashboard.md` | New |

## Tasks

### Task 1: Create Grafana dashboard JSON

- [ ] Create `deploy/grafana/kube-agent-helper-dashboard.json`
- [ ] Define datasource variable `$datasource` (type: prometheus)
- [ ] Define `$namespace` and `$cluster` template variables
- [ ] Create 5 rows with 14 panels total

**Files:** `deploy/grafana/kube-agent-helper-dashboard.json`

**Steps:**

Row 1 - Overview (3 panels):
- Stat: Total Diagnostic Runs (`sum(kah_diagnostic_runs_total)`)
- Stat: Active Runs (`kah_active_runs`)
- Stat: Total Findings (`sum(kah_findings_total)`)

Row 2 - Runs (3 panels):
- Time series: Run Rate (`rate(kah_diagnostic_runs_total[5m])` by phase)
- Heatmap: Run Duration (`kah_diagnostic_run_duration_seconds_bucket`)
- Table: Recent Runs (top 10 by duration)

Row 3 - Findings (3 panels):
- Pie chart: Findings by Severity (`sum(kah_findings_total) by (severity)`)
- Time series: Finding Rate (`rate(kah_findings_total[5m])`)
- Stat: Fix Success Rate (`sum(kah_fixes_total{status="Applied"}) / sum(kah_fixes_total)`)

Row 4 - LLM (3 panels):
- Time series: LLM Request Rate (`rate(kah_llm_requests_total[5m])`)
- Histogram: LLM Latency (`kah_llm_request_duration_seconds_bucket`)
- Time series: Token Usage (`rate(kah_llm_tokens_total[5m])` by direction)

Row 5 - Events (2 panels):
- Time series: Event Collection Rate (`rate(kah_event_collector_events_total[5m])`)
- Table: Top Event Reasons (`topk(10, sum by (reason) (kah_event_collector_events_total))`)

Dashboard settings:
```json
{
  "title": "Kube Agent Helper",
  "uid": "kube-agent-helper",
  "tags": ["kubernetes", "kube-agent-helper"],
  "timezone": "browser",
  "refresh": "30s",
  "time": {"from": "now-1h", "to": "now"}
}
```

**Test:** Import JSON into a Grafana instance and verify all panels render with sample data.

**Commit:** `feat(grafana): add kube-agent-helper dashboard JSON with 14 panels`

### Task 2: Helm ConfigMap template for sidecar auto-provisioning

- [ ] Create `deploy/helm/templates/grafana-dashboard-configmap.yaml`
- [ ] Gate behind `grafana.dashboard.enabled` value
- [ ] Add `grafana_dashboard: "1"` label for sidecar detection
- [ ] Embed dashboard JSON via `.Files.Get`
- [ ] Add values to `deploy/helm/values.yaml`

**Files:** `deploy/helm/templates/grafana-dashboard-configmap.yaml`, `deploy/helm/values.yaml`

**Steps:**

```yaml
# deploy/helm/templates/grafana-dashboard-configmap.yaml
{{- if .Values.grafana.dashboard.enabled }}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "kube-agent-helper.fullname" . }}-grafana-dashboard
  labels:
    {{- include "kube-agent-helper.labels" . | nindent 4 }}
    grafana_dashboard: "1"
  {{- with .Values.grafana.dashboard.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
data:
  kube-agent-helper.json: |-
    {{ .Files.Get "grafana/kube-agent-helper-dashboard.json" | nindent 4 }}
{{- end }}
```

Values additions:
```yaml
grafana:
  dashboard:
    enabled: false
    annotations: {}
```

**Test:** `helm template ./deploy/helm --set grafana.dashboard.enabled=true | grep grafana_dashboard`

**Commit:** `feat(helm): add grafana dashboard ConfigMap with sidecar label`

### Task 3: Documentation

- [ ] Create `docs/grafana-dashboard.md` with setup instructions
- [ ] Cover: prerequisites, auto-provisioning, manual import, customization
- [ ] Include screenshot placeholder descriptions

**Files:** `docs/grafana-dashboard.md`

**Steps:**

Document sections:
1. **Prerequisites** - kube-prometheus-stack or standalone Prometheus + Grafana
2. **Auto-Provisioning via Helm** - enable `grafana.dashboard.enabled=true` and `metrics.serviceMonitor.enabled=true`
3. **Manual Import** - copy JSON from `deploy/grafana/`, import via Grafana UI
4. **Dashboard Panels** - table listing all 14 panels with descriptions
5. **Customization** - how to modify datasource UID, add panels, change refresh interval
6. **Troubleshooting** - sidecar not picking up, datasource not found, no data

```markdown
## Quick Start

helm upgrade --install kah ./deploy/helm \
  --set metrics.serviceMonitor.enabled=true \
  --set grafana.dashboard.enabled=true

The dashboard will appear in Grafana under "Kube Agent Helper" within 60 seconds.
```

**Test:** Verify markdown renders correctly, all links are valid.

**Commit:** `docs: add grafana dashboard setup guide`
