package translator

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type chainTestProvider struct {
	skills []*store.Skill
}

func (p *chainTestProvider) ListEnabled(_ context.Context) ([]*store.Skill, error) {
	return p.skills, nil
}

func envByName(env []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range env {
		if env[i].Name == name {
			return &env[i]
		}
	}
	return nil
}

func TestBuildJob_InjectsModelChainEnvVars(t *testing.T) {
	primary := &k8saiV1.ModelConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec: k8saiV1.ModelConfigSpec{
			Provider:  "anthropic",
			Model:     "sonnet",
			BaseURL:   "https://primary.example.com",
			Retries:   3,
			APIKeyRef: k8saiV1.SecretKeyRef{Name: "p-secret", Key: "apiKey"},
		},
	}
	fb := &k8saiV1.ModelConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "default"},
		Spec: k8saiV1.ModelConfigSpec{
			Provider:  "anthropic",
			Model:     "haiku",
			BaseURL:   "",
			Retries:   0,
			APIKeyRef: k8saiV1.SecretKeyRef{Name: "f1-secret", Key: "apiKey"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(resolveTestScheme()).WithObjects(primary, fb).Build()

	provider := &chainTestProvider{skills: []*store.Skill{
		{Name: "s", Dimension: "health", Prompt: "x", Enabled: true},
	}}
	tr := NewWithClient(Config{AgentImage: "img:v1"}, provider, c)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "r", Namespace: "default", UID: types.UID("uid-1"),
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:                  k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			ModelConfigRef:          "p",
			FallbackModelConfigRefs: []string{"f1"},
		},
	}

	objs, err := tr.Compile(context.Background(), run)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	var job *batchv1.Job
	for _, o := range objs {
		if j, ok := o.(*batchv1.Job); ok {
			job = j
		}
	}
	if job == nil {
		t.Fatal("no Job in compile output")
	}

	env := job.Spec.Template.Spec.Containers[0].Env

	// Plain string envs
	wantPlain := map[string]string{
		"MODEL_COUNT":      "2",
		"MODEL_0_BASE_URL": "https://primary.example.com",
		"MODEL_0_MODEL":    "sonnet",
		"MODEL_0_RETRIES":  "3",
		"MODEL_1_BASE_URL": "",
		"MODEL_1_MODEL":    "haiku",
		"MODEL_1_RETRIES":  "0",
		// Backward-compat mirror of MODEL_0_*
		"ANTHROPIC_BASE_URL": "https://primary.example.com",
		"MODEL":              "sonnet",
	}
	for k, want := range wantPlain {
		got := envByName(env, k)
		if got == nil {
			t.Errorf("env %s missing", k)
			continue
		}
		if got.Value != want {
			t.Errorf("env %s: want %q, got %q", k, want, got.Value)
		}
	}

	// Secret refs
	checkSecret := func(name, secretName, secretKey string) {
		t.Helper()
		got := envByName(env, name)
		if got == nil || got.ValueFrom == nil || got.ValueFrom.SecretKeyRef == nil {
			t.Errorf("env %s: expected SecretKeyRef, got %+v", name, got)
			return
		}
		if got.ValueFrom.SecretKeyRef.Name != secretName {
			t.Errorf("env %s secret name: want %q got %q", name, secretName, got.ValueFrom.SecretKeyRef.Name)
		}
		if got.ValueFrom.SecretKeyRef.Key != secretKey {
			t.Errorf("env %s secret key: want %q got %q", name, secretKey, got.ValueFrom.SecretKeyRef.Key)
		}
	}
	checkSecret("MODEL_0_API_KEY", "p-secret", "apiKey")
	checkSecret("MODEL_1_API_KEY", "f1-secret", "apiKey")
	checkSecret("ANTHROPIC_API_KEY", "p-secret", "apiKey") // backward-compat
}

func TestBuildJob_LegacyNoClient_StillProducesEnv(t *testing.T) {
	provider := &chainTestProvider{skills: []*store.Skill{
		{Name: "s", Dimension: "health", Prompt: "x", Enabled: true},
	}}
	tr := New(Config{
		AgentImage:       "img:v1",
		AnthropicBaseURL: "https://legacy.example.com",
		Model:            "claude-default-model",
	}, provider)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default", UID: types.UID("uid-2")},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			ModelConfigRef: "claude-secret",
		},
	}

	objs, err := tr.Compile(context.Background(), run)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var job *batchv1.Job
	for _, o := range objs {
		if j, ok := o.(*batchv1.Job); ok {
			job = j
		}
	}
	env := job.Spec.Template.Spec.Containers[0].Env

	if v := envByName(env, "MODEL_COUNT"); v == nil || v.Value != "1" {
		t.Errorf("MODEL_COUNT: want 1, got %+v", v)
	}
	if v := envByName(env, "MODEL_0_BASE_URL"); v == nil || v.Value != "https://legacy.example.com" {
		t.Errorf("MODEL_0_BASE_URL: %+v", v)
	}
	if v := envByName(env, "MODEL_0_MODEL"); v == nil || v.Value != "claude-default-model" {
		t.Errorf("MODEL_0_MODEL: %+v", v)
	}
	if v := envByName(env, "ANTHROPIC_API_KEY"); v == nil || v.ValueFrom == nil ||
		v.ValueFrom.SecretKeyRef == nil ||
		v.ValueFrom.SecretKeyRef.Name != "claude-secret" ||
		v.ValueFrom.SecretKeyRef.Key != "apiKey" {
		t.Errorf("legacy ANTHROPIC_API_KEY: %+v", v)
	}
}
