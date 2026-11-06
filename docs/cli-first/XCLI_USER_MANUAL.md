# xcli CLI-first 使用手册

适用基线：2026-11-06，`f529e04` / `0.5.0-r3-dev`。

**Gateway 的 CLI 模式尚不可用。没有已验收的 Codex/Cursor/Antigravity HTTP 模型 alias，也没有可用的 CLI 图片 API 配置。** 当前替代入口是用户本人在终端执行受控 `run/resume/task`。本轮只验证了离线测试与诊断；下面的模型执行示例是源码支持的操作说明，尚未在本轮真实账号上通过。见[报告](ACCEPTANCE_REPORT.md)。

## 安装与构建

需要 Git、Go 1.26 或以上。Python 3 用于验收脚本。已发布 v0.4.0 不含当前 Task 和 Gateway 候选功能，不能用它替代本基线验收。

```bash
git clone https://github.com/Garden12138/xcli.git
cd xcli
git status --short
git rev-parse HEAD
go build -trimpath -o /tmp/xcli-cli-first .
/tmp/xcli-cli-first --version
/tmp/xcli-cli-first agents --json
```

已有仓库直接从 `git status` 开始；不要为了对齐文档强制 reset 或覆盖用户文件。若要固定本报告基线，在独立干净副本检出上述完整 SHA 后构建。构建输出使用独立文件；不要默认仓库 `./xcli` 就与当前源码一致。本轮新构建的摘要见[清单](evidence/source-manifest.json)。

需要日常保留时，由用户将新二进制放入自己可控的 PATH 目录，保留旧二进制及其版本摘要。`/tmp` 不适合长期安装；本轮没有向 PATH 安装或注册系统服务。

## 原生 CLI 前置检查

从各厂商官方入口安装：[Codex](https://learn.chatgpt.com/docs/codex/cli)、[Cursor](https://cursor.com/docs/cli/headless)、[Antigravity](https://antigravity.google/docs/install/)。按各自交互流程完成本人登录；本轮不代为登录、不复制 Token、不切换账号。

```bash
codex --version
cursor-agent --version
agy --version
/tmp/xcli-cli-first doctor codex --json
/tmp/xcli-cli-first doctor cursor --json
/tmp/xcli-cli-first doctor antigravity --json
```

`agents` 表示可发现安装；`doctor` 的 `source: help/declared` 表示帮助或配置声明，不代表登录、配额、沙箱或图片工具已实测。Codex 可用 `codex login status` 检查已有登录，Cursor 可用 `cursor-agent status --format json`；状态输出可能包含账号信息，只在本机查看，分享时仅保留认证布尔值。本轮宿主检查两者已有登录，Antigravity 有效性仍未测。

不要读取或上传登录文件、Cookie、Keychain 条目。若受限执行环境报告未认证，先区分 Keychain 权限与真实登录失效；不要立即注销或换账号。

可选 xcli 配置默认为 `~/.config/xcli/config.yaml`，支持 `XDG_CONFIG_HOME` 与 `--config`。没有文件时加载内置定义。以下是 xcli 配置，不是原生 CLI 登录或 Gateway 配置：

```yaml
version: 1
default_agent: codex
recording:
  output: false
```

内置命令映射为 `codex`、`cursor-agent`、`agy`。有别名或自定义路径时先检查用户已有配置；不要自动覆盖。原生 CLI 全局设置、MCP、代理、trust 和审批会影响执行；xcli 不会将它们转成一个通用安全沙箱。

## 单次文本任务与恢复

以下示例需要另有模型调用授权。只用临时、无敏感数据的 Git 项目；工作区信任未满足时由本人检查原生审批，不能自动添加 `--trust` 或永久权限。

```bash
XCLI_BIN=/tmp/xcli-cli-first
XCLI_PROJECT="$(mktemp -d)"
git -C "$XCLI_PROJECT" init -q
printf '%s\n' 'CLI-FIRST-DEMO-42' > "$XCLI_PROJECT/context.txt"

"$XCLI_BIN" run --agent codex --cwd "$XCLI_PROJECT" --json \
  '只读取 context.txt，记住并仅回复其中的标记。不要修改文件或调用其他 AI CLI。' \
  -- --sandbox read-only -c 'approval_policy="never"'
```

Cursor 替换为 `--agent cursor`，原生参数使用 `-- --mode ask --sandbox enabled`；Antigravity 使用 `--agent antigravity` 和 `-- --mode plan --sandbox`。这些是进程级约束，不能据此保证所有文件读取、工具及 MCP 均被隔离。

`--json` 输出一个 `RunResult`。检查 xcli 退出码、`status`、`parse_status`、`error_kind` 和独立标记；不要只检查原生退出 0。`session_id` 是原生会话，`run_id` 是 xcli 记录。恢复可以使用 Run ID 或明确指定 Agent 的 Session ID：

```bash
"$XCLI_BIN" resume '<session_id>' --agent codex --cwd "$XCLI_PROJECT" --json \
  '不读文件，不使用工具，重复上轮记住的标记。' \
  -- --sandbox read-only -c 'approval_policy="never"'
"$XCLI_BIN" runs list
"$XCLI_BIN" runs show '<run_id>'
```

不要使用原生 `--last` 作为多应用的会话路由；每个客户端必须保存并隔离自己的会话 ID。明确传 `--cwd`，保留原临时项目直到恢复结束。xcli 已适配 Codex exec resume 不接受 `--sandbox` 的差异，转换为该会话的配置覆盖。

`run --events` 或 `resume --events` 输出 xcli JSONL 事件，不能和 `--json` 合用。这些事件有 `started/progress/completed/failed`，可能合并 progress；它们不是模型逐 token 增量或 OpenAI SSE。Ctrl-C 取消当前前台执行；真实三种 CLI 的超时/取消语义本轮未测。R0 smoke 对每次进程设置 120 秒外部时限，底层 `run` 没有通用 `--timeout` 参数。

## Task 工作流

Task 源码与确定性测试已覆盖预算、产物、验证和恢复；本轮没有另行消费真实 Task 调用额度。如下命令可能启动多轮原生 CLI，**不属于 R0 的六次授权**：

```bash
"$XCLI_BIN" task '读取合成材料，生成 report.md 并自查' \
  --input ./synthetic-materials --output ./deliverables --agent codex \
  --max-calls 6 --call-timeout 120s --timeout 10m --json

"$XCLI_BIN" task status '<task_id>' --json
"$XCLI_BIN" task status '<task_id>' --events --follow
"$XCLI_BIN" task stop '<task_id>'
"$XCLI_BIN" task continue '<task_id>' '补充验收条件' --detach
```

默认累计最多 6 次 Agent 进程、10 分钟、每次 120 秒；内部模型轮数可能更多。`--agent` 固定整个 Task 的 CLI，不会自动换品牌。Project 模式用 `--project .` 建受管理 worktree；验证命令可用 `--verify '["go","test","./..."]'` 显式提供。不要在本次四应用业务仓库运行这些示例。

Task 的运行成功、验证通过、用户接收是不同状态。`waiting_user`、`needs_reconcile` 不代表完成；有不明确的外部副作用时，先检查保留材料再决定是否 `continue --reconcile`，不要盲目续跑。

## 图片交付

本轮图片能力 NOT_TESTED。历史图像例子只能参考，不能作为当前账号或版本可用承诺。原生 Agent 可能通过内置图片工具生成文件；其最终回答中的路径不是图片字节，也不等于 `/images/generations`。

经额外授权后，每 CLI 最多一次生成、一次编辑，保留原生格式，不要求转换或复制到一个虚构扩展名。检查文件存在且为普通文件、允许目录、真实 MIME、完整解码、尺寸、摘要、编辑输入未覆盖和实际视觉变化。无文件、被拒绝、只回复“完成”或用代码绘图替代时不得入库为成功。当前 xcli 不提供统一媒体产物校验；必须由验收/将来的 Agent API 补上。

## Mac 重启、日常检查与故障排查

当前 Agent 命令按需启动，不需要先启动一个 xcli 模型服务。重启后打开终端，确认保留的二进制路径、版本、原生 CLI 及已有登录可用，再执行 `agents/doctor`。临时目录和 shell 环境变量可能已消失；用持久的本人工作区和原 Session/Task 记录恢复。历史后台进程不可能跨重启继续存活，先 `task status` 检查状态，核对副作用再继续。本轮未实际重启机器。

默认 xcli 私有运行数据在 `~/.local/share/xcli`，支持 `XDG_DATA_HOME`；目录 0700、文件 0600。`recording.output: false` 不保留 xcli 原始 Agent 输出，但原生 CLI 仍可能保存自身会话，云推理也有厂商的数据政策。不要把 localhost 等同于离线推理。

| 现象 | 处理 |
| --- | --- |
| `doctor` 找不到 CLI | 在同一启动环境检查 PATH/配置里的 command，不自动安装或切换账号 |
| 受限环境显示未登录 | 本人检查 Keychain/执行权限，比较官方只读 status；不打印 Token |
| trust/approval 拒绝 | 停止并检查临时项目和最小所需权限；不加全权限跳过参数 |
| `quota_exceeded` | 停止，等待恢复或由本人明确授权下一任务；不换账号、自动重试或换后端 |
| `parse_error`/`truncated` | 保留脱敏版本、退出码及类别；运行离线协议回归，不能当成功输出 |
| 图片路径不存在/格式错 | 媒体任务未通过；不创建占位图或只改后缀补成功 |
| `native CLI bridge is disabled` | 当前架构缺实现；按接入计划工程化，配置普通 API 不能解决 CLI 目标 |
| Go cache/loopback 被沙箱拒绝 | 使用可写临时缓存；真实需要时申请离线测试的精确宿主权限 |

升级时保存旧二进制、配置及私有数据位置，先在独立输出路径构建新版本，完成离线门禁，再批准少量真实回归。失败则停止自己启动的任务，保留失败证据、恢复旧二进制，不清空原生登录或历史 Task。当前没有新 schema 迁移或服务安装。旧 HTTP Gateway 的独立维护见[原说明](../model-gateway/README.md)，其启动/Token/alias 不是 CLI 模式的可用配置。

未来接入需要：先实现并验收[受控 Agent Task API](INTEGRATION_ARCHITECTURE.md)，再为每应用发独立本地授权、按真实能力发现选择 Agent、接入审批和文件产物。B 通过之前，不发布所谓可运行的 CLI Gateway Token/alias/HTTP 调用教程。
