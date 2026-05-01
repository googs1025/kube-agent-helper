package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/notification"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// ── Test doubles ─────────────────────────────────────────────────────────────

// recordingNotifMgr records calls to ReloadFromConfigs / SendTest and is
// satisfied by httpserver.NotificationManager.
type recordingNotifMgr struct {
	mu          sync.Mutex
	reloadCalls int
	lastConfigs []*notification.NotificationConfig
	testCalls   atomic.Int32
	testErr     error
	notifyErr   error
	notifyCalls atomic.Int32
}

func (m *recordingNotifMgr) Notify(_ context.Context, _ notification.Event) error {
	m.notifyCalls.Add(1)
	return m.notifyErr
}

func (m *recordingNotifMgr) ReloadFromConfigs(cfgs []*notification.NotificationConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCalls++
	m.lastConfigs = cfgs
}

func (m *recordingNotifMgr) SendTest(_ context.Context, _ *notification.NotificationConfig) error {
	m.testCalls.Add(1)
	return m.testErr
}

// fakeK8sClient builds a controller-runtime fake client with the schemes used
// in this file, optionally seeded with objects.
func fakeK8sClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

// fakeClientset returns an empty client-go fake clientset for use as
// kubernetes.Interface (e.g. for log streaming endpoints not exercised here).
func fakeClientset() kubernetes.Interface { return clientsetfake.NewSimpleClientset() }

// ── ModelConfigs ─────────────────────────────────────────────────────────────

func TestModelConfigs_GET_NoK8sClient_503(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/modelconfigs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestModelConfigs_GET_ListReturnsMaskedAPIKey(t *testing.T) {
	mc := &v1alpha1.ModelConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "anthropic-creds", Namespace: "default"},
		Spec: v1alpha1.ModelConfigSpec{
			Provider: "anthropic", Model: "claude-sonnet-4-6",
			APIKeyRef: v1alpha1.SecretKeyRef{Name: "anthropic-creds", Key: "apiKey"},
		},
	}
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t, mc), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/modelconfigs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.Equal(t, "anthropic-creds", got[0]["name"])
	assert.Equal(t, "****", got[0]["apiKey"], "API key must always be masked")
	assert.Equal(t, "anthropic", got[0]["provider"])
}

func TestModelConfigs_POST_CreatesCRWithDefaults(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "ollama-local",
		"namespace": "default",
		// provider/model omitted → defaults must apply
	})
	req := httptest.NewRequest(http.MethodPost, "/api/modelconfigs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestModelConfigs_POST_BadJSON_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/modelconfigs", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestModelConfigs_POST_MissingNamespace_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)

	body, _ := json.Marshal(map[string]interface{}{"name": "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/modelconfigs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestModelConfigs_POST_AlreadyExists_409(t *testing.T) {
	existing := &v1alpha1.ModelConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "dup", Namespace: "default"},
		Spec:       v1alpha1.ModelConfigSpec{Provider: "anthropic", Model: "x"},
	}
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t, existing), nil)

	body, _ := json.Marshal(map[string]interface{}{"name": "dup", "namespace": "default"})
	req := httptest.NewRequest(http.MethodPost, "/api/modelconfigs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestModelConfigs_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPut, "/api/modelconfigs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── NotificationConfigs ──────────────────────────────────────────────────────

func TestNotificationConfigs_GET_EmptyReturnsArray(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/notification-configs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	body := bytes.TrimSpace(w.Body.Bytes())
	assert.Equal(t, []byte("[]"), body, "empty list should serialise as []")
}

func TestNotificationConfigs_POST_CreatesAndReloadsManager(t *testing.T) {
	mgr := &recordingNotifMgr{}
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil, httpserver.WithNotificationManager(mgr))

	body, _ := json.Marshal(map[string]interface{}{
		"name": "ops-slack", "type": "slack",
		"webhookURL": "https://example.invalid/hook",
		"events":     "fix.applied", "enabled": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/notification-configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	mgr.mu.Lock()
	assert.Equal(t, 1, mgr.reloadCalls, "reloadNotificationChannels must fire after Create")
	mgr.mu.Unlock()
}

func TestNotificationConfigs_POST_InvalidType_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "x", "type": "discord", "webhookURL": "http://x",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/notification-configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNotificationConfigs_POST_MissingFields_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	body, _ := json.Marshal(map[string]interface{}{"name": "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/notification-configs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNotificationConfigs_POST_BadJSON_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/notification-configs", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNotificationConfigs_PUT_UpdatesAndReloads(t *testing.T) {
	mgr := &recordingNotifMgr{}
	fs := &fakeStore{
		notifConfigs: []*store.NotificationConfig{
			{ID: "n1", Name: "old", Type: "slack", WebhookURL: "https://x"},
		},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil, httpserver.WithNotificationManager(mgr))

	body, _ := json.Marshal(map[string]interface{}{
		"name": "new-name", "type": "slack",
		"webhookURL": "https://example.invalid/v2",
		"enabled":    true,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/notification-configs/n1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	mgr.mu.Lock()
	assert.Equal(t, 1, mgr.reloadCalls, "reload must fire after PUT")
	mgr.mu.Unlock()
}

func TestNotificationConfigs_PUT_NotFound_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	body, _ := json.Marshal(map[string]interface{}{"name": "x", "type": "slack", "webhookURL": "http://y"})
	req := httptest.NewRequest(http.MethodPut, "/api/notification-configs/ghost", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestNotificationConfigs_PUT_BadJSON_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPut, "/api/notification-configs/n1", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNotificationConfigs_PUT_MissingFields_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	body, _ := json.Marshal(map[string]interface{}{"name": "only-name"})
	req := httptest.NewRequest(http.MethodPut, "/api/notification-configs/n1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestNotificationConfigs_DELETE_OK(t *testing.T) {
	mgr := &recordingNotifMgr{}
	fs := &fakeStore{
		notifConfigs: []*store.NotificationConfig{
			{ID: "n1", Name: "x", Type: "slack", WebhookURL: "https://y"},
		},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil, httpserver.WithNotificationManager(mgr))

	req := httptest.NewRequest(http.MethodDelete, "/api/notification-configs/n1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	mgr.mu.Lock()
	assert.Equal(t, 1, mgr.reloadCalls, "reload must fire after DELETE")
	mgr.mu.Unlock()
}

func TestNotificationConfigs_DELETE_NotFound(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/notification-configs/ghost", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestNotificationConfigs_TEST_NoManagerConfigured_500(t *testing.T) {
	fs := &fakeStore{
		notifConfigs: []*store.NotificationConfig{
			{ID: "n1", Name: "x", Type: "slack", WebhookURL: "https://y"},
		},
	}
	// No WithNotificationManager option.
	srv := httpserver.New(fs, fakeK8sClient(t), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/notification-configs/n1/test", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestNotificationConfigs_TEST_HappyPath(t *testing.T) {
	mgr := &recordingNotifMgr{}
	fs := &fakeStore{
		notifConfigs: []*store.NotificationConfig{
			{ID: "n1", Name: "x", Type: "slack", WebhookURL: "https://y"},
		},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil, httpserver.WithNotificationManager(mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/notification-configs/n1/test", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int32(1), mgr.testCalls.Load())
}

func TestNotificationConfigs_TEST_BackendFailure_502(t *testing.T) {
	mgr := &recordingNotifMgr{testErr: errors.New("downstream boom")}
	fs := &fakeStore{
		notifConfigs: []*store.NotificationConfig{
			{ID: "n1", Name: "x", Type: "slack", WebhookURL: "https://y"},
		},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil, httpserver.WithNotificationManager(mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/notification-configs/n1/test", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestNotificationConfigs_TEST_NotFound_404(t *testing.T) {
	mgr := &recordingNotifMgr{}
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil, httpserver.WithNotificationManager(mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/notification-configs/ghost/test", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestNotificationConfigs_PathTooShort_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/notification-configs/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestNotificationConfigs_DetailMethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPatch, "/api/notification-configs/n1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── handleAPIK8sResources ────────────────────────────────────────────────────

func TestK8sResources_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/k8s/resources", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestK8sResources_MissingKind_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/k8s/resources", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestK8sResources_UnsupportedKind_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/k8s/resources?kind=Widget", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestK8sResources_PodMissingNamespace_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/k8s/resources?kind=Pod", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestK8sResources_PodNotFound_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/k8s/resources?kind=Pod&namespace=default&name=missing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestK8sResources_PodGetByName_OK(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default", UID: types.UID("u1")},
	}
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t, pod), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/k8s/resources?kind=Pod&namespace=default&name=p1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var got map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	meta := got["metadata"].(map[string]interface{})
	assert.Equal(t, "p1", meta["name"])
}

func TestK8sResources_PodListInNamespace_OK(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}}
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t, pod), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/k8s/resources?kind=Pod&namespace=default", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var got []map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.Equal(t, "p1", got[0]["name"])
}

func TestK8sResources_NamespaceList_FiltersSystemNS(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-public"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-node-lease"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "user-app"}},
	), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/k8s/resources?kind=Namespace", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got []map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	names := map[string]bool{}
	for _, ns := range got {
		names[ns["name"]] = true
	}
	assert.True(t, names["default"])
	assert.True(t, names["user-app"])
	assert.False(t, names["kube-system"], "kube-system must be filtered out")
	assert.False(t, names["kube-public"], "kube-public must be filtered out")
	assert.False(t, names["kube-node-lease"], "kube-node-lease must be filtered out")
}

// ── handleAPIFixesBatchReject ────────────────────────────────────────────────

func TestBatchReject_BadJSON_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/fixes/batch-reject", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBatchReject_EmptyIDs_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	body, _ := json.Marshal(map[string]interface{}{"ids": []string{}})
	req := httptest.NewRequest(http.MethodPost, "/api/fixes/batch-reject", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBatchReject_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/fixes/batch-reject", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── handleInternal (path validation) ─────────────────────────────────────────

func TestInternal_BadPath_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/internal/runs/abc/wrong-tail", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestInternal_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/internal/runs/abc/findings", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestInternal_BadJSON_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/internal/runs/run-1/findings", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── WithNotifier / WithNotificationManager / WithClientset (functional opts) ─

func TestServerOptions_AcceptedAndApplied(t *testing.T) {
	mgr := &recordingNotifMgr{}
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil,
		httpserver.WithNotifier(mgr),
		httpserver.WithNotificationManager(mgr),
		httpserver.WithClientset(fakeClientset()),
	)
	require.NotNil(t, srv)

	// Smoke: a known endpoint still responds (sanity that mux is wired).
	req := httptest.NewRequest(http.MethodGet, "/api/notification-configs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ── handleAPIFixDetail (approve/reject paths) ────────────────────────────────

func TestFixDetail_ApprovePatch_NotifiesAndUpdatesPhase(t *testing.T) {
	mgr := &recordingNotifMgr{}
	fs := &fakeStore{fixes: []*store.Fix{{ID: "f-approve", Phase: store.FixPhasePendingApproval}}}
	srv := httpserver.New(fs, fakeK8sClient(t), nil, httpserver.WithNotifier(mgr))

	body, _ := json.Marshal(map[string]string{"approvedBy": "alice"})
	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/f-approve/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.GreaterOrEqual(t, mgr.notifyCalls.Load(), int32(1), "notifier must be called on approve")
}

func TestFixDetail_ApprovePatch_BadJSON_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/f1/approve", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFixDetail_RejectPatch_OK(t *testing.T) {
	mgr := &recordingNotifMgr{}
	fs := &fakeStore{fixes: []*store.Fix{{ID: "f-reject", Phase: store.FixPhasePendingApproval}}}
	srv := httpserver.New(fs, fakeK8sClient(t), nil, httpserver.WithNotifier(mgr))

	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/f-reject/reject", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.GreaterOrEqual(t, mgr.notifyCalls.Load(), int32(1), "notifier must be called on reject")
}

func TestFixDetail_PathTooShort_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/fixes/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── handleAPIFindingAction ───────────────────────────────────────────────────

func TestFindingAction_BadPath_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/findings/abc/wrong-action", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFindingAction_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/findings/abc/generate-fix", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestFindingAction_NoFixGenerator_500(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/findings/abc/generate-fix", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
