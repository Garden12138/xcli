# R4.1 — Process Launch Ledger & Execution Budget Accounting

2026-11-10。基线 `ae0b4be`（R4 核心 `d247a94`），候选版本 `0.5.0-r4.1-dev`。本轮只做离线工程验收，不新增真实原生 CLI 启动或真实模型请求。产品仍是统一 CLI 调度和任务编排，冻结 Gateway/OAuth 不变。

## 统一启动入口

`internal/launch.Start` 是原生进程的 OS spawn 边界。它先在持久账本原子预留，再持久化启动意图，最后执行 `exec.Cmd.Start`。`internal/runtime.RunProcess` 与只读探针共用此入口，不能只在 Adapter 里累计。

| 路径 | 接入与计数规则 |
| --- | --- |
| `run`、`use`、`resume` | 每次真实启动一条 Launch ID；Session 续接同样扣额，直接 run 无额外免费版本探针。 |
| Task 发现/阶段/恢复/修复/Session 重建 | 版本查询与实际执行分别计数；带 Task/Run/阶段/attempt/retry；不因 `task --max-calls` 另起计数口径。 |
| 后台 Run/Task、受监督 attempt | xcli 自己的 worker/监督进程不当作原生 CLI；通过可信 FD payload/环境传递同一个账本和窗口，内部原生启动正常扣额。 |
| Workflow Agent/command 阶段与 retry | Agent 均扣额；command 用已配置可执行文件身份识别别名、符号链接，再以统一入口计数；每次 retry 新 ID。普通 git/go 工具不冒充原生 CLI。 |
| `agents` / Doctor / 只读 probe | 已发现 CLI 的 `--version`、help、原生状态查询均扣额；PATH、文件散列和纯账本查询不启动 CLI。 |
| 兼容 MCP launcher | 若直接执行原生 CLI（含配置别名），同样计数；普通 MCP 工具不作为模型 CLI。 |
| 冻结 Native Adapter、Codex stdio 与策略探针 | 默认冻结未改；保留源码中的直接原生启动也接同一入口，不提供旁路。旧 Docker 隔离 Profile 仍关闭，没有据此提供容器内启动监控。 |

入口收敛不构成系统级进程监控。供应商 wrapper 内部的 node/工具/模型请求、Agent 或任意测试脚本自行启动的后代进程、外部终端和容器内进程无法由该账本精确观察。不能把一个原生入口的 Launch ID 宣称为其内部所有进程或模型请求的计数。严格验收只允许已审查的合成任务和这些受控直接入口；未知外部范围须 BLOCKED，不得扩大 READY 声明。

## 存储与审计

默认 `${XDG_DATA_HOME:-$HOME/.local/share}/xcli/launches/ledger.json`。提取既有 ModelStore 的私有文件、OS 文件锁和原子替换设施为中立 `internal/localstate`；旧 ModelStore 调用委托此设施，不启用模型服务，也不引入数据库/第三方依赖。Task/Run 保留既有写入规则和格式，仅增加可选关联字段。

账本是唯一预算事实源：窗口、预留、终态、单调 revision 与事件同属一个带 SHA-256 校验的原子快照。统计由记录推导，没有另一套可重置的计数文件。文件 fsync、替换后目录 fsync（Windows 使用 write-through 替换）、私有目录/权限检查、OS 全局事务锁与每 Launch owner lease 共同约束并发和恢复。持久化/锁/校验/版本错误一律拒绝启动；存在启动 lease 但快照丢失也拒绝重建。校验用于发现损坏，不是对同 UID 恶意修改的防篡改签名。须保留整个账本，不通过删目录或换 store 重置验收历史。

每条记录含唯一 ID、Task/Run/Session、Agent/Adapter、类型、规范化可执行路径、已观测版本或 `UNKNOWN`、PID、预留/启动/结束时间、退出码、原因、取消/超时、reserved/consumed/released、父任务/阶段/attempt/retry、关联证据。尚未知道的 PID/时间/退出码为 null。argv 仅作摘要用于幂等冲突检查，不记录 prompt、环境或凭据。`model_requests`、`tokens`、`actual_cost` 始终 null；这些不能由原生启动次数推断。

未对旧 Run/Task 或 R4 历史文件逐条伪造 Launch ID。首次使用初始化的是新账本 schema v1；旧数据保持原字节、查询兼容，`legacy_history=UNKNOWN`。未知/损坏账本版本不做隐式迁移或清零。旧 Task 第一次显式绑定新窗口时，只约束其今后启动，不把旧分派数导入为已知历史启动数。

## 查询与操作

下例仅展示命令，执行前必须有新的用户授权和可靠的外部基线。`owner-receipt` 必须替换为可核验来源；示例 0 不是对当前控制器的判断。

```bash
xcli launches window create acceptance-new --limit 6 --concurrency 1 \
  --external-agent none --external-count 0 --external-scope included \
  --external-source owner-receipt --confirm-external
export XCLI_LAUNCH_WINDOW=acceptance-new
xcli launches budget
xcli launches windows
xcli launches list
xcli launches list --task task-id
xcli launches list --run run-id
xcli launches show launch-id
xcli run --agent codex --cwd /tmp/synthetic-project --launch-request unique-request "合成任务" --json
xcli launches budget
```

也可每次显式给 `--launch-store /absolute/private/path --launch-window acceptance-new`。窗口不可覆写，ID 不重复；没有窗口/额度/外部确认则 fail closed。`--launch-request` 仅适用于一次直接 run/use/resume（含后台 run）；同窗口同 key 重放返回已有 Launch ID 错误，不再次启动或自动重放输出；参数冲突拒绝。Workflow/Task 各内部启动自动各取新 ID，不拿一个顶层 key 去复用多个阶段。

```bash
# 只把不完整历史标为 UNKNOWN，不补造启动记录
xcli launches record-legacy r4-native-unknown --source ae0b4be
# 只读查看历史，不会执行 CLI
xcli --launch-window r4-native-unknown launches budget
# 仅在明确停止证据审查后清除未知进程的并发占位，绝不退预算
xcli launches confirm-stopped launch-id --evidence owner-stop-receipt --confirm
```

版本/help/status 的显式受控入口为 `launches probe <agent> --kind version|help|status`，也会花额度。status 只支持已有明确只读命令的 Codex/Cursor，Antigravity 拒绝猜测接口；只返回认证布尔值，不回显账号原文。未实现登录、Token 管理或账号切换。

## 离线验收

使用 Fake CLI、独立临时 state 和拒绝真实 CLI 的 PATH 夹具；测试包括 100 竞争预算、100 竞争并发槽、100 幂等竞争、多 Task、retry/resume、立即退出、取消/超时、OS Start 失败、启动前/意图后/启动后崩溃、未知占位/停止证明、外部基线缺失、旧记录保护、损坏校验及重启恢复。全量保留源码回归覆盖冻结模块的历史 fake 契约，并不启动真实 Profile 或更改其默认关闭状态。

保留的 R3 native wrapper 已去除内嵌的额外版本子进程，仍用已有可执行文件摘要固定研究入口；研究分派统计不再冒充进程预算。

最终命令、结果、二进制 SHA-256 见 [IMPLEMENTATION_PROGRESS](IMPLEMENTATION_PROGRESS.md) 和 [证据目录](evidence/r4_1)。R4 的真实验收仍为 **NEEDS_FIXES / UNKNOWN**；离线账本通过不等于三个厂商的真实 Session/权限/额度已重验。
