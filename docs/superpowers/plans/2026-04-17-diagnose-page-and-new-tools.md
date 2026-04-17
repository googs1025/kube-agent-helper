# Diagnose Page + New MCP Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 5 new MCP diagnostic tools, a user-facing `/diagnose` page with symptom-driven entry, a `/diagnose/[id]` result page sorted by severity, and a lightweight K8s resource query API for autocomplete.

**Architecture:** New MCP tools follow existing handler pattern (factory func + `Deps` + `jsonResult`). Dashboard adds two new routes (`/diagnose`, `/diagnose/[id]`) using existing i18n/SWR patterns. One new backend API endpoint (`/api/k8s/resources`) proxies read-only K8s queries for autocomplete. Existing admin pages and CRD schema are unchanged.

**Tech Stack:** Go 1.22 (MCP tools + HTTP handler), Next.js + React + SWR (dashboard), `k8s.io/client-go` (K8s API), `mcp-go` (MCP protocol), `testify` + `client-go/fake` (testing)

---

### Task 1: MCP Tool — `kubectl_rollout_status`

**Files:**
- Create: `internal/mcptools/rollout_status.go`
- Create: `internal/mcptools/rollout_status_test.go`
- Modify: `internal/mcptools/register.go:55-90` (add to RegisterExtension)

- [ ] **Step 1: Write the failing test**

Create `internal/mcptools/rollout_status_test.go`:

```go
package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestRolloutStatus_MissingArgs(t *testing.T) {
	d := &Deps{Typed: k8sfake.NewSimpleClientset()}
	handler := NewRolloutStatusHandler(d)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestRolloutStatus_DeploymentWithReplicaSets(t *testing.T) {
	replicas := int32(3)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "prod",
			UID: "deploy-uid-1",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          3,
			UpdatedReplicas:   3,
			ReadyReplicas:     3,
			AvailableReplicas: 3,
			Conditions: []appsv1.DeploymentCondition{{
				Type:    appsv1.DeploymentAvailable,
				Status:  corev1.ConditionTrue,
				Message: "Deployment has minimum availability",
			}},
		},
	}
	isController := true
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web-abc123", Namespace: "prod",
			OwnerReferences: []metav1.OwnerReference{{
				UID:        "deploy-uid-1",
				Controller: &isController,
			}},
			CreationTimestamp: metav1.Now(),
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &replicas,
		},
		Status: appsv1.ReplicaSetStatus{
			Replicas:      3,
			ReadyReplicas: 3,
		},
	}

	client := k8sfake.NewSimpleClientset(deploy, rs)
	d := &Deps{Typed: client}
	handler := NewRolloutStatusHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"kind": "Deployment", "name": "web", "namespace": "prod",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, "web", payload["name"])
	assert.Equal(t, float64(3), payload["readyReplicas"])
	rs_list, ok := payload["replicaSets"].([]interface{})
	require.True(t, ok)
	assert.Len(t, rs_list, 1)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestRolloutStatus -v`
Expected: FAIL — `NewRolloutStatusHandler` not defined

- [ ] **Step 3: Write implementation**

Create `internal/mcptools/rollout_status.go`:

```go
package mcptools

import (
	"context"
	"fmt"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewRolloutStatusHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]interface{})
		kind, _ := args["kind"].(string)
		name, _ := args["name"].(string)
		namespace, _ := args["namespace"].(string)
		if kind == "" || name == "" || namespace == "" {
			return mcp.NewToolResultError("kind, name, and namespace are required"), nil
		}

		switch kind {
		case "Deployment":
			return rolloutDeployment(ctx, d, namespace, name)
		case "StatefulSet":
			return rolloutStatefulSet(ctx, d, namespace, name)
		default:
			return mcp.NewToolResultError("kind must be Deployment or StatefulSet"), nil
		}
	}
}

func rolloutDeployment(ctx context.Context, d *Deps, ns, name string) (*mcp.CallToolResult, error) {
	deploy, err := d.Typed.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get deployment: %v", err)), nil
	}

	// List ReplicaSets owned by this Deployment
	rsList, err := d.Typed.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list replicasets: %v", err)), nil
	}

	var owned []map[string]interface{}
	for _, rs := range rsList.Items {
		for _, ref := range rs.OwnerReferences {
			if ref.Controller != nil && *ref.Controller && ref.UID == deploy.UID {
				// Extract image from pod template
				var image string
				if len(rs.Spec.Template.Spec.Containers) > 0 {
					image = rs.Spec.Template.Spec.Containers[0].Image
				}
				owned = append(owned, map[string]interface{}{
					"name":          rs.Name,
					"replicas":      rs.Status.Replicas,
					"readyReplicas": rs.Status.ReadyReplicas,
					"image":         image,
					"createdAt":     rs.CreationTimestamp.Format("2006-01-02T15:04:05Z"),
				})
			}
		}
	}
	// Sort by creation time descending (newest first)
	sort.SliceStable(owned, func(i, j int) bool {
		return owned[i]["createdAt"].(string) > owned[j]["createdAt"].(string)
	})

	// Determine status
	status := "complete"
	if deploy.Status.UpdatedReplicas < *deploy.Spec.Replicas {
		status = "progressing"
	} else if deploy.Status.AvailableReplicas < *deploy.Spec.Replicas {
		status = "progressing"
	}
	for _, c := range deploy.Status.Conditions {
		if c.Type == "Progressing" && c.Reason == "ProgressDeadlineExceeded" {
			status = "degraded"
		}
	}

	conditions := make([]map[string]interface{}, 0, len(deploy.Status.Conditions))
	for _, c := range deploy.Status.Conditions {
		conditions = append(conditions, map[string]interface{}{
			"type":    string(c.Type),
			"status":  string(c.Status),
			"reason":  c.Reason,
			"message": c.Message,
		})
	}

	return jsonResult(map[string]interface{}{
		"name":              deploy.Name,
		"namespace":         deploy.Namespace,
		"status":            status,
		"desiredReplicas":   *deploy.Spec.Replicas,
		"updatedReplicas":   deploy.Status.UpdatedReplicas,
		"readyReplicas":     deploy.Status.ReadyReplicas,
		"availableReplicas": deploy.Status.AvailableReplicas,
		"conditions":        conditions,
		"replicaSets":       owned,
	})
}

func rolloutStatefulSet(ctx context.Context, d *Deps, ns, name string) (*mcp.CallToolResult, error) {
	sts, err := d.Typed.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get statefulset: %v", err)), nil
	}

	status := "complete"
	if sts.Status.UpdatedReplicas < *sts.Spec.Replicas {
		status = "progressing"
	} else if sts.Status.ReadyReplicas < *sts.Spec.Replicas {
		status = "progressing"
	}

	return jsonResult(map[string]interface{}{
		"name":              sts.Name,
		"namespace":         sts.Namespace,
		"status":            status,
		"desiredReplicas":   *sts.Spec.Replicas,
		"updatedReplicas":   sts.Status.UpdatedReplicas,
		"readyReplicas":     sts.Status.ReadyReplicas,
		"currentRevision":   sts.Status.CurrentRevision,
		"updateRevision":    sts.Status.UpdateRevision,
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestRolloutStatus -v`
Expected: PASS

- [ ] **Step 5: Register the tool**

In `internal/mcptools/register.go`, add at the end of `RegisterExtension()`:

```go
	registerTool(s, d, mcp.NewTool("kubectl_rollout_status",
		mcp.WithDescription("Show Deployment or StatefulSet rollout status with ReplicaSet history"),
		mcp.WithString("kind", mcp.Required(), mcp.Description("Deployment or StatefulSet")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Resource name")),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace")),
	), []string{"kind", "name", "namespace"}, NewRolloutStatusHandler(d))
```

- [ ] **Step 6: Run full tool test suite**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -v`
Expected: All tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/mcptools/rollout_status.go internal/mcptools/rollout_status_test.go internal/mcptools/register.go
git commit -m "feat(mcp): add kubectl_rollout_status tool for deployment/statefulset rollout diagnosis"
```

---

### Task 2: MCP Tool — `node_status_summary`

**Files:**
- Create: `internal/mcptools/node_status.go`
- Create: `internal/mcptools/node_status_test.go`
- Modify: `internal/mcptools/register.go` (add to RegisterExtension)

- [ ] **Step 1: Write the failing test**

Create `internal/mcptools/node_status_test.go`:

```go
package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNodeStatusSummary_Basic(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule}},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4000m"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "prod"},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{{
				Name: "main",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			}},
		},
	}

	client := k8sfake.NewSimpleClientset(node, pod)
	d := &Deps{Typed: client}
	handler := NewNodeStatusSummaryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"name": "node-1"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Nodes []struct {
			Name         string `json:"name"`
			Ready        bool   `json:"ready"`
			Unschedulable bool  `json:"unschedulable"`
			PodCount     int    `json:"podCount"`
		} `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	require.Len(t, payload.Nodes, 1)
	assert.Equal(t, "node-1", payload.Nodes[0].Name)
	assert.True(t, payload.Nodes[0].Ready)
	assert.Equal(t, 1, payload.Nodes[0].PodCount)
}

func TestNodeStatusSummary_NoTypedClient(t *testing.T) {
	d := &Deps{Typed: nil}
	handler := NewNodeStatusSummaryHandler(d)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestNodeStatusSummary -v`
Expected: FAIL — `NewNodeStatusSummaryHandler` not defined

- [ ] **Step 3: Write implementation**

Create `internal/mcptools/node_status.go`:

```go
package mcptools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewNodeStatusSummaryHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Typed == nil {
			return mcp.NewToolResultError("kubernetes typed client not available"), nil
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		name, _ := args["name"].(string)
		labelSelector, _ := args["labelSelector"].(string)

		var nodes []corev1.Node
		if name != "" {
			node, err := d.Typed.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get node: %v", err)), nil
			}
			nodes = []corev1.Node{*node}
		} else {
			list, err := d.Typed.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list nodes: %v", err)), nil
			}
			nodes = list.Items
			if len(nodes) > 20 {
				nodes = nodes[:20]
			}
		}

		// Gather all pods once (more efficient than per-node queries)
		podList, err := d.Typed.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list pods: %v", err)), nil
		}
		// Group pods by nodeName
		podsByNode := make(map[string][]corev1.Pod)
		for _, p := range podList.Items {
			if p.Spec.NodeName != "" {
				podsByNode[p.Spec.NodeName] = append(podsByNode[p.Spec.NodeName], p)
			}
		}

		result := make([]map[string]interface{}, 0, len(nodes))
		for _, node := range nodes {
			nodePods := podsByNode[node.Name]

			// Sum resource requests
			var cpuReqMilli, memReqMi int64
			for _, p := range nodePods {
				for _, c := range p.Spec.Containers {
					if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
						cpuReqMilli += q.MilliValue()
					}
					if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
						memReqMi += q.Value() / (1024 * 1024)
					}
				}
			}

			// Allocatable
			cpuAlloc := node.Status.Allocatable[corev1.ResourceCPU]
			memAlloc := node.Status.Allocatable[corev1.ResourceMemory]
			podAlloc := node.Status.Allocatable[corev1.ResourcePods]

			cpuAllocMilli := cpuAlloc.MilliValue()
			memAllocMi := memAlloc.Value() / (1024 * 1024)

			var cpuPct, memPct float64
			if cpuAllocMilli > 0 {
				cpuPct = float64(cpuReqMilli) / float64(cpuAllocMilli) * 100
			}
			if memAllocMi > 0 {
				memPct = float64(memReqMi) / float64(memAllocMi) * 100
			}

			// Conditions
			ready := false
			conditions := make([]map[string]interface{}, 0)
			for _, c := range node.Status.Conditions {
				conditions = append(conditions, map[string]interface{}{
					"type":    string(c.Type),
					"status":  string(c.Status),
					"reason":  c.Reason,
					"message": c.Message,
				})
				if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
					ready = true
				}
			}

			// Taints
			taints := make([]map[string]interface{}, 0, len(node.Spec.Taints))
			for _, t := range node.Spec.Taints {
				taints = append(taints, map[string]interface{}{
					"key": t.Key, "value": t.Value, "effect": string(t.Effect),
				})
			}

			result = append(result, map[string]interface{}{
				"name":          node.Name,
				"ready":         ready,
				"unschedulable": node.Spec.Unschedulable,
				"conditions":    conditions,
				"allocatable": map[string]interface{}{
					"cpuMilli": cpuAllocMilli,
					"memoryMi": memAllocMi,
					"pods":     podAlloc.Value(),
				},
				"allocated": map[string]interface{}{
					"cpuMilli": cpuReqMilli,
					"memoryMi": memReqMi,
				},
				"utilizationPct": map[string]interface{}{
					"cpu":    int(cpuPct),
					"memory": int(memPct),
				},
				"podCount": len(nodePods),
				"taints":   taints,
			})
		}

		return jsonResult(map[string]interface{}{"nodes": result})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestNodeStatusSummary -v`
Expected: PASS

- [ ] **Step 5: Register the tool**

In `internal/mcptools/register.go`, add to `RegisterExtension()`:

```go
	registerTool(s, d, mcp.NewTool("node_status_summary",
		mcp.WithDescription("Show node conditions, capacity, allocated resources, and taints"),
		mcp.WithString("name", mcp.Description("Specific node name (omit for all nodes, max 20)")),
		mcp.WithString("labelSelector", mcp.Description("Label selector to filter nodes")),
	), []string{"name", "labelSelector"}, NewNodeStatusSummaryHandler(d))
```

- [ ] **Step 6: Run full tool test suite**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -v`
Expected: All tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/mcptools/node_status.go internal/mcptools/node_status_test.go internal/mcptools/register.go
git commit -m "feat(mcp): add node_status_summary tool for node conditions and capacity diagnosis"
```

---

### Task 3: MCP Tool — `prometheus_alerts`

**Files:**
- Create: `internal/mcptools/prometheus_alerts.go`
- Create: `internal/mcptools/prometheus_alerts_test.go`
- Modify: `internal/mcptools/register.go` (add to RegisterExtension)

- [ ] **Step 1: Write the failing test**

Create `internal/mcptools/prometheus_alerts_test.go`:

```go
package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAlertsAPI struct {
	promv1.API
	alerts []promv1.Alert
}

func (f *fakeAlertsAPI) Alerts(ctx context.Context) (promv1.AlertsResult, error) {
	return promv1.AlertsResult{Alerts: f.alerts}, nil
}

func TestPrometheusAlerts_Unavailable(t *testing.T) {
	d := &Deps{Prometheus: nil}
	handler := NewPrometheusAlertsHandler(d)
	result, err := handler(context.Background(), mcp.CallToolRequest{})
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, false, payload["available"])
}

func TestPrometheusAlerts_FilterFiring(t *testing.T) {
	api := &fakeAlertsAPI{
		alerts: []promv1.Alert{
			{
				Labels:   model.LabelSet{"alertname": "HighCPU", "severity": "critical", "namespace": "prod"},
				State:    promv1.AlertStateFiring,
				ActiveAt: mustParseTime("2026-04-17T10:00:00Z"),
			},
			{
				Labels: model.LabelSet{"alertname": "DiskSlow", "severity": "warning"},
				State:  promv1.AlertStatePending,
			},
		},
	}
	d := &Deps{Prometheus: api}
	handler := NewPrometheusAlertsHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"state": "firing"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Available bool                     `json:"available"`
		Alerts    []map[string]interface{} `json:"alerts"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.True(t, payload.Available)
	require.Len(t, payload.Alerts, 1)
	assert.Equal(t, "HighCPU", payload.Alerts[0]["alertname"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestPrometheusAlerts -v`
Expected: FAIL — `NewPrometheusAlertsHandler` not defined

- [ ] **Step 3: Write implementation**

Create `internal/mcptools/prometheus_alerts.go`:

```go
package mcptools

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// AlertsAPI is the subset of promv1.API we need. Aids testing.
type AlertsAPI interface {
	Alerts(ctx context.Context) (promv1.AlertsResult, error)
}

func NewPrometheusAlertsHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Prometheus == nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "prometheus not configured (use --prometheus-url)",
			})
		}

		args, _ := req.Params.Arguments.(map[string]interface{})
		state, _ := args["state"].(string)
		if state == "" {
			state = "firing"
		}
		labelFilter, _ := args["labelFilter"].(string)

		// Parse label filter: "namespace=prod,severity=critical"
		filterMap := make(map[string]string)
		if labelFilter != "" {
			for _, pair := range strings.Split(labelFilter, ",") {
				kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
				if len(kv) == 2 {
					filterMap[kv[0]] = kv[1]
				}
			}
		}

		// d.Prometheus implements promv1.API which has Alerts()
		alerter, ok := d.Prometheus.(AlertsAPI)
		if !ok {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     "prometheus client does not support alerts API",
			})
		}

		result, err := alerter.Alerts(ctx)
		if err != nil {
			return jsonResult(map[string]interface{}{
				"available": false,
				"error":     err.Error(),
			})
		}

		sevOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}

		var filtered []map[string]interface{}
		for _, a := range result.Alerts {
			// Filter by state
			if state != "all" {
				if state == "firing" && a.State != promv1.AlertStateFiring {
					continue
				}
				if state == "pending" && a.State != promv1.AlertStatePending {
					continue
				}
			}
			// Filter by labels
			match := true
			for k, v := range filterMap {
				if string(a.Labels[model.LabelName(k)]) != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}

			labels := make(map[string]string, len(a.Labels))
			for k, v := range a.Labels {
				labels[string(k)] = string(v)
			}
			annotations := make(map[string]string, len(a.Annotations))
			for k, v := range a.Annotations {
				annotations[string(k)] = string(v)
			}

			entry := map[string]interface{}{
				"alertname":   string(a.Labels["alertname"]),
				"state":       string(a.State),
				"severity":    string(a.Labels["severity"]),
				"labels":      labels,
				"annotations": annotations,
			}
			if !a.ActiveAt.IsZero() {
				entry["activeAt"] = a.ActiveAt.UTC().Format(time.RFC3339)
			}
			if a.Value != "" {
				entry["value"] = string(a.Value)
			}

			filtered = append(filtered, entry)
		}

		// Sort: critical > warning > info, then by activeAt descending
		sort.SliceStable(filtered, func(i, j int) bool {
			si := sevOrder[filtered[i]["severity"].(string)]
			sj := sevOrder[filtered[j]["severity"].(string)]
			if si != sj {
				return si < sj
			}
			ai, _ := filtered[i]["activeAt"].(string)
			aj, _ := filtered[j]["activeAt"].(string)
			return ai > aj
		})

		return jsonResult(map[string]interface{}{
			"available": true,
			"count":     len(filtered),
			"alerts":    filtered,
		})
	}
}
```

Note: need to add the `model` import. The import line is: `"github.com/prometheus/common/model"`. Add it to the imports.

- [ ] **Step 4: Add `mustParseTime` helper to the test file**

Add at the bottom of `prometheus_alerts_test.go`:

```go
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
```

And add `"time"` to the test imports.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestPrometheusAlerts -v`
Expected: PASS

- [ ] **Step 6: Register the tool**

In `internal/mcptools/register.go`, add to `RegisterExtension()`:

```go
	registerTool(s, d, mcp.NewTool("prometheus_alerts",
		mcp.WithDescription("List active Prometheus alerts, sorted by severity"),
		mcp.WithString("state", mcp.Description("Filter: firing, pending, or all (default firing)")),
		mcp.WithString("labelFilter", mcp.Description("Filter by labels, e.g. namespace=prod,severity=critical")),
	), []string{"state", "labelFilter"}, NewPrometheusAlertsHandler(d))
```

- [ ] **Step 7: Run full test suite and commit**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -v`
Expected: All PASS

```bash
git add internal/mcptools/prometheus_alerts.go internal/mcptools/prometheus_alerts_test.go internal/mcptools/register.go
git commit -m "feat(mcp): add prometheus_alerts tool for active alert diagnosis"
```

---

### Task 4: MCP Tool — `pvc_status`

**Files:**
- Create: `internal/mcptools/pvc_status.go`
- Create: `internal/mcptools/pvc_status_test.go`
- Modify: `internal/mcptools/register.go` (add to RegisterExtension)

- [ ] **Step 1: Write the failing test**

Create `internal/mcptools/pvc_status_test.go`:

```go
package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestPVCStatus_BoundPVC(t *testing.T) {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data-vol", Namespace: "prod"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
			StorageClassName: strPtr("standard"),
			VolumeName:       "pv-123",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
		},
	}

	client := k8sfake.NewSimpleClientset(pvc)
	d := &Deps{Typed: client}
	handler := NewPVCStatusHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"namespace": "prod"}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		Items []struct {
			Name         string `json:"name"`
			Phase        string `json:"phase"`
			VolumeName   string `json:"volumeName"`
			StorageClass string `json:"storageClass"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	require.Len(t, payload.Items, 1)
	assert.Equal(t, "data-vol", payload.Items[0].Name)
	assert.Equal(t, "Bound", payload.Items[0].Phase)
	assert.Equal(t, "pv-123", payload.Items[0].VolumeName)
}

func TestPVCStatus_MissingNamespace(t *testing.T) {
	client := k8sfake.NewSimpleClientset()
	d := &Deps{Typed: client}
	handler := NewPVCStatusHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestPVCStatus -v`
Expected: FAIL — `NewPVCStatusHandler` not defined

- [ ] **Step 3: Write implementation**

Create `internal/mcptools/pvc_status.go`:

```go
package mcptools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewPVCStatusHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Typed == nil {
			return mcp.NewToolResultError("kubernetes typed client not available"), nil
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		namespace, _ := args["namespace"].(string)
		if namespace == "" {
			return mcp.NewToolResultError("namespace is required"), nil
		}
		name, _ := args["name"].(string)
		labelSelector, _ := args["labelSelector"].(string)

		var pvcs []corev1.PersistentVolumeClaim
		if name != "" {
			pvc, err := d.Typed.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("get pvc: %v", err)), nil
			}
			pvcs = []corev1.PersistentVolumeClaim{*pvc}
		} else {
			list, err := d.Typed.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("list pvcs: %v", err)), nil
			}
			pvcs = list.Items
		}

		items := make([]map[string]interface{}, 0, len(pvcs))
		for _, pvc := range pvcs {
			storageClass := ""
			if pvc.Spec.StorageClassName != nil {
				storageClass = *pvc.Spec.StorageClassName
			}
			requestedStorage := ""
			if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
				requestedStorage = q.String()
			}
			actualCapacity := ""
			if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
				actualCapacity = q.String()
			}
			accessModes := make([]string, len(pvc.Spec.AccessModes))
			for i, m := range pvc.Spec.AccessModes {
				accessModes[i] = string(m)
			}

			entry := map[string]interface{}{
				"name":             pvc.Name,
				"namespace":        pvc.Namespace,
				"phase":            string(pvc.Status.Phase),
				"storageClass":     storageClass,
				"requestedStorage": requestedStorage,
				"actualCapacity":   actualCapacity,
				"volumeName":       pvc.Spec.VolumeName,
				"accessModes":      accessModes,
			}

			// For Pending PVCs, fetch related events
			if pvc.Status.Phase == corev1.ClaimPending {
				events, _ := d.Typed.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
					FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=PersistentVolumeClaim", pvc.Name),
				})
				if events != nil && len(events.Items) > 0 {
					evList := make([]map[string]interface{}, 0)
					for _, ev := range events.Items {
						evList = append(evList, map[string]interface{}{
							"type":    ev.Type,
							"reason":  ev.Reason,
							"message": ev.Message,
						})
					}
					entry["events"] = evList
				}
			}

			items = append(items, entry)
		}

		return jsonResult(map[string]interface{}{"items": items})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestPVCStatus -v`
Expected: PASS

- [ ] **Step 5: Register the tool**

In `internal/mcptools/register.go`, add to `RegisterExtension()`:

```go
	registerTool(s, d, mcp.NewTool("pvc_status",
		mcp.WithDescription("List PersistentVolumeClaim status, capacity, and binding info"),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace")),
		mcp.WithString("name", mcp.Description("Specific PVC name (omit to list all)")),
		mcp.WithString("labelSelector", mcp.Description("Label selector to filter PVCs")),
	), []string{"namespace", "name", "labelSelector"}, NewPVCStatusHandler(d))
```

- [ ] **Step 6: Run full test suite and commit**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -v`
Expected: All PASS

```bash
git add internal/mcptools/pvc_status.go internal/mcptools/pvc_status_test.go internal/mcptools/register.go
git commit -m "feat(mcp): add pvc_status tool for storage diagnosis"
```

---

### Task 5: MCP Tool — `network_policy_check`

**Files:**
- Create: `internal/mcptools/network_policy.go`
- Create: `internal/mcptools/network_policy_test.go`
- Modify: `internal/mcptools/register.go` (add to RegisterExtension)

- [ ] **Step 1: Write the failing test**

Create `internal/mcptools/network_policy_test.go`:

```go
package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNetworkPolicyCheck_MissingArgs(t *testing.T) {
	client := k8sfake.NewSimpleClientset()
	d := &Deps{Typed: client}
	handler := NewNetworkPolicyCheckHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestNetworkPolicyCheck_MatchingPolicy(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web-1", Namespace: "prod",
			Labels: map[string]string{"app": "web", "tier": "frontend"},
		},
	}
	port80 := intstr.FromInt(80)
	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-frontend", Namespace: "prod"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "api"},
					},
				}},
				Ports: []networkingv1.NetworkPolicyPort{{
					Port: &port80,
				}},
			}},
		},
	}

	client := k8sfake.NewSimpleClientset(pod, policy)
	d := &Deps{Typed: client}
	handler := NewNetworkPolicyCheckHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace": "prod", "podName": "web-1",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload struct {
		PodLabels        map[string]string        `json:"podLabels"`
		MatchingPolicies []map[string]interface{} `json:"matchingPolicies"`
	}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, "web", payload.PodLabels["app"])
	require.Len(t, payload.MatchingPolicies, 1)
	assert.Equal(t, "allow-frontend", payload.MatchingPolicies[0]["name"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestNetworkPolicyCheck -v`
Expected: FAIL — `NewNetworkPolicyCheckHandler` not defined

- [ ] **Step 3: Write implementation**

Create `internal/mcptools/network_policy.go`:

```go
package mcptools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func NewNetworkPolicyCheckHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Typed == nil {
			return mcp.NewToolResultError("kubernetes typed client not available"), nil
		}
		args, _ := req.Params.Arguments.(map[string]interface{})
		namespace, _ := args["namespace"].(string)
		podName, _ := args["podName"].(string)
		if namespace == "" || podName == "" {
			return mcp.NewToolResultError("namespace and podName are required"), nil
		}

		pod, err := d.Typed.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get pod: %v", err)), nil
		}

		npList, err := d.Typed.NetworkingV1().NetworkPolicies(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list network policies: %v", err)), nil
		}

		var matching []map[string]interface{}
		defaultDeny := false

		for _, np := range npList.Items {
			sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
			if err != nil {
				continue
			}
			if !sel.Matches(labels.Set(pod.Labels)) {
				continue
			}

			// Check if this is a default-deny (empty selector + no rules)
			if sel.Empty() && len(np.Spec.Ingress) == 0 && len(np.Spec.Egress) == 0 {
				defaultDeny = true
			}

			policyTypes := make([]string, len(np.Spec.PolicyTypes))
			for i, pt := range np.Spec.PolicyTypes {
				policyTypes[i] = string(pt)
			}

			// Summarize ingress rules
			ingressRules := make([]map[string]interface{}, 0)
			for _, rule := range np.Spec.Ingress {
				from := make([]string, 0)
				for _, peer := range rule.From {
					if peer.PodSelector != nil {
						from = append(from, fmt.Sprintf("pods(%v)", peer.PodSelector.MatchLabels))
					}
					if peer.NamespaceSelector != nil {
						from = append(from, fmt.Sprintf("namespaces(%v)", peer.NamespaceSelector.MatchLabels))
					}
					if peer.IPBlock != nil {
						cidr := peer.IPBlock.CIDR
						from = append(from, fmt.Sprintf("cidr(%s)", cidr))
					}
				}
				ports := make([]string, 0)
				for _, p := range rule.Ports {
					proto := "TCP"
					if p.Protocol != nil {
						proto = string(*p.Protocol)
					}
					if p.Port != nil {
						ports = append(ports, fmt.Sprintf("%s/%s", proto, p.Port.String()))
					}
				}
				ingressRules = append(ingressRules, map[string]interface{}{
					"from": from, "ports": ports,
				})
			}

			// Summarize egress rules
			egressRules := make([]map[string]interface{}, 0)
			for _, rule := range np.Spec.Egress {
				to := make([]string, 0)
				for _, peer := range rule.To {
					if peer.PodSelector != nil {
						to = append(to, fmt.Sprintf("pods(%v)", peer.PodSelector.MatchLabels))
					}
					if peer.NamespaceSelector != nil {
						to = append(to, fmt.Sprintf("namespaces(%v)", peer.NamespaceSelector.MatchLabels))
					}
					if peer.IPBlock != nil {
						to = append(to, fmt.Sprintf("cidr(%s)", peer.IPBlock.CIDR))
					}
				}
				ports := make([]string, 0)
				for _, p := range rule.Ports {
					proto := "TCP"
					if p.Protocol != nil {
						proto = string(*p.Protocol)
					}
					if p.Port != nil {
						ports = append(ports, fmt.Sprintf("%s/%s", proto, p.Port.String()))
					}
				}
				egressRules = append(egressRules, map[string]interface{}{
					"to": to, "ports": ports,
				})
			}

			matching = append(matching, map[string]interface{}{
				"name":         np.Name,
				"policyTypes":  policyTypes,
				"ingressRules": ingressRules,
				"egressRules":  egressRules,
			})
		}

		// Build summary text
		summary := fmt.Sprintf("%d NetworkPolicy(ies) match this pod.", len(matching))
		if len(matching) == 0 {
			summary = "No NetworkPolicies match this pod. All traffic is allowed by default."
		}
		if defaultDeny {
			summary += " A default-deny policy is in effect."
		}

		return jsonResult(map[string]interface{}{
			"podLabels":        pod.Labels,
			"matchingPolicies": matching,
			"defaultDeny":      defaultDeny,
			"summary":          summary,
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -run TestNetworkPolicyCheck -v`
Expected: PASS

- [ ] **Step 5: Register the tool**

In `internal/mcptools/register.go`, add to `RegisterExtension()`:

```go
	registerTool(s, d, mcp.NewTool("network_policy_check",
		mcp.WithDescription("Analyze NetworkPolicies affecting a specific Pod"),
		mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace of the target Pod")),
		mcp.WithString("podName", mcp.Required(), mcp.Description("Name of the Pod to analyze")),
	), []string{"namespace", "podName"}, NewNetworkPolicyCheckHandler(d))
```

- [ ] **Step 6: Run full test suite and commit**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/mcptools/ -v`
Expected: All PASS

```bash
git add internal/mcptools/network_policy.go internal/mcptools/network_policy_test.go internal/mcptools/register.go
git commit -m "feat(mcp): add network_policy_check tool for network diagnosis"
```

---

### Task 6: Update Builtin Skill Prompts

**Files:**
- Modify: `skills/pod-health-analyst.md`
- Modify: `skills/pod-cost-analyst.md`
- Modify: `skills/reliability-analyst.md`
- Modify: `skills/config-drift-analyst.md`

- [ ] **Step 1: Update pod-health-analyst.md**

Change the tools line and add instructions for new tools:

```markdown
---
name: pod-health-analyst
dimension: health
tools: ["kubectl_get","kubectl_describe","kubectl_logs","events_list","kubectl_rollout_status","prometheus_alerts"]
requires_data: ["pods","events"]
---

You are a Kubernetes pod health specialist. Analyze all pods in the target namespaces.

## Instructions

1. List all pods using `kubectl_get` with kind=Pod for each target namespace.
2. For each pod that is NOT in Running or Succeeded state:
   - Use `kubectl_describe` to get details.
   - Use `events_list` to get related events.
   - If CrashLoopBackOff or OOMKilled, use `kubectl_logs` (previous=true) to get last crash logs.
3. Check for pods with high restart counts (>5 restarts).
4. If pods belong to a Deployment or StatefulSet, use `kubectl_rollout_status` to check if a rollout is stuck or recently changed.
5. Use `prometheus_alerts` to check for any firing alerts related to the target namespaces.
6. For each issue found, output one finding JSON per line:
   {"dimension":"health","severity":"<critical|high|medium|low>","title":"<short title>","description":"<what you observed>","resource_kind":"Pod","resource_namespace":"<ns>","resource_name":"<pod-name>","suggestion":"<actionable fix>"}

## Severity Guide
- critical: Pod won't start or is OOMKilled repeatedly
- high: CrashLoopBackOff or probe failures preventing traffic
- medium: High restart count but currently running
- low: Completed/evicted pods leaving stale entries
```

- [ ] **Step 2: Update pod-cost-analyst.md**

Change tools line:

```markdown
---
name: pod-cost-analyst
dimension: cost
tools: ["kubectl_get","top_pods","top_nodes","prometheus_query","node_status_summary"]
requires_data: ["pods","nodes","metrics"]
---
```

Add after step 6 (identify underutilized nodes):

```markdown
6. Use `node_status_summary` to check node capacity vs allocated resources. Identify nodes with very low utilization (<20% allocated) that could be candidates for consolidation.
```

- [ ] **Step 3: Update reliability-analyst.md**

Change tools line:

```markdown
---
name: reliability-analyst
dimension: reliability
tools: ["kubectl_get","kubectl_describe","events_list","kubectl_rollout_status","pvc_status","node_status_summary"]
requires_data: ["pods","events","deployments"]
---
```

Add new instructions after step 6:

```markdown
7. Use `kubectl_rollout_status` for Deployments with recent events or pods in non-Ready state to check if a rollout is stuck.
8. Use `pvc_status` to check for PVCs in Pending or Lost state that may block pod scheduling.
9. Use `node_status_summary` to check if any nodes have MemoryPressure, DiskPressure, or are NotReady.
```

- [ ] **Step 4: Update config-drift-analyst.md**

Change tools line:

```markdown
---
name: config-drift-analyst
dimension: reliability
tools: ["kubectl_get","kubectl_describe","network_policy_check"]
requires_data: ["pods","deployments","services","configmaps"]
---
```

Add after step 4:

```markdown
5. For Services with 0 endpoints, use `network_policy_check` on one of the intended backend pods to check if a NetworkPolicy might be blocking traffic.
```

- [ ] **Step 5: Commit**

```bash
git add skills/pod-health-analyst.md skills/pod-cost-analyst.md skills/reliability-analyst.md skills/config-drift-analyst.md
git commit -m "feat(skills): update builtin skills to use new M7 tools"
```

---

### Task 7: Backend API — `/api/k8s/resources`

**Files:**
- Modify: `internal/controller/httpserver/server.go` (add handler + route)
- Modify: `internal/controller/httpserver/server_test.go` (add tests)

- [ ] **Step 1: Write the failing test**

Add to `internal/controller/httpserver/server_test.go`:

```go
func TestK8sResources_ListNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}}
	nsSys := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, nsSys).Build()
	srv := httpserver.New(&fakeStore{}, k8s, nil)

	req := httptest.NewRequest("GET", "/api/k8s/resources?kind=Namespace", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var items []map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	// Should filter out kube-system
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item["name"]
	}
	assert.Contains(t, names, "production")
	assert.NotContains(t, names, "kube-system")
}

func TestK8sResources_ListDeployments(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "prod",
			Labels: map[string]string{"app": "web"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		},
	}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()
	srv := httpserver.New(&fakeStore{}, k8s, nil)

	req := httptest.NewRequest("GET", "/api/k8s/resources?kind=Deployment&namespace=prod", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var items []map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "web", items[0]["name"])
}

func TestK8sResources_GetSingleDeployment(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "prod",
			Labels: map[string]string{"app": "web"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		},
	}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()
	srv := httpserver.New(&fakeStore{}, k8s, nil)

	req := httptest.NewRequest("GET", "/api/k8s/resources?kind=Deployment&namespace=prod&name=web", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	meta := result["metadata"].(map[string]interface{})
	assert.Equal(t, "web", meta["name"])
	spec := result["spec"].(map[string]interface{})
	selector := spec["selector"].(map[string]interface{})
	matchLabels := selector["matchLabels"].(map[string]interface{})
	assert.Equal(t, "web", matchLabels["app"])
}
```

Add these imports to the test file's import block:

```go
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/ -run TestK8sResources -v`
Expected: FAIL — route not registered, 404

- [ ] **Step 3: Write implementation**

Add route registration in `server.go` `New()` function, after the existing HandleFunc lines:

```go
	srv.mux.HandleFunc("/api/k8s/resources", srv.handleAPIK8sResources)
```

Add the handler method to `server.go`:

```go
// GET /api/k8s/resources?kind=X&namespace=Y&name=Z
func (s *Server) handleAPIK8sResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kind := r.URL.Query().Get("kind")
	namespace := r.URL.Query().Get("namespace")
	name := r.URL.Query().Get("name")

	if kind == "" {
		http.Error(w, "kind query parameter is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	switch kind {
	case "Namespace":
		s.handleListNamespaces(ctx, w)
	case "Deployment":
		s.handleK8sResource(ctx, w, namespace, name, &appsv1.DeploymentList{}, &appsv1.Deployment{})
	case "Pod":
		s.handleK8sResource(ctx, w, namespace, name, &corev1.PodList{}, &corev1.Pod{})
	case "StatefulSet":
		s.handleK8sResource(ctx, w, namespace, name, &appsv1.StatefulSetList{}, &appsv1.StatefulSet{})
	case "DaemonSet":
		s.handleK8sResource(ctx, w, namespace, name, &appsv1.DaemonSetList{}, &appsv1.DaemonSet{})
	default:
		http.Error(w, "unsupported kind: "+kind, http.StatusBadRequest)
	}
}

func (s *Server) handleListNamespaces(ctx context.Context, w http.ResponseWriter) {
	var list corev1.NamespaceList
	if err := s.k8sClient.List(ctx, &list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}
	items := make([]map[string]string, 0)
	for _, ns := range list.Items {
		if systemNS[ns.Name] {
			continue
		}
		items = append(items, map[string]string{"name": ns.Name})
	}
	writeJSON(w, items)
}

func (s *Server) handleK8sResource(ctx context.Context, w http.ResponseWriter, namespace, name string, listObj client.ObjectList, singleObj client.Object) {
	if namespace == "" {
		http.Error(w, "namespace is required for this kind", http.StatusBadRequest)
		return
	}

	if name != "" {
		// Get single resource with full metadata (for label resolution)
		key := client.ObjectKey{Namespace: namespace, Name: name}
		if err := s.k8sClient.Get(ctx, key, singleObj); err != nil {
			if errors.IsNotFound(err) {
				http.NotFound(w, nil)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, singleObj)
		return
	}

	// List mode — return compact name/namespace pairs
	if err := s.k8sClient.List(ctx, listObj, client.InNamespace(namespace)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Use reflection-free approach: marshal then extract names
	raw, _ := json.Marshal(listObj)
	var parsed struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		} `json:"items"`
	}
	_ = json.Unmarshal(raw, &parsed)
	items := make([]map[string]string, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		items = append(items, map[string]string{
			"name":      item.Metadata.Name,
			"namespace": item.Metadata.Namespace,
		})
	}
	writeJSON(w, items)
}
```

Add the needed imports to `server.go`:

```go
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
```

Note: `errors` import for `errors.IsNotFound` is already present as `"k8s.io/apimachinery/pkg/api/errors"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/ -run TestK8sResources -v`
Expected: PASS

- [ ] **Step 5: Run full httpserver test suite**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./internal/controller/httpserver/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/controller/httpserver/server.go internal/controller/httpserver/server_test.go
git commit -m "feat(api): add /api/k8s/resources endpoint for resource autocomplete"
```

---

### Task 8: Dashboard — Symptom Config + API Helpers

**Files:**
- Create: `dashboard/src/lib/symptoms.ts`
- Modify: `dashboard/src/lib/api.ts` (add k8s resource query helpers)
- Modify: `dashboard/src/lib/types.ts` (add DiagnoseForm type)

- [ ] **Step 1: Create symptoms.ts**

Create `dashboard/src/lib/symptoms.ts`:

```typescript
export interface SymptomPreset {
  id: string;
  label_zh: string;
  label_en: string;
  skills: string[];
}

export const SYMPTOM_PRESETS: SymptomPreset[] = [
  {
    id: "cpu-high",
    label_zh: "CPU 利用率高",
    label_en: "High CPU usage",
    skills: ["pod-health-analyst", "pod-cost-analyst"],
  },
  {
    id: "memory-high",
    label_zh: "内存使用率高 / OOMKill",
    label_en: "High memory / OOMKill",
    skills: ["pod-health-analyst", "pod-cost-analyst"],
  },
  {
    id: "request-slow",
    label_zh: "请求延迟高 / 服务不通",
    label_en: "Slow requests / service unreachable",
    skills: ["pod-health-analyst", "config-drift-analyst"],
  },
  {
    id: "pod-restart",
    label_zh: "Pod 频繁重启",
    label_en: "Pod frequent restarts",
    skills: ["pod-health-analyst", "reliability-analyst"],
  },
  {
    id: "pod-not-start",
    label_zh: "Pod 启动失败",
    label_en: "Pod failed to start",
    skills: ["pod-health-analyst", "config-drift-analyst", "reliability-analyst"],
  },
  {
    id: "scaling-issue",
    label_zh: "扩缩容异常",
    label_en: "Scaling issues (HPA)",
    skills: ["pod-cost-analyst", "reliability-analyst"],
  },
  {
    id: "rollout-stuck",
    label_zh: "滚动更新卡住",
    label_en: "Rollout stuck",
    skills: ["pod-health-analyst", "reliability-analyst"],
  },
  {
    id: "full-check",
    label_zh: "全面体检",
    label_en: "Full health check",
    skills: [],
  },
];

export function symptomsToSkills(symptomIds: string[]): string[] | undefined {
  if (symptomIds.includes("full-check")) return undefined;
  const skills = new Set<string>();
  for (const id of symptomIds) {
    const preset = SYMPTOM_PRESETS.find((p) => p.id === id);
    if (preset) {
      for (const s of preset.skills) skills.add(s);
    }
  }
  return skills.size > 0 ? Array.from(skills) : undefined;
}
```

- [ ] **Step 2: Add types to types.ts**

Append to `dashboard/src/lib/types.ts`:

```typescript
export interface K8sResourceItem {
  name: string;
  namespace?: string;
}
```

- [ ] **Step 3: Add API helpers to api.ts**

Append to `dashboard/src/lib/api.ts`:

```typescript
import type { K8sResourceItem } from "./types";

export function useK8sNamespaces() {
  return useSWR<K8sResourceItem[]>("/api/k8s/resources?kind=Namespace", fetcher);
}

export function useK8sResources(kind: string, namespace: string) {
  const url = namespace
    ? `/api/k8s/resources?kind=${kind}&namespace=${namespace}`
    : null;
  return useSWR<K8sResourceItem[]>(url, fetcher);
}

export async function getK8sResourceDetail(
  kind: string,
  namespace: string,
  name: string
): Promise<Record<string, unknown>> {
  const res = await fetch(
    `/api/k8s/resources?kind=${kind}&namespace=${namespace}&name=${name}`
  );
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}
```

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/lib/symptoms.ts dashboard/src/lib/api.ts dashboard/src/lib/types.ts
git commit -m "feat(dashboard): add symptom config and k8s resource API helpers"
```

---

### Task 9: Dashboard — `/diagnose` Page

**Files:**
- Create: `dashboard/src/app/diagnose/page.tsx`
- Modify: `dashboard/src/i18n/zh.json` (add diagnose keys)
- Modify: `dashboard/src/i18n/en.json` (add diagnose keys)
- Modify: `dashboard/src/app/layout.tsx` (add nav item)

- [ ] **Step 1: Add i18n keys**

Add to `zh.json` top-level:

```json
  "diagnose": {
    "title": "快速诊断",
    "namespace": "命名空间",
    "namespacePlaceholder": "选择命名空间",
    "resourceType": "资源类型",
    "resourceName": "资源名称",
    "resourceNamePlaceholder": "选择资源",
    "symptoms": "你观察到的症状",
    "symptomsHint": "可多选，平台自动选择合适的诊断策略",
    "outputLanguage": "输出语言",
    "submit": "开始诊断",
    "submitting": "创建中...",
    "error": "创建诊断失败",
    "recent": "最近的诊断",
    "recentEmpty": "暂无诊断记录",
    "findings_count": "个问题"
  }
```

Add corresponding English keys to `en.json`:

```json
  "diagnose": {
    "title": "Quick Diagnose",
    "namespace": "Namespace",
    "namespacePlaceholder": "Select namespace",
    "resourceType": "Resource Type",
    "resourceName": "Resource Name",
    "resourceNamePlaceholder": "Select resource",
    "symptoms": "Observed Symptoms",
    "symptomsHint": "Select one or more, platform auto-selects diagnostic skills",
    "outputLanguage": "Output Language",
    "submit": "Start Diagnosis",
    "submitting": "Creating...",
    "error": "Failed to create diagnosis",
    "recent": "Recent Diagnoses",
    "recentEmpty": "No diagnoses yet",
    "findings_count": "findings"
  }
```

Also add to nav sections:

```json
  "nav": {
    ...
    "diagnose": "诊断"
  }
```

(English: `"diagnose": "Diagnose"`)

- [ ] **Step 2: Add nav item to layout.tsx**

In `dashboard/src/app/layout.tsx`, add the Diagnose link as the first item in the nav:

```tsx
        <div className="flex flex-1 gap-6 text-sm">
          <Link href="/diagnose" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.diagnose")}</Link>
          <Link href="/" className="text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-100">{t("nav.runs")}</Link>
```

- [ ] **Step 3: Create /diagnose/page.tsx**

Create `dashboard/src/app/diagnose/page.tsx`:

```tsx
"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useI18n } from "@/i18n/context";
import { useK8sNamespaces, useK8sResources, getK8sResourceDetail, createRun, useRuns } from "@/lib/api";
import { SYMPTOM_PRESETS, symptomsToSkills } from "@/lib/symptoms";
import { PhaseBadge } from "@/components/phase-badge";
import Link from "next/link";

const RESOURCE_TYPES = ["Deployment", "Pod", "StatefulSet", "DaemonSet"];

export default function DiagnosePage() {
  const { t, lang } = useI18n();
  const router = useRouter();

  const [namespace, setNamespace] = useState("");
  const [resourceType, setResourceType] = useState("Deployment");
  const [resourceName, setResourceName] = useState("");
  const [symptoms, setSymptoms] = useState<string[]>([]);
  const [outputLang, setOutputLang] = useState<"zh" | "en">("zh");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  const { data: namespaces } = useK8sNamespaces();
  const { data: resources } = useK8sResources(resourceType, namespace);
  const { data: runs } = useRuns();

  const toggleSymptom = (id: string) => {
    if (id === "full-check") {
      setSymptoms(["full-check"]);
      return;
    }
    setSymptoms((prev) => {
      const without = prev.filter((s) => s !== "full-check");
      return without.includes(id) ? without.filter((s) => s !== id) : [...without, id];
    });
  };

  const handleSubmit = async () => {
    if (!namespace || symptoms.length === 0) return;
    setSubmitting(true);
    setError("");

    try {
      // Resolve labels from the selected resource
      let labelSelector: Record<string, string> | undefined;
      if (resourceName) {
        const detail = await getK8sResourceDetail(resourceType, namespace, resourceName);
        const spec = (detail as Record<string, unknown>).spec as Record<string, unknown> | undefined;
        const selector = spec?.selector as Record<string, unknown> | undefined;
        const matchLabels = selector?.matchLabels as Record<string, string> | undefined;

        if (matchLabels && (resourceType === "Deployment" || resourceType === "StatefulSet")) {
          labelSelector = matchLabels;
        } else {
          const meta = (detail as Record<string, unknown>).metadata as Record<string, unknown>;
          const labels = meta?.labels as Record<string, string> | undefined;
          if (labels) {
            const appLabel = labels["app"] || labels["app.kubernetes.io/name"];
            if (appLabel) {
              labelSelector = { app: appLabel };
            }
          }
        }
      }

      const symptomSuffix = symptoms.slice(0, 2).join("-");
      const runName = resourceName
        ? `diagnose-${resourceName}-${symptomSuffix}-${Math.random().toString(36).slice(2, 6)}`
        : `diagnose-${namespace}-${symptomSuffix}-${Math.random().toString(36).slice(2, 6)}`;

      await createRun({
        name: runName,
        namespace: "kube-agent-helper",
        target: {
          scope: "namespace",
          namespaces: [namespace],
          labelSelector,
        },
        skills: symptomsToSkills(symptoms),
        modelConfigRef: "anthropic-credentials",
        outputLanguage: outputLang,
      });

      // Navigate to diagnose result page
      // We use the run name to find it — redirect to the diagnose results view
      router.push(`/diagnose/${encodeURIComponent(runName)}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  // Recent diagnoses: filter runs whose name starts with "diagnose-"
  const recentDiagnoses = (runs || [])
    .filter((r) => r.ID && (r.TargetJSON || "").includes("namespace"))
    .slice(0, 5);

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-bold">{t("diagnose.title")}</h1>

      <div className="rounded-lg border bg-white p-6 shadow-sm dark:bg-gray-900 dark:border-gray-800 space-y-6">
        {/* Namespace */}
        <div>
          <label className="block text-sm font-medium mb-1">{t("diagnose.namespace")}</label>
          <select
            value={namespace}
            onChange={(e) => { setNamespace(e.target.value); setResourceName(""); }}
            className="w-full rounded border px-3 py-2 text-sm dark:bg-gray-800 dark:border-gray-700"
          >
            <option value="">{t("diagnose.namespacePlaceholder")}</option>
            {(namespaces || []).map((ns) => (
              <option key={ns.name} value={ns.name}>{ns.name}</option>
            ))}
          </select>
        </div>

        {/* Resource Type */}
        <div>
          <label className="block text-sm font-medium mb-1">{t("diagnose.resourceType")}</label>
          <div className="flex gap-3">
            {RESOURCE_TYPES.map((rt) => (
              <label key={rt} className="flex items-center gap-1.5 text-sm cursor-pointer">
                <input
                  type="radio" name="resourceType" value={rt}
                  checked={resourceType === rt}
                  onChange={() => { setResourceType(rt); setResourceName(""); }}
                />
                {rt}
              </label>
            ))}
          </div>
        </div>

        {/* Resource Name */}
        <div>
          <label className="block text-sm font-medium mb-1">
            {t("diagnose.resourceName")}
            <span className="text-gray-400 ml-1 font-normal">({t("common.none")} = {lang === "zh" ? "全部" : "all"})</span>
          </label>
          <select
            value={resourceName}
            onChange={(e) => setResourceName(e.target.value)}
            disabled={!namespace}
            className="w-full rounded border px-3 py-2 text-sm dark:bg-gray-800 dark:border-gray-700 disabled:opacity-50"
          >
            <option value="">{t("diagnose.resourceNamePlaceholder")}</option>
            {(resources || []).map((r) => (
              <option key={r.name} value={r.name}>{r.name}</option>
            ))}
          </select>
        </div>

        {/* Symptoms */}
        <div>
          <label className="block text-sm font-medium mb-1">{t("diagnose.symptoms")}</label>
          <p className="text-xs text-gray-500 mb-2">{t("diagnose.symptomsHint")}</p>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
            {SYMPTOM_PRESETS.map((s) => (
              <label
                key={s.id}
                className={`flex items-center gap-2 rounded border px-3 py-2 text-sm cursor-pointer transition-colors ${
                  symptoms.includes(s.id)
                    ? "border-blue-500 bg-blue-50 dark:bg-blue-900/30 dark:border-blue-400"
                    : "border-gray-200 dark:border-gray-700 hover:border-gray-400"
                }`}
              >
                <input
                  type="checkbox"
                  checked={symptoms.includes(s.id)}
                  onChange={() => toggleSymptom(s.id)}
                  className="sr-only"
                />
                {lang === "zh" ? s.label_zh : s.label_en}
              </label>
            ))}
          </div>
        </div>

        {/* Output Language */}
        <div>
          <label className="block text-sm font-medium mb-1">{t("diagnose.outputLanguage")}</label>
          <div className="flex gap-4">
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input type="radio" name="outputLang" value="zh" checked={outputLang === "zh"} onChange={() => setOutputLang("zh")} />
              中文
            </label>
            <label className="flex items-center gap-1.5 text-sm cursor-pointer">
              <input type="radio" name="outputLang" value="en" checked={outputLang === "en"} onChange={() => setOutputLang("en")} />
              English
            </label>
          </div>
        </div>

        {/* Submit */}
        {error && <p className="text-sm text-red-600">{t("diagnose.error")}: {error}</p>}
        <button
          onClick={handleSubmit}
          disabled={submitting || !namespace || symptoms.length === 0}
          className="rounded bg-blue-600 px-6 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {submitting ? t("diagnose.submitting") : t("diagnose.submit")}
        </button>
      </div>

      {/* Recent Diagnoses */}
      <div>
        <h2 className="text-lg font-semibold mb-3">{t("diagnose.recent")}</h2>
        {recentDiagnoses.length === 0 ? (
          <p className="text-sm text-gray-500">{t("diagnose.recentEmpty")}</p>
        ) : (
          <div className="space-y-2">
            {recentDiagnoses.map((run) => (
              <Link
                key={run.ID}
                href={`/diagnose/${encodeURIComponent(run.ID)}`}
                className="flex items-center justify-between rounded border px-4 py-3 hover:bg-gray-50 dark:border-gray-800 dark:hover:bg-gray-800/50"
              >
                <div className="flex items-center gap-3">
                  <PhaseBadge phase={run.Status} />
                  <span className="text-sm font-medium">{run.ID}</span>
                </div>
                <span className="text-xs text-gray-500">
                  {new Date(run.CreatedAt).toLocaleString()}
                </span>
              </Link>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Verify the page renders**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Build succeeds without errors

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/app/diagnose/page.tsx dashboard/src/app/layout.tsx dashboard/src/i18n/zh.json dashboard/src/i18n/en.json
git commit -m "feat(dashboard): add /diagnose page with symptom-driven diagnostic entry"
```

---

### Task 10: Dashboard — `/diagnose/[id]` Result Page

**Files:**
- Create: `dashboard/src/app/diagnose/[id]/page.tsx`

- [ ] **Step 1: Create the result page**

Create `dashboard/src/app/diagnose/[id]/page.tsx`:

```tsx
"use client";

import { use } from "react";
import Link from "next/link";
import { useI18n } from "@/i18n/context";
import { useRun, useFindings, generateFix } from "@/lib/api";
import { SeverityBadge } from "@/components/severity-badge";
import { PhaseBadge } from "@/components/phase-badge";
import type { Finding } from "@/lib/types";
import { useState } from "react";

const SEVERITY_ORDER: Record<string, number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3,
  info: 4,
};

const SEVERITY_STYLES: Record<string, { border: string; bg: string; icon: string }> = {
  critical: { border: "border-red-300 dark:border-red-700", bg: "bg-red-50 dark:bg-red-900/20", icon: "🔴" },
  high: { border: "border-orange-300 dark:border-orange-700", bg: "bg-orange-50 dark:bg-orange-900/20", icon: "🟠" },
  medium: { border: "border-yellow-300 dark:border-yellow-700", bg: "bg-yellow-50 dark:bg-yellow-900/20", icon: "🟡" },
  low: { border: "border-blue-300 dark:border-blue-700", bg: "bg-blue-50 dark:bg-blue-900/20", icon: "🔵" },
  info: { border: "border-gray-200 dark:border-gray-700", bg: "bg-gray-50 dark:bg-gray-900/20", icon: "⚪" },
};

function sortFindings(findings: Finding[]): Finding[] {
  return [...findings].sort(
    (a, b) => (SEVERITY_ORDER[a.Severity] ?? 99) - (SEVERITY_ORDER[b.Severity] ?? 99)
  );
}

function groupBySeverity(findings: Finding[]): [string, Finding[]][] {
  const sorted = sortFindings(findings);
  const groups = new Map<string, Finding[]>();
  for (const f of sorted) {
    const g = groups.get(f.Severity) || [];
    g.push(f);
    groups.set(f.Severity, g);
  }
  return Array.from(groups.entries());
}

export default function DiagnoseResultPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { t, lang } = useI18n();
  const { data: run, error: runError } = useRun(id);
  const { data: findings } = useFindings(id);
  const [generatingIds, setGeneratingIds] = useState<Set<string>>(new Set());

  if (runError) return <p className="text-red-600">{t("common.loadFailed")}</p>;
  if (!run) return <p>{t("common.loading")}</p>;

  const grouped = findings ? groupBySeverity(findings) : [];
  const totalFindings = findings?.length ?? 0;

  // Parse run name for display
  const displayName = run.ID.startsWith("diagnose-")
    ? run.ID.replace(/^diagnose-/, "").replace(/-[a-z0-9]{4}$/, "").replace(/-/g, " ")
    : run.ID;

  const handleGenerateFix = async (findingId: string) => {
    setGeneratingIds((prev) => new Set(prev).add(findingId));
    try {
      await generateFix(findingId);
    } catch {
      // ignore — will show via fix link on next poll
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Link href="/diagnose" className="text-sm text-blue-600 hover:underline">
          ← {t("diagnose.title")}
        </Link>
      </div>

      <div className="rounded-lg border bg-white p-6 shadow-sm dark:bg-gray-900 dark:border-gray-800">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold">{displayName}</h1>
            <p className="text-sm text-gray-500 mt-1">
              {run.ID}
            </p>
          </div>
          <PhaseBadge phase={run.Status} />
        </div>

        <div className="mt-4 grid grid-cols-3 gap-4 text-sm">
          <div>
            <span className="text-gray-500">{t("runs.detail.created")}</span>
            <p>{new Date(run.CreatedAt).toLocaleString()}</p>
          </div>
          <div>
            <span className="text-gray-500">{t("runs.detail.completed")}</span>
            <p>{run.CompletedAt ? new Date(run.CompletedAt).toLocaleString() : "-"}</p>
          </div>
          <div>
            <span className="text-gray-500">{t("runs.detail.findings")}</span>
            <p className="font-semibold">{totalFindings}</p>
          </div>
        </div>

        {run.Status === "Running" && (
          <div className="mt-4 flex items-center gap-2 text-sm text-blue-600">
            <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            {lang === "zh" ? "诊断中..." : "Diagnosing..."}
          </div>
        )}

        {run.Status === "Failed" && run.Message && (
          <div className="mt-4 rounded bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-400">
            {run.Message}
          </div>
        )}
      </div>

      {/* Findings grouped by severity */}
      {totalFindings === 0 && run.Status === "Succeeded" && (
        <p className="text-sm text-gray-500">{t("runs.findings.empty")}</p>
      )}

      {grouped.map(([severity, items]) => {
        const style = SEVERITY_STYLES[severity] || SEVERITY_STYLES.info;
        return (
          <div key={severity} className="space-y-3">
            <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-gray-600 dark:text-gray-400">
              {style.icon} {t(`severity.${severity}` as never) || severity} ({items.length})
            </h2>
            {items.map((f) => (
              <div key={f.ID} className={`rounded-lg border ${style.border} ${style.bg} p-4 space-y-2`}>
                <div className="flex items-start justify-between gap-2">
                  <h3 className="font-medium">{f.Title}</h3>
                  <SeverityBadge severity={f.Severity} />
                </div>
                {f.ResourceKind && (
                  <p className="text-xs text-gray-500">
                    {f.ResourceKind}: {f.ResourceNamespace}/{f.ResourceName}
                  </p>
                )}
                <p className="text-sm">{f.Description}</p>
                {f.Suggestion && (
                  <div className="rounded bg-blue-50 p-3 text-sm text-blue-800 dark:bg-blue-900/30 dark:text-blue-300">
                    💡 {f.Suggestion}
                  </div>
                )}
                <div className="flex justify-end">
                  {f.FixID ? (
                    <Link
                      href={`/fixes/${f.FixID}`}
                      className="text-sm text-blue-600 hover:underline"
                    >
                      {t("runs.findings.viewFix")} →
                    </Link>
                  ) : (
                    <button
                      onClick={() => handleGenerateFix(f.ID)}
                      disabled={generatingIds.has(f.ID)}
                      className="rounded bg-blue-600 px-3 py-1 text-sm text-white hover:bg-blue-700 disabled:opacity-50"
                    >
                      {generatingIds.has(f.ID) ? t("runs.findings.generating") : t("runs.findings.generateFix")}
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
        );
      })}
    </div>
  );
}
```

- [ ] **Step 2: Verify the build succeeds**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/app/diagnose/[id]/page.tsx
git commit -m "feat(dashboard): add /diagnose/[id] result page with severity-sorted findings"
```

---

### Task 11: Update About Page Tool List

**Files:**
- Modify: `dashboard/src/i18n/zh.json`
- Modify: `dashboard/src/i18n/en.json`

- [ ] **Step 1: Update tool count and list in i18n**

In `zh.json`, update the about section:

```json
    "tools.desc": "Agent Pod 内嵌 k8s-mcp-server（Go），提供 14 个只读工具：",
    "tools.list": "kubectl_get · kubectl_describe · kubectl_logs · kubectl_explain · events_list · top_pods · top_nodes · list_api_resources · prometheus_query · kubectl_rollout_status · node_status_summary · prometheus_alerts · pvc_status · network_policy_check"
```

Make the same update in `en.json`.

- [ ] **Step 2: Commit**

```bash
git add dashboard/src/i18n/zh.json dashboard/src/i18n/en.json
git commit -m "docs(dashboard): update About page tool list to include 5 new M7 tools"
```

---

### Task 12: Full Integration Verification

**Files:** None (verification only)

- [ ] **Step 1: Run all Go tests**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && go test ./... -v`
Expected: All PASS

- [ ] **Step 2: Run dashboard build**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper/dashboard && npm run build`
Expected: Build succeeds

- [ ] **Step 3: Verify tool count**

Run: `cd /Users/zhenyu.jiang/kube-agent-helper && grep -c 'registerTool' internal/mcptools/register.go`
Expected: 14 (9 existing + 5 new)

- [ ] **Step 4: Verify new route exists**

Run: `grep -r 'diagnose' dashboard/src/app/ --include='*.tsx' -l`
Expected: `dashboard/src/app/diagnose/page.tsx` and `dashboard/src/app/diagnose/[id]/page.tsx`
