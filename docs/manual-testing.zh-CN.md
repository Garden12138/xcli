# xcli v0.1.0 手工测试与体验指南

本文用于在 macOS 或 Linux 上手工体验 xcli 的核心功能。完整流程分为两部分：

1. 使用本地假 Agent 完成零费用、无外部依赖的功能测试。
2. 使用真实 Agent CLI 验证安装、认证、交互与结构化输出。

建议先完成假 Agent 测试，再按需调用真实 Agent。完整测试约需 20～30 分钟。

## 1. 测试范围

| 功能 | 假 Agent | 真实 Agent |
| --- | --- | --- |
| 配置初始化与严格校验 | ✓ | — |
| Agent 检测与版本识别 | ✓ | ✓ |
| 默认 Agent 切换 | ✓ | ✓ |
| 交互模式 `use` | ✓ | ✓ |
| 单次运行 `run` | ✓ | ✓ |
| 原生参数透传 | ✓ | 可选 |
| JSON 归一化结果 | ✓ | ✓ |
| 网络配置与环境合并 | ✓ | 可选 |
| 安装命令选择 | dry-run | 可选实际安装 |
| 原生认证入口 | ✓ | ✓ |
| 串行工作流与变量传递 | ✓ | ✓ |
| 重试、跳过、超时和 Ctrl+C | ✓ | — |
| 运行记录与输出保存 | ✓ | ✓ |

## 2. 准备 xcli

确认当前使用的是发布版：

```bash
export XCLI_BIN="$(command -v xcli)"
"$XCLI_BIN" --version
```

预期输出：

```text
xcli version 0.1.0
```

如果尚未安装，请按 [中文 README](../README.zh-CN.md#安装) 下载发布二进制。也可以在 Go 1.26 环境中从源码构建：

```bash
go build -o /tmp/xcli .
export XCLI_BIN=/tmp/xcli
"$XCLI_BIN" --version
```

## 3. 创建隔离测试环境

以下变量只在当前终端会话生效。测试配置、运行记录和输出均写入临时目录，不会读取或覆盖真实的 `~/.config/xcli` 与 `~/.local/share/xcli`。

```bash
export XCLI_TEST_ROOT="$(mktemp -d)"
export XDG_CONFIG_HOME="$XCLI_TEST_ROOT/xdg-config"
export XDG_DATA_HOME="$XCLI_TEST_ROOT/xdg-data"
export XCLI_CONFIG="$XCLI_TEST_ROOT/config.yaml"

mkdir -p "$XCLI_TEST_ROOT/bin" "$XCLI_TEST_ROOT/work"
printf 'test workspace\n' > "$XCLI_TEST_ROOT/work/README.txt"
printf 'XCLI_TEST_ROOT=%s\n' "$XCLI_TEST_ROOT"
```

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
```

预期输出为 `fake-agent 1.0.0`。

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

  fake-alt:
    adapter: generic
    command: $XCLI_TEST_ROOT/bin/fake-agent
    network: isolated
    run_args: ["run", "{{ prompt }}"]
    output: text

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

- `fake` 和 `fake-alt` 均显示为已安装。
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

### 7.2 Agent 选择优先级

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" run fake "使用位置参数选择"
"$XCLI_BIN" --config "$XCLI_CONFIG" run --agent fake-alt --json "使用 --agent 选择"
```

预期结果：位置参数能够选择 `fake`，`--agent` 能够显式选择 `fake-alt`。

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

### 8.2 原生认证入口

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

## 10. 串行工作流

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

### 10.2 JSON 汇总和输出记录

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/success-workflow.yaml" \
  --var message="record this output" \
  --record-output \
  --json
```

预期结果：标准输出只有一个 JSON 执行摘要，包含：

- 工作流 `status: success`、`exit_code: 0`。
- 两个步骤的状态、输出、尝试次数和时间。
- 持久化的 `output_file` 路径。

### 10.3 重试、继续和依赖跳过

```bash
cat > "$XCLI_TEST_ROOT/failure-workflow.yaml" <<'YAML'
version: 1
name: fake-failure
cwd: work

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

### 10.4 超时

```bash
cat > "$XCLI_TEST_ROOT/timeout-workflow.yaml" <<'YAML'
version: 1
name: fake-timeout
cwd: work

steps:
  - id: slow
    agent: fake
    prompt: timeout test
    args: ["--sleep", "2"]
    timeout: 200ms
YAML

"$XCLI_BIN" --config "$XCLI_CONFIG" workflow run \
  "$XCLI_TEST_ROOT/timeout-workflow.yaml" --json
echo "exit_code=$?"
```

预期结果：步骤和工作流状态为 `timed_out`，命令返回退出码 `124`。

### 10.5 Ctrl+C

运行下面的命令，并在假 Agent 休眠时按一次 Ctrl+C：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" run fake "cancel test" -- --sleep 60
echo "exit_code=$?"
```

预期结果：子进程终止，xcli 返回退出码 `130`，不会遗留休眠进程。

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

- 记录包含类型、Agent 或工作流、工作目录、时间、状态和退出码。
- JSON 元数据文件权限为 `-rw-------`，目录权限仅允许当前用户访问。
- 普通运行不会保存完整输出。
- 使用 `--record-output` 的工作流会在对应运行目录中保存步骤输出。
- 记录中不应出现父进程环境变量值或 API Key。

## 12. 使用真实 Agent 体验

真实测试可能消耗 API 配额，并可能按提示修改工作目录。请使用临时目录和低风险提示词。

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
- 最终输出被归一化为包含 `agent`、`output`、`session_id`（原生协议提供时）、`exit_code` 和 `status` 的 JSON。
- 原生 CLI 的诊断信息仍可出现在 stderr。

可以依次替换为其他已安装 Agent：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" run --cwd "$XCLI_TEST_ROOT/real-work" --json claude "只回复 XCLI_OK"
"$XCLI_BIN" --config "$XCLI_CONFIG" run --cwd "$XCLI_TEST_ROOT/real-work" --json gemini "只回复 XCLI_OK"
"$XCLI_BIN" --config "$XCLI_CONFIG" run --cwd "$XCLI_TEST_ROOT/real-work" --json opencode "只回复 XCLI_OK"
```

### 12.3 真实交互模式

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" use \
  --cwd "$XCLI_TEST_ROOT/real-work" codex
```

确认终端颜色、键盘输入、Ctrl+C 和原生权限提示与直接运行 `codex` 时一致。退出后使用 `xcli runs list` 检查元数据记录。

### 12.4 双 Agent 串行工作流

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

完成测试后，可以按下列清单判断 v0.1.0 的核心体验是否符合预期：

- [ ] `config init/path/validate` 行为正确，配置权限为 `0600`。
- [ ] 未知 YAML 字段会立即失败。
- [ ] `agents` 和 `doctor` 能检测路径与版本。
- [ ] 默认 Agent、位置参数和 `--agent` 优先级正确。
- [ ] `use` 保留终端 stdio，`run` 保留普通原生输出。
- [ ] `--json` 返回合法、稳定的归一化结构。
- [ ] `--` 后参数不被 xcli 或 shell 重新解释。
- [ ] `--cwd`、网络变量清除和 Agent 环境变量生效。
- [ ] 安装器 dry-run 展示正确命令且不修改系统。
- [ ] 工作流严格串行，并能显式传递步骤输出。
- [ ] 重试、失败继续、依赖跳过、超时和 Ctrl+C 状态正确。
- [ ] 运行记录权限正确，默认不保存完整输出。
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
rm -rf "$XCLI_TEST_ROOT"
unset XCLI_TEST_ROOT XCLI_CONFIG XDG_CONFIG_HOME XDG_DATA_HOME HTTP_PROXY
```

如果测试过程中实际安装了某个真实 Agent，请使用对应的 npm 或 Homebrew 卸载命令；xcli 不会自动卸载原生 CLI。
