# chat-insight 分阶段真实体验与修复记录 · 2026-10-24

状态：**本轮约定案例已全部执行完成：R0 用户路径、R1 四条 Task 路径、R2 实际中断/材料恢复/追加需求、R3 原生对照均已完成。** 来源为用户真实终端反馈，以及用户随后明确授权助手修复并执行的本机原生 CLI 案例。用户自行操作与助手代为执行分别记录，不将本次自动执行计作独立用户观察。机器可读结果见 [本轮证据](evidence/chat-insight-experience-20261024.json)，额度恢复前结果另行保留。

19:12 用户要求继续后，沿同一原生入口恢复。原 session 的第 6 次调用出现连续连接重试，约 685 秒后显式 stop，Task 正确进入 paused。核对保留报告后，以 fresh-session 从材料继续，并补充每份文档约 4000 汉字、使用简明表格的篇幅要求，原事实覆盖与引用要求保留。[额度恢复前证据](evidence/chat-insight-experience-20261024-before-reset.json)和下文第一批结果保留所有失败。

恢复后已确认：指定 Codex 的组件 Task 在原 ID 中 completed，累计 5 次调用；test/lint/build 和 self_check 通过，独立 6 项行为检查通过。指定 Codex 的 Work Task 在原 ID 中 completed，累计 11 次调用；首轮自查指出报告错误地将 README 的年度数据说明转述为“只发送数字”，实际原文为“统计数字和聊天对象名称”，经一次修复后再次自查通过。两轮 review、各自材料快照及首次问题均保留。

本轮延续 [2026-10-15 的 R0 用户体验](../r0/user-experience.zh-CN.md)，全部使用真实 chat-insight 源码。项目基线为 `b20ad1e276f3b5566585be50f96549171dbf107c`，xcli 修复基线为 `c712f35`。原项目的未跟踪导览与视频保留；代码任务在该提交的独立副本上创建隔离 worktree。准备副本的 13 个前端测试文件、53 个测试、lint 和构建已通过。

## 最终结果与产物

下列调用数包含同一 Task 先前的失败、超时及取消；耗时为累计实际执行时间，暂停和等待额度不计入。Task completed 表示本轮声明的产物、验证和 review 通过，用户接收仍单独决定。

| 阶段 | 最终状态 / 累计调用 | 验证与结果 |
| --- | --- | --- |
| R0 | 两次用户 Run 均 success | 同一个原生 session，当前项目阅读与续聊已核对 |
| R1 Work / 指定 | completed，11 次，2671.473 秒 | 两份文档；首轮 self_check 发现年度报告转述错误，一次修复后再次通过 |
| R1 Work / 自动 | completed，4 次，850.154 秒 | Codex 执行、Cursor 的 cross_cli 复核通过，无新增 findings |
| R1 Project / 指定 | completed，5 次，359.822 秒 | 67 个前端测试、lint、build、self_check 通过；独立行为检查 6 项通过 |
| R1 Project / 自动 | completed，4 次，262.386 秒 | 66 个前端测试、lint、build、Cursor cross_cli 复核通过；独立行为检查 6 项通过 |
| R2 | completed，8 次，698.385 秒 | 实际 worker canceled → paused → 新 session 材料恢复 → 追加需求；最终 126 个前端测试、lint、build 和 self_check 通过；独立检查 8 项通过 |
| R3 原生 Codex | 本次正常退出 0，676.150 秒 | 一次直接执行交付两份文档，JSONL 含 turn.completed；另保留首次额度失败的一次调用 |

文档产物：

| 执行方式 | 项目体验评审 | 验收清单 |
| --- | --- | --- |
| Task 指定 Codex | [onboarding-review.single.md](artifacts/chat-insight-20261024/onboarding-review.single.md) | [acceptance-checklist.single.md](artifacts/chat-insight-20261024/acceptance-checklist.single.md) |
| Task 自动跨 CLI | [onboarding-review.cross.md](artifacts/chat-insight-20261024/onboarding-review.cross.md) | [acceptance-checklist.cross.md](artifacts/chat-insight-20261024/acceptance-checklist.cross.md) |
| 原生 Codex 对照 | [onboarding-review.native.md](artifacts/chat-insight-20261024/onboarding-review.native.md) | [acceptance-checklist.native.md](artifacts/chat-insight-20261024/acceptance-checklist.native.md) |

代码产物均以同一基线生成，作为三份可独立审阅的候选：

- [指定 Codex 的组件补丁](artifacts/chat-insight-20261024/r1-project-single.patch)：增加 aria-pressed，保留点击、受控属性与日期回调。
- [跨 CLI 复核的组件补丁](artifacts/chat-insight-20261024/r1-project-cross.patch)：组件实现与指定组的 SHA-256 相同，测试内容有差别；没有以更多 CLI 或测试条数宣称质量更高。
- [后台恢复及追加需求后的完整补丁](artifacts/chat-insight-20261024/r2.patch)：进一步增加日期端点 min/max 与空值约束测试。三份补丁是同基线的不同候选，按需要选择接收。

原 chat-insight 的 HEAD 仍为 b20ad1e，状态仍只有原来的两项未跟踪文件；来源副本保持干净。所有改动均在隔离工作区，补丁已在干净副本通过 git apply --check。

## R2 过程与连续性证据

Task 始终为 `task-20261024T075150Z-dcc59341`，指定 Codex，候选池、登录绑定及 worktree 保持一致：

1. 后台恢复协调后，观察到实际 worker 的原生进度，再请求 stop；对应 Run 保存 canceled，Task 保存 paused，当时累计调用数为 3。
2. 另一 CLI 观察进程保存了 47 条单调序号事件。检查保留材料和变更范围后，使用 `--reconcile --fresh-session --detach` 从材料继续。
3. 恢复后的成功 worker 使用与中断 worker 不同的原生 session；Task 在累计 5 次调用时 completed，真实验证包含 81 个前端测试、lint 与 build。
4. 同一 Task 再接收“检查自定义与预设切换回调兼容”的补充，产生第二版计划；再次验证与自查后在累计 8 次调用时 completed，前端测试扩展为 126 个。新需求没有清零已用预算。

对已完成 Task 提交后台补充时，提交返回的快照可能仍显示上个目标的 completed，同时 owner_active=true；应继续查询后续事件和 owner 状态。本次确实观察到 planning → worker → review → completed，而非把提交成功当成补充目标已经完成。

## R3 对照观察

三组报告都覆盖了浏览器/桌面、平台直读、AI 默认关闭、无需 AI 的功能、云端正文确认与年度统计例外、JSONL 导出范围。按所选源码对这些约定项做了来源抽查；没有把未启动应用的静态核对当作真实微信、系统授权或服务端网络请求的运行验收。

本次成功执行段为：指定 Task 1108.869 秒、5 次调用（计划/执行/自查/修复/再自查）；跨 CLI Task 846.127 秒、3 次调用；原生直接组 676.150 秒、1 次调用。前述总表另列完整历史成本。单 CLI 自查实际发现并纠正了一处转述错误，Cursor 的两条跨 CLI 复核没有新增 findings；代码两组实现相同。

这是带恢复历史的一次项目体验，任务顺序、材料历史和调用机会不同，未做受控重复实验。可以据此确认路径和产物已经完成，不能据三次耗时推导协作普遍更快或更优，也不把这次执行当作正式发布评审或独立用户研究。

## 用户已完成的 R0 与首次失败

时间为北京时间；耗时来自实际 Run 记录。

| 场景 | Task / Run | 时间 / 耗时 | 结果 |
| --- | --- | --- | --- |
| 阅读当前项目入口、首次使用和浏览器/桌面边界 | `run-20261024T063258Z-5d305437` | 14:32:58 / 72.154 秒 | success、complete；返回源码说明 |
| 沿同一原生会话追踪连接、同步与界面刷新 | `run-20261024T063544Z-7f459630` | 14:35:44 / 126.617 秒 | success、complete；关联前一 Run，原生 session 相同 |
| Work 指定 Codex：项目体验评审和验收清单 | `task-20261024T063848Z-07fa437f` | 14:38:48 / 201.699 秒 | failed；协调约 81 秒，worker 在 120 秒被终止，两份文件均不存在 |

首次失败并非总计十分钟耗尽：当时 Task 已用约 202 秒、调用 2/6 次，执行器却将每个 Agent 步骤固定为 120 秒。原生执行阶段仍在核对源码，只有过程说明，没有完成 handoff 或生成文件。用户随后 `cat` 报文件不存在，与实际产物状态一致。原始失败 JSON、Run、receipt 与事件保留，不改写成成功。

## 修复内容

- 新增 `task --call-timeout` 和 `task continue --call-timeout`，单独设置每次 Agent CLI 调用的时限，默认保持 2 分钟。`--timeout` 仍限制累计执行时间，较长的单次时限不会绕过剩余总预算。
- 单次时限写入 Task Options、继续更新及控制队列；旧记录缺少该字段时按 2 分钟处理。继续可提高后续调用时限，不重置已有调用数、执行时间、失败 attempts、模式或工作区。
- 单次超时错误明确说明与总预算的区别及恢复参数。验证命令仍各限 120 秒。
- Codex 非交互 resume 的 sandbox 参数统一转换为原生支持的配置覆盖，供普通 resume 与 Task 恢复共同使用，保留角色所要求的执行策略。
- 识别 Codex 明确的额度耗尽错误，保存脱敏的 `quota_exceeded` 原因，并在原生退出 1 时保留它；不再只显示进程退出码。任意原始错误正文不进入公开错误，解析损坏、取消和超时仍保留各自优先级。

第一次恢复使用 `--call-timeout 8m --timeout 30m`，原 Task 先核对输入摘要及空输出，再通过正常 continue 和 reconcile 沿原生 worker 会话恢复。该次调用写出约 38.6 KB 的第一份报告，仍在 480 秒达到上限，没有完成第二份文件。第二次检查已保留的报告与全部输入摘要，再提高至 `--call-timeout 15m --timeout 60m`；由于部分产物改变了快照，运行时重新规划，最终在 worker 阶段遇到真实额度耗尽。原 Task 合计 5 次调用，所有超时、部分产物和后续失败均保留。

原生登录入口沿用用户 R0/首次 Task 的默认 Codex 目录，不读取、复制或切换认证凭据。原生错误明确为 `usage_limit_exceeded`，提示当天 19:04 可再试；此提示是服务端建议时间，不是本页保证的恢复时间。助手已询问用户选择等待原入口，或明确指定其他已登录入口另建任务；未擅自换账号或把新入口混入原 Task。

## 第一批实际执行清单：额度耗尽时

| 阶段 | Task / 场景 | 实际状态 |
| --- | --- | --- |
| R0 | 上表用户运行与同 session 继续 | 已通过并沿用，无重复调用 |
| R1 Work / 指定 | `task-20261024T063848Z-07fa437f`，合计 5 次调用 | failed：保留第一份报告，两次超时后继续时遇到额度耗尽；第二份文件及自查未完成 |
| R1 Work / 自动 | `task-20261024T075141Z-f1b96a03`，1 次调用 | failed：Codex 协调阶段即额度耗尽，Cursor 未实际参与 |
| R1 Project / 指定 | `task-20261024T074906Z-8f6737ab`，2 次调用 | failed：实际写出组件改动与 13 项组件测试，尚未完成 handoff 和运行时验收即额度耗尽 |
| R1 Project / 自动 | `task-20261024T075146Z-3101a540`，1 次调用 | failed：协调阶段额度耗尽，未进入跨 CLI 审查 |
| R2 | `task-20261024T075150Z-dcc59341`，1 次调用 | 后台提交退出 0；最终协调失败，未进入可取消的 worker，未认证 stop/continue 或材料恢复成功 |
| R3 | 原生 Codex 一轮相同文档任务 | 原生退出 1，JSONL 明确 usage limit；无报告产物，无法作质量收益比较 |

第一批助手新增 9 次原生 CLI 调用；加用户最初 2 次 R0 和 2 次 Task 调用，当时累计 13 次。失败和超时不从计数中删除。当时尚无成功的跨 CLI 交接或 R2 恢复证据；后续沿原 Task 的成功另列最终结果。

## 额度恢复前保留的产物与后置检查

- [第一份项目体验评审](artifacts/chat-insight-20261024-before-reset/onboarding-review.partial.md)：真实 worker 生成，但当时两份交付整体未完成，文件名明确标记 partial；导出仅替换本机目录前缀，引用需回到所选源码核对。
- [当时的组件补丁](artifacts/chat-insight-20261024-before-reset/component.patch)、[组件源文件](artifacts/chat-insight-20261024-before-reset/DateRangePicker.tsx)及 [13 项组件测试](artifacts/chat-insight-20261024-before-reset/DateRangePicker.test.tsx)：保留额度中断时的具体产物。
- 助手在额度中断后独立执行 [test/lint/build 后置检查](artifacts/chat-insight-20261024-before-reset/partial-project-postcheck.log)：14 个测试文件、66 个测试通过，lint、构建均成功。这些结果与原 Task 当时的 checks 分开保存，不改写首次失败。
- [独立行为检查](artifacts/chat-insight-20261024-before-reset/DateRangePicker.acceptance.test.tsx)：在独立副本检查四种选中状态、外部 props 更新、点击与日期清空回调。[原基线为 5 项失败、1 项通过](artifacts/chat-insight-20261024-before-reset/independent-baseline.log)，[生成组件为 6 项通过](artifacts/chat-insight-20261024-before-reset/independent-project-single.log)，没有改动运行时验证所关联的工作区。

## 工程验证

新增回归覆盖自定义单次时限、继续后保留及提高时限、旧 Task 缺字段的默认值、队列拒绝缩小待应用时限、累计时限仍有效、Codex resume 参数，以及额度错误脱敏与退出码合成。

修复后的 `go test -race ./...`、`go vet ./...`、`go mod tidy -diff`、格式/差异检查与 R3 的 25 个确定性集成测试通过。初轮恢复使用单次时限/恢复参数修复候选，额度诊断随后使用真实错误形态离线回放验证；额度恢复后的案例使用最终候选，源码与二进制摘要见机器可读证据。本轮未执行正式发布门禁或创建 tag。

辅助体验脚本也已兼容 Project 无独立 inputs 字段及 Python 3.10 对可变精度 ISO 时间的解析；现成指南补充了篇幅约束、适配任务规模的调用时限，以及检查材料后的 reconcile/fresh-session 恢复步骤。

## 记录与边界

私有原始记录保存在本机体验目录 `campaign-20261024`、Task/Run 目录和原生 session；公开摘要仅导出选定结果，不附原生账号、session ID、凭据、原始工具流或应用数据。本轮不读取真实聊天数据库、不启动微信数据桥、不自动接收或合并代码、不推送发布。

调用统计按原生 CLI 进程轮次计算，包含失败、取消和恢复，不等同于厂商内部模型请求数。与历史 R0–R3 验收账本分开记录；不以此次助手代执行填充真人留存或独立观察记录。

本轮累计 36 次 Agent 执行调用，其中用户原有 4 次、助手新增 32 次；Codex 34 次、Cursor 2 次。此前两个超时、额度失败和连接重试中断均计入。原始材料、运行 JSON、事件、receipt、命令日志与原生输出留在私有目录；公开导出保留选定产物和证据摘要。

## 本机复看

在已载入本次体验环境的终端中，以下命令只查询现有记录，不发起模型调用：

```bash
source "$XCLI_CHAT_ROOT/env.sh"
"$XCLI_BIN" task status task-20261024T063848Z-07fa437f --json
"$XCLI_BIN" task status task-20261024T075141Z-f1b96a03 --json
"$XCLI_BIN" task status task-20261024T074906Z-8f6737ab --json
"$XCLI_BIN" task status task-20261024T075146Z-3101a540 --json
"$XCLI_BIN" task status task-20261024T075150Z-dcc59341 --json
python3 "$XCLI_CHAT_ROOT/compare.py"
```

重新执行可使用已更新的体验指南及环境脚本。新 Work 目标包含篇幅约束，示例显式设置 `--call-timeout 15m --timeout 60m`；组件案例设置 `--call-timeout 8m --timeout 30m`，后台示例为 45 分钟累计预算。完整目标另存为 [Work](artifacts/chat-insight-20261024/work-goal.txt)、[篇幅约束](artifacts/chat-insight-20261024/compact-goal.txt)、[Project](artifacts/chat-insight-20261024/project-goal.txt)和 [后台任务](artifacts/chat-insight-20261024/background-goal.txt)。这些参数有明确上限，不能保证任意项目或原生服务都在该时限内完成。
