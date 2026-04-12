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
