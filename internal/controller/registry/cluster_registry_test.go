package registry_test

import (
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
)

func TestClusterClientRegistry_SetGetDelete(t *testing.T) {
	reg := registry.NewClusterClientRegistry()
	fakeClient := fake.NewClientBuilder().Build()

	reg.Set("prod", fakeClient)

	got, ok := reg.Get("prod")
	if !ok {
		t.Fatal("expected to find 'prod'")
	}
	if got != fakeClient {
		t.Fatal("expected same client")
	}

	_, ok = reg.Get("staging")
	if ok {
		t.Fatal("expected 'staging' to be missing")
	}

	reg.Delete("prod")
	_, ok = reg.Get("prod")
	if ok {
		t.Fatal("expected 'prod' deleted")
	}
}
