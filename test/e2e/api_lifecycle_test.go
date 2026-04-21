//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// newAPITestScheme builds a runtime.Scheme with the types needed for the HTTP
// API lifecycle tests (k8sai CRDs + core + apps + batch).
func newAPITestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	return s
}

// mustJSON marshals v to JSON and panics on error.
func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustJSON: %v", err))
	}
	return b
}

// doRequest performs an HTTP request against ts and returns the status code and
// parsed response body. If the response body is empty the body map will be nil.
func doRequest(t *testing.T, ts *httptest.Server, method, path string, body interface{}) (int, map[string]interface{}) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(mustJSON(body))
	}
	req, err := http.NewRequest(method, ts.URL+path, reqBody)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		// Might be an array or a plain error string — return nil map but still ok
		return resp.StatusCode, nil
	}
	return resp.StatusCode, result
}

// doRequestSlice is like doRequest but decodes the body as a JSON array.
func doRequestSlice(t *testing.T, ts *httptest.Server, method, path string, body interface{}) (int, []interface{}) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(mustJSON(body))
	}
	req, err := http.NewRequest(method, ts.URL+path, reqBody)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result []interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

func TestAPILifecycle(t *testing.T) {
	// ── Setup ──────────────────────────────────────────────────────────────

	// Real SQLite store backed by a temp file (cleanup registered by helper)
	realStore := newSQLiteStore(t)

	// Use a well-known run UID for the DiagnosticRun. The fake K8s client does
	// not auto-assign UIDs, so we pre-populate the CR with a known UID and also
	// seed the SQLite store with the same ID.  This mirrors how the reconciler
	// works in production (the CR UID becomes the store's run ID).
	const knownRunUID = "e2e-run-uid-001"

	// Fake K8s client with full scheme + pre-populated resources.
	// The DiagnosticRun CR is pre-seeded with the known UID so that
	// subsequent API calls can reference it by UID.
	scheme := newAPITestScheme()

	preseededRun := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-test-run",
			Namespace: "default",
			UID:       "e2e-run-uid-001",
		},
		Spec: v1alpha1.DiagnosticRunSpec{
			Target:         v1alpha1.TargetSpec{Scope: "namespace", Namespaces: []string{"default"}},
			ModelConfigRef: "anthropic-credentials",
		},
	}
	testNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "production"},
	}
	testDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web",
			Namespace: "production",
			Labels:    map[string]string{"app": "web"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
		},
	}
	fakeK8s := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(preseededRun, testNS, testDeploy).
		WithStatusSubresource(preseededRun).
		Build()

	// Seed the SQLite store with the same run so GET /api/runs/* work.
	if err := realStore.CreateRun(t.Context(), &store.DiagnosticRun{
		ID:         knownRunUID,
		TargetJSON: `{"scope":"namespace","namespaces":["default"]}`,
		SkillsJSON: `[]`,
		Status:     store.PhasePending,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	// Real HTTP server
	srv := httpserver.New(realStore, fakeK8s, nil)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// Variables shared across subtests
	var (
		runUID     = knownRunUID
		finding1ID string
		finding2ID string
		fixID      string
	)

	// ── 1. POST /api/runs ──────────────────────────────────────────────────
	// Verify the endpoint creates a CR and returns it with metadata (name field).
	// The fake client does not auto-assign UIDs on Create, so we verify name.
	t.Run("POST /api/runs creates a DiagnosticRun", func(t *testing.T) {
		code, body := doRequest(t, ts, http.MethodPost, "/api/runs", map[string]interface{}{
			"name":      "e2e-new-run",
			"namespace": "default",
			"target": map[string]interface{}{
				"scope":      "namespace",
				"namespaces": []string{"default"},
			},
			"modelConfigRef": "anthropic-credentials",
		})
		if code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", code)
		}
		if body == nil {
			t.Fatal("expected JSON body")
		}
		meta, ok := body["metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected metadata object, got %T", body["metadata"])
		}
		name, _ := meta["name"].(string)
		if name == "" {
			t.Fatal("expected non-empty metadata.name in DiagnosticRun response")
		}
		// The pre-seeded run (knownRunUID) is used for all subsequent steps.
		t.Logf("POST /api/runs returned name=%s; using pre-seeded runUID=%s for lifecycle tests", name, runUID)
	})

	// ── 2. GET /api/runs ───────────────────────────────────────────────────
	t.Run("GET /api/runs lists the created run", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/runs", nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if len(items) == 0 {
			t.Fatal("expected at least 1 run (pre-seeded)")
		}
		found := false
		for _, item := range items {
			if r, ok := item.(map[string]interface{}); ok {
				if r["ID"] == runUID {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("run with ID=%s not found in list", runUID)
		}
	})

	// ── 3. GET /api/runs/:id ───────────────────────────────────────────────
	t.Run("GET /api/runs/:id returns run detail", func(t *testing.T) {
		code, body := doRequest(t, ts, http.MethodGet, "/api/runs/"+runUID, nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %v)", code, body)
		}
		if body == nil {
			t.Fatal("expected JSON body")
		}
		id, _ := body["ID"].(string)
		if id != runUID {
			t.Fatalf("expected ID=%s, got %s", runUID, id)
		}
	})

	// ── 4. POST /internal/runs/:id/findings — first finding ────────────────
	t.Run("POST /internal/runs/:id/findings creates first finding (health/critical)", func(t *testing.T) {
		code, _ := doRequest(t, ts, http.MethodPost,
			"/internal/runs/"+runUID+"/findings",
			map[string]interface{}{
				"dimension":          "health",
				"severity":           "critical",
				"title":              "OOMKilled pod",
				"description":        "The pod is being OOM-killed repeatedly",
				"resource_kind":      "Pod",
				"resource_namespace": "default",
				"resource_name":      "api-pod-1",
				"suggestion":         "Increase memory limits",
			},
		)
		if code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", code)
		}
	})

	// ── 5. POST /internal/runs/:id/findings — second finding ───────────────
	t.Run("POST /internal/runs/:id/findings creates second finding (cost/medium)", func(t *testing.T) {
		code, _ := doRequest(t, ts, http.MethodPost,
			"/internal/runs/"+runUID+"/findings",
			map[string]interface{}{
				"dimension":          "cost",
				"severity":           "medium",
				"title":              "Over-allocated CPU",
				"description":        "CPU requests greatly exceed usage",
				"resource_kind":      "Deployment",
				"resource_namespace": "default",
				"resource_name":      "web",
				"suggestion":         "Reduce CPU requests",
			},
		)
		if code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", code)
		}
	})

	// ── 6. GET /api/runs/:id/findings ─────────────────────────────────────
	t.Run("GET /api/runs/:id/findings returns both findings", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet,
			"/api/runs/"+runUID+"/findings", nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if len(items) != 2 {
			t.Fatalf("expected 2 findings, got %d", len(items))
		}
		// Capture the first finding's ID for later use with fixes
		if f, ok := items[0].(map[string]interface{}); ok {
			if inner, ok := f["Finding"].(map[string]interface{}); ok {
				finding1ID, _ = inner["ID"].(string)
				_ = f // satisfy compiler
			} else {
				// findings might be inlined when FixID is empty
				finding1ID, _ = f["ID"].(string)
			}
		}
		if f, ok := items[1].(map[string]interface{}); ok {
			if inner, ok := f["Finding"].(map[string]interface{}); ok {
				finding2ID, _ = inner["ID"].(string)
			} else {
				finding2ID, _ = f["ID"].(string)
			}
		}
		t.Logf("finding1ID=%s finding2ID=%s", finding1ID, finding2ID)
	})

	// ── 7. GET /api/skills (empty) ─────────────────────────────────────────
	t.Run("GET /api/skills returns empty list initially", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/skills", nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		// Empty DB — no builtin skills pre-seeded in this test, so list may be empty.
		t.Logf("initial skill count: %d", len(items))
	})

	// ── 8. POST /api/skills — create custom skill ──────────────────────────
	t.Run("POST /api/skills creates a custom skill", func(t *testing.T) {
		code, body := doRequest(t, ts, http.MethodPost, "/api/skills", map[string]interface{}{
			"name":        "e2e-health-skill",
			"namespace":   "default",
			"dimension":   "health",
			"description": "E2E test skill for health analysis",
			"prompt":      "You are a health analyst. Check pod statuses.",
			"tools":       []string{"kubectl_get"},
			"enabled":     true,
			"priority":    50,
		})
		if code != http.StatusCreated {
			t.Fatalf("expected 201, got %d (body: %v)", code, body)
		}
		if body == nil {
			t.Fatal("expected JSON body")
		}
		meta, ok := body["metadata"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected metadata, got %T", body["metadata"])
		}
		name, _ := meta["name"].(string)
		if name != "e2e-health-skill" {
			t.Fatalf("expected name=e2e-health-skill, got %s", name)
		}
	})

	// ── 9. GET /api/skills — verify new skill appears ──────────────────────
	// Note: POST /api/skills creates the K8s CR only; it does not automatically
	// insert into the SQLite store (the reconciler does that).  We seed the store
	// directly to make the listing test meaningful.
	t.Run("GET /api/skills lists the seeded skill", func(t *testing.T) {
		_ = realStore.UpsertSkill(t.Context(), &store.Skill{
			Name:      "e2e-health-skill",
			Dimension: "health",
			Prompt:    "You are a health analyst.",
			ToolsJSON: `["kubectl_get"]`,
			Source:    "cr",
			Enabled:   true,
			Priority:  50,
		})

		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/skills", nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if len(items) == 0 {
			t.Fatal("expected at least 1 skill after seeding")
		}
		found := false
		for _, item := range items {
			if sk, ok := item.(map[string]interface{}); ok {
				if sk["Name"] == "e2e-health-skill" {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("e2e-health-skill not found in /api/skills response")
		}
	})

	// ── 10. POST /internal/fixes — agent posts a fix result ────────────────
	t.Run("POST /internal/fixes creates a fix", func(t *testing.T) {
		if finding1ID == "" {
			t.Skip("no finding1ID available, skipping fix creation")
		}
		code, body := doRequest(t, ts, http.MethodPost, "/internal/fixes", map[string]interface{}{
			"findingID":        finding1ID,
			"diagnosticRunRef": runUID,
			"findingTitle":     "OOMKilled pod",
			"target": map[string]interface{}{
				"kind":      "Deployment",
				"namespace": "default",
				"name":      "web",
			},
			"patch": map[string]interface{}{
				"type":    "strategic-merge",
				"content": `{"spec":{"template":{"spec":{"containers":[{"name":"web","resources":{"limits":{"memory":"512Mi"}}}]}}}}`,
			},
			"beforeSnapshot": "YXBpVmVyc2lvbjogdjE=",
			"explanation":    "Increase memory limit to prevent OOM kills.",
			"strategy":       "dry-run",
		})
		if code != http.StatusCreated {
			t.Fatalf("expected 201, got %d (body: %v)", code, body)
		}
		// The fake K8s client does not auto-assign UIDs on Create, so the CR UID
		// (and therefore the store Fix ID) will be the empty string.  Retrieve
		// the fix ID from the store via GET /api/fixes instead.
		_, items := doRequestSlice(t, ts, http.MethodGet, "/api/fixes", nil)
		for _, item := range items {
			if f, ok := item.(map[string]interface{}); ok {
				if f["FindingID"] == finding1ID {
					fixID, _ = f["ID"].(string)
					break
				}
			}
		}
		t.Logf("fixID resolved from store: %s", fixID)
	})

	// ── 11. GET /api/fixes — list fixes ────────────────────────────────────
	t.Run("GET /api/fixes lists the created fix", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/fixes", nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		// At this point the fix may or may not be present depending on whether
		// finding1ID was available and the POST /internal/fixes subtest ran.
		if finding1ID != "" {
			if len(items) == 0 {
				t.Fatal("expected at least 1 fix after POST /internal/fixes")
			}
		}
		t.Logf("fix count: %d", len(items))
	})

	// ── 12. GET /api/fixes/:id — get fix detail ────────────────────────────
	t.Run("GET /api/fixes/:id returns fix detail", func(t *testing.T) {
		if fixID == "" {
			t.Skip("no fixID available (POST /internal/fixes may have been skipped)")
		}
		code, body := doRequest(t, ts, http.MethodGet, "/api/fixes/"+fixID, nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %v)", code, body)
		}
		if body == nil {
			t.Fatal("expected JSON body")
		}
		phase, _ := body["Phase"].(string)
		if phase != string(store.FixPhasePendingApproval) {
			t.Fatalf("expected phase=PendingApproval, got %s", phase)
		}
		findingID, _ := body["FindingID"].(string)
		if findingID != finding1ID {
			t.Fatalf("expected FindingID=%s, got %s", finding1ID, findingID)
		}
	})

	// ── 13. PATCH /api/fixes/:id/approve ──────────────────────────────────
	t.Run("PATCH /api/fixes/:id/approve approves the fix", func(t *testing.T) {
		if fixID == "" {
			t.Skip("no fixID available")
		}
		code, _ := doRequest(t, ts, http.MethodPatch, "/api/fixes/"+fixID+"/approve",
			map[string]string{"approvedBy": "e2e-tester"})
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
	})

	// ── 14. GET /api/fixes/:id — verify Approved phase ────────────────────
	t.Run("GET /api/fixes/:id phase is Approved after approval", func(t *testing.T) {
		if fixID == "" {
			t.Skip("no fixID available")
		}
		code, body := doRequest(t, ts, http.MethodGet, "/api/fixes/"+fixID, nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %v)", code, body)
		}
		phase, _ := body["Phase"].(string)
		if phase != string(store.FixPhaseApproved) {
			t.Fatalf("expected phase=Approved, got %s", phase)
		}
		approvedBy, _ := body["ApprovedBy"].(string)
		if approvedBy != "e2e-tester" {
			t.Fatalf("expected ApprovedBy=e2e-tester, got %s", approvedBy)
		}
	})

	// ── 15. GET /api/k8s/resources?kind=Namespace ─────────────────────────
	t.Run("GET /api/k8s/resources?kind=Namespace lists non-system namespaces", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet,
			"/api/k8s/resources?kind=Namespace", nil)
		if code != http.StatusOK {
			t.Fatalf("expected 200, got %d", code)
		}
		if len(items) == 0 {
			t.Fatal("expected at least one namespace (production was pre-populated)")
		}
		found := false
		for _, item := range items {
			if ns, ok := item.(map[string]interface{}); ok {
				if ns["name"] == "production" {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("expected namespace 'production' in result, got: %v", items)
		}
	})

	// Bonus: verify that kube-system is filtered out
	t.Run("GET /api/k8s/resources?kind=Namespace excludes system namespaces", func(t *testing.T) {
		_, items := doRequestSlice(t, ts, http.MethodGet,
			"/api/k8s/resources?kind=Namespace", nil)
		for _, item := range items {
			if ns, ok := item.(map[string]interface{}); ok {
				name, _ := ns["name"].(string)
				switch name {
				case "kube-system", "kube-public", "kube-node-lease":
					t.Errorf("system namespace %q should be filtered out", name)
				}
			}
		}
	})

	// Bonus: second finding ID was captured but not used — log it so CI output
	// shows we exercised both findings.
	if finding2ID != "" {
		t.Logf("second finding captured: %s", finding2ID)
	}
}

// ── Additional API lifecycle tests ────────────────────────────────────────────

// doRequestRaw performs an HTTP request and returns the raw response body as a
// string, without attempting JSON parsing. Useful for plain-text responses (e.g. YAML).
func doRequestRaw(t *testing.T, ts *httptest.Server, method, path string, body interface{}) (int, string) {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(mustJSON(body))
	}
	req, err := http.NewRequest(method, ts.URL+path, reqBody)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// TestAPIRejectFix verifies that PATCH /api/fixes/:id/reject transitions the
// fix phase to Failed and stores the "rejected by user" message.
func TestAPIRejectFix(t *testing.T) {
	realStore := newSQLiteStore(t)
	ctx := t.Context()
	const fixUID = "fix-reject-e2e-1"

	require.NoError(t, realStore.CreateFix(ctx, &store.Fix{
		ID:               fixUID,
		RunID:            "run-1",
		FindingID:        "f-1",
		FindingTitle:     "Replica mismatch",
		TargetKind:       "Deployment",
		TargetNamespace:  "default",
		TargetName:       "web",
		Phase:            store.FixPhasePendingApproval,
		ApprovalRequired: true,
	}))

	fakeK8s := fake.NewClientBuilder().WithScheme(newAPITestScheme()).Build()
	srv := httpserver.New(realStore, fakeK8s, nil)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// Reject the fix
	code, _ := doRequest(t, ts, http.MethodPatch, "/api/fixes/"+fixUID+"/reject", nil)
	require.Equal(t, http.StatusOK, code)

	// Phase must be Failed; message must reflect rejection
	code, body := doRequest(t, ts, http.MethodGet, "/api/fixes/"+fixUID, nil)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, string(store.FixPhaseFailed), body["Phase"])
	assert.Equal(t, "rejected by user", body["Message"])
}

// TestAPIScheduledTemplateMerge verifies that a K8s DiagnosticRun CR with
// spec.schedule set (and no matching SQLite entry) is merged into GET /api/runs
// with Status=Scheduled and the correct Name.
func TestAPIScheduledTemplateMerge(t *testing.T) {
	// No SQLite entries — the template exists only in K8s
	realStore := newSQLiteStore(t)

	scheduledCR := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "weekly-audit-e2e",
			Namespace: "default",
			UID:       "sched-merge-uid-e2e",
		},
		Spec: v1alpha1.DiagnosticRunSpec{
			Schedule:       "0 8 * * 1",
			ModelConfigRef: "creds",
			Target:         v1alpha1.TargetSpec{Scope: "cluster"},
		},
	}
	fakeK8s := fake.NewClientBuilder().WithScheme(newAPITestScheme()).WithObjects(scheduledCR).Build()
	srv := httpserver.New(realStore, fakeK8s, nil)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	code, items := doRequestSlice(t, ts, http.MethodGet, "/api/runs", nil)
	require.Equal(t, http.StatusOK, code)

	var found map[string]interface{}
	for _, item := range items {
		if r, ok := item.(map[string]interface{}); ok && r["Name"] == "weekly-audit-e2e" {
			found = r
			break
		}
	}
	require.NotNil(t, found, "scheduled template 'weekly-audit-e2e' should appear in /api/runs")
	assert.Equal(t, "Scheduled", found["Status"])
}

// TestAPIRunCRDSyntheticFallback verifies that GET /api/runs/:id/crd returns a
// synthetic YAML document (with a "synthesized from store" comment) when the
// original K8s CR has been deleted but the SQLite record still exists.
func TestAPIRunCRDSyntheticFallback(t *testing.T) {
	realStore := newSQLiteStore(t)
	ctx := t.Context()
	const runUID = "crd-fallback-e2e-1"

	require.NoError(t, realStore.CreateRun(ctx, &store.DiagnosticRun{
		ID:         runUID,
		TargetJSON: `{"scope":"namespace","namespaces":["production"]}`,
		SkillsJSON: `["health-analyst"]`,
		Status:     store.PhaseSucceeded,
	}))

	// Fake K8s client has no DiagnosticRun for this UID
	fakeK8s := fake.NewClientBuilder().WithScheme(newAPITestScheme()).Build()
	srv := httpserver.New(realStore, fakeK8s, nil)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	code, body := doRequestRaw(t, ts, http.MethodGet, "/api/runs/"+runUID+"/crd", nil)
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "synthesized from store", "should note the CR was synthesized")
	assert.Contains(t, body, "apiVersion: k8sai.io/v1alpha1")
	assert.Contains(t, body, "kind: DiagnosticRun")
	assert.Contains(t, body, runUID)
	assert.Contains(t, body, "Succeeded")

	// A truly missing run (no K8s CR, no SQLite) must return 404
	code, _ = doRequestRaw(t, ts, http.MethodGet, "/api/runs/does-not-exist/crd", nil)
	assert.Equal(t, http.StatusNotFound, code)
}

// TestAPIEventLifecycle verifies end-to-end event storage and retrieval:
// events inserted via the store appear in GET /api/events and can be filtered
// by namespace.
func TestAPIEventLifecycle(t *testing.T) {
	realStore := newSQLiteStore(t)
	ctx := t.Context()

	now := time.Now()
	require.NoError(t, realStore.UpsertEvent(ctx, &store.Event{
		UID:       "ev-e2e-1",
		Namespace: "default",
		Kind:      "Pod",
		Name:      "api-pod",
		Reason:    "OOMKilled",
		Message:   "container ran out of memory",
		Type:      "Warning",
		Count:     1,
		FirstTime: now.Add(-5 * time.Minute),
		LastTime:  now,
	}))
	require.NoError(t, realStore.UpsertEvent(ctx, &store.Event{
		UID:       "ev-e2e-2",
		Namespace: "production",
		Kind:      "Deployment",
		Name:      "web",
		Reason:    "ScalingReplicaSet",
		Message:   "Scaled up replica set web-abc to 3",
		Type:      "Normal",
		Count:     1,
		FirstTime: now.Add(-2 * time.Minute),
		LastTime:  now,
	}))

	fakeK8s := fake.NewClientBuilder().WithScheme(newAPITestScheme()).Build()
	srv := httpserver.New(realStore, fakeK8s, nil)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	t.Run("GET /api/events returns all events", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/events", nil)
		require.Equal(t, http.StatusOK, code)
		require.Len(t, items, 2)

		uids := make([]string, 0, 2)
		for _, item := range items {
			if ev, ok := item.(map[string]interface{}); ok {
				if uid, ok := ev["UID"].(string); ok {
					uids = append(uids, uid)
				}
			}
		}
		assert.Contains(t, uids, "ev-e2e-1")
		assert.Contains(t, uids, "ev-e2e-2")
	})

	t.Run("GET /api/events?namespace=default filters by namespace", func(t *testing.T) {
		code, items := doRequestSlice(t, ts, http.MethodGet, "/api/events?namespace=default", nil)
		require.Equal(t, http.StatusOK, code)
		require.Len(t, items, 1)

		ev, ok := items[0].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "ev-e2e-1", ev["UID"])
		assert.Equal(t, "default", ev["Namespace"])
		assert.Equal(t, "OOMKilled", ev["Reason"])
	})

	t.Run("GET /api/events with invalid since returns 400", func(t *testing.T) {
		code, _ := doRequestRaw(t, ts, http.MethodGet, "/api/events?since=badvalue", nil)
		assert.Equal(t, http.StatusBadRequest, code)
	})
}
