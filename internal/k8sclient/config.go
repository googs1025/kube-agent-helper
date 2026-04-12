package k8sclient

import (
	"errors"
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Flags holds the CLI flag values relevant to cluster connection.
type Flags struct {
	InCluster  bool
	Kubeconfig string
	Context    string
}

// Resolved holds the produced rest.Config together with descriptive metadata
// used for startup logging.
type Resolved struct {
	RESTConfig  *rest.Config
	Source      string // "in-cluster" | "kubeconfig"
	ContextName string // empty in in-cluster mode
	ClusterHost string
}

// Resolve produces a *rest.Config from the provided flags, honoring the
// precedence rules documented in the spec §1.2.
func Resolve(f Flags) (*Resolved, error) {
	if f.InCluster && f.Kubeconfig != "" {
		return nil, errors.New("--in-cluster and --kubeconfig are mutually exclusive")
	}

	if f.InCluster {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}
		return &Resolved{
			RESTConfig:  cfg,
			Source:      "in-cluster",
			ClusterHost: cfg.Host,
		}, nil
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if f.Kubeconfig != "" {
		loadingRules.ExplicitPath = f.Kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if f.Context != "" {
		overrides.CurrentContext = f.Context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	raw, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	ctxName := f.Context
	if ctxName == "" {
		ctxName = raw.CurrentContext
	}

	cfg, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	return &Resolved{
		RESTConfig:  cfg,
		Source:      "kubeconfig",
		ContextName: ctxName,
		ClusterHost: cfg.Host,
	}, nil
}