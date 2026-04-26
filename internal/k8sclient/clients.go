// Package k8sclient 集中构造给 MCP 工具用的所有客户端。
//
// 一个 Clients 结构封装：
//   - kubernetes.Interface (typed)  ─ 标准 Pod/Deployment 等
//   - dynamic.Interface              ─ 任意 GVR (CRD 也能用)
//   - metricsv.Interface             ─ metrics.k8s.io（top_pods/top_nodes 用）
//   - prometheus.API                 ─ 历史指标（PromQL 查询）
//   - Mapper（mapper.go）             ─ Kind ↔ GVR 解析
//
// 工厂在 cmd/kah 和 cmd/...mcp-server 启动时调用一次，
// 注入到所有 ToolHandler。Metrics / Prometheus 为 nil 安全 — 工具内做能力降级。
package k8sclient

import (
	"fmt"
	"net/http"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Clients bundles every Kubernetes-adjacent client used by tool handlers.
// Metrics and Prometheus clients are optional; callers must check for nil.
type Clients struct {
	Typed      kubernetes.Interface
	Dynamic    dynamic.Interface
	Metrics    metricsv.Interface // nil if metrics-server unavailable at construction
	Prometheus promv1.API         // nil if --prometheus-url not set
	Mapper     *Mapper
	Resolved   *Resolved
}

// Build constructs every client from a resolved rest.Config.
// metricsEnabled=true always tries to build the metrics client; failure is
// swallowed and logged later as "metrics-server unavailable".
// promURL=="" disables the Prometheus client entirely.
func Build(r *Resolved, promURL string) (*Clients, error) {
	typed, err := kubernetes.NewForConfig(r.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("typed client: %w", err)
	}

	dyn, err := dynamic.NewForConfig(r.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("dynamic client: %w", err)
	}

	mapper, err := NewMapper(r.RESTConfig)
	if err != nil {
		return nil, fmt.Errorf("rest mapper: %w", err)
	}

	var metrics metricsv.Interface
	if mc, err := metricsv.NewForConfig(r.RESTConfig); err == nil {
		metrics = mc
	}

	var prom promv1.API
	if promURL != "" {
		pc, err := promapi.NewClient(promapi.Config{
			Address: promURL,
			Client:  &http.Client{Timeout: 30 * time.Second},
		})
		if err != nil {
			return nil, fmt.Errorf("prometheus client: %w", err)
		}
		prom = promv1.NewAPI(pc)
	}

	return &Clients{
		Typed:      typed,
		Dynamic:    dyn,
		Metrics:    metrics,
		Prometheus: prom,
		Mapper:     mapper,
		Resolved:   r,
	}, nil
}
