package k8sclient

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveConfig_FlagConflict(t *testing.T) {
	_, err := Resolve(Flags{InCluster: true, Kubeconfig: "/tmp/kc.yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestResolveConfig_KubeconfigFlagWins(t *testing.T) {
	dir := t.TempDir()
	kc := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(kc, []byte(minimalKubeconfig), 0o600))
	t.Setenv("KUBECONFIG", "/does/not/exist")

	cfg, err := Resolve(Flags{Kubeconfig: kc})
	require.NoError(t, err)
	assert.Equal(t, "https://fake.example:6443", cfg.RESTConfig.Host)
	assert.Equal(t, "test-ctx", cfg.ContextName)
}

func TestResolveConfig_EnvFallback(t *testing.T) {
	dir := t.TempDir()
	kc := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(kc, []byte(minimalKubeconfig), 0o600))
	t.Setenv("KUBECONFIG", kc)

	cfg, err := Resolve(Flags{})
	require.NoError(t, err)
	assert.Equal(t, "https://fake.example:6443", cfg.RESTConfig.Host)
}

func TestResolveConfig_ContextOverride(t *testing.T) {
	dir := t.TempDir()
	kc := filepath.Join(dir, "kubeconfig")
	require.NoError(t, os.WriteFile(kc, []byte(twoContextKubeconfig), 0o600))

	cfg, err := Resolve(Flags{Kubeconfig: kc, Context: "second"})
	require.NoError(t, err)
	assert.Equal(t, "https://other.example:6443", cfg.RESTConfig.Host)
	assert.Equal(t, "second", cfg.ContextName)
}

const minimalKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://fake.example:6443
    insecure-skip-tls-verify: true
contexts:
- name: test-ctx
  context:
    cluster: test-cluster
    user: test-user
current-context: test-ctx
users:
- name: test-user
  user:
    token: fake
`

const twoContextKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: c1
  cluster: {server: https://first.example:6443, insecure-skip-tls-verify: true}
- name: c2
  cluster: {server: https://other.example:6443, insecure-skip-tls-verify: true}
contexts:
- name: first
  context: {cluster: c1, user: u}
- name: second
  context: {cluster: c2, user: u}
current-context: first
users:
- name: u
  user: {token: fake}
`