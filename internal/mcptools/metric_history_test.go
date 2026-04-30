package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// nopStore implements store.Store with no-op stubs.
type nopStore struct{}

func (nopStore) CreateRun(_ context.Context, _ *store.DiagnosticRun) error { return nil }
func (nopStore) GetRun(_ context.Context, _ string) (*store.DiagnosticRun, error) {
	return nil, nil
}
func (nopStore) UpdateRunStatus(_ context.Context, _ string, _ store.Phase, _ string) error {
	return nil
}
func (nopStore) ListRuns(_ context.Context, _ store.ListOpts) ([]*store.DiagnosticRun, error) {
	return nil, nil
}
func (nopStore) CreateFinding(_ context.Context, _ *store.Finding) error   { return nil }
func (nopStore) ListFindings(_ context.Context, _ string) ([]*store.Finding, error) {
	return nil, nil
}
func (nopStore) UpsertSkill(_ context.Context, _ *store.Skill) error  { return nil }
func (nopStore) ListSkills(_ context.Context) ([]*store.Skill, error) { return nil, nil }
func (nopStore) GetSkill(_ context.Context, _ string) (*store.Skill, error) {
	return nil, nil
}
func (nopStore) DeleteSkill(_ context.Context, _ string) error { return nil }
func (nopStore) CreateFix(_ context.Context, _ *store.Fix) error { return nil }
func (nopStore) GetFix(_ context.Context, _ string) (*store.Fix, error) { return nil, nil }
func (nopStore) ListFixes(_ context.Context, _ store.ListOpts) ([]*store.Fix, error) {
	return nil, nil
}
func (nopStore) ListFixesByRun(_ context.Context, _ string) ([]*store.Fix, error) {
	return nil, nil
}
func (nopStore) UpdateFixPhase(_ context.Context, _ string, _ store.FixPhase, _ string) error {
	return nil
}
func (nopStore) UpdateFixApproval(_ context.Context, _ string, _ string) error { return nil }
func (nopStore) UpdateFixSnapshot(_ context.Context, _ string, _ string) error { return nil }
func (nopStore) UpsertEvent(_ context.Context, _ *store.Event) error           { return nil }
func (nopStore) ListEvents(_ context.Context, _ store.ListEventsOpts) ([]*store.Event, error) {
	return nil, nil
}
func (nopStore) InsertMetricSnapshot(_ context.Context, _ *store.MetricSnapshot) error { return nil }
func (nopStore) QueryMetricHistory(_ context.Context, _ string, _ int) ([]*store.MetricSnapshot, error) {
	return nil, nil
}
func (nopStore) ListRunsPaginated(_ context.Context, _ store.ListOpts) (store.PaginatedResult[*store.DiagnosticRun], error) {
	return store.PaginatedResult[*store.DiagnosticRun]{}, nil
}
func (nopStore) ListFixesPaginated(_ context.Context, _ store.ListOpts) (store.PaginatedResult[*store.Fix], error) {
	return store.PaginatedResult[*store.Fix]{}, nil
}
func (nopStore) ListEventsPaginated(_ context.Context, _ store.ListEventsOpts, _, _ int) (store.PaginatedResult[*store.Event], error) {
	return store.PaginatedResult[*store.Event]{}, nil
}
func (nopStore) DeleteRuns(_ context.Context, _ []string) error { return nil }
func (nopStore) BatchUpdateFixPhase(_ context.Context, _ []string, _ store.FixPhase, _ string) error {
	return nil
}
func (nopStore) AppendRunLog(_ context.Context, _ store.RunLog) error { return nil }
func (nopStore) ListRunLogs(_ context.Context, _ string, _ int64) ([]store.RunLog, error) {
	return nil, nil
}
func (nopStore) ListNotificationConfigs(_ context.Context) ([]*store.NotificationConfig, error) {
	return nil, nil
}
func (nopStore) GetNotificationConfig(_ context.Context, _ string) (*store.NotificationConfig, error) {
	return nil, nil
}
func (nopStore) CreateNotificationConfig(_ context.Context, _ *store.NotificationConfig) error {
	return nil
}
func (nopStore) UpdateNotificationConfig(_ context.Context, _ *store.NotificationConfig) error {
	return nil
}
func (nopStore) DeleteNotificationConfig(_ context.Context, _ string) error   { return nil }
func (nopStore) PurgeOldEvents(_ context.Context, _ time.Time) error          { return nil }
func (nopStore) PurgeOldMetrics(_ context.Context, _ time.Time) error         { return nil }
func (nopStore) Close() error                                                  { return nil }

// mockMetricStore overrides QueryMetricHistory on top of nopStore.
type mockMetricStore struct {
	nopStore
	snaps        []*store.MetricSnapshot
	err          error
	capturedMins int
}

func (m *mockMetricStore) QueryMetricHistory(_ context.Context, _ string, sinceMinutes int) ([]*store.MetricSnapshot, error) {
	m.capturedMins = sinceMinutes
	return m.snaps, m.err
}

// --- Tests ---

func TestMetricHistory_NoStore(t *testing.T) {
	d := &Deps{Store: nil}
	handler := NewMetricHistoryHandler(d)

	req := mcp.CallToolRequest{}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, false, payload["available"])
	assert.Equal(t, "metric store not available", payload["error"])
}

func TestMetricHistory_QueryRequired(t *testing.T) {
	d := &Deps{Store: &mockMetricStore{}}
	handler := NewMetricHistoryHandler(d)

	req := mcp.CallToolRequest{}
	// No "query" argument set.
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, "query is required", payload["error"])
}

func TestMetricHistory_DefaultSinceMinutes(t *testing.T) {
	snap := &store.MetricSnapshot{ID: 1, Query: "up", Value: 1.0, Ts: time.Now()}
	ms := &mockMetricStore{snaps: []*store.MetricSnapshot{snap}}
	d := &Deps{Store: ms}
	handler := NewMetricHistoryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"query": "up",
		// since_minutes intentionally omitted → should default to 60
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	assert.Equal(t, 60, ms.capturedMins, "expected default since_minutes=60")

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, float64(1), payload["count"])
}

func TestMetricHistory_CustomSinceMinutes(t *testing.T) {
	ms := &mockMetricStore{snaps: []*store.MetricSnapshot{}}
	d := &Deps{Store: ms}
	handler := NewMetricHistoryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"query":         "node_cpu_seconds_total",
		"since_minutes": float64(30),
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	assert.Equal(t, 30, ms.capturedMins, "expected since_minutes=30")

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, float64(0), payload["count"])
}

func TestMetricHistory_StoreError(t *testing.T) {
	storeErr := errors.New("database unavailable")
	ms := &mockMetricStore{err: storeErr}
	d := &Deps{Store: ms}
	handler := NewMetricHistoryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"query": "up",
	}
	result, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.IsError)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(textOf(result)), &payload))
	assert.Equal(t, "database unavailable", payload["error"])
}
