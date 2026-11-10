# R4 Native CLI Acceptance

2026-11-10。**最终结论：NEEDS_FIXES。离线验收 PASS；真实验收因启动预算无法完整核算而 BLOCKED，未进入真实调用。**

这不是确认 R4 的 CLI 执行代码已失败，而是本轮不能提供 READY_FOR_DAILY_USE 所需的真实证据。执行了用户规定的停止条件，没有用历史成功、模拟结果或当前验收控制器的回答冒充被测 xcli 的真实结果。完成后停止，不进入 R5。

## 基线与二进制

| 项目 | 本轮事实 |
| --- | --- |
| 源码 | `d247a9485b86eafe53b5b670fcbe862146847c4c`，开始时工作区干净。 |
| 构建 | `go build -trimpath`，未修改被测 Go 源码。 |
| xcli 版本 | `xcli version 0.5.0-r4-dev`，来自新构建二进制。 |
| SHA-256 | `c356dbc8c076260a3999c7e8b1848e5a016f340626c7641a5509daf605b4ef8d` |
| 二进制位置 | `/private/tmp/xcli-r4-native-b8tl3i5g/bin/xcli`，临时资产可能被系统清理。 |
| 合成工作区 | 同一临时根下 `workspaces/codex`、`cursor`、`antigravity`，各自独立 Git 仓库，仅含合成 `context.txt` 与初始提交。 |
| 数据保护 | 三个合成仓库最终均干净、材料 hash 未变；没有将 xcli 源码仓库用作真实被测项目。 |

构建、源码审查和离线测试以 xcli 仓库为对象；真实测试材料单独准备。所有原生 CLI 登录、Trust、沙箱与长期规则保持不变。没有读取认证文件，也没有查询/导出 Token。

## 安装、版本、登录与能力

本轮只进行了 PATH/符号链接解析、安装文件散列与已有版本证据比对，**没有执行真实 CLI 的 `--version`、`--help`、登录状态或推理命令**。这些探针也会启动原生进程，在总启动数未审清前不把它们视为免费。

| CLI | 安装发现 | 版本信息及证据边界 | 当前只读登录状态 | 可用能力 |
| --- | --- | --- | --- | --- |
| Codex | PASS，`~/.local/bin/codex` | 安装目标为 `0.167.0-aarch64-apple-darwin`；当前二进制 hash 与 2026-11-06 的 `codex-cli 0.167.0` 证据完全相同。未重新执行版本命令。 | BLOCKED：未启动原生状态查询；用户提供“已登录”不升级为本轮检查 PASS。 | Adapter 声明的文本/结构化结果/Session；当前运行权限、额度和工具未重验。 |
| Cursor | PASS，`~/.local/bin/cursor-agent` | 安装目标目录为 `2026.10.22-27dcf5f`，wrapper hash 与历史相同；另记录当前 `index.js`/`node` hash。wrapper 相同不能独立证明整个 JS payload 相同，未重新执行版本命令。 | BLOCKED：未查询；不推断已注销。 | Adapter 声明的 print/结构化结果/Session；当前 workspace trust 与额度未重验。 |
| Antigravity | PASS，`~/.local/bin/agy` | 当前 Mach-O arm64 二进制 hash 与 2026-11-06 的 `1.5.2` 证据完全相同；未重新执行版本命令。 | BLOCKED：未启动状态或能力查询，不猜测未经确认的登录接口。 | Adapter 声明的 print/结构化结果/Session；当前权限与工具未重验。 |

安装文件 hash 和路径见 [preparation.json](evidence/native-20261110/preparation.json)。历史参照是 [bounded-preflight](../cli-first/evidence/native-20261106/bounded-preflight.json) 与 [text](../cli-first/evidence/native-20261106/text.json)；**历史版本与执行结果不作为当前登录、额度、Trust 或真实模型可用的证明**。

## 离线验收

使用 `tests/product_realignment/verify-core.sh`，在 PATH 首位加入拒绝真实 CLI 的 deny-only fixture。该 fixture 不委托到真实可执行文件；测试自己的临时 fake Agent 仍可运行。最终没有出现真实 CLI 名称的拦截请求。没有启动冻结 Gateway 服务、普通模型 API 或 Ollama。

| 检查 | 结果 |
| --- | --- |
| CLI 发现、选择与只读 Doctor | PASS，含 legacy login 不执行、配置字节保护。 |
| Run 结构化结果/失败/退出 | PASS，固定协议夹具。 |
| Session 记录与续接 | PASS，含缺失 Session 分类、同 CLI 材料恢复和历史 Run 查询。 |
| Task Work/Project、自动规划、角色与交接 | PASS，含不同 Adapter 复核、候选/绑定保留、材料摘要、确定性测试和当前结果审查。 |
| 后台状态、取消、恢复、日志 | PASS，含 owner/attempt 中断、paused/reconcile、继续和重复操作。 |
| 冻结模块默认关闭 | PASS，拒绝先于配置/状态 I/O；本轮未运行 Gateway 生命周期服务。 |
| Go race / vet / build | 核心包 race/vet/build PASS；额外 `go vet ./...` PASS。没有将冻结模块的运行就绪作为验收门禁。 |
| Python / 安装兼容 | R0 11、R3 25（36 项）PASS；候选安装兼容 12 项 PASS。 |

核心命令 exit 0，日志以 `R4_CORE_REGRESSION_PASS` 结束。见 [离线日志](evidence/native-20261110/offline-core.log)、[全仓库 vet 日志](evidence/native-20261110/vet-all.log)、[验证汇总](evidence/native-20261110/validation.json)。R3 旧 harness 输出中的 “real read-only invocation” 在此仅指注入 PATH 的 fake Agent，不能算真实供应商调用。

初次只读进程查询在沙箱内被拒，批准只读祖先名称检查后取得结果；没有读取进程参数/环境或改变系统权限。初次构建有 module stat-cache 写权限提示，但构建 exit 0、版本与二进制均有效；补充 vet 和回归在批准的缓存环境通过。离线未发现需修改 Go 代码的失败。

## 启动预算与真实调用明细

用户批准三 CLI 合计最多 6 次原生进程启动，每 CLI 最多 2 次、单次 120 秒、并发 1、不重试。当前工具进程的祖先链为 **Python → codex → zsh → login → Ghostty**，观察到验收控制器 Codex PID `91960`。这项检查只读 PID/PPID/可执行名称，不读 argv、环境或凭据。

当前控制器必须计入，不能因它未通过 `xcli run` 启动便排除。祖先快照只能确认至少一个现存 Codex 进程，不能提供控制链在本授权范围内完整、可验证的启动历史；本工具也没有权威的全局启动账本。现有 `tests/r0/smoke.py` 的 `invocations` 只预记其 `run/resume`，`safe_probe` 和控制器不在该计数中，因此不能用其“最多六次”字段证明本轮要求。

| 真实记录 | CLI / 类型 | 耗时 | 结果与计数 |
| --- | --- | --- | --- |
| 已存在的验收控制器 | Codex / 本会话控制进程 | 仍在运行，完整启动/累计耗时不可可靠归属；不填写虚假的 0 秒。 | 至少 1 个，纳入全局预算下界；它不是 xcli 合成文本测试 PASS。 |
| 新发起的真实验收调用 | 无 | 不适用 | 0 次，无真实推理/续接/Task 子进程。 |
| 新发起的版本/help/登录探针 | 无 | 不适用 | 0 次，没有把探针排除到计数之外。 |

预算结果：**新增真实验收启动 0；已知控制器至少 1；全局精确累计数 UNKNOWN；可剩余额度上界 5，精确剩余 UNKNOWN。** 六个预定文本/续接测试槽均未执行，不能把它们写成“全局六次额度仍完整可用”。本轮对新 dispatch 的可用额度按 0 处理，不补写或猜测控制器的历史消耗。

当前控制器自身会进行模型工作，原生进程内部也可能有多次模型/工具交互；内部请求数及实际费用均无法精确观测。**新增验收子进程为零不等于整个控制会话没有模型用量或零费用。**

详见 [native-budget.json](evidence/native-20261110/native-budget.json) 与 [controller-audit.json](evidence/native-20261110/controller-audit.json)。预算无法审清时即执行用户要求的停止条件：完成离线准备，停止在真实调用门槛前，不为了凑满额度发请求。

## 真实能力状态

| 能力 | 离线 | 本轮真实执行 | 门禁/原因 |
| --- | --- | --- | --- |
| Codex 首次文本 | PASS | NOT_TESTED | BLOCKED：全局启动计数未确认。 |
| Codex 同 Session 继续 | PASS | NOT_TESTED | BLOCKED：未创建真实测试 Session。 |
| Antigravity 首次文本/同 Session | PASS | NOT_TESTED | BLOCKED：同一预算门禁，不由历史成功补证。 |
| Cursor 首次文本/同 Session | PASS | NOT_TESTED | BLOCKED：同一预算门禁；本轮未证明 Trust 有效或无效。 |
| 原生事件/终态 | PASS | NOT_TESTED | 无真实流，不能把 fixture 事件升级为真实结果。 |
| 原生取消/120 秒超时 | PASS | NOT_TESTED | 本轮未额外分配取消调用，也没有运行中的被测子进程可取消。 |
| 真实 Task / 跨 Agent 编排 | PASS | NOT_TESTED | 按用户优先离线要求验证，没有追加真实多轮 Task。 |
| 图片生成 | 不作本轮媒体验收 | NOT_TESTED | 用户明确禁止；没有重复历史媒体调用。 |

权限不足应 BLOCKED，不自动 trust、改沙箱/长期规则或换账号。本轮没有进入可观察真实权限结果的阶段，不能捏造 permission/quota/network 失败。

## 问题、日常条件与 R5 建议

| 编号 / 优先级 | 本轮发现 | 判断 |
| --- | --- | --- |
| NA-01 / P0 验收门禁 | 当前控制器属于 Codex CLI，缺少覆盖控制链的完整启动账本。 | 阻止本轮真实验收；这是计数证据缺口，不是已复现的 CLI 执行缺陷。 |
| NA-02 / P1 harness | 旧 smoke 计数不覆盖 `safe_probe` 和外部控制器；`--preflight` 只能省去其自身探针，不能补齐外部账本。 | 本轮不运行旧 `--execute`，不把历史任务分派计数套用为全局进程数。 |
| NA-03 / P1 口径 | `agents`/Doctor 会启动原生探针，Task 发现/版本检查也可能额外启动 CLI；`task --max-calls` 是任务分派计数，不是全局原生进程启动上限。 | 使用手册补充严格预算场景说明；没有改变既有计数 schema。 |
| NA-04 / P2 可操作性 | 直接 run/resume 没有通用 `--timeout`，严格 120 秒须依赖外部监督。 | 已知能力边界，不伪造可运行参数；本轮未发生真实超时。 |

日常使用条件：本轮证明 d247a94 的核心离线路径可回归、可构建；没有新证据证明三个 CLI 在当前权限/额度下的真实文本、Session、取消和 Task 行为，故 **NEEDS_FIXES**，不能宣布 READY_FOR_DAILY_USE。历史实测支持范围保持原样，不被本轮改写为 FAIL。

R5 最优先解决：建立可审计的全局原生启动预留/完成账本，明确控制器、探针、任务分派的计数单位并在 spawn 前 fail closed；验证取消/超时也受同一监督和预算。其次是独立、无未计账原生祖先的验收执行入口，以及清晰的只读预检与角色/恢复计划预览。这里只记录建议，不自动实施或申请更多调用。

## 手册修订与结束状态

[使用手册](../cli-first/XCLI_USER_MANUAL.md)补充：严格启动预算下不能直接连续运行 `agents`/多个 Doctor；`task --max-calls 6` 不等于整个验收最多六个原生进程；不能在 run/resume 后虚构 `--timeout 120s`；本次授权不能直接使用旧 smoke 的 `--execute`。现有日常命令语法未发现需要修改的 Go 实现。

没有代码修复提交；本轮仅交付报告、手册澄清与脱敏证据。没有 Push、部署、四应用改动、原生登录/Trust/沙箱配置更改、普通 API/Ollama/Gateway 替代或额外真实 Task。未使用另一个 Agent 代替被测 CLI，未把本会话本身当成被测 xcli 的结果。

**本轮到此停止。** 后续若重启真实验收，先取得可靠启动账本和明确预算口径；不沿用本轮 UNKNOWN 余额，不自动进入 R5。
