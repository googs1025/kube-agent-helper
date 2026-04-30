// Package translator 把 DiagnosticRun CR 翻译成 K8s 原生资源。
//
// 核心方法 Compile() 一次产出 4 个对象（按顺序 apply）：
//
//	┌──────────────────┐
//	│ ServiceAccount    │ ─ Agent Pod 的身份
//	│ ClusterRoleBind   │ ─ 绑定到内置 "view" ClusterRole（最小只读）
//	│ ConfigMap         │ ─ 把选中的 Skill .md 文件挂到 /workspace/skills/
//	│ Job               │ ─ 启动 Agent Pod，注入环境变量、挂卷
//	└──────────────────┘
//
// 设计要点：
//   - SkillProvider 接口（duck-typed）— 不直接依赖 registry 包；
//     测试时可注入静态 skills 列表
//   - ModelConfig 解析顺序：run.Spec.ModelConfigRef → ModelConfig CR →
//     全局 flag fallback；任意一级缺失都能正确降级
//   - 多集群：Compile 只生成对象、不 Create；调用方（Reconciler）自己
//     选 client（本地或 ClusterClientRegistry.Get）做 Apply
//
// 配套：FixGenerator（fix_generator.go）专门为 DiagnosticFix 翻译 Job。
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

// defaultMaxTokens is the per-request output cap injected into the agent pod
// env when Config.MaxTokens is unset. Matches agent-runtime's own default so
// behavior is identical whether or not the controller overrides.
const defaultMaxTokens = 8192

type Config struct {
	AgentImage          string
	ControllerURL       string
	AnthropicBaseURL    string
	Model               string
	PrometheusURL       string
	LangfuseSecretName  string // optional; if set, injects LANGFUSE_* env vars
	MaxTokens           int    // optional; 0 = use defaultMaxTokens (8192)
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

	chain := t.resolveModelChain(ctx, run)
	if len(chain) == 0 {
		// Legacy path: no client or primary missing. Synthesize a single-element
		// chain from global flags + treat ModelConfigRef as a Secret name with
		// key "apiKey" (pre-ModelConfig-CR behavior).
		chain = []*k8saiV1.ModelConfig{{
			Spec: k8saiV1.ModelConfigSpec{
				Model:     t.cfg.Model,
				BaseURL:   t.cfg.AnthropicBaseURL,
				APIKeyRef: k8saiV1.SecretKeyRef{Name: run.Spec.ModelConfigRef, Key: "apiKey"},
			},
		}}
	}
	job := t.buildJob(run, runID, saName, cmName, selected, chain)

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

func (t *Translator) buildJob(run *k8saiV1.DiagnosticRun, runID, saName, cmName string, skills []*store.Skill, chain []*k8saiV1.ModelConfig) *batchv1.Job {
	ttl := int32(3600)
	backoff := int32(0)
	isController := true

	skillNames := make([]string, len(skills))
	for i, s := range skills {
		skillNames[i] = s.Name
	}

	maxTokens := t.cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}
	baseEnv := []corev1.EnvVar{
		{Name: "RUN_ID", Value: runID},
		{Name: "TARGET_NAMESPACES", Value: strings.Join(run.Spec.Target.Namespaces, ",")},
		{Name: "CONTROLLER_URL", Value: t.cfg.ControllerURL},
		{Name: "MCP_SERVER_PATH", Value: "/usr/local/bin/k8s-mcp-server"},
		{Name: "PROMETHEUS_URL", Value: t.cfg.PrometheusURL},
		{Name: "SKILL_NAMES", Value: strings.Join(skillNames, ",")},
		{Name: "OUTPUT_LANGUAGE", Value: func() string {
			if run.Spec.OutputLanguage != "" {
				return run.Spec.OutputLanguage
			}
			return "en"
		}()},
		{Name: "MAX_TOKENS", Value: fmt.Sprintf("%d", maxTokens)},
	}
	allEnv := append(baseEnv, buildChainEnv(chain)...)
	allEnv = append(allEnv, langfuseEnvVars(t.cfg.LangfuseSecretName)...)

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
						Env:     allEnv,
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

// buildChainEnv produces MODEL_COUNT plus indexed MODEL_<i>_* env vars for each
// chain entry (BASE_URL/MODEL/RETRIES as plain values, API_KEY as SecretKeyRef),
// plus backward-compat ANTHROPIC_BASE_URL / MODEL / ANTHROPIC_API_KEY mirroring
// chain[0] so older agent images still work.
func buildChainEnv(chain []*k8saiV1.ModelConfig) []corev1.EnvVar {
	env := []corev1.EnvVar{{Name: "MODEL_COUNT", Value: fmt.Sprintf("%d", len(chain))}}
	for i, mc := range chain {
		key := mc.Spec.APIKeyRef.Key
		if key == "" {
			key = "apiKey"
		}
		env = append(env,
			corev1.EnvVar{Name: fmt.Sprintf("MODEL_%d_BASE_URL", i), Value: mc.Spec.BaseURL},
			corev1.EnvVar{Name: fmt.Sprintf("MODEL_%d_MODEL", i), Value: mc.Spec.Model},
			corev1.EnvVar{Name: fmt.Sprintf("MODEL_%d_RETRIES", i), Value: fmt.Sprintf("%d", mc.Spec.Retries)},
			corev1.EnvVar{
				Name: fmt.Sprintf("MODEL_%d_API_KEY", i),
				ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: mc.Spec.APIKeyRef.Name},
					Key:                  key,
				}},
			},
		)
	}
	if len(chain) > 0 {
		mc := chain[0]
		key := mc.Spec.APIKeyRef.Key
		if key == "" {
			key = "apiKey"
		}
		env = append(env,
			corev1.EnvVar{Name: "ANTHROPIC_BASE_URL", Value: mc.Spec.BaseURL},
			corev1.EnvVar{Name: "MODEL", Value: mc.Spec.Model},
			corev1.EnvVar{
				Name: "ANTHROPIC_API_KEY",
				ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: mc.Spec.APIKeyRef.Name},
					Key:                  key,
				}},
			},
		)
	}
	return env
}

// langfuseEnvVars returns LANGFUSE_* env vars sourced from secretName.
// Returns nil when secretName is empty (Langfuse not configured).
// LANGFUSE_HOST is optional — if the "host" key is absent the SDK defaults to
// https://cloud.langfuse.com, so the Pod must not fail on a missing key.
func langfuseEnvVars(secretName string) []corev1.EnvVar {
	if secretName == "" {
		return nil
	}
	required := func(key string) *corev1.EnvVarSource {
		return &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
				Key:                  key,
			},
		}
	}
	hostOptional := true
	return []corev1.EnvVar{
		{Name: "LANGFUSE_PUBLIC_KEY", ValueFrom: required("publicKey")},
		{Name: "LANGFUSE_SECRET_KEY", ValueFrom: required("secretKey")},
		{
			Name: "LANGFUSE_HOST",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  "host",
					Optional:             &hostOptional,
				},
			},
		},
	}
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

// resolveModelChain returns the ordered list of ModelConfigs to try: the
// primary at index 0, then each fallback in spec order. Missing fallbacks
// are silently skipped (a fallback that no longer exists must not block the
// run). Returns an empty slice if the k8s client is unavailable or the
// primary ModelConfig is missing — callers should handle the empty-chain
// case as a hard failure or fall back to legacy single-Secret behavior.
func (t *Translator) resolveModelChain(ctx context.Context, run *k8saiV1.DiagnosticRun) []*k8saiV1.ModelConfig {
	chain := []*k8saiV1.ModelConfig{}
	if t.k8s == nil || run.Spec.ModelConfigRef == "" {
		return chain
	}
	var primary k8saiV1.ModelConfig
	if err := t.k8s.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: run.Spec.ModelConfigRef}, &primary); err == nil {
		chain = append(chain, &primary)
	}
	for _, name := range run.Spec.FallbackModelConfigRefs {
		var fb k8saiV1.ModelConfig
		if err := t.k8s.Get(ctx, client.ObjectKey{Namespace: run.Namespace, Name: name}, &fb); err != nil {
			continue
		}
		chain = append(chain, &fb)
	}
	return chain
}

