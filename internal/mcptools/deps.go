package mcptools

import (
	"log/slog"
	"regexp"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/kube-agent-helper/kube-agent-helper/internal/k8sclient"
	"github.com/kube-agent-helper/kube-agent-helper/internal/sanitize"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
	"github.com/kube-agent-helper/kube-agent-helper/internal/trimmer"
)

// ResourceMapper is satisfied by k8sclient.Mapper and test fakes.
type ResourceMapper interface {
	ResolveGVR(kind, apiVersion string) (schema.GroupVersionResource, bool, error)
}

// Deps holds tool handler dependencies via interfaces to ease testing.
type Deps struct {
	Dynamic      dynamic.Interface
	Typed        kubernetes.Interface
	Metrics      metricsv.Interface // nil if metrics-server unavailable
	Prometheus   promv1.API         // nil if --prometheus-url not set
	Mapper       ResourceMapper
	Discovery    discovery.DiscoveryInterface
	Logger       *slog.Logger
	SanitizeOpts sanitize.Options
	Projectors   *trimmer.Projectors
	Cluster      string
	Store        store.Store // nil if store not configured
}

// NewDeps constructs a Deps from a k8sclient.Clients produced in main.go.
func NewDeps(c *k8sclient.Clients, logger *slog.Logger, opts sanitize.Options) *Deps {
	return &Deps{
		Dynamic:      c.Dynamic,
		Typed:        c.Typed,
		Metrics:      c.Metrics,
		Prometheus:   c.Prometheus,
		Mapper:       c.Mapper,
		Discovery:    c.Mapper.Discovery(),
		Logger:       logger,
		SanitizeOpts: opts,
		Projectors:   &trimmer.Projectors{},
		Cluster:      c.Resolved.ClusterHost,
	}
}

// DefaultSanitizeOpts builds sanitize.Options from the --mask-configmap-keys
// regex string. Returns an error if the regex fails to compile.
func DefaultSanitizeOpts(maskCMKeys string) (sanitize.Options, error) {
	if maskCMKeys == "" {
		return sanitize.Options{}, nil
	}
	re, err := regexp.Compile(maskCMKeys)
	if err != nil {
		return sanitize.Options{}, err
	}
	return sanitize.Options{ConfigMapKeyMask: re}, nil
}