# Issue #10: 周期诊断 — DiagnosticRun.schedule cron 支持 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `DiagnosticRun` CRD 增加 `spec.schedule` cron 字段，通过新的 `ScheduledRunReconciler` 按计划自动创建子 `DiagnosticRun`，并保留最近 N 次历史（`spec.historyLimit`）。

**Architecture:** `DiagnosticRun`（有 schedule）作为模板，`ScheduledRunReconciler` 解析 cron，到时间创建子 `DiagnosticRun`（无 schedule，有 OwnerReference）。子 run 由现有 `DiagnosticRunReconciler` 处理。现有 reconciler 增加跳过 scheduled 父 run 的逻辑。

**Tech Stack:** `github.com/robfig/cron/v3` cron 解析，controller-runtime reconciler，sigs.k8s.io/controller-runtime client，已有 go.mod 中的所有依赖。

---

## 文件变更清单

| 操作 | 文件 | 变更内容 |
|------|------|---------|
| Modify | `internal/controller/api/v1alpha1/types.go` | DiagnosticRunSpec 增加 Schedule/HistoryLimit，Status 增加 LastRunAt/NextRunAt/ActiveRuns |
| Modify | `internal/controller/api/v1alpha1/zz_generated.deepcopy.go` | 手动更新 DeepCopyInto 处理新的指针和切片字段 |
| Modify | `internal/controller/reconciler/run_reconciler.go` | 开头增加跳过 scheduled run 的判断 |
| Create | `internal/controller/reconciler/scheduled_run_reconciler.go` | 新 reconciler：解析 cron，创建子 run，维护 historyLimit |
| Create | `internal/controller/reconciler/scheduled_run_reconciler_test.go` | 单元测试 |
| Modify | `cmd/controller/main.go` | 注册 ScheduledRunReconciler |

---

### Task 1: 更新 CRD types

**Files:**
- Modify: `internal/controller/api/v1alpha1/types.go`
- Modify: `internal/controller/api/v1alpha1/zz_generated.deepcopy.go`

- [ ] **Step 1: 在 DiagnosticRunSpec 末尾增加两个新字段**

在 `types.go` 中，`DiagnosticRunSpec` 结构体的 `OutputLanguage` 字段后面加：

```go
// Schedule is a cron expression for periodic runs, e.g. "0 * * * *".
// When set, this DiagnosticRun acts as a template; child runs are created automatically.
// +optional
Schedule string `json:"schedule,omitempty"`
// HistoryLimit is the maximum number of completed child runs to retain.
// +optional
// +kubebuilder:default=10
HistoryLimit *int32 `json:"historyLimit,omitempty"`
```

- [ ] **Step 2: 在 DiagnosticRunStatus 末尾增加三个新字段**

在 `types.go` 中，`Findings []FindingSummary` 字段后面加：

```go
// LastRunAt is the time the last child run was created (only set when schedule is used).
// +optional
LastRunAt *metav1.Time `json:"lastRunAt,omitempty"`
// NextRunAt is the scheduled time for the next child run.
// +optional
NextRunAt *metav1.Time `json:"nextRunAt,omitempty"`
// ActiveRuns lists the names of child DiagnosticRuns created by this scheduled run.
// +optional
ActiveRuns []string `json:"activeRuns,omitempty"`
```

- [ ] **Step 3: 在 printcolumn 增加 NextRun 列**

将 `DiagnosticRun` 的 kubebuilder marker 改为：

```go
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="NextRun",type=date,JSONPath=`.status.nextRunAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
```

- [ ] **Step 4: 更新 zz_generated.deepcopy.go 中 DiagnosticRunSpec.DeepCopyInto**

将现有的 `DiagnosticRunSpec.DeepCopyInto` 函数替换为：

```go
func (in *DiagnosticRunSpec) DeepCopyInto(out *DiagnosticRunSpec) {
	*out = *in
	in.Target.DeepCopyInto(&out.Target)
	if in.Skills != nil {
		in, out := &in.Skills, &out.Skills
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.TimeoutSeconds != nil {
		in, out := &in.TimeoutSeconds, &out.TimeoutSeconds
		*out = new(int32)
		**out = **in
	}
	if in.HistoryLimit != nil {
		in, out := &in.HistoryLimit, &out.HistoryLimit
		*out = new(int32)
		**out = **in
	}
}
```

- [ ] **Step 5: 更新 zz_generated.deepcopy.go 中 DiagnosticRunStatus.DeepCopyInto**

将现有的 `DiagnosticRunStatus.DeepCopyInto` 函数替换为：

```go
func (in *DiagnosticRunStatus) DeepCopyInto(out *DiagnosticRunStatus) {
	*out = *in
	if in.StartedAt != nil {
		in, out := &in.StartedAt, &out.StartedAt
		*out = (*in).DeepCopy()
	}
	if in.CompletedAt != nil {
		in, out := &in.CompletedAt, &out.CompletedAt
		*out = (*in).DeepCopy()
	}
	if in.FindingCounts != nil {
		in, out := &in.FindingCounts, &out.FindingCounts
		*out = make(map[string]int, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Findings != nil {
		in, out := &in.Findings, &out.Findings
		*out = make([]FindingSummary, len(*in))
		copy(*out, *in)
	}
	if in.LastRunAt != nil {
		in, out := &in.LastRunAt, &out.LastRunAt
		*out = (*in).DeepCopy()
	}
	if in.NextRunAt != nil {
		in, out := &in.NextRunAt, &out.NextRunAt
		*out = (*in).DeepCopy()
	}
	if in.ActiveRuns != nil {
		in, out := &in.ActiveRuns, &out.ActiveRuns
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}
```

- [ ] **Step 6: 验证编译**

```bash
go build ./internal/controller/api/...
```

Expected: 无错误

- [ ] **Step 7: Commit**

```bash
git add internal/controller/api/v1alpha1/types.go internal/controller/api/v1alpha1/zz_generated.deepcopy.go
git commit -m "feat(crd): add schedule/historyLimit to DiagnosticRunSpec and scheduled status fields"
```

---

### Task 2: 更新 run_reconciler 跳过 scheduled run

**Files:**
- Modify: `internal/controller/reconciler/run_reconciler.go`

- [ ] **Step 1: 在 Reconcile 函数开头的 terminal 判断后插入跳过逻辑**

在 `run_reconciler.go` 的 Reconcile 函数中，找到：

```go
// Already terminal — nothing to do.
if run.Status.Phase == string(store.PhaseSucceeded) || run.Status.Phase == string(store.PhaseFailed) {
    return ctrl.Result{}, nil
}
```

在其后插入：

```go
// Scheduled template run — managed by ScheduledRunReconciler, not here.
if run.Spec.Schedule != "" {
    return ctrl.Result{}, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./internal/controller/reconciler/...
```

Expected: 无错误

- [ ] **Step 3: Commit**

```bash
git add internal/controller/reconciler/run_reconciler.go
git commit -m "feat(reconciler): skip scheduled template runs in DiagnosticRunReconciler"
```

---

### Task 3: 实现 ScheduledRunReconciler

**Files:**
- Create: `internal/controller/reconciler/scheduled_run_reconciler.go`
- Create: `internal/controller/reconciler/scheduled_run_reconciler_test.go`

- [ ] **Step 1: 写失败测试**

创建 `internal/controller/reconciler/scheduled_run_reconciler_test.go`：

```go
package reconciler_test

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

// parseCronNext 验证 cron 表达式解析和下次触发时间计算
func TestParseCronNext(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"0 * * * *", false},    // 每小时
		{"*/15 * * * *", false}, // 每15分钟
		{"0 0 * * *", false},    // 每天午夜
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		_, err := parser.Parse(tt.expr)
		if (err != nil) != tt.wantErr {
			t.Errorf("Parse(%q) error=%v, wantErr=%v", tt.expr, err, tt.wantErr)
		}
	}
}

// TestNextRunAfterNow 验证到期判断逻辑
func TestNextRunAfterNow(t *testing.T) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, _ := parser.Parse("0 * * * *")

	now := time.Now().Truncate(time.Hour).Add(time.Hour) // 下一个整点
	next := sched.Next(now.Add(-2 * time.Hour))           // 从两小时前算起，下一个应该早于 now

	if !next.Before(now.Add(time.Second)) {
		t.Errorf("expected next run before now, got %v (now=%v)", next, now)
	}
}

// TestChildRunName 验证子 run 名称格式
func TestChildRunName(t *testing.T) {
	parent := "my-diagnostic-run"
	ts := time.Unix(1745123456, 0)
	name := childRunName(parent, ts)
	if name == "" {
		t.Fatal("childRunName returned empty string")
	}
	// 名称不超过 DNS 限制
	if len(name) > 253 {
		t.Errorf("childRunName too long: %d chars", len(name))
	}
}
```

- [ ] **Step 2: 运行测试（期望编译失败，因为 childRunName 未定义）**

```bash
go test ./internal/controller/reconciler/... 2>&1 | head -20
```

Expected: 编译错误 `undefined: childRunName`

- [ ] **Step 3: 创建 scheduled_run_reconciler.go**

```go
package reconciler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
)

const (
	scheduledByLabel    = "kube-agent-helper.io/scheduled-by"
	defaultHistoryLimit = int32(10)
)

// cronParser is the standard 5-field cron parser (minute hour dom month dow).
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

type ScheduledRunReconciler struct {
	client.Client
}

func (r *ScheduledRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var run k8saiV1.DiagnosticRun
	if err := r.Get(ctx, req.NamespacedName, &run); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Only handle scheduled template runs.
	if run.Spec.Schedule == "" {
		return ctrl.Result{}, nil
	}

	sched, err := cronParser.Parse(run.Spec.Schedule)
	if err != nil {
		logger.Error(err, "invalid cron expression", "schedule", run.Spec.Schedule)
		// Don't requeue — user must fix the schedule.
		return ctrl.Result{}, nil
	}

	now := time.Now().UTC()

	// Initialize nextRunAt on first reconcile.
	if run.Status.NextRunAt == nil {
		next := sched.Next(now)
		run.Status.NextRunAt = &metav1.Time{Time: next}
		if err := r.Status().Update(ctx, &run); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("scheduled run initialized", "nextRunAt", next)
		return ctrl.Result{RequeueAfter: time.Until(next)}, nil
	}

	nextRunAt := run.Status.NextRunAt.Time

	// Not yet time to trigger.
	if now.Before(nextRunAt) {
		return ctrl.Result{RequeueAfter: time.Until(nextRunAt)}, nil
	}

	// Time to create a child run.
	childName := childRunName(run.Name, nextRunAt)

	// Idempotency: check if child already exists.
	var existing k8saiV1.DiagnosticRun
	if err := r.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: childName}, &existing); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}

		// Create child run — copy spec without schedule/historyLimit.
		child := r.buildChildRun(&run, childName, nextRunAt)
		if err := r.Create(ctx, child); err != nil {
			return ctrl.Result{}, fmt.Errorf("create child run: %w", err)
		}
		logger.Info("created child run", "name", childName)
	}

	// Update status.
	next := sched.Next(nextRunAt)
	run.Status.LastRunAt = &metav1.Time{Time: nextRunAt}
	run.Status.NextRunAt = &metav1.Time{Time: next}
	run.Status.ActiveRuns = appendUnique(run.Status.ActiveRuns, childName)

	// Enforce historyLimit: delete oldest runs exceeding the limit.
	if err := r.enforceHistoryLimit(ctx, &run); err != nil {
		logger.Error(err, "enforce history limit")
		// Non-fatal — still update status.
	}

	if err := r.Status().Update(ctx, &run); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Until(next)}, nil
}

func (r *ScheduledRunReconciler) buildChildRun(parent *k8saiV1.DiagnosticRun, name string, triggeredAt time.Time) *k8saiV1.DiagnosticRun {
	truePtr := true
	return &k8saiV1.DiagnosticRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: parent.Namespace,
			Labels: map[string]string{
				scheduledByLabel: parent.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         parent.APIVersion,
					Kind:               parent.Kind,
					Name:               parent.Name,
					UID:                parent.UID,
					Controller:         &truePtr,
					BlockOwnerDeletion: &truePtr,
				},
			},
			Annotations: map[string]string{
				"kube-agent-helper.io/triggered-at": triggeredAt.Format(time.RFC3339),
			},
		},
		Spec: k8saiV1.DiagnosticRunSpec{
			Target:           parent.Spec.Target,
			Skills:           parent.Spec.Skills,
			ModelConfigRef:   parent.Spec.ModelConfigRef,
			TimeoutSeconds:   parent.Spec.TimeoutSeconds,
			OutputLanguage:   parent.Spec.OutputLanguage,
			// Schedule and HistoryLimit intentionally omitted — child is one-shot.
		},
	}
}

func (r *ScheduledRunReconciler) enforceHistoryLimit(ctx context.Context, parent *k8saiV1.DiagnosticRun) error {
	limit := defaultHistoryLimit
	if parent.Spec.HistoryLimit != nil {
		limit = *parent.Spec.HistoryLimit
	}
	if int32(len(parent.Status.ActiveRuns)) <= limit {
		return nil
	}

	// Fetch all child runs and sort by creation time.
	var children k8saiV1.DiagnosticRunList
	if err := r.List(ctx, &children,
		client.InNamespace(parent.Namespace),
		client.MatchingLabels{scheduledByLabel: parent.Name},
	); err != nil {
		return err
	}
	sort.Slice(children.Items, func(i, j int) bool {
		return children.Items[i].CreationTimestamp.Before(&children.Items[j].CreationTimestamp)
	})

	toDelete := len(children.Items) - int(limit)
	for i := 0; i < toDelete; i++ {
		child := children.Items[i]
		if err := r.Delete(ctx, &child,
			client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("delete old child run %s: %w", child.Name, err)
		}
		parent.Status.ActiveRuns = removeFromSlice(parent.Status.ActiveRuns, child.Name)
	}
	return nil
}

func (r *ScheduledRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8saiV1.DiagnosticRun{}).
		Owns(&k8saiV1.DiagnosticRun{}).
		Named("scheduled-run").
		Complete(r)
}

// childRunName returns a deterministic child run name based on parent name and trigger time.
func childRunName(parentName string, t time.Time) string {
	suffix := fmt.Sprintf("%d", t.Unix())
	name := fmt.Sprintf("%s-%s", parentName, suffix)
	// Kubernetes name limit is 253 characters.
	if len(name) > 253 {
		name = name[len(name)-253:]
	}
	return name
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

func removeFromSlice(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, v := range slice {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}

// PropagationPolicy helper (avoids import of metav1.DeletionPropagation directly in call site).
func init() {
	_ = corev1.SchemeBuilder // ensure corev1 import is used via OwnerReferences indirectly
}
```

> **注意**：`buildChildRun` 中 `parent.APIVersion` 和 `parent.Kind` 在从 API Server 读取时可能为空（Go 的 client 读取时不填充 TypeMeta）。需要硬编码或从 scheme 获取。将 `APIVersion` 和 `Kind` 改为：
>
> ```go
> APIVersion: "kube-agent-helper.io/v1alpha1",
> Kind:       "DiagnosticRun",
> ```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/controller/reconciler/... -run TestParseCron -v
go test ./internal/controller/reconciler/... -run TestNextRunAfterNow -v
go test ./internal/controller/reconciler/... -run TestChildRunName -v
```

Expected: 3 tests PASS

- [ ] **Step 5: 验证整体编译**

```bash
go build ./...
```

Expected: 无错误

- [ ] **Step 6: Commit**

```bash
git add internal/controller/reconciler/scheduled_run_reconciler.go \
        internal/controller/reconciler/scheduled_run_reconciler_test.go
git commit -m "feat(reconciler): add ScheduledRunReconciler for cron-based DiagnosticRun"
```

---

### Task 4: 注册到 main.go

**Files:**
- Modify: `cmd/controller/main.go`

- [ ] **Step 1: 在 main.go 的 DiagnosticFixReconciler 注册后面添加**

```go
if err := (&reconciler.ScheduledRunReconciler{
    Client: mgr.GetClient(),
}).SetupWithManager(mgr); err != nil {
    slog.Error("setup scheduled run reconciler", "error", err)
    os.Exit(1)
}
```

- [ ] **Step 2: 验证编译**

```bash
go build ./cmd/controller/...
```

Expected: 无错误

- [ ] **Step 3: 运行全部测试**

```bash
go test ./... -race -count=1 -timeout=120s
```

Expected: 全部 PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/controller/main.go
git commit -m "feat(main): register ScheduledRunReconciler"
```

---

### Task 5: Dashboard 前端 — 周期诊断 UI

**Files:**
- Modify: `dashboard/src/lib/types.ts`
- Modify: `dashboard/src/app/diagnose/page.tsx`
- Modify: `dashboard/src/app/runs/[id]/page.tsx`
- Modify: `dashboard/src/i18n/zh.json`
- Modify: `dashboard/src/i18n/en.json`

- [ ] **Step 1: 更新 types.ts — DiagnosticRun 增加 scheduled 字段**

在 `DiagnosticRun` 接口末尾加：

```ts
// Scheduled run fields (only present when spec.schedule is set)
Schedule?: string;
HistoryLimit?: number;
LastRunAt?: string | null;
NextRunAt?: string | null;
ActiveRuns?: string[];
```

在 `CreateRunRequest` 接口末尾加：

```ts
schedule?: string;
historyLimit?: number;
```

- [ ] **Step 2: 更新 zh.json — 新增翻译键**

在 `"diagnose"` 节点末尾加：

```json
"schedule": "定时执行（可选）",
"schedulePlaceholder": "Cron 表达式，如 0 * * * *（每小时）",
"scheduleHint": "留空 = 一次性诊断；填写后自动按计划重复执行"
```

在 `"runs"` 节点末尾加：

```json
"detail.schedule": "调度计划",
"detail.nextRunAt": "下次执行",
"detail.lastRunAt": "上次执行",
"detail.activeRuns": "历史子任务",
"detail.scheduledBadge": "周期任务"
```

- [ ] **Step 3: 更新 en.json — 新增翻译键**

在 `"diagnose"` 节点末尾加：

```json
"schedule": "Schedule (optional)",
"schedulePlaceholder": "Cron expression, e.g. 0 * * * * (hourly)",
"scheduleHint": "Leave empty for one-time run; fill to repeat on schedule"
```

在 `"runs"` 节点末尾加：

```json
"detail.schedule": "Schedule",
"detail.nextRunAt": "Next Run",
"detail.lastRunAt": "Last Run",
"detail.activeRuns": "Child Runs",
"detail.scheduledBadge": "Scheduled"
```

- [ ] **Step 4: 更新 diagnose/page.tsx — 在 outputLanguage 下面增加 schedule 输入**

在 `const [outputLang, setOutputLang]` 那行后面加 state：

```tsx
const [schedule, setSchedule] = useState("");
```

在 `handleSubmit` 的 `createRun(...)` 调用中，`outputLanguage: outputLang` 后面加：

```tsx
...(schedule ? { schedule } : {}),
```

在 outputLanguage 表单块（`</div>` 关闭标签）后面、`{error && ...}` 前面插入：

```tsx
<div>
  <label className="block text-sm font-medium mb-1">{t("diagnose.schedule")}</label>
  <input
    type="text"
    value={schedule}
    onChange={(e) => setSchedule(e.target.value)}
    placeholder={t("diagnose.schedulePlaceholder")}
    className="w-full rounded border px-3 py-2 text-sm dark:bg-gray-800 dark:border-gray-700 font-mono"
  />
  <p className="text-xs text-gray-500 mt-1">{t("diagnose.scheduleHint")}</p>
</div>
```

- [ ] **Step 5: 更新 runs/[id]/page.tsx — 显示 scheduled run 状态**

在 `formatTime` 函数之后，`export default function RunDetailPage` 之前加：

```tsx
function ScheduledRunInfo({ run }: { run: import("@/lib/types").DiagnosticRun }) {
  const { t } = useI18n();
  if (!run.Schedule) return null;
  return (
    <div className="mt-3 rounded-lg border border-blue-200 bg-blue-50 px-4 py-3 text-sm dark:border-blue-800 dark:bg-blue-950">
      <div className="flex items-center gap-2 font-medium text-blue-700 dark:text-blue-300 mb-2">
        <span>🔁</span>
        <span>{t("runs.detail.scheduledBadge")}</span>
        <code className="font-mono text-xs bg-blue-100 dark:bg-blue-900 px-1.5 py-0.5 rounded">{run.Schedule}</code>
      </div>
      <div className="grid grid-cols-2 gap-2 text-blue-600 dark:text-blue-400">
        <div><span className="font-medium">{t("runs.detail.lastRunAt")}:</span> {formatTime(run.LastRunAt ?? null)}</div>
        <div><span className="font-medium">{t("runs.detail.nextRunAt")}:</span> {formatTime(run.NextRunAt ?? null)}</div>
      </div>
      {run.ActiveRuns && run.ActiveRuns.length > 0 && (
        <div className="mt-2">
          <span className="font-medium">{t("runs.detail.activeRuns")}:</span>
          <div className="mt-1 flex flex-wrap gap-1">
            {run.ActiveRuns.slice(-5).map((name) => (
              <Link
                key={name}
                href={`/diagnose/${encodeURIComponent(name)}`}
                className="font-mono text-xs bg-blue-100 dark:bg-blue-900 px-2 py-0.5 rounded hover:underline"
              >
                {name}
              </Link>
            ))}
            {run.ActiveRuns.length > 5 && (
              <span className="text-xs text-blue-500">+{run.ActiveRuns.length - 5} more</span>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
```

在 `RunDetailPage` 内，`{run.Message && (...)}` 之后加：

```tsx
<ScheduledRunInfo run={run} />
```

- [ ] **Step 6: 验证前端编译**

```bash
cd dashboard && npm run build 2>&1 | tail -20
```

Expected: 无 TypeScript 错误，build 成功

- [ ] **Step 7: Commit**

```bash
git add dashboard/src/lib/types.ts \
        dashboard/src/app/diagnose/page.tsx \
        dashboard/src/app/runs/[id]/page.tsx \
        dashboard/src/i18n/zh.json \
        dashboard/src/i18n/en.json
git commit -m "feat(dashboard): add schedule UI for periodic DiagnosticRun"
```

---

## Self-Review

**Spec coverage:**
- ✅ `spec.schedule` cron 字段
- ✅ `spec.historyLimit` 保留历史数量
- ✅ `status.nextRunAt` / `status.lastRunAt` / `status.activeRuns`
- ✅ 子 run 有 OwnerReference（parent 删除时级联删除子 run）
- ✅ 子 run 有 label `kube-agent-helper.io/scheduled-by` 便于查询
- ✅ 幂等：到达触发时间前先检查 child 是否已存在
- ✅ 现有 `DiagnosticRunReconciler` 跳过 scheduled template run
- ✅ `RequeueAfter` 精确触发，不依赖 watch 事件

**已知限制（可接受）：**
- 时钟抖动：controller 重启时若 nextRunAt 已过，会立即触发一次补偿创建，符合预期。
- 精度：RequeueAfter 在 controller-runtime 中有少量漂移，分钟级 cron 可接受。
