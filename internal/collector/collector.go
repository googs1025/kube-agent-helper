// Package collector 控制器侧的后台数据采集器。
//
// 三个 goroutine（由 Start 启动）：
//  1. EventCollector  ─ Watch K8s Warning 事件，批量写入 Store.events 表
//  2. MetricCollector ─ 周期性 PromQL 查询，写入 Store.metric_snapshots 表
//  3. PurgeLoop       ─ 每小时清理超过 RetentionDays（默认 7 天）的旧记录
//
// 设计要点：
//   - 实现 controller-runtime 的 manager.Runnable 接口，与 Reconciler 共享生命周期
//   - NeedLeaderElection() = true，HA 部署时只有 leader pod 在采集，避免重复
//   - 与 Reconciler 解耦：只通过 Store 接口写数据，不感知 CR / Job
package collector

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kube-agent-helper/kube-agent-helper/internal/metrics"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// Config holds collector configuration.
type Config struct {
	PrometheusURL    string
	MetricsQueries   []string
	EventBatchSize   int
	EventFlushPeriod time.Duration
	PurgePeriod      time.Duration
	RetentionDays    int
}

func DefaultConfig() Config {
	return Config{
		EventBatchSize:   100,
		EventFlushPeriod: 5 * time.Second,
		PurgePeriod:      time.Hour,
		RetentionDays:    7,
	}
}

// Collector implements manager.Runnable and coordinates event + metric collection.
type Collector struct {
	Config  Config
	Store   store.Store
	Client  kubernetes.Interface
	Metrics *metrics.Metrics // nil-safe
}

// NeedLeaderElection returns true so only the leader pod runs the collector.
func (c *Collector) NeedLeaderElection() bool { return true }

// Start begins event and metric collection. Blocks until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("collector")
	logger.Info("starting collector")

	go c.runEventCollector(ctx)

	if c.Config.PrometheusURL != "" && len(c.Config.MetricsQueries) > 0 {
		go c.runMetricCollector(ctx)
	}

	go c.runPurge(ctx)

	<-ctx.Done()
	logger.Info("collector stopped")
	return nil
}

// runPurge periodically deletes records older than RetentionDays.
func (c *Collector) runPurge(ctx context.Context) {
	ticker := time.NewTicker(c.Config.PurgePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().AddDate(0, 0, -c.Config.RetentionDays)
			logger := log.FromContext(ctx).WithName("collector")
			if err := c.Store.PurgeOldEvents(ctx, cutoff); err != nil {
				logger.Error(err, "purge old events failed")
			}
			if err := c.Store.PurgeOldMetrics(ctx, cutoff); err != nil {
				logger.Error(err, "purge old metrics failed")
			}
		}
	}
}

// batcher accumulates items and flushes when cap is reached or ticker fires.
// The flush function receives the accumulated slice.
func runBatcher[T any](ctx context.Context, in <-chan T, batchSize int, flushPeriod time.Duration, flush func([]T)) {
	buf := make([]T, 0, batchSize)
	ticker := time.NewTicker(flushPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if len(buf) > 0 {
				flush(buf)
			}
			return
		case item, ok := <-in:
			if !ok {
				if len(buf) > 0 {
					flush(buf)
				}
				return
			}
			buf = append(buf, item)
			if len(buf) >= batchSize {
				flush(buf)
				buf = buf[:0]
			}
		case <-ticker.C:
			if len(buf) > 0 {
				flush(buf)
				buf = buf[:0]
			}
		}
	}
}