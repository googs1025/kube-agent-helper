package translator_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

var testSkills = []*store.Skill{
	{
		Name:      "pod-health-analyst",
		Dimension: "health",
		Prompt:    "You are a health analyst.",
		ToolsJSON: `["kubectl_get","events_list"]`,
		Enabled:   true,
	},
	{
		Name:      "pod-security-analyst",
		Dimension: "security",
		Prompt:    "You are a security analyst.",
		ToolsJSON: `["kubectl_get"]`,
		Enabled:   true,
	},
	{
		Name:      "disabled-skill",
		Dimension: "cost",
		Prompt:    "You are disabled.",
		ToolsJSON: `[]`,
		Enabled:   false,
	},
}

func newTranslator(skills []*store.Skill) *translator.Translator {
	return translator.New(translator.Config{
		AgentImage:    "ghcr.io/kube-agent-helper/agent-runtime:latest",
		ControllerURL: "http://controller.svc:8080",
	}, skills)
}

func newRun(name, ns string, uid string, skills []string, namespaces []string) *k8saiV1.DiagnosticRun {
	return &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: types.UID(uid)},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: namespaces},
			Skills:         skills,
			ModelConfigRef: "claude-default",
		},
	}
}

type compiled struct {
	job *batchv1.Job
	cm  *corev1.ConfigMap
	sa  *corev1.ServiceAccount
	rb  *rbacv1.ClusterRoleBinding
}

func extract(t *testing.T, objects []interface{ GetName() string }) compiled {
	t.Helper()
	var c compiled
	for _, o := range objects {
		switch v := o.(type) {
		case *batchv1.Job:
			c.job = v
		case *corev1.ConfigMap:
			c.cm = v
		case *corev1.ServiceAccount:
			c.sa = v
		case *rbacv1.ClusterRoleBinding:
			c.rb = v
		}
	}
	return c
}

func TestTranslator_Compile_ProducesExpectedObjects(t *testing.T) {
	tr := newTranslator(testSkills)
	run := newRun("test-run", "default", "uid-123", []string{"pod-health-analyst"}, []string{"default"})

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)
	require.Len(t, objects, 4, "expected SA, ConfigMap, ClusterRoleBinding, Job")

	var job *batchv1.Job
	var cm *corev1.ConfigMap
	var sa *corev1.ServiceAccount
	var rb *rbacv1.ClusterRoleBinding

	for _, o := range objects {
		switch v := o.(type) {
		case *batchv1.Job:
			job = v
		case *corev1.ConfigMap:
			cm = v
		case *corev1.ServiceAccount:
			sa = v
		case *rbacv1.ClusterRoleBinding:
			rb = v
		}
	}

	require.NotNil(t, job, "expected Job")
	require.NotNil(t, cm, "expected ConfigMap")
	require.NotNil(t, sa, "expected ServiceAccount")
	require.NotNil(t, rb, "expected ClusterRoleBinding")

	assert.Contains(t, cm.Data, "pod-health-analyst.md")
	assert.Equal(t, sa.Name, rb.Subjects[0].Name, "RoleBinding subject must match SA name")
	assert.Equal(t, "uid-123", job.Labels["run-id"])
}

func TestTranslator_Compile_NoEnabledSkills_ReturnsError(t *testing.T) {
	tr := newTranslator(testSkills)
	run := newRun("bad-run", "default", "uid-1", []string{"nonexistent-skill"}, []string{"default"})

	_, err := tr.Compile(context.Background(), run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no enabled skills found")
}

func TestTranslator_Compile_DisabledSkillsFiltered(t *testing.T) {
	tr := newTranslator(testSkills)
	run := newRun("run-disabled", "default", "uid-2", []string{"disabled-skill"}, []string{"default"})

	_, err := tr.Compile(context.Background(), run)
	require.Error(t, err, "disabled skill should not be selected")
}

func TestTranslator_Compile_EmptySkillsSelectsAllEnabled(t *testing.T) {
	tr := newTranslator(testSkills)
	run := newRun("run-all", "default", "uid-3", nil, []string{"default"})

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)

	var cm *corev1.ConfigMap
	for _, o := range objects {
		if v, ok := o.(*corev1.ConfigMap); ok {
			cm = v
		}
	}
	require.NotNil(t, cm)
	assert.Contains(t, cm.Data, "pod-health-analyst.md")
	assert.Contains(t, cm.Data, "pod-security-analyst.md")
	assert.NotContains(t, cm.Data, "disabled-skill.md", "disabled skill should be excluded")
}

func TestTranslator_Compile_MultipleSkills(t *testing.T) {
	tr := newTranslator(testSkills)
	run := newRun("run-multi", "default", "uid-4",
		[]string{"pod-health-analyst", "pod-security-analyst"}, []string{"ns1", "ns2"})

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)

	var job *batchv1.Job
	var cm *corev1.ConfigMap
	for _, o := range objects {
		switch v := o.(type) {
		case *batchv1.Job:
			job = v
		case *corev1.ConfigMap:
			cm = v
		}
	}

	// ConfigMap has both skills
	require.NotNil(t, cm)
	assert.Len(t, cm.Data, 2)
	assert.Contains(t, cm.Data, "pod-health-analyst.md")
	assert.Contains(t, cm.Data, "pod-security-analyst.md")

	// Job env SKILL_NAMES contains both
	require.NotNil(t, job)
	envMap := envToMap(job.Spec.Template.Spec.Containers[0].Env)
	assert.Equal(t, "pod-health-analyst,pod-security-analyst", envMap["SKILL_NAMES"])
	assert.Equal(t, "ns1,ns2", envMap["TARGET_NAMESPACES"])
}

func TestTranslator_Compile_RunIDFallsBackToName(t *testing.T) {
	tr := newTranslator(testSkills)
	run := newRun("name-only", "default", "", []string{"pod-health-analyst"}, []string{"default"})

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)

	for _, o := range objects {
		if sa, ok := o.(*corev1.ServiceAccount); ok {
			assert.Equal(t, "name-only", sa.Labels["run-id"], "run-id should fall back to run.Name when UID is empty")
		}
	}
}

func TestTranslator_Compile_JobSpec(t *testing.T) {
	tr := newTranslator(testSkills)
	run := newRun("job-test", "prod", "uid-5", []string{"pod-health-analyst"}, []string{"prod"})

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)

	var job *batchv1.Job
	for _, o := range objects {
		if v, ok := o.(*batchv1.Job); ok {
			job = v
		}
	}
	require.NotNil(t, job)

	assert.Equal(t, "agent-job-test", job.Name)
	assert.Equal(t, int32(3600), *job.Spec.TTLSecondsAfterFinished)
	assert.Equal(t, int32(0), *job.Spec.BackoffLimit)

	podSpec := job.Spec.Template.Spec
	assert.Equal(t, "run-job-test", podSpec.ServiceAccountName)
	assert.Equal(t, corev1.RestartPolicyNever, podSpec.RestartPolicy)

	// Volume: skills ConfigMap
	require.Len(t, podSpec.Volumes, 1)
	assert.Equal(t, "skill-bundle-job-test", podSpec.Volumes[0].ConfigMap.Name)

	// Container
	require.Len(t, podSpec.Containers, 1)
	c := podSpec.Containers[0]
	assert.Equal(t, "agent", c.Name)
	assert.Equal(t, "ghcr.io/kube-agent-helper/agent-runtime:latest", c.Image)
	assert.Equal(t, []string{"python", "-m", "runtime.main"}, c.Command)

	// Env vars
	envMap := envToMap(c.Env)
	assert.Equal(t, "uid-5", envMap["RUN_ID"])
	assert.Equal(t, "prod", envMap["TARGET_NAMESPACES"])
	assert.Equal(t, "http://controller.svc:8080", envMap["CONTROLLER_URL"])
	assert.Equal(t, "/usr/local/bin/k8s-mcp-server", envMap["MCP_SERVER_PATH"])
	assert.Equal(t, "pod-health-analyst", envMap["SKILL_NAMES"])

	// ANTHROPIC_API_KEY from Secret
	var apiKeyEnv *corev1.EnvVar
	for i := range c.Env {
		if c.Env[i].Name == "ANTHROPIC_API_KEY" {
			apiKeyEnv = &c.Env[i]
		}
	}
	require.NotNil(t, apiKeyEnv)
	require.NotNil(t, apiKeyEnv.ValueFrom)
	require.NotNil(t, apiKeyEnv.ValueFrom.SecretKeyRef)
	assert.Equal(t, "claude-default", apiKeyEnv.ValueFrom.SecretKeyRef.Name)
	assert.Equal(t, "apiKey", apiKeyEnv.ValueFrom.SecretKeyRef.Key)

	// VolumeMounts
	require.Len(t, c.VolumeMounts, 1)
	assert.Equal(t, "/workspace/skills", c.VolumeMounts[0].MountPath)

	// Resources
	assert.Equal(t, resource.MustParse("100m"), c.Resources.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("256Mi"), c.Resources.Requests[corev1.ResourceMemory])
	assert.Equal(t, resource.MustParse("512Mi"), c.Resources.Limits[corev1.ResourceMemory])
}

func TestTranslator_Compile_ConfigMapContent(t *testing.T) {
	tr := newTranslator(testSkills)
	run := newRun("cm-test", "default", "uid-6", []string{"pod-health-analyst"}, []string{"default"})

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)

	var cm *corev1.ConfigMap
	for _, o := range objects {
		if v, ok := o.(*corev1.ConfigMap); ok {
			cm = v
		}
	}
	require.NotNil(t, cm)

	md := cm.Data["pod-health-analyst.md"]
	assert.True(t, strings.HasPrefix(md, "---\n"), "should start with frontmatter")
	assert.Contains(t, md, "name: pod-health-analyst")
	assert.Contains(t, md, "dimension: health")
	assert.Contains(t, md, `tools: ["kubectl_get","events_list"]`)
	assert.Contains(t, md, "You are a health analyst.")
}

func TestTranslator_Compile_RBACBindsToCorrectNamespace(t *testing.T) {
	tr := newTranslator(testSkills)
	run := newRun("rbac-test", "monitoring", "uid-7", []string{"pod-health-analyst"}, []string{"monitoring"})

	objects, err := tr.Compile(context.Background(), run)
	require.NoError(t, err)

	var rb *rbacv1.ClusterRoleBinding
	for _, o := range objects {
		if v, ok := o.(*rbacv1.ClusterRoleBinding); ok {
			rb = v
		}
	}
	require.NotNil(t, rb)

	assert.Equal(t, "view", rb.RoleRef.Name)
	assert.Equal(t, "ClusterRole", rb.RoleRef.Kind)
	require.Len(t, rb.Subjects, 1)
	assert.Equal(t, "monitoring", rb.Subjects[0].Namespace, "SA subject must bind to run's namespace")
	assert.Equal(t, "run-rbac-test", rb.Subjects[0].Name)
}

func envToMap(envs []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envs))
	for _, e := range envs {
		if e.Value != "" {
			m[e.Name] = e.Value
		}
	}
	return m
}
