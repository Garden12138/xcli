**xcli 四个 tag 版本补充审查 · 2026-09-08**

本篇补充[全分支产品与代码审查](2026-09-07-product-and-code-review.zh-CN.md)，覆盖 `v0.1.0`、`v0.2.0`、`v0.3.0`、`v0.4.0` 的完整源码快照、相邻版本差异、发布说明和行为验证。定位依据仍是用户在[初衷对话](https://chatgpt.com/share/6a9e2083-3994-83ea-810a-321c4a209ace)中认可的“用于安装、运行、切换和编排主流 AI Coding Agent CLI 的统一入口”。市场对照沿用主报告 9 月 7 日核对的官方资料，本篇未重新评测竞品。

核心判断：四个版本形成了清晰的工程演进——基础入口、并行编排、配置与会话管理、后台任务管理。但首次成功、执行反馈、正确恢复和结果验收没有同步完善。建议以 v0.4 的任务管理基础继续迭代，先发布公共可靠性修复，再把 v0.5 的验证与工作区能力接入完整用户流程。

**范围和检查结果。** tag 使用其指向的 commit，避免把附注标签对象误当源码提交。通过 `git archive` 导出到临时目录，分别执行构建、静态检查及测试；未切换当前分支，未修改业务代码。

| tag | 源码 commit | 跟踪文件 | Go 文件 / 测试文件 | 构建 / vet / race 测试 | 总语句覆盖率 |
| --- | --- | ---: | ---: | --- | ---: |
| v0.1.0 | `a15a42ae20704d9c745b0d09ea901b9817e326ad` | 34 | 24 / 7 | 全部通过 | 44.2% |
| v0.2.0 | `ee98e9322e640d4eb0ed4b981c3f6c8e4a3d3b43` | 38 | 24 / 7 | 全部通过 | 51.6% |
| v0.3.0 | `0ba098a688bf691d5323cee188bcde9b6fb5d024` | 78 | 60 / 20 | 全部通过 | 67.4% |
| v0.4.0 | `70d8644f9daeb7f19d29c40a16a41bfcf1de7cfb` | 79 | 60 / 20 | 全部通过 | 67.7% |

检查命令为 `go build`、`go vet ./...`、`go test -race -coverprofile=... ./...`，环境为 macOS arm64、Go 1.26.4；覆盖率按 profile 的语句数加权。不同版本代码范围不同，覆盖率仅辅助判断测试投入，不能当作质量排名。

`v0.4.0` 与主报告的 main 提交 `feae607` 之间只有 `docs/manual-testing.zh-CN.md` 的变更，新增 110 行、删除 40 行，业务代码完全一致。`origin/v.0.5.0` 是后续开发分支，不能算作第五个已发布 tag。本篇针对固定快照，不声称审查之后远端不会发生变化。

行为验证使用模拟 Agent、临时项目和隔离的 XDG 配置/数据目录。每个 tag 都使用自己的二进制和示例文件；未调用真实模型、连接 MCP 服务或修改用户原生配置。所有“复现”均指本地夹具观察；没有将仓库测试通过视为最新原生 CLI 的端到端兼容证明。

**四个版本各自提供了什么产品价值。**

| 版本 | 实际能力变化 | 对初衷的贡献 | 体验、竞争力和吸引力判断 |
| --- | --- | --- | --- |
| v0.1 | 四类适配器、安装/认证、交互透传、单次运行、环境和网络 profile、串行工作流、私有记录 | 建立统一入口，保留原生工具行为 | 范围容易解释、依赖少；已有多 CLI 用户可受益于环境复用，但仅更换启动命令的吸引力有限；首次使用仍需手动配置，历史记录无法直接继续 |
| v0.2 | 可选并行度、依赖调度、模板引用推导依赖、失败取消、中文 README 和并行审查示例 | “编排”开始支持实用的并行审查与汇总 | 增量集中，默认串行保留旧行为；中文材料提高可接近性。仍需手写 YAML；共享目录并行写入需用户协调，审查权限没有由 xcli 统一约束 |
| v0.3 | 正则路由、usage、ACP、resume、后台 run、用户/项目级 MCP 同步与导入 | 环境复用和同一原生会话继续更接近真实日常需求 | 功能增长最大，适配与配置管理具备更多价值；首次接触的概念明显增加，MCP 与恢复路径存在缺陷，普通 run 的 stdout 反馈还出现倒退 |
| v0.4 | 后台 workflow、实时步骤状态、wait、delete/prune、匿名管道传参、未完成步骤状态整理 | 让长任务更容易管理和收尾 | 这是有价值的可靠性与可操作性投入；可以展示“离开终端后继续跑、回来查看并清理”。后台 restart/attach/resume、交互会话连续性、验证和接收结果仍有缺口 |

对应发布说明：[v0.1](https://github.com/Garden12138/xcli/blob/a15a42ae20704d9c745b0d09ea901b9817e326ad/.github/release-notes/v0.1.0.md)、[v0.2](https://github.com/Garden12138/xcli/blob/ee98e9322e640d4eb0ed4b981c3f6c8e4a3d3b43/.github/release-notes/v0.2.0.md)、[v0.3](https://github.com/Garden12138/xcli/blob/0ba098a688bf691d5323cee188bcde9b6fb5d024/.github/release-notes/v0.3.0.md)、[v0.4](https://github.com/Garden12138/xcli/blob/70d8644f9daeb7f19d29c40a16a41bfcf1de7cfb/.github/release-notes/v0.4.0.md)。表中的体验及市场吸引力判断属于产品分析，尚无真实留存或付费数据支持。

相邻差异也支持这个演进判断：v0.1→v0.2 为 14 个文件、+1,734/−104 行；v0.2→v0.3 为 57 个文件、+10,241/−114 行；v0.3→v0.4 为 14 个文件、+1,273/−138 行。行数包含文档和测试，用来说明变更范围。v0.2 的调度器测试投入明显，v0.4 则主要在已有模块内补任务生命周期；二者都值得沿用这种围绕具体用户场景的迭代方式。

**问题的首次出现版本与当前状态。** 编号沿用主报告。表中“无此功能”表示当时尚未引入相应路径；“复现”表示该 tag 二进制已独立验证。这里的“首次”指最早受影响的发布 tag。

| 问题 | v0.1 | v0.2 | v0.3 | v0.4 | 后续分支状态 |
| --- | --- | --- | --- | --- | --- |
| F01 worktree 步骤绝对 cwd 回到原仓库 | 无此功能 | 无此功能 | 无此功能 | 无此功能 | v0.5 分支新增问题 |
| F02 MCP 未识别外部 headers 改动，后续删除整个条目 | 无此功能 | 无此功能 | 复现 | 复现 | main、v0.5 均存在 |
| F03 MCP import 扩展 targets 时丢失已有 env | 无此功能 | 无此功能 | 复现 | 复现 | main、v0.5 均存在 |
| F04 workflow 子目录 session 恢复到顶层目录 | 无 resume | 无 resume | 复现 | 复现 | main、v0.5 均存在 |
| F05 大事件解析中止却报告成功 | 复现 | 复现 | 复现 | 复现 | main、v0.5 均存在 |
| F06 示例把运行目录设为 examples | 实现示例复现 | 两个示例复现 | 两个示例复现 | 两个示例复现 | 旧示例仍存在；v0.5 新验证示例目录正确 |
| F07 版本探测失败但 doctor 退出 0 | 复现 | 复现 | 复现 | 复现 | main、v0.5 均存在 |

F05 的版本差异需要特别说明：v0.1/v0.2 的复现入口是 `run codex test --json`，旧版本本来就没有 usage 功能；当时已会丢失大事件输出。v0.3 引入用量采集后，同样的截断还会丢失后续 usage，并把受影响路径扩大到内置 Agent 的普通 run。不能把“旧版缺少 usage 字段”单独算成缺陷。

各 tag 的最小观察结果如下，绝对临时路径已简写：

| 夹具与调用 | 观察结果 |
| --- | --- |
| 模拟 Codex 输出 session 开始事件 → 5 MiB 单行消息 → usage；调用 `run codex test --json` | 四版均退出 0、`status: success`，`output` 仅剩开始事件的 JSON；v0.3/v0.4 未采集到后续 usage |
| 模拟 Agent 的 `--version` 返回 9；调用 `doctor codex --json` | 四版均退出 0；JSON 同时有 `installed: true` 和 `error: version check exited with code 9` |
| 从另一个目录运行各 tag 自带 `examples/implement-and-review.yaml` | 四版执行记录和每个步骤实际 cwd 均为该 tag 的 `examples/`；并行示例从 v0.2 起同样复现 |
| workflow 总目录为 repo、步骤 `cwd: sub`；结束后 `resume <id> --step task continue --json` | v0.3/v0.4 原执行输出 `repo/sub`，恢复后输出 `repo` |
| 源 MCP wrapper 为 `targets: [claude]`、`env: {LOG_LEVEL: info}`；从 Gemini 导入同一 wrapper | v0.3/v0.4 均成功扩展至 `[claude, gemini]`，同时删除 env |
| 同步项目 Claude HTTP MCP → 原生条目增加 headers → plan → 删除源 server → sync | v0.3/v0.4 的 plan 均为 `action: noop`；最终原生 `mcpServers` 变成空对象 |
| 模拟交互 Agent 输出 session ID；调用 `use codex` 后读取运行记录 | 四版 `use` 记录都没有 session ID；v0.3 的 resume 新命令没有补上这条交互路径 |

源码可直接定位：[首版 Scanner](https://github.com/Garden12138/xcli/blob/a15a42ae20704d9c745b0d09ea901b9817e326ad/internal/agent/agent.go#L302)、[首版 doctor](https://github.com/Garden12138/xcli/blob/a15a42ae20704d9c745b0d09ea901b9817e326ad/cmd/agents_doctor.go#L72)、[v0.3 MCP 条目解析](https://github.com/Garden12138/xcli/blob/0ba098a688bf691d5323cee188bcde9b6fb5d024/internal/mcp/manager.go#L224)、[v0.3 env 复制](https://github.com/Garden12138/xcli/blob/0ba098a688bf691d5323cee188bcde9b6fb5d024/internal/mcp/import_engine.go#L231)、[v0.3 恢复目标](https://github.com/Garden12138/xcli/blob/0ba098a688bf691d5323cee188bcde9b6fb5d024/cmd/resume.go#L189)。修复方案和验收标准见主报告 F01–F07。

**新增发现：v0.3 改变了普通执行的反馈体验。** `db85e5c`（用量聚合提交，首次进入 v0.3）将内置 Agent 的普通 run 也切换为结构化执行。代码先累计 stdout，再在进程结束后解析并输出最终文本。这样能统一保存 session/usage，却没有同时提供人类可读的实时进度。[v0.3 选择结构化执行](https://github.com/Garden12138/xcli/blob/0ba098a688bf691d5323cee188bcde9b6fb5d024/cmd/run_use.go#L143)

用同一个模拟 Agent 验证：启动后立即 flush 第一条输出，等待 1.5 秒，再输出最终结果并退出。普通文本模式输出 `started`/`finished`；结构化模式发出对应 Codex 消息事件。仅测量 stdout，不把 stderr 的独立诊断行为混入结论。

| 版本 | 首个 stdout 字节出现时间 | 进程总时长 | xcli 最终展示 |
| --- | ---: | ---: | --- |
| v0.1 | 0.056 秒 | 1.576 秒 | 立即 started，随后 finished |
| v0.2 | 0.078 秒 | 1.593 秒 | 立即 started，随后 finished |
| v0.3 | 1.591 秒 | 1.597 秒 | 结束时才出现 finished |
| v0.4 | 1.580 秒 | 1.587 秒 | 结束时才出现 finished |

这是单次受控行为实验，用来证明输出时机变化；毫秒差异不代表性能排名，也不能据此推断真实模型响应时间。原生 stderr 仍可能实时显示，不能笼统描述为所有输出都被冻结。

建议作为 P2 体验修复：增量解析事件、实时展示活动和耗时，同时保留最终规范化结果与 usage。面向脚本保留现有 `--json` 单对象契约，可新增明确的 JSONL 事件模式。验收应包括“收到可展示事件后及时更新”“等待输入可识别”“大事件解析失败不会被掩盖”，让统计与执行反馈共同成立。

**已修复的历史问题也需要记入版本评价。** v0.3 启动后台任务时，把 Base64 编码后的 prompt 和原生参数放入 xcli worker argv。编码可以还原原文，会在允许观察进程参数的环境中暴露这份副本。v0.4 改为匿名管道传递 payload，worker argv 只包含配置路径、内部子命令和 run ID。[v0.3 传参实现](https://github.com/Garden12138/xcli/blob/0ba098a688bf691d5323cee188bcde9b6fb5d024/cmd/run_exec.go#L117)、[v0.4 管道实现](https://github.com/Garden12138/xcli/blob/70d8644f9daeb7f19d29c40a16a41bfcf1de7cfb/cmd/run_exec.go#L135)

该改进由源码差异及 v0.4 已通过的后台生命周期测试支持；本次未读取系统进程表。它解决的是 xcli worker 的传参副本。原生 Agent 自身仍可能通过 argv 接收 prompt，用户也可能将敏感内容输入 shell，不能据此承诺所有进程参数或日志都不含提示词。[相关测试](https://github.com/Garden12138/xcli/blob/70d8644f9daeb7f19d29c40a16a41bfcf1de7cfb/cmd/jobs_test.go#L52)

v0.4 的另外几项变化同样值得保留：workflow 步骤执行中持久化状态；wait 返回任务退出码，而等待超时/中断不会停止任务；清理要求显式选择终态任务和时间范围。这些都直接降低长任务的管理成本，但并不修复 MCP、解析或步骤 cwd 等独立问题。

**版本兼容与分发也需要产品化。** 四版的配置/工作流格式都使用 `version: 1`，这不表示新版字段能被旧二进制理解。实测同一份 v0.2 `parallel-review.yaml`：v0.1 校验失败，报 `field max_parallel not found`；v0.2 和 v0.4 校验通过。严格字段校验是有用的保护，应在模板和说明中另列“最低 xcli 版本”，遇到新增字段给出具体升级提示。[v0.2 格式和严格解析](https://github.com/Garden12138/xcli/blob/ee98e9322e640d4eb0ed4b981c3f6c8e4a3d3b43/internal/workflow/workflow.go#L17)

历史记录的可读取性也不等于可恢复性。没有保存原生 session ID 或步骤实际 cwd 的旧记录，升级不会自动补全；恢复时应显示缺少哪些信息，允许用户显式指定目录或从原生 session 索引选择。补充字段时必须明确旧记录的回退行为。

四个 tag 的 `.github/workflows` 与 `.goreleaser.yaml` 没有版本间变化：发布工作流核对 release notes 后运行 GoReleaser，未在这个发布 job 内先执行完整测试。已提供 macOS/Linux 双架构打包配置和校验和，但本次没有下载历史发布归档复验。建议让发布依赖同一提交的检查结果，再验证干净环境安装、PATH、版本号和一个最小运行。[发布工作流](https://github.com/Garden12138/xcli/blob/70d8644f9daeb7f19d29c40a16a41bfcf1de7cfb/.github/workflows/release.yml)

v0.1/v0.2 源码树没有 `LICENSE` 文件，v0.3 起加入 MIT LICENSE；中文 README 则从 v0.2 起提供。对于准备采用或贡献的用户，版本页应让这些信息容易找到；旧版文档也应给出当前维护版本的入口和已知限制。

**结合历史后的优化顺序。**

| 优先顺序 | 具体交付 | 对用户与产品的收益 | 验收方式 |
| --- | --- | --- | --- |
| 1：公共可靠性修复 | 先修 F02–F07，统一解析状态、完整 MCP 漂移保护和步骤实际 cwd | 已发布用户获得直接收益，为配置管理与会话连续性建立信任 | 本篇适用夹具转为跨模块回归并全部通过；在真实 CLI 固定版本执行关键 smoke 路径 |
| 2：日常使用体验 | 发现已有 Agent、轻量默认选择、实时活动、run ID、按项目找回最近任务 | 降低首次使用门槛，让“统一入口”每天有价值 | 用户能独立开始一次任务，识别其状态，隔天从正确目录继续 |
| 3：v0.5 发布条件 | 先修 F01，再连通 worktree、确定性测试、失败修复、结果 diff 与接收操作 | 把已有编排转为可检查的工作成果，提高展示吸引力 | 原仓库不被错误步骤目录写入；测试失败可进入有界修复；用户看得见并能接受结果 |
| 4：长期竞争力 | 适配器版本矩阵、真实脱敏事件样本、三类经验证模板、明确的兼容与支持策略 | 提高上游升级后的可靠性，积累用户可依赖的资产 | 每次升级能说明哪些原生版本、平台与关键路径已经验证 |

维护策略建议集中在一条稳定版本线和下一版本开发线；对四个旧 tag 保留可追溯说明，按实际支持承诺决定是否回补，避免默认承担四条长期维护线。公共修复应同步进入 v0.5，发布说明列出受影响的旧版本与修复行为。

对外演示也应按真实收益选择：先展示已有 CLI 的快速接入和正确恢复，再展示后台长任务，最后展示实现、测试与交叉审查。ACP、正则路由、token 表和 YAML 参数放入各自专题。四个版本的投入已经提供了这条路径的大部分基础；下一阶段应让用户在一个任务里连续体验到这些收益。
