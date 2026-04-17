# Personal Agent

## 问题陈述

**我们如何可能** 构建一个基于 Go 的 agent 框架 + 个人助手应用，既深入学习 agent 架构设计（参考 Claude Code），又为自己提供实际的提效工具（新闻采集、知识库管理、主动触达），同时内置 AI 可观测性能力（tracing、成本分析、评测）？

## 推荐方向

**「有骨架的应用驱动」**——先定义框架核心接口（Agent/Tool/Memory/Provider）+ 内置 tracing 数据模型，然后转入应用驱动节奏。

选择这个方向的原因：
- **框架先行的纯路径**风险太高——没有应用场景驱动容易过度设计，且缺少正反馈维持探索动力
- **纯应用路径**又不够——你明确想学习 agent 架构，需要有意识的分层设计
- **折中方案**：花 2-3 天定义核心骨架（接口 + tracing 数据模型），然后切入应用场景驱动迭代。框架从实践中验证和演进，而非凭空设计

### 技术选型

- **服务端/框架核心**：Go
- **前端/脚本**：TypeScript
- **可观测性**：初期集成 Langfuse（开源），框架内置 tracing hook，后期可替换为自建实现
- **LLM Provider**：以 Claude API 为主，Provider 接口支持扩展

### 架构分层

```
┌────────────────────────────────────────────────┐
│  应用层 (cmd/assistant)                         │
│  新闻采集、知识库写入、问答、主动触达、CLI/API     │
├────────────────────────────────────────────────┤
│  框架层 (pkg/agent)                             │
│  Agent Loop、Tool 系统、Memory、Provider 抽象    │
│  内置：tracing spans、token/cost tracking        │
├────────────────────────────────────────────────┤
│  可观测 & 评测 (pkg/observe)                     │
│  Trace 数据模型、Langfuse exporter              │
│  后期：自建 dashboard、评测框架、回归测试          │
├────────────────────────────────────────────────┤
│  基础设施 (pkg/llm, pkg/store)                   │
│  Claude/OpenAI API client、存储抽象              │
└────────────────────────────────────────────────┘
```

## 需要验证的关键假设

- [ ] Go 生态中 Claude API 的 SDK 是否成熟可用，还是需要自己封装 HTTP client —— 直接调研 anthropic-go SDK
- [ ] Langfuse 是否有 Go SDK 或 REST API 可直接对接 —— 查看 Langfuse 文档
- [ ] 个人助手的"主动触达"场景（定时任务、事件驱动）是否需要一个后台常驻服务，还是 cron job 够用 —— 先用 cron 验证，不够再升级
- [ ] 从 1-2 个应用场景提炼出的框架抽象是否足够通用 —— 实现 3 个不同类型的 tool 后评估
- [ ] 单人业余项目是否能维持长期动力 —— 设定里程碑，每个里程碑都有可用产出

## MVP 范围

**MVP 目标**：一个能通过 CLI 交互的 agent，能调用 Claude API，能执行自定义 tool，能追踪每次调用的 token 用量和成本。

具体包括：
1. **核心接口定义**：`Agent`、`Tool`、`Memory`、`Provider` 接口
2. **最小 Agent Loop**：接收用户输入 → 调用 LLM → 解析 tool call → 执行 tool → 返回结果
3. **Claude Provider**：对接 Claude API（tool use / streaming）
4. **2-3 个示例 Tool**：如 web_search、read_file、write_file
5. **内置 Tracing**：每次 LLM 调用和 tool 调用产生结构化 trace（span_id, duration, tokens, cost）
6. **CLI 入口**：简单的命令行交互界面
7. **成本统计**：session 级别的 token 和费用汇总

**不在 MVP 中**：
- Web/Mobile UI
- 新闻采集、知识库等具体应用功能
- Langfuse 集成（MVP 先用 structured log 输出 trace）
- 持久化 Memory
- 多轮对话管理
- 评测框架

## 不做的事（及原因）

- **不做通用 SDK 发布** —— 在没有 3+ 个应用场景验证之前，过早发布 SDK 会锁死不成熟的接口
- **不做 Web/Mobile UI** —— MVP 阶段 CLI 足够，UI 是后期在框架验证之后的事
- **不自建可观测 dashboard** —— 初期集成 Langfuse 获得能力，理解数据模型后再决定是否自建
- **不支持多 Provider 切换** —— MVP 只对接 Claude，Provider 接口预留扩展点但不实现多个
- **不做多 Agent 编排** —— 先把单 Agent 做好，多 Agent 是后期演进
- **不做 A2A 协议** —— 应用场景还不够清晰，过早设计协议会变成枷锁
- **不追求生产级部署** —— 这是个人工具，不需要 k8s / 高可用 / 灰度发布

## 里程碑规划

| 里程碑 | 产出 | 核心学习 |
|--------|------|----------|
| M1: 最小 Agent | CLI agent + Claude API + tool calling + tracing | Agent loop 设计、tool use 协议 |
| M2: 第一个真实应用 | AI 新闻采集 → 写入知识库 | Tool 实现模式、Memory 需求发现 |
| M3: 可观测性 | Langfuse 集成 + 成本 dashboard | AI 可观测数据模型 |
| M4: 对话 & 记忆 | 多轮对话 + 持久化 Memory | Context 管理、Memory 架构 |
| M5: 评测 | 简单评测框架 + 回归测试 | Prompt 评测方法论 |
| M6: 主动 Agent | 定时任务 + 事件驱动 + 主动触达 | Agent 自主性设计 |
| M7: 多端 | Web UI（后期可能 Mobile） | 前后端分离、API 设计 |

## 待解决问题

- 知识库是复用现有 Hugo 博客还是另建系统？（建议复用，减少重复建设）
- Agent 的"主动触达"通过什么渠道？（飞书消息？邮件？Push 通知？）
- 是否需要本地运行还是部署到服务器？（影响 Memory 持久化和定时任务方案）
- Go 的 Claude API 调用是否需要代理？（网络环境约束）
