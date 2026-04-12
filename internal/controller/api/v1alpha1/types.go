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
}

type TargetSpec struct {
	// +kubebuilder:validation:Enum=namespace;cluster
	Scope         string            `json:"scope"`
	Namespaces    []string          `json:"namespaces,omitempty"`
	LabelSelector map[string]string `json:"labelSelector,omitempty"`
}

type DiagnosticRunStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed
	Phase       string       `json:"phase,omitempty"`
	StartedAt   *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	ReportID    string       `json:"reportId,omitempty"`
	Message     string       `json:"message,omitempty"`
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

func init() {
	SchemeBuilder.Register(
		&DiagnosticSkill{}, &DiagnosticSkillList{},
		&DiagnosticRun{}, &DiagnosticRunList{},
		&ModelConfig{}, &ModelConfigList{},
	)
}
