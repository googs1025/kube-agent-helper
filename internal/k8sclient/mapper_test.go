package k8sclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// fakeDiscoveryWithResources builds a discovery client populated with the
// resource lists needed for ResolveGVR tests.
func fakeDiscoveryWithResources(t *testing.T) *fakediscovery.FakeDiscovery {
	t.Helper()
	cs := fake.NewSimpleClientset()
	disc, ok := cs.Discovery().(*fakediscovery.FakeDiscovery)
	require.True(t, ok, "expected *fakediscovery.FakeDiscovery")

	disc.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod"},
				{Name: "namespaces", SingularName: "namespace", Namespaced: false, Kind: "Namespace"},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", SingularName: "deployment", Namespaced: true, Kind: "Deployment"},
				{Name: "statefulsets", SingularName: "statefulset", Namespaced: true, Kind: "StatefulSet"},
			},
		},
	}
	return disc
}

// newMapperWithDiscovery mirrors NewMapper's internals but skips the
// rest.Config requirement so tests can inject a fake discovery client.
// Same-package access keeps this purely a test concern.
func newMapperWithDiscovery(d discovery.DiscoveryInterface) *Mapper {
	return &Mapper{
		discovery: d,
		mapper:    restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(d)),
	}
}

func TestNewMapper_ConstructsFromConfig(t *testing.T) {
	r := resolveTestKubeconfig(t)
	m, err := NewMapper(r.RESTConfig)
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.NotNil(t, m.Discovery())
}

func TestNewMapper_BadHostReturnsError(t *testing.T) {
	_, err := NewMapper(&rest.Config{Host: "://broken"})
	require.Error(t, err)
}

func TestResolveGVR_NamespacedKindNoApiVersion(t *testing.T) {
	m := newMapperWithDiscovery(fakeDiscoveryWithResources(t))

	gvr, namespaced, err := m.ResolveGVR("Pod", "")
	require.NoError(t, err)
	assert.Equal(t, "pods", gvr.Resource)
	assert.Equal(t, "v1", gvr.Version)
	assert.True(t, namespaced)
}

func TestResolveGVR_ClusterScopedKind(t *testing.T) {
	m := newMapperWithDiscovery(fakeDiscoveryWithResources(t))

	gvr, namespaced, err := m.ResolveGVR("Namespace", "")
	require.NoError(t, err)
	assert.Equal(t, "namespaces", gvr.Resource)
	assert.False(t, namespaced)
}

func TestResolveGVR_WithExplicitApiVersion(t *testing.T) {
	m := newMapperWithDiscovery(fakeDiscoveryWithResources(t))

	gvr, namespaced, err := m.ResolveGVR("Deployment", "apps/v1")
	require.NoError(t, err)
	assert.Equal(t, "apps", gvr.Group)
	assert.Equal(t, "v1", gvr.Version)
	assert.Equal(t, "deployments", gvr.Resource)
	assert.True(t, namespaced)
}

func TestResolveGVR_UnknownKind(t *testing.T) {
	m := newMapperWithDiscovery(fakeDiscoveryWithResources(t))

	_, _, err := m.ResolveGVR("WidgetCRD", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no mapping for WidgetCRD")
}

func TestResolveGVR_MalformedApiVersion(t *testing.T) {
	m := newMapperWithDiscovery(fakeDiscoveryWithResources(t))

	// "a/b/c" trips schema.ParseGroupVersion.
	_, _, err := m.ResolveGVR("Pod", "a/b/c")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse apiVersion")
}

func TestMapper_DiscoveryReturnsUnderlyingClient(t *testing.T) {
	disc := fakeDiscoveryWithResources(t)
	m := newMapperWithDiscovery(disc)

	got := m.Discovery()
	assert.Same(t, disc, got, "Discovery() must return the underlying client")
}
