# 版本与功能差异

## 基线与可复核方法

候选为 `0.5.0-r4.1-dev`，升级源码基线 `665ec15b512f54145df3bf34417d21fd0c73861f`（下称 U）。本次交付在该 HEAD 上追加整理/测试/文档提交，未替换升级代码。最终提交以 `git rev-parse feature/upgrade-chatgpt` 为准，见 [分支报告](BRANCH_CONSOLIDATION.md)。

| 对照 | 具体提交 SHA | 与 U 的关系 | U 的树差异（A/M/D） |
| --- | --- | --- | --- |
| 实际本地主分支 `main` | `665ec15b512f54145df3bf34417d21fd0c73861f` | 整理前恰好等于 U；没有假设所有分支已合并 | 0/0/0 |
| 默认远程跟踪快照 `origin/main` | `7e57c6774b4bcfd4735f0d4c35c7c7a3a58d2ac4` | U 超前 21、落后 0 | 352/59/0 |
| 最近稳定 Tag `v0.4.0^{commit}` | `70d8644f9daeb7f19d29c40a16a41bfcf1de7cfb` | U 超前 43、落后 0 | 724/33/26 |
| 额外分支 `origin/v.0.5.0` | `6d59f7fa2b3e83a89e9450927b507320f44264f7` | 对 U 有 1 个独有提交；U 有 42 个独有提交 | 721/37/38 |

`origin/HEAD -> origin/main` 确认默认主线；原差异分析没有 fetch，使用当时本机快照。用户后续授权推送后，已通过 `git ls-remote --symref origin` 确认服务器 main、v.0.5.0 与4个 Tag 均与这些快照一致。后续提交只更新发布/分支记录和源码获取说明，不改产品或测试源码。`origin/v.0.5.0` 是分支，不是发布 Tag。v0.4.0 是 annotated tag，其 Tag 对象 SHA 为 `7742b564088503782003ce6ff245a5a360134623`，比较使用解引用后的 commit；其他 Tag 原样保留。

实际执行 `git diff --name-status/--shortstat BASE U`、`git rev-list --left-right --count BASE...U`、`git log BASE..U` 和源码检查。结构化列表见 [version-diff.json](version-diff.json)，不复制巨型 patch。U 对 origin/main：**411 文件，+36,280/-653 行**；U 对 v0.4.0：**783 文件，+84,884/-7,552 行**。其中源码/测试（cmd/internal/tests 下 .go/.py/.sh）相对 origin/main 为160文件 +17,220/-248行，相对 Tag 为291文件 +35,431/-6,809行。总体统计包含历史证据和冻结实验，不能视为全部核心功能增量。本次追加交付的变化另在该 JSON 的 handoff 字段列出，与已有升级统计区分。原整理交付提交 `2ca75c4e5ad41a8737fb03cee1ca53cfeceddda7` 的树统计：相对本地 main 为 24 files changed, 3025 insertions(+), 171 deletions(-)；相对 origin/main 为 430 files changed, 39182 insertions(+), 701 deletions(-)；相对稳定 Tag 为 797 files changed, 87783 insertions(+), 7597 deletions(-)。相对本地 main 只有整理/文档及测试修复，无产品代码增量。

## 架构、源码与依赖

- v0.4.0 已有原生 Agent 包装、Run/Resume、Jobs、YAML Workflow、配置和 MCP 管理；并非从零开始。R0 增强流解析、诊断、事件、文件/路径约束；R1 引入 `internal/task`；R2 加入 checkpoint/journal/owner/受监督 attempt 与恢复；R3 提供复现和安装/对照验收工具。
- R4 的主链为 `cmd -> task/workflow -> agent/runtime/runstore/worktree/jobs/config`。退役 install/auth/default/acp/route/workflow/usage 等管理命令，当前使用 Task。内部 workflow runner、模板、后台旧记录恢复依然保留。不存在当前公开的 `workflow run` 或独立 worktrees 管理命令。
- U 相比 origin/main 新增 `internal/launch`、`launchtest`、`localstate`，并新增/扩展 task/runtime/agent 等调用关联；R4.1 收敛原生进程启动、不可变预算窗口、崩溃保守记账和幂等。这里不是 Token/费用账本。本次只修复一个测试同步问题，不扩展产品预算功能。
- v0.4.0 的 `internal/routing`、`internal/usage` 和 MCP import/sync 实现随产品收口移除；新建 agentapi/agentgateway/agentservice/agentstore/agentpolicy/nativeadapter/codexrpc/isolatedruntime、model/modelgateway/modelbackend/modelstore、siwc/media 等包主要属于保留实验。不能将文件量等同正式功能量。
- Go：Tag 的 `go 1.26` 变为 `go 1.26.0`，本机验证 Go 1.26.4。cobra 1.10.1、yaml.v3 3.0.1、pflag 1.0.9、mousetrap 1.1.0 保持；移除 BurntSushi/toml 1.2.1 与 tailscale/hujson，新增 x/image 0.47.0 和 x/sys 0.49.0。U 与 origin/main 的 go.mod 完全一致。本轮未改依赖。

## 功能升级对照

下表“当前测试”指本轮离线自动测试；历史真实证据是指定时间/版本/账号/任务的观察，均不是本轮模型验证。所有本轮真实模型调用均 NOT_TESTED。

| 能力 | v0.4.0 能力及限制 | 当前实现及用户价值 | 相对 origin/main 的增量 | 当前测试 / 真实证据与限制 |
| --- | --- | --- | --- | --- |
| CLI 发现与管理 | 有 agents/doctor 及安装/认证管理 | 原生自管安装/登录；安装、版本、帮助、声明能力分开，避免诊断冒充账号可用 | R4 收口管理；R4.1 探针计启动 | Detect/Doctor、R4 auth_args 不执行、安装验收 PASS；本轮 PATH 发现三者，未启动真实探针 |
| Codex Adapter | exec/resume 与结构化解析已有 | 增量流、完整终态/usage、错误分类、Session 与启动关联 | 接入统一启动入口 | BuiltinRun/Resume、stream/R0、launch 测试 PASS；11-06 文本和续接历史通过 |
| Cursor Adapter | Tag 无内置 Cursor（可 generic） | cursor-agent --print/stream-json、--resume，Task 候选/复核 | R4.1 统一计启动 | NewAdapterCommandsAndResume、Task 四路径 PASS；10-19 有恢复历史证据，11-06 新临时项目 Trust BLOCKED |
| Antigravity Adapter | Tag 无内置 agy（可 generic） | print/stream-json、conversation；denied_actions 即使原生退出 0 也失败 | R4.1 统一计启动 | 权限拒绝全入口测试 PASS；11-06 文本/续接/图片历史通过；print 原生参数仍为 120s |
| CLI Run / JSON / Events | 单次执行与归一化结果已有 | 有界流、parse_status/error_kind、事件先于结束可见、真实退出与合成状态区分 | 启动账本、幂等、冻结边界 | Run/Events/BrokenStream/diagnostics 测试 PASS；本轮真实执行 NOT_TESTED |
| Task / Workflow | 有公开 YAML workflow，无 task | Work/Project、协调→执行→实际验证→复核，一次有界修复；交接材料和证据绑定 | Task 原已存在；R4 调整主入口 | FourPathsAndRepair、CLIJSONEventsAndStatus、workflow runner PASS；R1–R3 Codex/Cursor 有历史实测；复杂计划不保证成功 |
| Session / Resume | Run/native Session 与 workflow step 可续接 | 校验 Session、保留 cwd、从 Run/Task 材料恢复，避免盲重放 | R4.1 按相同窗口绑定续接 | Resume 测试、MissingNativeSessionRestoresMaterials PASS；交互 use 未必记录 Session ID |
| Jobs / 后台 | run/workflow detach、logs/stop/wait 已有 | Task 独立后台 owner/attempt、暂停/继续、崩溃识别；旧 Jobs 保留 | 共享启动窗口传递 | DetachedRun、TaskDetachedStop、OwnerKill、Jobs 回收保护 PASS；Jobs 没有 retry/resume 子命令，Windows 后台不支持 |
| 多 CLI 编排 | YAML 显式分步/并行 Agent | 自动 Task 计划、固定候选/绑定、不同 Adapter 才算跨 CLI 复核 | 不继续通用模型路由 | Task 模式/交接/材料与绑定测试、R3 集成 PASS；没有 worker/reviewer 固定角色参数，也未证明协作必然更省 |
| 工作区 | Tag 仅基础工作流 cwd | Project 要求干净已提交仓库，独立 worktree、基线 diff、未跟踪文件与验证日志 | R4.1 原生启动元数据关联 | worktree 路径/dirty/越界、Task 材料保护 PASS；worktree 不是 OS 沙箱 |
| 恢复与持续执行 | Jobs 可查状态，缺 Task checkpoint | Task v2 journal、输入更新、native/fresh-session、needs_reconcile，累计限制不归零 | R4.1 unknown 启动保守占位 | CheckpointContinue、Reconciliation、SimulatedRestart PASS；无法证明副作用时必须用户核对 |
| 结果交付 | Run 输出/日志、Workflow 结果 | 实际文件摘要/非空/来源、handoff、测试、review 与 snapshot；保留 worktree/diff | CLI-first 区分执行、文件和用户接收 | MissingArtifact、SourceAndEvidenceIntegrity、review 修改拒绝 PASS；不会自动 commit/merge/deploy |
| 媒体 | 提示词可请求原生工具，无图片 API | 原生图片仍为文件交付；历史 R0 三 CLI 生图/编辑、11-06 Codex/agy 再验 | 未新增通用图片功能；保留媒体校验实验 | 文件/真实格式/symlink Fake 测试 PASS；历史视觉结论可查，本轮未生图，视频/音乐模型生成未正式验收 |
| 诊断与错误 | 基础退出码/安装检查 | 网络/认证/额度/权限/截断/解析失败分类，不通过切账号或自动扩大权限规避 | frozen default closed、budget UNKNOWN/EXHAUSTED | diagnostics、R0 denied_actions、launch 边界 PASS；厂商协议变化仍可能影响解析 |
| MCP 兼容 | 管理/import/sync/serve | 仅 mcp serve 兼容既有 native 配置，project-config 查找与环境筛选 | R4.1 原生 launcher 也记启动 | MCPServeFindsProject/PassesThrough/Cancellation PASS；不提供新 MCP 管理同步/原生授权平台 |

具体统计和测试实例见 [自动验收](AUTOMATED_ACCEPTANCE.md)，原生历史范围见 [R0](../r0/media-acceptance.zh-CN.md)、[R3](../r3/compatibility.zh-CN.md)、[11-06](../cli-first/ACCEPTANCE_REPORT.md)、[R4 BLOCKED](../product-realignment/R4_REAL_ACCEPTANCE.md)。

## CLI 命令对照

| Tag v0.4.0 | 当前 --help/源码 | 迁移 |
| --- | --- | --- |
| agents、doctor、use、run、resume | 保留；加入 Cursor/agy、结构化诊断/事件及启动预算 | 首次执行先建本次窗口；原生自己维护账号 |
| runs list/show | 保留；Run 记录添加 parse/error/task/role/launch 等可选字段 | show 本身输出 JSON，没有 --json 参数 |
| jobs list/show/logs/wait/stop/delete/prune | 保留 | 无 jobs resume；有 Session 的 Run 用 resume，Task 用 continue |
| workflow、route、usage、install、auth、default、acp | 已从公开命令树移除 | Task 接替工作流；default_agent 写配置；登录回原生 CLI |
| MCP 管理命令 | 仅 mcp serve 保留 | 旧 launcher 继续；不新增/同步原生条目 |
| 无 task | task + status/stop/continue | Work/Project；支持 --detach、--events、--call-timeout |
| 无 launches | launches window create/budget/list/show/windows/probe/confirm-stopped/record-legacy | 控制直接原生进程数；并非模型请求或费用预算 |
| 无 model/agent-gateway | 代码注册但隐藏、默认冻结 | 不用于本地体验；不会按历史文档启用 |

## 冻结成果与默认行为

| 模块 | 源码/历史成果 | 状态与正常 CLI 的关系 |
| --- | --- | --- |
| Model Gateway M0–M5 | modelgateway/modelbackend/modelstore、协议、应用补丁/生命周期 | 保留实验；正常帮助隐藏；默认服务启动拒绝，不是 CLI 产品前提 |
| Native Agent Gateway G1–G7 | agent HTTP、持久任务、图片交付、limited/app-server/隔离 spike | 保留实验；默认禁用；隔离仅 partial，不能宣称原生 Profile 可用 |
| SIWC/OAuth/Responses G8 | siwc、离线 PKCE/Responses/SSE/Mock 合同 | 仅离线合同；PRODUCTION_BLOCKED，未使用生产凭据 |

`cmd/experimental.go` 在 handler 执行前拒绝服务启动/授权，状态/日志/停止等兼容收尾入口保留。Go 全量回归中的临时 Mock HTTP fixture 只是测试进程，不是启用正式服务；本轮未运行显式 opt-in 的旧 Gateway 生命周期脚本。

## 兼容与已知限制

配置仍为 version 1，YAML/JSON、XDG 和 --config 保留；旧 routing/auth_args/ACP 字段可读，但相应管理行为不恢复。Task v1/v2、Run/Jobs、旧 workflow、MCP ownership 和原生 Session 不进行破坏性迁移；旧 Task 恢复需显式确认材料路径。R4.1 为 Task/Run 增加可选 launch 关联和独立私有 ledger；**旧未关联/未知历史不自动转换成有余额窗口**。原生 Session/账本跨版本可能无法直接续接，应保留数据、先检查状态，再按当前提示处理；不要删除账本“修复”。

当前开发版已具备离线验证的本地体验入口，尚无本轮账号/模型/真实权限/真实取消复验；历史 R4 NEEDS_FIXES 不因本次 Fake 通过而升级为日用发布。Antigravity 内部 print 120 秒、Windows 后台不支持、原生版本协议风险、跨 CLI 收益未证实、复杂计划/角色固定参数不支持均保留。`origin/v.0.5.0` 中 runs/worktrees/filter/plan 等旁支实现不等于当前支持；不把它的独有提交删除或擅自恢复进当前产品。
