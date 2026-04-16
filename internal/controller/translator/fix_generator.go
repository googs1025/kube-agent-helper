package translator

import (
	"encoding/json"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

// FixGeneratorConfig configures the short-lived Job that asks the LLM
// to propose a patch for a single finding.
type FixGeneratorConfig struct {
	AgentImage       string
	ControllerURL    string
	AnthropicBaseURL string
	Model            string
}

type FixGenerator struct {
	cfg FixGeneratorConfig
}

func NewFixGenerator(cfg FixGeneratorConfig) *FixGenerator {
	return &FixGenerator{cfg: cfg}
}

// Compile produces a single Kubernetes Job that runs the fix-generator
// entry point in the agent runtime container.
func (g *FixGenerator) Compile(run *k8saiV1.DiagnosticRun, finding *store.Finding) (*batchv1.Job, error) {
	if finding == nil || finding.ID == "" {
		return nil, fmt.Errorf("finding is required")
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}

	outputLang := run.Spec.OutputLanguage
	if outputLang == "" {
		outputLang = "en"
	}

	input := map[string]any{
		"findingID":   finding.ID,
		"runID":       finding.RunID,
		"title":       finding.Title,
		"description": finding.Description,
		"suggestion":  finding.Suggestion,
		"dimension":   finding.Dimension,
		"severity":    finding.Severity,
		"target": map[string]string{
			"kind":      finding.ResourceKind,
			"namespace": finding.ResourceNamespace,
			"name":      finding.ResourceName,
		},
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal fix input: %w", err)
	}

	backoff := int32(0)
	deadline := int64(120)
	ttl := int32(600)
	saName := fmt.Sprintf("run-%s", run.Name) // reuse per-run SA

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("fix-gen-%s", finding.ID),
			Namespace: run.Namespace,
			Labels: map[string]string{
				"finding-id": finding.ID,
				"run-id":     string(run.UID),
				"component":  "fix-generator",
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			BackoffLimit:            &backoff,
			ActiveDeadlineSeconds:   &deadline,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: saName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "fix-generator",
						Image:   g.cfg.AgentImage,
						Command: []string{"python", "-m", "runtime.fix_main"},
						Env: []corev1.EnvVar{
							{Name: "FIX_INPUT_JSON", Value: string(inputJSON)},
							{Name: "CONTROLLER_URL", Value: g.cfg.ControllerURL},
							{Name: "MCP_SERVER_PATH", Value: "/usr/local/bin/k8s-mcp-server"},
							{Name: "OUTPUT_LANGUAGE", Value: outputLang},
							{Name: "ANTHROPIC_BASE_URL", Value: g.cfg.AnthropicBaseURL},
							{Name: "MODEL", Value: g.cfg.Model},
							{
								Name: "ANTHROPIC_API_KEY",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: run.Spec.ModelConfigRef,
										},
										Key: "apiKey",
									},
								},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("50m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					}},
				},
			},
		},
	}, nil
}
