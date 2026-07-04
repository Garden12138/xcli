# xcli

[English](README.md) | [简体中文](README.zh-CN.md)

[手工测试指南](docs/manual-testing.zh-CN.md)

`xcli` 是一个统一入口，用于安装、运行、切换和编排主流 AI Coding Agent CLI。

它直接管理真实的 CLI 进程，而不是用模型 API 替代这些工具。原生认证、权限、插件、MCP 配置及 Agent 专属参数均可继续使用。

## 支持的 Agent

| Agent | 交互模式 | 非交互模式 | 工作流结构化输出 | npm | Homebrew |
| --- | --- | --- | --- | --- | --- |
| [Claude Code](https://code.claude.com/docs/en/cli-usage) | `claude` | `claude -p` | 流式 JSON | `@anthropic-ai/claude-code` | — |
| [Codex CLI](https://developers.openai.com/codex/cli) | `codex` | `codex exec` | JSONL | `@openai/codex` | `--cask codex` |
| [Gemini CLI](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/headless.md) | `gemini` | `gemini -p` | 流式 JSON | `@google/gemini-cli` | `gemini-cli` |
| [OpenCode](https://opencode.ai/docs/cli/) | `opencode` | `opencode run` | JSON 事件 | `opencode-ai` | `anomalyco/tap/opencode` |

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

# 执行单次任务；Agent 专属参数放在 -- 之后
xcli run codex "审查当前变更"
xcli run "修复失败的测试" -- --sandbox workspace-write
xcli route "审查认证相关改动"

# 运行工作流（并行需显式启用）
xcli workflow validate examples/implement-and-review.yaml
xcli workflow run examples/implement-and-review.yaml \
  --var requirement="实现缓存失效修复"
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
```

即使提示词包含空格、分号或 shell 元字符，它仍会作为单个进程参数传递。支持的输出模式为 `text`、`json` 和 `jsonl`。

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

## 运行记录与隐私

xcli 将权限为 `0600` 的私有元数据保存在 `$XDG_DATA_HOME/xcli/runs` 或 `~/.local/share/xcli/runs`。元数据包含 Agent、工作目录、时间、状态、退出码，以及结构化输出能够提供的原生会话 ID。

完整输出默认关闭，因为其中可能包含源代码或密钥。可以通过工作流参数 `--record-output` 或配置项 `recording.output: true` 显式启用。

```bash
xcli runs list
xcli runs show <run-id>
```

## 安全边界

- 安装器直接调用 npm 或 Homebrew，不使用 `sudo`、shell 或隐藏的 `curl | sh` 管道。
- 安装命令会在执行前展示并要求确认；自动化环境必须传入 `--yes`。
- 未知 YAML 字段、无效模板、缺失网络配置和前向工作流依赖都会导致校验失败。
- xcli 不添加遥测，也不会自动信任仓库配置。

费用聚合、ACP/CAP、MCP 同步、守护进程、进程控制、会话恢复、Windows ConPTY 和 Web UI 均明确延后到 v0.2 之后。
