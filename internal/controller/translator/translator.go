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
	AgentImage       string
	ControllerURL    string
	AnthropicBaseURL string
	Model            string
	PrometheusURL    string
}

// SkillProvider is the interface Translator uses to fetch enabled skills.
// It is intentionally defined here (not in registry) to avoid coupling.
type SkillProvider interface {
	ListEnabled(ctx context.Context) ([]*store.Skill, error)
}

type Translator struct {
	cfg      Config
	provider SkillProvider
	k8s      client.Client
}

func New(cfg Config, provider SkillProvider) *Translator {
	return &Translator{cfg: cfg, provider: provider}
}

func NewWithClient(cfg Config, provider SkillProvider, k8s client.Client) *Translator {
	return &Translator{cfg: cfg, provider: provider, k8s: k8s}
}

// Compile produces all Kubernetes objects needed for one DiagnosticRun.
func (t *Translator) Compile(ctx context.Context, run *k8saiV1.DiagnosticRun) ([]client.Object, error) {
	runID := string(run.UID)
	if runID == "" {
		runID = run.Name
	}

	// Fetch skills from the provider (hot-reload: reads store on every call)
	allSkills, err := t.provider.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	selected := selectSkills(allSkills, run.Spec.Skills)
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

	baseURL := t.resolveBaseURL(ctx, run)
	modelName := t.resolveModel(ctx, run)
	apiKeyEnv := t.resolveAPIKeyEnv(ctx, run)
	job := t.buildJob(run, runID, saName, cmName, selected, baseURL, modelName, apiKeyEnv)

	return []client.Object{sa, cm, rb, job}, nil
}

func selectSkills(skills []*store.Skill, names []string) []*store.Skill {
	if len(names) == 0 {
		return skills
	}
	byName := make(map[string]*store.Skill, len(skills))
	for _, s := range skills {
		byName[s.Name] = s
	}
	var selected []*store.Skill
	for _, n := range names {
		if s, ok := byName[n]; ok {
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

func (t *Translator) buildJob(run *k8saiV1.DiagnosticRun, runID, saName, cmName string, skills []*store.Skill, baseURL, modelName string, apiKeyEnv corev1.EnvVar) *batchv1.Job {
	ttl := int32(3600)
	backoff := int32(0)
	isController := true

	skillNames := make([]string, len(skills))
	for i, s := range skills {
		skillNames[i] = s.Name
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:   truncateName(fmt.Sprintf("agent-%s", run.Name), 63),
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
							{Name: "PROMETHEUS_URL", Value: t.cfg.PrometheusURL},
							{Name: "SKILL_NAMES", Value: strings.Join(skillNames, ",")},
							{Name: "ANTHROPIC_BASE_URL", Value: baseURL},
							{Name: "MODEL", Value: modelName},
							{Name: "OUTPUT_LANGUAGE", Value: func() string {
								if run.Spec.OutputLanguage != "" {
									return run.Spec.OutputLanguage
								}
								return "en"
							}()},
							apiKeyEnv,
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

// truncateName truncates s to max characters, keeping the suffix (end) for uniqueness.
func truncateName(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

// resolveModelConfig looks up the ModelConfig CR by name in the run's namespace.
// Returns nil (no error) if the CR doesn't exist or the client is unavailable.
func (t *Translator) resolveModelConfig(ctx context.Context, run *k8saiV1.DiagnosticRun) *k8saiV1.ModelConfig {
	if t.k8s == nil || run.Spec.ModelConfigRef == "" {
		return nil
	}
	var mc k8saiV1.ModelConfig
	if err := t.k8s.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: run.Spec.ModelConfigRef}, &mc); err != nil {
		return nil
	}
	return &mc
}

// resolveBaseURL returns ModelConfig.Spec.BaseURL if set, else the global config value.
func (t *Translator) resolveBaseURL(ctx context.Context, run *k8saiV1.DiagnosticRun) string {
	if mc := t.resolveModelConfig(ctx, run); mc != nil && mc.Spec.BaseURL != "" {
		return mc.Spec.BaseURL
	}
	return t.cfg.AnthropicBaseURL
}

// resolveModel returns ModelConfig.Spec.Model if set, else the global config value.
func (t *Translator) resolveModel(ctx context.Context, run *k8saiV1.DiagnosticRun) string {
	if mc := t.resolveModelConfig(ctx, run); mc != nil && mc.Spec.Model != "" {
		return mc.Spec.Model
	}
	return t.cfg.Model
}

// resolveAPIKeyEnv builds the ANTHROPIC_API_KEY EnvVar from ModelConfig.Spec.APIKeyRef,
// falling back to treating ModelConfigRef as the Secret name with key "apiKey".
func (t *Translator) resolveAPIKeyEnv(ctx context.Context, run *k8saiV1.DiagnosticRun) corev1.EnvVar {
	secretName := run.Spec.ModelConfigRef
	secretKey := "apiKey"
	if mc := t.resolveModelConfig(ctx, run); mc != nil {
		secretName = mc.Spec.APIKeyRef.Name
		secretKey = mc.Spec.APIKeyRef.Key
	}
	return corev1.EnvVar{
		Name: "ANTHROPIC_API_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  secretKey,
			},
		},
	}
}
