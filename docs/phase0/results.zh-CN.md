# 阶段 0 验收记录 · 2026-09-08

本轮只增加准备文档、兼容样例和独立验证工具。生产命令、解析器、MCP 写入和运行记录行为没有修改；F01–F07 仍待阶段 1 修复。

## M0 结论

**A01、A02、A03 完成，可以开始分模块实施。** 已固定两个源码快照、登记七项缺陷、冻结三类核心兼容边界，并建立可重复的本地夹具和真实验证依赖。真实 Codex 首轮 smoke 因账号配额耗尽失败；后续最小任务恢复成功，用户追加授权两次调用后，2026-09-08 16:02（Asia/Shanghai）的普通 run/JSON → 同 session resume 双轮验证已通过。M0 已达到；其他 CLI/平台、取消、MCP 和阶段 1 修复的发布验收仍需单独完成。

| 任务 | 状态 | 证据 |
| --- | --- | --- |
| A01 | 完成 | baseline 文档的完整 SHA、影响版本、目标模块、责任归属和开发线拆分 |
| A02 | 完成 | contracts 文档、testdata/contracts.json；旧 Record/RunResult/ownership 读取样例测试通过 |
| A03 | 完成 | 两次独立模拟报告、真实 run/JSON → resume 通过证据及先前失败记录、验证矩阵与后续测量清单 |

## 模拟复现

平台 darwin/arm64，Go 1.26.4；稳定线 feae607 与开发线 6d59f7f 均由 git archive 的源码构建。执行工具入口见 validation 文档；所有原生 Agent 都是模拟进程。本次没有重测四个历史 tag，历史影响范围仍引用审查证据。

| 场景 | 稳定线 | 开发线 | 实际观察 |
| --- | --- | --- | --- |
| F01 | 不适用 | 已复现 | worktree 模式步骤进入临时源仓库并写入标记 |
| F02 | 已复现 | 已复现 | headers 漂移后仍 noop；移除源声明后删除外部编辑条目 |
| F03 | 已复现 | 已复现 | targets 扩展为 claude/gemini，env 丢失；二次导入不再变化 |
| F04 | 已复现 | 已复现 | 初次进入 repo/sub，恢复进入 repo |
| F05 | 已复现 | 已复现 | 5 MiB 输出只剩 54 字节 session JSON，usage 丢失，仍 success/退出 0 |
| F06 | 已复现 | 已复现 | 两份示例从两个调用目录运行，全部步骤仍进入 examples |
| F07 | 已复现 | 已复现 | --version 退出 9，doctor 文本/JSON 都退出 0 |

13 个适用案例独立运行两遍，状态一致，均为 reproduced；执行异常为 0。初期夹具的 launcher PATH 与适配器安装识别问题已修正，最终报告不混入夹具调试失败。

原始脱敏证据：[第一次](evidence/simulation-1.json)、[第二次](evidence/simulation-2.json)。各报告含时间、二进制摘要、完整调用参数、退出码与输出；目录占位符代表每次新建的不同临时目录。F06 的多次调用仍算一个审查案例；不能把 13 个案例解释成所有后续回归矩阵已经覆盖。

## 真实 CLI 与外部依赖

| CLI / 版本 / 模型 | 平台 | 来源 | run / JSON | resume / cwd | 取消 / MCP | 预算及结果 |
| --- | --- | --- | --- | --- | --- | --- |
| Codex 0.153.4 / 本机默认模型 | darwin/arm64 | real | 通过：普通 run/JSON 退出 0，输出符合预期 | 通过：同 session、上下文记忆、原生 cwd 与记录关联均正确 | 未验证 | 原预算 2 次与追加授权 2 次均已使用，累计 4 次，无自动重试 |
| Claude / Gemini / OpenCode，版本待盘点 | darwin/arm64 | unverified | 未验证 | 未验证 | 未验证 | PATH 未发现；预算 0 |
| 四类 CLI，版本待盘点 | darwin/amd64、linux/amd64、linux/arm64 | unverified | 未验证 | 未验证 | 未验证 | 需要对应机器、可用账号与明确预算 |

真实调用约 6.1 秒后结束，未超时。原生 login status 探测成功只表明存在登录，不能证明可用配额。随后只读检查该次 smoke 的原生 session，确认 `codex_error_info=usage_limit_exceeded`，未再次发起模型调用。

证据：[smoke 原报告](evidence/codex-smoke.json)、[同次原生 session 的补充检查](evidence/codex-smoke-inspection.json)。原报告的 same_session/native_cwd_correct/records_linked 为 false，是首轮失败后未执行双轮验收的默认值，不代表观察到了会话或目录错误。该次补充检查只确认首轮 cwd；后来的双轮验证另存证据，不覆盖历史失败。原生 raw stderr 和任意模型输出没有提交。

首轮采集时工具写报告后仍返回 0，报告内的真实调用退出码为 1、status 为 failed。验收时已修正工具自身退出语义：真实模式 failed/blocked 返回 1，并以不启动模型的缺失可执行文件场景验证。没有为验证这一工具调整再次调用真实 Codex，原始报告保留其采集结果。

### 后续恢复检查 · 14:18 Asia/Shanghai

用户请求先处理额度失败后，先使用[官方 app-server 的 account/rateLimits/read 接口](https://learn.chatgpt.com/zh-Hans/docs/app-server#6-速率限制chatgpt)只读查询额度，不启动模型任务。返回的 codex 限额桶显示 5 小时窗口已用 68%、周窗口已用 11%，rateLimitReachedType 为 null；没有返回旧错误涉及的 premium 桶，因此不能只据查询宣称所有模型额度恢复。见[额度快照](evidence/codex-quota-check.json)。

随后使用原预算剩余的 1 次调用，沿用同一本机登录和默认模型、同一稳定线二进制，在新的临时仓库执行一次最小 run --json：约 8.9 秒，exit_code=0、status=success，得到 PHASE0_READY、非空 session ID 和 usage；保存的 record.cwd 与临时仓库一致。见[恢复验证证据](evidence/codex-recovery-check.json)。没有修改生产代码、原生认证、模型选择或订阅；这次恢复由实际执行成功确认，不声称手动重置了服务端额度。

初轮调用实际模型由原生 session 确认为 gpt-6-astra；本次继续使用 CLI 默认配置，不覆盖模型。截至这次检查，累计使用原预算 2 次，未运行需要额外调用的 resume。该检查只证明当次最小 run/JSON 可用，不把它当成两轮完整 smoke 通过。

### 追加授权后的双轮验证 · 16:02 Asia/Shanghai

用户明确允许新增最多两次真实调用后，执行已准备好的 tests/phase0 工具，使用新的临时仓库和输出报告，沿用现有登录与默认模型。每步限时 120 秒，实际执行两次，没有失败或重试。工具退出 0，报告 status=passed。原生 CLI 版本为 0.153.4，测试平台为 darwin/arm64，xcli 稳定基线仍为 feae607。

| 验收项 | 观察结果 |
| --- | --- |
| run --json | 约 9.8 秒，exit_code=0、status=success，输出 PHASE0_READY，保存 session 和 usage |
| resume 保存的 run ID --json | 约 10.4 秒，exit_code=0、status=success，输出上一轮要求记住的 PHASE0_MEMORY_7Q2；第二个 prompt 没有提供答案 |
| 同一原生会话 | 两次 session ID 一致 |
| 实际目录 | 原生进程执行入口独立采集的两个 cwd 均为本次临时仓库；resume 未指定 --cwd 覆盖 |
| 记录关联 | 新记录 resumed_from 指向首轮保存的 run ID，cwd 与 session 字段匹配 |

完整脱敏证据见 [codex-run-resume.json](evidence/codex-run-resume.json)。本轮消耗新增的 2 次授权，累计使用 4 次（初轮额度失败 1 次、恢复检查 1 次、本轮 2 次）。这些调用均未要求修改项目文件，测试结束后删除夹具目录；没有修改用户认证、订阅或模型配置。

本次补齐的是普通 run 记录的原生恢复路径。workflow 子目录恢复的 F04、取消、MCP、后台和其他平台未由这两次调用验证；阶段 1 仍按原缺陷清单实施，不将本次通过视为 F04 修复。

| 发布前依赖 | 负责人 | 解除条件 | 影响 |
| --- | --- | --- | --- |
| Codex 其余真实兼容路径 | 实施者确定场景，账号持有人确认后续预算 | 本机普通 run/JSON/resume 已通过；阶段 1 修复后另验取消、MCP、workflow 子目录与后台等路径 | 仅当前已测路径具备证据，B07/M1 发布验收仍需补齐 |
| 其他 CLI/平台 | 实施者组织测试环境，账号持有人提供预算 | 固定 CLI 版本、平台并填写 run/JSON/resume/取消/MCP 实测结果 | 发布前补齐，或明确收窄支持声明 |

M0 以后沿用这些依赖，不能因本次用户说“继续”就自动重试真实调用或扩大预算。

## 仓库检查

| 检查 | 结果 |
| --- | --- |
| go mod tidy -diff | 通过；go.mod/go.sum 无差异 |
| 只读 gofmt 检查、git diff --check、文档链接与 JSON 解析检查 | 通过 |
| go vet ./... | 通过 |
| go test -race ./... | 全部通过，含 tests/phase0；最后的工具退出语义调整另经 go test -race ./tests/phase0 和 go vet ./tests/phase0 复验；不调用真实模型 |
| CGO_ENABLED=0 go build -trimpath，darwin/amd64 | 通过 |
| 同上，darwin/arm64 | 通过 |
| 同上，linux/amd64 | 通过 |
| 同上，linux/arm64 | 通过 |

构建只证明目标二进制可编译，不证明它在目标机器上的真实 CLI 行为。网络和 Go 缓存受沙箱限制的检查在获得执行权限后完成；没有因此变更项目依赖。本轮不发布、不运行 GoReleaser release；归档、安装与同 tag 发布门禁仍由 B07 验收。

下一项实施任务为 **B01 / F03**，先修复 import 深拷贝并增加字段保留、输入不变和幂等回归；之后按 B02–B05 推进公共修复，B06 留在开发线处理。
