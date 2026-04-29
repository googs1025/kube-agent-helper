---
title: ModelConfig 多模型 Fallback 与重试机制设计规范
status: Implemented
date: 2026-04-29
owner: kube-agent-helper
scope: agent-runtime + controller + dashboard 跨层改动
---

# ModelConfig 多模型 Fallback 与重试机制设计规范

为 `kube-agent-helper` 增加两层容灾：

- **重试**（Retry，单模型内）：同一 ModelConfig 遇瞬时错误时退避重试
- **Fallback**（多模型跨切）：主 ModelConfig 失败用尽后，按优先级切到备选 ModelConfig

两层都是 **opt-in**：CRD 字段不显式配置即保持现行为（不重试、不 fallback）。

## 0. 背景

### 0.1 现状痛点

- `agent-runtime/runtime/orchestrator.py` 中的 `_stream_message` 单次 `httpx.stream` 调用，**无任何重试**：5xx / timeout / proxy 抖动 → 整个 turn 失败 → Run 直接 `Failed`
- 近期 commit `884c24a` 修过 message_delta usage 字段缺失，证明 proxy 不可靠是现实问题
- 一个 `DiagnosticRun` 只能引用单个 `ModelConfig`，主模型不可用时无业务连续性

### 0.2 v1 目标

1. ModelConfig 增加 `retries` 字段控制单模型重试次数（默认 0）
2. DiagnosticRun 增加 `fallbackModelConfigRefs` 字段控制 fallback 链（默认空）
3. 重试 + fallback 全在 **agent-runtime（Python）进程内**实现，messages 历史跨切完整保留
4. Dashboard 提供下拉框 + 多选 chips 的可视化配置入口
5. 全链路可观测：日志 / Langfuse trace / Prometheus metrics

### 0.3 v1 非目标

- 不做模型质量评估驱动的 fallback（仅 HTTP/网络层错误触发）
- 不做请求级 deadline / 总超时（保留单连接 `httpx.stream timeout=120` 不变）
- 不做 fallback 间的重试（fallback 切换后那个模型的重试用它自己的 `retries`）
- 不持久化 fallback 决策日志到 SQLite（仅 stdout + Langfuse + Prometheus）

---

## 1. 架构与数据流

```
┌──────────────────────────────────────────────────────────────┐
│  DiagnosticRun (CR)                                          │
│   spec.modelConfigRef: cn-proxy                              │
│   spec.fallbackModelConfigRefs:                              │
│     - direct                                                 │
│     - haiku-cheap                                            │
└────────────────────────┬─────────────────────────────────────┘
                         │ (Reconcile)
                         ▼
        ┌────────────────────────────────────────┐
        │  Translator.Compile (Go)               │
        │   1. Get(ModelConfig "cn-proxy")       │
        │   2. Get(ModelConfig "direct")         │
        │   3. Get(ModelConfig "haiku-cheap")    │
        │   4. buildJob 注入索引化 env：         │
        │      MODEL_COUNT=3                     │
        │      MODEL_0_BASE_URL  …               │
        │      MODEL_0_MODEL    …                │
        │      MODEL_0_RETRIES  …                │
        │      MODEL_0_API_KEY  ← Secret ref     │
        │      MODEL_1_…  MODEL_2_…              │
        └────────────────────┬───────────────────┘
                             ▼
        ┌────────────────────────────────────────┐
        │  Agent Pod (Python)                    │
        │                                        │
        │  ModelChain.from_env()                 │
        │  for turn in 0..MAX_TURNS:             │
        │    response = chain.invoke(            │
        │      tools, messages, tracer)          │
        │    └─ for i in 0..N-1 (endpoints):     │
        │        # 总尝试次数 = 1 + retries_i    │
        │        for attempt in 0..retries_i:    │
        │          try _stream_message(ep_i)     │
        │          ├─ 4xx → raise (终止)         │
        │          ├─ 5xx/超时 → 退避 → retry    │
        │          └─ SSE 中段 → break (跳 ep)   │
        │        # 重试用尽或流断 → 切下一 ep    │
        │      # 全部 ep 用尽 → ModelChainExhausted │
        └────────────────────────────────────────┘
```

### 1.1 不变量

1. **messages 历史跨 endpoint 完整保留** — 切 model 时同一 messages 数组直接喂给下一个 endpoint，不丢上下文
2. **重试在 turn 内** — 一次 `chain.invoke` 内部完成所有重试 + fallback 决策；返回值结构与现有 `_stream_message` 完全一致
3. **opt-in** — `retries` 默认 0、`fallbackModelConfigRefs` 默认空 → 行为与当前实现完全相同
4. **失败语义** — 链路全部用尽 → `chain.invoke` raise `ModelChainExhausted` → `run_agent` 不捕获 → Pod 退出码非零 → Reconciler 标 Run `Failed`

---

## 2. 组件与契约

### 2.1 CRD Schema 变更

**`internal/controller/api/v1alpha1/types.go`**：

```go
type ModelConfigSpec struct {
    Provider  string       `json:"provider"`
    Model     string       `json:"model"`
    BaseURL   string       `json:"baseURL,omitempty"`
    APIKeyRef SecretKeyRef `json:"apiKeyRef"`

    // Retries 控制单模型瞬时错误重试次数。0 = 不重试（默认）。
    // 仅在 5xx / 429 / 网络超时 / connection error 时触发；4xx 永不重试。
    // +optional
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=10
    Retries int `json:"retries,omitempty"`
}

type DiagnosticRunSpec struct {
    // ...existing fields...
    ModelConfigRef string `json:"modelConfigRef,omitempty"`

    // FallbackModelConfigRefs 是按优先级排序的备选 ModelConfig 名称列表。
    // 主 ModelConfig（modelConfigRef）所有重试用尽 / 流中段断开后，
    // 按本列表顺序切换。空列表 = 无 fallback（默认）。
    // 引用的 ModelConfig 必须与主在同一 namespace。
    // +optional
    FallbackModelConfigRefs []string `json:"fallbackModelConfigRefs,omitempty"`
}
```

`zz_generated.deepcopy.go` 同步重新生成；CRD YAML（`deploy/helm/crds/`）同步更新 schema。

### 2.2 Translator（Go）

**位置**：`internal/controller/translator/translator.go`

新增方法：

```go
// resolveModelChain 解析主 + fallback 全部 ModelConfig，按优先级返回。
// 主在 [0]，fallback 按 spec 顺序追加到 [1:]。
// 任一 fallback 不存在 → 跳过并打日志（不让 fallback 不存在阻塞 Run）。
// 主不存在 → 走当前 fallback 行为（resolveAPIKeyEnv 用 ModelConfigRef 当 Secret 名）。
func (t *Translator) resolveModelChain(ctx context.Context, run *k8saiV1.DiagnosticRun) []*k8saiV1.ModelConfig
```

`buildJob` 入参签名改为：

```go
func (t *Translator) buildJob(
    run *k8saiV1.DiagnosticRun,
    runID, saName, cmName string,
    skills []*store.Skill,
    chain []*k8saiV1.ModelConfig,  // 替换原 baseURL/modelName/apiKeyEnv 三参数
) *batchv1.Job
```

环境变量布局（按 chain 索引展开）：

```
MODEL_COUNT             "3"
MODEL_0_BASE_URL        "https://cn-proxy.example.com"
MODEL_0_MODEL           "claude-sonnet-4-6"
MODEL_0_RETRIES         "3"
MODEL_0_API_KEY         <SecretKeyRef cn-proxy-secret/apiKey>
MODEL_1_BASE_URL        ""        # 空 = 走 SDK 默认
MODEL_1_MODEL           "claude-sonnet-4-6"
MODEL_1_RETRIES         "0"
MODEL_1_API_KEY         <SecretKeyRef direct-secret/apiKey>
MODEL_2_BASE_URL        ""
MODEL_2_MODEL           "claude-haiku-4-5"
MODEL_2_RETRIES         "0"
MODEL_2_API_KEY         <SecretKeyRef haiku-secret/apiKey>
```

**兼容性 env**（过渡期保留）：

```
ANTHROPIC_BASE_URL      = MODEL_0_BASE_URL
MODEL                   = MODEL_0_MODEL
ANTHROPIC_API_KEY       = MODEL_0_API_KEY
```

### 2.3 Agent Runtime（Python）

**新增文件**：`agent-runtime/runtime/model_chain.py`

```python
from dataclasses import dataclass

@dataclass(frozen=True)
class ModelEndpoint:
    base_url: str  # "" 表示用 SDK 默认 (https://api.anthropic.com)
    model: str
    api_key: str
    retries: int  # 0 = 不重试

class ModelChainExhausted(Exception):
    """所有 endpoint + 重试都用尽后抛出。"""

class ModelChain:
    def __init__(self, endpoints: list[ModelEndpoint]):
        if not endpoints:
            raise ValueError("ModelChain requires at least one endpoint")
        self.endpoints = endpoints

    def invoke(self, tools, messages, tracer) -> dict:
        """跑一个 turn 的所有重试 + fallback 决策，返回与
        orchestrator._stream_message 同结构的 dict。
        失败时 raise ModelChainExhausted。"""

    @classmethod
    def from_env(cls) -> "ModelChain":
        """从 MODEL_COUNT / MODEL_<i>_* env 构建。
        MODEL_COUNT 缺失时降级读 ANTHROPIC_BASE_URL/MODEL/ANTHROPIC_API_KEY 单端点。"""
```

**`orchestrator.py` 改动**：

- `_stream_message` 函数体下沉到 `model_chain.py` 内的私有函数 `_stream_one(endpoint, tools, messages)`，签名加 `endpoint: ModelEndpoint` 替代读 env
- `run_agent` 启动时构建一次：`chain = ModelChain.from_env()`
- 主循环把 `_stream_message(client, tools, messages)` 替换为 `chain.invoke(tools, messages, tracer)`
- `tracer.generation` 仍按 turn 记录；endpoint 切换由 ModelChain 自己向 tracer 发 event

### 2.4 Dashboard（Next.js）

#### 新组件：`dashboard/src/components/model-config-picker.tsx`

```tsx
interface ModelConfigPickerProps {
  primary: string;
  fallbacks: string[];
  onChange: (primary: string, fallbacks: string[]) => void;
  excludeNames?: string[];
}
```

UI 结构：

- Primary：shadcn/ui `<Select>` 下拉框，options 来自 `useModelConfigs()`
- Fallback：横排 `<Badge>` chips（已选）+ 末尾一个 "+" 按钮弹下拉补选
- 顺序：上下箭头按钮调整（v1 不做拖拽，YAGNI）
- 主选中后从 fallback 候选中过滤

#### ModelConfig 编辑页 (`/modelconfigs/page.tsx`)

- 表单加 `retries` 数字输入（min=0 max=10，默认 0）
- 列表表格增列 "Retries"

#### DiagnosticRun 创建表单（两处）

- `dashboard/src/components/create-run-dialog.tsx`：替换原 ModelConfig 文本/下拉为 `<ModelConfigPicker>`
- `dashboard/src/app/diagnose/page.tsx`：同上

#### API client (`dashboard/src/lib/api.ts`)

- `ModelConfig` type 加 `retries?: number`
- DiagnosticRun create payload 加 `fallbackModelConfigRefs?: string[]`
- 现有 `useModelConfigs()` hook 不变

#### i18n 词条（`dashboard/src/i18n/zh.json` + `en.json`）

| key | 中文 | 英文 |
|---|---|---|
| `modelConfig.retries` | 重试次数 | Retries |
| `modelConfig.retries.help` | 0 表示不重试。仅在网络抖动严重时设置 1-3 | 0 = no retry. Set 1-3 only if proxy is flaky |
| `diagnose.primaryModel` | 主模型 | Primary Model |
| `diagnose.fallbackChain` | 备选链路 | Fallback Chain |
| `diagnose.fallbackChain.help` | 主模型不可用时按顺序切换 | Switched in order when primary fails |
| `diagnose.fallbackChain.empty` | 未配置（默认无 fallback） | Not configured |

---

## 3. 错误处理与重试策略

### 3.1 错误分类

| 异常 | 同模型重试 | 切 fallback |
|---|---|---|
| `httpx.HTTPStatusError` 4xx (400/401/403) | ❌ | ❌（直接 raise，配置错） |
| `httpx.HTTPStatusError` 5xx | ✅ | ✅（重试用尽后） |
| `httpx.HTTPStatusError` 429 | ✅（按 `Retry-After`） | ✅（重试用尽后） |
| `httpx.TimeoutException` | ✅ | ✅ |
| `httpx.ConnectError` / `RemoteProtocolError` | ✅ | ✅ |
| SSE 中段 EOF（`iter_lines` 提前退出且 `stop_reason` 仍是默认值） | ❌（避免 token 重复消费） | ✅（直接切下一 endpoint） |

### 3.2 退避（硬编码）

```python
BACKOFF_SCHEDULE = [1, 2, 4]  # 秒，指数

def backoff_for(attempt: int) -> float:
    idx = attempt - 1
    if idx < len(BACKOFF_SCHEDULE):
        return BACKOFF_SCHEDULE[idx]
    return 4  # 超出后封顶 4s
```

**语义**：`retries` = **额外**重试次数，总尝试 = 1 + `retries`。

- `retries=0` ⇒ 1 次调用，失败立即切 fallback（默认行为）
- `retries=3` ⇒ 共 4 次调用，间隔 1s / 2s / 4s

### 3.3 429 特殊处理

若响应 header 含 `Retry-After`：

- 数值秒数 `Retry-After: 30` → 退避 30s
- 封顶 60s（防恶意服务端阻塞）
- 优先级高于 `BACKOFF_SCHEDULE`

### 3.4 SSE 中段判定

`_stream_one` 内部维护 `stream_complete` 标志：

- 收到 `[DONE]` 或 `message_stop` 事件 → `stream_complete=True`
- `iter_lines` 循环退出后 `stream_complete=False` → raise `SSEStreamBroken`（私有异常）

ModelChain 捕获 `SSEStreamBroken` → 不计入 retry、直接进入下一 endpoint。

---

## 4. 可观测性

### 4.1 结构化日志（stdout JSON）

```python
logger.warn("model retry",
    endpoint_index=0, model="claude-sonnet-4-6",
    attempt=2, error="503 Service Unavailable", backoff_s=2)

logger.warn("model fallback",
    from_index=0, to_index=1,
    from_model="claude-sonnet-4-6", to_model="claude-sonnet-4-6",
    reason="retries_exhausted")  # or "sse_stream_broken" / "primary_4xx"

logger.error("model chain exhausted",
    endpoints_tried=3, last_error="...")
```

### 4.2 Langfuse trace events

`tracer.py` 增方法 `tracer.event(name, level, metadata)`：

- 每次 retry：`event("model_retry", level="WARNING", metadata={attempt, endpoint_index, error})`
- 每次 fallback 切换：`event("model_fallback", level="WARNING", metadata={from, to, reason})`
- 链路耗尽：`tracer.update_trace(status="failed", error_message=...)`

### 4.3 Prometheus metrics

新增到 `internal/metrics/metrics.go`：

```go
LLMRetriesTotal    *prometheus.CounterVec  // labels: model, reason
LLMFallbackTotal   *prometheus.CounterVec  // labels: from_model, to_model, reason
LLMChainExhausted  *prometheus.CounterVec  // labels: namespace
```

Agent Pod 通过现有 `POST /internal/llm-metrics` 端点上报（`reporter.py` 加批量 flush）。`internal/controller/httpserver/llm_metrics_handler.go` 增加 retry/fallback/exhausted 三个事件 type 的处理。

---

## 5. 测试策略

### 5.1 Go 单元测试

`internal/controller/translator/translator_test.go` 新增：

- `Test_resolveModelChain_PrimaryOnly`
- `Test_resolveModelChain_PrimaryWithFallbacks`
- `Test_resolveModelChain_MissingFallbackSkipped`
- `Test_resolveModelChain_AllMissing`
- `Test_buildJob_InjectsModelChainEnvVars`（验证 `MODEL_COUNT` + 索引化字段齐 + API_KEY SecretKeyRef 正确）
- `Test_buildJob_BackwardCompatEnvs`（`ANTHROPIC_BASE_URL` 等仍然存在且 = `MODEL_0_*`）

### 5.2 Python 单元测试

`agent-runtime/tests/test_model_chain.py` 新增：

- `test_from_env_single_endpoint_legacy`（仅 `ANTHROPIC_BASE_URL`/`MODEL`/`ANTHROPIC_API_KEY`）
- `test_from_env_multi_endpoint`（`MODEL_COUNT=3` + 完整索引）
- `test_invoke_succeeds_first_try`
- `test_invoke_retries_on_5xx_and_succeeds`
- `test_invoke_fallbacks_after_retries_exhausted`
- `test_invoke_4xx_no_retry_no_fallback`（直接 raise，messages 不变）
- `test_invoke_sse_broken_skips_to_fallback_no_retry`
- `test_invoke_429_uses_retry_after_header`（mock clock 验证退避秒数）
- `test_invoke_chain_exhausted_raises`
- `test_backoff_for_schedule`（1/2/4/4）

`agent-runtime/tests/test_orchestrator.py` 改动：

- 把直接 mock `_stream_message` 的测试改成 mock `ModelChain.invoke`

### 5.3 Frontend 单元测试

新增 `dashboard/src/components/__tests__/model-config-picker.test.tsx`：

- 主下拉渲染所有 ModelConfig
- 多选 chips 添加 / 移除
- 主选中后从 fallback 候选中排除
- 上下箭头排序：回调 `onChange` 顺序正确
- 空数据态：显示 "No ModelConfigs"

更新 `dashboard/src/app/diagnose/__tests__/page.test.tsx`：

- 已有 `useModelConfigs` mock；增加 fallback 字段提交断言

新增 `dashboard/src/app/modelconfigs/__tests__/page.test.tsx`：

- `retries` 字段渲染 + 表单提交时携带

### 5.4 e2e（Playwright）

`dashboard/e2e/modelconfig-fallback.spec.ts` 新增一条用例：

1. 创建两个 ModelConfig（主 retries=2、备 retries=0）
2. 在症状驱动诊断页选主 + 1 个 fallback
3. 提交后访问 Run 详情页 CRD YAML 标签
4. 断言 YAML 含 `modelConfigRef:` + `fallbackModelConfigRefs:` 列表

---

## 6. 兼容性与迁移

### 6.1 旧 DiagnosticRun

不带 `fallbackModelConfigRefs` 字段 = 行为不变（链长度 1）。

### 6.2 旧 ModelConfig

不带 `retries` 字段 = 默认 0 = 不重试，与现行行为一致。

### 6.3 旧 agent-runtime Pod

理论上 `MODEL_COUNT` 未读 = 走兼容性 env (`ANTHROPIC_BASE_URL` 等)。但 v1 同步发布新 image，运行环境应当用新版 ModelChain.from_env 走 `MODEL_COUNT` 路径。

### 6.4 Helm

`deploy/helm/crds/` 下两个 CRD YAML schema 同步更新；不需要 helm hook 迁移。

---

## 7. 验收标准

- [ ] `ModelConfig.spec.retries` 字段在 CRD schema 中验证 0-10
- [ ] `DiagnosticRun.spec.fallbackModelConfigRefs` 字段定义且空列表为合法值
- [ ] Translator 注入 `MODEL_COUNT` + 索引化 env，且兼容性 env 同时存在
- [ ] `ModelChain` Python 模块独立可测，覆盖 §5.2 全部用例
- [ ] `orchestrator.run_agent` 失败链路抛 `ModelChainExhausted` 并退出码非零
- [ ] Dashboard `<ModelConfigPicker>` 在 ModelConfig 编辑、create-run-dialog、diagnose 页生效
- [ ] i18n 中文 + 英文两套词条齐全
- [ ] retry / fallback / exhausted 三类 Prometheus metric 在 `/metrics` 输出
- [ ] Langfuse trace 含 `model_retry` / `model_fallback` 事件
- [ ] e2e 用例通过

---

## 8. 风险与开放问题

- **token 双倍计费**：fallback 切换时 messages 完整传递，新模型相当于从 turn 1 重新跑。在多轮 agentic 循环里这可能成本可观。v1 接受这个代价；v2 可考虑 turn 内 fallback 时仅重发当前 turn 的最后一条 user message + 上一轮 assistant 摘要（折中）。
- **不同模型的 tool_use schema 兼容性**：v1 假设 fallback 链上所有 endpoint 都是 Anthropic API 兼容（共享同一份 tools 定义）。跨 provider（Anthropic ↔ OpenAI）不在 v1 范围。
- **API key 不同的 ModelConfig**：每个 endpoint 自带独立 `MODEL_<i>_API_KEY` env，K8s Secret 各自独立挂载，不存在共享冲突。
