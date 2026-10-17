# R1 实施与验收记录 · 2026-10-16

**R1 开发及四条真实 Task 路径验收已完成，里程碑退出条件已满足。** 当前源码构建为 `0.5.0-r1-dev`，完整自动化回归和四平台构建通过。真实验收范围为 macOS arm64 上的 Codex 指定模式及 Codex/Cursor 自动分工模式。本记录覆盖源码开发及验收；未打 tag 或发布安装包。

实施基线为 R0 提交 `5a2a676`，在现有工作目录中开发。最终生产源码摘要、Go 版本和本机二进制指纹见 [validation-reconnect.json](evidence/validation-reconnect.json)。已更新仓库本地 `./xcli`，可直接查看 `./xcli task --help`。

## 最终真实验收

四条路径使用同一个最终二进制，SHA-256 为 `29ffa9a5f381cc8d54aaeddc8be6b60811c7aae0c95566e341e0d67e7e82fcc2`。最终四条路径使用 16 次 Agent CLI 调用，连同所有历史失败及定向诊断累计 **52/60 次**，未超过用户批准的上限。汇总与独立核对见 [final-four-paths.json](evidence/final-four-paths.json) 和 [调用账本](evidence/native-summary.json)。

| 路径 | 实际参与 | 调用数 | 结果 |
| --- | --- | --- | --- |
| Work 指定单 CLI | Codex 协调、产出、自查 | 3 | 通过；产物内容、算术、来源引用、交接和输入完整性符合要求 |
| Work 自动分工 | Codex 协调、产出；Cursor 独立复核 | 3 | 通过；包含两次真实 TLS 断流重连后的正常恢复，通知次数保留 |
| Project 指定单 CLI | Codex 协调、产出、修复、自查 | 5 | 通过；首次验证失败保留，一次修复后重验及最终自查通过 |
| Project 自动分工 | Codex 协调、产出、修复；Cursor 独立复核 | 5 | 通过；首次验证失败保留，一次修复后重验及跨 CLI 复核通过 |

Work 单 CLI 的原始脱敏记录见 [单路径报告](evidence/native-reconnect-work-single-20261016.json)，其余三条见 [最终批次报告](evidence/native-reconnect-remaining-20261016.json)。Project 使用原有测试文件中的一次性失败注入，运行时真实执行 `go test -count=1 ./...`，保留退出 1 的初始日志，经过修复步骤后得到退出 0；没有删除或改写测试。独立核对确认原仓库及受保护文件未变、当前文件内容和权限重建的快照正确、最终复核绑定当前快照、交接确实消费，以及 CLI 结果、持久 Task、Run 和单调事件一致。该核对没有新增模型调用。

最终真实调用使用 `--codex-reasoning-effort low --record-native-output`。low 通过单次原生 `-c` 参数传入，验收脚本未写入原生持久配置或更换登录入口；Task 的 10 分钟总时限、单次 120 秒限制、单次修复及验收标准保持原样。原始流只保留在私有临时目录，公开报告不包含任意工具载荷或原生 Session ID。

## 已交付

- Task/start 与 status 入口；Work/Project 两种任务，指定单 CLI 与自动分工共享模型、执行、事件和存储。
- 模式锁定、稳定候选顺序与能力过滤；协调者生成结构化动作，运行时检查角色、目录、命令和预算。指定模式不隐式换 CLI，跨 CLI 复核要求不同 Adapter。
- 复用 workflow 的步骤执行和现有 runtime；串行写入、受管理 worktree、真实 argv 验证、一次修复及重验，保留失败证据。
- Task/Run/Session 身份分离、计划版本、交接引用、实际 cwd、产物与输入摘要、单调事件序号、用量及停止原因。私有原子存储，原始 Agent 输出仍默认关闭。
- 固定复核包包含源码/产物、选定输入、实际验证日志及基线 diff；旧失败和当前结果分别保留。验证改动源码使证据过期，副本变化拒绝复核结果。Project 无真实验证进入 waiting_user。
- 退役 install/auth/default/acp/route/workflow/usage 和 MCP import/plan/sync，清理仅服务旧管理功能的实现及依赖。保留 mcp serve、底层 run/use/resume、jobs、runs、发现及配置；不删除用户配置、ownership、原生文件或历史 Run。
- 更新中英文 README、[R1 使用与迁移说明](README.zh-CN.md)，标明旧手工指南及计划中的历史状态。R2 的 Task 后台、stop/continue、检查点和失活恢复未混入本阶段。

## 工程验证

| 检查 | 最终结果 |
| --- | --- |
| `go test -race ./...` | 全部通过 |
| `go vet ./...`、`go mod tidy -diff` | 通过；移除退役 MCP 管理代码的 TOML/JSONC 依赖 |
| gofmt 与 `git diff --check` | 通过 |
| Darwin/Linux × amd64/arm64，CGO=0 构建 | 全部通过 |
| 四平台临时 tar.gz、独立 SHA-256 校验、归档内容/执行权限/二进制一致性 | 全部通过；本机 arm64 `--version` 正确 |
| GoReleaser snapshot | 本机未运行，工具未安装；现有固定版本 CI 门禁保留，未将手工归档冒充 GoReleaser snapshot |

最终四平台证据见 [build-and-archive-reconnect.json](evidence/build-and-archive-reconnect.json)；较早的 [构建记录](evidence/build-and-archive.json)与 [上一候选记录](evidence/build-and-archive-final.json)保留，不替代最终记录。跨平台编译不代表目标机器上的原生 CLI 已实测。

进程级夹具覆盖四条路径、真实 argv/cwd、原生解析、交接消费、一次修复和修复耗尽。故障回归包含模式逃逸、缺测试/缺候选、坏计划/坏流、预算、取消、存储及事件输出失败、路径越界、符号链接材料、来源变更、测试导致证据过期、复核写入，以及交接中“尚待验证”的历史说明不能覆盖后续真实通过证据。保留入口的 Run/resume/jobs 和 MCP launcher 回归继续通过。

## 真实调用与发现

沿用默认原生登录环境，真实工具版本为 Codex `0.162.1`、Cursor `2026.10.14-0f9edde`。材料、Git 项目与 XDG 数据均为本轮创建的临时目录；Cursor trust 只针对生成的测试目录。验收脚本没有执行登录/退出、切换账号、复制凭据、编辑长期权限或外部发布。

| 批次 | 调用数 | 结果与证据 |
| --- | --- | --- |
| 首轮四条路径 | 14 | 均未达到完整通过；见 [native-20261016.json](evidence/native-20261016.json) |
| Work 协议约束复验 | 2 | 协调计划被校验拒绝，未启动 worker；见 [native-work-fixed-20261016.json](evidence/native-work-fixed-20261016.json) |
| Work 单 CLI / Project 跨 CLI | 6 | Work 单 CLI 完整通过；Project 在 Cursor 复核协议解析处停止；见 [native-final-partial-20261016.json](evidence/native-final-partial-20261016.json) |
| Cursor 回答选择定向复验 | 1 | 首次仍失败，见 [诊断记录](evidence/cursor-final-message-smoke.json) |
| 最终 Cursor 回答选择定向复验 | 1 | 通过：原生 success/complete，提取的复核 JSON 正确报告已有验证失败；见 [最终记录](evidence/cursor-final-message-smoke-final.json) |
| 上一候选四路径重验 | 7 | Work 两条分别在 worker/coordinator 原生错误事件停止，Project 两条分别在 reviewer/coordinator 达到 120 秒超时；见 [记录](evidence/native-final-20261016.json) |
| low 参数 Work 两路径 | 5 | 单 CLI 在重连通知误判处失败；自动分工完整通过；见 [记录](evidence/native-work-low-20261016.json) |
| 重连修复后 Work 单 CLI | 3 | 完整通过：当前快照复核、来源、产物、交接消费及存储一致；见 [记录](evidence/native-reconnect-work-single-20261016.json) |
| 重连修复后其余三条路径 | 13 | 全部通过：Work 自动、Project 单 CLI、Project 自动；见 [记录](evidence/native-reconnect-remaining-20261016.json) |

原定上限为 24 次，用户先后批准提高至 40 次、60 次；全部批次累计 **52 次 Agent CLI 执行轮次**，见 [汇总](evidence/native-summary.json)。最终四条完整 Task 复验使用的二进制与最终 Darwin arm64 归档完全一致。只读版本/help 探测不算模型执行轮次；厂商内部模型与工具往返不可完整观测，不宣称内部请求数同样受这一计数限制。失败的真实调用与后续修复均独立保留，没有改写为通过。

实际发现及修复：

1. Work 中正确的相对文件来源被当成缺失绝对引用。现在接受从产物位置可追溯到已声明输入的相对或绝对引用，并校验输入内容未变。
2. 初始复核包缺少实际验证日志和基线 diff，导致模型只能看到状态摘要。现在复制必要证据到固定复核材料中，明确 Git 元数据/忽略的运行标记不属于源码快照。
3. 原生严格 JSON schema 让不适用的动作字段也必须填写，协调者为 delegate 填入 cwd 后被运行时拒绝。现在按动作类型生成联合 schema；运行时仍拒绝越权字段。
4. Cursor 先发送活动说明，再发送完整 JSON 回答；适配器原先拼接了两条 assistant 消息。现在选择最后一条完整 assistant 回答；当 result.result 再次聚合了前面的活动说明且以该回答结尾时，保留完整回答，不重新混入活动文本。独立的终结回答仍保留回退，终结状态与错误检查保持有效。模拟 race 回归与最终 Cursor 在线定向复验通过。
5. Worker 的“验证尚待运行”是交接时的历史事实。最终判定使用实际命令和当前复核，不让这类历史说明覆盖后续通过证据；新增回归验证。
6. 最终 Work 重验捕获 Codex 原始事件：`Reconnecting... 2/5 (request timed out)`、`3/5` 后出现正常 `turn.completed` 且原生退出 0。Codex exec 0.162.1 的[官方实现](https://github.com/openai/codex/blob/rust-v0.162.1/codex-rs/exec/src/event_processor_with_jsonl_output.rs)将 app-server Error notification 映射为只含 message 的顶层 error，未保留 willRetry。现在只识别完成之前格式合法的重连通知；未知错误、明确不可重试/错误标志、完成后的错误、turn.failed、坏流、缺完成和非零退出仍失败。重连通知次数记录在 Task Attempt、Run 和底层 Step 中，回归及完整 race 通过。脱敏事件证据见 [codex-reconnect-diagnostic.json](evidence/codex-reconnect-diagnostic.json)，该诊断复用已有流，不新增模型调用。

首轮缺乏完整错误流的调用仍保留原失败结果；后续捕获重连通知不能反推所有历史错误均为同一原因。Cursor 与 Codex 的 120 秒超时记录也保留。

产品仍默认关闭原始输出记录。首轮后续 Project 自动场景和最后几批诊断/验收通过临时配置显式启用记录，日志留在私有临时数据目录，没有写入公开证据。协议拒绝保存必要的最终角色响应与有界诊断，用于排障，不广播任意工具载荷。

## 验收范围与边界

R1 的 Work/Project × 指定/自动四条退出路径已在最终候选完整通过；旧候选及 Adapter 定向诊断的在线证据仅保留各自范围，没有替代最终复验。测试中的成功是本轮明确目标和材料的结果，不承诺任意项目、服务延迟或原生环境始终成功；超时、坏流与真实失败仍按失败保存。

Antigravity、Claude、Gemini、OpenCode 的 R1 Task 真实路径，以及非 macOS 实机仍未验证。现有 Adapter/模拟回归与 R0 真实记录不能替代这些新路径。R1 不承诺通用 OS 沙箱；native role 参数的实际能力仍须按版本和路径登记。

本轮已完成 R1 开发与约定的真实验收，发布是独立步骤，本批未执行。Task 的 waiting_user 当前只保存缺失条件；同 Task 继续、后台及异常恢复按 R2 开发，不通过临时状态改写冒充恢复能力。
