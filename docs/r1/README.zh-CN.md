# R1：一个 Task 入口，两种执行方式

本页对应 `0.5.0-r1-dev` 源码构建。已发布的 v0.4.0 不包含 Task；本批不打 tag、不发布安装包。Work/Project × 指定/自动四条真实路径已在 Codex/Cursor 上通过，完整范围与证据见 [验收记录](results.zh-CN.md)。

## 开始任务

先按原生工具的方式安装并登录。`xcli doctor codex` 检查可执行文件、版本和已声明/帮助中可观察的能力；它不证明登录、额度或权限可用。xcli 沿用原生登录入口，不登录、轮换账号或复制凭据。

```bash
# Work：固定材料 → 文件 → 自查
xcli task "根据材料生成 report.md，覆盖关键结论并列出来源" \
  --agent codex --input ./materials --output ./deliverables

# Work：自动安排执行与跨 CLI 复核
xcli task "整理材料形成报告，并由另一个 CLI 核对结论" \
  --input ./materials --output ./deliverables

# Project：从干净 Git 基线创建受管理 worktree
xcli task "修复缓存失效问题并通过项目测试" --project . --agent codex

# 显式验证 argv 覆盖协调者发现的命令，可以重复传入
xcli task "修复缓存失效问题并由另一个 CLI 审查" --project . \
  --verify '["go","test","-count=1","./..."]' --json

xcli task status <task-id> --json
```

路径参数相对调用目录解析。`--project` 指定本地 Git 单仓库，要求原仓库干净且有提交；从子目录传入时保留对应 cwd。Project 的 `--output` 必须在原仓库范围内，实际写入位置映射到 worktree。Work 的输出缺省为私有 Task 工作目录，也可指定交付目录。输入目录会展开为有内容摘要的文件引用。

`--agent` 将候选池锁定到一个配置名称，协调、执行、修复和自查都使用它；不会在失败后换 CLI。不同角色使用同一个 CLI 的结果标作 `self_check`。用户要求另一 CLI 却指定单 CLI 时需要澄清，不能解除约束。

自动模式优先选择配置的 `default_agent`，其余按注册名称排序，再过滤安装、版本和结构化执行能力。协调 CLI 根据目标和候选 Adapter 提出分工；显式要求跨 CLI 时，执行与复核必须使用不同 Adapter，别名不算另一种 CLI。认证、额度等未知情况通过真实执行报告；不自动修改原生信任、插件、MCP 或长期权限规则。

## 有界执行与真实验收

默认总时限 10 分钟、最多 6 次 Agent CLI 调用、每次最多 120 秒，允许一次修复。`--timeout` 和 `--max-calls` 可以修改总预算；协调、执行、自查/复核及修复均计数，确定性验证命令不计为 Agent 调用。厂商内部模型轮次和子 Agent 用量可能不可观测，不能将进程调用数当模型请求数。

R1 依次执行协调计划、产出、确定性检查、复核；失败时交接问题并修复一次，再次验证和复核。协调计划是经过校验的 JSON 动作，不是直接执行的 YAML 或 shell。所有步骤串行，一个 Project 同时只有一个可写角色。

自动验证支持 Go test/vet、Cargo test/check/clippy、pytest/unittest 和已声明的 npm/pnpm/yarn test、lint、typecheck 脚本。未知入口需要显式 `--verify`。验证直接执行 argv，不经过 shell；禁止另一个 Agent CLI、shell 包装、安装/发布入口及执行目录覆盖。项目测试脚本本身属于用户信任的代码，这些检查不构成操作系统沙箱，也不证明脚本没有其他副作用。

Project 缺少可执行验证时保存改动并进入 `waiting_user`；原生退出 0、Agent 声称测试通过和历史验证都不能代替当前真实命令证据。Work 检查计划声明的非空文件、必要内容和来源引用，并分别保留复核评价。声明的 HTTPS 来源按可访问性检查，本地来源限定为显式输入或工作区文件；事实正确性与用户接收仍需独立判断。

检查与复核都关联当前内容摘要，测试修改源文件会使本轮验证变为 `stale`。复核读取固定材料副本和选定输入副本；副本或工作材料变化即拒绝复核结果。Project 保存基线 diff，并列出未跟踪文件及其摘要。原仓库变化会让任务停止，不自动回滚用户数据。

Codex 的协调/复核使用原生 read-only，执行/修复使用 workspace-write；Cursor 协调/复核使用 ask，执行使用 sandbox enabled；Antigravity 使用 plan/accept-edits。未映射的 Adapter 仍是提示词约束，权限支持按真实版本验收记录判断。worktree、目录校验与材料摘要均不等于通用文件系统隔离。

## 输出、状态与私有数据

- 人类进度写 stderr，最终摘要写 stdout；`--json` 始终输出一个 Task 对象，`--events` 输出带单调序号的 JSONL，两者互斥。
- Task 状态为 `planning`、`running`、`waiting_user`、`completed`、`failed`、`canceled`。成功退出 0，缺验收/失败退出 1，控制进程取消退出 130，总时限耗尽退出 124；每个 Run 保留原生退出码和解析状态。
- Task ID 是跨角色的工作身份，Run ID 是一次执行，原生 Session ID 留在对应 Run 中。`task status` 只读保存状态，不自行修复控制进程异常退出的记录。
- 私有数据位于 `${XDG_DATA_HOME:-$HOME/.local/share}/xcli/tasks/<task-id>/`，目录 0700、文件 0600；必要的目标、计划、交接摘要、内容摘要、验证诊断及复核材料会持久保存。验证诊断可含项目内容，仅在私有目录中供修复和排查使用。
- 原始 Agent 输出仍由既有 `recording.output` 控制，默认关闭。Task 事件不广播原始工具载荷；最终结果和显式选择的交接材料仍可能包含用户目标和文件路径。
- 失败后保留工作区和证据。Project 的接收方式是检查摘要中的 worktree、diff 和未跟踪文件，人工提交或复制到目标分支；R1 不自动合并或删除工作区。

R2 再提供 Task 后台、stop/continue、检查点和失活恢复。R1 的 `waiting_user` 只保存待补条件，尚不能原 Task 继续；需要补充目标/`--verify` 新建任务，旧 Task 的材料保留供显式引用。Ctrl-C 会停止当前任务并保存取消状态，不等于可恢复暂停。

## 退役与兼容

R0 提交 `5a2a676` 是最后保留旧管理入口的源码里程碑；R1 源码移除 `install`、`auth`、`default`、`acp`、`route`、独立 `workflow`、`usage` 和 MCP import/plan/sync。没有把正式发布版本号当成已发布事实。

替代路径：安装/登录使用原生 CLI；选择使用 `task --agent` 或配置 `default_agent`；编排使用自然语言 Task；用量在 Task/Run 中查看。需要旧 YAML 入口的用户可以使用 R0 源码或适用的历史版本，不能在 R1 调用已移除的命令。

保留 `run/use/resume`、`jobs`、`runs`、`agents/doctor/config`；已有后台 workflow worker 的内部兼容仍保留。旧配置中的 routing、ACP、auth 等字段兼容读取，但不再启用已退役功能。原生文件、MCP 源声明、ownership、历史 Run 和用户产物不会因退役被删除或迁移。

`xcli mcp serve` 原样保留为旧原生配置的兼容 launcher，包含 project-config、cwd 和环境过滤语义。迁移 launcher 仍按 [R0 说明](../r0/README.zh-CN.md)操作，不随 R1 删除。

首版材料限制：单文件最多 32 MiB，工作材料总量最多 128 MiB，交接提示最多 128 KiB；材料中的符号链接和特殊文件保守拒绝。Project 排除 Git 忽略的构建/缓存文件，保留跟踪文件与非忽略的未跟踪文件。大项目应选择必要输入，不能将本阶段当作任意规模材料平台。
