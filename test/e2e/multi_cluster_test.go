//go:build e2e

package e2e_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/reconciler"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/registry"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// TestReconcilerWithClusterRef verifies that when a DiagnosticRun has
// spec.clusterRef="prod" the reconciler resolves the "prod" client from the
// registry and creates Job/SA/ConfigMap on the target cluster, not the local one.
func TestReconcilerWithClusterRef(t *testing.T) {
	ctx := context.Background()
	sqlStore := newTempSQLiteStore(t)
	seedPodHealthSkill(t, ctx, sqlStore)

	scheme := reconcilerTestScheme()

	// Main (local) fake client — holds the DiagnosticRun CR
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cross-cluster-run", Namespace: "default", UID: "uid-cross-1",
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health-analyst"},
			ModelConfigRef: "claude-default",
			ClusterRef:     "prod",
		},
	}

	localClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()

	// "prod" target fake client — where Job/SA/CM should be created
	prodClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	clusterRegistry := registry.NewClusterClientRegistry()
	clusterRegistry.Set("prod", prodClient)

	r := &reconciler.DiagnosticRunReconciler{
		Client:     localClient,
		Store:      sqlStore,
		Translator: newReconcilerTranslator(sqlStore),
		Registry:   clusterRegistry,
	}

	result := reconcileRunOnce(t, r, run.Name, run.Namespace)
	assert.NotZero(t, result.RequeueAfter, "should requeue to poll Job")

	// Job should be on prodClient, NOT localClient
	var job batchv1.Job
	err := prodClient.Get(ctx, types.NamespacedName{
		Name: "agent-cross-cluster-run", Namespace: "default",
	}, &job)
	require.NoError(t, err, "Job should be created on prod cluster")

	// Verify Job is NOT on localClient
	err = localClient.Get(ctx, types.NamespacedName{
		Name: "agent-cross-cluster-run", Namespace: "default",
	}, &job)
	assert.True(t, k8serrors.IsNotFound(err), "Job should NOT be on local cluster")

	// SQLite run should have ClusterName=prod
	storedRun, err := sqlStore.GetRun(ctx, "uid-cross-1")
	require.NoError(t, err)
	assert.Equal(t, "prod", storedRun.ClusterName)
}

// TestReconcilerWithoutClusterRef_DefaultsLocal verifies that runs without
// clusterRef create objects on the local client and store ClusterName="local".
func TestReconcilerWithoutClusterRef_DefaultsLocal(t *testing.T) {
	ctx := context.Background()
	sqlStore := newTempSQLiteStore(t)
	seedPodHealthSkill(t, ctx, sqlStore)

	run := makeDiagnosticRun("local-run", "uid-local-1")

	fakeClient := fake.NewClientBuilder().
		WithScheme(reconcilerTestScheme()).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()

	clusterRegistry := registry.NewClusterClientRegistry()

	r := &reconciler.DiagnosticRunReconciler{
		Client:     fakeClient,
		Store:      sqlStore,
		Translator: newReconcilerTranslator(sqlStore),
		Registry:   clusterRegistry,
	}

	reconcileRunOnce(t, r, run.Name, run.Namespace)

	// Job should be on the local (main) client
	var job batchv1.Job
	require.NoError(t, fakeClient.Get(ctx, types.NamespacedName{
		Name: "agent-local-run", Namespace: "default",
	}, &job), "Job should be on local cluster")

	// Store should have ClusterName=local
	storedRun, err := sqlStore.GetRun(ctx, "uid-local-1")
	require.NoError(t, err)
	assert.Equal(t, "local", storedRun.ClusterName)
}

// TestReconcilerWithUnknownClusterRef_Fails verifies that a run with an
// unregistered clusterRef transitions the CR to Failed with a clear message.
func TestReconcilerWithUnknownClusterRef_Fails(t *testing.T) {
	ctx := context.Background()
	sqlStore := newTempSQLiteStore(t)
	seedPodHealthSkill(t, ctx, sqlStore)

	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "unknown-cluster-run", Namespace: "default", UID: "uid-unknown-1",
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			Skills:         []string{"pod-health-analyst"},
			ModelConfigRef: "claude-default",
			ClusterRef:     "nonexistent",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(reconcilerTestScheme()).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()

	clusterRegistry := registry.NewClusterClientRegistry()
	// Not registering "nonexistent" — should fail

	r := &reconciler.DiagnosticRunReconciler{
		Client:     fakeClient,
		Store:      sqlStore,
		Translator: newReconcilerTranslator(sqlStore),
		Registry:   clusterRegistry,
	}

	reconcileRunOnce(t, r, run.Name, run.Namespace)

	// CR should be Failed with a message about the missing cluster
	var updated k8saiV1.DiagnosticRun
	require.NoError(t, fakeClient.Get(ctx,
		types.NamespacedName{Name: run.Name, Namespace: run.Namespace}, &updated))
	assert.Equal(t, "Failed", updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "nonexistent")
}

// TestAPIClustersAndMultiClusterFilter is an E2E test for the HTTP API cluster
// filtering across the full lifecycle.
func TestAPIClustersAndMultiClusterFilter(t *testing.T) {
	realStore := newSQLiteStore(t)
	ctx := t.Context()

	// Seed runs in different clusters
	require.NoError(t, realStore.CreateRun(ctx, &store.DiagnosticRun{
		ID: "run-local-1", ClusterName: "local",
		TargetJSON: `{"scope":"namespace"}`, SkillsJSON: `[]`, Status: store.PhaseSucceeded,
	}))
	require.NoError(t, realStore.CreateRun(ctx, &store.DiagnosticRun{
		ID: "run-prod-1", ClusterName: "prod",
		TargetJSON: `{"scope":"namespace"}`, SkillsJSON: `[]`, Status: store.PhaseSucceeded,
	}))
	require.NoError(t, realStore.CreateRun(ctx, &store.DiagnosticRun{
		ID: "run-prod-2", ClusterName: "prod",
		TargetJSON: `{"scope":"cluster"}`, SkillsJSON: `[]`, Status: store.PhasePending,
	}))

	// Seed a ClusterConfig CR in the fake K8s client
	cc := &k8saiV1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "kube-agent-helper"},
		Spec: k8saiV1.ClusterConfigSpec{
			KubeConfigRef: k8saiV1.SecretKeyRef{Name: "prod-kubeconfig", Key: "kubeconfig"},
			PrometheusURL: "http://prom.prod:9090",
			Description:   "Production",
		},
		Status: k8saiV1.ClusterConfigStatus{Phase: "Connected"},
	}
	fakeK8s := fake.NewClientBuilder().
		WithScheme(newAPITestScheme()).
		WithObjects(cc).
		WithStatusSubresource(cc).
		Build()

	srv := httpserver.New(realStore, fakeK8s, nil)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	t.Run("GET /api/clusters includes local and prod", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/clusters", nil)
		require.Equal(t, http.StatusOK, code)
		require.GreaterOrEqual(t, len(items), 2, "should have local + prod")

		names := make([]string, 0)
		for _, item := range items {
			if c, ok := item.(map[string]interface{}); ok {
				names = append(names, c["name"].(string))
			}
		}
		assert.Contains(t, names, "local")
		assert.Contains(t, names, "prod")
	})

	t.Run("GET /api/runs?cluster=prod returns only prod runs", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/runs?cluster=prod", nil)
		require.Equal(t, http.StatusOK, code)
		require.Len(t, items, 2)
	})

	t.Run("GET /api/runs?cluster=local returns only local runs", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/runs?cluster=local", nil)
		require.Equal(t, http.StatusOK, code)
		require.Len(t, items, 1)
	})

	t.Run("GET /api/runs without cluster returns all", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/runs", nil)
		require.Equal(t, http.StatusOK, code)
		require.Len(t, items, 3)
	})
}
