package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
)

// scheduledTestScheme is local because the public testScheme() lives in the
// external test package; this file is in `package reconciler` so it can
// access the unexported helpers (buildChildRun, enforceHistoryLimit,
// appendUnique, removeFromSlice).
func scheduledTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, k8saiV1.AddToScheme(s))
	return s
}

// ── Reconcile branches ───────────────────────────────────────────────────────

func TestScheduledRun_Reconcile_NotFound_NoOp(t *testing.T) {
	r := &ScheduledRunReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheduledTestScheme(t)).Build(),
	}
	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res)
}

func TestScheduledRun_Reconcile_NoSchedule_NoOp(t *testing.T) {
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default"},
		Spec:       k8saiV1.DiagnosticRunSpec{},
	}
	r := &ScheduledRunReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheduledTestScheme(t)).
			WithObjects(run).
			Build(),
	}
	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "r1", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, res, "no Schedule should be a no-op")
}

func TestScheduledRun_Reconcile_InvalidCronIsHandled(t *testing.T) {
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default"},
		Spec:       k8saiV1.DiagnosticRunSpec{Schedule: "this is not cron"},
	}
	r := &ScheduledRunReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheduledTestScheme(t)).
			WithObjects(run).
			Build(),
	}
	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "r1", Namespace: "default"},
	})
	require.NoError(t, err, "invalid cron must not bubble up as an error")
	assert.Equal(t, ctrl.Result{}, res)
}

func TestScheduledRun_Reconcile_InitializesNextRunAt(t *testing.T) {
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default"},
		Spec:       k8saiV1.DiagnosticRunSpec{Schedule: "*/5 * * * *"},
	}
	cli := fake.NewClientBuilder().
		WithScheme(scheduledTestScheme(t)).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()
	r := &ScheduledRunReconciler{Client: cli}

	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "r1", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.True(t, res.RequeueAfter > 0, "first reconcile should request requeue at NextRunAt")

	var got k8saiV1.DiagnosticRun
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "r1", Namespace: "default"}, &got))
	require.NotNil(t, got.Status.NextRunAt, "NextRunAt must be populated on first reconcile")
}

func TestScheduledRun_Reconcile_NotYetTime_RequeuesWithoutCreatingChild(t *testing.T) {
	future := metav1.NewTime(time.Now().Add(1 * time.Hour))
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default"},
		Spec:       k8saiV1.DiagnosticRunSpec{Schedule: "*/5 * * * *"},
		Status:     k8saiV1.DiagnosticRunStatus{NextRunAt: &future},
	}
	cli := fake.NewClientBuilder().
		WithScheme(scheduledTestScheme(t)).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()
	r := &ScheduledRunReconciler{Client: cli}

	res, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "r1", Namespace: "default"},
	})
	require.NoError(t, err)
	assert.Greater(t, res.RequeueAfter, time.Duration(0))

	var children k8saiV1.DiagnosticRunList
	require.NoError(t, cli.List(context.Background(), &children))
	assert.Len(t, children.Items, 1, "no child should be created when not yet time")
}

func TestScheduledRun_Reconcile_TriggerCreatesChildAndAdvances(t *testing.T) {
	past := metav1.NewTime(time.Now().Add(-1 * time.Minute))
	run := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tmpl", Namespace: "default", UID: types.UID("tmpl-uid"),
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Schedule: "*/1 * * * *",
			Skills:   []string{"pod-health"},
		},
		Status: k8saiV1.DiagnosticRunStatus{NextRunAt: &past},
	}
	cli := fake.NewClientBuilder().
		WithScheme(scheduledTestScheme(t)).
		WithObjects(run).
		WithStatusSubresource(run).
		Build()
	r := &ScheduledRunReconciler{Client: cli}

	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "tmpl", Namespace: "default"},
	})
	require.NoError(t, err)
	// RequeueAfter may be zero or even negative when the next scheduled minute
	// has already passed by the time the reactor finishes; we only care that
	// a child was created and ActiveRuns was updated below.

	var children k8saiV1.DiagnosticRunList
	require.NoError(t, cli.List(context.Background(), &children))
	require.Len(t, children.Items, 2, "expect template plus one new child")

	// Find the child (not the template).
	var child *k8saiV1.DiagnosticRun
	for i := range children.Items {
		if children.Items[i].Name != "tmpl" {
			child = &children.Items[i]
			break
		}
	}
	require.NotNil(t, child)
	assert.Equal(t, "tmpl", child.Labels[scheduledByLabel])
	require.Len(t, child.OwnerReferences, 1)
	assert.Equal(t, types.UID("tmpl-uid"), child.OwnerReferences[0].UID)
	assert.Equal(t, []string{"pod-health"}, child.Spec.Skills)

	// Parent ActiveRuns should now contain the child name.
	var got k8saiV1.DiagnosticRun
	require.NoError(t, cli.Get(context.Background(), types.NamespacedName{Name: "tmpl", Namespace: "default"}, &got))
	assert.Contains(t, got.Status.ActiveRuns, child.Name)
}

// ── buildChildRun ─────────────────────────────────────────────────────────────

func TestBuildChildRun_CopiesSpecAndSetsLabelsAndOwner(t *testing.T) {
	r := &ScheduledRunReconciler{}
	timeout := int32(600)
	parent := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "p", Namespace: "ns", UID: types.UID("p-uid"),
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:         k8saiV1.TargetSpec{Scope: "namespace", Namespaces: []string{"ns"}},
			Skills:         []string{"a", "b"},
			ModelConfigRef: "anthropic-creds",
			TimeoutSeconds: &timeout,
			OutputLanguage: "zh",
		},
	}
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	child := r.buildChildRun(parent, "p-1234", now)

	assert.Equal(t, "p-1234", child.Name)
	assert.Equal(t, "ns", child.Namespace)
	assert.Equal(t, "p", child.Labels[scheduledByLabel])
	require.Len(t, child.OwnerReferences, 1)
	assert.Equal(t, types.UID("p-uid"), child.OwnerReferences[0].UID)
	assert.Equal(t, "DiagnosticRun", child.OwnerReferences[0].Kind)
	require.NotNil(t, child.OwnerReferences[0].Controller)
	assert.True(t, *child.OwnerReferences[0].Controller)

	assert.Equal(t, parent.Spec.Target, child.Spec.Target)
	assert.Equal(t, parent.Spec.Skills, child.Spec.Skills)
	assert.Equal(t, "anthropic-creds", child.Spec.ModelConfigRef)
	require.NotNil(t, child.Spec.TimeoutSeconds)
	assert.Equal(t, int32(600), *child.Spec.TimeoutSeconds)
	assert.Equal(t, "zh", child.Spec.OutputLanguage)

	assert.Empty(t, child.Spec.Schedule, "child must NOT inherit the schedule")

	assert.Equal(t, now.Format(time.RFC3339),
		child.Annotations["kube-agent-helper.io/triggered-at"])
}

// ── enforceHistoryLimit ───────────────────────────────────────────────────────

func TestEnforceHistoryLimit_DeletesOldestOverLimit(t *testing.T) {
	parent := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
	}
	limit := int32(2)
	parent.Spec.HistoryLimit = &limit
	parent.Status.ActiveRuns = []string{"c1", "c2", "c3"}

	c1 := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c1", Namespace: "default",
			Labels:            map[string]string{scheduledByLabel: "p"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-3 * time.Hour)),
		},
	}
	c2 := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c2", Namespace: "default",
			Labels:            map[string]string{scheduledByLabel: "p"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
		},
	}
	c3 := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c3", Namespace: "default",
			Labels:            map[string]string{scheduledByLabel: "p"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
		},
	}
	cli := fake.NewClientBuilder().
		WithScheme(scheduledTestScheme(t)).
		WithObjects(parent, c1, c2, c3).
		Build()
	r := &ScheduledRunReconciler{Client: cli}

	err := r.enforceHistoryLimit(context.Background(), parent)
	require.NoError(t, err)

	// c1 is oldest and should be deleted; c2 and c3 remain.
	var list k8saiV1.DiagnosticRunList
	require.NoError(t, cli.List(context.Background(), &list))
	names := map[string]bool{}
	for _, r := range list.Items {
		names[r.Name] = true
	}
	assert.False(t, names["c1"], "oldest child should have been deleted")
	assert.True(t, names["c2"])
	assert.True(t, names["c3"])
	assert.NotContains(t, parent.Status.ActiveRuns, "c1")
}

func TestEnforceHistoryLimit_UnderLimit_NoOp(t *testing.T) {
	parent := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
	}
	limit := int32(5)
	parent.Spec.HistoryLimit = &limit
	parent.Status.ActiveRuns = []string{"c1", "c2"}

	cli := fake.NewClientBuilder().WithScheme(scheduledTestScheme(t)).WithObjects(parent).Build()
	r := &ScheduledRunReconciler{Client: cli}
	require.NoError(t, r.enforceHistoryLimit(context.Background(), parent))
	assert.Equal(t, []string{"c1", "c2"}, parent.Status.ActiveRuns, "under-limit must be no-op")
}

func TestEnforceHistoryLimit_DefaultLimit(t *testing.T) {
	parent := &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
	}
	// HistoryLimit nil → defaults to defaultHistoryLimit (10)
	parent.Status.ActiveRuns = []string{"c1"}

	cli := fake.NewClientBuilder().WithScheme(scheduledTestScheme(t)).WithObjects(parent).Build()
	r := &ScheduledRunReconciler{Client: cli}
	require.NoError(t, r.enforceHistoryLimit(context.Background(), parent))
	assert.Equal(t, []string{"c1"}, parent.Status.ActiveRuns)
}

// ── appendUnique / removeFromSlice ────────────────────────────────────────────

func TestAppendUnique(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		add  string
		want []string
	}{
		{"empty", nil, "a", []string{"a"}},
		{"new-element", []string{"a", "b"}, "c", []string{"a", "b", "c"}},
		{"duplicate", []string{"a", "b"}, "a", []string{"a", "b"}},
		{"duplicate-middle", []string{"a", "b", "c"}, "b", []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := appendUnique(tc.in, tc.add)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRemoveFromSlice(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		rm   string
		want []string
	}{
		{"empty", []string{}, "a", []string{}},
		{"present", []string{"a", "b", "c"}, "b", []string{"a", "c"}},
		{"absent", []string{"a", "b"}, "z", []string{"a", "b"}},
		{"all-duplicates", []string{"a", "a", "a"}, "a", []string{}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := removeFromSlice(tc.in, tc.rm)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ── kindToGVK ────────────────────────────────────────────────────────────────

func TestKindToGVK(t *testing.T) {
	cases := []struct {
		kind  string
		group string
		v     string
	}{
		{"Deployment", "apps", "v1"},
		{"StatefulSet", "apps", "v1"},
		{"DaemonSet", "apps", "v1"},
		{"Pod", "", "v1"},
		{"Service", "", "v1"},
		{"ConfigMap", "", "v1"},
		{"Secret", "", "v1"},
		{"ServiceAccount", "", "v1"},
		{"Namespace", "", "v1"},
		{"PersistentVolumeClaim", "", "v1"},
		{"ResourceQuota", "", "v1"},
		{"LimitRange", "", "v1"},
		{"Job", "batch", "v1"},
		{"CronJob", "batch", "v1"},
		{"Ingress", "networking.k8s.io", "v1"},
		{"NetworkPolicy", "networking.k8s.io", "v1"},
		{"PodDisruptionBudget", "policy", "v1"},
		{"HorizontalPodAutoscaler", "autoscaling", "v2"},
		{"ClusterRole", "rbac.authorization.k8s.io", "v1"},
		{"ClusterRoleBinding", "rbac.authorization.k8s.io", "v1"},
		{"Role", "rbac.authorization.k8s.io", "v1"},
		{"RoleBinding", "rbac.authorization.k8s.io", "v1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.kind, func(t *testing.T) {
			gvk := kindToGVK(tc.kind)
			assert.Equal(t, tc.group, gvk.Group)
			assert.Equal(t, tc.v, gvk.Version)
			assert.Equal(t, tc.kind, gvk.Kind)
		})
	}
}

func TestKindToGVK_UnknownReturnsEmpty(t *testing.T) {
	gvk := kindToGVK("WidgetCRD")
	assert.Empty(t, gvk.Kind)
	assert.Empty(t, gvk.Group)
}

// ── childRunName ─────────────────────────────────────────────────────────────

func TestChildRunName_TruncatesLongNames(t *testing.T) {
	long := ""
	for i := 0; i < 300; i++ {
		long += "x"
	}
	got := childRunName(long, time.Unix(1234567890, 0))
	assert.LessOrEqual(t, len(got), 253, "name must be truncated to 253 bytes max")
	assert.NotEmpty(t, got)
}

func TestChildRunName_ShortNamesUnaffected(t *testing.T) {
	got := childRunName("parent", time.Unix(1234567890, 0))
	assert.Equal(t, "parent-1234567890", got)
}
