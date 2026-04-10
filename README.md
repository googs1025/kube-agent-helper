# kube-agent-helper

> Kubernetes 原生的 AI 诊断与优化助手

**kube-agent-helper** 是一个跑在 Kubernetes 集群里、专门分析和优化 K8s 资源的 AI Agent。通过 CRD 声明诊断任务，Controller 编排 Agent Pod 执行分析，结合 MCP 工具链和 LLM 能力产出结构化的 findings 报告。

## ✨ 核心特性

- 🔍 **四维度诊断** — 健康 / 安全 / 成本 / 可靠性
- 📦 **CRD 驱动** — 用 `DiagnosticSkill` 和 `DiagnosticRun` 声明扩展点与任务
- 🧩 **声明式 Skill 系统** — Skill 即 CR，支持内置 + 用户自定义，GitOps 友好
- 🤖 **Claude Agent SDK 引擎** — 原生支持 SKILL.md，multi-turn agentic loop 直接读 K8s 资源并深挖根因
- 🔐 **最小权限自动生成** — Translator 根据 Skill 需求自动收缩 SA/Role
- 🧠 **向量案例库** — 历史 finding embedding 入库，新诊断检索相似案例注入 prompt
- 🔄 **双通道数据采集** — 实时 Watch K8s events + 按需 prefetch

## 🏗️ 架构概览

```
用户 → CR apply → Controller → Translator → Agent Job/Pod
                     ↓                         ↓
                  SkillRegistry           MCP Tools
                     ↓                         ↓
                  Postgres (findings, case_memory, events)
                     ↑
                Event Collector (Watch + Prometheus)
```

详细设计见 [docs/design.md](docs/design.md)。

## 📚 参考项目

- [kagent](https://github.com/kagent-dev/kagent) — K8s 原生 Agent 编排框架（借 CRD / Operator / Translator / DB 层 / A2A+MCP 协议）
- [ci-agent](https://github.com/googs1025/ci-agent) — GitHub CI 流水线 AI 分析器（借声明式 Skill / 动态 Orchestrator / 双引擎 / 双数据通道）

## 🗺️ 路线图

- **Phase 1** — Operator MVP：三个 CRD + 单次诊断 Job + 2 个内置 Skill
- **Phase 2** — Skill Registry + 多维度分析 + Dashboard
- **Phase 3** — 实时事件采集 + 向量记忆库
- **Phase 4** — 生产加固：最小权限 / Sandbox / OIDC / HITL

## 📄 License

Apache License 2.0
