# kube-agent-helper 架构设计文档

> 状态：草案 v0.1
> 日期：2026-04-10
> 定位：参考 [kagent](https://github.com/kagent-dev/kagent) 与 [ci-agent](https://github.com/googs1025/ci-agent) 设计的 Kubernetes 原生 AI 助手

---

## 1. 产品定位

**kube-agent-helper** 是一个跑在 Kubernetes 里、专门分析和优化 K8s 资源的 AI Agent，聚焦四个场景：

- 故障诊断（Pod 启动失败、CrashLoop、OOM 根因）
- 配置审计（安全加固、最佳实践）
- 成本优化（资源过度分配、Runner/Node 选型）
- 可靠性分析（flaky 行为、探针、并发守护）

### 与参考项目的对比

| 维度     | ci-agent               | kagent                    | **kube-agent-helper**                     |
| -------- | ---------------------- | ------------------------- | ----------------------------------------- |
| 分析对象 | GitHub repo + workflow | 任意 agent workload       | **K8s cluster + 工作负载**                |
| 运行形态 | 本地单进程 + Web       | 多 Agent Pod + Controller | **Operator + 每次诊断独立 Job/Pod**       |
| 触发方式 | CLI / Web 按需         | CR apply                  | **CR apply + K8s Event Watch + 定时**     |
| 扩展性   | `SKILL.md` 声明式      | CRD + Translator          | **Skill CRD（合并两者）**                 |
| 分析产出 | 四维度 JSON 报告       | Agent 执行结果            | **DiagnosticReport CR + 结构化 findings** |

### 核心取舍总结

| 借 kagent 的                 | 借 ci-agent 的                                   | 自己新增的                       |
| ---------------------------- | ------------------------------------------------ | -------------------------------- |
| CRD + Operator 形态          | SKILL.md 声明式扩展                              | 最小权限 SA 自动生成             |
| Translator 两阶段编译        | 动态 Orchestrator prompt                         | pgvector 案例库做 RAG            |
| 单 binary（Controller+HTTP） | `requiresData` 按需加载                          | MCP Server 层脱敏 Secret         |
| sqlc + migrate + pgvector    | SKILL.md 格式复用（CR spec 与 SDK 加载格式 1:1） | LLM 审计日志 + trace             |
| A2A/MCP 协议                 | Next.js Dashboard                                | Skill CR 而非文件（GitOps 友好） |
| Sandbox + NetworkPolicy      | 实时+按需双通道                                  | 命名空间白名单强约束             |

---

## 2. 整体架构

```
┌──────────────────────────────────────────────────────────────────┐
│                         用户入口                                  │
│  kubectl apply  │  Web UI (Next.js)  │  CLI (kah)                 │
└────────┬─────────────────┬──────────────────┬────────────────────┘
         │ CR              │ REST             │ A2A
         ▼                 ▼                  ▼
┌──────────────────────────────────────────────────────────────────┐
│           Control Plane（Go Controller + HTTP Server）            │
│                                                                    │
│  ┌──────────────┐  ┌───────────────┐  ┌──────────────────────┐  │
│  │ CRD 控制器    │  │ Skill Registry│  │   HTTP API :8080      │  │
│  │ ─────────────│  │ ───────────── │  │  /api/diagnose        │  │
│  │ AgentCtl      │  │ Watch Skill CR │ │  /api/reports/{id}    │  │
│  │ SkillCtl      │  │ Scan FS (内置) │ │  /api/skills          │  │
│  │ ScheduleCtl   │  │ Merge 用户覆盖 │ │  /api/dashboard       │  │
│  │ ReportCtl     │  │ 生成 AgentCfg │ │  /api/usage           │  │
│  └──────┬───────┘  └───────┬───────┘  └──────────┬───────────┘  │
│         │ Translator        │ Compile             │              │
│         ▼                   ▼                      ▼              │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │            Postgres (sqlc + golang-migrate + pgvector)     │  │
│  │  agents │ skills │ reports │ findings │ sessions           │  │
│  │  cluster_events │ memory (embeddings, 历史案例库)          │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────┬───────────────────────────────────────────────────┘
               │ creates
               ▼
┌──────────────────────────────────────────────────────────────────┐
│  Agent Pods (每个 DiagnosticRun 一个 Job，或常驻 Deployment)      │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  ADK Runtime (Python / Go)                                  │  │
│  │  ┌──────────────────────────────────────────────────────┐  │  │
│  │  │ Orchestrator Agent                                    │  │  │
│  │  │   │                                                    │  │  │
│  │  │   ├──▶ health-analyst     (SKILL.md)                 │  │  │
│  │  │   ├──▶ security-analyst   (SKILL.md)                 │  │  │
│  │  │   ├──▶ cost-analyst       (SKILL.md)                 │  │  │
│  │  │   └──▶ reliability-analyst(SKILL.md, 用户自定义)     │  │  │
│  │  │         │                                              │  │  │
│  │  │         │ MCP Tool Calls                              │  │  │
│  │  │         ▼                                              │  │  │
│  │  │   ┌────────────────────────────────────┐              │  │  │
│  │  │   │ K8s MCP Server                     │              │  │  │
│  │  │   │  - kubectl_get / describe / logs   │              │  │  │
│  │  │   │  - prometheus_query                │              │  │  │
│  │  │   │  - events_list                     │              │  │  │
│  │  │   └────────────────────────────────────┘              │  │  │
│  │  └──────────────────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────┬───────────────────────────────────────────────────┘
               │ 写回 findings
               ▼
        Postgres + 前端 Dashboard
               ▲
               │ 实时事件
┌──────────────┴─────────────────┐
│  Data Collector (Daemon/Watch) │   ← 借 ci-agent webhook 思路
│  - k8s events watcher           │
│  - prometheus metrics scraper   │
│  - audit log tail               │
└─────────────────────────────────┘
```

---

## 3. 关键 CRD 设计

### 3.1 DiagnosticSkill CRD（借 ci-agent SKILL.md，升级为 CR）

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticSkill
metadata:
  name: pod-health-analyst
spec:
  dimension: health
  description: "Analyzes pod health, restarts, OOMKills, and probe failures"
  priority: 100
  enabled: true
  prompt: |
    You are a Kubernetes pod health specialist...
    ## Instructions
    1. List all pods via kubectl_get
    2. For non-Running pods, fetch events and logs
    3. Identify OOMKill / CrashLoop / ImagePullBackOff patterns
    ...
  tools:                      # 允许调用的 MCP 工具白名单
    - k8s.kubectl_get
    - k8s.kubectl_describe
    - k8s.kubectl_logs
    - k8s.events_list
  requiresData:               # ← ci-agent 的按需加载
    - pods
    - events
    - node_status
  outputSchema:               # finding 结构化约束
    ref: common/finding-v1
```

### 3.2 DiagnosticRun CRD

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: prod-triage-2026-04-10
spec:
  target:
    scope: namespace          # cluster | namespace | workload
    namespaces: [prod, staging]
    labelSelector: {app: api}
  skills:                     # 不填则跑所有 enabled=true
    - pod-health-analyst
    - security-analyst
  modelConfigRef: claude-sonnet   # 复用 kagent ModelConfig 思路
  schedule: "0 */6 * * *"     # 可选，周期运行
status:
  phase: Running              # Running | Succeeded | Failed
  reportRef: {name: rep-xyz}
```

### 3.3 辅助 CRD

- **ModelConfig**：直接照搬 kagent，存 LLM provider + model + API key secret 引用
- **ClusterModelProviderConfig**：集群级凭据管理
- **DiagnosticReport**：运行结果的只读投影，或只存 DB

---

## 4. Skill 声明式加载

借 ci-agent `SkillRegistry` 架构，数据源从文件系统换成 K8s Skill CR：

```
┌─ 内置 Skill (ConfigMap 或镜像内嵌 skills/*/SKILL.md) ─┐
│                                                        │
├─ Skill CR（用户通过 kubectl 提交）                    │  ← 合并，CR 覆盖内置
│                                                        │
└─ Operator 启动时 Watch + 解析 → SkillRegistry        │
                          │
                          ▼
                compile_orchestrator_prompt()
                collect_required_tools_and_data()
                          │
                          ▼
              注入给即将创建的 Agent Pod
```

**关键决策**：用 Skill CR 而非文件系统，是因为 K8s 环境下 CR 更符合 GitOps，且 Operator 可以 Watch 变更实时生效。

---

## 5. Translator 模式（kagent 精华）

照搬 kagent 的两阶段编译：

```
DiagnosticRun CR
    │
    ▼
[Compile]  ← 解析 ModelConfig / Skill 引用、跨 NS 校验、生成 required_tools
    │
    ▼
CompileInputs { model, skills, targets, tools }
    │
    ▼
[BuildManifest]  ← 生成：Job/Pod + ConfigMap(skills prompt) + ServiceAccount + RoleBinding
    │
    ▼
[]client.Object → reconcileDesiredObjects
```

**改进点**：ServiceAccount + Role 要根据 Skill 的 `tools` 字段**自动收缩权限**（只申请实际需要的 K8s API verbs）—— 这是比 kagent 更保守的地方，因为 LLM 会读集群数据。

---

## 6. 数据通道设计（借 ci-agent 双通道）


| 通道         | 数据源                                      | 用途                 | 写入                |
| ------------ | ------------------------------------------- | -------------------- | ------------------- |
| **实时采集** | `Watch` K8s events / Prometheus / audit log | 趋势、告警、历史基线 | `cluster_events` 表 |
| **按需分析** | Agent Pod 跑时按`requiresData` 拉数据       | 深度诊断 + LLM 分析  | `findings` 表       |

实时通道不走 LLM，只入库；分析通道的 Agent 可以**查这些实时数据表**来做上下文增强（类似 ci-agent 的 prefetch）。

---

## 7. Agent 引擎层

**单引擎选型：Claude Agent SDK + SKILL.md 原生加载**

ci-agent 的 Anthropic 引擎已经验证了这条路，Claude Agent SDK 对 skill 有一等公民支持，对我们的架构来说是胶水代码最少的选择：

- `AgentDefinition` 字段与 SKILL.md frontmatter 几乎 1:1 映射
- `ClaudeAgentOptions(agents={...}, allowed_tools=["Agent"])` 原生提供 orchestrator + sub-agents 模型
- 新版 SDK 支持直接从工作目录自动发现 `SKILL.md` —— Translator 把 Skill CR 投影成文件，SDK 自动加载
- 内置 `Read/Glob/Grep/Bash` 工具 + MCP 工具集成，足以覆盖 K8s 诊断的所有数据读取需求
- Claude 目前在 multi-step tool use / agentic loop 场景下质量最强

**硬伤（诚实记录）**：

- Anthropic 单 vendor，无法切换 OpenAI/Gemini/Ollama
- Agent Pod 需要打包 Claude CLI（Node.js）子进程，镜像体积 +150MB 左右

**不做的事**：不实现 ci-agent 的"双引擎（Agentic vs Pipeline）"。那是 ci-agent 为了兼容 OpenAI 不得不做的分叉。我们单引擎足够，必要时通过 `max_turns` 或预注入数据即可模拟 Pipeline 行为。

**工具接入**：

- **K8s 数据访问**：自建 `k8s-mcp-server`（Go）封装 client-go + prometheus + events watcher
- **文件访问**：Claude Agent SDK 内置 `Read/Glob/Grep`，可直接读挂载到 Pod 的 workflow/manifest 文件

**子 Agent 通信**：走 Claude Agent SDK 原生的 `Agent` tool，不额外引入 A2A 协议。A2A 仅保留给 Agent Pod 对外暴露（给 Controller 和 UI 调用）。

### 7.1 Skill → Claude Agent SDK 映射

```
SKILL.md frontmatter            →    AgentDefinition 参数
────────────────────────────────────────────────────────
name                            →    AGENTS key
description                     →    AgentDefinition.description
prompt (body)                   →    AgentDefinition.prompt
tools: [Read, Grep, mcp__k8s__*] →    AgentDefinition.tools
requires_data                   →    Controller 层 prefetch hint（不进 SDK）
```

### 7.2 Agent Pod 最小运行样板

```python
from claude_agent_sdk import query, ClaudeAgentOptions, AgentDefinition

# 1) Controller 把 Skill CR 投影成 ConfigMap 内的 SKILL.md 文件
#    /workspace/skills/<name>/SKILL.md
skills = load_skills_from("/workspace/skills/")

# 2) 转换为 Claude Agent SDK 的 AgentDefinition
AGENTS = {
    s.name: AgentDefinition(
        description=s.description,
        prompt=s.prompt,
        tools=s.tools,
    )
    for s in skills
}

# 3) 动态拼 orchestrator prompt（借 ci-agent build_orchestrator_prompt 思路）
orchestrator_prompt = build_orchestrator_prompt(skills, run_context)

# 4) 启动 agentic loop
async for msg in query(
    prompt=orchestrator_prompt,
    options=ClaudeAgentOptions(
        agents=AGENTS,
        allowed_tools=["Agent", "mcp__k8s__kubectl_get",
                       "mcp__k8s__events_list", "mcp__k8s__kubectl_logs"],
        mcp_servers={
            "k8s": {"command": "k8s-mcp-server", "args": ["--readonly"]}
        },
        max_turns=20,
    ),
):
    # 把 LLM 的 tool call / artifact / findings 写入 DB
    ...
```

### 7.3 SKILL.md 作为共享格式（一鱼三吃）

同一份 SKILL.md 同时承担三个角色：

1. **CRD spec 的序列化格式** — Skill CR 的 `.spec` 字段结构直接对应 frontmatter
2. **Claude Agent SDK 的加载格式** — Translator 生成的 ConfigMap 里直接就是 `.md` 文件，SDK 自动发现
3. **用户可读的文档** — YAML + Markdown body 对非开发者友好

**设计原则**：Controller 侧的 Skill CR `.spec` 与 Pod 侧的 SKILL.md 文件必须保持字段 1:1 对应，任何时候都能无损互转。

---

## 8. 存储与数据模型

**DB 技术栈**：照搬 kagent —— `pgx + sqlc + golang-migrate + pgvector`。SQLite 不够用（Watch 写入频繁，且 LLM memory 需要向量搜索）。

### 核心表

```sql
-- Skill / Agent 元数据（CR 投影）
skills(id, name, dimension, prompt, tools_json, requires_data_json, source, updated_at)

-- 每次诊断运行
diagnostic_runs(id, target_json, skills_json, status, started_at, completed_at, duration_ms)

-- findings 扁平存储，按 severity 统计
findings(id, run_id, dimension, severity, title, description,
         resource_kind, resource_namespace, resource_name, suggestion, evidence_json)

-- 实时通道：K8s 事件 / 指标快照
cluster_events(id, cluster_id, namespace, kind, name, reason, message, observed_at)

-- 向量记忆：历史案例库，新 finding 检索相似历史
case_memory(id, embedding vector(1536), finding_id, outcome, created_at)
```

### pgvector 的用法（kagent 已埋坑，本项目率先用起来）

- 每个 finding 写入时同步做 embedding 存 `case_memory`
- 下次分析时在 prompt 里注入"历史相似案例 + 当时的 outcome"
- 显著提升 LLM 推理质量和一致性

---

## 9. 关键安全设计（比 kagent/ci-agent 更严）

1. **最小权限 SA**：Translator 根据 Skill 的 `tools` 字段生成 Role，只授与实际需要的 `verbs` 和 `resources`。默认只读，禁止写操作；Skill 单独声明 `mutating: true` 才能写。
2. **命名空间隔离**：`DiagnosticRun.spec.target.namespaces` 必须在 Controller 的 `AllowedNamespaces` 白名单内（照搬 kagent 设计）。
3. **敏感数据脱敏**：在 MCP Server 层面对 Secret/ConfigMap 的 value 做脱敏再给 LLM，这是独立于 Agent 的硬约束。
4. **LLM 输出审计**：所有 LLM 输入/输出落盘到独立的 `llm_audit_log` 表，带 trace_id 关联 Finding，便于事后追溯"为什么 LLM 给出了这条建议"。
5. **Sandbox 模式**（借 kagent PR #1640/#1648）：Agent Pod 默认开启 NetworkPolicy 白名单 —— 只允许访问 K8s API、Prometheus、LLM endpoint，其他一律拒。

---

## 10. 技术栈建议


| 层                | 设计选型                                             | 实际实现（Phase 1）                                |
| ----------------- | ---------------------------------------------------- | -------------------------------------------------- |
| Controller / HTTP | Go + controller-runtime + gorilla/mux                | Go + controller-runtime + net/http（stdlib）       |
| DB                | PostgreSQL + pgx + sqlc + golang-migrate + pgvector  | SQLite（Phase 1 足够；Phase 3 迁移 Postgres）      |
| Agent Runtime     | Python + Claude Agent SDK（含 Claude CLI 子进程）    | Python + Anthropic API + httpx 原生 SSE streaming  |
| 协议              | MCP（工具接入） + A2A（Pod 对外暴露）                | MCP stdio（工具接入）；A2A 留 Phase 2              |
| 内置 MCP Server   | `k8s-mcp-server`（Go）封装 client-go + prometheus    | 已实现，9 个工具                                   |
| UI                | Next.js 14 + shadcn + Tailwind                       | 留 Phase 2（当前通过 kubectl + REST API 查看）     |
| LLM 接入          | Anthropic（Claude Sonnet/Opus）                      | Anthropic（支持自定义 proxy + 模型选择）           |

---

## 11. 实施路线

### Phase 1 — 最小可用（Operator MVP） ✅ 已完成

- [x]  定义 `DiagnosticSkill` / `DiagnosticRun` / `ModelConfig` 三个 CRD
- [x]  Go Controller 单 binary：manager + HTTP server 同进程
- [x]  DB 层 SQLite（Phase 1 简化，Phase 3 迁移 Postgres）
- [x]  Translator：把 `DiagnosticRun` 编译成一次性 Job + ConfigMap + SA + ClusterRoleBinding
- [x]  Python Agent 镜像：
  - 基础镜像含 Python + Go MCP Server 二进制
  - 读挂载的 `/workspace/skills/*.md`
  - 通过 Anthropic API + httpx streaming SSE 跑 agentic loop
  - 写回 findings（通过 Controller HTTP API `POST /internal/runs/{id}/findings`）
- [x]  `k8s-mcp-server`：9 个工具（4 core + 5 extension）
- [x]  3 个内置 Skill：`pod-health-analyst`、`pod-security-analyst`、`pod-cost-analyst`
- [x]  Job completion watch：Running → Succeeded/Failed 自动转换
- [x]  Findings 写回 DiagnosticRun CR status（`findingCounts` + `findings` 摘要）
- [x]  Helm chart 一键部署
- [x]  GitHub Actions CI：unit test + envtest + build + helm lint + kind smoke test

**交付物**：`kubectl apply -f run.yaml` 能跑一次诊断，findings 写回 CR status + SQLite。

### Phase 2 — Skill 系统与多维度

- [ ]  `SkillRegistry`：扫描内置 ConfigMap + Watch Skill CR，合并去重
- [ ]  Orchestrator prompt 动态生成（照搬 ci-agent `build_orchestrator_prompt`）
- [ ]  `requiresData` 按需拉取（借 ci-agent prefetch 思路）
- [ ]  扩展内置 Skill 到 5-6 个（health / security / cost / reliability / config-drift）
- [ ]  Next.js Dashboard 看 Reports

### Phase 3 — 实时通道 + 向量记忆

- [ ]  新增 `EventCollector` runnable：Watch K8s events + 定时抓 Prometheus，写 `cluster_events`
- [ ]  Agent 里把实时数据作为 prefetch 的一部分（类似 ci-agent webhook 表被分析时查询）
- [ ]  新 finding 落库时同步做 embedding → `case_memory`
- [ ]  分析流程增加"检索相似历史 → 注入 prompt"步骤
- [ ]  可选：周期诊断（`DiagnosticRun.schedule`）

### Phase 4 — 生产加固

- [ ]  Translator 根据 Skill 自动生成最小权限 Role
- [ ]  Sandbox 网络白名单（照搬 kagent PR #1648）
- [ ]  LLM 审计日志 + trace 链路
- [ ]  OIDC 接入（照搬 kagent `ProxyAuthenticator`）
- [ ]  HITL：finding 标记为"需人工确认"，Agent Pod 暂停等待回调

---

## 12. 开工前需要回答的问题

1. **业务面窄化还是全覆盖？** "K8s 诊断"太大，建议第一版只做某个垂直场景（比如"Pod 启动失败根因分析"），做深比做宽有价值。
2. **集群接入方式？** In-cluster（Operator 跑在目标集群）还是 Agent-Hub 模式（一个管理集群观测 N 个 target 集群）？前者简单，后者适合多租户 SaaS。
3. **LLM 成本容忍度？** 决定默认引擎选 Agentic Loop 还是 Pipeline。
4. **是否需要写操作？** "只诊断不修复"最安全；"能自动修复"需要二次确认流程 + rollback 机制（工作量 × 3）。

---

## 13. 参考项目

- **kagent** — https://github.com/kagent-dev/kagent
  - 借：CRD/Operator 形态、Translator 两阶段编译、sqlc+migrate+pgvector DB 层、A2A/MCP 协议集成、Sandbox 安全模型
- **ci-agent** — https://github.com/googs1025/ci-agent
  - 借：`SKILL.md` 声明式扩展、动态 Orchestrator prompt、`requiresData` 按需加载、双引擎（Agentic/Pipeline）、双通道数据采集（实时 + 按需）
