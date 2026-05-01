package k8sclient

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// resolveTestKubeconfig returns a *Resolved built from minimalKubeconfig.
func resolveTestKubeconfig(t *testing.T) *Resolved {
	t.Helper()
	dir := t.TempDir()
	kc := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(kc, []byte(minimalKubeconfig), 0o600))
	r, err := Resolve(Flags{Kubeconfig: kc})
	require.NoError(t, err)
	return r
}

func TestBuild_HappyPathNoPrometheus(t *testing.T) {
	r := resolveTestKubeconfig(t)

	c, err := Build(r, "")
	require.NoError(t, err)
	require.NotNil(t, c)

	assert.NotNil(t, c.Typed, "typed client should be constructed")
	assert.NotNil(t, c.Dynamic, "dynamic client should be constructed")
	assert.NotNil(t, c.Mapper, "mapper should be constructed")
	assert.Same(t, r, c.Resolved, "Resolved should be carried through")
	// Metrics may be non-nil even without metrics-server because NewForConfig
	// only fails on a malformed config; we just assert no panic.
	assert.Nil(t, c.Prometheus, "prometheus must be nil when promURL=\"\"")
}

func TestBuild_HappyPathWithPrometheus(t *testing.T) {
	r := resolveTestKubeconfig(t)

	c, err := Build(r, "http://prom.example:9090")
	require.NoError(t, err)
	require.NotNil(t, c)
	assert.NotNil(t, c.Prometheus, "prometheus client should be constructed when promURL set")
}

func TestBuild_BadPrometheusURL(t *testing.T) {
	r := resolveTestKubeconfig(t)

	// Control characters in URLs make promapi.NewClient fail.
	_, err := Build(r, "http://prom.example\x7f:9090")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prometheus client")
}

func TestBuild_BadRestConfigHostFailsTypedClient(t *testing.T) {
	// A rest.Config with a malformed host triggers the typed-client error path.
	r := &Resolved{RESTConfig: &rest.Config{Host: "://broken"}}

	_, err := Build(r, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "typed client")
}
