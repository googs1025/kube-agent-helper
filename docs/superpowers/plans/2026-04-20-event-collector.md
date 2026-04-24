# Issue #8: EventCollector — Watch K8s Events + Prometheus 指标采集 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 controller 中持续 Watch K8s Warning 事件并定期 Scrape Prometheus 指标，存入 SQLite（7 天 TTL），新增 `events_history` 和 `metric_history` 两个 MCP 工具供 Agent 查询历史数据，Dashboard 新增 Events 历史页面。

**Architecture:** 新建 `internal/collector/` 包实现 `manager.Runnable`（Event Watch 使用 client-go ListWatch + 批量写入，Metric Scrape 每 15 分钟定时拉取）。Store 接口扩展 4 个方法（UpsertEvent、ListEvents、InsertMetricSnapshot、QueryMetricHistory）并在 SQLite 中实现。MCP `Deps` 增加 `Store` 字段，新增两个 MCP 工具文件。HTTP server 新增 `/api/events` 端点供 Dashboard 消费。

**Tech Stack:** `client-go` ListWatch（已有），`prometheus/client_golang` promv1.API（已有），`golang-migrate`（已有），`modernc.org/sqlite`（已有），Next.js SWR（dashboard 已有）。

---

## 工程风险与缓解

| 风险 | 缓解方案 |
|------|---------|
| Events 量过大 | 只 Watch `Warning` 类型；批量写入（buffer 100 或每 5 秒 flush） |
| Watch 重连 | client-go ListWatch + Reflector 内置重连；启动时先 List 再 Watch |
| UID 去重 | UPSERT ON CONFLICT(uid) DO UPDATE |
| Prometheus 不可用 | URL 为空时跳过整个 metric collector；失败只 warn 不 panic |
| PromQL 返回量大 | 每个 query 最多存 500 条 time series |
| 存储增长 | events + metric_snapshots 均做 7 天 TTL（每小时触发一次清理） |

---

## 文件变更清单

| 操作 | 文件 | 变更内容 |
|------|------|---------|
| Modify | `internal/store/store.go` | 新增 Event/MetricSnapshot 类型 + 4 个接口方法 |
| Create | `internal/store/sqlite/migrations/004_event_collector.up.sql` | events + metric_snapshots 表 |
| Create | `internal/store/sqlite/migrations/004_event_collector.down.sql` | DROP 两张表 |
| Modify | `internal/store/sqlite/sqlite.go` | 实现 4 个新方法 |
| Create | `internal/collector/collector.go` | Collector struct + Start()，实现 manager.Runnable |
| Create | `internal/collector/event_collector.go` | K8s Warning Events 的 ListWatch + batch writer |
| Create | `internal/collector/metric_collector.go` | Prometheus scraper，每 15 分钟执行 |
| Create | `internal/collector/collector_test.go` | 单元测试（批量写、去重、TTL 触发） |
| Modify | `internal/mcptools/deps.go` | Deps 增加 `Store store.Store` 字段 |
| Create | `internal/mcptools/events_history.go` | events_history MCP 工具 |
| Create | `internal/mcptools/events_history_test.go` | 单元测试 |
| Create | `internal/mcptools/metric_history.go` | metric_history MCP 工具 |
| Create | `internal/mcptools/metric_history_test.go` | 单元测试 |
| Modify | `internal/mcptools/register.go` | RegisterExtension 中注册 2 个新工具 |
| Modify | `internal/controller/httpserver/server.go` | 新增 `/api/events` 端点 |
| Modify | `cmd/controller/main.go` | 新增 flags + 初始化 Collector + 传 Store 给 Deps |
| Modify | `dashboard/src/lib/types.ts` | 新增 `KubeEvent` 类型 |
| Modify | `dashboard/src/lib/api.ts` | 新增 `useEvents` hook |
| Create | `dashboard/src/app/events/page.tsx` | Events 历史列表页 |
| Modify | `dashboard/src/app/layout.tsx` | Nav 增加 Events 链接 |
| Modify | `dashboard/src/i18n/zh.json` | 新增 events 翻译键 |
| Modify | `dashboard/src/i18n/en.json` | 新增 events 翻译键 |

---

### Task 1: Store 接口 + 数据类型

**Files:**
- Modify: `internal/store/store.go`

- [ ] **Step 1: 在 store.go 中增加 Event 和 MetricSnapshot 类型**

在 `Fix` 结构体之后，`ListOpts` 之前插入：

```go
type Event struct {
	ID        int64
	UID       string    // K8s Event.UID，去重用
	Namespace string
	Kind      string    // InvolvedObject.Kind
	Name      string    // InvolvedObject.Name
	Reason    string
	Message   string
	Type      string    // Warning | Normal
	Count     int32
	FirstTime time.Time
	LastTime  time.Time
	CreatedAt time.Time
}

type ListEventsOpts struct {
	Namespace string
	Name      string
	Type      string // "" = all, "Warning", "Normal"
	SinceMinutes int  // 0 = all
	Limit     int
}

type MetricSnapshot struct {
	ID         int64
	Query      string
	LabelsJSON string // JSON: {"namespace":"prod","pod":"api-xxx"}
	Value      float64
	Ts         time.Time
	CreatedAt  time.Time
}
```

- [ ] **Step 2: 在 Store 接口末尾（Close() 之前）增加 4 个方法**

```go
// Events (7-day retention)
UpsertEvent(ctx context.Context, e *Event) error
ListEvents(ctx context.Context, opts ListEventsOpts) ([]*Event, error)

// Metric snapshots
InsertMetricSnapshot(ctx context.Context, s *MetricSnapshot) error
QueryMetricHistory(ctx context.Context, query string, sinceMinutes int) ([]*MetricSnapshot, error)

// TTL cleanup
PurgeOldEvents(ctx context.Context, before time.Time) error
PurgeOldMetrics(ctx context.Context, before time.Time) error
```

- [ ] **Step 3: 验证编译（此时 SQLiteStore 会编译失败，因为未实现新方法）**

```bash
go build ./internal/store/... 2>&1 | head -10
```

Expected: 编译错误 `SQLiteStore does not implement Store`（正常，Task 3 中实现）

- [ ] **Step 4: Commit**

```bash
git add internal/store/store.go
git commit -m "feat(store): add Event/MetricSnapshot types and collector interface methods"
```

---

### Task 2: SQLite Migration 004

**Files:**
- Create: `internal/store/sqlite/migrations/004_event_collector.up.sql`
- Create: `internal/store/sqlite/migrations/004_event_collector.down.sql`

- [ ] **Step 1: 创建 up migration**

```sql
-- events 表：K8s Warning Events，7天保留
CREATE TABLE IF NOT EXISTS events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    uid        TEXT    NOT NULL UNIQUE,
    namespace  TEXT    NOT NULL DEFAULT '',
    kind       TEXT    NOT NULL DEFAULT '',
    name       TEXT    NOT NULL DEFAULT '',
    reason     TEXT    NOT NULL DEFAULT '',
    message    TEXT    NOT NULL DEFAULT '',
    type       TEXT    NOT NULL DEFAULT 'Warning',
    count      INTEGER NOT NULL DEFAULT 1,
    first_time INTEGER NOT NULL DEFAULT 0,  -- Unix seconds
    last_time  INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_events_namespace_name ON events(namespace, name);
CREATE INDEX IF NOT EXISTS idx_events_last_time      ON events(last_time);
CREATE INDEX IF NOT EXISTS idx_events_type           ON events(type);

-- metric_snapshots 表：Prometheus 指标快照，7天保留
CREATE TABLE IF NOT EXISTS metric_snapshots (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    query       TEXT    NOT NULL,
    labels_json TEXT    NOT NULL DEFAULT '{}',
    value       REAL    NOT NULL,
    ts          INTEGER NOT NULL,           -- Unix seconds
    created_at  INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_metrics_query_ts ON metric_snapshots(query, ts);
```

- [ ] **Step 2: 创建 down migration**

```sql
DROP TABLE IF EXISTS metric_snapshots;
DROP TABLE IF EXISTS events;
```

- [ ] **Step 3: Commit**

```bash
git add internal/store/sqlite/migrations/
git commit -m "feat(migration): add events and metric_snapshots tables (migration 004)"
```

---

### Task 3: SQLite Store 实现新方法

**Files:**
- Modify: `internal/store/sqlite/sqlite.go`

- [ ] **Step 1: 实现 UpsertEvent**

在 `sqlite.go` 末尾追加：

```go
func (s *SQLiteStore) UpsertEvent(ctx context.Context, e *Event) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO events (uid, namespace, kind, name, reason, message, type, count, first_time, last_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			count      = excluded.count,
			last_time  = excluded.last_time,
			message    = excluded.message`,
		e.UID, e.Namespace, e.Kind, e.Name, e.Reason, e.Message,
		e.Type, e.Count, e.FirstTime.Unix(), e.LastTime.Unix(),
	)
	return err
}
```

> 注意：`store.Event` 在 sqlite 包内通过 import 使用，这里省略包前缀是因为 sqlite.go 已经 import store。

实际代码中类型为 `*store.Event`：

```go
func (s *SQLiteStore) UpsertEvent(ctx context.Context, e *store.Event) error {
```

- [ ] **Step 2: 实现 ListEvents**

```go
func (s *SQLiteStore) ListEvents(ctx context.Context, opts store.ListEventsOpts) ([]*store.Event, error) {
	query := `SELECT id, uid, namespace, kind, name, reason, message, type, count, first_time, last_time, created_at
	          FROM events WHERE 1=1`
	args := []interface{}{}

	if opts.Namespace != "" {
		query += " AND namespace = ?"
		args = append(args, opts.Namespace)
	}
	if opts.Name != "" {
		query += " AND name = ?"
		args = append(args, opts.Name)
	}
	if opts.Type != "" {
		query += " AND type = ?"
		args = append(args, opts.Type)
	}
	if opts.SinceMinutes > 0 {
		cutoff := time.Now().Add(-time.Duration(opts.SinceMinutes) * time.Minute).Unix()
		query += " AND last_time >= ?"
		args = append(args, cutoff)
	}
	query += " ORDER BY last_time DESC"
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*store.Event
	for rows.Next() {
		var ev store.Event
		var firstTS, lastTS int64
		if err := rows.Scan(&ev.ID, &ev.UID, &ev.Namespace, &ev.Kind, &ev.Name,
			&ev.Reason, &ev.Message, &ev.Type, &ev.Count, &firstTS, &lastTS, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.FirstTime = time.Unix(firstTS, 0)
		ev.LastTime = time.Unix(lastTS, 0)
		events = append(events, &ev)
	}
	return events, rows.Err()
}
```

- [ ] **Step 3: 实现 InsertMetricSnapshot**

```go
func (s *SQLiteStore) InsertMetricSnapshot(ctx context.Context, snap *store.MetricSnapshot) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO metric_snapshots (query, labels_json, value, ts) VALUES (?, ?, ?, ?)`,
		snap.Query, snap.LabelsJSON, snap.Value, snap.Ts.Unix(),
	)
	return err
}
```

- [ ] **Step 4: 实现 QueryMetricHistory**

```go
func (s *SQLiteStore) QueryMetricHistory(ctx context.Context, query string, sinceMinutes int) ([]*store.MetricSnapshot, error) {
	cutoff := time.Now().Add(-time.Duration(sinceMinutes) * time.Minute).Unix()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, query, labels_json, value, ts, created_at
		 FROM metric_snapshots WHERE query = ? AND ts >= ?
		 ORDER BY ts DESC LIMIT 500`,
		query, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []*store.MetricSnapshot
	for rows.Next() {
		var snap store.MetricSnapshot
		var ts int64
		if err := rows.Scan(&snap.ID, &snap.Query, &snap.LabelsJSON, &snap.Value, &ts, &snap.CreatedAt); err != nil {
			return nil, err
		}
		snap.Ts = time.Unix(ts, 0)
		snaps = append(snaps, &snap)
	}
	return snaps, rows.Err()
}
```

- [ ] **Step 5: 实现 PurgeOldEvents 和 PurgeOldMetrics**

```go
func (s *SQLiteStore) PurgeOldEvents(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM events WHERE last_time < ?`, before.Unix())
	return err
}

func (s *SQLiteStore) PurgeOldMetrics(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM metric_snapshots WHERE ts < ?`, before.Unix())
	return err
}
```

- [ ] **Step 6: 验证编译**

```bash
go build ./internal/store/...
```

Expected: 无错误

- [ ] **Step 7: Commit**

```bash
git add internal/store/sqlite/sqlite.go
git commit -m "feat(store/sqlite): implement UpsertEvent, ListEvents, metric snapshot and purge methods"
```

---

### Task 4: Collector — Event Watch + Metric Scrape

**Files:**
- Create: `internal/collector/collector.go`
- Create: `internal/collector/event_collector.go`
- Create: `internal/collector/metric_collector.go`
- Create: `internal/collector/collector_test.go`

- [ ] **Step 1: 写失败测试**

创建 `internal/collector/collector_test.go`：

```go
package collector_test

import (
	"testing"
	"time"
)

// TestBatchFlush 验证 batch writer 在达到阈值时 flush
func TestBatchFlush(t *testing.T) {
	flushed := 0
	flush := func(items int) { flushed += items }

	b := newBatcher(3, 10*time.Second, flush)
	b.add(1)
	b.add(2)
	if flushed != 0 {
		t.Errorf("expected 0 flushes before threshold, got %d", flushed)
	}
	b.add(3) // 达到 batchSize=3，触发 flush
	if flushed != 3 {
		t.Errorf("expected flush of 3 items, got %d", flushed)
	}
}

// TestPurgeWindow 验证 7 天 TTL cutoff 计算
func TestPurgeWindow(t *testing.T) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	if cutoff.After(time.Now()) {
		t.Error("cutoff should be in the past")
	}
	if time.Since(cutoff) < 6*24*time.Hour {
		t.Error("cutoff should be ~7 days ago")
	}
}
```

- [ ] **Step 2: 运行测试（期望编译失败）**

```bash
go test ./internal/collector/... 2>&1 | head -10
```

Expected: 编译错误 `undefined: newBatcher`

- [ ] **Step 3: 创建 collector.go**

```go
package collector

import (
	"context"
	"log/slog"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

const (
	eventBatchSize    = 100
	eventFlushPeriod  = 5 * time.Second
	metricScrapeInterval = 15 * time.Minute
	ttlRetention      = 7 * 24 * time.Hour
	purgeInterval     = 1 * time.Hour
)

// Config holds all collector configuration.
type Config struct {
	Store          store.Store
	K8sTyped       kubernetes.Interface
	Prometheus     promv1.API    // nil = skip metric collection
	MetricsQueries []string      // PromQL expressions to scrape
	Logger         *slog.Logger
}

// Collector runs as a manager.Runnable and coordinates sub-collectors.
type Collector struct {
	cfg Config
}

func New(cfg Config) *Collector {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Collector{cfg: cfg}
}

func (c *Collector) Start(ctx context.Context) error {
	c.cfg.Logger.Info("collector starting")

	ec := &eventCollector{cfg: c.cfg}
	go ec.run(ctx)

	if c.cfg.Prometheus != nil && len(c.cfg.MetricsQueries) > 0 {
		mc := &metricCollector{cfg: c.cfg}
		go mc.run(ctx)
	}

	go c.runPurge(ctx)

	<-ctx.Done()
	c.cfg.Logger.Info("collector stopped")
	return nil
}

func (c *Collector) NeedLeaderElection() bool { return true }

func (c *Collector) runPurge(ctx context.Context) {
	ticker := time.NewTicker(purgeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-ttlRetention)
			if err := c.cfg.Store.PurgeOldEvents(ctx, cutoff); err != nil {
				c.cfg.Logger.Warn("purge events failed", "error", err)
			}
			if err := c.cfg.Store.PurgeOldMetrics(ctx, cutoff); err != nil {
				c.cfg.Logger.Warn("purge metrics failed", "error", err)
			}
		}
	}
}

// batcher accumulates items and flushes when batchSize or flushPeriod is reached.
type batcher struct {
	size    int
	period  time.Duration
	onFlush func(int)
	count   int
	timer   *time.Timer
}

func newBatcher(size int, period time.Duration, onFlush func(int)) *batcher {
	b := &batcher{size: size, period: period, onFlush: onFlush}
	b.timer = time.AfterFunc(period, b.flush)
	return b
}

func (b *batcher) add(n int) {
	b.count += n
	if b.count >= b.size {
		b.flush()
	}
}

func (b *batcher) flush() {
	if b.count == 0 {
		b.timer.Reset(b.period)
		return
	}
	n := b.count
	b.count = 0
	b.timer.Reset(b.period)
	b.onFlush(n)
}
```

- [ ] **Step 4: 创建 event_collector.go**

```go
package collector

import (
	"context"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type eventCollector struct {
	cfg Config
	mu  sync.Mutex
	buf []*store.Event
}

func (ec *eventCollector) run(ctx context.Context) {
	// Flush buffer periodically or when full
	flushFn := func(_ int) {
		ec.mu.Lock()
		items := ec.buf
		ec.buf = nil
		ec.mu.Unlock()
		for _, ev := range items {
			if err := ec.cfg.Store.UpsertEvent(ctx, ev); err != nil {
				ec.cfg.Logger.Warn("upsert event failed", "uid", ev.UID, "error", err)
			}
		}
	}
	b := newBatcher(eventBatchSize, eventFlushPeriod, flushFn)
	_ = b // batcher triggers via timer; enqueue calls b.add()

	// Initial List to catch existing events
	list, err := ec.cfg.K8sTyped.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
	})
	if err != nil {
		ec.cfg.Logger.Warn("initial event list failed", "error", err)
	} else {
		for i := range list.Items {
			ev := k8sEventToStore(&list.Items[i])
			ec.enqueue(ev, b)
		}
		// Force flush after initial list
		flushFn(0)
	}

	// Watch for new events
	resourceVersion := ""
	if list != nil {
		resourceVersion = list.ResourceVersion
	}

	for {
		select {
		case <-ctx.Done():
			flushFn(0)
			return
		default:
		}

		watcher, err := ec.cfg.K8sTyped.CoreV1().Events("").Watch(ctx, metav1.ListOptions{
			FieldSelector:   "type=Warning",
			ResourceVersion: resourceVersion,
		})
		if err != nil {
			ec.cfg.Logger.Warn("watch events failed, retrying", "error", err)
			continue
		}

		for ev := range watcher.ResultChan() {
			switch ev.Type {
			case watch.Added, watch.Modified:
				k8sEv, ok := ev.Object.(*corev1.Event)
				if !ok {
					continue
				}
				storeEv := k8sEventToStore(k8sEv)
				ec.enqueue(storeEv, b)
				resourceVersion = k8sEv.ResourceVersion
			case watch.Error:
				ec.cfg.Logger.Warn("watch event error", "object", ev.Object)
				// Reset resourceVersion to trigger re-list
				resourceVersion = ""
			}
		}
	}
}

func (ec *eventCollector) enqueue(ev *store.Event, b *batcher) {
	ec.mu.Lock()
	ec.buf = append(ec.buf, ev)
	ec.mu.Unlock()
	b.add(1)
}

func k8sEventToStore(ev *corev1.Event) *store.Event {
	return &store.Event{
		UID:       string(ev.UID),
		Namespace: ev.InvolvedObject.Namespace,
		Kind:      ev.InvolvedObject.Kind,
		Name:      ev.InvolvedObject.Name,
		Reason:    ev.Reason,
		Message:   ev.Message,
		Type:      ev.Type,
		Count:     ev.Count,
		FirstTime: ev.FirstTimestamp.Time,
		LastTime:  ev.LastTimestamp.Time,
	}
}
```

- [ ] **Step 5: 创建 metric_collector.go**

```go
package collector

import (
	"context"
	"encoding/json"
	"time"

	prommodel "github.com/prometheus/common/model"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type metricCollector struct {
	cfg Config
}

func (mc *metricCollector) run(ctx context.Context) {
	ticker := time.NewTicker(metricScrapeInterval)
	defer ticker.Stop()

	// Scrape once immediately on start
	mc.scrape(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mc.scrape(ctx)
		}
	}
}

func (mc *metricCollector) scrape(ctx context.Context) {
	scrapeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for _, q := range mc.cfg.MetricsQueries {
		if q == "" {
			continue
		}
		result, _, err := mc.cfg.Prometheus.Query(scrapeCtx, q, time.Now())
		if err != nil {
			mc.cfg.Logger.Warn("prometheus query failed", "query", q, "error", err)
			continue
		}

		vec, ok := result.(prommodel.Vector)
		if !ok {
			continue
		}

		// Limit to 500 time series per query
		limit := 500
		if len(vec) < limit {
			limit = len(vec)
		}
		for _, sample := range vec[:limit] {
			labels := make(map[string]string, len(sample.Metric))
			for k, v := range sample.Metric {
				labels[string(k)] = string(v)
			}
			labelsJSON, _ := json.Marshal(labels)

			snap := &store.MetricSnapshot{
				Query:      q,
				LabelsJSON: string(labelsJSON),
				Value:      float64(sample.Value),
				Ts:         sample.Timestamp.Time(),
			}
			if err := mc.cfg.Store.InsertMetricSnapshot(ctx, snap); err != nil {
				mc.cfg.Logger.Warn("insert metric snapshot failed", "error", err)
			}
		}
		mc.cfg.Logger.Info("scraped metric", "query", q, "series", limit)
	}
}
```

- [ ] **Step 6: 运行测试**

```bash
go test ./internal/collector/... -v
```

Expected: TestBatchFlush PASS, TestPurgeWindow PASS

- [ ] **Step 7: 验证编译**

```bash
go build ./internal/collector/...
```

Expected: 无错误

- [ ] **Step 8: Commit**

```bash
git add internal/collector/
git commit -m "feat(collector): add EventCollector and MetricCollector as manager.Runnable"
```

---

### Task 5: MCP 工具 — events_history 和 metric_history

**Files:**
- Modify: `internal/mcptools/deps.go`
- Create: `internal/mcptools/events_history.go`
- Create: `internal/mcptools/events_history_test.go`
- Create: `internal/mcptools/metric_history.go`
- Create: `internal/mcptools/metric_history_test.go`
- Modify: `internal/mcptools/register.go`

- [ ] **Step 1: Deps 增加 Store 字段**

在 `deps.go` 的 `Deps` struct 末尾（`Cluster string` 之后）加：

```go
Store store.Store // nil if collector not enabled
```

同时在顶部 import 中加：

```go
"github.com/kube-agent-helper/kube-agent-helper/internal/store"
```

- [ ] **Step 2: 写 events_history 测试**

创建 `internal/mcptools/events_history_test.go`：

```go
package mcptools_test

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/kube-agent-helper/kube-agent-helper/internal/mcptools"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type stubStore struct{ events []*store.Event }

func (s *stubStore) UpsertEvent(_ context.Context, _ *store.Event) error { return nil }
func (s *stubStore) ListEvents(_ context.Context, _ store.ListEventsOpts) ([]*store.Event, error) {
	return s.events, nil
}
func (s *stubStore) InsertMetricSnapshot(_ context.Context, _ *store.MetricSnapshot) error { return nil }
func (s *stubStore) QueryMetricHistory(_ context.Context, _ string, _ int) ([]*store.MetricSnapshot, error) {
	return nil, nil
}
func (s *stubStore) PurgeOldEvents(_ context.Context, _ time.Time) error  { return nil }
func (s *stubStore) PurgeOldMetrics(_ context.Context, _ time.Time) error { return nil }

// stubStore must also implement the full store.Store interface (runs, findings, etc.)
// For simplicity in tests only EventStore methods are needed; embed a nop base.
func (s *stubStore) CreateRun(_ context.Context, _ *store.DiagnosticRun) error   { return nil }
func (s *stubStore) GetRun(_ context.Context, _ string) (*store.DiagnosticRun, error) { return nil, store.ErrNotFound }
func (s *stubStore) UpdateRunStatus(_ context.Context, _, _ string, _ store.Phase) error { return nil }
func (s *stubStore) ListRuns(_ context.Context, _ store.ListOpts) ([]*store.DiagnosticRun, error) { return nil, nil }
func (s *stubStore) CreateFinding(_ context.Context, _ *store.Finding) error { return nil }
func (s *stubStore) ListFindings(_ context.Context, _ string) ([]*store.Finding, error) { return nil, nil }
func (s *stubStore) UpsertSkill(_ context.Context, _ *store.Skill) error { return nil }
func (s *stubStore) ListSkills(_ context.Context) ([]*store.Skill, error) { return nil, nil }
func (s *stubStore) GetSkill(_ context.Context, _ string) (*store.Skill, error) { return nil, store.ErrNotFound }
func (s *stubStore) DeleteSkill(_ context.Context, _ string) error { return nil }
func (s *stubStore) CreateFix(_ context.Context, _ *store.Fix) error { return nil }
func (s *stubStore) GetFix(_ context.Context, _ string) (*store.Fix, error) { return nil, store.ErrNotFound }
func (s *stubStore) ListFixes(_ context.Context, _ store.ListOpts) ([]*store.Fix, error) { return nil, nil }
func (s *stubStore) ListFixesByRun(_ context.Context, _ string) ([]*store.Fix, error) { return nil, nil }
func (s *stubStore) UpdateFixPhase(_ context.Context, _ string, _ store.FixPhase, _ string) error { return nil }
func (s *stubStore) UpdateFixApproval(_ context.Context, _ string, _ string) error { return nil }
func (s *stubStore) UpdateFixSnapshot(_ context.Context, _ string, _ string) error { return nil }
func (s *stubStore) Close() error { return nil }

func TestEventsHistoryHandler_returnsEvents(t *testing.T) {
	st := &stubStore{events: []*store.Event{
		{ID: 1, UID: "u1", Namespace: "prod", Kind: "Pod", Name: "api-xxx", Reason: "OOMKilled", Type: "Warning", Count: 3},
	}}
	d := &mcptools.Deps{Store: st}
	handler := mcptools.NewEventsHistoryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"namespace":     "prod",
		"since_minutes": float64(60),
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
}
```

> **注意**：`UpdateRunStatus` 方法签名须与 store.Store 接口完全一致（`phase store.Phase`）。

- [ ] **Step 3: 运行测试（期望编译失败）**

```bash
go test ./internal/mcptools/... -run TestEventsHistory 2>&1 | head -10
```

Expected: 编译错误 `undefined: mcptools.NewEventsHistoryHandler`

- [ ] **Step 4: 创建 events_history.go**

```go
package mcptools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

func NewEventsHistoryHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Store == nil {
			return mcp.NewToolResultError("event collector not enabled (--prometheus-url not set or collector disabled)"), nil
		}

		args, _ := req.Params.Arguments.(map[string]interface{})
		namespace, _ := args["namespace"].(string)
		name, _ := args["name"].(string)
		eventType, _ := args["event_type"].(string)
		if eventType == "" {
			eventType = "Warning"
		}
		sinceMinutes := 60
		if v, ok := args["since_minutes"].(float64); ok && v > 0 {
			sinceMinutes = int(v)
		}
		limit := 100
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}

		events, err := d.Store.ListEvents(ctx, store.ListEventsOpts{
			Namespace:    namespace,
			Name:         name,
			Type:         eventType,
			SinceMinutes: sinceMinutes,
			Limit:        limit,
		})
		if err != nil {
			return mcp.NewToolResultError("list events: " + err.Error()), nil
		}

		items := make([]map[string]interface{}, 0, len(events))
		for _, ev := range events {
			items = append(items, map[string]interface{}{
				"namespace": ev.Namespace,
				"kind":      ev.Kind,
				"name":      ev.Name,
				"reason":    ev.Reason,
				"message":   ev.Message,
				"type":      ev.Type,
				"count":     ev.Count,
				"firstTime": ev.FirstTime.Format("2006-01-02T15:04:05Z"),
				"lastTime":  ev.LastTime.Format("2006-01-02T15:04:05Z"),
			})
		}

		return jsonResult(map[string]interface{}{
			"count":  len(items),
			"events": items,
		})
	}
}
```

- [ ] **Step 5: 创建 metric_history.go**

```go
package mcptools

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

func NewMetricHistoryHandler(d *Deps) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.Store == nil {
			return mcp.NewToolResultError("metric collector not enabled"), nil
		}

		args, _ := req.Params.Arguments.(map[string]interface{})
		query, _ := args["query"].(string)
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}
		sinceMinutes := 60
		if v, ok := args["since_minutes"].(float64); ok && v > 0 {
			sinceMinutes = int(v)
		}

		snaps, err := d.Store.QueryMetricHistory(ctx, query, sinceMinutes)
		if err != nil {
			return mcp.NewToolResultError("query metric history: " + err.Error()), nil
		}

		items := make([]map[string]interface{}, 0, len(snaps))
		for _, s := range snaps {
			items = append(items, map[string]interface{}{
				"query":      s.Query,
				"labels":     s.LabelsJSON,
				"value":      s.Value,
				"timestamp":  s.Ts.Format("2006-01-02T15:04:05Z"),
			})
		}

		return jsonResult(map[string]interface{}{
			"count":     len(items),
			"snapshots": items,
		})
	}
}
```

- [ ] **Step 6: 写 metric_history_test.go**

```go
package mcptools_test

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/kube-agent-helper/kube-agent-helper/internal/mcptools"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type metricStubStore struct{ stubStore }

func (m *metricStubStore) QueryMetricHistory(_ context.Context, _ string, _ int) ([]*store.MetricSnapshot, error) {
	return []*store.MetricSnapshot{
		{Query: "up", LabelsJSON: `{"pod":"api"}`, Value: 1.0, Ts: time.Now()},
	}, nil
}

func TestMetricHistoryHandler_returnsSnapshots(t *testing.T) {
	d := &mcptools.Deps{Store: &metricStubStore{}}
	handler := mcptools.NewMetricHistoryHandler(d)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"query":         "up",
		"since_minutes": float64(60),
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %v", result.Content)
	}
}
```

- [ ] **Step 7: 在 register.go 的 RegisterExtension 末尾注册 2 个新工具**

```go
registerTool(s, d, mcp.NewTool("events_history",
    mcp.WithDescription("Query historical K8s Warning events from local store (7-day retention). Faster than live API for trend analysis."),
    mcp.WithString("namespace", mcp.Description("Filter by namespace (omit for all)")),
    mcp.WithString("name", mcp.Description("Filter by involvedObject name")),
    mcp.WithString("event_type", mcp.Description("Warning or Normal (default Warning)")),
    mcp.WithNumber("since_minutes", mcp.Description("Look back N minutes (default 60)")),
    mcp.WithNumber("limit", mcp.Description("Max results (default 100, max 500)")),
), []string{"namespace", "name", "event_type", "since_minutes", "limit"}, NewEventsHistoryHandler(d))

registerTool(s, d, mcp.NewTool("metric_history",
    mcp.WithDescription("Query historical Prometheus metric snapshots (scraped every 15min, 7-day retention)."),
    mcp.WithString("query", mcp.Required(), mcp.Description("PromQL expression (must match a previously scraped query)")),
    mcp.WithNumber("since_minutes", mcp.Description("Look back N minutes (default 60)")),
), []string{"query", "since_minutes"}, NewMetricHistoryHandler(d))
```

- [ ] **Step 8: 运行所有 MCP 测试**

```bash
go test ./internal/mcptools/... -v 2>&1 | tail -30
```

Expected: 全部 PASS

- [ ] **Step 9: Commit**

```bash
git add internal/mcptools/deps.go \
        internal/mcptools/events_history.go \
        internal/mcptools/events_history_test.go \
        internal/mcptools/metric_history.go \
        internal/mcptools/metric_history_test.go \
        internal/mcptools/register.go
git commit -m "feat(mcptools): add events_history and metric_history tools with Store dependency"
```

---

### Task 6: HTTP Server 新增 /api/events 端点

**Files:**
- Modify: `internal/controller/httpserver/server.go`

- [ ] **Step 1: 注册新路由**

在 `New()` 函数内 `srv.mux.HandleFunc("/api/k8s/resources", ...)` 之后加：

```go
srv.mux.HandleFunc("/api/events", srv.handleAPIEvents)
```

- [ ] **Step 2: 实现 handleAPIEvents**

在 server.go 末尾加：

```go
// GET /api/events?namespace=prod&since_minutes=60&limit=100
func (s *Server) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	namespace := q.Get("namespace")
	sinceMinutes := 60
	if v := q.Get("since_minutes"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sinceMinutes = n
		}
	}
	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	events, err := s.store.ListEvents(r.Context(), store.ListEventsOpts{
		Namespace:    namespace,
		Type:         "Warning",
		SinceMinutes: sinceMinutes,
		Limit:        limit,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type eventResp struct {
		Namespace string `json:"namespace"`
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Reason    string `json:"reason"`
		Message   string `json:"message"`
		Count     int32  `json:"count"`
		LastTime  string `json:"lastTime"`
	}
	resp := make([]eventResp, 0, len(events))
	for _, ev := range events {
		resp = append(resp, eventResp{
			Namespace: ev.Namespace,
			Kind:      ev.Kind,
			Name:      ev.Name,
			Reason:    ev.Reason,
			Message:   ev.Message,
			Count:     ev.Count,
			LastTime:  ev.LastTime.Format(time.RFC3339),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
```

同时在 import 中加 `"strconv"`（如果尚未存在）。

- [ ] **Step 3: 验证编译**

```bash
go build ./internal/controller/httpserver/...
```

Expected: 无错误

- [ ] **Step 4: Commit**

```bash
git add internal/controller/httpserver/server.go
git commit -m "feat(httpserver): add GET /api/events endpoint for event history"
```

---

### Task 7: main.go 注册 Collector + 传 Store 给 Deps

**Files:**
- Modify: `cmd/controller/main.go`

- [ ] **Step 1: 新增 flags**

在现有 flag 定义末尾加：

```go
var (
	// existing vars...
	prometheusURL  string
	metricsQueries string
)
```

在 `flag.Parse()` 之前加：

```go
flag.StringVar(&prometheusURL, "prometheus-url", "", "Prometheus URL for historical metric scraping (optional)")
flag.StringVar(&metricsQueries, "metrics-queries", "", "Comma-separated PromQL expressions to scrape every 15min")
```

- [ ] **Step 2: 初始化 Collector 并注册到 Manager**

在 `mgr.Add(&runnableHTTP{...})` 之后，`ctx, stop := ...` 之前加：

```go
// Start EventCollector + MetricCollector
var promAPI promv1.API
if prometheusURL != "" {
    promClient, err := api.NewClient(api.Config{Address: prometheusURL})
    if err != nil {
        slog.Warn("prometheus client init failed", "error", err)
    } else {
        promAPI = promv1.NewAPI(promClient)
    }
}

var queries []string
if metricsQueries != "" {
    for _, q := range strings.Split(metricsQueries, ",") {
        if q = strings.TrimSpace(q); q != "" {
            queries = append(queries, q)
        }
    }
}

col := collector.New(collector.Config{
    Store:          st,
    K8sTyped:       k8sTypedClient,  // see note below
    Prometheus:     promAPI,
    MetricsQueries: queries,
    Logger:         logger,
})
if err := mgr.Add(col); err != nil {
    slog.Error("add collector", "error", err)
    os.Exit(1)
}
```

> **注意**：`k8sTypedClient` 需要是 `kubernetes.Interface`。在 main.go 中使用 `kubernetes.NewForConfig(ctrl.GetConfigOrDie())` 创建，或使用已有的 k8sclient 包。查看现有 main.go 是否已有 typed client；若没有：
>
> ```go
> k8sTypedClient, err := kubernetes.NewForConfig(mgr.GetConfig())
> if err != nil {
>     slog.Error("k8s typed client", "error", err)
>     os.Exit(1)
> }
> ```
>
> 同时在 import 中加：
> ```go
> "k8s.io/client-go/kubernetes"
> promapi "github.com/prometheus/client_golang/api"
> promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
> "github.com/kube-agent-helper/kube-agent-helper/internal/collector"
> ```

- [ ] **Step 3: 验证编译**

```bash
go build ./cmd/controller/...
```

Expected: 无错误

- [ ] **Step 4: 运行所有测试**

```bash
go test ./... -race -count=1 -timeout=120s
```

Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/controller/main.go
git commit -m "feat(main): register Collector with EventCollector and MetricCollector"
```

---

### Task 8: Dashboard 前端 — Events 历史页

**Files:**
- Modify: `dashboard/src/lib/types.ts`
- Modify: `dashboard/src/lib/api.ts`
- Create: `dashboard/src/app/events/page.tsx`
- Modify: `dashboard/src/app/layout.tsx`
- Modify: `dashboard/src/i18n/zh.json`
- Modify: `dashboard/src/i18n/en.json`

- [ ] **Step 1: 在 types.ts 末尾加 KubeEvent 类型**

```ts
export interface KubeEvent {
  namespace: string;
  kind: string;
  name: string;
  reason: string;
  message: string;
  count: number;
  lastTime: string;
}
```

- [ ] **Step 2: 在 api.ts 末尾加 useEvents hook**

```ts
export function useEvents(namespace?: string, sinceMinutes = 60) {
  const url = `/api/events?since_minutes=${sinceMinutes}${namespace ? `&namespace=${namespace}` : ""}`;
  return useSWR<KubeEvent[]>(url, fetcher, { refreshInterval: 30000 });
}
```

- [ ] **Step 3: 更新 zh.json — 新增 events 翻译键**

在顶层 JSON 对象末尾（`"diagnose"` 节点之后）加：

```json
"events": {
  "title": "事件历史",
  "subtitle": "最近 7 天的 Warning 事件（每 30 秒刷新）",
  "namespace": "命名空间",
  "namespacePlaceholder": "全部",
  "col.namespace": "命名空间",
  "col.kind": "资源类型",
  "col.name": "资源名称",
  "col.reason": "原因",
  "col.message": "消息",
  "col.count": "次数",
  "col.lastTime": "最后发生",
  "empty": "暂无 Warning 事件",
  "loading": "加载中..."
}
```

在 `"nav"` 节点加 `"events": "事件"`。

- [ ] **Step 4: 更新 en.json — 新增 events 翻译键**

```json
"events": {
  "title": "Event History",
  "subtitle": "Warning events from the last 7 days (refreshes every 30s)",
  "namespace": "Namespace",
  "namespacePlaceholder": "All",
  "col.namespace": "Namespace",
  "col.kind": "Kind",
  "col.name": "Name",
  "col.reason": "Reason",
  "col.message": "Message",
  "col.count": "Count",
  "col.lastTime": "Last Seen",
  "empty": "No Warning events",
  "loading": "Loading..."
}
```

在 `"nav"` 节点加 `"events": "Events"`。

- [ ] **Step 5: 创建 dashboard/src/app/events/page.tsx**

```tsx
"use client";

import { useState } from "react";
import { useI18n } from "@/i18n/context";
import { useEvents } from "@/lib/api";
import { useK8sNamespaces } from "@/lib/api";

export default function EventsPage() {
  const { t } = useI18n();
  const [namespace, setNamespace] = useState("");
  const [sinceMinutes, setSinceMinutes] = useState(60);

  const { data: namespaces } = useK8sNamespaces();
  const { data: events, isLoading } = useEvents(namespace || undefined, sinceMinutes);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("events.title")}</h1>
        <p className="text-sm text-gray-500 mt-1">{t("events.subtitle")}</p>
      </div>

      <div className="flex gap-4">
        <div>
          <label className="block text-xs font-medium mb-1">{t("events.namespace")}</label>
          <select
            value={namespace}
            onChange={(e) => setNamespace(e.target.value)}
            className="rounded border px-3 py-1.5 text-sm dark:bg-gray-800 dark:border-gray-700"
          >
            <option value="">{t("events.namespacePlaceholder")}</option>
            {(namespaces || []).map((ns) => (
              <option key={ns.name} value={ns.name}>{ns.name}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="block text-xs font-medium mb-1">时间范围</label>
          <select
            value={sinceMinutes}
            onChange={(e) => setSinceMinutes(Number(e.target.value))}
            className="rounded border px-3 py-1.5 text-sm dark:bg-gray-800 dark:border-gray-700"
          >
            <option value={60}>1 小时</option>
            <option value={360}>6 小时</option>
            <option value={1440}>24 小时</option>
            <option value={10080}>7 天</option>
          </select>
        </div>
      </div>

      {isLoading && <p className="text-sm text-gray-500">{t("events.loading")}</p>}

      {!isLoading && (!events || events.length === 0) && (
        <p className="text-sm text-gray-500">{t("events.empty")}</p>
      )}

      {events && events.length > 0 && (
        <div className="overflow-x-auto rounded-lg border dark:border-gray-800">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800/50">
              <tr>
                {[
                  "col.namespace", "col.kind", "col.name",
                  "col.reason", "col.message", "col.count", "col.lastTime"
                ].map((key) => (
                  <th key={key} className="px-4 py-2 text-left font-medium text-gray-600 dark:text-gray-400">
                    {t(`events.${key}`)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y dark:divide-gray-800">
              {events.map((ev, i) => (
                <tr key={i} className="hover:bg-gray-50 dark:hover:bg-gray-800/30">
                  <td className="px-4 py-2 font-mono text-xs">{ev.namespace}</td>
                  <td className="px-4 py-2">{ev.kind}</td>
                  <td className="px-4 py-2 font-mono text-xs max-w-32 truncate" title={ev.name}>{ev.name}</td>
                  <td className="px-4 py-2">
                    <span className="rounded bg-orange-100 px-1.5 py-0.5 text-xs text-orange-700 dark:bg-orange-900/30 dark:text-orange-300">
                      {ev.reason}
                    </span>
                  </td>
                  <td className="px-4 py-2 text-xs text-gray-600 dark:text-gray-400 max-w-64 truncate" title={ev.message}>
                    {ev.message}
                  </td>
                  <td className="px-4 py-2 text-center">{ev.count}</td>
                  <td className="px-4 py-2 text-xs text-gray-500">
                    {new Date(ev.lastTime).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 6: 在 layout.tsx 的导航中加 Events 链接**

找到现有 nav 链接列表（通常是 `href="/runs"` 之类的 `<Link>` 列表），加入：

```tsx
<Link href="/events" className={...}>{t("nav.events")}</Link>
```

具体位置和 className 与现有 nav 链接保持一致。

- [ ] **Step 7: 验证前端编译**

```bash
cd dashboard && npm run build 2>&1 | tail -20
```

Expected: 无 TypeScript 错误，build 成功

- [ ] **Step 8: Commit**

```bash
git add dashboard/src/lib/types.ts \
        dashboard/src/lib/api.ts \
        dashboard/src/app/events/page.tsx \
        dashboard/src/app/layout.tsx \
        dashboard/src/i18n/zh.json \
        dashboard/src/i18n/en.json
git commit -m "feat(dashboard): add Events history page with namespace filter"
```

---

## Self-Review

**Spec coverage:**
- ✅ K8s Warning Events Watch（只 Watch Warning，降低写入量）
- ✅ 批量写入（buffer 100 or flush every 5s）
- ✅ UID 去重（UPSERT ON CONFLICT）
- ✅ List+Watch 启动模式（不丢历史）
- ✅ Prometheus scraping 可选（URL 为空时跳过）
- ✅ PromQL 每 query 最多 500 条 time series
- ✅ 7 天 TTL（hourly purge）
- ✅ MCP 工具：events_history + metric_history
- ✅ `/api/events` HTTP 端点
- ✅ Dashboard Events 历史页（namespace 过滤 + 时间范围选择）
- ✅ i18n zh/en

**已知限制（可接受）：**
- `events_history` MCP 工具依赖 collector 运行；collector 停止后数据仍然保留（只是停止更新）。
- metric_history 只能查询已配置的 PromQL（非任意 PromQL），预期行为。
- Watch 重连时若 `resourceVersion` 过期，会重新 List（短暂全量同步），已通过重置 `resourceVersion = ""` 处理。
- dashboard Events 页中时间范围文字目前写死为中文（`1 小时`），后续可提取为 i18n key。