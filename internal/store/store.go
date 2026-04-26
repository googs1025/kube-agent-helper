// Package store 定义系统的持久化抽象层。
//
// 架构角色：
//   - 整个系统的"数据库门面"。Reconciler / HTTP Server / Collector 全部
//     通过 Store 接口读写数据，不直接依赖 SQL。
//   - 内置的 sqlite 子包是默认实现，未来可扩展 Postgres 而上层无感知。
//
// 数据模型：
//   - DiagnosticRun  一次诊断任务（与 K8s CR 一一对应，UID 作为 ID）
//   - Finding        诊断产出（一次 Run 多条 Finding）
//   - Skill          诊断能力（来自 builtin .md 或 DiagnosticSkill CR）
//   - Fix            修复建议（findings 衍生，可批准/应用）
//   - Event          K8s Warning 事件（7 天保留）
//   - MetricSnapshot Prometheus 指标采样
//   - RunLog         Agent Pod 结构化日志
//   - NotificationConfig 通知通道配置
//
// 多集群：所有列表查询都支持 ClusterName 过滤，配合 ClusterClientRegistry
// 实现"一份控制器、多个目标集群"的视图。
package store

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

type Phase string

const (
	PhasePending   Phase = "Pending"
	PhaseRunning   Phase = "Running"
	PhaseSucceeded Phase = "Succeeded"
	PhaseFailed    Phase = "Failed"
)

type DiagnosticRun struct {
	ID          string
	Name        string // K8s CR name, populated at API layer (not persisted in SQLite)
	ClusterName string
	TargetJSON  string
	SkillsJSON  string
	Status      Phase
	Message     string
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
}

type Finding struct {
	ID                string
	RunID             string
	ClusterName       string
	Dimension         string
	Severity          string
	Title             string
	Description       string
	ResourceKind      string
	ResourceNamespace string
	ResourceName      string
	Suggestion        string
	CreatedAt         time.Time
}

type Skill struct {
	ID               string
	Name             string
	Dimension        string
	Prompt           string
	ToolsJSON        string
	RequiresDataJSON string
	Source           string // builtin | cr
	Enabled          bool
	Priority         int
	UpdatedAt        time.Time
}

type FixPhase string

const (
	FixPhasePendingApproval FixPhase = "PendingApproval"
	FixPhaseApproved        FixPhase = "Approved"
	FixPhaseApplying        FixPhase = "Applying"
	FixPhaseSucceeded       FixPhase = "Succeeded"
	FixPhaseFailed          FixPhase = "Failed"
	FixPhaseRolledBack      FixPhase = "RolledBack"
	FixPhaseDryRunComplete  FixPhase = "DryRunComplete"
)

type Fix struct {
	ID               string
	Name             string // K8s CR name, populated at API layer (not persisted in SQLite)
	ClusterName      string
	RunID            string
	FindingTitle     string
	TargetKind       string
	TargetNamespace  string
	TargetName       string
	Strategy         string
	ApprovalRequired bool
	PatchType        string
	PatchContent     string
	Phase            FixPhase
	ApprovedBy       string
	RollbackSnapshot string
	Message          string
	FindingID        string
	BeforeSnapshot   string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Event represents a stored Kubernetes event (Warning type, 7-day retention).
type Event struct {
	ID          int64
	UID         string
	ClusterName string
	Namespace   string
	Kind      string
	Name      string
	Reason    string
	Message   string
	Type      string
	Count     int32
	FirstTime time.Time
	LastTime  time.Time
	CreatedAt time.Time
}

// ListEventsOpts filters for ListEvents.
type ListEventsOpts struct {
	ClusterName  string
	Namespace    string
	Name         string
	Type         string // "" = all, "Warning", "Normal"
	SinceMinutes int    // 0 = all time
	Limit        int
}

// MetricSnapshot represents a single scraped Prometheus metric data point.
type MetricSnapshot struct {
	ID          int64
	ClusterName string
	Query       string
	LabelsJSON string
	Value      float64
	Ts         time.Time
	CreatedAt  time.Time
}

// RunLog represents a single structured log entry emitted by an agent pod.
type RunLog struct {
	ID        int64  `json:"id"`
	RunID     string `json:"run_id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Message   string `json:"message"`
	Data      string `json:"data,omitempty"`
}

type ListOpts struct {
	ClusterName string
	Limit       int
	Offset      int
	// Paginated query fields (Page >= 1 enables paginated mode)
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string            // "asc" or "desc"
	Filters   map[string]string // e.g. {"phase":"Running","namespace":"default"}
}

// PaginatedResult wraps a paginated list response.
type PaginatedResult[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// DefaultListOpts returns sensible defaults for paginated queries.
func DefaultListOpts() ListOpts {
	return ListOpts{Page: 1, PageSize: 20, SortBy: "created_at", SortOrder: "desc"}
}

// NotificationConfig represents a notification channel configuration stored in DB.
type NotificationConfig struct {
	ID         string
	Name       string
	Type       string // webhook, slack, dingtalk, feishu
	WebhookURL string
	Secret     string
	Events     string // comma-separated event types
	Enabled    bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Store is the persistence interface. Both SQLite and PostgreSQL implement it.
type Store interface {
	// Runs
	CreateRun(ctx context.Context, run *DiagnosticRun) error
	GetRun(ctx context.Context, id string) (*DiagnosticRun, error)
	UpdateRunStatus(ctx context.Context, id string, phase Phase, msg string) error
	ListRuns(ctx context.Context, opts ListOpts) ([]*DiagnosticRun, error)

	// Findings
	CreateFinding(ctx context.Context, f *Finding) error
	ListFindings(ctx context.Context, runID string) ([]*Finding, error)

	// Skills
	UpsertSkill(ctx context.Context, s *Skill) error
	ListSkills(ctx context.Context) ([]*Skill, error)
	GetSkill(ctx context.Context, name string) (*Skill, error)
	DeleteSkill(ctx context.Context, name string) error

	// Fixes
	CreateFix(ctx context.Context, f *Fix) error
	GetFix(ctx context.Context, id string) (*Fix, error)
	ListFixes(ctx context.Context, opts ListOpts) ([]*Fix, error)
	ListFixesByRun(ctx context.Context, runID string) ([]*Fix, error)
	UpdateFixPhase(ctx context.Context, id string, phase FixPhase, msg string) error
	UpdateFixApproval(ctx context.Context, id string, approvedBy string) error
	UpdateFixSnapshot(ctx context.Context, id string, snapshot string) error

	// Events (7-day retention)
	UpsertEvent(ctx context.Context, e *Event) error
	ListEvents(ctx context.Context, opts ListEventsOpts) ([]*Event, error)

	// Metric snapshots
	InsertMetricSnapshot(ctx context.Context, s *MetricSnapshot) error
	QueryMetricHistory(ctx context.Context, query string, sinceMinutes int) ([]*MetricSnapshot, error)

	// Paginated list methods
	ListRunsPaginated(ctx context.Context, opts ListOpts) (PaginatedResult[*DiagnosticRun], error)
	ListFixesPaginated(ctx context.Context, opts ListOpts) (PaginatedResult[*Fix], error)
	ListEventsPaginated(ctx context.Context, opts ListEventsOpts, page, pageSize int) (PaginatedResult[*Event], error)

	// Batch operations
	DeleteRuns(ctx context.Context, ids []string) error
	BatchUpdateFixPhase(ctx context.Context, ids []string, phase FixPhase, msg string) error

	// Run logs (agent pod structured log entries)
	AppendRunLog(ctx context.Context, log RunLog) error
	ListRunLogs(ctx context.Context, runID string, afterID int64) ([]RunLog, error)

	// Notification configs
	ListNotificationConfigs(ctx context.Context) ([]*NotificationConfig, error)
	GetNotificationConfig(ctx context.Context, id string) (*NotificationConfig, error)
	CreateNotificationConfig(ctx context.Context, cfg *NotificationConfig) error
	UpdateNotificationConfig(ctx context.Context, cfg *NotificationConfig) error
	DeleteNotificationConfig(ctx context.Context, id string) error

	// TTL cleanup
	PurgeOldEvents(ctx context.Context, before time.Time) error
	PurgeOldMetrics(ctx context.Context, before time.Time) error

	Close() error
}