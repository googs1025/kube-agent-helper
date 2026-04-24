# kube-agent-helper CRD 使用指南

> Kubernetes YAML 驱动的 AI 诊断 Operator 完整操作手册

**kube-agent-helper** 通过 **4 个 CRD** 驱动所有功能。你只需编写 YAML，控制器负责拉起 AI 诊断 Pod、调用 Claude、存储结果。

```
用户写 YAML → kubectl apply → Controller → Agent Pod → Claude AI → 诊断结论/修复建议
```

| CRD | 作用 | 你需要写吗？ |
|-----|------|-------------|
| `ModelConfig` | 配置 LLM 模型和 API Key | 一次性配置 |
| `DiagnosticRun` | 触发一次诊断任务 | **每次诊断都要写** |
| `DiagnosticSkill` | 自定义诊断技能 | 可选（内置 10 个） |
| `DiagnosticFix` | 修复提案（系统自动生成） | 一般不需要手写 |
| `ClusterConfig` | 注册远端集群（kubeconfig） | 多集群时配置 |

---

## 第一步：安装

### 创建命名空间和 API Key Secret

```bash
kubectl create namespace kube-agent-helper

kubectl create secret generic anthropic-credentials \
  -n kube-agent-helper \
  --from-literal=apiKey=sk-ant-api03-xxxxxx
```

### Helm 安装

```bash
# 标准安装
helm install kah deploy/helm --namespace kube-agent-helper

# 使用自定义代理和模型
helm install kah deploy/helm \
  --namespace kube-agent-helper \
  --set anthropic.baseURL=https://my-proxy.example.com \
  --set anthropic.model=claude-sonnet-4-6
```

### 验证安装

```bash
kubectl get pods -n kube-agent-helper
# 应看到 controller 和 dashboard Pod 处于 Running 状态

kubectl get crd | grep k8sai.io
# 应看到 4 个 CRD
```

---

## CRD 1 — `ModelConfig`

**作用**：配置 AI 模型提供商、API Key 来源、模型名称、最大推理轮数。`DiagnosticRun` 通过名称引用它。

### 字段说明

```yaml
apiVersion: k8sai.io/v1alpha1
kind: ModelConfig
metadata:
  name: <名称>               # DiagnosticRun.spec.modelConfigRef 填这里
  namespace: kube-agent-helper
spec:
  # 【必填】模型提供商，目前仅支持 anthropic
  provider: anthropic

  # 【必填】模型 ID，默认 claude-sonnet-4-6
  model: claude-sonnet-4-6

  # 【必填】API Key 来源（引用 Kubernetes Secret）
  apiKeyRef:
    name: anthropic-credentials  # Secret 名称
    key: apiKey                  # Secret 中的 key

  # 【可选】Agent 最大推理轮数，默认 20
  # 复杂诊断（全集群）建议调高到 30-50
  maxTurns: 20
```

### 使用示例

**多模型配置（生产 vs 节约成本）**

```yaml
# 生产环境：旗舰模型，推理能力最强
apiVersion: k8sai.io/v1alpha1
kind: ModelConfig
metadata:
  name: prod-model
  namespace: kube-agent-helper
spec:
  provider: anthropic
  model: claude-opus-4-6
  apiKeyRef:
    name: anthropic-credentials
    key: apiKey
  maxTurns: 40
---
# 开发/测试环境：速度快，成本低
apiVersion: k8sai.io/v1alpha1
kind: ModelConfig
metadata:
  name: dev-model
  namespace: kube-agent-helper
spec:
  provider: anthropic
  model: claude-haiku-4-5-20251001
  apiKeyRef:
    name: anthropic-credentials
    key: apiKey
  maxTurns: 10
```

**使用私有代理**

```bash
helm install kah deploy/helm \
  --namespace kube-agent-helper \
  --set anthropic.baseURL=https://my-company-proxy.internal \
  --set anthropic.model=claude-sonnet-4-6
```

### 常用命令

```bash
kubectl get modelconfig -n kube-agent-helper
kubectl describe modelconfig prod-model -n kube-agent-helper
```

---

## CRD 2 — `DiagnosticRun`

**作用**：声明一次诊断任务。控制器收到后立即创建 Agent Job，Agent Pod 通过 MCP 工具调用 Claude 完成分析。

### 字段说明

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: <名称>
  namespace: kube-agent-helper
spec:
  # ─── 【必填】诊断目标 ───────────────────────────────
  target:
    # scope 只有两个值：
    #   namespace — 诊断指定命名空间
    #   cluster   — 全集群扫描（范围更大，耗时更长）
    scope: namespace | cluster

    # scope=namespace 时填写，支持多个
    namespaces:
      - default
      - production

    # 可选：只诊断带有这些 label 的资源
    labelSelector:
      app: my-service
      env: prod

  # ─── 【必填】引用 ModelConfig 名称 ──────────────────
  modelConfigRef: "anthropic-credentials"

  # ─── 【可选】指定运行哪些 Skill ──────────────────────
  # 不填 → 运行所有已注册的 Skill（内置 10 个 + 自定义）
  # 填写 → 只运行列出的 Skill（按名称，不含 .md 后缀）
  skills:
    - pod-health-analyst
    - pod-security-analyst

  # ─── 【可选】超时控制 ────────────────────────────────
  # 单位：秒。不填则无超时。
  # 建议：namespace 级别 120-300s，cluster 级别 600s+
  timeoutSeconds: 300

  # ─── 【可选】输出语言 ────────────────────────────────
  # zh（默认）：诊断结论、建议全部中文
  # en：全英文输出
  outputLanguage: zh
```

### Status 说明（只读，由控制器写入）

```yaml
status:
  phase: Pending | Running | Succeeded | Failed
  startedAt: "2026-04-20T10:00:00Z"
  completedAt: "2026-04-20T10:02:30Z"
  reportId: "01JXXXXXXXXXXXXXXX"    # 用于 REST API 查询
  message: "..."                    # 失败时包含错误原因
  findingCounts:                    # 各维度发现数量汇总
    health: 3
    security: 1
    reliability: 2
  findings:                         # 发现摘要（存在 CR status 中）
    - dimension: health
      severity: critical
      title: "Pod nginx-xxx CrashLoopBackOff"
      resourceKind: Pod
      resourceNamespace: default
      resourceName: nginx-xxx
      suggestion: "检查启动命令和镜像..."
```

### 使用示例

**1. 快速健康检查**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: quick-health
  namespace: kube-agent-helper
spec:
  target:
    scope: namespace
    namespaces: [default]
  skills: [pod-health-analyst]
  modelConfigRef: "anthropic-credentials"
  timeoutSeconds: 120
```

**2. 生产全量审查（所有 Skill）**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: prod-full-audit
  namespace: kube-agent-helper
spec:
  target:
    scope: namespace
    namespaces:
      - production
      - staging
  modelConfigRef: "prod-model"
  timeoutSeconds: 600
  outputLanguage: zh
```

**3. 定向诊断单个服务**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: payment-service-check
  namespace: kube-agent-helper
spec:
  target:
    scope: namespace
    namespaces: [production]
    labelSelector:
      app: payment-service
  skills:
    - pod-health-analyst
    - reliability-analyst
    - network-troubleshooter
  modelConfigRef: "anthropic-credentials"
  timeoutSeconds: 180
```

**4. 全集群安全合规扫描**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: security-compliance-q2-2026
  namespace: kube-agent-helper
spec:
  target:
    scope: cluster
  skills:
    - pod-security-analyst
    - config-drift-analyst
  modelConfigRef: "prod-model"
  outputLanguage: en
  timeoutSeconds: 900
```

**5. 告警响应（配合 Prometheus）**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: alert-response-20260420
  namespace: kube-agent-helper
spec:
  target:
    scope: namespace
    namespaces: [monitoring, production]
  skills:
    - alert-responder
    - node-health-analyst
  modelConfigRef: "anthropic-credentials"
  timeoutSeconds: 300
```

### 生命周期与操作

```
kubectl apply  →  Pending  →  Running  →  Succeeded
                                       →  Failed
```

```bash
# 创建并观察进度
kubectl apply -f run.yaml
kubectl get diagnosticrun -n kube-agent-helper -w

# 查看 Agent Pod 实时日志（观察多轮推理过程）
kubectl logs -n kube-agent-helper \
  -l job-name=<run-name> --follow

# 查看 findings 摘要（存储在 CR status）
kubectl get diagnosticrun <name> -n kube-agent-helper \
  -o jsonpath='{.status.findings}' | jq .

# 通过 REST API 获取完整 findings（包含 description 和 suggestion）
curl http://localhost:8080/api/runs/<reportId>/findings | jq .

# 重新运行诊断（删除旧的，创建新的）
kubectl delete diagnosticrun <name> -n kube-agent-helper
kubectl apply -f run.yaml
```

---

## CRD 3 — `DiagnosticSkill`

**作用**：定义一个诊断技能，告诉 Agent 用哪些工具、按什么逻辑去分析，输出什么格式的 finding。可以理解为 Agent 的"分析剧本"。

> **提示**：内置 10 个 Skill（`pod-health-analyst` 等）从 `skills/*.md` 文件热加载，不需要创建 CR。通过 CR 创建的同名 Skill 优先级更高。

### 字段说明

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticSkill
metadata:
  name: <技能名>            # DiagnosticRun.spec.skills[] 填这个名称
  namespace: kube-agent-helper
spec:
  # 【必填】描述这个 Skill 做什么（用于 Dashboard 展示）
  description: "检测 JVM 应用内存溢出问题"

  # 【必填】诊断维度，只能是以下四种之一：
  #   health      — 可用性、崩溃、重启
  #   security    — 安全配置、权限、漏洞
  #   cost        — 资源浪费、过度申请
  #   reliability — 探针、PDB、副本数、配置一致性
  dimension: health

  # 【必填】是否启用（false 则该 Skill 不参与任何 Run）
  enabled: true

  # 【可选】执行优先级，数字越小越先执行，默认 100
  priority: 50

  # 【必填】使用的 MCP 工具列表（至少 1 个）
  tools:
    - kubectl_get
    - kubectl_describe
    - kubectl_logs
    - prometheus_query

  # 【可选】提示 Agent 需要哪些数据类型（优化预加载顺序）
  requiresData:
    - pods
    - metrics

  # 【必填】给 Agent 的分析指令（Prompt）
  # 决定 Agent 的分析逻辑、输出格式，是 Skill 的核心
  prompt: |
    你是 [领域专家角色]。[一句话说明任务目标]。
    ...
```

### 可用 MCP 工具列表

| 工具名 | 作用 |
|--------|------|
| `kubectl_get` | 列出资源（pods、deployments 等） |
| `kubectl_describe` | 描述资源详情 |
| `kubectl_logs` | 获取 Pod 日志 |
| `kubectl_apply` | 应用 YAML |
| `events_list` | 列出事件 |
| `kubectl_rollout_status` | 检查 Rollout 状态 |
| `prometheus_query` | 执行 PromQL 查询 |
| `prometheus_alerts` | 获取告警列表 |
| `network_policy_list` | 列出网络策略 |
| `pvc_list` | 列出 PVC |
| `node_list` | 列出节点信息 |
| `pod_exec` | 在 Pod 中执行命令 |
| `resource_usage` | 获取资源使用率 |
| `top_pods` | 查看 Pod 资源消耗 |

### Prompt 编写规范

```markdown
你是 [领域专家角色]。[一句话说明任务目标]。

## 分析步骤
1. 用 [工具名] 获取 [数据]...
2. 对每个 [资源] 检查 [条件]...
3. 如果发现 [问题]，使用 [工具] 进一步确认...

## Finding 输出格式（每行一个 JSON）
{"dimension":"[维度]","severity":"[级别]",
 "title":"[简短标题]","description":"[详情]",
 "resource_kind":"[资源类型]","resource_namespace":"[命名空间]",
 "resource_name":"[资源名]","suggestion":"[修复建议]"}

## 严重程度指南
- critical: [最严重场景]
- high:     [较严重场景]
- medium:   [一般问题]
- low:      [轻微/建议性问题]
```

### 自定义 Skill 示例

**示例 A：检测 Pod 无法调度（资源不足）**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticSkill
metadata:
  name: scheduling-analyst
  namespace: kube-agent-helper
spec:
  description: "检测 Pod 因资源不足无法调度的问题"
  dimension: reliability
  enabled: true
  priority: 30
  tools:
    - kubectl_get
    - kubectl_describe
    - events_list
    - node_list
  requiresData:
    - pods
    - nodes
  prompt: |
    你是 Kubernetes 调度问题专家。

    ## 步骤
    1. 用 kubectl_get（kind=Pod）找出所有 Pending 状态的 Pod
    2. 对每个 Pending Pod：
       - 用 kubectl_describe 查看 Events 中的调度失败原因
       - 用 events_list 获取相关事件
    3. 用 node_list 查看各节点可用资源
    4. 判断失败原因：Insufficient cpu/memory、Taints 不匹配、NodeSelector 无满足节点
    5. 输出 finding JSON（dimension: reliability）

    ## 严重程度
    - high: 核心服务 Pod 超过 5 分钟无法调度
    - medium: 非关键 Pod Pending 或偶发调度失败
```

**示例 B：PVC 挂载异常检测**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticSkill
metadata:
  name: pvc-mount-analyst
  namespace: kube-agent-helper
spec:
  description: "检测 PVC 挂载失败和存储异常"
  dimension: reliability
  enabled: true
  tools:
    - kubectl_get
    - kubectl_describe
    - pvc_list
    - events_list
  prompt: |
    你是 Kubernetes 存储专家。

    ## 步骤
    1. 用 pvc_list 列出所有 PVC，重点关注状态非 Bound 的
    2. 对异常 PVC 用 kubectl_describe 查看详情
    3. 用 events_list 获取相关存储事件
    4. 检查：PVC Pending、PVC Lost、Pod 挂载失败（VolumeMountError）
    5. 输出 finding JSON（dimension: reliability）

    ## 严重程度
    - critical: 有状态服务的 PVC Lost 或无法挂载
    - high: PVC Pending 超过 10 分钟
    - medium: 存储容量使用率超过 80%
```

**示例 C：Ingress 和 Service 连通性检查**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticSkill
metadata:
  name: ingress-connectivity-analyst
  namespace: kube-agent-helper
spec:
  description: "检测 Ingress 和 Service 端点连通性"
  dimension: reliability
  enabled: true
  tools:
    - kubectl_get
    - kubectl_describe
    - network_policy_list
    - events_list
  prompt: |
    你是 Kubernetes 网络专家。

    ## 步骤
    1. 用 kubectl_get（kind=Ingress）列出所有 Ingress
    2. 对每个 Ingress：
       - 用 kubectl_describe 查看后端 Service 配置
       - 用 kubectl_get（kind=Endpoints）检查 Service 是否有就绪端点
       - 如端点为空，查找 Pod 和 selector 是否匹配
    3. 用 network_policy_list 检查是否有 NetworkPolicy 阻断流量
    4. 输出 finding JSON（dimension: reliability）

    ## 严重程度
    - critical: Service 端点为空（流量无法到达任何 Pod）
    - high: Ingress 指向不存在的 Service
    - medium: NetworkPolicy 可能阻断部分流量
    - low: Ingress 缺少 TLS 配置
```

### Skill 管理命令

```bash
# 查看所有已注册 Skill（包括内置和自定义）
kubectl get diagnosticskill -n kube-agent-helper

# 查看 Skill 详情
kubectl describe diagnosticskill jvm-memory-analyst -n kube-agent-helper

# 临时禁用某个 Skill（不删除）
kubectl patch diagnosticskill pod-cost-analyst -n kube-agent-helper \
  --type=merge -p '{"spec":{"enabled":false}}'

# 通过 API 查看已加载的 Skill
curl http://localhost:8080/api/skills | jq '.[].name'
```

---

## CRD 4 — `DiagnosticFix`

**作用**：代表一个 AI 生成的修复提案。通常由系统自动创建（通过 Dashboard 或 API 触发），也可以手动写 YAML 创建。

### 字段说明

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticFix
metadata:
  name: <名称>
  namespace: kube-agent-helper
spec:
  # 【必填】关联的 DiagnosticRun 名称
  diagnosticRunRef: "prod-full-audit"

  # 【必填】对应的 Finding 标题（说明修复什么问题）
  findingTitle: "Pod nginx-xxx missing resource limits"

  # 【可选】Finding ID（精确关联）
  findingID: "01JXX..."

  # 【必填】修复目标资源
  target:
    kind: Deployment        # 资源类型
    namespace: production   # 命名空间
    name: nginx             # 资源名称

  # 【必填】修复策略
  # dry-run  — 只预览，不实际修改，进入 DryRunComplete 状态供审查
  # auto     — 等待人工审批后自动 patch
  # create   — 创建全新资源（patch.content 为完整 YAML）
  strategy: auto

  # 【可选】是否需要人工审批，默认 true
  # false 则审批通过后立即自动执行（生产环境慎用）
  approvalRequired: true

  # 【必填】Patch 内容
  patch:
    # strategic-merge — 标准 K8s 合并 patch（推荐）
    # json-patch      — RFC 6902 JSON Patch
    type: strategic-merge
    content: |
      spec:
        template:
          spec:
            containers:
            - name: nginx
              resources:
                requests:
                  cpu: 100m
                  memory: 128Mi
                limits:
                  cpu: 500m
                  memory: 256Mi

  # 【可选】回滚配置
  rollback:
    enabled: true                # 允许回滚，默认 true
    snapshotBefore: true         # 应用前快照原始状态，默认 true
    autoRollbackOnFailure: true  # 失败时自动回滚，默认 true
    healthCheckTimeout: 300      # 健康检查超时（秒），默认 300
```

### Status 说明（只读）

```yaml
status:
  phase: PendingApproval | Approved | Applying | Succeeded | Failed | RolledBack | DryRunComplete
  approvedBy: "admin"
  approvedAt: "2026-04-20T10:05:00Z"
  appliedAt: "2026-04-20T10:05:01Z"
  completedAt: "2026-04-20T10:05:05Z"
  rollbackSnapshot: "<base64 原始资源>"  # 回滚快照
  message: "Patch applied successfully"
```

### 完整生命周期

```
创建 CR（或 AI 自动生成）
         │
         ├─ strategy=dry-run ──────────────────────► DryRunComplete（只预览，永不执行）
         │
         ├─ approvalRequired=true ────────────────► PendingApproval
         │                                               │ 人工审批
         │                                               ▼
         └─ approvalRequired=false ──────────────► Approved
                                                        │ 控制器自动执行
                                                        ▼
                                                   Applying
                                                   │       │
                                             成功  │       │ 失败
                                                   ▼       ▼
                                               Succeeded  Failed
                                                           │ autoRollbackOnFailure=true
                                                           ▼
                                                       RolledBack
```

### 使用示例

**示例 1：dry-run 预览补丁（不执行）**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticFix
metadata:
  name: preview-limits-fix
  namespace: kube-agent-helper
spec:
  diagnosticRunRef: "prod-full-audit"
  findingTitle: "Containers without resource limits"
  target:
    kind: Deployment
    namespace: production
    name: api-server
  strategy: dry-run
  approvalRequired: true
  patch:
    type: strategic-merge
    content: |
      spec:
        template:
          spec:
            containers:
            - name: api
              resources:
                limits:
                  cpu: "1"
                  memory: 512Mi
```

**示例 2：标准修复（人工审批后执行）**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticFix
metadata:
  name: fix-replica-count
  namespace: kube-agent-helper
spec:
  diagnosticRunRef: "reliability-check"
  findingTitle: "Single replica Deployment with no PDB"
  target:
    kind: Deployment
    namespace: production
    name: payment-service
  strategy: auto
  approvalRequired: true
  patch:
    type: strategic-merge
    content: |
      spec:
        replicas: 3
  rollback:
    enabled: true
    snapshotBefore: true
    autoRollbackOnFailure: true
    healthCheckTimeout: 120
```

**示例 3：create 策略 — 创建新资源**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticFix
metadata:
  name: create-pdb-fix
  namespace: kube-agent-helper
spec:
  diagnosticRunRef: "reliability-check"
  findingTitle: "No PodDisruptionBudget for critical Deployment"
  target:
    kind: PodDisruptionBudget
    namespace: production
    name: payment-service-pdb
  strategy: create
  approvalRequired: true
  patch:
    type: strategic-merge
    content: |
      apiVersion: policy/v1
      kind: PodDisruptionBudget
      metadata:
        name: payment-service-pdb
        namespace: production
      spec:
        minAvailable: 2
        selector:
          matchLabels:
            app: payment-service
```

**示例 4：json-patch 精确修改**

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticFix
metadata:
  name: fix-security-context
  namespace: kube-agent-helper
spec:
  diagnosticRunRef: "security-audit-q2"
  findingTitle: "Container running as root"
  target:
    kind: Deployment
    namespace: production
    name: web-frontend
  strategy: auto
  approvalRequired: true
  patch:
    type: json-patch
    content: |
      [
        {"op": "add", "path": "/spec/template/spec/securityContext",
         "value": {"runAsNonRoot": true, "runAsUser": 1000}},
        {"op": "add", "path": "/spec/template/spec/containers/0/securityContext",
         "value": {"readOnlyRootFilesystem": true, "allowPrivilegeEscalation": false}}
      ]
```

### Fix 管理命令

```bash
# 列出所有 Fix（shortname: dfix）
kubectl get dfix -n kube-agent-helper

# 查看 Fix 详情（含回滚快照）
kubectl describe dfix <name> -n kube-agent-helper

# 通过 API 查询所有待审批的 Fix
curl http://localhost:8080/api/fixes \
  | jq '.[] | select(.status.phase=="PendingApproval")'

# 审批
curl -X PATCH http://localhost:8080/api/fixes/<fix-id>/approve

# 拒绝
curl -X PATCH http://localhost:8080/api/fixes/<fix-id>/reject

# 触发 AI 生成 Fix（从已有 Finding 出发）
curl -X POST http://localhost:8080/api/findings/<finding-id>/generate-fix

# 观察执行过程
kubectl get dfix <name> -n kube-agent-helper -w
# PendingApproval → Approved → Applying → Succeeded
```

---

## 多集群诊断

### 概述

默认情况下，kube-agent-helper 诊断的是控制器所在的本地集群。通过 `ClusterConfig` CRD 注册远端集群，然后在 `DiagnosticRun` 中通过 `spec.clusterRef` 指定目标集群。

### 第一步：准备远端集群的 kubeconfig

**方式 A：直接使用现有 kubeconfig 文件**

```bash
kubectl create secret generic prod-kubeconfig \
  -n kube-agent-helper \
  --from-file=kubeconfig=$HOME/.kube/prod-config
```

**方式 B：使用 ServiceAccount Token（推荐生产环境）**

在远端集群执行：

```bash
kubectl create sa kah-reader -n kube-system
kubectl create clusterrolebinding kah-reader \
  --clusterrole=view --serviceaccount=kube-system:kah-reader

TOKEN=$(kubectl create token kah-reader -n kube-system --duration=8760h)
CA=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.certificate-authority-data}')
SERVER=$(kubectl config view --raw -o jsonpath='{.clusters[0].cluster.server}')

cat > /tmp/prod-kubeconfig.yaml <<EOF
apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: ${CA}
    server: ${SERVER}
  name: prod
contexts:
- context:
    cluster: prod
    user: kah-reader
  name: prod
current-context: prod
users:
- name: kah-reader
  user:
    token: ${TOKEN}
EOF
```

回到本地集群：

```bash
kubectl create secret generic prod-kubeconfig \
  -n kube-agent-helper \
  --from-file=kubeconfig=/tmp/prod-kubeconfig.yaml
```

### 第二步：创建 ClusterConfig CR

```yaml
apiVersion: k8sai.io/v1alpha1
kind: ClusterConfig
metadata:
  name: prod
  namespace: kube-agent-helper
spec:
  kubeConfigRef:
    name: prod-kubeconfig
    key: kubeconfig
  prometheusURL: "http://prometheus.monitoring:9090"
  description: "生产集群"
```

```bash
kubectl apply -f the-above.yaml
kubectl get clusterconfig prod -n kube-agent-helper
# NAME   PHASE       AGE
# prod   Connected   10s
```

### 第三步：在 DiagnosticRun 中指定目标集群

```yaml
apiVersion: k8sai.io/v1alpha1
kind: DiagnosticRun
metadata:
  name: prod-health-check
  namespace: kube-agent-helper
spec:
  clusterRef: "prod"
  target:
    scope: namespace
    namespaces:
      - default
  modelConfigRef: "anthropic-credentials"
  outputLanguage: zh
```

省略 `clusterRef` 或留空 = 在本地集群运行（向后兼容）。

### ClusterConfig 字段说明

| 字段 | 说明 |
|------|------|
| `spec.kubeConfigRef.name` | 包含 kubeconfig 的 Secret 名称 |
| `spec.kubeConfigRef.key` | Secret 中 kubeconfig 数据的 key |
| `spec.prometheusURL` | 远端集群的 Prometheus 端点（可选） |
| `spec.description` | 集群描述（显示在 Dashboard） |
| `status.phase` | 连接状态：`Connected` 或 `Error` |
| `status.message` | 错误信息（仅在 `Error` 时有值） |

---

## 四个 CRD 关系图

```
Secret (anthropic-credentials)
    │ apiKeyRef
    ▼
ModelConfig (prod-model)
    │ modelConfigRef
    ▼
DiagnosticRun (prod-full-audit) ──── skills ────► DiagnosticSkill (pod-health-analyst)
    │ reportId / status.findings                   DiagnosticSkill (jvm-memory-analyst)
    │
    ▼ [Finding 触发 Fix 生成]
DiagnosticFix (fix-replica-count)
    │ diagnosticRunRef
    └── target → Deployment/production/payment-service
        strategy: auto → PendingApproval → Approved → Applying → Succeeded
```

---

## 常见场景速查

| 场景 | 推荐 Skill 组合 | 备注 |
|------|----------------|------|
| Pod 频繁崩溃 | `pod-health-analyst`, `rollout-analyst` | 加 `timeoutSeconds: 120` 快速响应 |
| 安全合规审查 | `pod-security-analyst`, `config-drift-analyst` | 建议 `scope: cluster` |
| 成本优化 | `pod-cost-analyst` | 建议全集群扫描 |
| 告警根因分析 | `alert-responder`, `node-health-analyst` | 需要 Prometheus 可访问 |
| 网络故障排查 | `network-troubleshooter` | 配合 `labelSelector` 缩小范围 |
| 存储问题 | `storage-analyst` | 关注 PVC 状态 |
| 发布卡住 | `rollout-analyst` | 指定发布中的命名空间 |

---

## 字段速查表

| 场景 | CRD | 关键字段 |
|------|-----|---------|
| 配置 AI 模型 | `ModelConfig` | `model`, `maxTurns`, `apiKeyRef` |
| 触发诊断 | `DiagnosticRun` | `target.scope`, `skills`, `modelConfigRef` |
| 扩展诊断能力 | `DiagnosticSkill` | `dimension`, `tools`, `prompt` |
| 预览修复效果 | `DiagnosticFix` | `strategy: dry-run` |
| 审批后自动修复 | `DiagnosticFix` | `strategy: auto`, `approvalRequired: true` |
| 创建新 K8s 资源 | `DiagnosticFix` | `strategy: create` |
| 失败自动回滚 | `DiagnosticFix` | `rollback.autoRollbackOnFailure: true` |

---

## 常见问题排查

```bash
# Q: DiagnosticRun 卡在 Pending？
kubectl get pods -n kube-agent-helper          # 看 Agent Pod 是否被创建
kubectl describe diagnosticrun <name> -n kube-agent-helper  # 看 events

# Q: DiagnosticRun 状态 Failed？
kubectl logs -n kube-agent-helper -l job-name=<run-name>   # 看 Agent 日志

# Q: Skill 没有被执行？
curl http://localhost:8080/api/skills          # 确认 Skill 已注册
kubectl get diagnosticskill -n kube-agent-helper

# Q: DiagnosticFix 应用后失败了，是否已回滚？
kubectl get dfix -n kube-agent-helper          # 查看 Phase 是否为 RolledBack
kubectl describe dfix <name> -n kube-agent-helper   # 查看 rollbackSnapshot 和 message
```
