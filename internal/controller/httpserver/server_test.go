package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/translator"
	"github.com/kube-agent-helper/kube-agent-helper/internal/metrics"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type fakeStore struct {
	runs            []*store.DiagnosticRun
	findings        []*store.Finding
	skills          []*store.Skill
	fixes           []*store.Fix
	events          []*store.Event
	runLogs         []store.RunLog
	notifConfigs    []*store.NotificationConfig
	lastListEvtsOpt store.ListEventsOpts
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
func (f *fakeStore) UpsertEvent(_ context.Context, _ *store.Event) error { return nil }
func (f *fakeStore) ListEvents(_ context.Context, opts store.ListEventsOpts) ([]*store.Event, error) {
	f.lastListEvtsOpt = opts
	return f.events, nil
}
func (f *fakeStore) InsertMetricSnapshot(_ context.Context, _ *store.MetricSnapshot) error {
	return nil
}
func (f *fakeStore) QueryMetricHistory(_ context.Context, _ string, _ int) ([]*store.MetricSnapshot, error) {
	return nil, nil
}
func (f *fakeStore) AppendRunLog(_ context.Context, log store.RunLog) error {
	log.ID = int64(len(f.runLogs) + 1)
	f.runLogs = append(f.runLogs, log)
	return nil
}
func (f *fakeStore) ListRunLogs(_ context.Context, runID string, afterID int64) ([]store.RunLog, error) {
	var out []store.RunLog
	for _, l := range f.runLogs {
		if l.RunID == runID && l.ID > afterID {
			out = append(out, l)
		}
	}
	return out, nil
}
func (f *fakeStore) ListRunsPaginated(_ context.Context, opts store.ListOpts) (store.PaginatedResult[*store.DiagnosticRun], error) {
	filtered := f.runs
	if opts.ClusterName != "" {
		var out []*store.DiagnosticRun
		for _, r := range filtered {
			if r.ClusterName == opts.ClusterName {
				out = append(out, r)
			}
		}
		filtered = out
	}
	if f := opts.Filters; f != nil {
		if v, ok := f["phase"]; ok && v != "" {
			var out []*store.DiagnosticRun
			for _, r := range filtered {
				if string(r.Status) == v {
					out = append(out, r)
				}
			}
			filtered = out
		}
	}
	total := len(filtered)
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return store.PaginatedResult[*store.DiagnosticRun]{
		Items: filtered[start:end], Total: total, Page: page, PageSize: pageSize,
	}, nil
}
func (f *fakeStore) ListFixesPaginated(_ context.Context, opts store.ListOpts) (store.PaginatedResult[*store.Fix], error) {
	filtered := f.fixes
	if opts.ClusterName != "" {
		var out []*store.Fix
		for _, fx := range filtered {
			if fx.ClusterName == opts.ClusterName {
				out = append(out, fx)
			}
		}
		filtered = out
	}
	if fil := opts.Filters; fil != nil {
		if v, ok := fil["phase"]; ok && v != "" {
			var out []*store.Fix
			for _, fx := range filtered {
				if string(fx.Phase) == v {
					out = append(out, fx)
				}
			}
			filtered = out
		}
	}
	total := len(filtered)
	page := opts.Page
	if page <= 0 {
		page = 1
	}
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return store.PaginatedResult[*store.Fix]{
		Items: filtered[start:end], Total: total, Page: page, PageSize: pageSize,
	}, nil
}
func (f *fakeStore) ListEventsPaginated(_ context.Context, opts store.ListEventsOpts, page, pageSize int) (store.PaginatedResult[*store.Event], error) {
	filtered := f.events
	if opts.ClusterName != "" {
		var out []*store.Event
		for _, ev := range filtered {
			if ev.ClusterName == opts.ClusterName {
				out = append(out, ev)
			}
		}
		filtered = out
	}
	total := len(filtered)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + pageSize
	if end > len(filtered) {
		end = len(filtered)
	}
	return store.PaginatedResult[*store.Event]{
		Items: filtered[start:end], Total: total, Page: page, PageSize: pageSize,
	}, nil
}
func (f *fakeStore) DeleteRuns(_ context.Context, ids []string) error {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var remaining []*store.DiagnosticRun
	for _, r := range f.runs {
		if !idSet[r.ID] {
			remaining = append(remaining, r)
		}
	}
	f.runs = remaining
	return nil
}
func (f *fakeStore) BatchUpdateFixPhase(_ context.Context, ids []string, phase store.FixPhase, msg string) error {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	for _, fx := range f.fixes {
		if idSet[fx.ID] {
			fx.Phase = phase
			fx.Message = msg
		}
	}
	return nil
}
func (f *fakeStore) ListNotificationConfigs(_ context.Context) ([]*store.NotificationConfig, error) {
	return f.notifConfigs, nil
}
func (f *fakeStore) GetNotificationConfig(_ context.Context, id string) (*store.NotificationConfig, error) {
	for _, c := range f.notifConfigs {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, store.ErrNotFound
}
func (f *fakeStore) CreateNotificationConfig(_ context.Context, cfg *store.NotificationConfig) error {
	if cfg.ID == "" {
		cfg.ID = fmt.Sprintf("nc-%d", len(f.notifConfigs)+1)
	}
	f.notifConfigs = append(f.notifConfigs, cfg)
	return nil
}
func (f *fakeStore) UpdateNotificationConfig(_ context.Context, cfg *store.NotificationConfig) error {
	for i, c := range f.notifConfigs {
		if c.ID == cfg.ID {
			f.notifConfigs[i] = cfg
			return nil
		}
	}
	return store.ErrNotFound
}
func (f *fakeStore) DeleteNotificationConfig(_ context.Context, id string) error {
	for i, c := range f.notifConfigs {
		if c.ID == id {
			f.notifConfigs = append(f.notifConfigs[:i], f.notifConfigs[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}
func (f *fakeStore) PurgeOldEvents(_ context.Context, _ time.Time) error  { return nil }
func (f *fakeStore) PurgeOldMetrics(_ context.Context, _ time.Time) error { return nil }
func (f *fakeStore) Close() error                                         { return nil }

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

func TestInternalFixes_CreatesCRAndStoreEntry(t *testing.T) {
	fs := &fakeStore{}
	fc := newFakeK8sClient()
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	body := `{
	  "findingID":"f-1",
	  "diagnosticRunRef":"run-uid-1",
	  "findingTitle":"t",
	  "target":{"kind":"Deployment","namespace":"ns","name":"nginx"},
	  "patch":{"type":"strategic-merge","content":"{\"spec\":{\"replicas\":2}}"},
	  "beforeSnapshot":"YXBpVmVyc2lvbjogdjE=",
	  "explanation":"Scale up to reduce risk."
	}`
	req := httptest.NewRequest(http.MethodPost, "/internal/fixes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// CR exists in fake k8s client
	var cr v1alpha1.DiagnosticFixList
	_ = fc.List(context.Background(), &cr)
	assert.Len(t, cr.Items, 1)
	assert.Equal(t, "f-1", cr.Items[0].Spec.FindingID)
	assert.Equal(t, "run-uid-1", cr.Items[0].Spec.DiagnosticRunRef)
	assert.Equal(t, "Deployment", cr.Items[0].Spec.Target.Kind)
	assert.Equal(t, "strategic-merge", cr.Items[0].Spec.Patch.Type)

	// Store has matching entry
	assert.Len(t, fs.fixes, 1)
	assert.Equal(t, "f-1", fs.fixes[0].FindingID)
	assert.Equal(t, "YXBpVmVyc2lvbjogdjE=", fs.fixes[0].BeforeSnapshot)
	assert.Equal(t, "Scale up to reduce risk.", fs.fixes[0].Message)
}

func TestInternalFixes_RejectsMissingFields(t *testing.T) {
	fs := &fakeStore{}
	fc := newFakeK8sClient()
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	body := `{"findingID":"f-1"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/fixes", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetFindings_IncludesFixID(t *testing.T) {
	fs := &fakeStore{}
	finding := &store.Finding{ID: "f-1", RunID: "run-1", Title: "t"}
	fs.findings = append(fs.findings, finding)
	fs.fixes = append(fs.fixes, &store.Fix{
		ID: "fix-uid-1", RunID: "run-1", FindingID: "f-1",
	})
	fc := newFakeK8sClient()
	fg := translator.NewFixGenerator(translator.FixGeneratorConfig{AgentImage: "a", ControllerURL: "http://x"})
	srv := httpserver.New(fs, fc, fg)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/run-1/findings", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"FixID":"fix-uid-1"`)
}

func TestGetFix_Success(t *testing.T) {
	fs := &fakeStore{}
	fs.fixes = append(fs.fixes, &store.Fix{
		ID:           "fix-123",
		RunID:        "run-1",
		FindingID:    "finding-1",
		FindingTitle: "Pod crash",
		Phase:        store.FixPhasePendingApproval,
	})
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/fixes/fix-123", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "fix-123", resp["ID"])
	assert.Equal(t, "finding-1", resp["FindingID"])
}

func TestGetFix_NotFound(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/fixes/unknown-id", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestApproveFix_Success(t *testing.T) {
	fs := &fakeStore{}
	fs.fixes = append(fs.fixes, &store.Fix{
		ID:        "fix-456",
		RunID:     "run-1",
		FindingID: "finding-1",
		Phase:     store.FixPhasePendingApproval,
	})
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	body, _ := json.Marshal(map[string]string{"approvedBy": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/fix-456/approve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestApproveFix_MissingApprovedBy(t *testing.T) {
	fs := &fakeStore{}
	fs.fixes = append(fs.fixes, &store.Fix{
		ID:        "fix-789",
		RunID:     "run-1",
		FindingID: "finding-1",
		Phase:     store.FixPhasePendingApproval,
	})
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	// Send valid JSON but omit approvedBy — server should reject with 400
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/fix-789/approve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRejectFix_Success(t *testing.T) {
	fs := &fakeStore{}
	fs.fixes = append(fs.fixes, &store.Fix{
		ID:        "fix-abc",
		RunID:     "run-1",
		FindingID: "finding-1",
		Phase:     store.FixPhasePendingApproval,
	})
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/fix-abc/reject", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRejectFix_NotFound(t *testing.T) {
	// rejectErrFakeStore returns ErrNotFound from UpdateFixPhase so that the
	// server's reject handler responds with 404 for an unknown fix ID.
	srv := httpserver.New(&rejectErrFakeStore{}, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodPatch, "/api/fixes/does-not-exist/reject", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// rejectErrFakeStore is a fakeStore variant whose UpdateFixPhase returns ErrNotFound.
type rejectErrFakeStore struct {
	fakeStore
}

func (r *rejectErrFakeStore) UpdateFixPhase(_ context.Context, _ string, _ store.FixPhase, _ string) error {
	return store.ErrNotFound
}

// filteringFakeStore extends fakeStore with ClusterName filtering for ListRuns, ListFixes, and ListEvents.
type filteringFakeStore struct {
	fakeStore
}

func (f *filteringFakeStore) ListRuns(_ context.Context, opts store.ListOpts) ([]*store.DiagnosticRun, error) {
	if opts.ClusterName == "" {
		return f.runs, nil
	}
	var out []*store.DiagnosticRun
	for _, r := range f.runs {
		if r.ClusterName == opts.ClusterName {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *filteringFakeStore) ListFixes(_ context.Context, opts store.ListOpts) ([]*store.Fix, error) {
	if opts.ClusterName == "" {
		return f.fixes, nil
	}
	var out []*store.Fix
	for _, fx := range f.fixes {
		if fx.ClusterName == opts.ClusterName {
			out = append(out, fx)
		}
	}
	return out, nil
}

func (f *filteringFakeStore) ListEvents(_ context.Context, opts store.ListEventsOpts) ([]*store.Event, error) {
	f.lastListEvtsOpt = opts
	if opts.ClusterName == "" {
		return f.events, nil
	}
	var out []*store.Event
	for _, ev := range f.events {
		if ev.ClusterName == opts.ClusterName {
			out = append(out, ev)
		}
	}
	return out, nil
}

func TestK8sResources_ListNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}}
	nsSys := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns, nsSys).Build()
	srv := httpserver.New(&fakeStore{}, k8s, nil)

	req := httptest.NewRequest("GET", "/api/k8s/resources?kind=Namespace", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var items []map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item["name"]
	}
	assert.Contains(t, names, "production")
	assert.NotContains(t, names, "kube-system")
}

func TestK8sResources_ListDeployments(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "prod",
			Labels: map[string]string{"app": "web"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		},
	}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()
	srv := httpserver.New(&fakeStore{}, k8s, nil)

	req := httptest.NewRequest("GET", "/api/k8s/resources?kind=Deployment&namespace=prod", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var items []map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "web", items[0]["name"])
}

func TestK8sResources_GetSingleDeployment(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web", Namespace: "prod",
			Labels: map[string]string{"app": "web"},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
		},
	}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy).Build()
	srv := httpserver.New(&fakeStore{}, k8s, nil)

	req := httptest.NewRequest("GET", "/api/k8s/resources?kind=Deployment&namespace=prod&name=web", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	meta := result["metadata"].(map[string]interface{})
	assert.Equal(t, "web", meta["name"])
	spec := result["spec"].(map[string]interface{})
	selector := spec["selector"].(map[string]interface{})
	matchLabels := selector["matchLabels"].(map[string]interface{})
	assert.Equal(t, "web", matchLabels["app"])
}

// TestGetRuns_EnrichesWithK8sNames verifies that enrichWithK8sNames populates
// the Name field on SQLite runs by matching their IDs (UIDs) to K8s CR names.
func TestGetRuns_EnrichesWithK8sNames(t *testing.T) {
	const uid = "run-uid-enrich-1"
	fs := &fakeStore{}
	fs.runs = []*store.DiagnosticRun{
		{ID: uid, Status: store.PhaseSucceeded},
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "my-run", Namespace: "default", UID: uid},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
	srv := httpserver.New(fs, fc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var runs []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&runs))
	require.Len(t, runs, 1)
	assert.Equal(t, "my-run", runs[0]["Name"])
}

// TestGetRuns_MergeScheduledTemplates verifies that K8s DiagnosticRun CRs with
// spec.schedule set are merged into the list response with Status=Scheduled.
func TestGetRuns_MergeScheduledTemplates(t *testing.T) {
	// No SQLite runs — the scheduled template exists only in K8s
	fs := &fakeStore{}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "weekly-audit", Namespace: "default", UID: "sched-uid-1"},
		Spec: v1alpha1.DiagnosticRunSpec{
			Schedule:       "0 8 * * 1",
			ModelConfigRef: "creds",
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
	srv := httpserver.New(fs, fc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var runs []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&runs))
	require.Len(t, runs, 1, "scheduled template should appear in list")
	assert.Equal(t, "weekly-audit", runs[0]["Name"])
	assert.Equal(t, "Scheduled", runs[0]["Status"])
}

// TestGetRuns_MergeScheduledTemplates_NoDuplicate verifies that a scheduled
// template already present in SQLite (by UID) is NOT duplicated.
func TestGetRuns_MergeScheduledTemplates_NoDuplicate(t *testing.T) {
	const uid = "sched-uid-dup"
	fs := &fakeStore{
		runs: []*store.DiagnosticRun{
			{ID: uid, Status: store.Phase("Scheduled")},
		},
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	cr := &v1alpha1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{Name: "hourly-audit", Namespace: "default", UID: uid},
		Spec: v1alpha1.DiagnosticRunSpec{
			Schedule:       "0 * * * *",
			ModelConfigRef: "creds",
		},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
	srv := httpserver.New(fs, fc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var runs []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&runs))
	assert.Len(t, runs, 1, "should not duplicate runs already in SQLite")
}

// TestGetFixes_EnrichesWithK8sNames verifies that enrichFixesWithK8sNames
// populates the Name field on fixes by matching their IDs (UIDs) to DiagnosticFix CR names.
func TestGetFixes_EnrichesWithK8sNames(t *testing.T) {
	const uid = "fix-uid-enrich-1"
	fs := &fakeStore{
		fixes: []*store.Fix{
			{ID: uid, RunID: "run-1", FindingID: "f-1", Phase: store.FixPhasePendingApproval},
		},
	}

	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	cr := &v1alpha1.DiagnosticFix{
		ObjectMeta: metav1.ObjectMeta{Name: "fix-f-1", Namespace: "default", UID: uid},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cr).Build()
	srv := httpserver.New(fs, fc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/fixes", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var fixes []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&fixes))
	require.Len(t, fixes, 1)
	assert.Equal(t, "fix-f-1", fixes[0]["Name"])
}

// TestGetRunCRD_SyntheticFallback verifies that when the K8s CR is gone but
// the SQLite record exists, the /api/runs/{id}/crd endpoint returns a synthetic
// YAML with the "(synthesized from store)" comment.
func TestGetRunCRD_SyntheticFallback(t *testing.T) {
	const uid = "run-uid-crd-fallback"
	fs := &fakeStore{
		runs: []*store.DiagnosticRun{
			{ID: uid, TargetJSON: `{"scope":"namespace"}`, SkillsJSON: `["health"]`, Status: store.PhaseSucceeded},
		},
	}
	// K8s client has no DiagnosticRun for this UID
	fc := newFakeK8sClient()
	srv := httpserver.New(fs, fc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/"+uid+"/crd", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "synthesized from store")
	assert.Contains(t, body, "apiVersion: k8sai.io/v1alpha1")
	assert.Contains(t, body, "kind: DiagnosticRun")
	assert.Contains(t, body, uid)
}

// TestGetRunCRD_NotFound verifies that /api/runs/{id}/crd returns 404 when
// neither the K8s CR nor the SQLite record exists.
func TestGetRunCRD_NotFound(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/runs/does-not-exist/crd", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAPIRunsClusterFilter(t *testing.T) {
	fs := &filteringFakeStore{}
	fs.runs = []*store.DiagnosticRun{
		{ID: "r1", ClusterName: "local", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending},
		{ID: "r2", ClusterName: "local", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending},
		{ID: "r3", ClusterName: "prod", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending},
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	// Filter by cluster=prod → expect 1 run
	req := httptest.NewRequest(http.MethodGet, "/api/runs?cluster=prod", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var prodRuns []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&prodRuns))
	assert.Len(t, prodRuns, 1)

	// Filter by cluster=local → expect 2 runs
	req = httptest.NewRequest(http.MethodGet, "/api/runs?cluster=local", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var localRuns []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&localRuns))
	assert.Len(t, localRuns, 2)

	// No filter → expect 3 runs
	req = httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var allRuns []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&allRuns))
	assert.Len(t, allRuns, 3)
}

func TestAPIFixesClusterFilter(t *testing.T) {
	fs := &filteringFakeStore{}
	fs.runs = []*store.DiagnosticRun{
		{ID: "run-1", ClusterName: "local", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhaseSucceeded},
	}
	fs.fixes = []*store.Fix{
		{ID: "fix-1", RunID: "run-1", ClusterName: "local", FindingTitle: "t1",
			TargetKind: "Deployment", TargetNamespace: "ns", TargetName: "app",
			Phase: store.FixPhasePendingApproval},
		{ID: "fix-2", RunID: "run-1", ClusterName: "staging", FindingTitle: "t2",
			TargetKind: "Deployment", TargetNamespace: "ns", TargetName: "app2",
			Phase: store.FixPhasePendingApproval},
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	// Filter by cluster=staging → expect 1 fix
	req := httptest.NewRequest(http.MethodGet, "/api/fixes?cluster=staging", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var stagingFixes []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&stagingFixes))
	assert.Len(t, stagingFixes, 1)

	// No filter → expect 2 fixes
	req = httptest.NewRequest(http.MethodGet, "/api/fixes", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var allFixes []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&allFixes))
	assert.Len(t, allFixes, 2)
}

func TestAPIClustersGet(t *testing.T) {
	// Use a fake k8s client with no ClusterConfig CRs — should still return "local"
	srv := httpserver.New(&fakeStore{}, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var clusters []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&clusters))
	require.NotEmpty(t, clusters, "response should contain at least the local cluster")

	// Verify the local cluster entry is present with phase "Connected"
	var foundLocal bool
	for _, c := range clusters {
		if c["name"] == "local" {
			foundLocal = true
			assert.Equal(t, "Connected", c["phase"])
		}
	}
	assert.True(t, foundLocal, "response must include a 'local' cluster entry")
}

func TestAPIEventsClusterFilter(t *testing.T) {
	fs := &filteringFakeStore{}
	fs.events = []*store.Event{
		{UID: "ev-1", ClusterName: "local", Namespace: "default", Kind: "Pod", Name: "pod-1",
			Reason: "OOMKilled", Message: "out of memory", Type: "Warning", Count: 1},
		{UID: "ev-2", ClusterName: "prod", Namespace: "default", Kind: "Pod", Name: "pod-2",
			Reason: "BackOff", Message: "crash loop", Type: "Warning", Count: 3},
	}
	srv := httpserver.New(fs, nil, nil)

	// Filter by cluster=prod → expect 1 event
	req := httptest.NewRequest(http.MethodGet, "/api/events?cluster=prod", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var prodEvents []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&prodEvents))
	assert.Len(t, prodEvents, 1)
	assert.Equal(t, "ev-2", prodEvents[0]["UID"])
	assert.Equal(t, "prod", fs.lastListEvtsOpt.ClusterName)

	// No filter → expect 2 events
	req = httptest.NewRequest(http.MethodGet, "/api/events", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var allEvents []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&allEvents))
	assert.Len(t, allEvents, 2)
	assert.Equal(t, "", fs.lastListEvtsOpt.ClusterName)
}

func TestHandleAPIEvents(t *testing.T) {
	t.Run("GET with no filters returns empty array when store returns nil", func(t *testing.T) {
		fs := &fakeStore{}
		srv := httpserver.New(fs, nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp []interface{}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Len(t, resp, 0)
	})

	t.Run("GET with namespace and since passes correct opts to store", func(t *testing.T) {
		fs := &fakeStore{
			events: []*store.Event{
				{UID: "ev-1", Namespace: "default", Kind: "Pod", Name: "api-pod",
					Reason: "OOMKilled", Message: "pod ran out of memory", Type: "Warning"},
			},
		}
		srv := httpserver.New(fs, nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/events?namespace=default&since=60", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "default", fs.lastListEvtsOpt.Namespace)
		assert.Equal(t, 60, fs.lastListEvtsOpt.SinceMinutes)

		var resp []map[string]interface{}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		require.Len(t, resp, 1)
		assert.Equal(t, "ev-1", resp[0]["UID"])
	})

	t.Run("non-GET method returns 405", func(t *testing.T) {
		fs := &fakeStore{}
		srv := httpserver.New(fs, nil, nil)

		req := httptest.NewRequest(http.MethodPost, "/api/events", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid since value returns 400", func(t *testing.T) {
		fs := &fakeStore{}
		srv := httpserver.New(fs, nil, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/events?since=notanumber", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ── POST /api/clusters tests ──────────────────────────────────────────────────

func TestAPIClustersPost_Success(t *testing.T) {
	fc := newFakeK8sClient()
	srv := httpserver.New(&fakeStore{}, fc, nil)

	body, _ := json.Marshal(map[string]string{
		"name":       "prod",
		"namespace":  "kube-agent-helper",
		"secretName": "prod-kubeconfig",
		"secretKey":  "kubeconfig",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/clusters", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "prod", resp["name"])
	assert.Equal(t, "kube-agent-helper", resp["namespace"])

	// Verify the ClusterConfig CR was actually created in the fake client
	var cc v1alpha1.ClusterConfig
	require.NoError(t, fc.Get(context.Background(), client.ObjectKey{
		Name: "prod", Namespace: "kube-agent-helper",
	}, &cc))
	assert.Equal(t, "prod-kubeconfig", cc.Spec.KubeConfigRef.Name)
	assert.Equal(t, "kubeconfig", cc.Spec.KubeConfigRef.Key)
}

func TestAPIClustersPost_DefaultNamespace(t *testing.T) {
	fc := newFakeK8sClient()
	srv := httpserver.New(&fakeStore{}, fc, nil)

	body, _ := json.Marshal(map[string]string{
		"name":       "staging",
		"secretName": "staging-secret",
		"secretKey":  "config",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/clusters", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "kube-agent-helper", resp["namespace"], "should default to kube-agent-helper namespace")
}

func TestAPIClustersPost_MissingRequiredFields(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, newFakeK8sClient(), nil)

	tests := []struct {
		name string
		body map[string]string
	}{
		{"missing name", map[string]string{"secretName": "s", "secretKey": "k"}},
		{"missing secretName", map[string]string{"name": "n", "secretKey": "k"}},
		{"missing secretKey", map[string]string{"name": "n", "secretName": "s"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/clusters", bytes.NewReader(body))
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestAPIClustersPost_InvalidJSON(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/clusters", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPIClustersPost_WithOptionalFields(t *testing.T) {
	fc := newFakeK8sClient()
	srv := httpserver.New(&fakeStore{}, fc, nil)

	body, _ := json.Marshal(map[string]string{
		"name":          "prod",
		"namespace":     "default",
		"secretName":    "prod-secret",
		"secretKey":     "kubeconfig",
		"prometheusURL": "http://prometheus.prod:9090",
		"description":   "Production cluster",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/clusters", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var cc v1alpha1.ClusterConfig
	require.NoError(t, fc.Get(context.Background(), client.ObjectKey{
		Name: "prod", Namespace: "default",
	}, &cc))
	assert.Equal(t, "http://prometheus.prod:9090", cc.Spec.PrometheusURL)
	assert.Equal(t, "Production cluster", cc.Spec.Description)
}

func TestAPIClusters_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/clusters", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// ── Prometheus metrics endpoint tests ──────────────────────────────────────────

func TestMetricsEndpoint(t *testing.T) {
	m := metrics.New()
	m.RecordRunCompleted("default", "Succeeded", "local")

	srv := httpserver.New(&fakeStore{}, nil, nil, httpserver.WithMetrics(m))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "kah_diagnostic_runs_total")
}

func TestMetricsEndpoint_NotRegisteredWithoutOption(t *testing.T) {
	// Without WithMetrics, /metrics should 404
	srv := httpserver.New(&fakeStore{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLLMMetrics_Success(t *testing.T) {
	m := metrics.New()
	srv := httpserver.New(&fakeStore{}, nil, nil, httpserver.WithMetrics(m))

	body, _ := json.Marshal(map[string]interface{}{
		"model":            "gpt-4",
		"duration_ms":      1200.0,
		"prompt_tokens":    500,
		"completion_tokens": 100,
		"status":           "ok",
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/llm-metrics", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify metrics were recorded
	families, err := m.Registry().Gather()
	require.NoError(t, err)
	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}
	assert.True(t, names["kah_llm_requests_total"])
	assert.True(t, names["kah_llm_request_duration_seconds"])
	assert.True(t, names["kah_llm_tokens_total"])
}

func TestLLMMetrics_WithoutMetrics(t *testing.T) {
	// Without metrics configured, the handler should still succeed (nil-safe)
	srv := httpserver.New(&fakeStore{}, nil, nil)

	body, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-4", "duration_ms": 100.0, "status": "ok",
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/llm-metrics", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestLLMMetrics_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/internal/llm-metrics", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestPostFindings_RecordsMetric(t *testing.T) {
	m := metrics.New()
	srv := httpserver.New(&fakeStore{}, nil, nil, httpserver.WithMetrics(m))

	body, _ := json.Marshal(map[string]interface{}{
		"dimension": "health", "severity": "critical",
		"title": "Pod crash", "resource_kind": "Pod",
		"resource_namespace": "default", "resource_name": "api-pod",
	})

	req := httptest.NewRequest(http.MethodPost, "/internal/runs/run-123/findings",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	// Verify findings metric was incremented
	families, err := m.Registry().Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == "kah_findings_total" {
			require.Len(t, f.GetMetric(), 1)
			assert.Equal(t, 1.0, f.GetMetric()[0].GetCounter().GetValue())
			return
		}
	}
	t.Fatal("kah_findings_total metric not found after creating a finding")
}

// ── GET /api/clusters with existing ClusterConfig CRs ──────────────────

func TestAPIClustersGet_WithClusterConfigs(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	cc := &v1alpha1.ClusterConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "prod", Namespace: "default"},
		Spec: v1alpha1.ClusterConfigSpec{
			KubeConfigRef: v1alpha1.SecretKeyRef{Name: "s", Key: "k"},
			PrometheusURL: "http://prom:9090",
			Description:   "Production",
		},
		Status: v1alpha1.ClusterConfigStatus{Phase: "Connected"},
	}
	fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cc).Build()
	srv := httpserver.New(&fakeStore{}, fc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var clusters []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&clusters))
	require.Len(t, clusters, 2, "should have 'local' + 'prod'")

	// Verify local is always first
	assert.Equal(t, "local", clusters[0]["name"])

	// Verify prod cluster has all fields
	assert.Equal(t, "prod", clusters[1]["name"])
	assert.Equal(t, "Connected", clusters[1]["phase"])
	assert.Equal(t, "http://prom:9090", clusters[1]["prometheusURL"])
	assert.Equal(t, "Production", clusters[1]["description"])
}

// ── Pagination, Filtering, and Batch Operation Tests ───────────────────

func TestPaginatedRunsList(t *testing.T) {
	fs := &fakeStore{}
	for i := 0; i < 25; i++ {
		fs.runs = append(fs.runs, &store.DiagnosticRun{
			ID:          fmt.Sprintf("run-%d", i),
			ClusterName: "local",
			TargetJSON:  "{}",
			SkillsJSON:  "[]",
			Status:      store.PhaseSucceeded,
		})
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	// Page 1, size 10
	req := httptest.NewRequest(http.MethodGet, "/api/runs?page=1&pageSize=10", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var result store.PaginatedResult[map[string]interface{}]
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Equal(t, 25, result.Total)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 10, result.PageSize)
	assert.Len(t, result.Items, 10)

	// Page 3, size 10 — should get 5 items
	req = httptest.NewRequest(http.MethodGet, "/api/runs?page=3&pageSize=10", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Equal(t, 25, result.Total)
	assert.Equal(t, 3, result.Page)
	assert.Len(t, result.Items, 5)
}

func TestPaginatedRunsFilter(t *testing.T) {
	fs := &fakeStore{}
	for i := 0; i < 10; i++ {
		status := store.PhaseSucceeded
		if i%2 == 0 {
			status = store.PhaseFailed
		}
		fs.runs = append(fs.runs, &store.DiagnosticRun{
			ID:          fmt.Sprintf("run-%d", i),
			ClusterName: "local",
			TargetJSON:  "{}",
			SkillsJSON:  "[]",
			Status:      status,
		})
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	// Filter by phase=Failed
	req := httptest.NewRequest(http.MethodGet, "/api/runs?page=1&pageSize=20&phase=Failed", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var result store.PaginatedResult[map[string]interface{}]
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Equal(t, 5, result.Total)
	assert.Len(t, result.Items, 5)
}

func TestPaginatedFixesList(t *testing.T) {
	fs := &fakeStore{}
	for i := 0; i < 15; i++ {
		fs.fixes = append(fs.fixes, &store.Fix{
			ID:          fmt.Sprintf("fix-%d", i),
			ClusterName: "local",
			RunID:       "run-1",
			Phase:       store.FixPhasePendingApproval,
		})
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/fixes?page=1&pageSize=10", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var result store.PaginatedResult[map[string]interface{}]
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Equal(t, 15, result.Total)
	assert.Len(t, result.Items, 10)
	assert.Equal(t, 1, result.Page)
}

func TestPaginatedEventsList(t *testing.T) {
	fs := &fakeStore{}
	for i := 0; i < 30; i++ {
		fs.events = append(fs.events, &store.Event{
			UID:         fmt.Sprintf("ev-%d", i),
			ClusterName: "local",
			Namespace:   "default",
			Kind:        "Pod",
			Name:        fmt.Sprintf("pod-%d", i),
			Reason:      "OOMKilled",
			Message:     "out of memory",
			Type:        "Warning",
			Count:       1,
		})
	}
	srv := httpserver.New(fs, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/events?page=2&pageSize=10", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var result store.PaginatedResult[map[string]interface{}]
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Equal(t, 30, result.Total)
	assert.Equal(t, 2, result.Page)
	assert.Len(t, result.Items, 10)
}

func TestDeleteRunsBatch(t *testing.T) {
	fs := &fakeStore{
		runs: []*store.DiagnosticRun{
			{ID: "r1", ClusterName: "local", Status: store.PhaseSucceeded},
			{ID: "r2", ClusterName: "local", Status: store.PhaseSucceeded},
			{ID: "r3", ClusterName: "local", Status: store.PhaseFailed},
		},
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	body, _ := json.Marshal(map[string]interface{}{"ids": []string{"r1", "r3"}})
	req := httptest.NewRequest(http.MethodDelete, "/api/runs/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, fs.runs, 1)
	assert.Equal(t, "r2", fs.runs[0].ID)
}

func TestBatchApproveFixes(t *testing.T) {
	fs := &fakeStore{
		fixes: []*store.Fix{
			{ID: "f1", Phase: store.FixPhasePendingApproval},
			{ID: "f2", Phase: store.FixPhasePendingApproval},
			{ID: "f3", Phase: store.FixPhasePendingApproval},
		},
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"ids":        []string{"f1", "f2"},
		"approvedBy": "admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/fixes/batch-approve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, float64(2), resp["approved"])
}

func TestBatchRejectFixes(t *testing.T) {
	fs := &fakeStore{
		fixes: []*store.Fix{
			{ID: "f1", Phase: store.FixPhasePendingApproval},
			{ID: "f2", Phase: store.FixPhasePendingApproval},
		},
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	body, _ := json.Marshal(map[string]interface{}{
		"ids": []string{"f1", "f2"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/fixes/batch-reject", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Verify both fixes were updated
	assert.Equal(t, store.FixPhaseFailed, fs.fixes[0].Phase)
	assert.Equal(t, store.FixPhaseFailed, fs.fixes[1].Phase)
}

func TestDeleteRunsBatch_EmptyIDs(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, newFakeK8sClient(), nil)

	body, _ := json.Marshal(map[string]interface{}{"ids": []string{}})
	req := httptest.NewRequest(http.MethodDelete, "/api/runs/batch", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBatchApprove_MethodNotAllowed(t *testing.T) {
	srv := httpserver.New(&fakeStore{}, newFakeK8sClient(), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/fixes/batch-approve", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestPaginatedRunsPageSizeCapped(t *testing.T) {
	fs := &fakeStore{}
	for i := 0; i < 5; i++ {
		fs.runs = append(fs.runs, &store.DiagnosticRun{
			ID: fmt.Sprintf("r%d", i), ClusterName: "local", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending,
		})
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	// pageSize > 100 should be capped
	req := httptest.NewRequest(http.MethodGet, "/api/runs?page=1&pageSize=200", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var result store.PaginatedResult[map[string]interface{}]
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	// fakeStore caps at 100 in its impl
	assert.LessOrEqual(t, result.PageSize, 100)
}

func TestLegacyRunsEndpoint_StillWorks(t *testing.T) {
	fs := &fakeStore{
		runs: []*store.DiagnosticRun{
			{ID: "r1", ClusterName: "local", TargetJSON: "{}", SkillsJSON: "[]", Status: store.PhasePending},
		},
	}
	srv := httpserver.New(fs, newFakeK8sClient(), nil)

	// Without page= param, should return legacy array response
	req := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Should decode as an array, not a paginated envelope
	var runs []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&runs))
	assert.Len(t, runs, 1)
}
