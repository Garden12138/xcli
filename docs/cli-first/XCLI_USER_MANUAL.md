# xcli 使用手册 · R4 CLI-First

2026-11-10，未发布源码候选 `0.5.0-r4-dev`。xcli 调度原生 AI Coding Agent CLI，厂商 CLI 自管安装、登录、账号、模型和额度。日常入口不需要 Gateway/OAuth/模型服务配置。旧验收手册完整保留在[2026-11-06 快照](XCLI_USER_MANUAL_20261106.md)。

## 构建与原生前提

```bash
go build -trimpath -o /tmp/xcli-r4 .
/tmp/xcli-r4 --version
/tmp/xcli-r4 agents --json
/tmp/xcli-r4 doctor codex --json
/tmp/xcli-r4 doctor cursor --json
/tmp/xcli-r4 doctor antigravity --json
```

以下 `xcli` 指该构建产物，请用完整路径或自行放入 PATH。内置映射是 Codex=`codex`、Cursor=`cursor-agent`、Antigravity=`agy`；可在自己的 xcli 配置中覆盖 command。其余已有 Adapter 继续保留。

用户通过原生 CLI 自行安装、登录和处理工作区信任。`agents`/`doctor` 只检查安装/版本/help；没有认证/额度保证。xcli 不调用登录，不提取凭据，不自动 trust/force 或改变原生规则。R3 和 CLI-first 的真实历史结果有具体版本与角色边界，见[支持矩阵](../r3/compatibility.zh-CN.md)。R4 产品收敛批次未执行新真实调用；单独的 [R4 真实验收](../product-realignment/R4_REAL_ACCEPTANCE.md)受新的预算与停止条件约束。

严格按“原生 CLI 进程启动数”限额验收时，不要直接连续执行上面所有诊断命令。`agents` 会对已发现的配置 CLI 启动版本查询，`doctor <agent>` 最多执行版本与 help 两个原生进程；原生登录状态命令也会另起进程。必须先明确探针、验收控制器和任务的统一账本，计数不明时停在真实调用门槛前，不把只读理解成零启动。

## 一次执行与 Session

```bash
xcli run --agent codex --cwd ./project "说明这个项目的结构" --json
xcli run --agent cursor --cwd ./project "复核选定改动" --json
xcli run --agent antigravity --cwd ./project "分析选定材料" --json

xcli use codex --cwd ./project
xcli use cursor --cwd ./project
xcli use antigravity --cwd ./project

xcli resume <run-id> "继续检查" --json
xcli resume <native-session-id> "继续" --agent codex --cwd ./project --json
xcli runs list
xcli runs show <run-id>
```

`run --json` 返回 xcli `run_id`、原生 `session_id`（原生有报告时）、status、parse_status、原生退出与错误分类。只有含有效 Session 的记录能按 Run ID 恢复。`use` 是原生交互入口，交互记录不保证能解析到 Session；此时使用原生提供的 Session ID 和正确 Agent。

Session 不能跨 CLI 使用，也不把其他任务的 `--last` 当路由。`--cwd` 指向用户已核准目录。`--events` 输出 xcli JSONL 活动/终态，与 `--json` 互斥；它不是模型 token SSE。直接 run 没有通用 `--timeout`，需要严格预算时使用 Task。Ctrl-C 取消当前前台进程。

## Work / Project Task

```bash
# Work：输入可重复指定；输出目录保存真实交付文件
xcli task "读取材料，生成 report.md 并检查引用" \
  --agent codex --input ./materials --output ./deliverables --json

# Project：干净且有提交的本地仓库，实际写入受管理 worktree
xcli task "修复缓存问题并通过测试" --agent codex --project ./project \
  --verify '["go","test","-count=1","./..."]' --json

# 自动分阶段执行和跨 CLI 复核
xcli task "执行修复，并由另一个 CLI 复核当前结果" --project ./project \
  --verify '["go","test","-count=1","./..."]' --json
```

显式 `--agent` 将协调、执行、修复、自查全部锁定到该配置名称。自动模式由协调者在已安装结构化 Adapter 中分配阶段；计划分工不等于失败后 fallback。要求独立复核时必须不同 Adapter，别名不算。当前没有公开 `--worker/--reviewer` 或独立 workflow YAML 命令；具体角色要求写入目标并检查保存的计划/复核记录，不能声称已有强绑定参数。

默认累计 10 分钟、最多 6 次 Agent CLI 调用、每次最多 120 秒、一次修复。使用 `--timeout 30m --call-timeout 8m --max-calls 8` 调整预算，不能把调用次数当作供应商内部模型轮数或收费上限。Project 测试按 argv 执行受信代码，未知验证入口用 `--verify`；没有有效验证进入 waiting_user，不虚构通过。

`task --max-calls` 限制任务分派，不覆盖全部发现/版本/help 进程或外部验收控制器，不能充当全局原生启动硬上限。旧 `tests/r0/smoke.py` 的任务计数同样没有覆盖其 `safe_probe` 和控制器；严格总启动限额下不能只加 `--execute` 就运行，复用 `--preflight` 也不证明外部计数完整。直接 run/resume 没有 `--timeout 120s` 参数，需要受同一预算约束的外部监督；不能拿普通 API、冻结 Gateway 或换 CLI 补测试结果。

Work 验收非空产物、所需文本和来源；Project 保留 diff、文件摘要、测试和绑定当前内容的复核。输出图片等二进制可以作为文件保留，但没有统一视觉/工具来源自动验收。应打开真实图片核对格式和内容，明确区分文件存在、格式有效、来源可核实和用户接收。原生 CLI 未具备相应工具时如实失败，不换模型 API 补产物。

## 后台、日志、停止和继续

```bash
xcli task "整理材料并形成报告" --agent codex \
  --input ./materials --output ./deliverables --detach --json
xcli task status <task-id> --json
xcli task status <task-id> --events --after 0 --follow
xcli task stop <task-id> --json
xcli task continue <task-id> --detach --json
xcli task continue <task-id> "补充核查兼容性" --detach --json
xcli task continue <task-id> --verify '["go","test","./..."]'

# 低层后台 Run 保持兼容
xcli run --agent codex --cwd ./project "检查选定文件" --detach --json
xcli jobs list
xcli jobs show <run-id>
xcli jobs logs <run-id> --follow
xcli jobs wait <run-id> --json
xcli jobs stop <run-id> --json
```

Task 日志/审计使用 `task status --events`；`jobs logs` 属于低层后台 Run，不把 Task ID 当 Jobs ID。提交后台成功不等于执行或验收完成。停止观察不会停止 Task；stop 要等待原进程确认退出。

继续保留原 Task、工作区、候选/角色绑定和累计预算，不清零已用时间/调用。原生 Session 不可用时可显式 `--fresh-session` 从材料继续；认证、额度和网络失败不会自动换 CLI/账号。遇到 needs_reconcile 先检查未知副作用、worktree、diff、产物和证据，确认后再执行：

```bash
xcli task continue <task-id> --reconcile --fresh-session --detach --json
```

`--reconcile` 不允许绕过工作区丢失、越界或原仓库保护。Windows 后台 Task/jobs 仍不支持；模拟重启、跨终端继续不等于开机自启或真实重启实测。

## 配置、记录与交付

`config path/init/validate` 管 xcli 自己的配置，默认 `~/.config/xcli/config.yaml`，支持 XDG 与显式 `--config`。不要在配置中放供应商 Token；旧 auth_args/ACP/routing 等字段兼容读取但不恢复退役功能。已有 `mcp serve` 原生 launcher 保留，不替用户修改 MCP 或登录配置。

私有 Task/Run/Jobs 位于 `${XDG_DATA_HOME:-$HOME/.local/share}/xcli`；原始流默认不录制，但目标、最终文本、选定材料、检查诊断与交接仍会保存。失败后保留材料供检查。Project 由用户检查 worktree/diff 后自行提交或复制；xcli 不自动合并、部署或发布。worktree、路径检查与原生权限不是通用安全沙箱。

## 验收与冻结范围

```bash
bash tests/product_realignment/verify-core.sh
# 维护保留的实验代码时，再运行完整离线回归
bash tests/product_realignment/verify-offline.sh
```

核心使用不需要 Model Gateway、Native Agent Gateway HTTP、SIWC/OAuth/Responses 或 Media Gateway。它们默认关闭，旧状态收尾和只读检查保留；详见[模块处置](../product-realignment/MODULE_DISPOSITION.md)。未来应用仅设计[有界 Agent Task](../product-realignment/APPLICATION_INTEGRATION_GUIDE.md)，没有当前可发布的新 HTTP/IPC 接口，也没有无限订阅代理。
