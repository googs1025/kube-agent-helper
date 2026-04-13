---
title: k8s-mcp-server v1 设计规范
status: Draft
date: 2026-04-11
owner: kube-agent-helper
parent: docs/design.md
scope: Phase 1 子项目 B —— k8s-mcp-server
---

# k8s-mcp-server v1 设计规范

本规范定义 `kube-agent-helper` 项目内 `k8s-mcp-server` 子组件的 v1 目标、边界、接口和交付条件。它是 [`docs/design.md`](../../design.md) §7/§10 描述的"内置 MCP Server"的具体实现规范，也是后续 writing-plans 拆分实施步骤的输入。

## 0. 背景与定位

### 0.1 它是什么

`k8s-mcp-server` 是一个用 Go 实现、通过 stdio 传输 [Model Context Protocol (MCP)](https://modelcontextprotocol.io) 的单进程工具服务器。它把 Kubernetes 集群的只读诊断能力封装成 MCP "tools" 暴露给 LLM Agent 调用。

### 0.2 为什么先做它

参考 `docs/design.md` §11 Phase 1 的清单，整个 Phase 1 依赖链里：

- Controller (子项目 A) 的 Translator 需要按 Skill 的 `tools` 字段生成最小权限 Role，前提是知道每个工具对应哪些 K8s verbs → 依赖 MCP server 的工具清单
- Agent Runtime (子项目 C) 的 agentic loop 必须通过工具读数据 → 依赖 MCP server 存在

因此 B 是 A 和 C 的共同前置依赖，且本身零依赖（只需 `client-go`），适合最先落地。同时它独立可用——能挂 Claude Desktop 做真实诊断 demo。

### 0.3 消费者

v1 同时服务两类消费者，共用同一份 binary：

| 场景 | 启动方式 | 凭据 |
|---|---|---|
| **Agent Pod 子进程** | Claude Agent SDK 的 `mcp_servers` 配置拉起 | In-cluster ServiceAccount |
| **本地 stand-alone** | `k8s-mcp-server --kubeconfig ~/.kube/config` 挂 Claude Desktop 或直连 JSON-RPC | 本地 kubeconfig |

### 0.4 v1 非目标（明确不做）

- 任何写操作（apply / patch / delete / scale）
- 多集群运行时动态切换（单进程绑一个集群）
- HTTP / SSE transport（只 stdio）
- Namespace 白名单（归 Controller 层，见 `docs/design.md` §9）
- 镜像发布 pipeline（归 v1 之后的 M8 可选里程碑）
- LLM 审计日志入 Postgres（归 Phase 3）
- 写 K8s 操作的二次确认 / rollback 机制

---

## 1. 架构与数据流

### 1.1 进程内组件

```
┌─ k8s-mcp-server 进程 ───────────────────────────────┐
│                                                      │
│  ┌─ mcp 层 (mark3labs/mcp-go) ─────────────────┐    │
│  │   stdio JSON-RPC 2.0                         │    │
│  │   tools/list, tools/call                     │    │
│  └────┬─────────────────────────────────────────┘    │
│       │ tool invocation                              │
│       ▼                                              │
│  ┌─ mcptools 层（每个工具一个 handler）──────────┐   │
│  │  kubectl_get.go  events_list.go  ...          │   │
│  │       │                                         │   │
│  │       │ 1. 校验 args                            │   │
│  │       │ 2. 调 k8sclient                         │   │
│  │       │ 3. 过 sanitize                          │   │
│  │       │ 4. trimmer 裁剪                         │   │
│  │       │ 5. 返回 + audit log                     │   │
│  └────┬────────────────────────────────────────┘   │
│       ▼                                              │
│  ┌─ k8sclient 层 ──────────────────────────────┐    │
│  │  rest.Config（启动期固定）                   │    │
│  │  dynamic.Interface（通用 GVR → 对象）        │    │
│  │  kubernetes.Interface（Pod logs 等特例）     │    │
│  │  metricsclient.Interface（top_*）            │    │
│  │  promv1.API（prometheus_query）              │    │
│  └────┬─────────────────────────────────────────┘    │
│       │ HTTPS                                        │
└───────┼──────────────────────────────────────────────┘
        │
        ▼
   K8s API Server / metrics.k8s.io / Prometheus
```

### 1.2 启动流程

1. 解析 CLI flags（见 §5 完整列表）
2. 按优先级构造 `rest.Config`：
   - `--in-cluster` 模式 → `rest.InClusterConfig()`
   - 否则：`--kubeconfig` 显式路径 → `KUBECONFIG` env → `~/.kube/config`
   - 若同时指定 `--in-cluster` 和 `--kubeconfig`，报错退出
3. 向 stderr 输出 resolved cluster info（JSON 格式，见 §4.5）
4. 调用 `SelfSubjectRulesReview`（或退化到 `auth.Can-I List Pods`）做预检；失败 `os.Exit(1)`
5. 初始化四个 client：`dynamic` / `kubernetes` / `metricsclient`（optional）/ `promv1`（optional）
6. 构造 `audit.Middleware`，向 `mcp-go` 注册 8 个工具（每个都用 middleware 包裹）
7. `server.ServeStdio()` 阻塞监听 stdin/stdout

### 1.3 关键约束

- 所有 K8s client 共享同一个 `rest.Config`，启动后不可变
- 所有工具 handler 第一行校验 `ctx`，不泄漏 goroutine
- 任何错误包装成 `mcp.NewToolResultError(msg)` 而不是 panic
- 顶层 `recover` 把未捕获 panic 转为 JSON-RPC error + ERROR 级 audit log
- **绝不** 向 stdout 写任何非 JSON-RPC 协议字节（日志一律 stderr）

---

## 2. 工具接口契约

v1 提供 8 个工具。每个工具的入参和返回 schema 在此规范，下游实现必须一致。

### 2.1 `kubectl_get`

**用途**：通用资源拉取，list 或 get 二合一（根据是否传 `name` 判断模式）。

**入参**：
```json
{
  "kind": "Pod",
  "apiVersion": "v1",
  "namespace": "prod",
  "name": "api-xxx",
  "labelSelector": "app=api,env=prod",
  "fieldSelector": "status.phase!=Running",
  "limit": 100
}
```

- `kind` (required) — 资源 Kind
- `apiVersion` (optional) — 默认按 Kind 自动推断（RESTMapper）
- `namespace` (optional) — 不传=list all namespaces（仅对 namespaced 资源有效）
- `name` (optional) — 传了进入 get 模式；不传进入 list 模式
- `labelSelector` (optional) — 标准 selector 语法
- `fieldSelector` (optional) — 标准 field selector 语法
- `limit` (optional) — list 模式默认 100，最大 500

**返回（list 模式）**：
```json
{
  "kind": "Pod",
  "apiVersion": "v1",
  "totalCount": 237,
  "returnedCount": 100,
  "truncated": true,
  "countAccurate": true,
  "items": [
    {
      "name": "api-7d8-abc",
      "namespace": "prod",
      "phase": "Running",
      "nodeName": "node-3",
      "restarts": 2,
      "age": "3d12h",
      "ready": "2/2",
      "labels": {"app":"api","env":"prod"}
    }
  ]
}
```

- `totalCount` 仅在 API server 返回 `RemainingItemCount != nil` 时可信，否则等于 `returnedCount`，并设 `countAccurate: false`
- list 模式按 Kind 走 `trimmer`：Pod/Deployment/Node/Service/Event 各有专门投影，其他资源用通用投影（name/ns/age/labels）

**返回（get 模式）**：完整 `unstructured.Unstructured.Object`，但经过 `sanitize.Clean()` 处理（去除 `managedFields`、`selfLink`、脱敏 secret 字段等，见 §3）。

**失败场景**：
- 不支持的 Kind → `mcp.NewToolResultError("unsupported kind, try list_api_resources")`
- namespaced 资源未传 namespace 且非 list all → 返回错误
- `limit > 500` → 返回错误（不自动截断）

### 2.2 `kubectl_describe`

**用途**：单资源详细描述，包含相关事件合并。

**入参**：
```json
{
  "kind": "Pod",
  "apiVersion": "v1",
  "namespace": "prod",
  "name": "api-7d8-abc"
}
```

- `kind` (required)
- `apiVersion` (optional)
- `namespace` (required for namespaced resources)
- `name` (required)

**返回**：
```json
{
  "object": { /* 完整对象，过 sanitize，去 managedFields */ },
  "relatedEvents": [
    {
      "type": "Warning",
      "reason": "BackOff",
      "message": "Back-off restarting failed container",
      "firstTimestamp": "2026-04-10T10:00:00Z",
      "lastTimestamp": "2026-04-11T09:00:00Z",
      "count": 47
    }
  ]
}
```

- `relatedEvents` 通过 `core/v1/Events` + `involvedObject.uid == obj.UID` 过滤
- 最多返回 20 条，按 `lastTimestamp` 倒序

### 2.3 `kubectl_logs`

**入参**：
```json
{
  "namespace": "prod",
  "pod": "api-7d8-abc",
  "container": "main",
  "tailLines": 200,
  "previous": false,
  "sinceSeconds": 3600
}
```

- `namespace` (required)
- `pod` (required)
- `container` (optional) — 不传且 Pod 多容器时报错列出候选
- `tailLines` (optional) — 默认 200，最大 2000
- `previous` (optional) — 默认 false；true 对应 `--previous`
- `sinceSeconds` (optional) — 只拉最近 N 秒

**返回**：
```json
{
  "logs": "2026-04-11T09:00:00Z INFO starting...\n...",
  "truncated": true,
  "lineCount": 200
}
```

- 硬上限 256KB；超了做 byte 级截断并设 `truncated: true`
- **不支持** follow / stream（stdio MCP 不适合流式返回）

### 2.4 `events_list`

**入参**：
```json
{
  "namespace": "prod",
  "involvedKind": "Pod",
  "involvedName": "api-7d8-abc",
  "types": ["Warning"],
  "limit": 100
}
```

- 全部 optional；`types` 为 `["Normal","Warning"]` 的子集；`limit` 默认 100，最大 500

**返回**：
```json
{
  "totalCount": 340,
  "returnedCount": 100,
  "truncated": true,
  "events": [
    {
      "namespace": "prod",
      "type": "Warning",
      "reason": "FailedScheduling",
      "message": "0/5 nodes available...",
      "involvedObject": {"kind":"Pod","name":"api-xxx"},
      "firstTimestamp": "...",
      "lastTimestamp": "...",
      "count": 12
    }
  ]
}
```

按 `lastTimestamp` 倒序。

### 2.5 `top_pods`

**入参**：
```json
{
  "namespace": "prod",
  "labelSelector": "app=api",
  "sortBy": "cpu",
  "limit": 50
}
```

- `sortBy`: `cpu` | `memory`，默认 `cpu`
- 其余 optional

**返回**：
```json
{
  "available": true,
  "items": [
    {"name":"api-xxx","namespace":"prod","cpuMilli":1250,"memoryMi":512}
  ]
}
```

**metrics-server 未装时**：
```json
{"available": false, "error": "metrics-server not installed"}
```

注意：`available=false` 时 **不** 抛错，返回正常 `ToolResult`，让 LLM 优雅跳过。

### 2.6 `top_nodes`

同 `top_pods`，但没有 `namespace` 参数。其他语义一致。

### 2.7 `list_api_resources`

**入参**：
```json
{
  "namespaced": true,
  "verb": "list"
}
```

两个字段均 optional。

**返回**：
```json
{
  "resources": [
    {
      "group": "apps",
      "version": "v1",
      "kind": "Deployment",
      "namespaced": true,
      "verbs": ["get","list","watch"]
    }
  ]
}
```

### 2.8 `prometheus_query`

**入参**：
```json
{
  "query": "sum(rate(http_requests_total[5m])) by (job)",
  "time": "2026-04-11T09:00:00Z",
  "range": {
    "start": "2026-04-11T08:00:00Z",
    "end": "2026-04-11T09:00:00Z",
    "step": "30s"
  }
}
```

- `query` (required) — PromQL
- `time` (optional) — 默认 now；传了走 `Query` API
- `range` (optional) — 传了走 `QueryRange` API；`time` 和 `range` 互斥

**返回**：
```json
{
  "available": true,
  "resultType": "vector",
  "samples": [
    {"metric": {"job":"api"}, "value": [1712815200, "42.5"]}
  ]
}
```

**`--prometheus-url` 未配置时**：
```json
{"available": false, "error": "prometheus not configured"}
```

### 2.9 `kubectl_explain`

**入参**：
```json
{
  "kind": "Pod",
  "apiVersion": "v1",
  "field": "spec.containers.resources"
}
```

- `kind` (required)
- `apiVersion` (optional)
- `field` (optional) — 点分字段路径（例：`spec.containers.resources`），不传返回顶层

**返回**：
```json
{
  "description": "ResourceRequirements describes the resources requested...",
  "fields": [
    {"name":"limits","type":"object","description":"...","required":false},
    {"name":"requests","type":"object","description":"..."}
  ]
}
```

**实现**：调 OpenAPI v3 endpoint (`/openapi/v3/apis/<group>/<version>`)，按 `field` 逐级解析 `$ref`。

### 2.10 工具通用约束

- 每个工具的 handler 都必须走 `audit.Middleware` 包裹
- 每个工具的 handler 在返回前都必须过 `sanitize.Clean`（不管返回值是不是 K8s 对象，给统一入口就不会漏）
- 每个工具的错误消息必须脱敏（不能让 K8s API 的错误原样穿透，可能含 secret name）
- 每个工具的入参校验失败一律返回 `ToolResult{IsError: true}`，不 panic

---

## 3. Sanitize 脱敏层

### 3.1 工作模型

- 包位置：`internal/sanitize/`
- 入口：`Clean(obj *unstructured.Unstructured, opts Options) *unstructured.Unstructured`
- **永远返回副本**，不修改入参
- **幂等**：`Clean(Clean(x)) == Clean(x)`
- 按 Kind 分发到专用清理器；未知 Kind 走通用规则

### 3.2 规则表

| Kind | 字段 | 处理 |
|---|---|---|
| **所有资源** | `metadata.managedFields` | 删除 |
| 所有资源 | `metadata.selfLink` | 删除 |
| 所有资源 | `metadata.annotations["kubectl.kubernetes.io/last-applied-configuration"]` | 删除 |
| **Secret** | `data.*` | 替换为 `"<redacted len=N>"`，保留 key 列表和 `type` |
| Secret | `stringData.*` | 同上 |
| **ConfigMap** | `data.<key>`，当 key 匹配 `--mask-configmap-keys` 正则 | 替换为 `"<redacted>"` |
| **Pod** | `spec.containers[].env[].value`，当 `name` 匹配内置敏感正则 | 替换为 `"<redacted>"` |
| Pod | `spec.initContainers[].env[].value` | 同上 |
| Pod | `spec.containers[].env[].valueFrom.secretKeyRef` | 保留引用结构 |
| Pod | `spec.containers[].envFrom[].secretRef` | 保留引用结构 |
| **Node** | `status.images[]` | 保留（诊断用） |

### 3.3 默认正则

**`--mask-configmap-keys`（可配置，默认值）**：
```
(?i)(password|passwd|pwd|secret|token|apikey|api_key|credential|private[_-]?key|cert)
```

**Pod env 变量名脱敏正则（内置常量，不暴露 flag）**：
```
(?i)(password|passwd|secret|token|apikey|api_key|credential|auth)
```

设计理由：
- 匹配 **key 名**而不是 value，避免 false positive
- 大小写不敏感
- Pod env 的正则写死以确保"最后一道防线"不会被误关

### 3.4 脱敏留痕

脱敏后在对象里保留痕迹，让 LLM 明确知道：

```json
{
  "kind": "Secret",
  "metadata": {"name":"db-creds","namespace":"prod"},
  "type": "Opaque",
  "data": {
    "username": "<redacted len=8>",
    "password": "<redacted len=32>"
  }
}
```

### 3.5 不做

- 不做内容扫描（不检测 value 是否"像" JWT / base64 / UUID 等）
- 不做按 Secret 名字的例外白名单
- 不做图像 / 文件脱敏

### 3.6 测试

`internal/sanitize/sanitize_test.go`，表驱动：
- 每条规则至少 1 个正例 + 1 个反例
- 幂等性测试
- 不修改入参测试（用 `reflect.DeepEqual(originalCopy, input)`）
- 未知 CRD 走通用规则不崩

覆盖率目标：**≥ 90%**。

---

## 4. 审计日志层

### 4.1 载体

v1 使用 `log/slog` 输出 JSON 到 **stderr**，不写文件或 DB。

**理由**：
- stdout 是 JSON-RPC 协议通道，不能混入非协议字节
- stderr 在 Agent Pod 场景由 kubelet 收走；本地场景直接可见
- Phase 3 改为写 Postgres 时，只需替换 `slog.Handler`，调用方零改动

### 4.2 字段 Schema

每次 `tools/call` 对应一条日志：

```json
{
  "time": "2026-04-11T09:00:00.123Z",
  "level": "INFO",
  "msg": "tool_call",
  "trace_id": "01HXYZ...",
  "tool": "kubectl_get",
  "args": {
    "kind": "Pod",
    "namespace": "prod",
    "labelSelector": "app=api"
  },
  "result": {
    "ok": true,
    "itemCount": 100,
    "truncated": true,
    "bytes": 24576
  },
  "latency_ms": 87,
  "cluster": "https://k8s.example.com",
  "error": null
}
```

字段说明：
- `trace_id` — 每次 call 生成一个 ULID
- `args` — 按工具的**入参白名单**拷贝已知字段；未知字段丢弃
- `result` — 只记摘要（计数、截断状态、字节数），**不记内容**
- `cluster` — 启动时 resolved 的 API server URL
- `error` — 失败时填错误消息（过 sanitize）

### 4.3 中间件接口

`internal/audit/middleware.go` 暴露：

```go
func Wrap(toolName string, next ToolHandler) ToolHandler
```

其中 `ToolHandler` 是 `mcp-go` 的 handler 签名。`register.go` 在注册每个工具时一律包一层。

### 4.4 异常处理

- 工具失败：`level=ERROR`，`result.ok=false`，`error` 字段填消息
- panic：顶层 `recover` 写一条 `level=ERROR msg=panic`，stack 放进 `error` 字段
- args 校验失败：一样记一条，`error="invalid arguments: ..."`

### 4.5 启动日志样本

```
{"time":"2026-04-11T09:00:01.234Z","level":"INFO","msg":"server started","cluster":"https://127.0.0.1:6443","context":"kind-dev","mode":"stdio"}
{"time":"2026-04-11T09:00:01.245Z","level":"INFO","msg":"precheck passed","verbs":["get","list","watch"]}
{"time":"2026-04-11T09:00:12.102Z","level":"INFO","msg":"tool_call","trace_id":"01HXYZ...","tool":"kubectl_get","args":{"kind":"Pod","namespace":"prod"},"result":{"ok":true,"itemCount":47,"truncated":false,"bytes":18321},"latency_ms":87,"cluster":"https://127.0.0.1:6443"}
{"time":"2026-04-11T09:00:13.501Z","level":"ERROR","msg":"tool_call","trace_id":"01HXZA...","tool":"kubectl_logs","args":{"namespace":"prod","pod":"missing-pod"},"result":{"ok":false},"latency_ms":12,"error":"pod not found","cluster":"https://127.0.0.1:6443"}
```

### 4.6 `--log-level` 开关

- `info`（默认）— 只记 `tool_call` 和 `error`
- `debug` — 额外记：client-go 请求 URL、sanitize 摘要、kubeconfig 解析过程
- **debug 模式下仍然不打印** 完整 request/response body

### 4.7 不做

- 不接 OpenTelemetry（v1 无外部 tracing 系统可接）
- 不做 log rotation（交给 kubelet / systemd / 用户）
- 不做跨 call 的 session 关联（那是 Agent Runtime 层的责任）

---

## 5. 仓库结构与依赖

### 5.1 Monorepo 目录布局

```
kube-agent-helper/
├── go.mod                          # module github.com/<owner>/kube-agent-helper
├── go.sum
├── Makefile
├── .golangci.yml
├── README.md
├── .gitignore
│
├── cmd/
│   └── k8s-mcp-server/
│       └── main.go                 # flag 解析 + wire + ServeStdio
│
├── internal/
│   ├── k8sclient/
│   │   ├── config.go               # 三段式 rest.Config 解析
│   │   ├── clients.go              # dynamic + kubernetes + metrics + prom 构造
│   │   ├── mapper.go               # RESTMapper 封装
│   │   └── precheck.go             # SelfSubjectRulesReview 启动预检
│   │
│   ├── sanitize/
│   │   ├── sanitize.go             # Clean() 入口 + 通用规则
│   │   ├── secret.go
│   │   ├── configmap.go
│   │   ├── pod.go
│   │   └── sanitize_test.go
│   │
│   ├── audit/
│   │   ├── logger.go               # slog JSON handler 封装
│   │   ├── middleware.go
│   │   ├── argmask.go              # args 白名单 sanitize
│   │   └── audit_test.go
│   │
│   ├── trimmer/
│   │   ├── trimmer.go              # 通用投影
│   │   ├── pod.go
│   │   ├── deployment.go
│   │   ├── node.go
│   │   ├── service.go
│   │   ├── event.go
│   │   ├── testdata/               # golden files
│   │   └── trimmer_test.go
│   │
│   └── mcptools/
│       ├── register.go             # 向 mcp server 注册 8 个工具
│       ├── kubectl_get.go
│       ├── kubectl_describe.go
│       ├── kubectl_logs.go
│       ├── events_list.go
│       ├── top.go                  # top_pods + top_nodes
│       ├── list_api_resources.go
│       ├── prometheus_query.go
│       ├── kubectl_explain.go
│       └── *_test.go
│
├── test/
│   └── integration/
│       └── README.md               # kind 集成测试说明
│
└── docs/
    ├── design.md                   # 已有（父设计）
    ├── k8s-mcp-server.md           # 用户指南（M7 产出）
    └── superpowers/
        └── specs/
            └── 2026-04-11-k8s-mcp-server-design.md   # 本文档
```

### 5.2 依赖清单

```go
require (
    github.com/mark3labs/mcp-go v0.x                      // MCP SDK

    k8s.io/api v0.31.x
    k8s.io/apimachinery v0.31.x
    k8s.io/client-go v0.31.x
    k8s.io/metrics v0.31.x                                 // top_*

    github.com/prometheus/client_golang v1.x               // prometheus_query
    github.com/prometheus/common v0.x

    github.com/oklog/ulid/v2 v2.x                          // trace_id

    github.com/stretchr/testify v1.x
)
```

**版本锚定**：
- `k8s.io/*` 统一 0.31 系列，与 `kagent` 对齐，方便子项目 A 后续复用
- 不引入 `controller-runtime`、`cobra`、`viper`、`logrus`、`zap`
- `log/slog` 用 Go 标准库

### 5.3 Go 版本

`go.mod` 声明 `go 1.23`。

### 5.4 CLI Flag 清单

| Flag | 类型 | 默认 | 说明 |
|---|---|---|---|
| `--in-cluster` | bool | false | 使用 in-cluster config |
| `--kubeconfig` | string | "" | kubeconfig 文件路径 |
| `--context` | string | "" | kubeconfig context 名 |
| `--prometheus-url` | string | "" | Prometheus HTTP endpoint，不配则 `prometheus_query` 返回 unavailable |
| `--mask-configmap-keys` | string | 见 §3.3 | ConfigMap key 脱敏正则 |
| `--log-level` | string | `info` | `info` \| `debug` |
| `--help` / `-h` | - | - | 打印 flag 列表 |

互斥约束：`--in-cluster` 与 `--kubeconfig` 不能同时指定。

### 5.5 Makefile 目标

```makefile
BINARY := k8s-mcp-server
PKG := ./cmd/$(BINARY)

.PHONY: build test lint vet fmt image integration clean

build:
	go build -o bin/$(BINARY) $(PKG)

test:
	go test ./... -race -cover

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

integration:
	./test/integration/run.sh

image:
	docker build -f build/k8s-mcp-server.Dockerfile -t k8s-mcp-server:dev .

clean:
	rm -rf bin/
```

### 5.6 Lint 配置

`.golangci.yml` 启用：`errcheck`、`govet`、`staticcheck`、`ineffassign`、`gosimple`、`unused`。

---

## 6. 测试策略

### 6.1 金字塔

```
          ┌──────────────┐
          │  集成测试 ~5  │   kind 真集群 + 真 MCP 协议
          └──────┬───────┘
         ┌───────┴───────┐
         │ 组件测试 ~15   │  envtest (fake API server)
         └───────┬───────┘
    ┌────────────┴────────────┐
    │    单元测试 ~60+         │  纯函数 + fake clientset
    └──────────────────────────┘
```

### 6.2 单元测试

**`internal/sanitize/`**：
- 每条规则正例 + 反例各 1（~15 个 case）
- 幂等性、不修改入参、未知 CRD 走通用规则

**`internal/trimmer/`**：
- 每个专用投影 1-2 个样本 + golden file (`testdata/trim/*.golden.json`)
- 通用投影覆盖未知 Kind
- `managedFields` 一定被删

**`internal/audit/`**：
- `argmask.go` 白名单拷贝行为
- `middleware.go` 用 mock slog handler 验证字段
- 失败路径（panic、args 校验失败）

**`internal/k8sclient/config.go`**：
- 三种 config 源优先级：表驱动
- 非法参数组合报错

**`internal/mcptools/*`**：
- 每个 handler 的 args 校验路径
- 用 `client-go/dynamic/fake` 注入假对象测 happy path 和空结果
- **不**在单元测试里测真实 K8s 行为

覆盖率目标：
- `sanitize`/`trimmer`/`audit` **≥ 90%**
- `mcptools` **≥ 70%**
- `k8sclient` **≥ 60%**
- `cmd/k8s-mcp-server/main.go` 不测

### 6.3 组件测试（envtest）

**工具**：`sigs.k8s.io/controller-runtime/pkg/envtest`

覆盖范围：
1. `k8sclient/precheck.go` 的 RBAC 交互
2. `kubectl_get` 对真实对象的 list/get + RESTMapper
3. `events_list`：写入 Event + 按 involvedObject 过滤
4. `kubectl_describe`：events 合并逻辑
5. `list_api_resources`：discovery client 路径

**不覆盖**：
- `kubectl_logs`（envtest 无 kubelet）
- `top_*`（无 metrics-server）
- `prometheus_query`（用 httptest mock）

envtest 设置放 `test/envtest/setup_test.go`。

### 6.4 集成测试（kind）

**触发方式**：
- 本地：`make integration`
- CI：单独 job，PR label 触发

**5 个核心场景**：
1. 端到端 stdio 协议（真 MCP client 调 `tools/list` + `tools/call`）
2. 真 Pod 日志（busybox 输出）
3. CrashLoop 场景 + `kubectl_describe` 的 events 合并
4. Secret 脱敏链路（apply Secret → `kubectl_get` → 验证 `<redacted>`）
5. 预检失败快速退出（权限为零的 SA → exit 1）

### 6.5 CI 布局

```
lint        golangci-lint
unit        go test ./... -race -cover
envtest     组件测试
integration kind 集成测试（仅 main + label 触发）
```

### 6.6 v1 不做

- 模糊测试
- 性能基准
- 混沌测试
- 端到端 Claude 真调用

---

## 7. 交付里程碑

里程碑是验收清单，不是实施步骤。实施步骤由 writing-plans skill 生成。

### 7.1 M1 — 可构建

- `go.mod` + 依赖就位
- 目录骨架按 §5.1 创建，`go build ./...` 通过
- `Makefile` + `.golangci.yml`，`make build/test/lint/vet` 全绿
- `main.go` 注册 0 个工具也能跑，`tools/list` 返回 `[]`

**验收**：
```bash
make build && ./bin/k8s-mcp-server --help
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ./bin/k8s-mcp-server
```

### 7.2 M2 — Cluster 连接 + 预检

- `internal/k8sclient/{config,clients,mapper,precheck}.go` 实现
- `main.go` 接通 flag 解析 → config → clients → 预检 → `ServeStdio`
- §5.4 的 flag 全部生效
- 启动时 stderr 打印 resolved cluster info

**验收**：
```bash
./bin/k8s-mcp-server --kubeconfig ~/.kube/config
# stderr: "server started" + "precheck passed"
```

故意给无权限 kubeconfig 时：
```
stderr: "precheck failed"
exit 1
```

### 7.3 M3 — Sanitize + Trimmer 纯函数层

- `internal/sanitize/` 全规则 + 表驱动测试
- `internal/trimmer/` 所有专用投影 + golden files
- 覆盖率 `sanitize ≥ 90%`、`trimmer ≥ 90%`

**验收**：`go test ./internal/sanitize/... ./internal/trimmer/... -race -cover` 全绿且达标。

### 7.4 M4 — Audit 中间件

- `internal/audit/{logger,middleware,argmask}.go`
- 单元测试用 mock slog handler 验证字段
- 中间件接口定版：`func Wrap(toolName string, next ToolHandler) ToolHandler`

**验收**：`internal/audit/` 单元测试绿，覆盖率 ≥ 85%。

### 7.5 M5 — 核心 4 工具

- `kubectl_get`、`kubectl_describe`、`kubectl_logs`、`events_list`
- `register.go` 把 4 个注册，全部走 audit 中间件
- 每个工具 ≥ 3 个单元测试
- envtest 覆盖 `kubectl_get`、`kubectl_describe`、`events_list`

**验收**：stand-alone 挂真集群，能用 JSON-RPC 调 `kubectl_get` 拿到 Pod 瘦身列表。

**此点为"最小可用"自然退出点**——万一 M6/M7 时间不够，可以在 M5 后暂停并发布 alpha。

### 7.6 M6 — 扩展 4 工具

- `top_pods`、`top_nodes`：实现 + metrics-server 缺失降级
- `list_api_resources`：discovery client
- `prometheus_query`：实现 + `--prometheus-url` 缺失降级 + httptest mock
- `kubectl_explain`：OpenAPI v3 解析
- 每个新工具 ≥ 2 个单元测试

**验收**：8 个工具全部出现在 `tools/list` 响应里；都能被 `tools/call` 成功调用（优雅降级也算成功）。

### 7.7 M7 — 集成测试 + 文档

- `test/integration/` 下 5 个 kind 场景（§6.4）
- `make integration` 一键 kind 起/跑/销毁
- `docs/k8s-mcp-server.md` 用户指南：flag 说明 + 工具清单 + Claude Desktop 接入示例
- 根 `README.md` 加 Components 章节指向子模块文档

**验收**：`make integration` 全绿 + 文档可被外人 copy-paste 上手。

### 7.8 M8 — 可选（不在 v1 承诺内）

- Dockerfile + 镜像发布
- goreleaser 配置
- CI workflow 文件
- `trimmer` / `sanitize` 的 benchmark

### 7.9 v1 Definition of Done

必须满足：
- [ ] M1-M7 全部验收通过
- [ ] 8 个工具全部可调用
- [ ] `go test ./... -race` 全绿
- [ ] `golangci-lint run` 全绿
- [ ] `sanitize` / `trimmer` / `audit` 覆盖率 ≥ 85%
- [ ] 用 `--kubeconfig` 挂 Claude Desktop 完成一次真实诊断 demo
- [ ] `docs/k8s-mcp-server.md` 完成

---

## 8. 参考

- [`docs/design.md`](../../design.md) — 父设计文档
- [Model Context Protocol](https://modelcontextprotocol.io) — MCP 协议规范
- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) — 选用的 Go SDK
- [`kagent`](https://github.com/kagent-dev/kagent) — 参考项目，子项目 A 将直接借鉴其 Controller 结构
- [`ci-agent`](https://github.com/googs1025/ci-agent) — 参考项目，SKILL.md 声明式扩展启发