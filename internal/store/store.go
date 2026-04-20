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
	ID        int64
	UID       string
	Namespace string
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
	Namespace    string
	Name         string
	Type         string // "" = all, "Warning", "Normal"
	SinceMinutes int    // 0 = all time
	Limit        int
}

// MetricSnapshot represents a single scraped Prometheus metric data point.
type MetricSnapshot struct {
	ID         int64
	Query      string
	LabelsJSON string
	Value      float64
	Ts         time.Time
	CreatedAt  time.Time
}

type ListOpts struct {
	Limit  int
	Offset int
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

	// TTL cleanup
	PurgeOldEvents(ctx context.Context, before time.Time) error
	PurgeOldMetrics(ctx context.Context, before time.Time) error

	Close() error
}