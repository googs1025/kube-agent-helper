package collector

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
	sqlitestore "github.com/kube-agent-helper/kube-agent-helper/internal/store/sqlite"
)

// newCollectorTestStore returns a real sqlite store backed by a tmpfile.
// Using the real store keeps the test honest about row-level behaviour.
func newCollectorTestStore(t *testing.T) store.Store {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "collector-*.db")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	s, err := sqlitestore.New(filepath.Clean(f.Name()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// ── Trivial helpers ──────────────────────────────────────────────────────────

func TestDefaultConfig_Values(t *testing.T) {
	c := DefaultConfig()
	assert.Equal(t, 100, c.EventBatchSize)
	assert.Equal(t, 5*time.Second, c.EventFlushPeriod)
	assert.Equal(t, time.Hour, c.PurgePeriod)
	assert.Equal(t, 7, c.RetentionDays)
}

func TestNeedLeaderElection_True(t *testing.T) {
	c := &Collector{}
	assert.True(t, c.NeedLeaderElection())
}

// ── runPurge ─────────────────────────────────────────────────────────────────

// purgeCounterStore wraps a real Store and counts Purge* calls so tests can
// observe the loop firing without needing tmpfile inspection.
type purgeCounterStore struct {
	store.Store
	purgeEvents  atomic.Int32
	purgeMetrics atomic.Int32
}

func (s *purgeCounterStore) PurgeOldEvents(ctx context.Context, before time.Time) error {
	s.purgeEvents.Add(1)
	return s.Store.PurgeOldEvents(ctx, before)
}

func (s *purgeCounterStore) PurgeOldMetrics(ctx context.Context, before time.Time) error {
	s.purgeMetrics.Add(1)
	return s.Store.PurgeOldMetrics(ctx, before)
}

func TestRunPurge_FiresOnTickAndExitsOnCancel(t *testing.T) {
	cs := &purgeCounterStore{Store: newCollectorTestStore(t)}
	c := &Collector{
		Config: Config{PurgePeriod: 5 * time.Millisecond, RetentionDays: 7},
		Store:  cs,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.runPurge(ctx)
		close(done)
	}()

	// Wait long enough for several ticks.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runPurge did not exit within 1s of ctx cancel")
	}

	assert.Greater(t, int(cs.purgeEvents.Load()), 0, "PurgeOldEvents should fire at least once")
	assert.Greater(t, int(cs.purgeMetrics.Load()), 0, "PurgeOldMetrics should fire at least once")
}

// ── watchEvents ──────────────────────────────────────────────────────────────

func TestWatchEvents_DrainsListedWarnings(t *testing.T) {
	now := time.Now()
	preExisting := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{UID: types.UID("ev-1"), Namespace: "default", Name: "pod.x.1"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "pod-x"},
		Reason:         "BackOff",
		Message:        "back-off restarting",
		Type:           "Warning",
		Count:          1,
		FirstTimestamp: metav1.NewTime(now),
		LastTimestamp:  metav1.NewTime(now),
	}

	clientset := fake.NewSimpleClientset(preExisting)

	c := &Collector{
		Config: DefaultConfig(),
		Client: clientset,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan *store.Event, 10)
	errCh := make(chan error, 1)
	go func() { errCh <- c.watchEvents(ctx, ch) }()

	select {
	case ev := <-ch:
		assert.Equal(t, "ev-1", ev.UID)
		assert.Equal(t, "BackOff", ev.Reason)
		assert.Equal(t, "Pod", ev.Kind)
	case <-time.After(time.Second):
		t.Fatal("expected pre-existing Warning event on channel within 1s")
	}

	cancel()
	select {
	case err := <-errCh:
		assert.NoError(t, err, "watchEvents should return nil on ctx cancel")
	case <-time.After(time.Second):
		t.Fatal("watchEvents did not return after cancel")
	}
}

func TestWatchEvents_HandlesAddedEventFromWatcher(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	// Inject a fake watcher we control.
	w := watch.NewFake()
	clientset.PrependWatchReactor("events", func(_ k8stesting.Action) (bool, watch.Interface, error) {
		return true, w, nil
	})

	c := &Collector{Config: DefaultConfig(), Client: clientset}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan *store.Event, 10)
	errCh := make(chan error, 1)
	go func() { errCh <- c.watchEvents(ctx, ch) }()

	now := time.Now()
	w.Add(&corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{UID: types.UID("ev-2"), Namespace: "kube-system"},
		InvolvedObject: corev1.ObjectReference{Kind: "Node", Name: "node-1"},
		Reason:         "NodeNotReady",
		Type:           "Warning",
		Count:          3,
		FirstTimestamp: metav1.NewTime(now),
		LastTimestamp:  metav1.NewTime(now),
	})

	select {
	case ev := <-ch:
		assert.Equal(t, "ev-2", ev.UID)
		assert.Equal(t, "NodeNotReady", ev.Reason)
	case <-time.After(time.Second):
		t.Fatal("expected watcher-injected event on channel within 1s")
	}

	cancel()
	<-errCh
}

func TestWatchEvents_ListErrorReturned(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, assertErr("list boom")
	})

	c := &Collector{Config: DefaultConfig(), Client: clientset}
	err := c.watchEvents(context.Background(), make(chan *store.Event, 1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list boom")
}

// assertErr is a tiny helper to build an error; keeps imports minimal.
type assertErr string

func (e assertErr) Error() string { return string(e) }

// ── scrapeAll ────────────────────────────────────────────────────────────────

// stubPromAPI implements prometheusv1.API and returns a configurable result for
// Query. All other methods panic — callers should only invoke Query.
type stubPromAPI struct {
	prometheusv1.API
	queryResult model.Value
	queryErr    error
	queries     []string
	mu          sync.Mutex
}

func (s *stubPromAPI) Query(_ context.Context, q string, _ time.Time, _ ...prometheusv1.Option) (model.Value, prometheusv1.Warnings, error) {
	s.mu.Lock()
	s.queries = append(s.queries, q)
	s.mu.Unlock()
	return s.queryResult, prometheusv1.Warnings{"sample warning"}, s.queryErr
}

func TestScrapeAll_StoresVectorSamples(t *testing.T) {
	s := newCollectorTestStore(t)
	c := &Collector{
		Config: Config{
			MetricsQueries: []string{`up{job="kubelet"}`},
		},
		Store: s,
	}

	ts := model.TimeFromUnix(time.Now().Unix())
	api := &stubPromAPI{
		queryResult: model.Vector{
			&model.Sample{
				Metric:    model.Metric{"__name__": "up", "job": "kubelet", "instance": "n1"},
				Value:     model.SampleValue(1),
				Timestamp: ts,
			},
			&model.Sample{
				Metric:    model.Metric{"__name__": "up", "job": "kubelet", "instance": "n2"},
				Value:     model.SampleValue(0),
				Timestamp: ts,
			},
		},
	}

	c.scrapeAll(context.Background(), api)

	// Query the snapshots back via store interface.
	snaps, err := s.QueryMetricHistory(context.Background(), `up{job="kubelet"}`, 60)
	require.NoError(t, err)
	assert.Len(t, snaps, 2)
}

func TestScrapeAll_SkipsNonVectorResult(t *testing.T) {
	s := newCollectorTestStore(t)
	c := &Collector{
		Config: Config{MetricsQueries: []string{`scalar_query`}},
		Store:  s,
	}
	api := &stubPromAPI{
		queryResult: &model.Scalar{Timestamp: model.TimeFromUnix(time.Now().Unix()), Value: 42},
	}
	c.scrapeAll(context.Background(), api)

	snaps, err := s.QueryMetricHistory(context.Background(), `scalar_query`, 60)
	require.NoError(t, err)
	assert.Empty(t, snaps, "non-Vector result should be silently skipped")
}

func TestScrapeAll_QueryErrorIsLoggedAndContinues(t *testing.T) {
	s := newCollectorTestStore(t)
	c := &Collector{
		Config: Config{MetricsQueries: []string{`bad_query`, `good_query`}},
		Store:  s,
	}
	ts := model.TimeFromUnix(time.Now().Unix())

	calls := 0
	api := &errThenSuccessStub{
		onQuery: func(q string) (model.Value, error) {
			calls++
			if q == "bad_query" {
				return nil, assertErr("query boom")
			}
			return model.Vector{
				&model.Sample{
					Metric:    model.Metric{"__name__": "good"},
					Value:     1,
					Timestamp: ts,
				},
			}, nil
		},
	}

	c.scrapeAll(context.Background(), api)

	snaps, err := s.QueryMetricHistory(context.Background(), `good_query`, 60)
	require.NoError(t, err)
	assert.Len(t, snaps, 1, "successful query should still record despite earlier failure")
	assert.Equal(t, 2, calls, "both queries must be attempted")
}

// errThenSuccessStub lets each query return a tailored value.
type errThenSuccessStub struct {
	prometheusv1.API
	onQuery func(q string) (model.Value, error)
}

func (s *errThenSuccessStub) Query(_ context.Context, q string, _ time.Time, _ ...prometheusv1.Option) (model.Value, prometheusv1.Warnings, error) {
	v, err := s.onQuery(q)
	return v, nil, err
}

func TestScrapeAll_TruncatesOversizedVector(t *testing.T) {
	s := newCollectorTestStore(t)
	c := &Collector{
		Config: Config{MetricsQueries: []string{`huge`}},
		Store:  s,
	}

	ts := model.TimeFromUnix(time.Now().Unix())
	vec := make(model.Vector, maxSeriesPerQuery+50)
	for i := range vec {
		vec[i] = &model.Sample{
			Metric:    model.Metric{"__name__": "x", "i": model.LabelValue(string(rune('a' + i%26)))},
			Value:     model.SampleValue(i),
			Timestamp: ts,
		}
	}
	api := &stubPromAPI{queryResult: vec}

	c.scrapeAll(context.Background(), api)

	snaps, err := s.QueryMetricHistory(context.Background(), `huge`, 60)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(snaps), maxSeriesPerQuery, "oversized vector must be truncated to maxSeriesPerQuery")
}

// ── Start (full launcher) ────────────────────────────────────────────────────

func TestCollector_Start_LaunchesAndExitsOnCancel(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	store := newCollectorTestStore(t)

	c := &Collector{
		Config: Config{
			PurgePeriod:      5 * time.Millisecond,
			RetentionDays:    7,
			EventBatchSize:   10,
			EventFlushPeriod: 100 * time.Millisecond,
			// PrometheusURL empty + no Queries → metric goroutine is skipped.
		},
		Store:  store,
		Client: clientset,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()

	time.Sleep(30 * time.Millisecond) // give goroutines time to spin up
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return within 2s of cancel")
	}
}

func TestCollector_Start_WithMetricsConfigSpawnsMetricGoroutine(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	store := newCollectorTestStore(t)

	c := &Collector{
		Config: Config{
			PurgePeriod:      time.Second,
			EventBatchSize:   10,
			EventFlushPeriod: 100 * time.Millisecond,
			// runMetricCollector branch — uses real prometheusapi.NewClient with
			// an unreachable address; since we cancel before any tick, no
			// network call is actually made.
			PrometheusURL:  "http://prom.invalid:9090",
			MetricsQueries: []string{`up`},
		},
		Store:  store,
		Client: clientset,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}
}
