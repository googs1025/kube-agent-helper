package agent

import (
	batchv1 "k8s.io/api/batch/v1"
	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// AgentRuntime generates the Job manifest for an Agent Pod.
type AgentRuntime interface {
	BuildJobSpec(run *k8saiV1.DiagnosticRun, skills []*store.Skill, model *k8saiV1.ModelConfig) (*batchv1.Job, error)
}
