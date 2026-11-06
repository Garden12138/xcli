# xcli CLI-first 使用手册

适用基线：2026-11-06，真实验收 HEAD `7e57c6774b4bcfd4735f0d4c35c7c7a3a58d2ac4` / `0.5.0-r3-dev`。

**Gateway 的 CLI 模式尚不可用。没有已验收的 Codex/Cursor/Antigravity HTTP 模型 alias，也没有可用的 CLI 图片 API 配置。** 当前替代入口是用户本人在终端执行受控 `run/resume/task`。本轮已验证 Codex、Antigravity 的文本新会话、同 Session 继续、图片生成和参考编辑；Cursor 受临时项目 trust 阻断，未重试。Task、独立 Vision、严格 Schema、真实取消/超时仍未验收。见[报告](ACCEPTANCE_REPORT.md)。

## 安装与构建

需要 Git、Go 1.26 或以上。Python 3 用于验收脚本，媒体采集/校验另需 Pillow（本机 12.2.0）；本轮未自动安装依赖。已发布 v0.4.0 不含当前 Task 和 Gateway 候选功能，不能用它替代本基线验收。

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

`agents` 表示可发现安装；`doctor` 的 `source: help/declared` 表示帮助或配置声明，不代表登录、配额、沙箱或图片工具已实测。Codex 可用 `codex login status` 检查已有登录，Cursor 可用 `cursor-agent status --format json`；状态输出可能包含账号信息，只在本机查看，分享时仅保留认证布尔值。先前宿主检查两者已有登录；本轮 Codex、Antigravity 已完成真实推理，Cursor 的失败来自 workspace trust，不能据此判账号失效。为保留 12 次上限，本轮没有额外启动版本/认证探测。

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

以下方法已在本轮 Codex/Antigravity 合成文本测试通过；再次运行仍需新的模型调用授权，本轮已结束，不重跑。只用临时、无敏感数据的 Git 项目；工作区信任未满足时由本人检查原生审批，不能自动添加 `--trust` 或永久权限。

```bash
XCLI_BIN=/tmp/xcli-cli-first
XCLI_PROJECT="$(mktemp -d)"
XCLI_CONFIG="$XCLI_PROJECT/xcli.yaml"
printf 'version: 1\nrecording:\n  output: false\n' > "$XCLI_CONFIG"
XCLI_DATA_DIR="$XCLI_PROJECT/xcli-state"
export XDG_DATA_HOME="$XCLI_DATA_DIR"
git -C "$XCLI_PROJECT" init -q
printf '%s\n' 'CLI-FIRST-DEMO-42' > "$XCLI_PROJECT/context.txt"

"$XCLI_BIN" --config "$XCLI_CONFIG" run --agent codex --cwd "$XCLI_PROJECT" --json \
  '只读取 context.txt，记住并仅回复其中的标记。不要修改文件或调用其他 AI CLI。' \
  -- --sandbox read-only -c 'approval_policy="never"'
```

Antigravity 同样的文本读标记方法使用 `--agent antigravity` 和 `-- --mode plan --sandbox`，首次与恢复均已通过。Cursor 的 `--mode ask --sandbox enabled` 调用在未受信任的临时项目中被拒，本轮没有经验证可直接启动的 Cursor 命令；不要自动添加 trust/force。这些是进程级约束，不能据此保证所有文件读取、工具及 MCP 均被隔离。

`--json` 输出一个 `RunResult`。检查 xcli 退出码、`status`、`parse_status`、`error_kind` 和独立标记；不要只检查原生退出 0。`session_id` 是原生会话，`run_id` 是 xcli 记录。恢复可以使用 Run ID 或明确指定 Agent 的 Session ID：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" resume '<session_id>' --agent codex --cwd "$XCLI_PROJECT" --json \
  '不读文件，不使用工具，重复上轮记住的标记。' \
  -- --sandbox read-only -c 'approval_policy="never"'
"$XCLI_BIN" runs list
"$XCLI_BIN" runs show '<run_id>'
```

不要使用原生 `--last` 作为多应用的会话路由；每个客户端必须保存并隔离自己的会话 ID。明确传 `--cwd`，保留原临时项目直到恢复结束。xcli 已适配 Codex exec resume 不接受 `--sandbox` 的差异，转换为该会话的配置覆盖。

`run --events` 或 `resume --events` 输出 xcli JSONL 事件，不能和 `--json` 合用。这些事件有 `started/progress/completed/failed`，可能合并 progress；它们不是模型逐 token 增量或 OpenAI SSE。Ctrl-C 取消当前前台执行；真实三种 CLI 的主动取消/超时语义本轮未测，不能宣称已经通过。所有本轮前台进程已自然退出；没有运行中的后台服务需要停止。R0 smoke 对每次进程设置 120 秒外部时限，底层 `run` 没有通用 `--timeout` 参数。

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

本轮 Codex 与 Antigravity 的生成和参考编辑均通过，Cursor 未启动媒体场景（trust 前置 BLOCKED）。[四张实际产物及摘要](ACCEPTANCE_REPORT.md#媒体字节与视觉)已经文件与视觉核验；历史图像仍只适用于历史范围。原生 Agent 可能通过内置图片工具生成文件；其最终回答中的路径不是图片字节，也不等于 `/images/generations`。

经额外授权后，每 CLI 最多一次生成、一次编辑，保留原生格式，不要求转换或复制到一个虚构扩展名。检查文件存在且为普通文件、允许目录、真实 MIME、完整解码、尺寸、摘要、编辑输入未覆盖和实际视觉变化。无文件、被拒绝、只回复“完成”或用代码绘图替代时不得入库为成功。当前 xcli 产品 Runtime 不提供统一媒体产物校验；本轮由 `tests/r0/native_media.py` 与离线人工复核补上，未来 Agent API 仍须自行实现。

## 本轮已验证的媒体调用方式

仅适用于本人批准的隔离合成项目，`run` 只负责原生任务。Codex 的生成采用 `--sandbox workspace-write -c 'approval_policy="never"'`；编辑还附加 `--image "$REFERENCE"`，两次均确认原生 image_gen 工具。Antigravity 使用 `--sandbox`，提示明确使用内置 image-generator、返回原生文件，不选择 `--agent image-generator`。Antigravity 内部子代理的精确模型请求数不可观测。

先把 `REFERENCE` 设为已校验的原图绝对路径；不要直接使用未经验证的模型返回路径。以下是与本轮相同的调用形态；每条会消费新的原生额度，不在当前已结束的 9 次中自动执行：

```bash
"$XCLI_BIN" --config "$XCLI_CONFIG" run --agent codex --cwd "$XCLI_PROJECT" --json \
  '仅用原生图片工具生成1张绿植与陶土花盆、米白背景的插画。保留原生格式，不用代码绘图、不转换、不重试；返回实际本地路径。权限或工具不可用就停止。' \
  -- --sandbox workspace-write -c 'approval_policy="never"'

"$XCLI_BIN" --config "$XCLI_CONFIG" run --agent codex --cwd "$XCLI_PROJECT" --json \
  "用原生图片工具编辑参考图 $REFERENCE，只把花盆改蓝，保持植物和背景，不覆盖原图；返回实际路径。" \
  -- --sandbox workspace-write -c 'approval_policy="never"' --image "$REFERENCE"
```

完整受限验收入口（已经执行过，重跑需要新授权）为：

```bash
python3 tests/r0/smoke.py --binary "$XCLI_BIN" --execute \
  --preflight docs/cli-first/evidence/preflight.json --out "$PRIVATE/text.json"
python3 tests/r0/native_media.py --binary "$XCLI_BIN" --execute \
  --text-report "$PRIVATE/text.json" --preflight docs/cli-first/evidence/preflight.json \
  --out "$PRIVATE/media.json" --artifacts "$PRIVATE/artifacts"
```

`--out` 必须是新路径；有失败/缺前置时不重试或借用其他 CLI 槽。嵌套图片工具在外层事件不可见时保留为待复核，不能直接判失败，也不能伪造通过。离线检查真实文件与来源后，仅对从未执行过的编辑槽使用 `--edit-only --verified-generations ... --prior-media-report ...`；它检查原图哈希和既有调用账本，不重新生图。未来版本不得盲用旧版本元数据。

## 停止和运行记录

本轮全部是前台任务，已自然结束，native/xcli 退出码记录在证据中，没有安装服务或启用后台任务。需要主动中断时终端 Ctrl-C 是源码支持方式，但本轮没有为此另起模型请求；`task stop`/`jobs stop` 也只保留为源码与离线测试支持，不能称本轮原生取消已通过。云端图片任务可能继续执行/计费，不能承诺杀本地进程会撤销它。

以下命令已在本轮真实媒体 Run 的隔离 XDG 数据目录执行成功，不启动模型：

```bash
XDG_DATA_HOME="$XCLI_DATA_DIR" "$XCLI_BIN" --config "$XCLI_CONFIG" runs list --json
XDG_DATA_HOME="$XCLI_DATA_DIR" "$XCLI_BIN" --config "$XCLI_CONFIG" runs show '<run_id>'
XDG_DATA_HOME="$XCLI_DATA_DIR" "$XCLI_BIN" --config "$XCLI_CONFIG" jobs list --json
```

`XCLI_DATA_DIR` 是启动该 Run 时实际使用的私有数据根（上例是 `$XCLI_PROJECT/xcli-state`），不是任意用户历史目录。`runs show` 本身输出 JSON，没有 `--json` 标志；本轮 `jobs list --json` 也已确认隔离状态中没有后台任务。Run 元数据不是原始聊天日志；本轮 `recording.output=false`，没有 raw output_file。透明观察器也只留类型、工具标识、时间和摘要。不要为排错开启全量凭据/聊天日志上传。

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
