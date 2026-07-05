# xcli

[English](README.md) | [简体中文](README.zh-CN.md)

[手工测试指南](docs/manual-testing.zh-CN.md)

`xcli` 是一个统一入口，用于安装、运行、切换和编排主流 AI Coding Agent CLI。

它直接管理真实的 CLI 进程，而不是用模型 API 替代这些工具。原生认证、权限、插件、MCP 配置及 Agent 专属参数均可继续使用。

## 支持的 Agent

| Agent | 交互模式 | 非交互模式 | 工作流结构化输出 | ACP | npm | Homebrew |
| --- | --- | --- | --- | --- | --- | --- |
| [Claude Code](https://code.claude.com/docs/en/cli-usage) | `claude` | `claude -p` | 流式 JSON | `claude-agent-acp` 桥接器 | `@anthropic-ai/claude-code` | — |
| [Codex CLI](https://developers.openai.com/codex/cli) | `codex` | `codex exec` | JSONL | `codex-acp` 桥接器 | `@openai/codex` | `--cask codex` |
| [Gemini CLI](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/headless.md) | `gemini` | `gemini -p` | 流式 JSON | 原生 `--acp` | `@google/gemini-cli` | `gemini-cli` |
| [OpenCode](https://opencode.ai/docs/cli/) | `opencode` | `opencode run` | JSON 事件 | 原生 `acp` | `opencode-ai` | `anomalyco/tap/opencode` |

无需重新构建 xcli，即可通过声明式配置添加别名和自定义 Agent。

## 安装

从 [GitHub Releases](https://github.com/Garden12138/xcli/releases) 页面下载预构建归档。当前提供 macOS 和 Linux 的 amd64、arm64 版本。

从 `darwin_amd64`、`darwin_arm64`、`linux_amd64` 或 `linux_arm64` 中选择对应平台，然后校验并安装：

```bash
VERSION=0.2.0
PLATFORM=darwin_arm64
ARCHIVE="xcli_${VERSION}_${PLATFORM}.tar.gz"

curl -fLO "https://github.com/Garden12138/xcli/releases/download/v${VERSION}/${ARCHIVE}"
curl -fLO "https://github.com/Garden12138/xcli/releases/download/v${VERSION}/checksums.txt"

# Linux：将 `shasum -a 256` 替换为 `sha256sum`
grep -F "  ${ARCHIVE}" checksums.txt | shasum -a 256 --check

tar -xzf "${ARCHIVE}"
mkdir -p "$HOME/.local/bin"
mv xcli "$HOME/.local/bin/xcli"
xcli --version
```

macOS 归档尚未进行代码签名或公证。运行下载的二进制文件前，请务必校验发布的 SHA-256 校验和。

### 从源码构建

xcli 以 Go 1.26 为目标版本，构建结果为独立二进制文件：

```bash
go build -o xcli .
./xcli --help
```

## 快速开始

```bash
# 创建 ~/.config/xcli/config.yaml
xcli config init

# 查看已配置和已安装的 Agent
xcli agents
xcli doctor

# 使用原生工具安装并认证
xcli install codex
xcli auth login codex

# 选择并使用默认 Agent
xcli default codex
xcli use

# 通过 stdio 向 ACP 客户端暴露所选 Agent
xcli acp codex

# 预览并同步声明的 MCP server
xcli mcp plan
xcli mcp sync
xcli mcp plan --scope project --project .

# 安全导入现有原生 MCP 条目
xcli mcp import plan
xcli mcp import apply

# 执行单次任务；Agent 专属参数放在 -- 之后
xcli run codex "审查当前变更"
xcli run "修复失败的测试" -- --sandbox workspace-write
xcli route "审查认证相关改动"

# 以交互方式或追加提示词恢复已记录会话
xcli resume <run-id>
xcli resume <run-id> "继续执行下一步" --json

# 在后台执行非交互任务
xcli run --detach "运行完整测试套件"
xcli jobs list

# 运行工作流（并行需显式启用）
xcli workflow validate examples/implement-and-review.yaml
xcli workflow run examples/implement-and-review.yaml \
  --var requirement="实现缓存失效修复"

# 汇总已记录的 Token 用量和原生费用估算
xcli usage --days 7
```

`--var` 会覆盖工作流中已经声明的变量；未声明的键会被拒绝。

仅当第一个位置参数与已配置的 Agent 完全匹配时，`xcli run` 才会将其视为 Agent。任何时候都可以使用 `--agent` 消除歧义。

## 配置

xcli 默认只加载用户级配置：

```text
$XDG_CONFIG_HOME/xcli/config.yaml
~/.config/xcli/config.yaml
```

仓库内配置不会被隐式加载。仅当项目信息可信时，才通过 `--config path/to/config.yaml` 显式指定。

代理分流、直连、Agent 别名和原生默认参数示例请参阅 [`examples/config.yaml`](examples/config.yaml)。

### 自定义 Agent

自定义命令使用 argv 模板，而不是 shell 片段：

```yaml
agents:
  custom:
    adapter: generic
    command: my-agent
    interactive_args: []
    run_args: ["run", "{{ prompt }}"]
    auth_args: ["auth", "login"]
    output: text
    acp:
      command: my-agent-acp
      args: ["--stdio"]
```

即使提示词包含空格、分号或 shell 元字符，它仍会作为单个进程参数传递。支持的输出模式为 `text`、`json` 和 `jsonl`。

### ACP stdio 入口

`xcli acp [agent] [-- native-args...]` 启动所选 Agent 的 [Agent Client Protocol](https://agentclientprotocol.com/) 服务，并将 stdin、stdout、stderr 直接连接到 ACP 客户端。位置参数优先于 `default_agent`；xcli 不检查协议流，因此不会应用提示词路由。`--cwd` 用于设置子进程目录，已有的 Agent 环境变量和网络配置仍然生效。

Gemini 和 OpenCode 使用原生 ACP 命令。Claude 与 Codex 需要显式安装维护中的桥接器；xcli 启动协议服务时不会联网下载：

```bash
npm install -g @agentclientprotocol/claude-agent-acp
npm install -g @agentclientprotocol/codex-acp
```

可选的 `agents.<name>.acp` 是完整命令覆盖：其中的参数会替代普通 Agent `args`，而 `--` 后的参数会继续追加。generic Agent 必须配置该结构后才能用于 `xcli acp`。

xcli 不解析、捕获或持久化 ACP 消息。一个连接可以承载多个会话和任务，因此 ACP 进程不会创建运行记录，也不会计入 `xcli usage`。协议版本协商、认证、权限和能力处理均由下游 ACP 服务与客户端负责。

### MCP 配置同步

在一处声明 MCP server，并按用户级或项目级 scope 同步到已安装的 Claude、Codex、Gemini 和 OpenCode：

```yaml
mcp:
  servers:
    local-tools:
      transport: stdio
      command: npx
      args: ["-y", "@example/mcp-server"]
      cwd: ./tools
      env:
        LOG_LEVEL: info
      env_vars: [SERVICE_TOKEN]
      targets: [claude, codex, gemini, opencode]

    docs:
      transport: http
      url: https://example.com/mcp
```

省略 `targets` 时默认包含四个内置 Adapter。相对 `cwd` 以 xcli 配置目录为基准。`env_vars` 只在 `xcli mcp serve` 启动本地 server 时复制，并且当时必须存在；敏感值应通过它传递，不要写入 `env`。

对于 stdio server，原生客户端启动 xcli，再由 xcli 应用真实命令、工作目录和最小环境。HTTP URL 直接写入原生配置，各客户端自行完成 OAuth 登录。首版不负责 SSE、静态鉴权 header、工具策略或厂商专属 OAuth 字段。

```bash
xcli mcp plan
xcli mcp plan --target codex --json
xcli mcp sync
xcli mcp sync --yes
```

默认的 user scope 保持已有机器级行为：stdio launcher 和源配置均写入绝对路径。若 xcli 构建在临时目录，需通过 `--launcher` 指定稳定安装路径；移动任一路径后必须重新同步。用户级 OpenCode JSONC 修改会保留注释和无关设置、创建 `.xcli.bak` 备份，并使用原子写入。

project scope 写入可供团队共享的可移植配置，并要求 xcli 源配置位于指定项目内：

```bash
xcli --config .xcli/config.yaml mcp plan --scope project --project .
xcli --config .xcli/config.yaml mcp sync --scope project --project . --yes
```

项目 launcher 默认写入字面量 PATH 命令 `xcli`。`--launcher` 可以指定其他 PATH 命令名，但拒绝绝对路径和路径分隔符。项目 stdio 条目只保存该命令名、`mcp serve --project-config <相对路径> <server>` 和环境变量名称，不写入当前机器的 launcher/config 绝对路径或变量值。每位项目成员都需将 launcher 安装到 PATH，并保持源配置的项目内相对位置不变。

项目原生文件分别为 `.mcp.json`（Claude）、`.codex/config.toml`（Codex）、`.gemini/settings.json`（Gemini）和 `opencode.json`（OpenCode）。JSONC/TOML 注释及无关设置会保留；新文件权限为 `0644`，现有文件保留原权限，原子写入不会在仓库留下备份。Codex 只在受信任项目中加载项目配置；Claude 项目审批和各家 OAuth 流程仍由原生客户端负责。

可共享的源配置示例见 [`examples/project-mcp.yaml`](examples/project-mcp.yaml)。

`sync` 始终先展示排序后的新增、更新和删除计划，再请求确认；自动化必须传入 `--yes`。xcli 只管理记录在 `$XDG_DATA_HOME/xcli/mcp-sync.json` 的对应 scope/project namespace 中的自有条目，v1 用户级 ownership 会自动迁移。同名原生条目、其他 xcli 来源/scope/project 的所有权以及外部修改都会成为冲突；使用 `--force` 前应先检查。删除 xcli 中的 server 或 target 时，只会计划删除仍未漂移的托管条目。

### 导入原生 MCP 配置

使用 import 工作流可将已有公共 MCP 定义拉入 xcli，且不会修改原生文件：

```bash
xcli mcp import plan
xcli mcp import apply

xcli --config .xcli/config.yaml mcp import plan --scope project --project .
xcli --config .xcli/config.yaml mcp import apply --scope project --project . --yes
```

import 直接读取原生配置文件，不要求对应 CLI 已安装。未传 `--target` 时扫描当前 scope 下实际存在的文件；显式选择但文件不存在的 target 视为空。多个 Agent 中同名且等价的定义会合并为一个带显式 targets 的 server；不同原生定义会冲突。`--force` 可以覆盖不同的 xcli 定义、接受原生漂移或接管 ownership。导入仅增量合并，不会因为原生端缺失而删除 xcli server。

只有能够无损映射到 xcli stdio 或 Streamable HTTP 结构的定义才会导入。静态环境变量值、headers、OAuth、SSE/WebSocket、禁用状态、timeout/tool policy、变量展开命令及其他厂商字段会显示为 `unsupported` 并跳过，且不会暴露其中的值。项目相对 cwd 会改写为相对 xcli 源配置的路径；项目绝对路径和语义不稳定的用户级相对 cwd 会跳过。

`import apply` 会保留 YAML 注释、顺序、无关设置和文件权限。user 源配置不存在时会创建最小、私有（`0600`）的版本 1 配置。应用后条目会登记 ownership，因此 HTTP 条目可立即成为 sync noop，直接 stdio 条目则会在下一次 `mcp sync` 中转换为 xcli launcher。已有 xcli launcher wrapper 会被识别，不会递归导入。可导入的原生示例见 [`examples/native-mcp-claude.json`](examples/native-mcp-claude.json)。

### 提示词路由

当 `xcli run` 没有显式指定 Agent 时，可按声明顺序使用 Go 正则表达式匹配完整提示词并选择 Agent：

```yaml
default_agent: codex

routing:
  rules:
    - name: review
      prompt_regex: '(?i)(review|audit|审查)'
      agent: claude
```

首个命中的规则生效。没有规则命中时使用 `default_agent`；两者都无法选出 Agent 时，命令会在启动进程前失败。大小写敏感性由正则表达式决定，需要忽略大小写时可使用 `(?i)`。

选择优先级依次为 `--agent`、首个位置参数中的已配置 Agent、首个命中的路由规则、`default_agent`。显式选择不会计算路由规则。使用 `xcli route <prompt>` 或 `xcli route --json <prompt>` 可以预览路由决定，且不会启动 Agent 或创建运行记录。路由仅用于 `run`；交互会话和工作流步骤继续使用显式 Agent 语义。

### 会话恢复

`xcli resume <target> [prompt...]` 通过 Claude 的 `--resume`、Codex 的 `resume`/`exec resume`、Gemini 的 `--resume` 或 OpenCode 的 `--session` 继续原生会话。没有 prompt 时，xcli 将终端直接连接到恢复后的交互会话；提供 prompt 时，则复用 `run` 的结构化输出、归一化结果、usage 采集和 `--json` 行为：

```bash
# 从 xcli 记录恢复 Agent、session ID 和工作目录
xcli resume run-20260705T010203Z-ab12cd34
xcli resume run-20260705T010203Z-ab12cd34 "实现剩余测试" --json

# workflow 记录必须显式指定步骤
xcli resume workflow-20260705T020304Z-ef56ab78 --step review "处理审查意见"

# xcli 之外创建的会话必须显式指定已配置 Agent
xcli resume 7f9f9a2e-1b3c-4c7a-9b0e-000000000000 --agent codex
```

xcli 始终先查找同名运行记录。未找到时，只有提供 `--agent` 才会将 target 视为原生 session ID，从而避免拼错 run ID 后静默使用 `default_agent`。记录会提供原 Agent 和 `cwd`；相同的 `--agent` 可接受，冲突值会报错，目录移动或失效时可用 `--cwd` 覆盖。resume 不使用提示词路由或 `default_agent`，`--json` 必须同时提供后续 prompt，首版不支持 generic Agent。

交互恢复创建 `use` 记录，继续排除在 `xcli usage` 之外。非交互恢复创建普通 `run` task，并写入 `selection_source: resume`、`resumed_from` 和可选的 `resumed_step`。如果原生客户端报告了新 session ID，新记录会采用它；否则保留请求中的 ID。

### 后台任务

为非交互 `run` 添加 `--detach` 即可在独立进程会话中启动任务并立即返回。Agent 选择、提示词路由、cwd、环境配置、结构化解析、usage 采集和原生参数均与前台运行一致：

```bash
xcli run --detach "运行耗时的集成测试"
xcli run codex --detach --json "审查仓库" -- --sandbox workspace-write

xcli jobs list
xcli jobs show <run-id>
xcli jobs logs <run-id>
xcli jobs logs <run-id> --follow
xcli jobs stop <run-id>
```

`run --detach --json` 返回初始 Job 对象，而不是最终 `RunResult`。Job JSON 包含 ID、Agent、状态、worker PID、cwd、时间和私有日志路径，并在可用时增加退出码、session ID 与 usage。`jobs list` 只列出后台记录；`jobs show` 始终以 JSON 返回单个 Job。

每个任务都会保存权限为 `0600` 的私有归一化日志。原生 stderr 实时追加；普通 generic stdout 实时追加；结构化 Agent 只在完成后追加归一化最终文本，因此 `jobs logs` 不会泄漏 JSONL 事件。原始结构化 stdout 仍然只有在启用 `recording.output` 时保存。日志可能包含源代码、诊断或密钥，首版不提供自动保留策略。

`jobs stop` 先向完整任务进程组发送 TERM，默认等待五秒后升级为 KILL。可用 `--timeout` 修改宽限期，或用 `--force` 立即强杀。对已结束任务重复停止是安全的。xcli 在发送信号前会验证任务持有的锁；没有活动锁的非终态记录会变为 `orphaned`，而不会冒险向已经复用的无关 PID 发送信号。

后台任务可以在启动终端退出后继续，但不能跨机器重启恢复。首版不包含 daemon、任务重启/attach/删除、后台 workflow、后台 resume 或交互后台模式。

### 网络配置

子进程先继承当前环境，然后 xcli 应用选定的网络配置及 Agent 专属环境变量。直连配置可以同时清除大小写形式的代理变量：

```yaml
networks:
  direct:
    unset: [HTTP_PROXY, HTTPS_PROXY, ALL_PROXY, http_proxy, https_proxy, all_proxy]
```

xcli 不保存 API Key。认证仍由各原生 CLI 管理，xcli 也不会打印环境变量值。

## 工作流

为保持向后兼容，工作流默认一次只运行一个步骤。设置 `max_parallel` 后，互不依赖的步骤可以并行运行；`--max-parallel` 可在单次调用中覆盖文件配置。显式 `depends_on` 和步骤结果模板引用都会让步骤等待前置步骤成功：

```yaml
version: 1
name: parallel-review
cwd: .
max_parallel: 2
steps:
  - id: correctness
    agent: codex
    prompt: 审查当前变更的正确性。

  - id: security
    agent: claude
    prompt: 审查当前变更的安全问题。

  - id: summarize
    agent: codex
    depends_on: [correctness, security]
    prompt: |
      汇总以下审查结果：
      {{ steps.correctness.output }}
      {{ steps.security.output }}
```

依赖和模板引用仍必须指向更早声明的步骤；即使完成顺序不同，执行摘要也始终保持声明顺序。完整的 fan-out/fan-in 工作流参见 [`examples/parallel-review.yaml`](examples/parallel-review.yaml)。

支持以下引用：

- `{{ vars.name }}`
- `{{ steps.id.output }}`
- `{{ steps.id.output_file }}`
- `{{ steps.id.session_id }}`

内联输出上限为 128 KiB，更大的结果必须通过 `output_file` 传递。步骤默认超时 30 分钟、不重试，并在失败时立即停止。致命失败或超时会取消正在运行的同级步骤并跳过待运行步骤。只有独立分支仍需继续时，才应使用 `continue_on_error: true`；依赖步骤仍会跳过，工作流整体仍为失败。

并行步骤沿用现有的共享 `cwd` 语义。xcli 不会自动创建隔离 worktree，因此可能写文件的步骤应使用不同的 `cwd`，或自行协调并发访问。并发 stderr 会实时输出，不同步骤的内容可能交错。

## 运行记录、用量与隐私

xcli 将权限为 `0600` 的私有元数据保存在 `$XDG_DATA_HOME/xcli/runs` 或 `~/.local/share/xcli/runs`。元数据包含 Agent、工作目录、时间、状态、退出码、适用时的恢复来源/步骤和后台 PID/日志信息，以及结构化输出能够提供的原生会话 ID 和标准化 Token 用量。

完整输出默认关闭，因为其中可能包含源代码或密钥。可以通过工作流参数 `--record-output` 或配置项 `recording.output: true` 显式启用。

```bash
xcli runs list
xcli runs show <run-id>
```

内置 Agent 的非交互运行默认使用机器可读模式。xcli 捕获原生事件后只向 stdout 输出归一化的最终消息；原生进度和诊断信息仍写入 stderr。generic Agent 保持原有输出行为，且首版不定义 usage 协议。

`xcli usage` 会汇总单次运行和已尝试的工作流步骤；工作流重试累加到对应的逻辑步骤。交互式 `use` 会话不参与统计，旧记录和 generic 任务则显示为未采集，以便覆盖率保持可见：

```bash
xcli usage
xcli usage --days 30 --agent claude
xcli usage --json
```

Codex 和 Gemini 提供 Token 统计但不提供美元估算；Claude 和 OpenCode 可能提供原生 `estimated_cost_usd`，它只是客户端估算而非权威账单。`TRACKED` 和 `COSTED` 列展示覆盖率，缺失费用与原生明确报告的零费用保持可区分。usage 元数据不包含提示词或模型输出。

## 安全边界

- 安装器直接调用 npm 或 Homebrew，不使用 `sudo`、shell 或隐藏的 `curl | sh` 管道。
- 安装命令会在执行前展示并要求确认；自动化环境必须传入 `--yes`。
- 未知 YAML 字段、无效模板、缺失网络配置和前向工作流依赖都会导致校验失败。
- xcli 不添加遥测，也不会自动信任仓库配置。

CAP、自动/持续 MCP 对账、项目自动发现、厂商高级 MCP 字段、守护进程、后台 workflow 与交互会话、任务 restart/attach、Windows ConPTY 和 Web UI 均明确延后到 v0.2 之后。
