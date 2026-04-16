package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type fakeStore struct {
	runs     []*store.DiagnosticRun
	findings []*store.Finding
	skills   []*store.Skill
	fixes    []*store.Fix
}

func (f *fakeStore) CreateFinding(_ context.Context, finding *store.Finding) error {
	f.findings = append(f.findings, finding)
	return nil
}
func (f *fakeStore) ListFindings(_ context.Context, runID string) ([]*store.Finding, error) {
	var out []*store.Finding
	for _, fi := range f.findings {
		if fi.RunID == runID {
			out = append(out, fi)
		}
	}
	return out, nil
}
func (f *fakeStore) CreateRun(_ context.Context, r *store.DiagnosticRun) error {
	f.runs = append(f.runs, r)
	return nil
}
func (f *fakeStore) GetRun(_ context.Context, id string) (*store.DiagnosticRun, error) {
	for _, r := range f.runs {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, nil
}
func (f *fakeStore) UpdateRunStatus(_ context.Context, id string, p store.Phase, msg string) error {
	return nil
}
func (f *fakeStore) ListRuns(_ context.Context, opts store.ListOpts) ([]*store.DiagnosticRun, error) {
	return f.runs, nil
}
func (f *fakeStore) UpsertSkill(_ context.Context, s *store.Skill) error {
	f.skills = append(f.skills, s)
	return nil
}
func (f *fakeStore) ListSkills(_ context.Context) ([]*store.Skill, error) {
	return f.skills, nil
}
func (f *fakeStore) GetSkill(_ context.Context, name string) (*store.Skill, error) {
	return nil, nil
}
func (f *fakeStore) DeleteSkill(_ context.Context, _ string) error { return nil }
func (f *fakeStore) CreateFix(_ context.Context, fix *store.Fix) error {
	f.fixes = append(f.fixes, fix)
	return nil
}
func (f *fakeStore) GetFix(_ context.Context, id string) (*store.Fix, error) {
	for _, fx := range f.fixes {
		if fx.ID == id {
			return fx, nil
		}
	}
	return nil, store.ErrNotFound
}
func (f *fakeStore) ListFixes(_ context.Context, _ store.ListOpts) ([]*store.Fix, error) {
	return f.fixes, nil
}
func (f *fakeStore) ListFixesByRun(_ context.Context, runID string) ([]*store.Fix, error) {
	var out []*store.Fix
	for _, fx := range f.fixes {
		if fx.RunID == runID {
			out = append(out, fx)
		}
	}
	return out, nil
}
func (f *fakeStore) UpdateFixPhase(_ context.Context, _ string, _ store.FixPhase, _ string) error {
	return nil
}
func (f *fakeStore) UpdateFixApproval(_ context.Context, _ string, _ string) error { return nil }
func (f *fakeStore) UpdateFixSnapshot(_ context.Context, _ string, _ string) error { return nil }
func (f *fakeStore) Close() error                                                  { return nil }

func TestPostFindings(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, nil, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"dimension": "health", "severity": "critical",
		"title": "Pod crash", "resource_kind": "Pod",
		"resource_namespace": "default", "resource_name": "api-pod",
	})

	req := httptest.NewRequest(http.MethodPost, "/internal/runs/run-123/findings",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusCreated, rr.Code)
	require.Len(t, fs.findings, 1)
	assert.Equal(t, "run-123", fs.findings[0].RunID)
}

func TestGetFindings(t *testing.T) {
	fs := &fakeStore{}
	_ = fs.CreateFinding(context.Background(), &store.Finding{
		RunID: "run-abc", Dimension: "security", Severity: "high", Title: "Root container",
	})
	srv := httpserver.New(fs, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-abc/findings", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp []map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Len(t, resp, 1)
}

func TestGetSkills(t *testing.T) {
	fs := &fakeStore{}
	ctx := context.Background()
	_ = fs.UpsertSkill(ctx, &store.Skill{Name: "s1", Dimension: "health", Enabled: true})

	srv := httpserver.New(fs, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var skills []*store.Skill
	require.NoError(t, json.NewDecoder(w.Body).Decode(&skills))
	require.Len(t, skills, 1)
	assert.Equal(t, "s1", skills[0].Name)
}

func newFakeK8sClient() client.Client {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}

func TestPostRun(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"namespace":      "default",
		"target":         map[string]interface{}{"scope": "namespace", "namespaces": []string{"default"}},
		"modelConfigRef": "anthropic-credentials",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.NotEmpty(t, resp["metadata"])
}

func TestPostSkill(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"name":        "my-analyst",
		"namespace":   "default",
		"dimension":   "health",
		"description": "Analyzes pod health",
		"prompt":      "You are a health analyst...",
		"tools":       []string{"kubectl_get"},
		"enabled":     true,
		"priority":    100,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.NotEmpty(t, resp["metadata"])
}

func TestPostSkillMissingFields(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"namespace": "default",
		// name, dimension, prompt, tools missing
	})
	req := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPostRunMissingModelConfig(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"namespace": "default",
		"target":    map[string]interface{}{"scope": "namespace"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPostRunWithTimeout(t *testing.T) {
	fs := &fakeStore{}
	fc := newFakeK8sClient()
	srv := httpserver.New(fs, fc, nil)

	body := `{"namespace":"default","target":{"scope":"namespace"},"modelConfigRef":"creds","timeoutSeconds":300}`
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), `"timeoutSeconds":300`)
}

func TestPostRunWithOutputLanguage(t *testing.T) {
	fs := &fakeStore{}
	fc := newFakeK8sClient()
	srv := httpserver.New(fs, fc, nil)

	body := `{"namespace":"default","target":{"scope":"namespace"},"modelConfigRef":"creds","outputLanguage":"zh"}`
	req := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), `"outputLanguage":"zh"`)
}

func TestGenerateFix_CreatesJob(t *testing.T) {
	fs := &fakeStore{}
	run := &store.DiagnosticRun{ID: "run-uid-1", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhaseSucceeded}
	fs.runs = append(fs.runs, run)
	finding := &store.Finding{
		ID: "finding-1", RunID: "run-uid-1",
		Title: "Test finding", ResourceKind: "Deployment",
		ResourceNamespace: "ns", ResourceName: "nginx",
	}
	fs.findings = append(fs.findings, finding)

	fc := newFakeK8sClient()
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run-abc", Namespace: "default", UID: "run-uid-1"},
		Spec:       v1alpha1.DiagnosticRunSpec{ModelConfigRef: "creds", OutputLanguage: "en"},
	}
	_ = fc.Create(context.Background(), cr)

	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{
		AgentImage: "agent:test", ControllerURL: "http://x",
	})
	srv := httpserver.New(fs, fc, fg)

	req := httptest.NewRequest(http.MethodPost, "/api/findings/finding-1/generate-fix", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var jobList batchv1.JobList
	err := fc.List(context.Background(), &jobList)
	assert.NoError(t, err)
	assert.Len(t, jobList.Items, 1)
	assert.Equal(t, "fix-gen-finding-1", jobList.Items[0].Name)
}

func TestGenerateFix_ReturnsExistingFix(t *testing.T) {
	fs := &fakeStore{}
	fs.runs = append(fs.runs, &store.DiagnosticRun{ID: "run-uid-1"})
	finding := &store.Finding{ID: "finding-2", RunID: "run-uid-1", Title: "t",
		ResourceKind: "Deployment", ResourceNamespace: "ns", ResourceName: "nginx"}
	fs.findings = append(fs.findings, finding)
	fs.fixes = append(fs.fixes, &store.Fix{
		ID: "existing-fix-uid", RunID: "run-uid-1", FindingID: "finding-2", FindingTitle: "t",
	})
	fc := newFakeK8sClient()
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default", UID: "run-uid-1"},
		Spec:       v1alpha1.DiagnosticRunSpec{ModelConfigRef: "creds"},
	}
	_ = fc.Create(context.Background(), cr)
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	req := httptest.NewRequest(http.MethodPost, "/api/findings/finding-2/generate-fix", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"fixID":"existing-fix-uid"`)

	var jobList batchv1.JobList
	_ = fc.List(context.Background(), &jobList)
	assert.Len(t, jobList.Items, 0)
}

func TestGenerateFix_FindingNotFound(t *testing.T) {
	fs := &fakeStore{}
	fc := newFakeK8sClient()
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	req := httptest.NewRequest(http.MethodPost, "/api/findings/does-not-exist/generate-fix", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
