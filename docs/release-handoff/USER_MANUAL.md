# xcli 本地使用手册

适用当前 `0.5.0-r4.1-dev`。从 [快速开始](QUICK_START.md) 完成构建、原生 CLI 自行登录和启动窗口设置后使用本页。下列 run/resume/task/use 是用户主动发起的真实执行示例，本轮没有执行它们。普通本地命令即可使用，不需要启动服务。

## 选择 CLI 与最基本操作

```bash
xcli agents --json
xcli doctor codex --json
xcli doctor cursor --json
xcli doctor antigravity --json

# 用自己的目录替换；仅分析，是否允许读取由原生权限决定
XCLI_PROJECT='/path/to/your/project'
xcli run --agent codex --cwd "$XCLI_PROJECT" '只读分析项目结构，列出主要模块与风险，不修改文件。' --json
xcli run --agent cursor --cwd "$XCLI_PROJECT" '只读复核当前 git diff，给出问题和依据，不修改文件。' --json
xcli run --agent antigravity --cwd "$XCLI_PROJECT" '只读分析项目，说明入口和测试方式，不修改文件。' --json
```

`--agent` 使用 xcli 配置名称；同名内置 adapter 对应 codex/cursor-agent/agy。可改 command 路径，不改认证或模型选择。默认 Agent 来自 config.default_agent，显式 --agent 优先于位置参数和默认；旧 routing 不再自动分派。

`agents` 会探测所有已配置工具，可能消耗多次进程启动；doctor 检查安装/版本/help/声明能力。它们都不能证明账号、模型、额度或工具授权可用。没有缺省启动窗口时，会显示相应预算/探针错误；仅 PATH 发现见安装手册。

文本输出与 JSONL 事件可分别选择：

```bash
xcli run --agent codex --cwd "$XCLI_PROJECT" '说明测试入口，不修改文件。'
xcli run --agent codex --cwd "$XCLI_PROJECT" '说明测试入口，不修改文件。' --events
```

`--json` 是一个最终对象；`--events` 是 version 1 JSONL 流，不能同时使用。前台活动诊断写 stderr；解析失败/缺完整终态即使原生退出 0 也会使 xcli 失败。明确要传原生参数时放在 `--` 之后，xcli 不解释其含义、不默认放宽权限。例如原生 Codex 的图片参数见媒体章节。厂商参数必须与自己的版本帮助一致。

## Run、Session 与后台 Jobs

```bash
xcli runs list --json
XCLI_RUN_ID='run-请替换为实际ID'
xcli runs show "$XCLI_RUN_ID"
xcli resume "$XCLI_RUN_ID" '继续分析刚才的结论，不修改文件。' --json

# 直接续接原生 Session 时必须指定 Agent；不要把其他 CLI 的 Session 交给它
XCLI_SESSION_ID='请替换为真实原生SessionID'
xcli resume "$XCLI_SESSION_ID" '继续核对结论。' --agent cursor --cwd "$XCLI_PROJECT" --json
# 进入已有原生交互客户端
xcli use codex --cwd "$XCLI_PROJECT"
```

`runs show` 本身输出 JSON，没有 --json 参数。只有有效 session_id 的记录能按 Run ID 续接；交互 use 未必捕获 Session。旧 workflow Run 可用 `resume ID --step STEP`，内部兼容不代表公开 workflow 命令恢复。原生 Session 丢失或协议不支持时，resume 不能凭空恢复会话；Task 有独立的材料恢复路径。

单次 Run 的后台模式：

```bash
xcli run --agent codex --cwd "$XCLI_PROJECT" '只读分析测试覆盖并给出建议，不修改文件。' --detach --json
# 将返回的后台 id 填入变量
XCLI_JOB_ID='run-请替换为实际ID'
xcli jobs list --json
xcli jobs show "$XCLI_JOB_ID"
xcli jobs logs "$XCLI_JOB_ID" --follow
xcli jobs wait "$XCLI_JOB_ID" --timeout 10m --json
xcli jobs stop "$XCLI_JOB_ID" --json
```

`jobs wait --timeout` 仅停止等待，不取消 Job；取消使用 jobs stop。前台 run 可用 Ctrl-C，xcli 通常返回 130。jobs stop 默认给原生进程退出宽限；强制停止可能留下不确定外部副作用，应先检查记录和文件。

Jobs 没有 resume/retry/recover 子命令。worker 崩溃后查询会核对锁并收敛为中断状态；若 Run 保存了有效 Session，使用 `xcli resume RUN_ID ...` 创建续接 Run，否则由用户检查文件后决定新任务。新 Run 不是自动重放旧命令。长任务需 checkpoint 续接时优先 Task。日志/删除命令可能影响保留数据，本轮没有删除用户 Jobs。

## Task：文件、验证、交接和恢复

Work 以选定材料生成真实文件；Project 使用本地 Git 项目对应的 worktree。Project 原仓库必须已有提交且干净；任务不会自动合并回原分支。

```bash
# Work：材料目录是已有的个人输入；输出指定到自己的新目录
xcli task '阅读材料生成 report.md，列出可核对来源并自查结论。' \
  --agent codex --input ./materials --output ./deliverables --json

# Project：在独立 worktree 修复并执行真实测试
xcli task '修复已定位的缓存过期 Bug，补充必要边界验证并复核。' \
  --project "$XCLI_PROJECT" --agent codex \
  --verify '["go","test","-count=1","./..."]' --json
```

验证参数是 JSON argv 数组，可重复；`--verify '["go","test","./..."]'` 直接执行 argv，不隐式交给 shell。按项目实际测试命令选择；不要在非 Go 项目照抄 Go 验证。验证会执行受信项目代码，worktree 只隔离工作目录，不是系统沙箱。

任务结果包含 plans、attempts、handoffs、checks、reviews、artifacts、output、workspace 和 diff_file（以实际字段为准）。文件验收会核对存在性/非空/摘要/来源；review 绑定当前 snapshot。缺文件、改输入、错误测试或拒绝权限均不能用一句“完成”盖过。用户仍需检查实际文件及可读的测试/复核结论。

```bash
XCLI_TASK_ID='task-请替换为实际ID'
xcli task status "$XCLI_TASK_ID" --json
xcli task status "$XCLI_TASK_ID" --events
xcli task status "$XCLI_TASK_ID" --events --follow
xcli task stop "$XCLI_TASK_ID" --json
xcli task continue "$XCLI_TASK_ID" '补充错误边界的验证，保留原要求。' --detach --json
```

status --after N 可增量读取事件。stop 终止当前 attempt，通常保留为 paused；continue 在活动 Task 上排入下一 checkpoint，而不并行启动第二个 owner。完成 Task 重复 continue 不自动再次执行。新终端需导出该 Task 原有 XCLI_LAUNCH_WINDOW，若使用自定义 XCLI_LAUNCH_STORE 也需相同。

常见状态：running/planning/executing/verifying/reviewing 表示进行中；paused 可继续；waiting_user 需补充目标或验证；needs_reconcile 需核查不确定副作用；completed 表示当前产物/验证/复核通过，用户接收是独立决定；failed 不代表清除材料。具体 reason 和 attempts 比单个状态更重要。

默认累计最多 6 次 Agent 调用、10 分钟执行时间（暂停不计）、每次 2 分钟及一次有界修复。版本发现也消耗独立的启动窗口，但不计 Task.calls。限额需要提高时，用户明确指定，例如：

```bash
xcli task continue "$XCLI_TASK_ID" --call-timeout 8m --timeout 30m --max-calls 10 --json
```

这只提高 Task 后续累计上限，不退还已用时间/次数，不改变原窗口余额。验证命令仍各限 120 秒；Antigravity Adapter 另传原生 --print-timeout 120s，不能把更高 Task call-timeout 当作已扩大 agy 内部时限。

Session 缺失但材料可核对时：

```bash
xcli task continue "$XCLI_TASK_ID" --fresh-session --detach --json
```

若 reason 明确需要确认未知副作用，先检查工作目录、checkpoint、attempt、文件和原生进程，再由用户决定：

```bash
xcli task continue "$XCLI_TASK_ID" --reconcile --fresh-session --detach --json
```

不要把 --reconcile 当作所有失败的通用解锁键；它确认已有材料和副作用。候选池、既定角色绑定、累计预算不会因恢复清零；不通过切 CLI/账号绕过权限或额度。

## 三个实际场景

### A：单 CLI 修复一个小 Bug

先在自己的干净、已提交项目重现失败并明确范围；以下项目使用 Go 测试：

```bash
git -C "$XCLI_PROJECT" status --short
xcli task '修复 TTL 恰好到期时仍被当作有效的 Bug，仅修改相关实现和必要测试，运行测试并自查。' \
  --project "$XCLI_PROJECT" --agent codex \
  --verify '["go","test","-count=1","./..."]' --json
```

--agent codex 将协调、实现、修复和自查均绑定 Codex，不宣称跨 Agent 审查。完成后从 workspace.path/output（实际 JSON）打开 worktree，用 diff_file 和 checks 检查交付，再自行决定提交/合并；xcli 不代为执行。

### B：一个 CLI 实现，另一个复核

需要自动交接时，不指定 --agent，目标明确要求跨 CLI：

```bash
xcli task '修复已确认的 TTL 问题；要求 Codex 实现并由不同 Adapter 的 Cursor 复核；无法安排时说明限制，不用同 CLI 冒充跨 CLI。' \
  --project "$XCLI_PROJECT" \
  --verify '["go","test","-count=1","./..."]' --json
```

当前协调者在已安装、有结构化能力的候选中生成计划；没有 --worker/--reviewer 参数。上述名称是目标要求，不是命令行强制绑定；必须查看 plans、attempts 的 agent/role 及 reviews.kind，确认 cross_cli 且 Adapter 不同。安装/账号不足或计划无效会失败/等待处理，而不会强行完成。

如果你需要明确固定先 Codex 后 Cursor，可自行执行两次 run（手动交接，不宣称是 Task 自动编排）：

```bash
xcli run --agent codex --cwd "$XCLI_PROJECT" '按已确认需求实现 TTL 修复，运行项目测试，说明修改文件与结果。' --json
xcli run --agent cursor --cwd "$XCLI_PROJECT" '只读审查当前 git diff 和相关测试，指出缺陷及依据，不修改文件。' --json
```

这会直接作用于指定工作目录，应由用户选择目录并检查 diff；与 Task 的受管理 worktree 不同。两个 CLI 自己维护各自登录、模型、额度与权限。

### C：长任务分阶段、后台执行与交付

```bash
xcli task '阅读材料，制定小范围改动计划，逐阶段实现并验证，交付说明与未解决问题；超出本任务范围先报告。' \
  --project "$XCLI_PROJECT" --agent codex \
  --verify '["go","test","-count=1","./..."]' \
  --max-calls 8 --call-timeout 5m --timeout 30m --detach --json
# 取返回 task id，再使用 status/stop/continue
xcli task status "$XCLI_TASK_ID" --events --follow
```

先用实际返回 ID 更新 XCLI_TASK_ID；发现需要补充信息或计划调整时，在 checkpoint 加入消息或停止后继续。Task 执行现有的规划/执行/验证/复核及最多一次修复，不是无限循环、自主持续开发或 R5 的 DAG/固定角色产品。较长目标可能需要用户拆成多个明确任务。启动窗口余量不足会停止，不能给同一 Task 开新窗绕过。

## 媒体任务与真实文件交付

当前支持通过原生 CLI 的现有工具请求图片；**xcli 不提供通用图片 API**，不保证账号有生图工具，不会把文字路径当作图片成功。

| CLI | 仓库真实历史证据 | 限制 |
| --- | --- | --- |
| Codex | [10-15](../r0/media-acceptance.zh-CN.md) 与 [11-06](../cli-first/ACCEPTANCE_REPORT.md) 生成/参考图编辑，PNG 与人工视觉核对 | 仅当时账号、版本、原生工具；尺寸受原生工具限制 |
| Cursor | 10-15 GenerateImage 生成 JPEG、参考编辑并转换为 PNG | 11-06 新临时项目 Trust BLOCKED；不能外推为当前已恢复 |
| Antigravity | 10-15、11-06 image-generator 原生图片生成/编辑，JPEG 与人工视觉核对 | 子代理内部请求不透明；曾因复制权限拒绝导致交付失败，精确处理后恢复；不自动放行 |

本轮仅离线检查已有文件/测试逻辑，未重新调用生图。视频/音乐**模型生成**没有正式通过；历史 FFmpeg 基础剪辑不能证明视频/音乐模型能力。

用户主动执行的提示词示例（需自己的预算/原生权限）：

```bash
xcli run --agent codex --cwd "$XCLI_PROJECT" \
  '通过已有原生图像工具生成一张植物插画，只生成一次；保留原生格式、不覆盖已有文件，返回真实文件路径。' --json
xcli run --agent antigravity --cwd "$XCLI_PROJECT" \
  '请用已有 image-generator 子代理生成一张植物插画，只生成一次；不转换不复制，返回原生产物的实际路径。' --json
# Codex 参考图：原图路径与原生参数支持情况由用户核对
xcli run --agent codex --cwd "$XCLI_PROJECT" \
  '仅把参考图花盆改为蓝色；不覆盖原图，返回新文件实际路径。' --json -- --image /path/to/source.png
```

原生产物可能在 CLI 自己目录，默认 run 不自动导入所有文件。先核对结果中的路径是否存在、是否是普通非 symlink 文件、真实 MIME/尺寸/可完整打开，再人工检查画面。若需复制到交付目录，由用户确认源/目标权限并验证副本摘要；不要只改扩展名。Task 可以收集其声明输出范围内的文件/摘要，但 completed 也不替代视觉接收或工具来源证据。

```bash
XCLI_IMAGE='/path/to/actual-output.png'
test -f "$XCLI_IMAGE" && test ! -L "$XCLI_IMAGE"
file "$XCLI_IMAGE"
# macOS 尺寸与摘要
sips -g pixelWidth -g pixelHeight "$XCLI_IMAGE"
shasum -a 256 "$XCLI_IMAGE"
```

Linux 使用 `sha256sum` 和本机图片查看器/已安装的解码工具。命令通过只证明文件属性；语义、原图未变、授权来源和用户视觉验收需另外确认。

## 配置、保留数据与 MCP

`config path/init/validate`、--config、XDG 路径见 [安装说明](LOCAL_INSTALLATION.md)。默认数据根 `${XDG_DATA_HOME:-$HOME/.local/share}/xcli`，包括 runs、tasks、launches，旧配置和数据不自动删除。recording.output 默认 false，目标、选定材料、结果与必要诊断仍有私有保留；后台 logs 仍存在。不要把整个私有数据目录提交到 Git。

MCP 仅保留 `xcli mcp serve <server>`，读取已有 xcli MCP 声明并作为原生配置 launcher；可用 --project-config 指向项目配置。它不是 MCP server 安装器、同步管理或登录工具。既有原生配置继续由各 CLI 管理。

## 故障排查

| 现象 | 排查与处理 |
| --- | --- |
| xcli 版本仍为旧版 | command -v xcli；检查 PATH 优先级或使用本仓库 bin/xcli；新 clone 的远程 main 不含本次本地提交 |
| CLI 未安装/command not found | command -v codex/cursor-agent/agy；检查 xcli 配置 command；按原生安装渠道补齐，重开终端 |
| 未登录/认证错误 | 用户在对应原生 CLI 登录；doctor 成功不证明登录；不把 Token 写入配置/日志 |
| 额度不足/限流 | 查看 error_kind/reason 与原生提示，等待或在原生工具检查额度；不自动切账号/CLI 重试 |
| 原生权限拒绝/denied_actions | 检查拒绝的具体工具/路径，用户在原生规则中做范围明确的决定；xcli 不主动 force/自动批准 |
| Cursor Workspace Trust | 用户针对实际 cwd/worktree 在 Cursor 原生界面确认信任；新 worktree 可能另需确认。不要把 --trust 作为默认绕过参数 |
| 网络/代理错误 | 检查当前终端网络及代理变量、原生 CLI 自身网络；必要时使用已有 networks 配置，不输出代理密码。诊断为网络失败不证明登录失效 |
| launch_window_required / UNKNOWN | 没有已确认本次窗口；按安装手册建立真实范围，未知外部计数不得填写 0 |
| EXHAUSTED / concurrency_limit | 查询 launches budget/list/show；预算耗尽停止；核对仍运行/未知停止记录。相同 Task/Run 不能换窗口清零 |
| unknown 启动占位 | 先独立核实原生进程是否退出，再按 launches confirm-stopped --help 提供真实停止证据；只释放并发，不返还启动额度 |
| Session 不可恢复 | runs show 查 session_id/agent/cwd；用正确原生会话；Task 可 fresh-session 从材料继续；单 Run 没 Session 不能自动重建 |
| Task 超时 | 查看 status 的 reason、attempts/checks；检查文件与副作用后提高 call-timeout/timeout；需要时显式 reconcile；暂停不清预算 |
| Jobs 无法恢复 | 查看 jobs show/logs 与 Run Session；没有 jobs resume，必要时 resume RUN_ID 或经检查后创建新任务；Windows 后台未支持 |
| needs_reconcile / owner 丢失 | 确认原进程、workspace、checkpoint 与材料，再决定 --reconcile；不要删锁/账本或伪造结束证据 |
| 图片未生成/路径不对 | completed/文字路径不等于文件；核实源路径、MIME、非空、完整解码和视觉；复制权限不足须用户决定，不能改后缀冒充格式 |
| 命令/版本不兼容 | xcli --help 与该原生 CLI 自己的 --help 分别核对；unknown flag/parse error 保留诊断。旧 docs/r0–r3 示例不是当前命令清单 |
| Project 拒绝启动 | git status 检查干净与已有提交、输入路径和 symlink 越界；不自动 stash/reset 或删除用户改动 |

已知限制：本轮真实模型/账号/原生取消仍 NOT_TESTED；macOS Intel/Linux 只有编译证据；Windows 后台不支持；没有固定角色参数/无限任务/自动合并发布；冻结服务不属于体验流程。历史 R4 NEEDS_FIXES/UNKNOWN 保持事实，详见 [自动验收](AUTOMATED_ACCEPTANCE.md)。
