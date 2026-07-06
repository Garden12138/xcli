# xcli 手工测试与体验指南

本文用于在 macOS 或 Linux 上手工验证 xcli 的当前实现，基线为 `v0.4.0`（从源码构建时显示 `0.4.0-dev`）。完整流程分为两部分：

1. 使用本地假 Agent 完成零费用、无外部依赖的功能测试。
2. 使用真实 Agent CLI 验证安装、认证、交互与结构化输出。

建议先完成假 Agent 测试，再按需调用真实 Agent。假 Agent 主流程约需 20～30 分钟；包含后台进程、MCP 和全部真实 Agent 的完整验证通常需要 45～90 分钟。

## 1. 当前实现与测试范围

| 功能域 | 当前已实现 | 主要命令 | 假 Agent | 真实 Agent |
| --- | --- | --- | --- | --- |
| 配置 | 默认配置初始化、显式配置路径、严格 YAML 字段/语义校验、`0600` 写入 | `config init/path/validate` | ✓ | — |
| CLI 基础 | 版本/帮助，以及 Bash、Zsh、Fish、PowerShell 补全脚本生成 | `--version`、`--help`、`completion` | ✓ | — |
| Agent 管理 | Claude、Codex、Gemini、OpenCode 内置适配器，自定义 generic Agent，安装检测、版本、默认项 | `agents`、`doctor`、`default`、`install` | ✓ / dry-run | ✓ |
| 运行入口 | 交互运行、非交互运行、工作目录、环境/网络合并、原生 argv 透传 | `use`、`run` | ✓ | ✓ |
| 选择与路由 | `--agent`、位置 Agent、正则路由、默认 Agent 的确定性优先级和预览 | `route`、`run` | ✓ | ✓ |
| 结构化结果 | 四个内置 Adapter 的事件解析、统一文本/JSON、session ID 和 usage 提取 | `run --json` | ✓ | ✓ |
| 会话恢复 | 从 run/workflow 记录或显式原生 session ID 恢复，支持交互与追加 prompt | `resume` | ✓ | ✓ |
| 后台任务 | 脱离终端运行、日志、等待、停止/强杀、删除、按时间清理和孤儿状态识别 | `run/workflow --detach`、`jobs` | ✓ | ✓ |
| 工作流 | 变量模板、步骤输出传递、依赖 DAG、并行上限、重试、超时、继续/跳过和取消 | `workflow validate/run` | ✓ | ✓ |
| ACP | stdio 透明代理；Gemini/OpenCode 原生，Claude/Codex 使用外部桥接器 | `acp` | ✓ | ✓ |
| MCP | stdio launcher、用户/项目级 plan/sync、ownership/漂移/冲突保护、原生配置安全导入 | `mcp serve/plan/sync/import` | serve / import | ✓ |
| 记录与统计 | 私有运行元数据、可选原始输出、按 Agent/时间汇总 Token 与原生费用估算 | `runs`、`usage` | ✓ | ✓ |

当前边界也需要一并确认：xcli 启动和编排的是原生 Agent CLI，不直接调用模型 API，不保存原生认证凭据；ACP 消息不会被解析或记录；机器重启后后台 Job 不会自动恢复，只会被识别为 `orphaned`；MCP 首版不处理 SSE、静态 header、工具策略或厂商 OAuth 字段，HTTP OAuth 仍由各原生客户端完成。

推荐按以下顺序自测：

1. 第 2～7 节：构建、隔离环境、配置、检测、路由和单次运行。
2. 第 8～11 节：交互、恢复、后台任务、协议入口、工作流、记录与用量。
3. 第 12 节：只在需要时测试真实 Agent 与会修改原生配置的 MCP 同步。
4. 第 13 节：逐项勾选验收结果；失败时保留命令、退出码和脱敏后的记录。

除非小节明确说明，后续命令都应在仓库根目录、同一个终端会话中依次执行；若更换终端，需要先重新导出第 2、3 节的变量。文档中的“预期失败”也是通过条件，建议不要对整个会话启用 `set -e`；执行后应检查错误文本和退出码，而不是继续把该错误当作环境故障。

## 2. 准备 xcli

确认当前使用的是待测试构建。开发中的功能应从当前源码构建：

```bash
go test ./...
go build -o /tmp/xcli .
export XCLI_BIN=/tmp/xcli
"$XCLI_BIN" --version
"$XCLI_BIN" --help
"$XCLI_BIN" completion zsh >/dev/null
```

预期结果：

- 全部 Go 测试通过。
- 版本输出为 `xcli version 0.4.0-dev`；发布构建则可能为对应的正式版本。
- 帮助中包含 `run`、`resume`、`jobs`、`workflow`、`acp`、`mcp`、`runs` 和 `usage` 等命令，Zsh 补全脚本可成功生成。

## 3. 创建隔离测试环境

以下变量只在当前终端会话生效。测试配置、运行记录和输出均写入临时目录，不会读取或覆盖真实的 `~/.config/xcli` 与 `~/.local/share/xcli`。

```bash
export XCLI_TEST_ROOT="$(mktemp -d)"
export XDG_CONFIG_HOME="$XCLI_TEST_ROOT/xdg-config"
export XDG_DATA_HOME="$XCLI_TEST_ROOT/xdg-data"
export XCLI_CONFIG="$XCLI_TEST_ROOT/config.yaml"
export CODEX_HOME="$XCLI_TEST_ROOT/codex-home"
export XCLI_ORIGINAL_PATH="$PATH"
export PATH="$XCLI_TEST_ROOT/bin:$PATH"

mkdir -p "$XCLI_TEST_ROOT/bin" "$XCLI_TEST_ROOT/work" "$CODEX_HOME"
printf 'test workspace\n' > "$XCLI_TEST_ROOT/work/README.txt"
printf 'XCLI_TEST_ROOT=%s\n' "$XCLI_TEST_ROOT"
```

`XCLI_CONFIG` 只是本文为减少重复输入而定义的 shell 变量，xcli 不会隐式读取它；后续自定义配置命令都会明确传入 `--config "$XCLI_CONFIG"`。`XDG_CONFIG_HOME` 和 `XDG_DATA_HOME` 会隔离 xcli 配置、运行记录和部分原生客户端配置，`CODEX_HOME` 会额外隔离 Codex 配置、认证和会话。Claude、Gemini 与 OpenCode 仍可能在 `$HOME` 下使用自己的原生目录；第 12 节会单独标出这些风险。

### 3.1 测试默认配置初始化

```bash
"$XCLI_BIN" config path
"$XCLI_BIN" config init
"$XCLI_BIN" config validate
ls -l "$XDG_CONFIG_HOME/xcli/config.yaml"
```

预期结果：

- 配置路径位于临时目录的 `xdg-config/xcli/config.yaml`。
- `config init` 输出 `Created ...`。
- `config validate` 输出 `Config is valid: ...`。
- 配置文件权限为仅当前用户可读写，即 `-rw-------`。

## 4. 创建零费用假 Agent

下面的脚本支持版本检测、认证、交互、参数回显、失败和休眠，可覆盖大部分进程管理逻辑。

```bash
cat > "$XCLI_TEST_ROOT/bin/fake-agent" <<'SH'
#!/bin/sh
set -u

case "${1:-}" in
  --version)
    echo "fake-agent 1.0.0"
    ;;

  auth)
    echo "fake authentication succeeded"
    ;;

  interactive)
    shift
    printf 'mode=interactive\n'
    printf 'cwd=%s\n' "$PWD"
    printf 'native_args=%s\n' "$*"
    printf 'input> '
    IFS= read -r input || input=""
    printf 'received=<%s>\n' "$input"
    ;;

  resume)
    shift
    session="${1:-}"
    printf 'mode=resume-interactive\n'
    printf 'session=<%s>\n' "$session"
    printf 'cwd=%s\n' "$PWD"
    printf 'input> '
    IFS= read -r input || input=""
    printf 'received=<%s>\n' "$input"
    ;;

  exec)
    shift
    session="fake-session-$$"
    mode="run"
    if [ "${1:-}" = "resume" ]; then
      mode="resume-run"
      shift
    fi
    if [ "${1:-}" = "--json" ]; then
      shift
    fi
    if [ "$mode" = "resume-run" ]; then
      session="${1:-}"
      shift || true
    elif [ "${1:-}" = "--" ]; then
      shift
    fi
    prompt="${1:-}"
    printf '{"type":"thread.started","thread_id":"%s"}\n' "$session"
    printf '{"type":"item.completed","item":{"type":"agent_message","text":"%s: %s"}}\n' "$mode" "$prompt"
    printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":12,"cached_input_tokens":2,"output_tokens":4,"reasoning_output_tokens":1,"total_tokens":16}}'
    ;;

  acp)
    shift
    printf 'acp_cwd=%s agent_env=%s network_env=%s native_args=%s\n' \
      "$PWD" "${XCLI_AGENT_ENV:-<unset>}" "${XCLI_NETWORK_ENV:-<unset>}" "$*" >&2
    cat
    ;;

  mcp-server)
    shift
    printf 'mcp_cwd=%s static=%s token=%s native_args=%s\n' \
      "$PWD" "${MCP_STATIC:-<unset>}" "${MCP_TOKEN:-<unset>}" "$*" >&2
    cat
    ;;

  run)
    shift
    prompt="${1:-}"
    shift || true

    if [ "${1:-}" = "--fail" ]; then
      echo "requested failure" >&2
      exit 7
    fi

    if [ "${1:-}" = "--sleep" ]; then
      seconds="${2:-2}"
      shift 2
      sleep "$seconds"
    fi

    printf 'mode=run\n'
    printf 'prompt=<%s>\n' "$prompt"
    printf 'cwd=%s\n' "$PWD"
    printf 'agent_env=%s\n' "${XCLI_AGENT_ENV:-<unset>}"
    printf 'network_env=%s\n' "${XCLI_NETWORK_ENV:-<unset>}"
    printf 'HTTP_PROXY=%s\n' "${HTTP_PROXY:-<unset>}"

    index=0
    for argument in "$@"; do
      printf 'arg[%s]=<%s>\n' "$index" "$argument"
      index=$((index + 1))
    done
    ;;

  *)
    echo "unknown fake-agent command: ${1:-<empty>}" >&2
    exit 2
    ;;
esac
SH

chmod +x "$XCLI_TEST_ROOT/bin/fake-agent"
"$XCLI_TEST_ROOT/bin/fake-agent" --version
ln -sf "$XCLI_BIN" "$XCLI_TEST_ROOT/bin/xcli-under-test"
command -v xcli-under-test
```

预期输出为 `fake-agent 1.0.0`，并且 `xcli-under-test` 指向本次构建。该临时命令名稍后用于验证项目级 MCP 配置不会写入机器绝对路径。

## 5. 创建隔离配置

```bash
cat > "$XCLI_CONFIG" <<YAML
version: 1
default_agent: fake

agents:
  fake:
    adapter: generic
    command: $XCLI_TEST_ROOT/bin/fake-agent
    network: isolated
    env:
      XCLI_AGENT_ENV: agent-value
    interactive_args: ["interactive"]
    run_args: ["run", "{{ prompt }}"]
    auth_args: ["auth"]
    output: text
    acp:
      command: $XCLI_TEST_ROOT/bin/fake-agent
      args: ["acp"]

  fake-alt:
    adapter: generic
    command: $XCLI_TEST_ROOT/bin/fake-agent
    network: isolated
    run_args: ["run", "{{ prompt }}"]
    output: text

  fake-codex:
    adapter: codex
    command: $XCLI_TEST_ROOT/bin/fake-agent
    network: isolated

networks:
  isolated:
    set:
      XCLI_NETWORK_ENV: network-value
    unset:
      - HTTP_PROXY
      - HTTPS_PROXY
      - ALL_PROXY
      - http_proxy
      - https_proxy
      - all_proxy

routing:
  rules:
    - name: review
      prompt_regex: '(?i)(review|audit|审查)'
      agent: fake-alt

mcp:
  servers:
    fake-tools:
      transport: stdio
      command: $XCLI_TEST_ROOT/bin/fake-agent
      args: ["mcp-server"]
      cwd: work
      env:
        MCP_STATIC: configured
      env_vars: [MCP_TOKEN]
      targets: [codex]

    fake-docs:
      transport: http
      url: https://example.com/mcp
      targets: [codex]

recording:
  output: false
YAML

chmod 600 "$XCLI_CONFIG"
"$XCLI_BIN" --config "$XCLI_CONFIG" config validate
"$XCLI_BIN" --config "$XCLI_CONFIG" config path
```

预期结果：配置校验成功，`config path` 输出 `$XCLI_CONFIG` 的绝对路径。

### 5.1 验证严格 YAML 校验

```bash
cp "$XCLI_CONFIG" "$XCLI_TEST_ROOT/invalid-config.yaml"
printf '\nunknown_field: true\n' >> "$XCLI_TEST_ROOT/invalid-config.yaml"

"$XCLI_BIN" --config "$XCLI_TEST_ROOT/invalid-config.yaml" config validate
echo "exit_code=$?"
```

预期结果：命令失败并报告找不到字段 `unknown_field`。不要将无效配置用于后续步骤。

## 6. Agent 检测和 doctor

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" agents
"$XCLI_BIN" --config "$XCLI_CONFIG" agents --json
"$XCLI_BIN" --config "$XCLI_CONFIG" doctor fake
"$XCLI_BIN" --config "$XCLI_CONFIG" doctor fake --json
```

预期结果：

- `fake`、`fake-alt` 和 `fake-codex` 均显示为已安装。
- `fake` 的路径指向临时脚本。
- 版本为 `fake-agent 1.0.0`。
- JSON 输出可以被标准 JSON 解析器读取。

不带 Agent 的 `doctor` 会同时报告内置 Agent；尚未安装的真实 Agent 显示为 `missing`，这是正常现象。

## 7. 默认 Agent 与单次运行

### 7.1 查看和切换默认 Agent

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" default
"$XCLI_BIN" --config "$XCLI_CONFIG" default fake-alt
"$XCLI_BIN" --config "$XCLI_CONFIG" default
"$XCLI_BIN" --config "$XCLI_CONFIG" run --json "由默认 Agent 执行"
"$XCLI_BIN" --config "$XCLI_CONFIG" default fake
```

预期结果：

- 初始默认值为 `fake`，切换后为 `fake-alt`。
- JSON 结果中的 `agent` 为 `fake-alt`、`status` 为 `success`、`exit_code` 为 `0`。
- 最后一条命令将默认值恢复为 `fake`。

### 7.2 Agent 选择优先级与提示词路由

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" route "请审查当前变更"
"$XCLI_BIN" --config "$XCLI_CONFIG" route --json "执行普通任务"

"$XCLI_BIN" --config "$XCLI_CONFIG" run --json "请审查当前变更"
ROUTED_RUN_ID=$(basename "$(ls -t "$XDG_DATA_HOME/xcli/runs"/run-*.json | head -n 1)" .json)
"$XCLI_BIN" --config "$XCLI_CONFIG" runs show "$ROUTED_RUN_ID"

"$XCLI_BIN" --config "$XCLI_CONFIG" run fake "使用位置参数选择"
"$XCLI_BIN" --config "$XCLI_CONFIG" run --agent fake-alt --json "使用 --agent 选择"
"$XCLI_BIN" --config "$XCLI_CONFIG" run --agent fake --json "请审查但显式使用 fake"
```

预期结果：

- `route "请审查当前变更"` 选择 `fake-alt`，来源为 `rule`，规则名为 `review`；`route --json "执行普通任务"` 选择默认的 `fake`。
- 未显式指定 Agent 的审查任务由 `fake-alt` 执行，对应运行记录包含 `selection_source: rule` 和 `route_rule: review`。
- 位置参数能够选择 `fake`，`--agent` 能够显式选择 `fake-alt`。
- 即使 prompt 命中 `review`，显式 `--agent fake` 仍然优先。

完整选择优先级为：`--agent` > 第一个位置参数中的已配置 Agent > 第一个命中的路由规则 > `default_agent`。`route` 只解释选择结果，不启动 Agent，也不创建运行记录。

### 7.3 工作目录、环境合并和原生参数透传

先在父进程中设置一个应被网络配置清除的代理变量：

```bash
export HTTP_PROXY=http://should-be-removed.invalid

"$XCLI_BIN" --config "$XCLI_CONFIG" run \
  --cwd "$XCLI_TEST_ROOT/work" \
  fake "检查环境与参数" -- --flag "two words" --literal '; echo unsafe'
```

预期输出包含：

```text
cwd=<临时目录>/work
agent_env=agent-value
network_env=network-value
HTTP_PROXY=<unset>
arg[0]=<--flag>
arg[1]=<two words>
arg[2]=<--literal>
arg[3]=<; echo unsafe>
```

这同时证明：

- `--cwd` 生效。
- 环境合并顺序正确，代理变量被清除。
- `--` 后的参数保持 argv 边界。
- shell 元字符只是普通参数，没有被执行。

## 8. 交互模式与认证入口

### 8.1 交互 stdio

```bash
printf 'hello from stdin\n' | "$XCLI_BIN" --config "$XCLI_CONFIG" use \
  --cwd "$XCLI_TEST_ROOT/work" fake -- --native-use-arg
```

预期输出包含：

```text
mode=interactive
native_args=--native-use-arg
received=<hello from stdin>
```

这说明 xcli 将 stdin/stdout/stderr 直接交给原生进程。

### 8.2 会话恢复

先通过内置 Adapter 形状的零费用假 Agent 创建一条带 session ID 的记录：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" run fake-codex "create resumable session"

RESUME_RUN_ID=$(basename "$(ls -t "$XDG_DATA_HOME/xcli/runs"/run-*.json | head -n 1)" .json)
printf 'source_run=%s\n' "$RESUME_RUN_ID"

"$XCLI_BIN" --config "$XCLI_CONFIG" resume \
  "$RESUME_RUN_ID" "continue non-interactively" --json

printf 'continue interactively\n' | "$XCLI_BIN" --config "$XCLI_CONFIG" resume \
  "$RESUME_RUN_ID"
```

预期结果：

- 非交互结果为合法 JSON，包含 `agent: fake-codex`、原 session ID、归一化文本和 usage。
- 新增 `run` 记录包含 `selection_source: resume` 和 `resumed_from: <source_run>`。
- 交互恢复输出 `mode=resume-interactive`，收到 stdin，并新增不计入 usage 的 `use` 记录。
- 两次恢复都继承源记录的工作目录；使用 `--cwd` 可以显式覆盖。

再验证原生 session ID 和 workflow 步骤：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" resume native-session-1 \
  --agent fake-codex "continue external session" --json

cat > "$XCLI_TEST_ROOT/resume-workflow.yaml" <<'YAML'
version: 1
name: resumable-workflow
cwd: work
steps:
  - id: review
    agent: fake-codex
    prompt: create workflow session
YAML

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/resume-workflow.yaml" --json

RESUME_WORKFLOW_ID=$(basename "$(ls -t "$XDG_DATA_HOME/xcli/runs"/workflow-*.json | head -n 1)" .json)
"$XCLI_BIN" --config "$XCLI_CONFIG" resume \
  "$RESUME_WORKFLOW_ID" --step review "continue workflow step" --json
```

最后确认保护性错误：省略原生 ID 的 `--agent`、省略 workflow 的 `--step`、对 `fake` generic Agent 执行 resume，或在无 prompt 时传 `--json`，都应在启动子进程前失败。

### 8.3 后台任务与进程控制

启动一个耗时的 generic 后台任务并提取 run ID：

```bash
JOB_STARTED=$("$XCLI_BIN" --config "$XCLI_CONFIG" run --detach \
  --cwd "$XCLI_TEST_ROOT/work" fake "background success" -- --sleep 2)
printf '%s\n' "$JOB_STARTED"
JOB_ID=$(printf '%s\n' "$JOB_STARTED" | awk '{print $3}')

"$XCLI_BIN" --config "$XCLI_CONFIG" jobs list
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs show "$JOB_ID"
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs logs "$JOB_ID" --follow
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs wait "$JOB_ID" --json
```

预期结果：

- 启动命令立即输出 `Started job <id> (pid <pid>)`，不等待两秒任务完成。
- `jobs list` 固定显示 `ID / KIND / AGENT/WORKFLOW / STATUS / PID / STARTED / ENDED`，不包含前台 run/use/workflow。
- `jobs show` 返回合法 Job JSON，完成后状态为 `success`、退出码为 `0`。
- `jobs logs --follow` 先等待并输出后台日志，在任务完成后自动退出；日志文件权限为 `0600`。
- 对 generic Agent，stderr 与 stdout 都会实时进入日志；任务作为一个普通 run task 计入 `usage`。
- `jobs wait` 输出最终 Job，并以 Job 的退出码结束。

验证优雅停止、强杀和幂等性：

```bash
STOP_STARTED=$("$XCLI_BIN" --config "$XCLI_CONFIG" run --detach \
  fake "background stop" -- --sleep 60)
STOP_ID=$(printf '%s\n' "$STOP_STARTED" | awk '{print $3}')
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs stop "$STOP_ID" --timeout 2s --json
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs stop "$STOP_ID"

KILL_STARTED=$("$XCLI_BIN" --config "$XCLI_CONFIG" run --detach \
  fake "background kill" -- --sleep 60)
KILL_ID=$(printf '%s\n' "$KILL_STARTED" | awk '{print $3}')
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs stop "$KILL_ID" --force --json
```

预期结果：优雅停止变为 `canceled`、退出码 `130`；重复停止不再发送信号且仍成功；强杀变为 `killed`、退出码 `137`。子进程组内不应残留 `sleep` 进程。

内置 Agent 的后台日志只包含实时 stderr 和完成后的归一化最终文本，不应出现 JSONL 事件。只有将 `recording.output` 改为 `true` 后，原始结构化 stdout 才会另外保存为 `output.log`。后台日志可能包含源码或敏感诊断，测试结束后应随隔离目录一同删除。

验证等待超时与显式清理：

```bash
WAIT_STARTED=$("$XCLI_BIN" --config "$XCLI_CONFIG" run --detach \
  fake "wait timeout" -- --sleep 60)
WAIT_ID=$(printf '%s\n' "$WAIT_STARTED" | awk '{print $3}')
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs wait "$WAIT_ID" --timeout 100ms --json
echo "exit_code=$?"
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs stop "$WAIT_ID" --force
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs delete "$WAIT_ID" --yes --json
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs prune --older-than 1h --dry-run --json
```

等待超时应返回 124，但 Job 仍保持运行，随后可显式停止并删除。`prune --dry-run` 只返回候选项；省略 `--dry-run` 的非交互清理必须传 `--yes`。运行中的 Job、前台记录和没有 `ended_at` 的旧记录都不会被批量删除。

### 8.4 ACP stdio 透传

```bash
ACP_MESSAGE='{"jsonrpc":"2.0","id":1,"method":"test"}'

printf '%s' "$ACP_MESSAGE" | "$XCLI_BIN" --config "$XCLI_CONFIG" acp \
  --cwd "$XCLI_TEST_ROOT/work" fake -- --native-acp-arg \
  > "$XCLI_TEST_ROOT/acp.stdout" \
  2> "$XCLI_TEST_ROOT/acp.stderr"

printf '%s' "$ACP_MESSAGE" | cmp - "$XCLI_TEST_ROOT/acp.stdout"
cat "$XCLI_TEST_ROOT/acp.stderr"
```

预期结果：

- `cmp` 不输出差异，证明 stdout 只包含假 ACP 服务返回的原始协议消息。
- stderr 包含 `acp_cwd`、`agent_env=agent-value`、`network_env=network-value` 和 `native_args=--native-acp-arg`。
- `--cwd`、环境合并和 `--` 后参数均生效。
- `xcli runs list` 不会因为该 ACP 连接新增记录。

将 `fake` 从命令中省略也应得到相同结果，因为它是 `default_agent`。ACP 选择不使用提示词路由。

### 8.5 MCP stdio launcher

```bash
export MCP_TOKEN=runtime-secret
MCP_MESSAGE='{"jsonrpc":"2.0","id":2,"method":"initialize"}'

printf '%s' "$MCP_MESSAGE" | "$XCLI_BIN" --config "$XCLI_CONFIG" \
  mcp serve fake-tools \
  > "$XCLI_TEST_ROOT/mcp.stdout" \
  2> "$XCLI_TEST_ROOT/mcp.stderr"

printf '%s' "$MCP_MESSAGE" | cmp - "$XCLI_TEST_ROOT/mcp.stdout"
cat "$XCLI_TEST_ROOT/mcp.stderr"
```

预期结果：

- `cmp` 不输出差异，协议 stdout 未被 xcli 污染。
- stderr 包含 `mcp_cwd=<临时目录>/work`、`static=configured` 和 `token=runtime-secret`。
- `env_vars` 只保存变量名，敏感值在启动时读取。
- 删除 `MCP_TOKEN` 后重新执行会在启动进程前报缺失变量。
- `xcli runs list` 不会因为 `mcp serve` 新增记录。

`mcp serve` 是同步后的原生 Agent 实际调用入口；日常使用不需要手工运行。它只接受 server 名称，不向真实 MCP 命令追加 CLI 参数。

### 8.6 原生认证入口

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" auth login fake
```

预期输出为 `fake authentication succeeded`。

## 9. 安装器 dry-run

以下命令只展示安装命令，不会实际安装软件：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" install codex --method npm --dry-run
"$XCLI_BIN" --config "$XCLI_CONFIG" install gemini --method brew --dry-run
"$XCLI_BIN" --config "$XCLI_CONFIG" install opencode --method auto --dry-run
```

预期结果：

- npm Codex 命令为 `npm install -g @openai/codex`。
- Homebrew Gemini 命令为 `brew install gemini-cli`。
- macOS 的 `auto` 优先选择可用的 Homebrew，Linux 优先选择 npm。
- 如果指定的包管理器不在 `PATH` 中，xcli 会明确报错；这不代表 xcli 安装逻辑失败。

不要在体验 dry-run 时添加 `--yes` 或删除 `--dry-run`，除非确实希望修改本机安装。

## 10. 依赖工作流与并行调度

### 10.1 创建成功工作流

```bash
cat > "$XCLI_TEST_ROOT/success-workflow.yaml" <<'YAML'
version: 1
name: fake-success
cwd: work

vars:
  message: default message

steps:
  - id: first
    agent: fake
    prompt: "{{ vars.message }}"
    timeout: 10s

  - id: second
    agent: fake-alt
    depends_on: [first]
    prompt: |
      前一步输出：
      {{ steps.first.output }}
      前一步输出文件：{{ steps.first.output_file }}
YAML

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow validate \
  "$XCLI_TEST_ROOT/success-workflow.yaml"

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/success-workflow.yaml" \
  --var message="manual workflow test"
```

预期结果：

- 校验输出 `Workflow is valid: ...`。
- 进度依次显示 `first`、`second`，不会并行执行。
- 两个步骤均为 `success`。
- 第二步收到第一步的显式文本输出和 `output_file` 路径。

未设置 `max_parallel` 时，并发度默认为 1，因此旧工作流仍保持串行行为。

### 10.2 并行 fan-out/fan-in

```bash
cat > "$XCLI_TEST_ROOT/parallel-workflow.yaml" <<'YAML'
version: 1
name: fake-parallel
cwd: work
max_parallel: 2

steps:
  - id: first
    agent: fake
    prompt: first parallel branch
    args: ["--sleep", "1"]

  - id: second
    agent: fake-alt
    prompt: second parallel branch
    args: ["--sleep", "1"]

  - id: summarize
    agent: fake
    depends_on: [first, second]
    prompt: |
      first={{ steps.first.output }}
      second={{ steps.second.output }}
YAML

time "$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/parallel-workflow.yaml" --json

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/parallel-workflow.yaml" --max-parallel 1 --json
```

预期结果：

- 第一次执行的 `max_parallel` 为 `2`，`first` 和 `second` 的时间范围重叠，总耗时接近一次休眠而不是两次。
- `summarize` 只在两个分支成功后运行，并收到两份输出。
- JSON 中的步骤始终按 `first`、`second`、`summarize` 的声明顺序排列。
- 第二次执行的 `max_parallel` 为 `1`，CLI 成功覆盖 YAML 并恢复串行执行。

并行步骤共享工作目录。不要让两个会写文件的 Agent 并发修改同一目录；应通过 `cwd` 分离工作区或自行协调。

### 10.3 JSON 汇总和输出记录

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/success-workflow.yaml" \
  --var message="record this output" \
  --record-output \
  --json
```

预期结果：标准输出只有一个 JSON 执行摘要，包含：

- 工作流 `status: success`、`exit_code: 0`。
- 实际并发度 `max_parallel: 1`。
- 两个步骤的状态、输出、尝试次数和时间。
- 持久化的 `output_file` 路径。

### 10.4 重试、继续和依赖跳过

```bash
cat > "$XCLI_TEST_ROOT/failure-workflow.yaml" <<'YAML'
version: 1
name: fake-failure
cwd: work
max_parallel: 2

steps:
  - id: failing
    agent: fake
    prompt: fail deliberately
    args: ["--fail"]
    retries: 1
    continue_on_error: true

  - id: independent
    agent: fake
    prompt: still runs

  - id: dependent
    agent: fake
    depends_on: [failing]
    prompt: must be skipped
YAML

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/failure-workflow.yaml" --json
echo "exit_code=$?"
```

预期结果：

- `failing` 尝试 2 次后失败。
- `independent` 仍然成功。
- `dependent` 因依赖失败而标记为 `skipped`。
- 整体工作流失败并返回非零退出码。

### 10.5 超时与同级取消

```bash
cat > "$XCLI_TEST_ROOT/timeout-workflow.yaml" <<'YAML'
version: 1
name: fake-timeout
cwd: work
max_parallel: 2

steps:
  - id: slow
    agent: fake
    prompt: timeout test
    args: ["--sleep", "2"]
    timeout: 200ms

  - id: sibling
    agent: fake-alt
    prompt: cancel with sibling
    args: ["--sleep", "2"]

  - id: later
    agent: fake
    prompt: must not start
YAML

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/timeout-workflow.yaml" --json
echo "exit_code=$?"
```

预期结果：`slow` 和工作流状态为 `timed_out`，`sibling` 为 `canceled`，`later` 为 `skipped`，命令返回退出码 `124`。

### 10.6 Ctrl+C

创建并行休眠工作流，运行后按一次 Ctrl+C：

```bash
cat > "$XCLI_TEST_ROOT/cancel-workflow.yaml" <<'YAML'
version: 1
name: fake-cancel
cwd: work
max_parallel: 2
steps:
  - id: first
    agent: fake
    prompt: first cancel test
    args: ["--sleep", "60"]
  - id: second
    agent: fake-alt
    prompt: second cancel test
    args: ["--sleep", "60"]
  - id: later
    agent: fake
    prompt: must not start
YAML

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/cancel-workflow.yaml"
echo "exit_code=$?"
```

预期结果：两个运行中的步骤均被取消，`later` 被跳过，工作流返回退出码 `130`，不会遗留休眠进程。

### 10.7 后台工作流与输入隐私

使用前面的依赖工作流启动后台 Job，并通过变量传入只用于测试的标记值：

```bash
WORKFLOW_JOB=$("$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/success-workflow.yaml" \
  --detach --json --var message=WORKFLOW_PRIVATE_MARKER)
printf '%s\n' "$WORKFLOW_JOB"
WORKFLOW_JOB_ID=$(printf '%s\n' "$WORKFLOW_JOB" | sed -n 's/.*"id": "\([^"]*\)".*/\1/p')

"$XCLI_BIN" --config "$XCLI_CONFIG" jobs show "$WORKFLOW_JOB_ID"
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs logs "$WORKFLOW_JOB_ID" --follow
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs wait "$WORKFLOW_JOB_ID" --json
grep -R "WORKFLOW_PRIVATE_MARKER" "$XDG_DATA_HOME/xcli/runs" && echo LEAKED || true
```

启动应立即返回；`jobs show` 在执行期间展示 `pending/running`，完成后展示每步终态、实际并行度与 usage。默认日志只包含进度、stderr 和最终摘要。最后的 grep 不应找到标记值，证明变量和 prompt 没有写入运行元数据；也可用 `ps` 确认 worker argv 只有配置路径、内部命令和 Job ID。

## 11. 运行记录与隐私

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" runs list
"$XCLI_BIN" --config "$XCLI_CONFIG" runs list --json
ls -la "$XDG_DATA_HOME/xcli/runs"
```

从列表复制任一 `run-*`、`use-*` 或 `workflow-*` ID，然后查看详情：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" runs show <run-id>
```

预期结果：

- 记录包含类型、Agent 或工作流、实际并发度、工作目录、时间、状态和退出码。
- JSON 元数据文件权限为 `-rw-------`，目录权限仅允许当前用户访问。
- 普通运行不会保存完整输出。
- 使用 `--record-output` 的工作流会在对应运行目录中保存步骤输出。
- 记录中不应出现父进程环境变量值或 API Key。

### 11.1 用量覆盖率

假 Agent 使用 generic Adapter，因此会计入任务数但不提供 usage：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" usage
"$XCLI_BIN" --config "$XCLI_CONFIG" usage --agent fake --json
```

预期结果：

- 文本表包含固定的 Token、费用和覆盖率列以及 `TOTAL` 行。
- `fake` 的任务数大于零，`tracked_tasks` 和 `costed_tasks` 为零。
- `use` 记录不会计入任务数；未尝试的工作流步骤也不会计入。

## 12. 使用真实 Agent 体验

真实运行可能消耗 API 配额，并可能按提示修改工作目录。请使用临时目录和低风险提示词。第 12.6～12.8 节主要验证 MCP 配置文件，不会调用模型，但会写入原生客户端配置或项目文件；执行 `sync` 前必须阅读完整计划。

本指南已经把 Codex 的配置、认证和会话隔离到 `$CODEX_HOME`。因此若要运行 Codex，需要在第 12.1 节的隔离环境中重新认证。Claude、Gemini 和 OpenCode 是否支持独立配置根目录取决于原生 CLI；测试它们之前应自行备份相应配置，且不要在不清楚影响范围时执行安装、认证或 MCP 同步。

### 12.1 检测、安装和认证

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" doctor codex
"$XCLI_BIN" --config "$XCLI_CONFIG" install codex --dry-run
"$XCLI_BIN" --config "$XCLI_CONFIG" auth login codex
```

将 `codex` 替换为 `claude`、`gemini` 或 `opencode` 即可测试其他 Agent。

- `doctor <agent>` 在缺失时返回非零退出码。
- `install --dry-run` 只展示命令。
- 去掉 `--dry-run` 后，xcli 会展示命令并要求确认。
- `auth login` 完全进入原生 CLI 的认证流程，xcli 不保存凭据。

### 12.2 单次真实调用

```bash
mkdir -p "$XCLI_TEST_ROOT/real-work"
printf '# Manual test\n' > "$XCLI_TEST_ROOT/real-work/README.md"

"$XCLI_BIN" --config "$XCLI_CONFIG" run \
  --cwd "$XCLI_TEST_ROOT/real-work" \
  --json codex \
  "只回复 XCLI_OK，不要修改任何文件"
```

预期结果：

- xcli 使用 Codex 的结构化协议运行。
- 最终输出被归一化为包含 `agent`、`output`、`session_id`（原生协议提供时）、`exit_code`、`status` 和可选 `usage` 的 JSON。
- 原生 CLI 的诊断信息仍可出现在 stderr。

可以依次替换为其他已安装 Agent：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" run --cwd "$XCLI_TEST_ROOT/real-work" --json claude "只回复 XCLI_OK"
"$XCLI_BIN" --config "$XCLI_CONFIG" run --cwd "$XCLI_TEST_ROOT/real-work" --json gemini "只回复 XCLI_OK"
"$XCLI_BIN" --config "$XCLI_CONFIG" run --cwd "$XCLI_TEST_ROOT/real-work" --json opencode "只回复 XCLI_OK"
```

随后检查聚合结果：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" usage --days 1
"$XCLI_BIN" --config "$XCLI_CONFIG" usage --days 1 --agent codex --json
```

预期结果：内置 Agent 运行显示为已采集；Codex 和 Gemini 只有 Token，Claude 和 OpenCode 在原生事件提供时显示估算费用。费用只是客户端估算，不应与账单核对或用于财务决策。

### 12.3 真实会话恢复

从上一节的 `run` 记录复制 ID，然后分别验证非交互和交互恢复：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" runs list

"$XCLI_BIN" --config "$XCLI_CONFIG" resume <run-id> \
  "只回复 RESUME_OK，不要修改任何文件" --json

"$XCLI_BIN" --config "$XCLI_CONFIG" resume <run-id>
```

预期结果：后续 prompt 能看到原会话上下文，JSON 结果包含 session ID 和可用的 usage；交互模式进入原生 TUI。若原工作目录已经删除，命令应在启动前报错，并提示通过 `--cwd` 覆盖。也可从原生 CLI 获取 session ID，再用 `resume <session-id> --agent <agent>` 验证 xcli 外部会话。

四个 Agent 的会话文件和保留策略由原生工具管理；本测试不应移动、复制或编辑这些文件。

### 12.4 真实后台任务

使用只读提示启动一个真实 Agent 后台任务：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" run codex --detach --json \
  --cwd "$XCLI_TEST_ROOT/real-work" \
  "只检查仓库并回复 BACKGROUND_OK，不要修改文件"

"$XCLI_BIN" --config "$XCLI_CONFIG" jobs list
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs logs <run-id> --follow
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs show <run-id>
"$XCLI_BIN" --config "$XCLI_CONFIG" jobs wait <run-id>
```

确认启动命令立即返回，日志中没有 Codex JSONL，完成后的 Job 包含 session ID 和 usage。再启动一个耗时任务并执行 `jobs stop <run-id>`，确认整个原生 Agent 进程组退出且记录为 `canceled`。

关闭启动终端后任务仍应继续；机器重启后不恢复任务，下一次 `jobs list/show` 会将遗留的非终态记录标为 `orphaned`。不要对生产中的重要任务测试 `--force`。

还可将上一节的只读双 Agent workflow 加上 `--detach --json`，确认 workflow Job 能脱离终端完成并持续记录步骤状态。输入通过匿名管道发送，prompt 和 `--var` 值不应出现在 worker 进程参数或 Job JSON 中。

### 12.5 真实 ACP 客户端

Gemini 和 OpenCode 直接使用原生 ACP 模式。Codex 与 Claude 需要先显式安装桥接器：

```bash
npm install -g @agentclientprotocol/codex-acp
npm install -g @agentclientprotocol/claude-agent-acp
command -v codex-acp claude-agent-acp
```

在 ACP 客户端中将启动命令配置为 xcli，例如 Codex 的命令和参数分别为：

```json
{
  "command": "/tmp/xcli",
  "args": ["--config", "/absolute/path/to/config.yaml", "acp", "codex"]
}
```

实际路径应替换为 `$XCLI_BIN` 和 `$XCLI_CONFIG` 的绝对路径。连接后新建会话，发送一个只读提示词并确认：

- 客户端能够完成 ACP 初始化和会话创建。
- 文本、工具调用、权限请求等能力由下游 ACP 服务报告并处理。
- xcli 自身不会在协议 stdout 中插入日志。
- 断开连接后没有新增 xcli 运行记录或 usage 任务。

不要在普通终端中裸运行 `xcli acp` 后等待人类可读提示；该命令专门等待 ACP 客户端发送协议消息。

### 12.6 用户级 MCP 同步

以下示例只同步到 Codex，并使用无鉴权 HTTP server 验证计划和幂等性。由于第 3 节设置了临时 `CODEX_HOME`，它只会修改 `$CODEX_HOME/config.toml`；如果当前终端没有该变量，请停止并重新完成第 3 节，避免修改日常 Codex 配置。

```bash
test -n "$CODEX_HOME"
case "$CODEX_HOME" in
  "$XCLI_TEST_ROOT"/*) ;;
  *) echo "CODEX_HOME is not isolated" >&2; false ;;
esac

"$XCLI_BIN" --config "$XCLI_CONFIG" mcp plan \
  --target codex --launcher "$XCLI_BIN"

"$XCLI_BIN" --config "$XCLI_CONFIG" mcp sync \
  --target codex --launcher "$XCLI_BIN"

"$XCLI_BIN" --config "$XCLI_CONFIG" mcp plan \
  --target codex --launcher "$XCLI_BIN" --json
```

预期结果：

- 首次计划包含 `add`；同步前完整展示变更并要求确认。
- 再次计划中的托管条目为 `noop`。
- `$XDG_DATA_HOME/xcli/mcp-sync.json` 权限为 `0600`，且不包含 `MCP_TOKEN` 的值。
- 同名原生条目不会被覆盖，而是显示 `conflict`；只有检查后显式使用 `--force` 才能接管。
- 未传 `--target` 时只选择当前已安装的规范 Agent。

清理时从 `$XCLI_CONFIG` 的 `mcp.servers` 删除测试 server，再运行同一条 `mcp sync` 并确认 `remove` 计划。不要直接删除原生条目，否则下一次计划会按外部漂移报告冲突。

HTTP OAuth 仍需分别使用各 Agent 的原生 MCP 登录流程；xcli 不复制认证状态。stdio 同步写入 `$XCLI_BIN` 与 `$XCLI_CONFIG` 的绝对路径，移动文件后需重新同步。

### 12.7 项目级 MCP 同步

在一个临时 Git 项目内创建 `.xcli/config.yaml`（可从 `examples/project-mcp.yaml` 复制并换成可用的测试 server）。第 4 节创建的 `xcli-under-test` 已在临时 PATH 中，可用来验证可移植 launcher：

```bash
PROJECT="$XCLI_TEST_ROOT/mcp-project"
mkdir -p "$PROJECT/.xcli"
cp examples/project-mcp.yaml "$PROJECT/.xcli/config.yaml"

"$XCLI_BIN" --config "$PROJECT/.xcli/config.yaml" mcp plan \
  --scope project --project "$PROJECT" --target codex \
  --launcher xcli-under-test --json

"$XCLI_BIN" --config "$PROJECT/.xcli/config.yaml" mcp sync \
  --scope project --project "$PROJECT" --target codex \
  --launcher xcli-under-test

"$XCLI_BIN" --config "$PROJECT/.xcli/config.yaml" mcp plan \
  --scope project --project "$PROJECT" --target codex \
  --launcher xcli-under-test --json
```

预期结果：

- JSON 固定包含 `scope: "project"` 与规范化的 `project_dir`。
- `.codex/config.toml` 中 stdio command 是 `xcli-under-test`，参数包含 `mcp serve --project-config .xcli/config.yaml`，不包含 `$XCLI_BIN`、项目或源配置的绝对路径。
- 当前只选择 Codex，因此不会改动其他客户端文件；分别选择已安装的 Claude/Gemini/OpenCode 时，对应写入 `.mcp.json`、`.gemini/settings.json`、`opencode.json`，并保留已有注释和无关字段。
- 新文件权限为 `0644`，仓库内没有 `.xcli.bak`；重复 plan 为 `noop`。
- 从项目子目录启动同步后的 stdio server 时，`mcp serve --project-config` 能向父目录找到源配置；指向项目外的符号链接会被拒绝。
- 将同名 server 同步到另一个项目不会接管第一个项目的 ownership；user scope 也保持隔离。

清理前从源配置删除测试 server 并同步，确认只删除未漂移的托管条目。不要提交包含真实静态凭据的 `env`；团队密钥只声明 `env_vars` 名称。

### 12.8 原生 MCP 安全导入

在临时项目中创建独立的 xcli 源配置与 Claude 项目配置，避免修改真实用户配置：

```bash
IMPORT_PROJECT="$XCLI_TEST_ROOT/mcp-import"
mkdir -p "$IMPORT_PROJECT/.xcli"
cat > "$IMPORT_PROJECT/.xcli/config.yaml" <<'YAML'
version: 1
# This comment must survive import.
YAML
cp examples/native-mcp-claude.json "$IMPORT_PROJECT/.mcp.json"

if command -v shasum >/dev/null 2>&1; then
  IMPORT_HASH_BEFORE=$(shasum -a 256 "$IMPORT_PROJECT/.mcp.json" | awk '{print $1}')
else
  IMPORT_HASH_BEFORE=$(sha256sum "$IMPORT_PROJECT/.mcp.json" | awk '{print $1}')
fi

"$XCLI_BIN" --config "$IMPORT_PROJECT/.xcli/config.yaml" mcp import plan \
  --scope project --project "$IMPORT_PROJECT" --target claude --json

"$XCLI_BIN" --config "$IMPORT_PROJECT/.xcli/config.yaml" mcp import apply \
  --scope project --project "$IMPORT_PROJECT" --target claude --yes

if command -v shasum >/dev/null 2>&1; then
  IMPORT_HASH_AFTER=$(shasum -a 256 "$IMPORT_PROJECT/.mcp.json" | awk '{print $1}')
else
  IMPORT_HASH_AFTER=$(sha256sum "$IMPORT_PROJECT/.mcp.json" | awk '{print $1}')
fi
test "$IMPORT_HASH_BEFORE" = "$IMPORT_HASH_AFTER"
printf 'native_config_unchanged=%s\n' "$IMPORT_HASH_AFTER"
```

预期结果：

- plan 使用 `SERVER / TARGETS / ACTION / STATUS / DETAIL`，JSON 包含 scope、project、targets、applicable、applied 和 changes。
- `.mcp.json` 两次校验和相同；import 只修改 `.xcli/config.yaml` 与私有 ownership state。
- xcli YAML 中已有注释和无关字段保留，新条目显式包含 `targets: [claude]`。
- `local-tools.cwd` 从项目根的 `tools` 改写为相对 `.xcli/config.yaml` 的 `../tools`。
- 再次 import 为 `noop`；若本机已安装对应的 Claude CLI，后续 `mcp plan --scope project --project "$IMPORT_PROJECT" --target claude --launcher xcli-under-test` 中，HTTP 条目为 `noop`，直接 stdio 条目计划为 launcher `update`。

随后在 `.mcp.json` 增加一个包含静态环境值、header 或 timeout 的测试条目，确认它显示为 `unsupported/skipped`，值不会出现在文本/JSON 计划、xcli YAML 或 state 中，且不阻止其他安全条目导入。创建两个 Agent 的同名不同 URL，确认即使传 `--force` 也会报告 conflict。

### 12.9 真实交互模式

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" use \
  --cwd "$XCLI_TEST_ROOT/real-work" codex
```

确认终端颜色、键盘输入、Ctrl+C 和原生权限提示与直接运行 `codex` 时一致。退出后使用 `xcli runs list` 检查元数据记录。

### 12.10 双 Agent 串行工作流

仅在 Codex 和 Claude 均已安装并完成认证后执行：

```bash
cat > "$XCLI_TEST_ROOT/real-workflow.yaml" <<'YAML'
version: 1
name: real-read-only-review
cwd: real-work

vars:
  task: 阅读 README.md，给出一句摘要，不要修改文件。

steps:
  - id: inspect
    agent: codex
    prompt: "{{ vars.task }}"
    timeout: 5m

  - id: review
    agent: claude
    depends_on: [inspect]
    prompt: |
      检查下面的摘要是否准确，只回复“准确”或简短修正，不要修改文件：
      {{ steps.inspect.output }}
    timeout: 5m
YAML

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow validate \
  "$XCLI_TEST_ROOT/real-workflow.yaml"

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/real-workflow.yaml" --json
```

预期结果：Codex 完成后 Claude 才启动，第二步能够收到第一步的归一化文本输出。

## 13. 验收清单

完成测试后，可以按下列清单判断当前核心体验是否符合预期：

- [ ] `config init/path/validate` 行为正确，配置权限为 `0600`。
- [ ] 未知 YAML 字段会立即失败。
- [ ] `agents` 和 `doctor` 能检测路径与版本。
- [ ] `route` 能预览规则/默认选择且不创建记录，默认 Agent、位置参数和 `--agent` 优先级正确。
- [ ] `use` 保留终端 stdio，内置 Agent 的普通 `run` 只输出归一化最终文本。
- [ ] `resume` 能恢复普通记录、workflow 步骤和显式 Agent 的原生 session ID，且交互/非交互记录语义正确。
- [ ] `run/workflow --detach` 能跨终端运行，`jobs` 可查看步骤与日志、等待退出码，并安全停止任务。
- [ ] `jobs delete/prune` 只显式清理终态后台 Job，dry-run 与非交互确认保护有效。
- [ ] `acp` 保持协议 stdio 字节不变，复用 cwd/环境配置且不创建运行记录。
- [ ] `mcp serve` 使用最小环境并保持协议 stdio；`mcp plan/sync` 能检测冲突且重复执行为 noop。
- [ ] 项目级 MCP 文件不含机器绝对路径，保留 JSONC/TOML 注释，跨项目 ownership 相互隔离。
- [ ] `mcp import` 不修改原生配置，能保留源 YAML 注释并拒绝有损或冲突条目。
- [ ] `--json` 返回合法、稳定的归一化结构。
- [ ] `--` 后参数不被 xcli 或 shell 重新解释。
- [ ] `--cwd`、网络变量清除和 Agent 环境变量生效。
- [ ] 安装器 dry-run 展示正确命令且不修改系统。
- [ ] 工作流默认串行，配置后能并行运行独立步骤并按依赖汇合。
- [ ] 重试、失败继续、依赖跳过、超时和 Ctrl+C 状态正确。
- [ ] 运行记录权限正确，默认不保存完整输出。
- [ ] `usage` 正确区分已采集、未采集、有费用和无费用的任务。
- [ ] 至少一个真实 Agent 能完成认证、单次运行和交互运行。

## 14. 问题反馈建议

出现问题时建议记录以下信息，并在粘贴前移除用户名、绝对路径、提示词、源代码和凭据：

```bash
"$XCLI_BIN" --version
uname -a
"$XCLI_BIN" --config "$XCLI_CONFIG" config validate
"$XCLI_BIN" --config "$XCLI_CONFIG" doctor --json
```

同时附上：

- 执行的 xcli 命令及退出码。
- 预期行为与实际行为。
- 使用的 Agent 名称及原生 CLI 版本。
- 是否使用代理、`--json`、`--cwd`、原生参数或工作流。
- 可最小复现的脱敏配置或工作流。

不要提交 API Key、认证文件、完整环境变量或包含私有源码的输出记录。

## 15. 清理

确认不再需要测试记录后执行：

```bash
PATH="$XCLI_ORIGINAL_PATH"
export PATH
rm -rf "$XCLI_TEST_ROOT"
unset XCLI_TEST_ROOT XCLI_CONFIG XDG_CONFIG_HOME XDG_DATA_HOME CODEX_HOME
unset XCLI_BIN XCLI_ORIGINAL_PATH HTTP_PROXY MCP_TOKEN PROJECT IMPORT_PROJECT
```

如果测试过程中实际安装了某个真实 Agent，请使用对应的 npm 或 Homebrew 卸载命令；xcli 不会自动卸载原生 CLI。
