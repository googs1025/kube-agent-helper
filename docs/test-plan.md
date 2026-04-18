# kube-agent-helper 生产就绪测试计划

## 1. 现状总结

| 模块 | 现有测试 | 覆盖率 | 评估 |
|------|---------|--------|------|
| MCP 工具 (14 个) | 13 个 test 文件 | ~95% | 优秀，仅 register.go 未测 |
| HTTP Server | 16 个 test | ~75% | 缺 fix approve/reject/get |
| Reconciler (4 个) | 2 个 test 文件 | ~50% | 缺 ModelConfig + Skill |
| SQLite Store | 1 个 test 文件 | ~60% | 缺 Fix CRUD |
| Translator | 2 个 test 文件 | ~90% | 良好 |
| Registry | 1 个 test 文件 | ~90% | 良好 |
| Envtest 集成 | 2 个 test 文件 | — | CRD 基础验证 |
| Dashboard 前端 | **0 个 test** | **0%** | **无任何测试** |
| Python Agent | **0 个 test** | **0%** | **无任何测试** |

**总计：29 个 Go test 文件，~117 个测试用例，前端 0 个。**

---

## 2. 测试分层策略

```
┌─────────────────────────────────────────┐
│        E2E Tests (Playwright)           │  ← 少量关键路径
│  浏览器 → Dashboard → API → K8s 集群    │
├─────────────────────────────────────────┤
│     Integration Tests (envtest/kind)     │  ← Reconciler + CRD 完整流程
│  Controller → K8s API → Store → Job      │
├─────────────────────────────────────────┤
│         API Tests (httptest)             │  ← 每个 endpoint 正常+异常
│  HTTP Handler → Store Mock → K8s Fake    │
├─────────────────────────────────────────┤
│       Component Tests (Vitest/RTL)       │  ← 前端组件 + 页面
│  React Component → Mock API → DOM        │
├─────────────────────────────────────────┤
│         Unit Tests (go test)             │  ← 每个工具/函数
│  MCP Tool → Fake K8s Client → JSON       │
└─────────────────────────────────────────┘
```

---

## 3. Phase 1：后端关键路径补全（预估 3 天）

### 3.1 HTTP Server — Fix 审批流程

**文件**：`internal/controller/httpserver/server_test.go`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 1 | `TestGetFix_Success` | GET /api/fixes/:id 返回 fix 详情含 BeforeSnapshot | P0 |
| 2 | `TestGetFix_NotFound` | GET /api/fixes/:id 返回 404 | P0 |
| 3 | `TestApproveFix_Success` | PATCH /api/fixes/:id/approve 更新 phase → Approved | P0 |
| 4 | `TestApproveFix_MissingApprovedBy` | approve 缺少 approvedBy 字段返回 400 | P0 |
| 5 | `TestApproveFix_AlreadyApplied` | approve 已 Succeeded 的 fix 返回冲突 | P1 |
| 6 | `TestRejectFix_Success` | PATCH /api/fixes/:id/reject 更新 phase → Failed | P0 |
| 7 | `TestRejectFix_NotFound` | reject 不存在的 fix 返回 404 | P1 |

### 3.2 SQLite Store — Fix CRUD

**文件**：`internal/store/sqlite/sqlite_test.go`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 8 | `TestCreateFix_Success` | CreateFix 写入并返回完整 Fix | P0 |
| 9 | `TestGetFix_Success` | GetFix 按 ID 读取 | P0 |
| 10 | `TestListFixes` | ListFixes 返回全部 fix | P0 |
| 11 | `TestListFixesByRun` | ListFixesByRun 按 runID 过滤 | P0 |
| 12 | `TestUpdateFixPhase` | UpdateFixPhase 更新阶段 | P0 |
| 13 | `TestUpdateFixApproval` | UpdateFixApproval 写入 approvedBy + 时间 | P0 |
| 14 | `TestUpdateFixSnapshot` | UpdateFixSnapshot 写入 BeforeSnapshot | P1 |
| 15 | `TestGetFix_NotFound` | 不存在的 ID 返回 ErrNotFound | P1 |

### 3.3 MCP 工具注册

**新文件**：`internal/mcptools/register_test.go`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 16 | `TestRegisterCore_ToolCount` | RegisterCore 注册 4 个核心工具 | P0 |
| 17 | `TestRegisterExtension_ToolCount` | RegisterExtension 注册 10 个扩展工具 | P0 |
| 18 | `TestRegisterAll_TotalCount` | RegisterAll 注册全部 14 个工具 | P0 |
| 19 | `TestRegisterAll_NoDuplicateNames` | 无重复工具名 | P0 |
| 20 | `TestRegisteredTool_HasDescription` | 每个工具都有非空 description | P1 |
| 21 | `TestRegisteredTool_HasInputSchema` | 每个工具都有 inputSchema | P1 |

---

## 4. Phase 2：Reconciler + Agent 补全（预估 4 天）

### 4.1 Skill Reconciler

**新文件**：`internal/controller/reconciler/skill_reconciler_test.go`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 22 | `TestSkillReconciler_CreateBuiltinSkill` | 内置 skill CR 创建 → store upsert | P0 |
| 23 | `TestSkillReconciler_UpdateSkill` | skill CR 更新 → store 同步 | P0 |
| 24 | `TestSkillReconciler_DeleteSkill` | skill CR 删除 → store 删除 | P0 |
| 25 | `TestSkillReconciler_InvalidSpec` | 缺少必填字段不应 panic | P1 |
| 26 | `TestSkillReconciler_DisabledSkill` | enabled=false 时 store 标记禁用 | P1 |

### 4.2 ModelConfig Reconciler

**新文件**：`internal/controller/reconciler/modelconfig_reconciler_test.go`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 27 | `TestModelConfigReconciler_CreateConfig` | ModelConfig CR 创建 → store 记录 | P0 |
| 28 | `TestModelConfigReconciler_SecretRef` | 引用的 Secret 不存在 → 写入 error 状态 | P0 |
| 29 | `TestModelConfigReconciler_UpdateConfig` | config 更新 → store 同步 | P1 |

### 4.3 Envtest 增强

**文件**：`test/envtest/`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 30 | `TestDiagnosticRun_FullLifecycle` | 创建 Run → Job → Findings → Completed | P0 |
| 31 | `TestDiagnosticFix_ApproveAndApply` | Fix DryRun → Approve → Apply → Succeeded | P0 |
| 32 | `TestDiagnosticRun_Timeout` | timeoutSeconds 到期 → Failed | P1 |
| 33 | `TestDiagnosticRun_PodStatusCapture` | Pod ImagePullBackOff → status.message | P1 |
| 34 | `TestDiagnosticSkill_CRUD` | Skill CR 创建/更新/删除全流程 | P1 |

### 4.4 Python Agent 基础测试

**新文件**：`agent-runtime/tests/test_skill_loader.py`、`test_mcp_client.py`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 35 | `test_skill_loader_parse_md` | 解析 SKILL.md frontmatter | P0 |
| 36 | `test_skill_loader_missing_field` | 缺少字段时报错 | P1 |
| 37 | `test_mcp_client_parse_response` | 解析 MCP JSON 响应 | P1 |
| 38 | `test_orchestrator_language_inject` | outputLanguage 注入 prompt | P1 |

---

## 5. Phase 3：前端测试建立（预估 8 天）

### 5.1 基础设施搭建

```bash
# 安装 Vitest + React Testing Library
npm install --save-dev vitest @vitest/ui jsdom \
  @testing-library/react @testing-library/jest-dom @testing-library/user-event \
  msw  # Mock Service Worker 用于拦截 API
```

**vitest.config.ts**：
```typescript
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import path from 'path'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    globals: true,
  },
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
})
```

**package.json 新增**：
```json
{
  "scripts": {
    "test": "vitest",
    "test:ui": "vitest --ui",
    "test:coverage": "vitest --coverage"
  }
}
```

### 5.2 核心工具函数测试

**新文件**：`dashboard/src/lib/__tests__/symptoms.test.ts`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 39 | `symptomsToSkills_singleSymptom` | 单个症状映射正确的 skill 列表 | P0 |
| 40 | `symptomsToSkills_multipleSymptoms` | 多症状去重合并 | P0 |
| 41 | `symptomsToSkills_fullCheck` | full-check 返回全部 skill | P0 |
| 42 | `symptomsToSkills_emptyArray` | 空数组返回空 | P1 |
| 43 | `SYMPTOM_PRESETS_valid` | 每个 preset 有 id/label_zh/label_en/skills | P1 |

**新文件**：`dashboard/src/lib/__tests__/api.test.ts`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 44 | `createRun_success` | POST /api/runs → 返回 UUID | P0 |
| 45 | `createRun_httpError` | 非 2xx → throw Error | P0 |
| 46 | `createSkill_success` | POST /api/skills 成功 | P1 |
| 47 | `approveFix_success` | PATCH approve 成功 | P1 |
| 48 | `rejectFix_success` | PATCH reject 成功 | P1 |
| 49 | `generateFix_returnsFixID` | POST generate-fix 返回 fixID | P1 |

### 5.3 Context Provider 测试

**新文件**：`dashboard/src/i18n/__tests__/context.test.tsx`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 50 | `I18nProvider_defaultLang` | 默认渲染中文 | P0 |
| 51 | `I18nProvider_switchLang` | setLang("en") 后文案切换 | P0 |
| 52 | `t_nestedKey` | t("runs.stat.total") 正确解析嵌套 key | P0 |
| 53 | `t_unknownKey` | 未知 key 返回 key 本身 | P1 |
| 54 | `I18nProvider_localStorage` | setLang 后 localStorage 更新 | P1 |

**新文件**：`dashboard/src/theme/__tests__/context.test.tsx`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 55 | `ThemeProvider_defaultDark` | 默认深色主题 | P0 |
| 56 | `ThemeProvider_switchLight` | 切换浅色 → class 移除 dark | P0 |

### 5.4 组件测试

**新文件**：`dashboard/src/components/__tests__/phase-badge.test.tsx`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 57 | `PhaseBadge_Running` | Running 显示蓝色 | P1 |
| 58 | `PhaseBadge_Succeeded` | Succeeded 显示绿色 | P1 |
| 59 | `PhaseBadge_Failed` | Failed 显示红色 | P1 |

**新文件**：`dashboard/src/components/__tests__/severity-badge.test.tsx`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 60 | `SeverityBadge_critical` | critical 显示红色 | P1 |
| 61 | `SeverityBadge_low` | low 显示灰色 | P1 |

### 5.5 页面测试（MSW Mock API）

**新文件**：`dashboard/src/app/diagnose/__tests__/page.test.tsx`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 62 | `DiagnosePage_renders` | 页面渲染标题和表单 | P0 |
| 63 | `DiagnosePage_selectNamespace` | 选择 namespace 后资源列表加载 | P0 |
| 64 | `DiagnosePage_selectSymptoms` | 勾选症状后提交按钮可用 | P0 |
| 65 | `DiagnosePage_submit` | 提交后跳转到 /diagnose/:uuid | P0 |
| 66 | `DiagnosePage_fullCheck` | 选 full-check 清除其他勾选 | P1 |
| 67 | `DiagnosePage_submitError` | API 报错时显示错误信息 | P1 |

**新文件**：`dashboard/src/app/diagnose/__tests__/[id]/page.test.tsx`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 68 | `DiagnoseResultPage_running` | 运行中显示 spinner | P0 |
| 69 | `DiagnoseResultPage_findings` | 完成后按严重程度排序展示 | P0 |
| 70 | `DiagnoseResultPage_generateFix` | 点击"生成修复"调用 API | P1 |
| 71 | `DiagnoseResultPage_failed` | 失败状态显示错误信息 | P1 |

**其他页面**：

| # | 文件 | 测试用例 | 优先级 |
|---|------|---------|--------|
| 72 | `app/__tests__/page.test.tsx` | 首页渲染 run 列表 | P1 |
| 73 | `app/skills/__tests__/page.test.tsx` | Skills 页面渲染 10 个 skill | P1 |
| 74 | `app/fixes/__tests__/page.test.tsx` | Fixes 页面渲染列表 + approve/reject | P1 |
| 75 | `app/runs/__tests__/[id]/page.test.tsx` | Run 详情页渲染 findings | P1 |

### 5.6 API Proxy 测试

**新文件**：`dashboard/src/app/api/__tests__/proxy.test.ts`

| # | 测试用例 | 说明 | 优先级 |
|---|---------|------|--------|
| 76 | `proxy_forwardsQueryString` | /api/k8s/resources?kind=Namespace 转发完整 URL | P0 |
| 77 | `proxy_forwardsPostBody` | POST body 正确转发 | P0 |
| 78 | `proxy_returnsBackendStatus` | 后端 4xx/5xx 原样返回 | P1 |

---

## 6. Phase 4：E2E 测试（预估 5 天）

### 6.1 Playwright 环境

```bash
npm install --save-dev @playwright/test
npx playwright install
```

### 6.2 关键用户旅程

| # | 场景 | 步骤 | 优先级 |
|---|------|------|--------|
| 79 | **症状驱动诊断** | 打开 /diagnose → 选 namespace → 选 Deployment → 勾选 "Pod 无法启动" + "频繁重启" → 提交 → 等待结果 → 看到 findings 按严重程度排序 | P0 |
| 80 | **管理员创建 Run** | 打开 / → Create Run → 填写表单 → 提交 → 跳转详情页 → 等待 Running → Succeeded | P0 |
| 81 | **Fix 审批流程** | 找到 completed run → 点 Generate Fix → 等待 fix 生成 → 查看 Before/After diff → Approve → 确认 Succeeded | P0 |
| 82 | **Skill 管理** | 打开 /skills → 看到 10 个 builtin skill → 展开查看 prompt → 创建自定义 skill → 确认列表更新 | P1 |
| 83 | **语言切换** | 切到 English → 所有页面文案变英文 → 切回中文 → 刷新后保持中文 | P1 |
| 84 | **深浅色主题** | 切到 light → 背景变白 → 刷新后保持 light → 切回 dark | P2 |
| 85 | **Fix 拒绝** | 生成 fix → Reject → 确认状态变 Failed | P1 |
| 86 | **超时处理** | 创建 run 设置 timeoutSeconds=10 → 等待超时 → 确认 status=Failed + message 含 timeout | P1 |

### 6.3 E2E 环境要求

```yaml
# 需要一个 kind/minikube 集群 + 部署完整 Helm chart
# CI 中可复用 smoke-test job 的 kind 集群
prerequisites:
  - kind cluster running
  - CRDs installed
  - controller deployed with test image
  - dashboard accessible at localhost:3000
  - Anthropic API key (或 mock LLM server)
```

---

## 7. Phase 5：性能和压力测试（预估 3 天）

| # | 测试场景 | 方法 | 通过标准 | 优先级 |
|---|---------|------|---------|--------|
| 87 | API 并发 | `wrk -t4 -c100 -d30s /api/runs` | P99 < 200ms，无 5xx | P1 |
| 88 | SQLite 并发写入 | 50 并发 CreateFinding | 无死锁，全部成功 | P1 |
| 89 | 大量 findings | 1 个 run 产生 500+ findings | 列表加载 < 2s | P2 |
| 90 | Dashboard 初始加载 | Lighthouse audit | LCP < 2s，FCP < 1s | P2 |
| 91 | 同时 10 个 DiagnosticRun | 10 个 Job 并行 | 全部完成，无资源泄漏 | P1 |
| 92 | 长时间运行 | 连续 24h 运行 controller | 内存无泄漏，Pod 不重启 | P2 |

---

## 8. 安全测试（预估 2 天）

| # | 测试场景 | 方法 | 优先级 |
|---|---------|------|--------|
| 93 | RBAC 最小权限 | 验证 controller SA 无法执行未授权操作（如 delete namespace） | P0 |
| 94 | Agent Pod 隔离 | 验证 agent job 只有 view ClusterRole，无 write 权限 | P0 |
| 95 | Secret 不泄漏 | API 响应和 findings 中不含 API key 明文 | P0 |
| 96 | SQL 注入 | 向 API 提交 `'; DROP TABLE --` 类输入 | P0 |
| 97 | API 输入校验 | 超长 name、特殊字符、空 body | P1 |
| 98 | Dashboard XSS | findings 中注入 `<script>` 标签，验证不执行 | P1 |
| 99 | Pod ServiceAccount | 验证 agent pod automountServiceAccountToken 配置 | P1 |

---

## 9. CI 集成方案

```yaml
# .github/workflows/ci.yml 新增

  # Phase 1-2: 后端测试（已有，需扩展覆盖率检查）
  test:
    steps:
      - run: go test ./... -race -count=1 -coverprofile=coverage.out
      - run: go tool cover -func=coverage.out  # 输出覆盖率报告

  # Phase 3: 前端测试（新增）
  dashboard-test:
    name: Dashboard Unit Tests
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with: { node-version: 20, cache: npm, cache-dependency-path: dashboard/package-lock.json }
      - run: npm ci
        working-directory: dashboard
      - run: npm run test -- --coverage
        working-directory: dashboard

  # Phase 4: E2E（新增）
  e2e:
    name: E2E Tests
    runs-on: ubuntu-latest
    needs: [build]
    steps:
      - uses: helm/kind-action@v1
      - run: kubectl apply -f deploy/helm/templates/crds/
      - run: helm install kah deploy/helm --namespace kube-agent-helper --create-namespace
      - run: npx playwright test
        working-directory: dashboard
```

---

## 10. 覆盖率目标

| 模块 | 当前 | 目标 | 截止日期 |
|------|------|------|---------|
| MCP 工具 | ~95% | ≥95% | 维持 |
| HTTP Server | ~75% | ≥90% | Phase 1 |
| Reconciler | ~50% | ≥80% | Phase 2 |
| SQLite Store | ~60% | ≥90% | Phase 1 |
| Translator | ~90% | ≥90% | 维持 |
| Dashboard 工具函数 | 0% | ≥90% | Phase 3 |
| Dashboard 组件 | 0% | ≥70% | Phase 3 |
| Dashboard 页面 | 0% | ≥60% | Phase 3 |
| E2E 关键路径 | 0% | 8 条 | Phase 4 |
| **整体后端** | **~65%** | **≥85%** | **Phase 2 完成** |
| **整体前端** | **0%** | **≥60%** | **Phase 3 完成** |

---

## 11. 执行时间表

```
Week 1 ─── Phase 1: 后端关键路径 (21 个测试)
            ├── HTTP fix approve/reject/get (7 tests)
            ├── SQLite fix CRUD (8 tests)
            └── MCP tool registration (6 tests)

Week 2 ─── Phase 2: Reconciler + Agent (16 个测试)
            ├── Skill Reconciler (5 tests)
            ├── ModelConfig Reconciler (3 tests)
            ├── Envtest 增强 (5 tests)
            └── Python Agent 基础 (4 tests, pytest)

Week 3-4 ── Phase 3: 前端测试建立 (40 个测试)
            ├── 基础设施搭建 (Vitest + RTL + MSW)
            ├── 工具函数 (11 tests)
            ├── Context Provider (6 tests)
            ├── 组件 (5 tests)
            ├── 页面 (14 tests)
            └── API Proxy (3 tests)

Week 5 ─── Phase 4: E2E (8 条关键路径)
            ├── Playwright 环境搭建
            └── 8 条用户旅程

Week 6 ─── Phase 5 + 安全 (13 个测试)
            ├── 性能基线 (6 tests)
            └── 安全验证 (7 tests)
```

**总计：~99 个新增测试用例 + 8 条 E2E 路径**

---

## 12. 验收标准

项目可以正式上线当且仅当：

- [ ] Phase 1 全部 P0 测试通过
- [ ] Phase 2 全部 P0 测试通过
- [ ] Phase 3 症状驱动诊断页面测试通过
- [ ] Phase 4 至少 3 条 P0 E2E 路径通过
- [ ] 安全测试 #93-#96 全部通过
- [ ] CI pipeline 绿色（所有 job 通过）
- [ ] 后端覆盖率 ≥ 85%
- [ ] go vet / eslint / tsc 零 error