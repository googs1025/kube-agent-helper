package envtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var SharedConfig *rest.Config

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// Prefer the path set up by setup-envtest into bin/envtest/k8s/<version>.
		root, err := filepath.Abs("../../bin/envtest/k8s")
		if err == nil {
			entries, _ := os.ReadDir(root)
			if len(entries) > 0 {
				os.Setenv("KUBEBUILDER_ASSETS", filepath.Join(root, entries[0].Name()))
			}
		}
	}

	env := &envtest.Environment{}
	cfg, err := env.Start()
	if err != nil {
		ctrl.Log.Error(err, "envtest start failed")
		os.Exit(1)
	}
	SharedConfig = cfg

	code := m.Run()

	_ = env.Stop()
	os.Exit(code)
}

func TestEnvtest_Boots(t *testing.T) {
	client := NewTypedClient(t)
	ns, err := client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	// default namespace exists out of the box
	assert.NotEmpty(t, ns.Items)
}