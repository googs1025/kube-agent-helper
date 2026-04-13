package k8sclient

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// Mapper wraps a cached DeferredDiscoveryRESTMapper with helpers that turn
// (kind, apiVersion) pairs into concrete GVRs for the dynamic client.
type Mapper struct {
	discovery discovery.DiscoveryInterface
	mapper    meta.RESTMapper
}

func NewMapper(cfg *rest.Config) (*Mapper, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	cached := memory.NewMemCacheClient(dc)
	return &Mapper{
		discovery: dc,
		mapper:    restmapper.NewDeferredDiscoveryRESTMapper(cached),
	}, nil
}

// ResolveGVR turns a user-provided (kind, apiVersion) into a GVR plus the
// namespaced flag. apiVersion may be empty, in which case the mapper picks
// the preferred version.
func (m *Mapper) ResolveGVR(kind, apiVersion string) (schema.GroupVersionResource, bool, error) {
	gk := schema.GroupKind{Kind: kind}
	if apiVersion != "" {
		gv, err := schema.ParseGroupVersion(apiVersion)
		if err != nil {
			return schema.GroupVersionResource{}, false, fmt.Errorf("parse apiVersion: %w", err)
		}
		gk.Group = gv.Group
	}

	versions := []string{}
	if apiVersion != "" {
		gv, _ := schema.ParseGroupVersion(apiVersion)
		versions = []string{gv.Version}
	}

	mapping, err := m.mapper.RESTMapping(gk, versions...)
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("no mapping for %s: %w", kind, err)
	}
	return mapping.Resource, mapping.Scope.Name() == meta.RESTScopeNameNamespace, nil
}

// Discovery returns the underlying discovery client for list_api_resources.
func (m *Mapper) Discovery() discovery.DiscoveryInterface {
	return m.discovery
}
