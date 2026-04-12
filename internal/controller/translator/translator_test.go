package translator_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func TestTranslator_Compile_ProducesExpectedObjects(t *testing.T) {
	skills := []*store.Skill{{
		Name:      "pod-health-analyst",
		Dimension: "health",
		Prompt:    "You are a health analyst.",
		ToolsJSON: `["kubectl_get","events_list"]`,
		Enabled:   true,
	}}
	tr := translator.New(translator.Config{
		AgentImage:    "ghcr.io/kube-agent-helper/agent-runtime:latest",
		ControllerURL: "http://controller.svc:8080",
	}, skills)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "test-run", Namespace: "default", UID: "uid-123"},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health-analyst"},
			ModelConfigRef: "claude-default",
		},
	}

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)

	var job *batchv1.Job
	var cm *corev1.ConfigMap
	var sa *corev1.ServiceAccount
	var rb *rbacv1.RoleBinding

	for _, o := range objects {
		switch v := o.(type) {
		case *batchv1.Job:
			job = v
		case *corev1.ConfigMap:
			cm = v
		case *corev1.ServiceAccount:
			sa = v
		case *rbacv1.RoleBinding:
			rb = v
		}
	}

	require.NotNil(t, job, "expected Job")
	require.NotNil(t, cm, "expected ConfigMap")
	require.NotNil(t, sa, "expected ServiceAccount")
	require.NotNil(t, rb, "expected RoleBinding")

	assert.Contains(t, cm.Data, "pod-health-analyst.md")
	assert.Equal(t, sa.Name, rb.Subjects[0].Name)
	assert.Equal(t, "uid-123", job.Labels["run-id"])
}
