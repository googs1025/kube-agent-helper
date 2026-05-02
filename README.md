# KubeDoctor

> Kubernetes 原生 AI 诊断 Operator，支持自动修复建议

**KubeDoctor** 是运行在 Kubernetes 集群内的 AI 智能体。声明一个 `DiagnosticRun` CR，控制器即刻拉起隔离的 Agent Pod，通过 MCP 工具调用 Claude，输出结构化诊断结论，并可选生成 `DiagnosticFix` CR（含 patch 或新资源 manifest）。支持定时调度诊断、K8s 事件采集、Prometheus 指标快照。

[![CI](https://github.com/googs1025/kube-agent-helper/actions/workflows/ci.yml/badge.svg)](https://github.com/googs1025/kube-agent-helper/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

[English](README_EN.md)

> **这个项目有两个名字：** **KubeDoctor** 是新名字，`kube-agent-helper` 是老名字 —— 仓库 URL、`kubectl` 命令、镜像路径里还能看到老名字，是同一个项目。

## 功能特性

- **CRD 驱动** — 用 `DiagnosticRun` 声明诊断任务，用 `DiagnosticSkill` CR 扩展技能
- **5 个 CRD** — `DiagnosticRun`、`DiagnosticSkill`、`ModelConfig`、`DiagnosticFix`、`ClusterConfig`
- **10 个内置 Skill** — 健康、安全、成本、可靠性、配置漂移、告警响应、网络、节点、发布、存储
- **16 个 MCP 工具** — 覆盖 kubectl、Prometheus、日志、网络策略、PVC、节点、事件历史、指标快照等
- **Claude 驱动的 Agentic 循环** — 多轮推理，实时访问集群数据
- **定时诊断** — `spec.schedule`（cron 表达式）+ `spec.historyLimit` 实现周期性自动诊断，控制器按计划创建子 Run
- **EventCollector** — 后台自动采集 K8s Warning 事件和 Prometheus 指标快照，存入 SQLite，为 Agent 提供历史上下文
- **多集群支持** — 通过 `ClusterConfig` CRD 注册远端集群，`spec.clusterRef` 指定诊断目标集群
- **Fix 生成** — 在任意 Finding 上点击"生成修复"，短生命周期 Pod 通过 LLM 生成 patch 或资源 manifest
- **Before/After Diff** — Fix 详情页展示变更前后对比
- **人工审批流程 (HITL)** — Fix 经过 `PendingApproval → Approved → Applying → Succeeded`，支持自动回滚
- **症状驱动诊断入口** — `/diagnose` 页面：从监控告警进入，选择症状即可触发精准诊断，支持调度预设（一次性 / 每小时 / 每天 08:00 / 每周一 08:00 / 自定义）
- **Dashboard** — Next.js Web UI，支持中英文切换、深浅色主题、数据统计、快速创建、CRD YAML 查看器、事件页面
- **输出语言可控** — `spec.outputLanguage: zh|en` 控制诊断结论语言
- **ModelConfig 独立配置** — 每个 `ModelConfig` CR 可指定独立的 `spec.baseURL` 代理端点，Translator 按 Run 级别解析
- **重试 + Fallback 链** — `ModelConfig.spec.retries`（单模型重试，opt-in）+ `DiagnosticRun.spec.fallbackModelConfigRefs`（多模型 fallback，messages 历史跨切完整保留）
- **最小化 RBAC** — Translator 为每次运行自动生成最小权限 ServiceAccount
- **SQLite 持久化** — findings、fixes、事件和指标快照本地存储，无需外部数据库

## 架构

![Architecture](docs/architecture.svg)

## 快速开始

### 前置条件

- Kubernetes 集群（minikube、kind 或云上集群均可）
- `helm` >= 3.14
- Anthropic API Key（或兼容代理）

### 1. 创建 API Key Secret

```bash
kubectl create namespace kube-agent-helper
kubectl create secret generic anthropic-credentials \
  -n kube-agent-helper \
  --from-literal=apiKey=sk-ant-...
```

### 2. 创建 ModelConfig CR

```yaml
apiVersion: k8sai.io/v1alpha1
kind: ModelConfig
metadata:
  name: anthropic-credentials
  namespace: kube-agent-helper
spec:
  provider: anthropic
  model: claude-3-5-sonnet-20241022
  baseURL: "https://my-proxy.example.com"   # 可选，省略则直连 Anthropic API
  retries: 3                                # 可选，默认 0；代理抖动严重时设 1-3
  # apiKeyRef 引用 Step 1 创建的 Secret，等价于：
  #   kubectl create secret generic anthropic-credentials \
  #     -n kube-agent-helper --from-literal=apiKey=sk-ant-...
  apiKeyRef:
    name: anthropic-credentials   # ← Secret 名称（与 Step 1 一致）
    key: apiKey                   # ← Secret 中存放 API Key 的 data key
```

`spec.baseURL` 允许每个 ModelConfig 指定独立的 API 代理端点。`spec.retries` 控制单模型瞬时错误（5xx / 429 / 网络超时）的重试次数，0 表示不重试。Translator 在创建 Agent Job 时从 ModelConfig CR 中解析 `baseURL` 和 `apiKeyRef`，而非全局控制器配置。

### 3. Helm 安装

> 完整参数列表和部署场景请参阅 [Helm Values Reference](deploy/helm/VALUES.md)。

```bash
helm install kah deploy/helm \
  --namespace kube-agent-helper
```

使用自定义代理和模型：

```bash
helm install kah deploy/helm \
  --namespace kube-agent-helper \
  --set anthropic.baseURL=https://my-proxy.example.com \
  --set anthropic.model=claude-3-5-sonnet-20241022
```

### 4. 访问 Dashboard

```bash
kubectl port-forward svc/kah -n kube-agent-helper 8080:8080 &
kubectl port-forward svc/kah-dashboard -n kube-agent-helper 3000:3000 &
open http://localhost:3000
```

Dashboard 默认中文界面，支持切换英文，深色/浅色主题可切换。

### 5. 症状驱动诊断（推荐）

打开 Dashboard → 点击 **诊断** → 选择命名空间、资源、勾选症状（如 CPU 高、Pod 频繁重启）→ 选择调度方式（一次性 / 每小时 / 每天 / 每周 / 自定义 cron）→ 提交。

系统自动匹配 Skill 并触发诊断，结果按严重程度排序展示。

### 6. 通过 kubectl 创建诊断任务

一次性诊断：

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: cluster-health-check
  namespace: kube-agent-helper
spec:
  target:
    scope: namespace
    namespaces:
      - default
  modelConfigRef: "anthropic-credentials"
  fallbackModelConfigRefs:                  # 可选，主不可用时按顺序切换；messages 历史跨切完整保留
    - claude-direct
    - claude-haiku-cheap
  timeoutSeconds: 600     # 可选，不填则无超时
  outputLanguage: zh      # 可选：zh | en
```

定时诊断（每小时执行一次，保留最近 5 次）：

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: hourly-health-check
  namespace: kube-agent-helper
spec:
  target:
    scope: namespace
    namespaces:
      - default
  modelConfigRef: "anthropic-credentials"
  schedule: "0 * * * *"    # cron 表达式，每小时执行
  historyLimit: 5           # 保留最近 5 次子 Run，旧的自动清理
  outputLanguage: zh
```

控制器检测到 `spec.schedule` 后，按 cron 计划自动创建子 `DiagnosticRun`，并通过 `historyLimit` 控制历史记录数量。

```bash
kubectl apply -f the-above.yaml
kubectl get diagnosticrun cluster-health-check -w
```

### 7. 生成修复建议

在 Dashboard 上：打开已完成的 Run → 在任意 Finding 卡片点击"生成修复"。

或通过 API：

```bash
curl -X POST http://localhost:8080/api/findings/<finding-id>/generate-fix
```

Fix 生成后可在 Dashboard 查看 Before/After Diff，然后审批或拒绝。

### 8. 查看事件

打开 Dashboard → 点击 **事件** → 按命名空间、资源名称、时间范围筛选 K8s Warning 事件。

EventCollector 在后台持续采集集群 Warning 事件和 Prometheus 指标快照，Agent 在诊断过程中可通过 `events_history` 和 `metric_history` 工具查询这些历史数据。

## 内置 Skill

| Skill | 维度 | 说明 |
|-------|------|------|
| `pod-health-analyst` | health | 检测 CrashLoopBackOff、OOMKilled、Pending 等 |
| `pod-security-analyst` | security | 检查特权容器、缺失 securityContext 等 |
| `pod-cost-analyst` | cost | 发现资源过度申请、僵尸 Deployment |
| `reliability-analyst` | reliability | 分析探针配置、PDB、副本数 |
| `config-drift-analyst` | reliability | 检测 selector/label 不匹配、ConfigMap/Secret 引用断裂 |
| `alert-responder` | health | 对 Prometheus 告警逐一定位根因 |
| `network-troubleshooter` | reliability | 诊断 Service 不通、NetworkPolicy 阻断 |
| `node-health-analyst` | reliability | 检测节点压力（内存/磁盘/PID）、容量不足 |
| `rollout-analyst` | health | 诊断发布卡住、新版本起不来 |
| `storage-analyst` | reliability | 诊断 PVC Pending/Lost、挂载失败 |

自定义 Skill：创建 `DiagnosticSkill` CR 或在 `skills/` 目录放置 `.md` 文件。

## CRD 说明

| CRD | 用途 |
|-----|------|
| `DiagnosticRun` | 声明诊断任务（一次性或定时），控制器创建 Agent Job |
| `DiagnosticSkill` | 声明诊断技能（维度、Prompt、工具列表） |
| `ModelConfig` | LLM 提供商配置（API Key Secret 引用、`baseURL` 代理端点） |
| `DiagnosticFix` | 修复提案（patch 或新资源），含审批流程 |
| `ClusterConfig` | 远端集群注册（kubeconfig Secret 引用），状态显示连接状态 |

### ModelConfig

`ModelConfig` CR 封装了 LLM 提供商的完整配置。Translator 在创建 Agent Pod 时，从 `DiagnosticRun.spec.modelConfigRef` 引用的 ModelConfig 中解析所有参数：

| 字段 | 说明 |
|------|------|
| `spec.provider` | LLM 提供商（如 `anthropic`） |
| `spec.model` | 模型名称 |
| `spec.baseURL` | API 代理端点（可选，省略则使用提供商默认地址） |
| `spec.apiKeyRef.name` | Secret 名称 |
| `spec.apiKeyRef.key` | Secret 中的 key |

### DiagnosticFix 生命周期

```
DryRunComplete → [用户审批] → Approved → Applying → Succeeded
                                                   → Failed → (自动回滚) → RolledBack
               [用户拒绝]  → Failed
```

策略：`dry-run`（仅预览）、`auto`（自动 patch）、`create`（创建新资源）。

## API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/runs` | 列出诊断任务 |
| POST | `/api/runs` | 创建 DiagnosticRun CR |
| GET | `/api/runs/:id` | 获取任务详情 |
| GET | `/api/runs/:id/findings` | 列出 findings |
| GET | `/api/runs/:id/crd` | 获取 DiagnosticRun CR 原始 YAML |
| GET | `/api/skills` | 列出注册的 Skill |
| POST | `/api/skills` | 创建 DiagnosticSkill CR |
| GET | `/api/fixes` | 列出修复提案 |
| GET | `/api/fixes/:id` | 获取修复详情 |
| PATCH | `/api/fixes/:id/approve` | 审批修复 |
| PATCH | `/api/fixes/:id/reject` | 拒绝修复 |
| POST | `/api/findings/:id/generate-fix` | 触发 Fix 生成 |
| GET | `/api/events` | 列出 K8s Warning 事件（查询参数：namespace, name, since） |
| GET | `/api/k8s/resources` | 列出集群资源（namespace/workload 自动补全） |

## 本地开发

```bash
# 运行所有单元测试
make test

# 运行集成测试（需要 kubebuilder 二进制）
make envtest

# 构建二进制
make build

# 构建 Docker 镜像
make image

# 启动 Dashboard 开发服务器
cd dashboard && npm run dev
```

## 项目结构

```
cmd/controller/          Go controller 入口
internal/
  controller/
    api/v1alpha1/        CRD 类型定义
    reconciler/          Run、Skill、Fix、ModelConfig、ScheduledRun Reconciler
    translator/          Run → Job 编译器，FixGenerator → Job 编译器
    httpserver/          REST API 处理器
    registry/            Skill 注册表（热加载）
  store/                 Store 接口 + SQLite 实现
  mcptools/              MCP 工具实现（16 个）
  collector/             EventCollector（K8s Warning 事件 + Prometheus 指标采集）
agent-runtime/
  runtime/
    main.py              诊断 Agent 入口（多轮 LLM）
    fix_main.py          Fix 生成器入口（单次 LLM）
    orchestrator.py      Agentic 循环（httpx SSE 流式）
    mcp_client.py        MCP stdio 客户端
    skill_loader.py      SKILL.md 解析器
dashboard/
  src/
    app/                 Next.js 页面（runs、skills、fixes、diagnose、events）
    components/          UI 组件（对话框、Badge、Diff 查看器、CRD YAML 查看器）
    i18n/                zh.json + en.json 字典
    theme/               深浅色主题 Context
    lib/                 API hooks（SWR）、类型、工具函数
skills/                  内置 Skill .md 文件（10 个）
deploy/helm/             Helm Chart（CRD、RBAC、Deployment）
```

## Roadmap

- [x] **Phase 1** — Operator MVP：4 CRD、单次 Job 运行、5 个内置 Skill
- [x] **Phase 2** — Dashboard（Next.js）、Skill Registry UI、i18n（中/英）、深色模式
- [x] **Phase 3** — DiagnosticFix：LLM 生成 patch、Before/After Diff、HITL 审批、自动回滚
- [x] **Phase 3.5** — 5 个新 MCP 工具、10 个内置 Skill、症状驱动 /diagnose 页面
- [x] **Phase 4** — 定时诊断、EventCollector（事件 + 指标采集）、2 个新 MCP 工具、ModelConfig baseURL、Dashboard 事件页面与 CRD YAML 查看器
- [ ] **Phase 5** — 向量案例库（RAG）、多集群支持、Webhook 通知

## 参考项目

- [kagent](https://github.com/kagent-dev/kagent) — Kubernetes 原生 Agent 编排框架
- [ci-agent](https://github.com/googs1025/ci-agent) — GitHub CI 流水线 AI 分析器

## License

Apache License 2.0 — 详见 [LICENSE](LICENSE)。
