# Grafana Dashboard

kube-agent-helper ships a production-ready Grafana dashboard covering diagnostic runs, findings, LLM usage, and Kubernetes event collection. The dashboard can be auto-provisioned via Helm or imported manually.

## Prerequisites

- Prometheus scraping kube-agent-helper metrics (port 8080, path `/metrics`)
- Grafana 9.0+ with a Prometheus datasource configured
- For auto-provisioning: Grafana sidecar enabled (standard in [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack))

## Auto-Provisioning via Helm

Enable the Grafana dashboard ConfigMap in your Helm values:

```bash
helm upgrade --install kah ./deploy/helm \
  --set grafana.dashboard.enabled=true
```

The ConfigMap is labeled with `grafana_dashboard: "1"`. The Grafana sidecar detects this label and provisions the dashboard automatically. It typically appears in Grafana within 60 seconds under the title **Kube Agent Helper**.

### Custom annotations

You can add annotations to the ConfigMap (for example, to specify a Grafana folder):

```yaml
grafana:
  dashboard:
    enabled: true
    annotations:
      grafana_folder: "Kubernetes"
```

## Manual Import

1. Copy the dashboard JSON from `deploy/grafana/kube-agent-helper-dashboard.json`
2. Open Grafana and navigate to **Dashboards > Import**
3. Paste the JSON or upload the file
4. Select your Prometheus datasource when prompted
5. Click **Import**

## Dashboard Panels

The dashboard contains 14 panels organized in 5 rows:

| Row | Panel | Type | Description |
|-----|-------|------|-------------|
| Overview | Total Diagnostic Runs | Stat | Cumulative count of all diagnostic runs |
| Overview | Active Runs | Stat | Currently running diagnostics |
| Overview | Total Findings | Stat | Cumulative count of all findings |
| Runs | Run Rate by Phase | Time series | Stacked run rate broken down by phase (e.g., Completed, Failed) |
| Runs | Run Duration Distribution | Heatmap | Duration distribution of diagnostic runs |
| Runs | Recent Runs (Top 10) | Table | Top 10 runs by average duration |
| Findings | Findings by Severity | Pie chart | Donut chart of findings split by severity (critical, warning, info) |
| Findings | Finding Rate | Time series | Rate of new findings over time, stacked by severity |
| Findings | Fix Success Rate | Stat | Percentage of fixes successfully applied |
| LLM | LLM Request Rate | Time series | Rate of outbound LLM API calls |
| LLM | LLM Latency (p50/p95/p99) | Time series | Latency percentiles for LLM requests |
| LLM | Token Usage | Time series | Token consumption rate by direction (input/output) |
| Events | Event Collection Rate | Time series | Rate of Kubernetes events collected |
| Events | Top Event Reasons | Table | Top 10 event reasons by count |

## Template Variables

The dashboard includes three template variables at the top:

- **Datasource** -- Select the Prometheus datasource to query
- **Cluster** -- Filter by cluster label (multi-select, defaults to all)
- **Namespace** -- Filter by namespace label (multi-select, defaults to all)
- **Interval** -- Rate/increase window (auto, 1m, 5m, 15m, 1h)

## Metrics Reference

All metrics use the `kah_` prefix:

| Metric | Type | Description |
|--------|------|-------------|
| `kah_diagnostic_runs_total` | Counter | Total diagnostic runs by phase |
| `kah_active_runs` | Gauge | Currently active diagnostic runs |
| `kah_findings_total` | Counter | Total findings by severity |
| `kah_fixes_total` | Counter | Total fixes by status |
| `kah_llm_requests_total` | Counter | Total LLM API requests |
| `kah_llm_request_duration_seconds` | Histogram | LLM request latency |
| `kah_llm_tokens_total` | Counter | LLM tokens by direction |
| `kah_diagnostic_run_duration_seconds` | Histogram | Diagnostic run duration |
| `kah_event_collector_events_total` | Counter | Collected Kubernetes events |

## Customization

### Change the default time range

Edit the `time` field in the dashboard JSON:

```json
"time": { "from": "now-6h", "to": "now" }
```

### Change the refresh interval

Edit the `refresh` field:

```json
"refresh": "1m"
```

### Add custom panels

1. Export the dashboard JSON from Grafana after making changes
2. Replace the file at `deploy/grafana/kube-agent-helper-dashboard.json`
3. Copy the updated file to `deploy/helm/grafana/kube-agent-helper-dashboard.json`

## Troubleshooting

### Dashboard does not appear in Grafana

- Verify the ConfigMap exists: `kubectl get configmap -l grafana_dashboard=1`
- Check the Grafana sidecar logs: `kubectl logs -l app.kubernetes.io/name=grafana -c grafana-sc-dashboard`
- Ensure `grafana.dashboard.enabled` is set to `true` in your Helm values

### "No data" on all panels

- Confirm Prometheus is scraping kube-agent-helper: visit Prometheus Targets page
- Check that the selected datasource variable matches your Prometheus instance
- Verify the `cluster` and `namespace` filters are not excluding all data

### Datasource not found

- The dashboard uses a template variable `$datasource` of type `prometheus`
- Ensure at least one Prometheus-type datasource exists in Grafana
- When importing manually, select the correct datasource during the import dialog
