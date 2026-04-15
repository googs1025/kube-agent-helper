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

	Close() error
}