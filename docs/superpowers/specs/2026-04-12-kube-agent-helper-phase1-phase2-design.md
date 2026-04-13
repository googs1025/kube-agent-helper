# kube-agent-helper Phase 1 + Phase 2 Design Spec

> 状态：草案 v1.0
> 日期：2026-04-12
> 范围：Phase 1 (Operator MVP) + Phase 2 (SkillRegistry + Dashboard)
> 依赖：`k8s-mcp-server` 已完成（9 个只读诊断工具）

---

## 1. 目标

**Phase 1 交付物**：`kubectl apply -f run.yaml` 能触发一次诊断，Agent Pod 运行 3 个内置 Skill，findings 写回 Controller 并入库。

**Phase 2 交付物**：SkillRegistry（内置 + CR 合并）、动态 Orchestrator prompt、Next.js Dashboard（4 页面）。

---

## 2. 仓库结构

单仓库。`k8s-mcp-server` 从 `kube-agent-helper-mcp` worktree 合并进来，作为第一步。

```
kube-agent-helper/
├── cmd/
│   ├── k8s-mcp-server/          ← 迁入（已完成）
│   └── controller/              ← 新增：Operator binary
├── internal/
│   ├── k8sclient/               ← 已完成
│   ├── sanitize/                ← 已完成
│   ├── trimmer/                 ← 已完成
│   ├── audit/                   ← 已完成
│   ├── mcptools/                ← 已完成
│   ├── store/                   ← 新增：DB 访问层（SQLite → PG 可切换）
│   ├── controller/              ← 新增：CRD types + Reconcilers
│   │   ├── api/v1alpha1/        ← Go types + controller-gen markers
│   │   ├── reconciler/          ← DiagnosticRun/Skill/ModelConfig reconcilers
│   │   ├── translator/          ← Compile DiagnosticRun → Job manifests
│   │   └── registry/            ← Phase 2: SkillRegistry
│   └── agent/                   ← 新增：AgentRuntime interface
├── skills/                      ← 内置 SKILL.md × 3
│   ├── pod-health-analyst/
│   ├── pod-security-analyst/
│   └── pod-cost-analyst/
├── agent-runtime/               ← Python Agent 镜像
│   ├── Dockerfile
│   └── runtime/
├── dashboard/                   ← Phase 2：Next.js 14
├── deploy/
│   ├── crds/                    ← controller-gen 生成的 CRD YAML
│   └── helm/                    ← Helm chart（Phase 1 末）
└── docs/
```

---

## 3. Phase 0：垂直切片（最小可跑链路）

Phase 0 的目标是打通完整链路，不追求功能完整性。

```
kubectl apply DiagnosticRun
    │
    ▼
DiagnosticRunReconciler 检测到 Run
    │ Translate()：查 SQLite 获取 Skill 定义
    ▼
创建 Job + ConfigMap(SKILL.md 文件树) + SA + RoleBinding
    │
    ▼
Agent Pod 启动
    │ 读 /workspace/skills/pod-health-analyst/SKILL.md
    │ claude_agent_sdk.query() + MCP tool calls → k8s-mcp-server (in-cluster)
    ▼
POST /internal/runs/{id}/findings → Controller HTTP Server
    │
    ▼
Controller 存 SQLite，更新 DiagnosticRun.status.phase = Succeeded
```

---

## 4. DB 层（`internal/store/`）

### 4.1 Store Interface

```go
type Store interface {
    // Runs
    CreateRun(ctx context.Context, run *DiagnosticRun) error
    GetRun(ctx context.Context, id string) (*DiagnosticRun, error)
    UpdateRunStatus(ctx context.Context, id string, phase Phase, msg string) error
    ListRuns(ctx context.Context, opts ListOpts) ([]*DiagnosticRun, error)

    // Findings
    CreateFinding(ctx context.Context, f *Finding) error
    ListFindings(ctx context.Context, runID string) ([]*Finding, error)

    // Skills
    UpsertSkill(ctx context.Context, s *Skill) error
    ListSkills(ctx context.Context) ([]*Skill, error)
    GetSkill(ctx context.Context, name string) (*Skill, error)
}
```

### 4.2 Phase 1：SQLite 实现

- 驱动：`modernc.org/sqlite`（纯 Go，无 cgo，无外部依赖）
- Migration：`golang-migrate/migrate`，SQL 文件放 `internal/store/migrations/`
- `case_memory` 表（pgvector）Phase 1 不实现

### 4.3 Phase 2：PostgreSQL 实现

- 驱动：`pgx/v5`
- 同一套 `Store` interface，实现放 `internal/store/postgres/`
- 新增 `case_memory` 表 + pgvector extension
- 启动参数：`--db-driver sqlite|postgres`，`--db-dsn <dsn>`

### 4.4 核心表（Phase 1）

```sql
-- 诊断运行
CREATE TABLE diagnostic_runs (
    id           TEXT PRIMARY KEY,
    target_json  TEXT NOT NULL,
    skills_json  TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'Pending',
    message      TEXT,
    started_at   DATETIME,
    completed_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 分析发现
CREATE TABLE findings (
    id                  TEXT PRIMARY KEY,
    run_id              TEXT NOT NULL REFERENCES diagnostic_runs(id),
    dimension           TEXT NOT NULL,
    severity            TEXT NOT NULL,
    title               TEXT NOT NULL,
    description         TEXT,
    resource_kind       TEXT,
    resource_namespace  TEXT,
    resource_name       TEXT,
    suggestion          TEXT,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Skill 元数据（CR 投影 + 内置）
CREATE TABLE skills (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    dimension    TEXT NOT NULL,
    prompt       TEXT NOT NULL,
    tools_json   TEXT NOT NULL,
    requires_data_json TEXT,
    source       TEXT NOT NULL DEFAULT 'builtin',  -- builtin | cr
    enabled      INTEGER NOT NULL DEFAULT 1,
    priority     INTEGER NOT NULL DEFAULT 100,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

---

## 5. CRD 定义（`internal/controller/api/v1alpha1/`）

用 `controller-gen` 从 Go struct 生成 CRD YAML，放 `deploy/crds/`。

### DiagnosticSkill

```go
type DiagnosticSkillSpec struct {
    Dimension    string   `json:"dimension"`           // health|security|cost|reliability
    Description  string   `json:"description"`
    Prompt       string   `json:"prompt"`
    Tools        []string `json:"tools"`               // ["k8s.kubectl_get", ...]
    RequiresData []string `json:"requiresData,omitempty"`
    Enabled      bool     `json:"enabled"`
    Priority     int      `json:"priority,omitempty"`
}
```

### DiagnosticRun

```go
type DiagnosticRunSpec struct {
    Target         TargetSpec `json:"target"`
    Skills         []string   `json:"skills,omitempty"` // 空 = 所有 enabled=true
    ModelConfigRef string     `json:"modelConfigRef"`
}

type TargetSpec struct {
    Scope         string            `json:"scope"`                  // namespace|cluster
    Namespaces    []string          `json:"namespaces,omitempty"`
    LabelSelector map[string]string `json:"labelSelector,omitempty"`
}

type DiagnosticRunStatus struct {
    Phase       Phase        `json:"phase"`        // Pending|Running|Succeeded|Failed
    StartedAt   *metav1.Time `json:"startedAt,omitempty"`
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`
    ReportID    string       `json:"reportId,omitempty"`
    Message     string       `json:"message,omitempty"`
}
```

### ModelConfig

```go
type ModelConfigSpec struct {
    Provider  string       `json:"provider"`           // anthropic
    Model     string       `json:"model"`              // claude-sonnet-4-6
    APIKeyRef SecretKeyRef `json:"apiKeyRef"`
    MaxTurns  int          `json:"maxTurns,omitempty"` // default 20
}

type SecretKeyRef struct {
    Name string `json:"name"`
    Key  string `json:"key"`
}
```

---

## 6. Controller 架构（`internal/controller/`）

### 6.1 三个 Reconciler

| Reconciler | 职责 |
|---|---|
| `DiagnosticRunReconciler` | 核心：Translate → 创建 Job → Watch Job 状态 → 更新 status |
| `DiagnosticSkillReconciler` | 同步 Skill CR → `Store.UpsertSkill()` |
| `ModelConfigReconciler` | 验证 Secret 引用，缓存 model 配置 |

### 6.2 DiagnosticRunReconciler 状态机

```
Pending
  │ Validate：Skills 存在、ModelConfig 有效、namespaces 在白名单
  │ Translate：生成 Job + ConfigMap + SA + RoleBinding
  ▼
Running
  │ 创建 Kubernetes 资源
  │ Watch Job Phase（controller-runtime owns reference）
  ▼
Succeeded / Failed
  │ 更新 status.phase + completedAt
  │ 记录 message（失败原因）
  │ 可选：TTL 后清理 Job
```

### 6.3 Translator 输出物

每次 DiagnosticRun 生成：

- **Job**：Agent Pod，`ttlSecondsAfterFinished: 3600`
- **ConfigMap `skill-bundle-{runID}`**：每个 Skill 一个 key，内容为 SKILL.md
- **ServiceAccount `run-{runID}`**：最小权限，只读 target namespaces
- **RoleBinding**：绑定到 target.namespaces，只授实际用到的 verbs

### 6.4 HTTP Server（同进程）

Controller binary 同时监听 `:8080`：

```
内网（Agent Pod 调用）：
POST /internal/runs/{id}/findings   ← Agent 写回 findings

对外（Dashboard / kubectl 调用）：
GET  /api/runs
GET  /api/runs/{id}
GET  /api/runs/{id}/findings
POST /api/runs                      ← 触发新诊断（Phase 2）
GET  /api/skills                    ← Phase 2
PUT  /api/skills/{name}             ← Phase 2
GET  /api/model-configs             ← Phase 2
```

### 6.5 进程入口

```go
mgr := ctrl.NewManager(restConfig, ctrl.Options{...})
mgr.Add(httpServer)   // Runnable 接口，与 manager 共享生命周期
DiagnosticRunReconciler{Store: store, Runtime: agentRuntime}.SetupWithManager(mgr)
DiagnosticSkillReconciler{Store: store}.SetupWithManager(mgr)
ModelConfigReconciler{}.SetupWithManager(mgr)
mgr.Start(ctx)
```

---

## 7. Agent Runtime（`internal/agent/` + `agent-runtime/`）

### 7.1 Go Interface

```go
// AgentRuntime 由 Translator 调用，生成 Agent Job manifest
type AgentRuntime interface {
    BuildJobSpec(run *DiagnosticRun, skills []*Skill, model *ModelConfig) (*batchv1.Job, error)
}

// PythonRuntime 默认实现
type PythonRuntime struct {
    Image string // ghcr.io/kube-agent-helper/agent-runtime:latest
}
```

### 7.2 Python Agent 目录

```
agent-runtime/
├── Dockerfile          ← python:3.12-slim + claude-agent-sdk + requests
└── runtime/
    ├── main.py         ← 入口：读环境变量 → 加载 skills → query() → 写回
    ├── skill_loader.py ← 扫描 /workspace/skills/*/SKILL.md
    ├── orchestrator.py ← build_orchestrator_prompt(skills, run_context)
    └── reporter.py     ← POST findings → /internal/runs/{id}/findings
```

### 7.3 Job 环境变量

| 变量 | 来源 | 用途 |
|---|---|---|
| `RUN_ID` | DiagnosticRun.metadata.name | findings 写回路由 |
| `TARGET_NAMESPACES` | target.namespaces 逗号拼接 | 注入 prompt |
| `CONTROLLER_URL` | Controller Service DNS | findings POST 地址 |
| `ANTHROPIC_API_KEY` | ModelConfig.APIKeyRef → Secret | LLM 调用 |
| `MCP_SERVER_PATH` | `/usr/local/bin/k8s-mcp-server` | MCP stdio 启动 |

### 7.4 k8s-mcp-server 位置

Agent 镜像**内嵌** k8s-mcp-server binary（多阶段 build 从 Go 镜像 copy），通过 MCP stdio 协议启动，`--in-cluster` 模式，不需要 sidecar：

```python
mcp_servers={"k8s": {"command": "/usr/local/bin/k8s-mcp-server", "args": ["--in-cluster"]}}
```

---

## 8. 内置 Skills × 3（`skills/`）

所有 Skill 遵守统一的 finding 输出 JSON schema：

```json
{
  "dimension": "health|security|cost",
  "severity": "critical|high|medium|low|info",
  "title": "string",
  "description": "string",
  "resource_kind": "string",
  "resource_namespace": "string",
  "resource_name": "string",
  "suggestion": "string"
}
```

### pod-health-analyst
- **dimension**: health
- **tools**: kubectl_get, kubectl_describe, kubectl_logs, events_list
- **requiresData**: pods, events
- **检查重点**: CrashLoopBackOff / OOMKill / ImagePullBackOff、探针持续失败、Pod 长期 Pending（调度失败）

### pod-security-analyst
- **dimension**: security
- **tools**: kubectl_get, kubectl_describe
- **requiresData**: pods, serviceaccounts
- **检查重点**: root 运行（securityContext 缺失）、privileged/hostPID/hostNetwork、SA 不必要 token 挂载、缺少 resource limits

### pod-cost-analyst
- **dimension**: cost
- **tools**: kubectl_get, top_pods, top_nodes, prometheus_query
- **requiresData**: pods, nodes, metrics
- **检查重点**: request >> actual usage（过度分配）、长期 0 副本 Deployment（僵尸资源）、Node 利用率 < 20%、未设置 request（BestEffort）

---

## 9. Phase 2：SkillRegistry + Dashboard

### 9.1 SkillRegistry（`internal/controller/registry/`）

```
内置 Skills（镜像内嵌 skills/*.md）
        +
DiagnosticSkill CR（用户 kubectl apply）
        ↓
SkillRegistry.Merge()    ← CR 同名覆盖内置
        ↓
Translator.Compile()     ← 生成动态 Orchestrator prompt
```

**动态 Orchestrator prompt 模板**：

```
你是 Kubernetes 诊断 Orchestrator。
本次运行目标 namespaces：{target.namespaces}

可用的分析师 Agents（按 priority 排序）：
{for skill in skills}
- {skill.name}：{skill.description}，可用工具：{skill.tools}
{endfor}

请按顺序依次调用各分析师，收集 findings 后统一返回。
每个 finding 必须符合 JSON schema（见 system prompt）。
```

### 9.2 Next.js Dashboard（`dashboard/`）

**技术栈**：Next.js 14 App Router + shadcn/ui + Tailwind CSS

**4 个页面**：

| 路由 | 内容 |
|---|---|
| `/runs` | DiagnosticRun 列表，status badge，"触发新诊断"按钮 |
| `/runs/[id]` | Run 详情：findings 按 severity 分组，resource 链接 |
| `/skills` | 内置 + CR Skill 列表，enabled 开关，prompt 预览 |
| `/settings` | ModelConfig 管理：provider、model、API key 引用 |

**数据全部走 Controller HTTP API**，Dashboard 不直连 DB。

---

## 10. 安全设计

1. **只读**：Agent Pod SA 只授 `get`/`list`/`watch`，Translator 根据 Skill `tools` 字段自动收缩 Role
2. **命名空间白名单**：Controller 启动参数 `--allowed-namespaces`，DiagnosticRun 只能指定白名单内的 namespace
3. **Secret 脱敏**：k8s-mcp-server 层硬约束，LLM 永远看不到 Secret 明文
4. **内网 findings 端点**：`/internal/runs/{id}/findings` 仅 Agent Pod 可访问，不对外暴露
5. **审计日志**：所有 MCP tool call 经 `audit.Wrap()` 落 stderr，带 trace_id

---

## 11. 实施顺序（垂直切片路径 B）

### Phase 0：打通链路（目标：一次完整诊断跑通）

1. **合并 k8s-mcp-server** 到主仓库
2. **DB 层**：Store interface + SQLite 实现 + migrations（3 张表）
3. **CRD types**：Go struct + controller-gen 生成 YAML
4. **最简 Reconciler**：DiagnosticRunReconciler（Pending → Running → Succeeded/Failed）
5. **Translator**：生成 Job + ConfigMap + SA + RoleBinding
6. **Python Agent runtime**：main.py + skill_loader + reporter，单 Skill 跑通
7. **HTTP Server**：`/internal/runs/{id}/findings` 写回端点
8. **pod-health-analyst SKILL.md**：第一个完整 Skill 验证链路

### Phase 1：补全功能

9. **DiagnosticSkillReconciler + ModelConfigReconciler**
10. **pod-security-analyst + pod-cost-analyst SKILL.md**
11. **HTTP API 完善**：`/api/runs`、`/api/runs/{id}`、`/api/runs/{id}/findings`
12. **Helm chart**：`deploy/helm/`，一键部署到集群
13. **单元测试 + envtest 覆盖 Reconciler**

### Phase 2：SkillRegistry + Dashboard

14. **SkillRegistry**：内置 + CR 合并，动态 Orchestrator prompt
15. **`requiresData` 按需拉取**：Translator prefetch hint
16. **Next.js Dashboard**：4 个页面
17. **Dashboard HTTP API 补全**：POST /api/runs、GET/PUT /api/skills
18. **PostgreSQL Store 实现**（替换 SQLite）
19. **端到端测试**

---

## 12. 不做的事（Phase 1+2 范围外）

- pgvector 向量记忆（Phase 3）
- 实时 EventCollector / Prometheus scraper（Phase 3）
- OIDC 接入（Phase 4）
- HITL 人工确认流程（Phase 4）
- 多引擎支持（OpenAI / Gemini）
- Agent 写操作 / mutating Skill
