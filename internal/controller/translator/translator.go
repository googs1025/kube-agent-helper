package translator

import (
	"context"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8saiV1 "github.com/kube-agent-helper/kube-agent-helper/internal/controller/api/v1alpha1"
	"github.com/kube-agent-helper/kube-agent-helper/internal/store"
)

type Config struct {
	AgentImage     string
	ControllerURL  string
	AnthropicBaseURL string
	Model            string
}

type Translator struct {
	cfg    Config
	skills []*store.Skill
}

func New(cfg Config, skills []*store.Skill) *Translator {
	return &Translator{cfg: cfg, skills: skills}
}

// Compile produces all Kubernetes objects needed for one DiagnosticRun.
func (t *Translator) Compile(_ context.Context, run *k8saiV1.DiagnosticRun) ([]client.Object, error) {
	runID := string(run.UID)
	if runID == "" {
		runID = run.Name
	}

	// Select skills for this run
	selected := t.selectSkills(run.Spec.Skills)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no enabled skills found for run %s", run.Name)
	}

	saName := fmt.Sprintf("run-%s", run.Name)
	cmName := fmt.Sprintf("skill-bundle-%s", run.Name)
	namespaces := run.Spec.Target.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{run.Namespace}
	}

	sa := t.buildSA(saName, runID)
	cm := t.buildConfigMap(cmName, runID, selected)
	rb := t.buildRoleBinding(saName, runID, run.Namespace)
	job := t.buildJob(run, runID, saName, cmName, selected)

	return []client.Object{sa, cm, rb, job}, nil
}

func (t *Translator) selectSkills(names []string) []*store.Skill {
	if len(names) == 0 {
		var all []*store.Skill
		for _, s := range t.skills {
			if s.Enabled {
				all = append(all, s)
			}
		}
		return all
	}
	byName := make(map[string]*store.Skill, len(t.skills))
	for _, s := range t.skills {
		byName[s.Name] = s
	}
	var selected []*store.Skill
	for _, n := range names {
		if s, ok := byName[n]; ok && s.Enabled {
			selected = append(selected, s)
		}
	}
	return selected
}

func (t *Translator) buildSA(name, runID string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"run-id": runID},
		},
	}
}

func (t *Translator) buildConfigMap(name, runID string, skills []*store.Skill) *corev1.ConfigMap {
	data := make(map[string]string, len(skills))
	for _, s := range skills {
		key := s.Name + ".md"
		data[key] = fmt.Sprintf("---\nname: %s\ndimension: %s\ntools: %s\n---\n\n%s\n",
			s.Name, s.Dimension, s.ToolsJSON, s.Prompt)
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"run-id": runID},
		},
		Data: data,
	}
}

func (t *Translator) buildRoleBinding(saName, runID, saNamespace string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   saName,
			Labels: map[string]string{"run-id": runID},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "view",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      saName,
			Namespace: saNamespace,
		}},
	}
}

func (t *Translator) buildJob(run *k8saiV1.DiagnosticRun, runID, saName, cmName string, skills []*store.Skill) *batchv1.Job {
	ttl := int32(3600)
	backoff := int32(0)
	isController := true

	skillNames := make([]string, len(skills))
	for i, s := range skills {
		skillNames[i] = s.Name
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("agent-%s", run.Name),
			Labels: map[string]string{"run-id": runID},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: run.APIVersion,
				Kind:       run.Kind,
				Name:       run.Name,
				UID:        run.UID,
				Controller: &isController,
			}},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			BackoffLimit:            &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: saName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{{
						Name: "skills",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
							},
						},
					}},
					Containers: []corev1.Container{{
						Name:    "agent",
						Image:   t.cfg.AgentImage,
						Command: []string{"python", "-m", "runtime.main"},
						Env: []corev1.EnvVar{
							{Name: "RUN_ID", Value: runID},
							{Name: "TARGET_NAMESPACES", Value: strings.Join(run.Spec.Target.Namespaces, ",")},
							{Name: "CONTROLLER_URL", Value: t.cfg.ControllerURL},
							{Name: "MCP_SERVER_PATH", Value: "/usr/local/bin/k8s-mcp-server"},
							{Name: "SKILL_NAMES", Value: strings.Join(skillNames, ",")},
							{Name: "ANTHROPIC_BASE_URL", Value: t.cfg.AnthropicBaseURL},
							{Name: "MODEL", Value: t.cfg.Model},
							// Phase 1 simplification: ModelConfigRef is used directly as the Secret name.
							// Phase 2 will resolve the ModelConfig CR to read APIKeyRef.Name and APIKeyRef.Key.
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
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "skills",
							MountPath: "/workspace/skills",
						}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					}},
				},
			},
		},
	}
}
