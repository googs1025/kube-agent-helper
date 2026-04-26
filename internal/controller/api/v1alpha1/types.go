// Package v1alpha1 定义系统全部 CRD 类型。
//
// 资源关系：
//
//	ClusterConfig（远程集群配置） ◀──┐
//	                                │ spec.clusterRef
//	                                │
//	DiagnosticSkill ◀── DiagnosticRun ──▶ ModelConfig
//	  (诊断能力)         (一次诊断任务)      (LLM 配置)
//	                       │
//	                       │ 产出
//	                       ▼
//	                    Findings ──▶ DiagnosticFix
//	                                 (修复建议，可批准/应用)
//
//	ScheduledRun（cron 模板，由 spec.schedule 启用）
//	   └─▶ 周期性创建子 DiagnosticRun
//
// 这些类型的 Reconciler 在 internal/controller/reconciler 包，每种 CRD 一个。
// 其中 DiagnosticRunReconciler 是核心 — 它负责把 CR 翻译成 K8s Job 并跟踪生命周期。
//
// kubebuilder marker（// +kubebuilder:...）用于 controller-gen 生成 deepcopy
// 函数和 CRD YAML（输出到 deploy/helm/templates/crds/）。
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
// +kubebuilder:printcolumn:name="NextRun",type=date,JSONPath=`.status.nextRunAt`
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
	// +optional
	// +kubebuilder:validation:Enum=zh;en
	OutputLanguage string `json:"outputLanguage,omitempty"`
	// Schedule is a cron expression for periodic runs, e.g. "0 * * * *".
	// When set, this DiagnosticRun acts as a template; child runs are created automatically.
	// +optional
	Schedule string `json:"schedule,omitempty"`
	// HistoryLimit is the maximum number of completed child runs to retain.
	// +optional
	// +kubebuilder:default=10
	HistoryLimit *int32 `json:"historyLimit,omitempty"`
	// ClusterRef is the name of a ClusterConfig CR in the same namespace.
	// When empty, the local (controller) cluster is used.
	// +optional
	ClusterRef string `json:"clusterRef,omitempty"`
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
	// LastRunAt is the time the last child run was created (only set when schedule is used).
	// +optional
	LastRunAt *metav1.Time `json:"lastRunAt,omitempty"`
	// NextRunAt is the scheduled time for the next child run.
	// +optional
	NextRunAt *metav1.Time `json:"nextRunAt,omitempty"`
	// ActiveRuns lists the names of child DiagnosticRuns created by this scheduled run.
	// +optional
	ActiveRuns []string `json:"activeRuns,omitempty"`
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
	// BaseURL overrides the Anthropic API endpoint (e.g. for custom proxies).
	// +optional
	BaseURL string `json:"baseURL,omitempty"`
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
// +kubebuilder:resource:scope=Namespaced,shortName=dfix
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
	// +kubebuilder:validation:Enum=auto;dry-run;create
	// +kubebuilder:default=auto
	Strategy         string `json:"strategy"`
	// +kubebuilder:default=true
	ApprovalRequired bool     `json:"approvalRequired"`
	Patch            FixPatch `json:"patch"`
	Rollback         RollbackConfig `json:"rollback,omitempty"`
	// +optional
	FindingID string `json:"findingID,omitempty"`
}

type FixTarget struct {
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

// ── ClusterConfig ─────────────────────────────────────────────────────────

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ClusterConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterConfigSpec   `json:"spec,omitempty"`
	Status            ClusterConfigStatus `json:"status,omitempty"`
}

type ClusterConfigSpec struct {
	// KubeConfigRef is the reference to a Secret containing a kubeconfig for the remote cluster.
	KubeConfigRef SecretKeyRef `json:"kubeConfigRef"`
	// PrometheusURL is the Prometheus endpoint accessible from within the remote cluster (optional).
	// +optional
	PrometheusURL string `json:"prometheusURL,omitempty"`
	// +optional
	Description string `json:"description,omitempty"`
}

type ClusterConfigStatus struct {
	// +kubebuilder:validation:Enum=Connected;Error
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
type ClusterConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(
		&DiagnosticSkill{}, &DiagnosticSkillList{},
		&DiagnosticRun{}, &DiagnosticRunList{},
		&ModelConfig{}, &ModelConfigList{},
		&DiagnosticFix{}, &DiagnosticFixList{},
		&ClusterConfig{}, &ClusterConfigList{},
	)
}
