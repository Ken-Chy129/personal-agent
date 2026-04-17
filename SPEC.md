# Personal Agent — 技术规格说明

## 1. 目标

构建一个基于 Go 的 agent 框架和个人助手应用，参考 Claude Code 的架构设计，实现一个生产级的 autonomous agent 系统。

### 1.1 项目定位

- **框架层**：Go 语言的 agent 核心库，提供 Tool 系统、Agent Loop、Provider 抽象、上下文管理、权限控制、Hook 机制
- **应用层**：基于框架构建的个人助手，支持 CLI 交互、新闻采集、知识库管理、主动触达等场景
- **可观测层**：内置 tracing、成本追踪，初期集成 Langfuse

### 1.2 设计原则

1. **参考 Claude Code，而非 Eino**：Claude Code 的 Tool 安全模型、多级上下文管理、Hook 机制是核心学习对象；Eino 仅作为 Provider 实现的参考
2. **应用驱动，框架自然生长**：先定义核心接口骨架，通过应用场景验证和演进设计
3. **内置可观测性**：每个 LLM 调用和 Tool 执行都产生结构化 trace，不是事后补的
4. **AI-Friendly**：对外暴露 CLI / API / MCP server，方便其他 agent 集成

### 1.3 Non-Goals（MVP 阶段）

- 不做通用 SDK 发布
- 不做 Web/Mobile UI
- 不自建可观测 dashboard
- 不做多 Agent 编排
- 不做 A2A 协议
- 不追求生产级部署（k8s / 高可用）

---

## 2. 架构

### 2.1 分层架构

```
┌──────────────────────────────────────────────────────┐
│  cmd/                                                 │
│  ├── agent/       CLI 入口（交互式 agent）             │
│  └── assistant/   个人助手应用（后期）                  │
├──────────────────────────────────────────────────────┤
│  internal/app/    应用层逻辑                           │
│  ├── assistant/   个人助手业务逻辑                     │
│  └── tools/       应用级 tool 实现                     │
├──────────────────────────────────────────────────────┤
│  pkg/agent/       框架核心                             │
│  ├── agent.go     Agent 接口 + AgentLoop 实现          │
│  ├── tool.go      Tool 接口 + 注册/调度                │
│  ├── permission.go 权限系统                           │
│  ├── hook.go      Hook 机制                           │
│  ├── context.go   上下文/对话管理                      │
│  └── compact.go   上下文压缩                          │
├──────────────────────────────────────────────────────┤
│  pkg/provider/    LLM Provider 抽象                    │
│  ├── provider.go  Provider 接口                       │
│  ├── openai/      OpenAI 实现                         │
│  └── claude/      Claude (Vertex AI) 实现              │
├──────────────────────────────────────────────────────┤
│  pkg/trace/       可观测性                             │
│  ├── span.go      Span 数据模型                       │
│  ├── tracer.go    Tracer 接口                         │
│  ├── logger.go    结构化日志 exporter                  │
│  └── langfuse/    Langfuse exporter（后期）            │
├──────────────────────────────────────────────────────┤
│  pkg/message/     消息模型                             │
│  ├── message.go   Message 类型定义                    │
│  └── normalize.go API 消息格式化                      │
├──────────────────────────────────────────────────────┤
│  pkg/memory/      记忆系统（后期）                      │
│  ├── memory.go    Memory 接口                         │
│  └── file.go      基于文件的持久化 Memory              │
└──────────────────────────────────────────────────────┘
```

### 2.2 核心数据流

```
User Input
    │
    ▼
┌─────────┐    ┌──────────┐    ┌──────────┐
│  Agent   │───▶│ Provider │───▶│ LLM API  │
│  Loop    │◀───│ (OpenAI) │◀───│          │
│          │    └──────────┘    └──────────┘
│          │
│          │──▶ Parse tool_use blocks
│          │
│          │    ┌──────────────────────────┐
│          │───▶│ Tool Orchestration       │
│          │    │ ├─ Permission Check      │
│          │    │ ├─ Pre-Hook              │
│          │    │ ├─ Execute (concurrent?) │
│          │    │ ├─ Post-Hook             │
│          │    │ └─ Trace Span            │
│          │◀───│                          │
│          │    └──────────────────────────┘
│          │
│          │──▶ Append tool_result to messages
│          │──▶ Check: need follow-up? → loop
│          │──▶ Check: context too long? → compact
│          │
└─────────┘
    │
    ▼
Output to User
```

---

## 3. 核心接口设计

### 3.1 Provider

```go
// pkg/provider/provider.go

// Provider 是 LLM API 的抽象接口
type Provider interface {
    // Chat 发送消息并返回完整响应
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

    // Stream 发送消息并返回流式响应
    Stream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)

    // Name 返回 provider 名称（用于 tracing 和日志）
    Name() string
}

type ChatRequest struct {
    Model       string
    Messages    []message.Message
    Tools       []ToolDefinition    // tool schema for LLM
    SystemPrompt string
    MaxTokens   int
    Temperature *float64
}

type ChatResponse struct {
    Content    []ContentBlock  // text blocks + tool_use blocks
    Usage      Usage
    StopReason string         // "end_turn", "tool_use", "max_tokens"
}

type StreamEvent struct {
    Type    string          // "content_delta", "tool_use_start", "tool_use_delta", "message_end"
    Delta   *ContentDelta
    Usage   *Usage          // 仅在 message_end 时有值
}

type Usage struct {
    InputTokens  int
    OutputTokens int
    // 用于成本计算
    Model        string
}
```

### 3.2 Tool

参考 Claude Code 的 Tool 设计，每个 tool 不只是"能做什么"，还声明安全属性和行为约束。

```go
// pkg/agent/tool.go

// Tool 定义了一个可被 agent 调用的工具
type Tool interface {
    // 基本信息
    Name() string
    Description() string
    InputSchema() json.RawMessage  // JSON Schema

    // 执行
    Execute(ctx context.Context, input json.RawMessage, tctx *ToolContext) (*ToolResult, error)

    // 安全属性（参考 Claude Code）
    IsReadOnly() bool       // 只读操作不需要严格权限
    IsConcurrencySafe() bool // 可并发执行的 tool 会被批量并行调用
    IsDestructive() bool     // 不可逆操作（删除、覆盖）需要额外确认

    // 权限检查（tool-specific 逻辑，在通用权限之后调用）
    CheckPermission(input json.RawMessage, pctx *PermissionContext) (PermissionResult, error)
}

// ToolContext 是 tool 执行时的上下文
type ToolContext struct {
    WorkingDir  string
    Messages    []message.Message  // 当前对话历史
    Tracer      trace.Tracer
    AbortChan   <-chan struct{}     // 用户中断信号
}

// ToolResult 是 tool 执行的结果
type ToolResult struct {
    Content     string             // 返回给 LLM 的文本内容
    IsError     bool
    // 可选：tool 可以注入新消息到对话中（参考 Claude Code 的 newMessages）
    NewMessages []message.Message
}

// PermissionResult 表示权限检查结果
type PermissionResult struct {
    Behavior     string  // "allow", "deny", "ask"
    Reason       string  // 拒绝原因（展示给用户）
    UpdatedInput json.RawMessage // 权限系统可以修改输入
}
```

### 3.3 Agent Loop

参考 Claude Code 的 query.ts，核心是一个循环：调 LLM → 解析 tool_use → 执行 tool → 拼回结果 → 继续。

```go
// pkg/agent/agent.go

type Agent struct {
    provider    provider.Provider
    tools       []Tool
    hooks       *HookManager
    permissions *PermissionManager
    tracer      trace.Tracer
    config      *AgentConfig
}

type AgentConfig struct {
    Model            string
    SystemPrompt     string
    MaxTurns         int     // 防止无限循环，默认 50
    MaxConcurrency   int     // tool 并发数，默认 10
    PermissionMode   string  // "default", "auto", "ask-always"
}

// Run 执行一轮完整的 agent 交互
// 返回一个 channel，调用方通过它接收流式事件
func (a *Agent) Run(ctx context.Context, messages []message.Message) (<-chan AgentEvent, error)

// AgentEvent 是 agent loop 产生的事件流
type AgentEvent struct {
    Type    string  // 见下方枚举

    // 根据 Type 不同，以下字段有不同含义
    Message     *message.Message    // assistant 消息
    ToolUse     *ToolUseEvent       // tool 调用事件
    ToolResult  *ToolResultEvent    // tool 结果事件
    Error       *ErrorEvent         // 错误
    Status      *StatusEvent        // 状态变更（compacting, waiting_permission 等）
}

// Event types:
// "assistant_text"      - LLM 文本输出（流式）
// "tool_use_start"      - 开始调用 tool
// "tool_use_progress"   - tool 执行进度
// "tool_use_result"     - tool 执行完成
// "permission_request"  - 需要用户确认
// "error"               - 可恢复错误
// "fatal_error"         - 不可恢复错误
// "compact"             - 上下文被压缩
// "done"                - 本轮结束
```

### 3.4 Hook 机制

参考 Claude Code 的 hooks 设计，通过 shell 命令扩展 agent 行为，无需写代码。

```go
// pkg/agent/hook.go

// HookPoint 定义了 hook 的触发点
type HookPoint string

const (
    HookPreToolUse   HookPoint = "pre_tool_use"
    HookPostToolUse  HookPoint = "post_tool_use"
    HookPreSampling  HookPoint = "pre_sampling"    // LLM 调用前
    HookPostSampling HookPoint = "post_sampling"   // LLM 调用后
    HookSessionStart HookPoint = "session_start"
    HookStop         HookPoint = "stop"            // agent 即将停止时
)

// HookConfig 定义一个 hook 规则
type HookConfig struct {
    Point   HookPoint
    // 匹配条件：仅在特定 tool 或特定模式时触发
    Match   *HookMatch
    // 执行的 shell 命令
    Command string
    // 超时时间
    Timeout time.Duration
}

type HookMatch struct {
    ToolName string  // 仅匹配特定 tool
    Pattern  string  // 模式匹配（如 "git *"）
}

// HookResult 是 hook 执行的结果
type HookResult struct {
    ExitCode int
    Stdout   string
    Stderr   string
    // hook 可以阻止操作继续
    Block    bool
    Reason   string
}
```

### 3.5 上下文管理

参考 Claude Code 的多级 compact 策略。MVP 先实现最基础的 auto-compact。

```go
// pkg/agent/context.go

// ContextManager 管理对话上下文，负责在 token 超限时压缩
type ContextManager struct {
    provider    provider.Provider  // 用于 compact 时的 LLM 调用
    maxTokens   int                // 上下文窗口大小
    threshold   float64            // 触发 compact 的阈值（如 0.8）
}

// CheckAndCompact 检查是否需要压缩，如果需要则执行
// 返回压缩后的消息列表
func (cm *ContextManager) CheckAndCompact(
    ctx context.Context,
    messages []message.Message,
) ([]message.Message, *CompactResult, error)

type CompactResult struct {
    PreTokenCount  int
    PostTokenCount int
    CompactUsage   provider.Usage  // compact 本身的 token 消耗
}
```

### 3.6 Trace 数据模型

```go
// pkg/trace/span.go

type Span struct {
    TraceID     string
    SpanID      string
    ParentID    string        // 空表示根 span
    Name        string        // "llm_call", "tool_execute", "agent_turn"
    StartTime   time.Time
    EndTime     time.Time
    Duration    time.Duration

    // LLM 调用特有
    Model       string
    InputTokens int
    OutputTokens int
    Cost        float64       // USD

    // Tool 调用特有
    ToolName    string
    ToolInput   string        // JSON
    ToolOutput  string        // 截断后的输出
    IsError     bool

    // 通用属性
    Attributes  map[string]any
}

// Tracer 接口，支持多种 exporter
type Tracer interface {
    StartSpan(name string, opts ...SpanOption) *Span
    EndSpan(span *Span)
    // Flush 确保所有 span 被发送
    Flush(ctx context.Context) error
}

// Exporter 将 span 发送到外部系统
type Exporter interface {
    Export(ctx context.Context, spans []*Span) error
}
```

---

## 4. 项目结构

```
personal-agent/
├── cmd/
│   └── agent/
│       └── main.go              # CLI 入口
├── pkg/
│   ├── agent/                   # 框架核心
│   │   ├── agent.go             # Agent 接口和 Loop 实现
│   │   ├── agent_test.go
│   │   ├── tool.go              # Tool 接口
│   │   ├── tool_registry.go     # Tool 注册和查找
│   │   ├── tool_orchestration.go # Tool 调度（并发/串行分区）
│   │   ├── permission.go        # 权限系统
│   │   ├── hook.go              # Hook 机制
│   │   ├── context.go           # 上下文管理
│   │   ├── compact.go           # 上下文压缩
│   │   └── config.go            # Agent 配置
│   ├── provider/                # LLM Provider
│   │   ├── provider.go          # Provider 接口
│   │   ├── openai/
│   │   │   ├── openai.go        # OpenAI 实现
│   │   │   └── openai_test.go
│   │   └── claude/
│   │       ├── claude.go        # Claude (Vertex AI) 实现
│   │       └── claude_test.go
│   ├── message/                 # 消息模型
│   │   ├── message.go           # Message 类型
│   │   └── normalize.go         # API 格式转换
│   ├── trace/                   # 可观测性
│   │   ├── span.go              # Span 数据模型
│   │   ├── tracer.go            # Tracer 实现
│   │   ├── cost.go              # 成本计算
│   │   └── exporter/
│   │       ├── logger.go        # 结构化日志 exporter
│   │       └── langfuse/        # Langfuse exporter（后期）
│   └── memory/                  # 记忆系统（后期）
│       ├── memory.go
│       └── file.go
├── internal/
│   ├── cli/                     # CLI 交互层
│   │   ├── repl.go              # REPL 交互循环
│   │   └── render.go            # 输出渲染
│   └── tools/                   # 内置 tool 实现
│       ├── bash.go              # Shell 命令执行
│       ├── file_read.go         # 文件读取
│       ├── file_write.go        # 文件写入
│       ├── file_edit.go         # 文件编辑
│       ├── glob.go              # 文件搜索
│       ├── grep.go              # 内容搜索
│       └── web_fetch.go         # HTTP 请求
├── configs/
│   ├── default.yaml             # 默认配置
│   └── hooks.yaml               # Hook 配置示例
├── docs/
│   └── ideas/
│       └── personal-agent.md    # 创意文档
├── go.mod
├── go.sum
├── SPEC.md                      # 本文件
└── README.md
```

---

## 5. 代码风格

### 5.1 Go 规范

- Go 1.22+（使用 range over func 等新特性）
- 遵循标准 Go 项目布局：`pkg/` 放可复用库，`internal/` 放应用内部代码，`cmd/` 放入口
- 接口定义在使用方，而非实现方（Go 惯例）——但框架核心接口（Tool, Provider）例外，定义在 `pkg/` 中
- 错误处理用 `fmt.Errorf("xxx: %w", err)` 包装，保留错误链
- 使用 `context.Context` 传递取消信号和超时
- 不用全局变量；依赖通过构造函数注入

### 5.2 命名约定

- 包名简短、小写：`agent`, `provider`, `trace`, `message`
- 接口名：名词（`Tool`, `Provider`, `Tracer`），不加 `I` 前缀
- 实现名：具体名称（`OpenAIProvider`, `BashTool`, `LogExporter`）
- 配置结构体：`XxxConfig`（如 `AgentConfig`, `ToolConfig`）
- 选项模式：对于可选参数使用 functional options（`WithXxx`）

### 5.3 项目约定

- 每个包有独立的 `_test.go` 文件
- `pkg/` 下的包不依赖 `internal/`
- Provider 实现不依赖 Agent 核心包（单向依赖）
- Trace 包是独立的，任何层都可以使用

---

## 6. 测试策略

### 6.1 单元测试

- Tool 接口的 mock 实现，用于测试 Agent Loop
- Provider 接口的 mock 实现（预设返回 tool_use / text / error）
- Tool 权限检查的边界测试
- 上下文 compact 的 token 计数测试

### 6.2 集成测试

- 真实 OpenAI API 调用（需要 API key，CI 中可选跳过）
- Agent Loop 端到端测试：用户输入 → tool 调用 → 最终输出
- Hook 执行的集成测试（shell 命令真实执行）

### 6.3 测试约定

- `go test ./...` 必须通过
- Mock 使用接口 + 结构体，不依赖 mock 框架
- 需要外部 API 的测试用 `//go:build integration` 标记
- 测试文件命名：`xxx_test.go`

---

## 7. 边界和约束

### 7.1 必须做的

- [x] Tool 接口必须包含安全属性（ReadOnly, ConcurrencySafe, Destructive）
- [x] Agent Loop 必须支持用户中断（通过 context cancel 或 abort channel）
- [x] 每次 LLM 调用和 Tool 执行必须产生 trace span
- [x] 每次 session 结束输出 token 使用量和成本汇总
- [x] Tool 并发执行：ConcurrencySafe 的 tool 批量并行，非安全的串行

### 7.2 需要先确认再做的

- [ ] Claude API 通过 Vertex AI 调用的可行性验证
- [ ] Langfuse Go SDK / REST API 的可用性
- [ ] 上下文 compact 的具体策略（是否需要多级）
- [ ] Hook 机制的配置格式（YAML？JSON？类似 Claude Code 的 settings.json？）

### 7.3 绝不做的

- 不在框架层引入 React/UI 概念（Claude Code 的 UI 层不复制）
- 不做 prompt engineering 的自动优化（那是评测框架的事）
- 不在 MVP 中做多 provider 自动切换/fallback
- 不在 MVP 中做多 agent 编排

---

## 8. MVP 里程碑分解

### M1: 最小可运行 Agent（Week 1-2）

**交付物**：一个 CLI 程序，能与 OpenAI API 对话，支持 tool calling。

- [ ] `pkg/provider/openai/` — OpenAI Chat Completion + Tool Use + Streaming
- [ ] `pkg/message/` — 消息模型定义
- [ ] `pkg/agent/tool.go` — Tool 接口定义
- [ ] `pkg/agent/agent.go` — 最小 Agent Loop（循环调用 LLM + 执行 tool）
- [ ] `pkg/agent/tool_orchestration.go` — Tool 调度（串行/并行分区）
- [ ] `internal/tools/bash.go` — Bash tool（执行 shell 命令）
- [ ] `internal/tools/file_read.go` — 文件读取 tool
- [ ] `internal/tools/file_write.go` — 文件写入 tool
- [ ] `internal/cli/repl.go` — 最简 CLI REPL
- [ ] `cmd/agent/main.go` — 入口

**验收标准**：
```bash
$ go run cmd/agent/main.go
> 帮我在当前目录创建一个 hello.go 文件，输出 Hello World
# Agent 调用 file_write tool，创建文件
# Agent 调用 bash tool，运行 go run hello.go
# 输出 Hello World
```

### M2: 权限 + Hook + Trace（Week 3-4）

- [ ] `pkg/agent/permission.go` — 权限系统（default/auto/ask-always 模式）
- [ ] `pkg/agent/hook.go` — Hook 机制（pre/post tool use）
- [ ] `pkg/trace/` — Span 数据模型 + Tracer + Log exporter
- [ ] `pkg/trace/cost.go` — 基于 model 和 token 数的成本计算
- [ ] session 结束时输出成本汇总
- [ ] 更多内置 tools：glob, grep, file_edit

### M3: 上下文管理 + 第一个真实应用（Week 5-8）

- [ ] `pkg/agent/compact.go` — 上下文压缩（基本 auto-compact）
- [ ] `pkg/agent/context.go` — token 计数和阈值检测
- [ ] 第一个应用场景：AI 新闻采集 → 写入知识库（Hugo 博客）
- [ ] Claude Provider（通过 Vertex AI）

### M4+: 持续演进

- Memory 系统（持久化记忆）
- Langfuse 集成
- 评测框架
- 多轮对话管理
- 主动触达（定时任务 + 事件驱动）
- Web UI

---

## 9. 依赖

### 核心依赖

| 包 | 用途 |
|----|------|
| `github.com/openai/openai-go` | OpenAI 官方 Go SDK |
| `github.com/anthropics/anthropic-sdk-go` | Claude API (Vertex AI) |
| `github.com/spf13/cobra` | CLI 命令行框架 |
| `github.com/fatih/color` | 终端彩色输出 |

### 可选依赖

| 包 | 用途 | 何时引入 |
|----|------|----------|
| `github.com/pkoukk/tiktoken-go` | Token 计数 | M2 |
| Langfuse REST API | 可观测性 | M3+ |
| `gopkg.in/yaml.v3` | 配置文件解析 | M2 |

---

## 10. 配置

Agent 通过 YAML 配置文件 + 环境变量初始化：

```yaml
# configs/default.yaml
agent:
  model: "gpt-4o"
  max_turns: 50
  max_concurrency: 10
  permission_mode: "default"  # default | auto | ask-always
  system_prompt: |
    You are a helpful assistant with access to tools.

provider:
  type: "openai"
  # 或
  # type: "claude"
  # project_id: "xxx"
  # region: "us-central1"

trace:
  enabled: true
  exporters:
    - type: "logger"
      level: "info"
    # - type: "langfuse"
    #   host: "https://cloud.langfuse.com"
    #   public_key: "xxx"
    #   secret_key: "xxx"

hooks:
  - point: "pre_tool_use"
    match:
      tool: "Bash"
      pattern: "rm -rf *"
    command: "echo 'BLOCKED: destructive command' && exit 1"
```

环境变量优先级高于配置文件：

- `OPENAI_API_KEY` — OpenAI API key
- `OPENAI_BASE_URL` — 自定义 API endpoint
- `GOOGLE_APPLICATION_CREDENTIALS` — GCP 认证（Claude via Vertex AI）
- `AGENT_MODEL` — 覆盖默认模型
- `AGENT_PERMISSION_MODE` — 覆盖权限模式
- `AGENT_DEBUG` — 启用 debug 日志
