package envtest

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/stretchr/testify/require"
)

func NewTypedClient(t *testing.T) kubernetes.Interface {
	t.Helper()
	c, err := kubernetes.NewForConfig(SharedConfig)
	require.NoError(t, err)
	return c
}

func NewDynamicClient(t *testing.T) dynamic.Interface {
	t.Helper()
	c, err := dynamic.NewForConfig(SharedConfig)
	require.NoError(t, err)
	return c
}

func CreateNamespace(t *testing.T, client kubernetes.Interface, name string) {
	t.Helper()
	_, err := client.CoreV1().Namespaces().Create(context.Background(),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}},
		metav1.CreateOptions{})
	require.NoError(t, err)
}

var PodGVR = schema.GroupVersionResource{Version: "v1", Resource: "pods"}