package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ── DiagnosticSkill ────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type DiagnosticSkill struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiagnosticSkillSpec `json:"spec,omitempty"`
}

type DiagnosticSkillSpec struct {
	// +kubebuilder:validation:Enum=health;security;cost;reliability
	Dimension    string   `json:"dimension"`
	Description  string   `json:"description"`
	Prompt       string   `json:"prompt"`
	// +kubebuilder:validation:MinItems=1
	Tools        []string `json:"tools"`
	RequiresData []string `json:"requiresData,omitempty"`
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`
	// +kubebuilder:default=100
	Priority *int `json:"priority,omitempty"`
}

// +kubebuilder:object:root=true
type DiagnosticSkillList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiagnosticSkill `json:"items"`
}

// ── DiagnosticRun ─────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type DiagnosticRun struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiagnosticRunSpec   `json:"spec,omitempty"`
	Status            DiagnosticRunStatus `json:"status,omitempty"`
}

type DiagnosticRunSpec struct {
	Target         TargetSpec `json:"target"`
	Skills         []string   `json:"skills,omitempty"`
	ModelConfigRef string     `json:"modelConfigRef"`
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

type TargetSpec struct {
	// +kubebuilder:validation:Enum=namespace;cluster
	Scope         string            `json:"scope"`
	Namespaces    []string          `json:"namespaces,omitempty"`
	LabelSelector map[string]string `json:"labelSelector,omitempty"`
}

type DiagnosticRunStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed
	Phase         string            `json:"phase,omitempty"`
	StartedAt     *metav1.Time      `json:"startedAt,omitempty"`
	CompletedAt   *metav1.Time      `json:"completedAt,omitempty"`
	ReportID      string            `json:"reportId,omitempty"`
	Message       string            `json:"message,omitempty"`
	FindingCounts map[string]int    `json:"findingCounts,omitempty"`
	Findings      []FindingSummary  `json:"findings,omitempty"`
}

// FindingSummary is a compact representation of a finding stored in CR status.
type FindingSummary struct {
	Dimension         string `json:"dimension"`
	Severity          string `json:"severity"`
	Title             string `json:"title"`
	ResourceKind      string `json:"resourceKind,omitempty"`
	ResourceNamespace string `json:"resourceNamespace,omitempty"`
	ResourceName      string `json:"resourceName,omitempty"`
	Suggestion        string `json:"suggestion,omitempty"`
}

// +kubebuilder:object:root=true
type DiagnosticRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiagnosticRun `json:"items"`
}

// ── ModelConfig ───────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type ModelConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ModelConfigSpec `json:"spec,omitempty"`
}

type ModelConfigSpec struct {
	// +kubebuilder:default=anthropic
	// +kubebuilder:validation:Enum=anthropic
	Provider string `json:"provider"`
	// +kubebuilder:default="claude-sonnet-4-6"
	Model     string       `json:"model"`
	APIKeyRef SecretKeyRef `json:"apiKeyRef"`
	// +kubebuilder:default=20
	MaxTurns *int `json:"maxTurns,omitempty"`
}

type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// +kubebuilder:object:root=true
type ModelConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelConfig `json:"items"`
}

// ── DiagnosticFix ────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type DiagnosticFix struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DiagnosticFixSpec   `json:"spec,omitempty"`
	Status            DiagnosticFixStatus `json:"status,omitempty"`
}

type DiagnosticFixSpec struct {
	DiagnosticRunRef string    `json:"diagnosticRunRef"`
	FindingTitle     string    `json:"findingTitle"`
	Target           FixTarget `json:"target"`
	// +kubebuilder:validation:Enum=auto;dry-run
	// +kubebuilder:default=auto
	Strategy         string `json:"strategy"`
	// +kubebuilder:default=true
	ApprovalRequired bool     `json:"approvalRequired"`
	Patch            FixPatch `json:"patch"`
	Rollback         RollbackConfig `json:"rollback,omitempty"`
}

type FixTarget struct {
	// +kubebuilder:validation:Enum=Deployment;StatefulSet;DaemonSet;Service;ConfigMap
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type FixPatch struct {
	// +kubebuilder:validation:Enum=strategic-merge;json-patch
	// +kubebuilder:default=strategic-merge
	Type    string `json:"type"`
	Content string `json:"content"`
}

type RollbackConfig struct {
	// +kubebuilder:default=true
	Enabled               bool `json:"enabled"`
	// +kubebuilder:default=true
	SnapshotBefore        bool `json:"snapshotBefore"`
	// +kubebuilder:default=true
	AutoRollbackOnFailure bool `json:"autoRollbackOnFailure"`
	// +kubebuilder:default=300
	HealthCheckTimeout    int  `json:"healthCheckTimeout,omitempty"`
}

type DiagnosticFixStatus struct {
	// +kubebuilder:validation:Enum=PendingApproval;Approved;Applying;Succeeded;Failed;RolledBack;DryRunComplete
	Phase            string       `json:"phase,omitempty"`
	ApprovedBy       string       `json:"approvedBy,omitempty"`
	ApprovedAt       *metav1.Time `json:"approvedAt,omitempty"`
	AppliedAt        *metav1.Time `json:"appliedAt,omitempty"`
	CompletedAt      *metav1.Time `json:"completedAt,omitempty"`
	RollbackSnapshot string       `json:"rollbackSnapshot,omitempty"`
	Message          string       `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
type DiagnosticFixList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DiagnosticFix `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&DiagnosticSkill{}, &DiagnosticSkillList{},
		&DiagnosticRun{}, &DiagnosticRunList{},
		&ModelConfig{}, &ModelConfigList{},
		&DiagnosticFix{}, &DiagnosticFixList{},
	)
}
