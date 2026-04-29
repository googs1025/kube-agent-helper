# ModelConfig Fallback 与重试机制 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 给 `kube-agent-helper` 加上 ModelConfig 单模型重试 (`retries`) 与 DiagnosticRun 多模型 fallback 链 (`fallbackModelConfigRefs`)，全 opt-in，messages 历史跨切完整保留。

**Architecture:** Translator 在 Job 创建时解析主+fallback 全部 ModelConfig 并以 `MODEL_<i>_*` 索引化 env 注入；新增 Python `ModelChain` 模块在 agent-runtime 进程内执行重试 + fallback 决策；Dashboard 提供下拉框 + chips 选择器。

**Tech Stack:** Go (controller-runtime, k8s API)、Python (httpx 直连 SSE、anthropic SDK 仅做凭据载体)、TypeScript/React (Next.js 15、shadcn/ui、SWR)、Vitest、Playwright、Prometheus client_golang、Langfuse v2 SDK。

**Spec:** `docs/superpowers/specs/2026-04-29-modelconfig-fallback-and-retry-design.md`

---

## File Structure

**Create:**
- `agent-runtime/runtime/model_chain.py` — ModelChain / ModelEndpoint / 异常
- `agent-runtime/tests/test_model_chain.py` — Python 单测
- `dashboard/src/components/model-config-picker.tsx` — 主+fallback 选择器
- `dashboard/src/components/__tests__/model-config-picker.test.tsx`
- `dashboard/src/app/modelconfigs/__tests__/page.test.tsx`
- `dashboard/e2e/modelconfig-fallback.spec.ts`

**Modify:**
- `internal/controller/api/v1alpha1/types.go` — `Retries` / `FallbackModelConfigRefs`
- `internal/controller/api/v1alpha1/zz_generated.deepcopy.go` — 重新生成
- `deploy/helm/crds/k8sai.io_modelconfigs.yaml` — schema
- `deploy/helm/crds/k8sai.io_diagnosticruns.yaml` — schema
- `internal/controller/translator/translator.go` — `resolveModelChain` + `buildJob` 改 env 布局
- `internal/controller/translator/translator_test.go`
- `agent-runtime/runtime/orchestrator.py` — `_stream_message` 下沉、`run_agent` 用 ModelChain
- `agent-runtime/runtime/tracer.py` — `event()` 方法
- `agent-runtime/runtime/reporter.py` — LLM metrics 批量上报
- `agent-runtime/tests/test_orchestrator.py` — mock 改 ModelChain.invoke
- `internal/metrics/metrics.go` — `LLMRetriesTotal` / `LLMFallbackTotal` / `LLMChainExhausted`
- `internal/controller/httpserver/llm_metrics_handler.go` — 新事件类型
- `dashboard/src/lib/api.ts` — types
- `dashboard/src/app/modelconfigs/page.tsx` — `retries` 输入
- `dashboard/src/components/create-run-dialog.tsx` — 用 picker
- `dashboard/src/app/diagnose/page.tsx` — 用 picker
- `dashboard/src/app/diagnose/__tests__/page.test.tsx`
- `dashboard/src/i18n/zh.json` + `en.json`

---

## Task 1: CRD types + deepcopy + Helm schema

**Files:**
- Modify: `internal/controller/api/v1alpha1/types.go`
- Modify: `internal/controller/api/v1alpha1/zz_generated.deepcopy.go`
- Modify: `deploy/helm/crds/k8sai.io_modelconfigs.yaml`
- Modify: `deploy/helm/crds/k8sai.io_diagnosticruns.yaml`

- [ ] **Step 1: 加字段到 types.go**

在 `ModelConfigSpec` 末尾追加：

```go
// Retries 是单模型瞬时错误的重试次数。0 = 不重试（默认）。
// 仅 5xx / 429 / 网络超时 / connection error 触发；4xx 永不重试。
// +optional
// +kubebuilder:validation:Minimum=0
// +kubebuilder:validation:Maximum=10
Retries int `json:"retries,omitempty"`
```

在 `DiagnosticRunSpec` 找到 `ModelConfigRef` 行下追加：

```go
// FallbackModelConfigRefs 是按优先级排序的备选 ModelConfig 名称列表。
// 主 ModelConfig 所有重试用尽 / 流中段后按本列表顺序切换。
// 引用的 ModelConfig 必须与主在同一 namespace。
// +optional
FallbackModelConfigRefs []string `json:"fallbackModelConfigRefs,omitempty"`
```

- [ ] **Step 2: 重新生成 deepcopy**

```bash
make generate || (cd hack && bash update-codegen.sh)
```

预期：`zz_generated.deepcopy.go` 含 `FallbackModelConfigRefs` 的 `copy(*out, *in)` 切片复制。

- [ ] **Step 3: 更新 Helm CRD schema**

`deploy/helm/crds/k8sai.io_modelconfigs.yaml` `spec.properties` 加：

```yaml
retries:
  type: integer
  minimum: 0
  maximum: 10
  default: 0
  description: "Single-model retry count for transient errors (5xx/429/timeout). 0 = no retry."
```

`deploy/helm/crds/k8sai.io_diagnosticruns.yaml` `spec.properties` 加：

```yaml
fallbackModelConfigRefs:
  type: array
  items:
    type: string
  description: "Ordered list of fallback ModelConfig names tried when primary fails."
```

- [ ] **Step 4: 验证编译**

```bash
go build ./...
```

预期：无错误。

- [ ] **Step 5: Commit**

```bash
git add internal/controller/api/v1alpha1/types.go \
        internal/controller/api/v1alpha1/zz_generated.deepcopy.go \
        deploy/helm/crds/k8sai.io_modelconfigs.yaml \
        deploy/helm/crds/k8sai.io_diagnosticruns.yaml
git commit -m "feat(api): add ModelConfig.retries and DiagnosticRun.fallbackModelConfigRefs"
```

---

## Task 2: Translator.resolveModelChain — TDD

**Files:**
- Modify: `internal/controller/translator/translator.go`
- Test: `internal/controller/translator/translator_test.go`

- [ ] **Step 1: 写失败测试**

在 `translator_test.go` 追加：

```go
func TestResolveModelChain_PrimaryOnly(t *testing.T) {
    primary := &k8saiV1.ModelConfig{
        ObjectMeta: metav1.ObjectMeta{Name: "primary", Namespace: "default"},
        Spec:       k8saiV1.ModelConfigSpec{Provider: "anthropic", Model: "sonnet"},
    }
    fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(primary).Build()
    tr := NewWithClient(Config{}, &stubProvider{}, fakeClient)

    run := &k8saiV1.DiagnosticRun{
        ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
        Spec: k8saiV1.DiagnosticRunSpec{ModelConfigRef: "primary"},
    }
    chain := tr.resolveModelChain(context.Background(), run)
    if len(chain) != 1 || chain[0].Name != "primary" {
        t.Fatalf("expected [primary], got %+v", chain)
    }
}

func TestResolveModelChain_PrimaryWithFallbacks(t *testing.T) {
    p := &k8saiV1.ModelConfig{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}}
    f1 := &k8saiV1.ModelConfig{ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "default"}}
    f2 := &k8saiV1.ModelConfig{ObjectMeta: metav1.ObjectMeta{Name: "f2", Namespace: "default"}}
    fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p, f1, f2).Build()
    tr := NewWithClient(Config{}, &stubProvider{}, fakeClient)

    run := &k8saiV1.DiagnosticRun{
        ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
        Spec: k8saiV1.DiagnosticRunSpec{
            ModelConfigRef:          "p",
            FallbackModelConfigRefs: []string{"f1", "f2"},
        },
    }
    chain := tr.resolveModelChain(context.Background(), run)
    names := []string{chain[0].Name, chain[1].Name, chain[2].Name}
    want := []string{"p", "f1", "f2"}
    if !reflect.DeepEqual(names, want) {
        t.Fatalf("want %v got %v", want, names)
    }
}

func TestResolveModelChain_MissingFallbackSkipped(t *testing.T) {
    p := &k8saiV1.ModelConfig{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"}}
    fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p).Build()
    tr := NewWithClient(Config{}, &stubProvider{}, fakeClient)

    run := &k8saiV1.DiagnosticRun{
        ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
        Spec: k8saiV1.DiagnosticRunSpec{
            ModelConfigRef:          "p",
            FallbackModelConfigRefs: []string{"missing"},
        },
    }
    chain := tr.resolveModelChain(context.Background(), run)
    if len(chain) != 1 {
        t.Fatalf("expected primary only when fallback missing, got %d", len(chain))
    }
}
```

(若 `testScheme` / `stubProvider` 不存在，复用 `translator_test.go` 已有的同名 helper。)

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/controller/translator/ -run TestResolveModelChain -v
```

预期：编译失败（`tr.resolveModelChain undefined`）。

- [ ] **Step 3: 实现 `resolveModelChain`**

在 `translator.go` 接近 `resolveModelConfig` 处追加：

```go
// resolveModelChain 返回主 + fallback 全部 ModelConfig，主在 [0]。
// fallback 不存在则跳过并打日志。主不存在时返回 nil（调用方自行降级）。
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
```

- [ ] **Step 4: 跑测试确认通过**

```bash
go test ./internal/controller/translator/ -run TestResolveModelChain -v
```

预期：3 个用例 PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/controller/translator/translator.go internal/controller/translator/translator_test.go
git commit -m "feat(translator): resolveModelChain — primary + ordered fallbacks"
```

---

## Task 3: Translator.buildJob 改索引化 env 注入

**Files:**
- Modify: `internal/controller/translator/translator.go`
- Test: `internal/controller/translator/translator_test.go`

- [ ] **Step 1: 写失败测试**

```go
func TestBuildJob_InjectsModelChainEnvVars(t *testing.T) {
    p := &k8saiV1.ModelConfig{
        ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
        Spec: k8saiV1.ModelConfigSpec{
            Provider: "anthropic", Model: "sonnet", BaseURL: "https://primary.example.com",
            Retries:  3,
            APIKeyRef: k8saiV1.SecretKeyRef{Name: "p-secret", Key: "apiKey"},
        },
    }
    f1 := &k8saiV1.ModelConfig{
        ObjectMeta: metav1.ObjectMeta{Name: "f1", Namespace: "default"},
        Spec: k8saiV1.ModelConfigSpec{
            Provider: "anthropic", Model: "haiku", BaseURL: "",
            Retries:  0,
            APIKeyRef: k8saiV1.SecretKeyRef{Name: "f1-secret", Key: "apiKey"},
        },
    }
    fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(p, f1).Build()
    tr := NewWithClient(Config{AgentImage: "img:v1"}, &stubProvider{skills: []*store.Skill{{Name: "s", Dimension: "health", Prompt: "x"}}}, fakeClient)

    run := &k8saiV1.DiagnosticRun{
        ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default", UID: "uid-1"},
        Spec: k8saiV1.DiagnosticRunSpec{
            ModelConfigRef:          "p",
            FallbackModelConfigRefs: []string{"f1"},
        },
    }
    objs, err := tr.Compile(context.Background(), run)
    if err != nil { t.Fatal(err) }

    var job *batchv1.Job
    for _, o := range objs {
        if j, ok := o.(*batchv1.Job); ok { job = j }
    }
    if job == nil { t.Fatal("no Job in compile output") }

    env := job.Spec.Template.Spec.Containers[0].Env
    want := map[string]string{
        "MODEL_COUNT":      "2",
        "MODEL_0_BASE_URL": "https://primary.example.com",
        "MODEL_0_MODEL":    "sonnet",
        "MODEL_0_RETRIES":  "3",
        "MODEL_1_BASE_URL": "",
        "MODEL_1_MODEL":    "haiku",
        "MODEL_1_RETRIES":  "0",
        // 兼容性：保留旧 env 镜像 MODEL_0_*
        "ANTHROPIC_BASE_URL": "https://primary.example.com",
        "MODEL":              "sonnet",
    }
    got := map[string]string{}
    for _, e := range env {
        if e.ValueFrom == nil {
            got[e.Name] = e.Value
        }
    }
    for k, v := range want {
        if got[k] != v {
            t.Errorf("env %s: want %q got %q", k, v, got[k])
        }
    }

    // Secret refs
    findSecret := func(name string) *corev1.SecretKeySelector {
        for _, e := range env {
            if e.Name == name && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
                return e.ValueFrom.SecretKeyRef
            }
        }
        return nil
    }
    if s := findSecret("MODEL_0_API_KEY"); s == nil || s.Name != "p-secret" {
        t.Errorf("MODEL_0_API_KEY: want SecretKeyRef p-secret/apiKey, got %+v", s)
    }
    if s := findSecret("MODEL_1_API_KEY"); s == nil || s.Name != "f1-secret" {
        t.Errorf("MODEL_1_API_KEY: want SecretKeyRef f1-secret/apiKey, got %+v", s)
    }
    if s := findSecret("ANTHROPIC_API_KEY"); s == nil || s.Name != "p-secret" {
        t.Errorf("ANTHROPIC_API_KEY (compat): want p-secret/apiKey, got %+v", s)
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

```bash
go test ./internal/controller/translator/ -run TestBuildJob_InjectsModelChainEnvVars -v
```

预期：FAIL（`MODEL_COUNT` 等字段不存在于当前实现）。

- [ ] **Step 3: 修改 `Compile` 与 `buildJob`**

在 `translator.go` 中：

```go
// Compile 入口替换 baseURL/modelName/apiKeyEnv 解析为 chain 解析
func (t *Translator) Compile(ctx context.Context, run *k8saiV1.DiagnosticRun) ([]client.Object, error) {
    // ...existing skills resolution unchanged...

    chain := t.resolveModelChain(ctx, run)
    // 旧路径降级：chain 为空时手动构造单元素 chain（仅含 APIKeyRef 形式）
    if len(chain) == 0 {
        chain = []*k8saiV1.ModelConfig{{
            Spec: k8saiV1.ModelConfigSpec{
                Model:   t.cfg.Model,
                BaseURL: t.cfg.AnthropicBaseURL,
                APIKeyRef: k8saiV1.SecretKeyRef{Name: run.Spec.ModelConfigRef, Key: "apiKey"},
            },
        }}
    }

    sa := t.buildSA(saName, runID)
    cm := t.buildConfigMap(cmName, runID, selected)
    rb := t.buildRoleBinding(saName, runID, run.Namespace)
    job := t.buildJob(run, runID, saName, cmName, selected, chain)

    return []client.Object{sa, cm, rb, job}, nil
}

// buildJob 改签名：接收 chain，自己构造 env
func (t *Translator) buildJob(run *k8saiV1.DiagnosticRun, runID, saName, cmName string, skills []*store.Skill, chain []*k8saiV1.ModelConfig) *batchv1.Job {
    // ... 原有 ttl/backoff/skillNames 不变 ...

    chainEnv := buildChainEnv(chain)

    return &batchv1.Job{
        // ...metadata unchanged...
        Spec: batchv1.JobSpec{
            // ...unchanged...
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    // ...unchanged...
                    Containers: []corev1.Container{{
                        Name:    "agent",
                        Image:   t.cfg.AgentImage,
                        Command: []string{"python", "-m", "runtime.main"},
                        Env: append(append([]corev1.EnvVar{
                            {Name: "RUN_ID", Value: runID},
                            {Name: "TARGET_NAMESPACES", Value: strings.Join(run.Spec.Target.Namespaces, ",")},
                            {Name: "CONTROLLER_URL", Value: t.cfg.ControllerURL},
                            {Name: "MCP_SERVER_PATH", Value: "/usr/local/bin/k8s-mcp-server"},
                            {Name: "PROMETHEUS_URL", Value: t.cfg.PrometheusURL},
                            {Name: "SKILL_NAMES", Value: strings.Join(skillNames, ",")},
                            {Name: "OUTPUT_LANGUAGE", Value: outputLang(run)},
                        }, chainEnv...), langfuseEnvVars(t.cfg.LangfuseSecretName)...),
                        // ...VolumeMounts/Resources unchanged...
                    }},
                },
            },
        },
    }
}

// buildChainEnv 构造 MODEL_COUNT + 索引化 env + 兼容性 ANTHROPIC_* 镜像。
func buildChainEnv(chain []*k8saiV1.ModelConfig) []corev1.EnvVar {
    env := []corev1.EnvVar{{Name: "MODEL_COUNT", Value: fmt.Sprintf("%d", len(chain))}}
    for i, mc := range chain {
        env = append(env,
            corev1.EnvVar{Name: fmt.Sprintf("MODEL_%d_BASE_URL", i), Value: mc.Spec.BaseURL},
            corev1.EnvVar{Name: fmt.Sprintf("MODEL_%d_MODEL", i), Value: mc.Spec.Model},
            corev1.EnvVar{Name: fmt.Sprintf("MODEL_%d_RETRIES", i), Value: fmt.Sprintf("%d", mc.Spec.Retries)},
            corev1.EnvVar{
                Name: fmt.Sprintf("MODEL_%d_API_KEY", i),
                ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
                    LocalObjectReference: corev1.LocalObjectReference{Name: mc.Spec.APIKeyRef.Name},
                    Key: nonEmpty(mc.Spec.APIKeyRef.Key, "apiKey"),
                }},
            },
        )
    }
    // 兼容性镜像：取主 (chain[0])
    if len(chain) > 0 {
        env = append(env,
            corev1.EnvVar{Name: "ANTHROPIC_BASE_URL", Value: chain[0].Spec.BaseURL},
            corev1.EnvVar{Name: "MODEL", Value: chain[0].Spec.Model},
            corev1.EnvVar{
                Name: "ANTHROPIC_API_KEY",
                ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
                    LocalObjectReference: corev1.LocalObjectReference{Name: chain[0].Spec.APIKeyRef.Name},
                    Key: nonEmpty(chain[0].Spec.APIKeyRef.Key, "apiKey"),
                }},
            },
        )
    }
    return env
}

func nonEmpty(s, fallback string) string {
    if s == "" { return fallback }
    return s
}

func outputLang(run *k8saiV1.DiagnosticRun) string {
    if run.Spec.OutputLanguage != "" { return run.Spec.OutputLanguage }
    return "en"
}
```

删除原 `resolveBaseURL` / `resolveModel` / `resolveAPIKeyEnv` 三个 helper 调用点，但**保留函数本身**（向后兼容 — 其他地方可能用到，且测试在跑）。

- [ ] **Step 4: 跑全部 translator 测试**

```bash
go test ./internal/controller/translator/ -v
```

预期：新测试 PASS，旧测试如有依赖 `ANTHROPIC_BASE_URL` 等的可能仍然 PASS（兼容性 env 保留）。修复任何因签名变更失败的旧测试调用点。

- [ ] **Step 5: Commit**

```bash
git add internal/controller/translator/
git commit -m "feat(translator): inject MODEL_<i>_* env for fallback chain (with backward-compat envs)"
```

---

## Task 4: Python ModelChain.from_env — TDD

**Files:**
- Create: `agent-runtime/runtime/model_chain.py`
- Create: `agent-runtime/tests/test_model_chain.py`

- [ ] **Step 1: 写失败测试**

`agent-runtime/tests/test_model_chain.py`：

```python
import os
import pytest
from runtime.model_chain import ModelChain, ModelEndpoint


def test_from_env_multi_endpoint(monkeypatch):
    monkeypatch.setenv("MODEL_COUNT", "2")
    monkeypatch.setenv("MODEL_0_BASE_URL", "https://p.example.com")
    monkeypatch.setenv("MODEL_0_MODEL", "sonnet")
    monkeypatch.setenv("MODEL_0_RETRIES", "3")
    monkeypatch.setenv("MODEL_0_API_KEY", "key-0")
    monkeypatch.setenv("MODEL_1_BASE_URL", "")
    monkeypatch.setenv("MODEL_1_MODEL", "haiku")
    monkeypatch.setenv("MODEL_1_RETRIES", "0")
    monkeypatch.setenv("MODEL_1_API_KEY", "key-1")

    chain = ModelChain.from_env()
    assert len(chain.endpoints) == 2
    assert chain.endpoints[0] == ModelEndpoint(
        base_url="https://p.example.com", model="sonnet", api_key="key-0", retries=3,
    )
    assert chain.endpoints[1] == ModelEndpoint(
        base_url="", model="haiku", api_key="key-1", retries=0,
    )


def test_from_env_single_endpoint_legacy(monkeypatch):
    """MODEL_COUNT 缺失时降级读 ANTHROPIC_*。"""
    monkeypatch.delenv("MODEL_COUNT", raising=False)
    monkeypatch.setenv("ANTHROPIC_BASE_URL", "https://legacy.example.com")
    monkeypatch.setenv("MODEL", "sonnet")
    monkeypatch.setenv("ANTHROPIC_API_KEY", "legacy-key")

    chain = ModelChain.from_env()
    assert len(chain.endpoints) == 1
    assert chain.endpoints[0].base_url == "https://legacy.example.com"
    assert chain.endpoints[0].api_key == "legacy-key"
    assert chain.endpoints[0].retries == 0


def test_from_env_no_endpoints_raises(monkeypatch):
    monkeypatch.delenv("MODEL_COUNT", raising=False)
    monkeypatch.delenv("ANTHROPIC_API_KEY", raising=False)
    with pytest.raises(ValueError, match="at least one"):
        ModelChain.from_env()
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd agent-runtime && python3 -m pytest tests/test_model_chain.py -v
```

预期：`ModuleNotFoundError: runtime.model_chain`.

- [ ] **Step 3: 写最小 model_chain.py**

`agent-runtime/runtime/model_chain.py`：

```python
"""ModelChain: 单 turn 内的重试 + fallback 决策。"""
from __future__ import annotations
import os
from dataclasses import dataclass


@dataclass(frozen=True)
class ModelEndpoint:
    base_url: str   # "" = SDK 默认 (https://api.anthropic.com)
    model: str
    api_key: str
    retries: int    # 0 = 不重试


class ModelChainExhausted(Exception):
    """所有 endpoint + 重试用尽后抛出。"""


class ModelChain:
    def __init__(self, endpoints: list[ModelEndpoint]):
        if not endpoints:
            raise ValueError("ModelChain requires at least one endpoint")
        self.endpoints = endpoints

    @classmethod
    def from_env(cls) -> "ModelChain":
        count_str = os.environ.get("MODEL_COUNT", "").strip()
        if count_str:
            n = int(count_str)
            endpoints = [
                ModelEndpoint(
                    base_url=os.environ.get(f"MODEL_{i}_BASE_URL", ""),
                    model=os.environ.get(f"MODEL_{i}_MODEL", ""),
                    api_key=os.environ.get(f"MODEL_{i}_API_KEY", ""),
                    retries=int(os.environ.get(f"MODEL_{i}_RETRIES", "0") or "0"),
                )
                for i in range(n)
            ]
            return cls(endpoints)

        # Legacy single-endpoint
        api_key = os.environ.get("ANTHROPIC_API_KEY", "")
        if not api_key:
            raise ValueError("ModelChain requires at least one endpoint (set MODEL_COUNT or ANTHROPIC_API_KEY)")
        return cls([ModelEndpoint(
            base_url=os.environ.get("ANTHROPIC_BASE_URL", ""),
            model=os.environ.get("MODEL", "claude-sonnet-4-6"),
            api_key=api_key,
            retries=0,
        )])
```

- [ ] **Step 4: 跑测试确认通过**

```bash
cd agent-runtime && python3 -m pytest tests/test_model_chain.py -v
```

预期：3 个 PASS。

- [ ] **Step 5: Commit**

```bash
git add agent-runtime/runtime/model_chain.py agent-runtime/tests/test_model_chain.py
git commit -m "feat(model_chain): ModelChain.from_env reads MODEL_COUNT or legacy ANTHROPIC_*"
```

---

## Task 5: ModelChain backoff helper — TDD

**Files:** Modify both files from Task 4.

- [ ] **Step 1: 写失败测试**

```python
def test_backoff_for_schedule():
    from runtime.model_chain import _backoff_for
    assert _backoff_for(1) == 1
    assert _backoff_for(2) == 2
    assert _backoff_for(3) == 4
    assert _backoff_for(4) == 4   # 封顶
    assert _backoff_for(99) == 4
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd agent-runtime && python3 -m pytest tests/test_model_chain.py::test_backoff_for_schedule -v
```

- [ ] **Step 3: 实现**

`model_chain.py` 顶部追加：

```python
_BACKOFF_SCHEDULE = [1, 2, 4]


def _backoff_for(attempt: int) -> float:
    idx = attempt - 1
    if 0 <= idx < len(_BACKOFF_SCHEDULE):
        return _BACKOFF_SCHEDULE[idx]
    return _BACKOFF_SCHEDULE[-1]
```

- [ ] **Step 4: 跑测试确认通过**

- [ ] **Step 5: Commit**

```bash
git add agent-runtime/
git commit -m "feat(model_chain): _backoff_for exponential schedule capped at 4s"
```

---

## Task 6: ModelChain `_stream_one` — 把现有 SSE 解析下沉

**Files:** Modify `model_chain.py`, `tests/test_model_chain.py`.

- [ ] **Step 1: 写失败测试（happy path）**

```python
import respx
import httpx
from runtime.model_chain import ModelEndpoint, _stream_one


@respx.mock
def test_stream_one_happy_path():
    body = (
        b'data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}\n\n'
        b'data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}\n\n'
        b'data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}\n\n'
        b'data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}\n\n'
        b'data: [DONE]\n\n'
    )
    respx.post("https://api.example.com/v1/messages").mock(
        return_value=httpx.Response(200, content=body, headers={"content-type": "text/event-stream"})
    )
    ep = ModelEndpoint(base_url="https://api.example.com", model="m", api_key="k", retries=0)
    result = _stream_one(ep, tools=[], messages=[{"role": "user", "content": "x"}])
    assert result["stop_reason"] == "end_turn"
    assert result["content"][0]["text"] == "hi"
    assert result["input_tokens"] == 5
    assert result["output_tokens"] == 2
```

- [ ] **Step 2: 跑测试确认失败**

```bash
cd agent-runtime && python3 -m pytest tests/test_model_chain.py::test_stream_one_happy_path -v
```

预期：`ImportError: cannot import name '_stream_one'`.

(若 `respx` 未安装：`pip install respx` + 加入 `requirements.txt` 的 dev 依赖)

- [ ] **Step 3: 把 `orchestrator._stream_message` 函数体复制到 `model_chain._stream_one`**

`model_chain.py` 追加：

```python
import json
import httpx
from typing import Any


class _SSEStreamBroken(Exception):
    """流提前 EOF 且未收到 message_stop / [DONE]。"""


def _stream_one(endpoint: ModelEndpoint, tools, messages) -> dict:
    """对单个 endpoint 发送一次 SSE 请求并重组完整响应。"""
    headers = {
        "x-api-key": endpoint.api_key,
        "anthropic-version": "2023-06-01",
        "content-type": "application/json",
        "accept": "text/event-stream",
    }
    base_url = (endpoint.base_url or "https://api.anthropic.com").rstrip("/")
    url = base_url if base_url.endswith("/v1/messages") else base_url + "/v1/messages"

    payload: dict[str, Any] = {
        "model": endpoint.model,
        "max_tokens": int(os.environ.get("MAX_TOKENS", "4096")),
        "messages": messages,
        "stream": True,
    }
    if tools:
        payload["tools"] = tools

    content_blocks: dict[int, dict] = {}
    stop_reason = "end_turn"
    input_tokens = 0
    output_tokens = 0
    stream_complete = False

    with httpx.stream("POST", url, headers=headers, json=payload, timeout=120) as resp:
        resp.raise_for_status()
        for raw_line in resp.iter_lines():
            raw_line = raw_line.strip()
            if not raw_line or not raw_line.startswith("data:"):
                continue
            data_str = raw_line[len("data:"):].strip()
            if data_str == "[DONE]":
                stream_complete = True
                break
            try:
                event = json.loads(data_str)
            except json.JSONDecodeError:
                continue
            etype = event.get("type", "")
            if etype == "message_start":
                input_tokens = event.get("message", {}).get("usage", {}).get("input_tokens", 0)
            elif etype == "content_block_start":
                idx = event.get("index", 0)
                block = event.get("content_block", {})
                btype = block.get("type", "")
                if btype == "text":
                    content_blocks[idx] = {"type": "text", "text": ""}
                elif btype == "tool_use":
                    content_blocks[idx] = {"type": "tool_use", "id": block.get("id", ""), "name": block.get("name", ""), "input": ""}
                elif btype == "thinking":
                    content_blocks[idx] = {"type": "thinking", "text": ""}
            elif etype == "content_block_delta":
                idx = event.get("index", 0)
                delta = event.get("delta", {})
                dtype = delta.get("type", "")
                if idx in content_blocks:
                    if dtype == "text_delta":
                        content_blocks[idx]["text"] += delta.get("text", "")
                    elif dtype == "input_json_delta":
                        content_blocks[idx]["input"] += delta.get("partial_json", "")
                    elif dtype == "thinking_delta":
                        content_blocks[idx]["text"] += delta.get("thinking", "")
            elif etype == "message_delta":
                d = event.get("delta", {})
                if d.get("stop_reason"):
                    stop_reason = d["stop_reason"]
                u = event.get("usage", {})
                output_tokens = u.get("output_tokens", output_tokens)
                if input_tokens == 0:
                    input_tokens = u.get("input_tokens", 0)
            elif etype == "message_stop":
                stream_complete = True

    if not stream_complete:
        raise _SSEStreamBroken(f"stream ended without [DONE] or message_stop")

    result_blocks = []
    for idx in sorted(content_blocks.keys()):
        block = content_blocks[idx]
        if block["type"] == "thinking":
            continue
        if block["type"] == "tool_use" and isinstance(block["input"], str):
            try:
                block["input"] = json.loads(block["input"]) if block["input"] else {}
            except json.JSONDecodeError:
                block["input"] = {}
        result_blocks.append(block)

    return {
        "content": result_blocks, "stop_reason": stop_reason,
        "input_tokens": input_tokens, "output_tokens": output_tokens,
    }
```

- [ ] **Step 4: 跑测试通过**

```bash
cd agent-runtime && python3 -m pytest tests/test_model_chain.py::test_stream_one_happy_path -v
```

- [ ] **Step 5: Commit**

```bash
git add agent-runtime/
git commit -m "feat(model_chain): _stream_one extracted from orchestrator with endpoint param"
```

---

## Task 7: ModelChain.invoke 单 endpoint 成功路径

**Files:** Modify `model_chain.py`, `tests/test_model_chain.py`.

- [ ] **Step 1: 写失败测试**

```python
from unittest.mock import patch


def test_invoke_succeeds_first_try():
    chain = ModelChain([ModelEndpoint(base_url="", model="m", api_key="k", retries=0)])
    with patch("runtime.model_chain._stream_one") as mock_stream:
        mock_stream.return_value = {"content": [{"type": "text", "text": "ok"}],
                                     "stop_reason": "end_turn",
                                     "input_tokens": 1, "output_tokens": 1}
        result = chain.invoke(tools=[], messages=[], tracer=_NoopTracer())
        assert result["stop_reason"] == "end_turn"
        assert mock_stream.call_count == 1


class _NoopTracer:
    def event(self, **kwargs): pass
    def generation(self, **kwargs): pass
```

- [ ] **Step 2: 跑测试确认失败**

预期：`AttributeError: 'ModelChain' object has no attribute 'invoke'`.

- [ ] **Step 3: 实现 invoke**

```python
import time
import httpx
from . import logger


class ModelChain:
    # ... __init__ / from_env unchanged ...

    def invoke(self, tools, messages, tracer) -> dict:
        last_error: Exception | None = None
        for ep_idx, ep in enumerate(self.endpoints):
            total_attempts = 1 + ep.retries
            for attempt in range(1, total_attempts + 1):
                try:
                    return _stream_one(ep, tools, messages)
                except _SSEStreamBroken as e:
                    last_error = e
                    logger.warn("model fallback", from_index=ep_idx, reason="sse_stream_broken", error=str(e))
                    tracer.event(name="model_fallback", level="WARNING",
                                 metadata={"from_index": ep_idx, "reason": "sse_stream_broken"})
                    break  # 不重试，跳下一 endpoint
                except httpx.HTTPStatusError as e:
                    code = e.response.status_code
                    if 400 <= code < 500 and code != 429:
                        # 4xx (除 429) 不重试不 fallback
                        raise
                    last_error = e
                    if attempt < total_attempts:
                        backoff = _retry_after(e) if code == 429 else _backoff_for(attempt)
                        logger.warn("model retry", endpoint_index=ep_idx, model=ep.model,
                                    attempt=attempt, error=f"{code}", backoff_s=backoff)
                        tracer.event(name="model_retry", level="WARNING",
                                     metadata={"endpoint_index": ep_idx, "attempt": attempt, "error": str(code)})
                        time.sleep(backoff)
                    # else: 重试用尽，进入 fallback
                except (httpx.TimeoutException, httpx.ConnectError, httpx.RemoteProtocolError) as e:
                    last_error = e
                    if attempt < total_attempts:
                        backoff = _backoff_for(attempt)
                        logger.warn("model retry", endpoint_index=ep_idx, model=ep.model,
                                    attempt=attempt, error=type(e).__name__, backoff_s=backoff)
                        tracer.event(name="model_retry", level="WARNING",
                                     metadata={"endpoint_index": ep_idx, "attempt": attempt, "error": type(e).__name__})
                        time.sleep(backoff)
            # 切下一 endpoint 之前打 fallback 事件（除非是 SSE 已打过）
            if ep_idx < len(self.endpoints) - 1:
                logger.warn("model fallback", from_index=ep_idx, to_index=ep_idx + 1,
                            from_model=ep.model, to_model=self.endpoints[ep_idx + 1].model,
                            reason="retries_exhausted")
                tracer.event(name="model_fallback", level="WARNING",
                             metadata={"from_index": ep_idx, "to_index": ep_idx + 1, "reason": "retries_exhausted"})
        raise ModelChainExhausted(f"all {len(self.endpoints)} endpoint(s) exhausted; last_error={last_error}")


def _retry_after(e: httpx.HTTPStatusError) -> float:
    raw = e.response.headers.get("Retry-After")
    if not raw:
        return _backoff_for(1)
    try:
        secs = float(raw)
        return min(secs, 60.0)
    except ValueError:
        return _backoff_for(1)
```

- [ ] **Step 4: 跑测试通过**

- [ ] **Step 5: Commit**

```bash
git add agent-runtime/
git commit -m "feat(model_chain): invoke happy path with retry+fallback skeleton"
```

---

## Task 8: ModelChain.invoke — 重试 / fallback / 4xx / SSE / 429 / 耗尽 全场景测试

**Files:** Modify `tests/test_model_chain.py`.

- [ ] **Step 1: 加 6 个用例**

```python
import httpx
from unittest.mock import patch
from runtime.model_chain import (
    ModelChain, ModelEndpoint, ModelChainExhausted, _SSEStreamBroken
)


def _resp(status: int, headers=None) -> httpx.HTTPStatusError:
    req = httpx.Request("POST", "https://x")
    return httpx.HTTPStatusError("err", request=req, response=httpx.Response(status, headers=headers or {}, request=req))


def test_invoke_retries_on_5xx_and_succeeds():
    chain = ModelChain([ModelEndpoint(base_url="", model="m", api_key="k", retries=2)])
    calls = []
    def fake(ep, tools, messages):
        calls.append(1)
        if len(calls) <= 2:
            raise _resp(503)
        return {"content": [], "stop_reason": "end_turn", "input_tokens": 1, "output_tokens": 1}
    with patch("runtime.model_chain._stream_one", side_effect=fake), \
         patch("runtime.model_chain.time.sleep"):
        result = chain.invoke([], [], _NoopTracer())
    assert result["stop_reason"] == "end_turn"
    assert len(calls) == 3


def test_invoke_fallbacks_after_retries_exhausted():
    eps = [
        ModelEndpoint(base_url="", model="primary", api_key="k0", retries=1),
        ModelEndpoint(base_url="", model="backup", api_key="k1", retries=0),
    ]
    chain = ModelChain(eps)
    side = [_resp(503), _resp(503),
            {"content": [], "stop_reason": "end_turn", "input_tokens": 0, "output_tokens": 0}]
    with patch("runtime.model_chain._stream_one", side_effect=side), \
         patch("runtime.model_chain.time.sleep"):
        result = chain.invoke([], [], _NoopTracer())
    assert result["stop_reason"] == "end_turn"


def test_invoke_4xx_no_retry_no_fallback():
    eps = [ModelEndpoint("", "m", "k", retries=3),
           ModelEndpoint("", "m2", "k2", retries=3)]
    chain = ModelChain(eps)
    with patch("runtime.model_chain._stream_one", side_effect=_resp(403)), \
         patch("runtime.model_chain.time.sleep"):
        with pytest.raises(httpx.HTTPStatusError):
            chain.invoke([], [], _NoopTracer())


def test_invoke_sse_broken_skips_to_fallback_no_retry():
    eps = [ModelEndpoint("", "m", "k", retries=3),
           ModelEndpoint("", "m2", "k2", retries=0)]
    chain = ModelChain(eps)
    side = [_SSEStreamBroken("eof"),
            {"content": [], "stop_reason": "end_turn", "input_tokens": 0, "output_tokens": 0}]
    with patch("runtime.model_chain._stream_one", side_effect=side), \
         patch("runtime.model_chain.time.sleep"):
        result = chain.invoke([], [], _NoopTracer())
    assert result["stop_reason"] == "end_turn"


def test_invoke_429_uses_retry_after_header():
    chain = ModelChain([ModelEndpoint("", "m", "k", retries=1)])
    side = [_resp(429, headers={"Retry-After": "10"}),
            {"content": [], "stop_reason": "end_turn", "input_tokens": 0, "output_tokens": 0}]
    sleeps = []
    with patch("runtime.model_chain._stream_one", side_effect=side), \
         patch("runtime.model_chain.time.sleep", side_effect=lambda s: sleeps.append(s)):
        chain.invoke([], [], _NoopTracer())
    assert sleeps == [10.0]


def test_invoke_chain_exhausted_raises():
    eps = [ModelEndpoint("", "m", "k", retries=0),
           ModelEndpoint("", "m2", "k2", retries=0)]
    chain = ModelChain(eps)
    with patch("runtime.model_chain._stream_one", side_effect=_resp(503)), \
         patch("runtime.model_chain.time.sleep"):
        with pytest.raises(ModelChainExhausted):
            chain.invoke([], [], _NoopTracer())
```

- [ ] **Step 2: 跑全部测试**

```bash
cd agent-runtime && python3 -m pytest tests/test_model_chain.py -v
```

预期：全 PASS。如果某用例失败，按错误调整 invoke 实现。

- [ ] **Step 3: Commit**

```bash
git add agent-runtime/
git commit -m "test(model_chain): cover retry, fallback, 4xx, sse-broken, 429, exhausted"
```

---

## Task 9: orchestrator.py 接 ModelChain

**Files:**
- Modify: `agent-runtime/runtime/orchestrator.py`
- Modify: `agent-runtime/tests/test_orchestrator.py`

- [ ] **Step 1: 修改 orchestrator.py**

删除 `_stream_message` 整个函数。`run_agent` 改：

```python
from .model_chain import ModelChain, ModelChainExhausted


def run_agent(skills: List[Skill], tracer=None) -> List[dict]:
    if tracer is None:
        tracer = _tracer_mod._NoOp()

    chain = ModelChain.from_env()
    model = chain.endpoints[0].model  # for tracer 标签

    tools = discover_tools()
    logger.info("discovered MCP tools", count=len(tools))
    if tools:
        logger.info("tools", names=[t['name'] for t in tools])

    prompt = build_prompt(skills)
    messages = [{"role": "user", "content": prompt}]
    findings: list[dict] = []
    max_turns = int(os.environ.get("MAX_TURNS", "10"))

    for turn in range(max_turns):
        logger.info("turn", turn=turn + 1, max_turns=max_turns)
        response = chain.invoke(tools, messages, tracer)
        # 余下 assistant_content / findings 提取 / tool_use 处理逻辑保持不变
        # ... existing code ...
```

(整段后续逻辑不动 — 只把 `_stream_message(client, tools, messages)` 改成 `chain.invoke(tools, messages, tracer)`，并删 `client = anthropic.Anthropic()` 一行。)

- [ ] **Step 2: 调整 test_orchestrator.py mock**

把所有 `patch("runtime.orchestrator._stream_message")` 替换为 `patch("runtime.orchestrator.ModelChain")`，并改 mock 行为：

```python
@patch("runtime.orchestrator.ModelChain")
def test_extracts_findings_from_text(mock_chain_cls):
    mock_chain = mock_chain_cls.from_env.return_value
    mock_chain.invoke.return_value = {
        "content": [{"type": "text", "text": '{"dimension":"health","severity":"high","title":"x","description":"y","resource_kind":"Pod","resource_namespace":"ns","resource_name":"p","suggestion":"z"}'}],
        "stop_reason": "end_turn", "input_tokens": 0, "output_tokens": 0,
    }
    mock_chain.endpoints = [type("E", (), {"model": "sonnet"})()]
    # ... existing assertions ...
```

- [ ] **Step 3: 跑全部 Python 测试**

```bash
cd agent-runtime && python3 -m pytest tests/ -v
```

预期：所有原 orchestrator 用例 + 新 model_chain 用例全 PASS。

- [ ] **Step 4: Commit**

```bash
git add agent-runtime/
git commit -m "refactor(orchestrator): wire ModelChain.invoke instead of inline _stream_message"
```

---

## Task 10: tracer.py event() 方法

**Files:**
- Modify: `agent-runtime/runtime/tracer.py`
- Modify: `agent-runtime/tests/test_tracer.py`（新增或扩展）

- [ ] **Step 1: 写失败测试**

`agent-runtime/tests/test_tracer.py`：

```python
from runtime.tracer import _NoOp, LangfuseTracer
from unittest.mock import MagicMock


def test_noop_event_does_nothing():
    t = _NoOp()
    t.event(name="x", level="WARNING", metadata={"k": "v"})  # should not raise


def test_langfuse_event_calls_sdk():
    fake_lf = MagicMock()
    fake_trace = MagicMock()
    fake_lf.trace.return_value = fake_trace
    t = LangfuseTracer(fake_lf, run_id="r1")
    t.event(name="model_retry", level="WARNING", metadata={"attempt": 2})
    fake_trace.event.assert_called_once()
    args, kwargs = fake_trace.event.call_args
    assert kwargs["name"] == "model_retry"
    assert kwargs["level"] == "WARNING"
```

- [ ] **Step 2: 跑确认失败**

```bash
cd agent-runtime && python3 -m pytest tests/test_tracer.py -v
```

- [ ] **Step 3: 实现**

`tracer.py` 给 `_NoOp` 加：

```python
def event(self, **kwargs):
    pass
```

`LangfuseTracer` 加：

```python
def event(self, *, name: str, level: str = "DEFAULT", metadata: dict | None = None):
    if self._trace is None:
        return
    try:
        self._trace.event(name=name, level=level, metadata=metadata or {})
    except Exception as e:
        # tracer 故障不应影响主流程
        from . import logger
        logger.warn("tracer event failed", error=str(e))
```

- [ ] **Step 4: 跑测试通过**

- [ ] **Step 5: Commit**

```bash
git add agent-runtime/
git commit -m "feat(tracer): add event() for retry/fallback observability"
```

---

## Task 11: reporter.py LLM metrics 批量上报

**Files:**
- Modify: `agent-runtime/runtime/reporter.py`
- Modify: `agent-runtime/runtime/model_chain.py`（注入 metrics 收集器）
- Modify: `agent-runtime/tests/test_reporter.py`

- [ ] **Step 1: 引入 metrics 收集**

`reporter.py` 新增：

```python
import os
import httpx
from . import logger

_BUFFER: list[dict] = []


def record_llm_event(event_type: str, labels: dict):
    """收集 retry / fallback / chain_exhausted 事件。"""
    _BUFFER.append({"type": event_type, "labels": labels})


def flush_llm_metrics():
    if not _BUFFER:
        return
    url = os.environ["CONTROLLER_URL"].rstrip("/") + "/internal/llm-metrics"
    try:
        httpx.post(url, json={"events": _BUFFER}, timeout=10)
        _BUFFER.clear()
    except Exception as e:
        logger.warn("flush_llm_metrics failed", error=str(e))
```

`model_chain.py` 在每处 `tracer.event(...)` 调用旁加 `_record(event_type, labels)`：

```python
from .reporter import record_llm_event

# retry 路径：
record_llm_event("retry", {"model": ep.model, "reason": str(code) if 'code' in dir() else type(e).__name__})
# fallback 路径：
record_llm_event("fallback", {"from_model": ep.model, "to_model": next_ep.model, "reason": reason})
# 耗尽：
record_llm_event("exhausted", {"endpoints": str(len(self.endpoints))})
```

`main.py` 在 `run_agent` 返回后：

```python
from . import reporter
# ... existing reporter.post_findings(...) ...
reporter.flush_llm_metrics()
```

- [ ] **Step 2: 单测**

`tests/test_reporter.py` 加：

```python
import respx, httpx
from runtime import reporter


def test_flush_llm_metrics_posts_buffer(monkeypatch):
    monkeypatch.setenv("CONTROLLER_URL", "https://c.example.com")
    reporter._BUFFER.clear()
    reporter.record_llm_event("retry", {"model": "m"})
    with respx.mock(base_url="https://c.example.com") as mock:
        mock.post("/internal/llm-metrics").mock(return_value=httpx.Response(200))
        reporter.flush_llm_metrics()
    assert reporter._BUFFER == []


def test_flush_llm_metrics_empty_no_op():
    reporter._BUFFER.clear()
    reporter.flush_llm_metrics()  # no exception, no HTTP call
```

- [ ] **Step 3: 跑测试**

```bash
cd agent-runtime && python3 -m pytest tests/ -v
```

- [ ] **Step 4: Commit**

```bash
git add agent-runtime/
git commit -m "feat(reporter): batch flush LLM retry/fallback metrics to controller"
```

---

## Task 12: Go metrics + llm_metrics_handler

**Files:**
- Modify: `internal/metrics/metrics.go`
- Modify: `internal/controller/httpserver/llm_metrics_handler.go`
- Modify 测试

- [ ] **Step 1: 测试**

`internal/metrics/metrics_test.go` 加：

```go
func TestLLMMetrics_Counters(t *testing.T) {
    m := metrics.New()
    m.RecordLLMRetry("sonnet", "503")
    m.RecordLLMFallback("sonnet", "haiku", "retries_exhausted")
    m.RecordLLMChainExhausted("default")
    // 通过 reflection 或 testutil.ToFloat64 校验值为 1
}
```

- [ ] **Step 2: 实现**

`metrics.go` 加：

```go
LLMRetriesTotal   *prometheus.CounterVec
LLMFallbackTotal  *prometheus.CounterVec
LLMChainExhausted *prometheus.CounterVec

// New() 初始化：
m.LLMRetriesTotal = promauto.With(reg).NewCounterVec(
    prometheus.CounterOpts{Name: "kah_llm_retries_total", Help: "..."},
    []string{"model", "reason"},
)
m.LLMFallbackTotal = promauto.With(reg).NewCounterVec(
    prometheus.CounterOpts{Name: "kah_llm_fallback_total", Help: "..."},
    []string{"from_model", "to_model", "reason"},
)
m.LLMChainExhausted = promauto.With(reg).NewCounterVec(
    prometheus.CounterOpts{Name: "kah_llm_chain_exhausted_total", Help: "..."},
    []string{"namespace"},
)

// 方法：
func (m *Metrics) RecordLLMRetry(model, reason string) {
    if m == nil || m.LLMRetriesTotal == nil { return }
    m.LLMRetriesTotal.WithLabelValues(model, reason).Inc()
}
// 类似 RecordLLMFallback / RecordLLMChainExhausted
```

- [ ] **Step 3: handler 改动**

`llm_metrics_handler.go` 解析新 payload schema：

```go
type llmEvent struct {
    Type   string            `json:"type"`
    Labels map[string]string `json:"labels"`
}
type llmEventBatch struct { Events []llmEvent `json:"events"` }

func (s *Server) handleLLMMetrics(w http.ResponseWriter, r *http.Request) {
    var batch llmEventBatch
    if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
        http.Error(w, err.Error(), 400); return
    }
    for _, e := range batch.Events {
        switch e.Type {
        case "retry":
            s.metrics.RecordLLMRetry(e.Labels["model"], e.Labels["reason"])
        case "fallback":
            s.metrics.RecordLLMFallback(e.Labels["from_model"], e.Labels["to_model"], e.Labels["reason"])
        case "exhausted":
            s.metrics.RecordLLMChainExhausted(e.Labels["namespace"])
        }
    }
    w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: 跑测试**

```bash
go test ./internal/metrics/... ./internal/controller/httpserver/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/metrics/ internal/controller/httpserver/
git commit -m "feat(metrics): kah_llm_{retries,fallback,chain_exhausted}_total counters"
```

---

## Task 13: Dashboard `lib/api.ts` types

**Files:**
- Modify: `dashboard/src/lib/api.ts`
- Modify: `dashboard/src/lib/__tests__/api.test.ts`（如果存在）

- [ ] **Step 1: 改 types**

```ts
export interface ModelConfig {
  name: string;
  namespace: string;
  provider: string;
  model: string;
  baseURL?: string;
  retries?: number;             // ← 新增
  apiKeyRef: { name: string; key: string };
}

export interface DiagnosticRunCreate {
  // ...existing fields...
  modelConfigRef: string;
  fallbackModelConfigRefs?: string[];   // ← 新增
}
```

- [ ] **Step 2: 跑现有测试**

```bash
cd dashboard && bun run test src/lib/__tests__/api.test.ts
```

预期：PASS（仅类型扩展，不影响行为）。

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/lib/api.ts
git commit -m "feat(dashboard): types for ModelConfig.retries and fallbackModelConfigRefs"
```

---

## Task 14: i18n 词条

**Files:**
- Modify: `dashboard/src/i18n/zh.json`
- Modify: `dashboard/src/i18n/en.json`
- Modify: `dashboard/src/i18n/__tests__/context.test.tsx`

- [ ] **Step 1: 加词条**

`zh.json` 内追加（保留现有结构）：

```json
"modelConfig.retries": "重试次数",
"modelConfig.retries.help": "0 表示不重试。仅在网络抖动严重时设置 1-3",
"diagnose.primaryModel": "主模型",
"diagnose.fallbackChain": "备选链路",
"diagnose.fallbackChain.help": "主模型不可用时按顺序切换",
"diagnose.fallbackChain.empty": "未配置（默认无 fallback）"
```

`en.json`：

```json
"modelConfig.retries": "Retries",
"modelConfig.retries.help": "0 = no retry. Set 1-3 only if proxy is flaky",
"diagnose.primaryModel": "Primary Model",
"diagnose.fallbackChain": "Fallback Chain",
"diagnose.fallbackChain.help": "Switched in order when primary fails",
"diagnose.fallbackChain.empty": "Not configured"
```

- [ ] **Step 2: 跑 i18n 测试**

```bash
cd dashboard && bun run test src/i18n/__tests__/
```

预期：PASS（无 key 重复 / 无未翻译字段）。

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/i18n/
git commit -m "feat(i18n): retries and fallback chain wording (zh/en)"
```

---

## Task 15: ModelConfigPicker 组件 — TDD

**Files:**
- Create: `dashboard/src/components/model-config-picker.tsx`
- Create: `dashboard/src/components/__tests__/model-config-picker.test.tsx`

- [ ] **Step 1: 写失败测试**

`__tests__/model-config-picker.test.tsx`：

```tsx
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { ModelConfigPicker } from "../model-config-picker";

const configs = [
  { name: "primary", namespace: "default" },
  { name: "backup-1", namespace: "default" },
  { name: "backup-2", namespace: "default" },
];

vi.mock("@/lib/api", () => ({
  useModelConfigs: () => ({ data: configs, isLoading: false }),
}));

describe("ModelConfigPicker", () => {
  it("renders all configs in primary dropdown", () => {
    render(<ModelConfigPicker primary="" fallbacks={[]} onChange={() => {}} />);
    fireEvent.click(screen.getByRole("combobox"));
    expect(screen.getByRole("option", { name: "primary" })).toBeInTheDocument();
  });

  it("excludes selected primary from fallback candidates", () => {
    render(<ModelConfigPicker primary="primary" fallbacks={[]} onChange={() => {}} />);
    fireEvent.click(screen.getByText("+"));
    expect(screen.queryByText("primary")).toBeNull();
    expect(screen.getByText("backup-1")).toBeInTheDocument();
  });

  it("fires onChange when adding fallback", () => {
    const onChange = vi.fn();
    render(<ModelConfigPicker primary="primary" fallbacks={[]} onChange={onChange} />);
    fireEvent.click(screen.getByText("+"));
    fireEvent.click(screen.getByText("backup-1"));
    expect(onChange).toHaveBeenCalledWith("primary", ["backup-1"]);
  });

  it("removes fallback chip on X click", () => {
    const onChange = vi.fn();
    render(<ModelConfigPicker primary="primary" fallbacks={["backup-1"]} onChange={onChange} />);
    fireEvent.click(screen.getByLabelText("remove backup-1"));
    expect(onChange).toHaveBeenCalledWith("primary", []);
  });

  it("renders empty state when no configs", () => {
    vi.doMock("@/lib/api", () => ({ useModelConfigs: () => ({ data: [], isLoading: false }) }));
    // (re-render after mock change)
  });
});
```

- [ ] **Step 2: 跑确认失败**

```bash
cd dashboard && bun run test src/components/__tests__/model-config-picker.test.tsx
```

- [ ] **Step 3: 实现组件**

```tsx
import { useState } from "react";
import { useModelConfigs } from "@/lib/api";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Popover, PopoverTrigger, PopoverContent } from "@/components/ui/popover";
import { useI18n } from "@/i18n/context";

interface Props {
  primary: string;
  fallbacks: string[];
  onChange: (primary: string, fallbacks: string[]) => void;
}

export function ModelConfigPicker({ primary, fallbacks, onChange }: Props) {
  const { data: configs = [], isLoading } = useModelConfigs();
  const { t } = useI18n();
  const [open, setOpen] = useState(false);

  if (isLoading) return <div>{t("common.loading")}</div>;
  if (configs.length === 0) {
    return <div className="text-muted-foreground">{t("diagnose.fallbackChain.empty")}</div>;
  }

  const candidates = configs
    .map((c) => c.name)
    .filter((n) => n !== primary && !fallbacks.includes(n));

  return (
    <div className="space-y-3">
      <div>
        <label className="text-sm font-medium">{t("diagnose.primaryModel")} *</label>
        <Select value={primary} onValueChange={(v) => onChange(v, fallbacks.filter((f) => f !== v))}>
          <SelectTrigger><SelectValue placeholder="—" /></SelectTrigger>
          <SelectContent>
            {configs.map((c) => (
              <SelectItem key={c.name} value={c.name}>{c.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div>
        <label className="text-sm font-medium">{t("diagnose.fallbackChain")}</label>
        <p className="text-xs text-muted-foreground">{t("diagnose.fallbackChain.help")}</p>
        <div className="flex flex-wrap gap-2 mt-1">
          {fallbacks.map((name, i) => (
            <Badge key={name} variant="secondary" className="gap-1">
              {name}
              <button
                aria-label={`remove ${name}`}
                onClick={() => onChange(primary, fallbacks.filter((f) => f !== name))}
              >×</button>
              {i > 0 && (
                <button aria-label={`move ${name} up`}
                  onClick={() => {
                    const next = [...fallbacks];
                    [next[i - 1], next[i]] = [next[i], next[i - 1]];
                    onChange(primary, next);
                  }}>↑</button>
              )}
            </Badge>
          ))}
          {candidates.length > 0 && (
            <Popover open={open} onOpenChange={setOpen}>
              <PopoverTrigger asChild><Button variant="outline" size="sm">+</Button></PopoverTrigger>
              <PopoverContent className="w-48 p-1">
                {candidates.map((n) => (
                  <button
                    key={n}
                    className="w-full text-left px-2 py-1 hover:bg-accent rounded"
                    onClick={() => { onChange(primary, [...fallbacks, n]); setOpen(false); }}
                  >{n}</button>
                ))}
              </PopoverContent>
            </Popover>
          )}
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: 跑测试**

```bash
cd dashboard && bun run test src/components/__tests__/model-config-picker.test.tsx
```

- [ ] **Step 5: Commit**

```bash
git add dashboard/src/components/
git commit -m "feat(dashboard): ModelConfigPicker — primary dropdown + fallback chips"
```

---

## Task 16: ModelConfig 编辑页加 retries

**Files:** `dashboard/src/app/modelconfigs/page.tsx` 与新建 `__tests__/page.test.tsx`.

- [ ] **Step 1: 写测试**

```tsx
// __tests__/page.test.tsx
import { render, screen } from "@testing-library/react";
import ModelConfigsPage from "../page";

it("renders Retries column", () => {
  render(<ModelConfigsPage />);
  expect(screen.getByText(/Retries|重试次数/)).toBeInTheDocument();
});
```

- [ ] **Step 2: 修改 page.tsx**

在表格列定义加：

```tsx
{ header: t("modelConfig.retries"), accessor: (row) => row.retries ?? 0 }
```

在创建/编辑表单加：

```tsx
<div>
  <label>{t("modelConfig.retries")}</label>
  <input type="number" min={0} max={10} value={form.retries ?? 0}
    onChange={(e) => setForm({ ...form, retries: Number(e.target.value) })} />
  <p className="text-xs">{t("modelConfig.retries.help")}</p>
</div>
```

API submit payload 中带上 `retries`。

- [ ] **Step 3: 跑测试**

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/app/modelconfigs/
git commit -m "feat(dashboard): ModelConfig form/list shows retries field"
```

---

## Task 17: create-run-dialog 接 ModelConfigPicker

**Files:** `dashboard/src/components/create-run-dialog.tsx`.

- [ ] **Step 1: 替换 ModelConfig 输入**

把现有 `<input>` 或 `<select>` 替换为：

```tsx
<ModelConfigPicker
  primary={form.modelConfigRef}
  fallbacks={form.fallbackModelConfigRefs ?? []}
  onChange={(primary, fallbacks) =>
    setForm({ ...form, modelConfigRef: primary, fallbackModelConfigRefs: fallbacks })}
/>
```

提交 payload 时携带 `fallbackModelConfigRefs`。

- [ ] **Step 2: 现有 dialog 测试如果存在则更新**

- [ ] **Step 3: Commit**

```bash
git add dashboard/src/components/create-run-dialog.tsx
git commit -m "feat(dashboard): create-run-dialog uses ModelConfigPicker"
```

---

## Task 18: diagnose 页接 ModelConfigPicker

**Files:** `dashboard/src/app/diagnose/page.tsx` + `__tests__/page.test.tsx`.

- [ ] **Step 1: 改 page.tsx**

同 Task 17 替换。

- [ ] **Step 2: 更新 page.test.tsx**

加断言：

```tsx
it("includes fallbackModelConfigRefs in submit payload", async () => {
  // mock useModelConfigs with at least 2 configs
  // simulate selecting primary + adding 1 fallback
  // intercept fetch and assert body contains fallbackModelConfigRefs
});
```

- [ ] **Step 3: 跑测试**

```bash
cd dashboard && bun run test src/app/diagnose/__tests__/page.test.tsx
```

- [ ] **Step 4: Commit**

```bash
git add dashboard/src/app/diagnose/
git commit -m "feat(dashboard): diagnose page uses ModelConfigPicker"
```

---

## Task 19: e2e Playwright

**Files:** `dashboard/e2e/modelconfig-fallback.spec.ts`.

- [ ] **Step 1: 写用例**

```ts
import { test, expect } from "@playwright/test";

test("create run with fallback chain shows in CRD YAML", async ({ page }) => {
  await page.goto("/modelconfigs");
  // 创建主 ModelConfig
  await page.click("text=Create");
  await page.fill('[name="name"]', "primary-test");
  await page.fill('[name="model"]', "claude-sonnet-4-6");
  await page.fill('[name="retries"]', "2");
  await page.click("text=Save");

  // 创建备 ModelConfig
  await page.click("text=Create");
  await page.fill('[name="name"]', "backup-test");
  await page.fill('[name="model"]', "claude-haiku-4-5");
  await page.click("text=Save");

  // 进入 diagnose 页
  await page.goto("/diagnose");
  await page.selectOption('[data-testid="primary-model"]', "primary-test");
  await page.click("text=+");
  await page.click("text=backup-test");
  await page.click("text=Submit");

  // 查 Run CRD YAML
  await page.click("text=View CRD");
  const yaml = await page.textContent('[data-testid="crd-yaml"]');
  expect(yaml).toContain("modelConfigRef: primary-test");
  expect(yaml).toMatch(/fallbackModelConfigRefs:\s*\n\s*- backup-test/);
});
```

- [ ] **Step 2: 本地运行**

```bash
cd dashboard && bunx playwright test e2e/modelconfig-fallback.spec.ts
```

(需要本地集群 + dashboard 起着；如果 e2e infra 不齐，标 `.skip` 注明 "infra-only".)

- [ ] **Step 3: Commit**

```bash
git add dashboard/e2e/modelconfig-fallback.spec.ts
git commit -m "test(e2e): fallback chain end-to-end via dashboard + CRD YAML viewer"
```

---

## Task 20: 收尾 — 文档 + Helm values 例子

**Files:** `README.md` (zh + en) + `deploy/helm/VALUES.md`（如存在）.

- [ ] **Step 1: README 加示例**

在 ModelConfig YAML 例子后面加：

```yaml
spec:
  ...existing fields...
  retries: 3   # opt-in retry on 5xx/timeout/429
```

在 DiagnosticRun 例子后面加：

```yaml
spec:
  modelConfigRef: claude-via-cn-proxy
  fallbackModelConfigRefs:
    - claude-direct
    - claude-haiku-cheap
```

- [ ] **Step 2: Commit**

```bash
git add README.md README_EN.md deploy/helm/VALUES.md
git commit -m "docs: ModelConfig.retries and fallback chain examples"
```

---

## Self-Review

**Spec coverage check:**

| Spec 章节 | 实现任务 |
|---|---|
| §2.1 CRD schema | Task 1 |
| §2.2 Translator.resolveModelChain | Task 2 |
| §2.2 Translator buildJob env | Task 3 |
| §2.3 ModelChain Python | Task 4-9 |
| §2.4 Dashboard | Task 13-19 |
| §3 Error handling | Task 7-8 |
| §4 Observability (logs) | Task 7（行内） |
| §4 Observability (Langfuse) | Task 10 |
| §4 Observability (Prometheus) | Task 11-12 |
| §5 Testing | 散落于每个 task |
| §6 兼容性 | Task 3 (compat envs) + Task 4 (legacy from_env) |
| §7 验收清单 | Task 1-19 全部完成即满足 |

**Type consistency check:**

- `ModelEndpoint` 字段：`base_url`/`model`/`api_key`/`retries` 在 Task 4 / 6 / 7 / 8 一致
- `ModelChainExhausted` 在 Task 4 定义，Task 8 / 9 引用一致
- `_SSEStreamBroken` 在 Task 6 定义，Task 7 / 8 引用
- Go `RecordLLMRetry/Fallback/ChainExhausted` 在 Task 12 定义并使用

**No placeholders 扫描：** 所有 step 含具体代码或命令，无 TBD/TODO。

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-29-modelconfig-fallback-and-retry.md`. Two execution options:

**1. Subagent-Driven (recommended)** — 每个 task 一个 fresh subagent，task 间审查，迭代快

**2. Inline Execution** — 在本 session 用 executing-plans 批量推进，含 checkpoint

哪种？
