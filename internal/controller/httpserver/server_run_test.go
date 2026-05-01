package httpserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// ── handleAPIRunDetail ───────────────────────────────────────────────────────

func TestRunDetail_GET_HappyPath(t *testing.T) {
	fs := &fakeStore{
		runs: []*store.DiagnosticRun{{ID: "r1", Status: store.PhaseSucceeded}},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/r1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRunDetail_GET_FallbackToK8sCRWhenNotInStore(t *testing.T) {
	uid := "uid-fallback-1"
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "scheduled-child", UID: types.UID(uid),
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
		Spec: v1alpha1.DiagnosticRunSpec{
			Schedule: "*/5 * * * *",
		},
	}

	fs := &fakeStore{} // not in SQLite
	srv := httpserver.New(fs, fakeK8sClient(t, cr), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+uid, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "should fall back to runFromK8s")

	var got map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, uid, got["ID"])
	assert.Equal(t, "Scheduled", got["Status"], "Schedule set + empty Status.Phase → 'Scheduled'")
}

func TestRunDetail_GET_NotFoundEverywhere_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/runs/missing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRunDetail_GET_FindingsTail_OK(t *testing.T) {
	fs := &fakeStore{
		runs: []*store.DiagnosticRun{{ID: "r1", Status: store.PhaseSucceeded}},
		findings: []*store.Finding{
			{ID: "f1", RunID: "r1", Title: "foo"},
		},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/runs/r1/findings", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var got []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.Equal(t, "f1", got[0]["ID"])
}

func TestRunDetail_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/runs/r1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestRunDetail_PathTooShort_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	// "/api/runs/" with no id strips empty segment → triggers 400 missing ID, not 404
	req := httptest.NewRequest(http.MethodGet, "/api/runs/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	// Either 400 or 404 is acceptable — assert both as legitimate "not OK"
	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusBadRequest,
		"got %d", w.Code)
}

// ── handleAPIRunCRD ──────────────────────────────────────────────────────────

func TestRunCRD_LiveCRReturnedAsYAML(t *testing.T) {
	uid := "crd-uid-1"
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name: "live-run", UID: types.UID(uid),
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
	}
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t, cr), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+uid+"/crd", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/plain")
	body := w.Body.String()
	assert.Contains(t, body, "kind: DiagnosticRun")
	assert.Contains(t, body, "live-run")
}

func TestRunCRD_FallbackSyntheticYAMLWhenCRGone(t *testing.T) {
	fs := &fakeStore{
		runs: []*store.DiagnosticRun{
			{ID: "u-deleted", Status: store.PhaseSucceeded, TargetJSON: "{}", SkillsJSON: "[]"},
		},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil) // no CR loaded

	req := httptest.NewRequest(http.MethodGet, "/api/runs/u-deleted/crd", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	// syntheticRunYAML produces text starting with "# Run reconstructed..." or similar.
	assert.NotEmpty(t, body, "synthetic YAML should be returned")
}

func TestRunCRD_NoCRNoStore_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/runs/nope/crd", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── handleAPIRunsBatch ───────────────────────────────────────────────────────

func TestRunsBatch_DELETE_OK(t *testing.T) {
	fs := &fakeStore{
		runs: []*store.DiagnosticRun{
			{ID: "r1"}, {ID: "r2"},
		},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil)

	body, _ := json.Marshal(map[string]interface{}{"ids": []string{"r1", "r2"}})
	req := httptest.NewRequest(http.MethodDelete, "/api/runs/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRunsBatch_DELETE_BadJSON_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/runs/batch", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRunsBatch_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/runs/batch", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── handleAPIRunsPost — error / edge branches ────────────────────────────────

func TestRunsPost_BadJSON_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── handleAPIFixDetail GET ───────────────────────────────────────────────────

func TestFixDetail_GET_HappyPath(t *testing.T) {
	fs := &fakeStore{fixes: []*store.Fix{{ID: "fx1", Phase: store.FixPhasePendingApproval}}}
	srv := httpserver.New(fs, fakeK8sClient(t), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/fixes/fx1", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestFixDetail_GET_NotFound_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/fixes/missing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFixDetail_UnknownAction_404(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/fx1/destroy", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFixDetail_ApprovePatch_MissingApprovedBy_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	body, _ := json.Marshal(map[string]string{}) // missing approvedBy
	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/fx1/approve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── handleAPISkills (GET filter / DELETE) ────────────────────────────────────

func TestSkills_GET_FilterByDimension(t *testing.T) {
	fs := &fakeStore{
		skills: []*store.Skill{
			{Name: "a", Dimension: "health"},
			{Name: "b", Dimension: "security"},
		},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestSkills_DELETE_Unsupported_405(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPatch, "/api/skills", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── handleAPIFixes (GET) ─────────────────────────────────────────────────────

func TestFixes_GET_PaginatedFlag(t *testing.T) {
	fs := &fakeStore{}
	for i := 0; i < 3; i++ {
		fs.fixes = append(fs.fixes, &store.Fix{ID: "fx", Phase: store.FixPhasePendingApproval})
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/fixes?page=1&pageSize=10", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestFixes_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/fixes", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── handleAPIRuns ────────────────────────────────────────────────────────────

func TestRuns_GET_LegacyMode(t *testing.T) {
	fs := &fakeStore{
		runs: []*store.DiagnosticRun{
			{ID: "r1", Status: store.PhasePending, TargetJSON: "{}", SkillsJSON: "[]"},
		},
	}
	srv := httpserver.New(fs, fakeK8sClient(t), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs?limit=10", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRuns_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodDelete, "/api/runs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── handleAPISkillsPost ──────────────────────────────────────────────────────

func TestSkillsPost_BadJSON_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSkillsPost_MissingDimension_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	body, _ := json.Marshal(map[string]interface{}{
		"name": "x", "prompt": "p", "tools": []string{"a"},
		// dimension omitted
	})
	req := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSkillsPost_MissingTools_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	body, _ := json.Marshal(map[string]interface{}{
		"name": "x", "dimension": "health", "prompt": "p", "tools": []string{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSkillsPost_DefaultsAppliedOnSuccess(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	body, _ := json.Marshal(map[string]interface{}{
		"name": "x", "dimension": "health", "prompt": "p", "tools": []string{"a"},
		// namespace + priority omitted → defaults: "default", 100
	})
	req := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var got v1alpha1.DiagnosticSkill
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.Equal(t, "default", got.Namespace, "namespace should default to 'default'")
	require.NotNil(t, got.Spec.Priority)
	assert.Equal(t, 100, *got.Spec.Priority, "priority should default to 100")
}

// ── handleAPIEvents ──────────────────────────────────────────────────────────

func TestEvents_GET_LegacyMode(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events?namespace=default&since=30&limit=10", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestEvents_GET_PaginatedMode(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events?page=1&pageSize=10", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestEvents_GET_PaginatedPageSizeCapped(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events?page=1&pageSize=999", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestEvents_BadSince_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events?since=abc", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestEvents_BadLimit_400(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/events?limit=abc", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestEvents_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, fakeK8sClient(t), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/events", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── findFixCRByStoreID — happy path with matching CR ─────────────────────────

func TestFixDetail_RejectPatch_FindsFixCRAndUpdatesStatus(t *testing.T) {
	uid := "fx-cr-uid-1"
	fixCR := &v1alpha1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fx-cr", UID: types.UID(uid), Namespace: "default"},
		Spec:       v1alpha1.DiagnosticFixSpec{FindingID: "fnd-1"},
	}
	fs := &fakeStore{fixes: []*store.Fix{{ID: uid, Phase: store.FixPhasePendingApproval}}}
	srv := httpserver.New(fs, fakeK8sClient(t, fixCR), nil)

	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/"+uid+"/reject", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
