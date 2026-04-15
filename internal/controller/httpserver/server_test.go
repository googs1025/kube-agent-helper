package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/controller/httpserver"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type fakeStore struct {
	findings []*store.Finding
	skills   []*store.Skill
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
func (f *fakeStore) CreateRun(_ context.Context, r *store.DiagnosticRun) error { return nil }
func (f *fakeStore) GetRun(_ context.Context, id string) (*store.DiagnosticRun, error) {
	return nil, nil
}
func (f *fakeStore) UpdateRunStatus(_ context.Context, id string, p store.Phase, msg string) error {
	return nil
}
func (f *fakeStore) ListRuns(_ context.Context, opts store.ListOpts) ([]*store.DiagnosticRun, error) {
	return nil, nil
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
func (f *fakeStore) DeleteSkill(_ context.Context, _ string) error                          { return nil }
func (f *fakeStore) CreateFix(_ context.Context, _ *store.Fix) error                        { return nil }
func (f *fakeStore) GetFix(_ context.Context, _ string) (*store.Fix, error)                 { return nil, store.ErrNotFound }
func (f *fakeStore) ListFixes(_ context.Context, _ store.ListOpts) ([]*store.Fix, error)    { return nil, nil }
func (f *fakeStore) ListFixesByRun(_ context.Context, _ string) ([]*store.Fix, error)       { return nil, nil }
func (f *fakeStore) UpdateFixPhase(_ context.Context, _ string, _ store.FixPhase, _ string) error { return nil }
func (f *fakeStore) UpdateFixApproval(_ context.Context, _ string, _ string) error          { return nil }
func (f *fakeStore) UpdateFixSnapshot(_ context.Context, _ string, _ string) error          { return nil }
func (f *fakeStore) Close() error                                                           { return nil }

func TestPostFindings(t *testing.T) {
	fs := &fakeStore{}
	srv := httpserver.New(fs)

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
	srv := httpserver.New(fs)

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

	srv := httpserver.New(fs)
	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var skills []*store.Skill
	require.NoError(t, json.NewDecoder(w.Body).Decode(&skills))
	require.Len(t, skills, 1)
	assert.Equal(t, "s1", skills[0].Name)
}
