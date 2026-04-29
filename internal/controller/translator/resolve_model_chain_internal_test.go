package translator

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func resolveTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = rbacv1.AddToScheme(s)
	_ = k8saiV1.AddToScheme(s)
	return s
}

type emptyProvider struct{}

func (emptyProvider) ListEnabled(_ context.Context) ([]*store.Skill, error) {
	return nil, nil
}

func TestResolveModelChain_PrimaryOnly(t *testing.T) {
	primary := &k8saiV1.ModelConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "primary", Namespace: "default"},
		Spec:       k8saiV1.ModelConfigSpec{Provider: "anthropic", Model: "sonnet"},
	}
	c := fake.NewClientBuilder().WithScheme(resolveTestScheme()).WithObjects(primary).Build()
	tr := NewWithClient(Config{}, emptyProvider{}, c)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec:       k8saiV1.DiagnosticRunSpec{ModelConfigRef: "primary"},
	}
	chain := tr.resolveModelChain(context.Background(), run)
	if len(chain) != 1 || chain[0].Name != "primary" {
		t.Fatalf("expected [primary], got %+v", chain)
	}
}

func TestResolveModelChain_PrimaryWithFallbacks(t *testing.T) {
	p := &k8saiV1.ModelConfig{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}}
	f1 := &k8saiV1.ModelConfig{ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "default"}}
	f2 := &k8saiV1.ModelConfig{ObjectMeta: metav1.ObjectMeta{Name: "f2", Namespace: "default"}}
	c := fake.NewClientBuilder().WithScheme(resolveTestScheme()).WithObjects(p, f1, f2).Build()
	tr := NewWithClient(Config{}, emptyProvider{}, c)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec: k8saiV1.DiagnosticRunSpec{
			ModelConfigRef:          "p",
			FallbackModelConfigRefs: []string{"f1", "f2"},
		},
	}
	chain := tr.resolveModelChain(context.Background(), run)
	got := []string{}
	for _, mc := range chain {
		got = append(got, mc.Name)
	}
	want := []string{"p", "f1", "f2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v got %v", want, got)
	}
}

func TestResolveModelChain_MissingFallbackSkipped(t *testing.T) {
	p := &k8saiV1.ModelConfig{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}}
	c := fake.NewClientBuilder().WithScheme(resolveTestScheme()).WithObjects(p).Build()
	tr := NewWithClient(Config{}, emptyProvider{}, c)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec: k8saiV1.DiagnosticRunSpec{
			ModelConfigRef:          "p",
			FallbackModelConfigRefs: []string{"missing"},
		},
	}
	chain := tr.resolveModelChain(context.Background(), run)
	if len(chain) != 1 {
		t.Fatalf("expected primary only when fallback missing, got %d", len(chain))
	}
}

func TestResolveModelChain_NoClient(t *testing.T) {
	tr := New(Config{}, emptyProvider{})
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec:       k8saiV1.DiagnosticRunSpec{ModelConfigRef: "primary"},
	}
	chain := tr.resolveModelChain(context.Background(), run)
	if len(chain) != 0 {
		t.Fatalf("expected empty chain without k8s client, got %d", len(chain))
	}
}
