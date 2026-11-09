# G5B：逐 CLI 的首次真实 HTTP 文本验收计划

G6 更新（2026-11-09）：app-server Adapter 和假 HTTP 纵向路径已实现；真实无推理握手通过，当前决策 **ISOLATED_RUNTIME_REQUIRED**。本机 restricted access 字段缺失／请求拒绝、管理面可读假凭据、严格外层初始化失败的独立证据见 [G6_SECURITY_BOUNDARY](G6_SECURITY_BOUNDARY.md)。不能仅批准一次调用后执行 exec 或 app-server。下一授权入口与 --adapter codex-app-server-v1 的准确脚本形态见 [G6_DECISION](G6_DECISION.md)，当前只能 dry-run／前置阻断。

首次文本仍最多 1 Task／1 顶层进程、并发 1、120s、无重试／fallback；真实 builtin 安全试验若有必要须另审另批，不占用或暗增首次文本预算。用户账号、计划、用途、云同意及费用风险要与新安装／策略／环境绑定，不能挪用旧 exec 声明；原生内部调用和费用不由进程次数精确限制。真实 Gate B 仍非 PASS。

G5A.2 最终决定：Codex 当前 ENGINEERING_BLOCKED／USER_CONFIRMATION_REQUIRED；[独立隔离证据与替代路线](CODEX_ISOLATION_BOUNDARY.md)和[本人声明入口](CODEX_ACCOUNT_AND_USAGE_CONFIRMATION.md)为前置。脚本已实现新 generation、防陈旧端口及双轴门禁，dry-run／显式 execute 负向均零 Task。**本轮没有 G5B 推理授权，真实 Gate B 仍非 PASS。**

G5A.1 已交付 [Codex 默认 dry-run 的独立脚本与准确参数](CODEX_FIRST_REAL_HTTP_TEST.md)，以及 [受限配置／控制面／隔离证据](CODEX_LIMITED_PROFILE.md)。首个文本仍未授权、未执行；全量工具／认证目录隔离与用途确认仍 BLOCKED，不能仅批准一次调用后直接运行。

2026-11-08。**未授权、未执行。** 本文件不是启用开关或调用授权，不沿用此前剩余 3 次。当前三家均未达到执行前置；先解决 [G5A 分项阻塞](G5A_PROFILE_READINESS.md)，不能仅获一次调用同意就绕过 `Profile.Check`。

本计划把 L01 拆为 **L01-a 首次文本（1 次）**、L01-b 续接（另 1 次）。不改变 ACCEPTANCE 的完整门槛；首次文本成功也不能将 NAG 真实 Gate 或原 Gate B 标 PASS。旧 [REAL_HTTP_ACCEPTANCE_PLAN](REAL_HTTP_ACCEPTANCE_PLAN.md) 的 L01 2 次指两个子场景，本计划以更小的首个子场景作为下一次授权单位。

## 执行前的硬性条件

1. 固定单一官方 CLI、精确 binary/bundle hash、版本、adapter/schema/policy/profile/capability revision。版本漂移重新审查，不自动更新或换 transport。
2. S01–S03 已实现并经离线安全审查；合成验收入口只能绑定本人客户端、text_task、单一非秘密材料、1 dispatch／1 顶层原生进程、期限和授权摘要。普通应用仍 unavailable；该受限控制入口已在 G5A.1 实现，但完整工具／认证边界在 G5A.2 仍未证明，不得注释 Native 拒绝或伪装 Native=false。若要证明 L04，额外 canary 场景须另批，不能借本次文本预算完成。
3. 原生认证由官方进程自行使用，不读取／复制／打印认证资料。确认账号／组织及应用场景适用条件；若改用 SIWC，先另行完成应用注册、OAuth 与用户 plan usage 授权，不能套用历史 native 登录。
4. 采用合成材料、固定服务工作区，不读取真实业务数据库／原生会话历史。工具、读写、shell、网络及全局扩展边界能实际强制，不能以“请不要使用工具”的 prompt 代替隔离。
5. 本人提交新的明确授权：供应商、具体路径／hash、用途与账号、可承担的订阅／credits 费用风险、最多 1 次启动、120s、云同意、停止条件。若要求精确金额硬上限而官方 route 无法强制，则该任务不能执行；`max_cost_usd` 仍返回 budget_unenforceable。

没有条件通过的项目保持独立 BLOCKED/NOT_TESTED；不为了首个文本请求降低 L04 或扩大权限。必要安全 canary 与 L01-a 使用独立槽位，先写预算再请求授权，不能从“1 次文本”暗增为多次策略试验。

## 独立方案 C：Codex 优先候选

| 项目 | 计划值／要求 |
| --- | --- |
| 首选入口 | codex-exec-v1；当前 0.167.0、hash 见 G5A。只有 exec 安全边界无法满足且新 adapter 经审查后才另选 app-server，不在失败后切换 |
| 最小预算 | L01-a：1 dispatch、1 顶层 native 进程；Session/GET/幂等重放 0 新启动；全服务／安装域／Session 并发 1 |
| 超时／清理 | 120s 含排队／启动／验证；本地清理另最多 5s。失败消耗次数，不退款、不重试 |
| 实际前置缺口 | 受限 workspace／身份／固定文本／持久一次预算已实现；G5A.2 仍缺 exec 有效工具目录、builtin 独立 IO 边界、实际工具环境、当前登录及本人声明。当前仅有 app-server 握手/schema，没有运行 adapter。不能仅凭 Canary 或一次调用许可启用 exec |
| 账号／费用 | 用户确认所选账号、计划及应用用途；首次 turn 才能证明该次模型访问。单进程内推理／工具／重试数量不可精确限制，订阅额度、credits 或其他计费可能消耗；不得切换 API Key、账号或 provider 补测 |
| 真实权限审批 | 保留只读／拒绝语义；需要升级权限时拒绝并 blocked，不批准工具或使用自动审查／全权限参数。工作区/Git 前置须在推理前完成本人审查 |
| 固定任务 | HTTP 内联固定标记 XCLI_G5B_CODEX_SYNTHETIC_20261108，请求只返回同一标记。不附文件，不使用图片、shell、MCP 或外部业务材料；此约束还须由执行策略强制 |
| PASS | 独立 HTTP Task succeeded；完整原生会话 ID 观察、terminal 成功及 exit 0；最终文本严格匹配标记、final=true、预算仅 1/1，持久 GET／原键重放为同一 Task，无额外 native 启动 |
| 停止 | 任一策略／登录／额度／支付／权限／Git 拒绝、缺终态或 ID、文本不匹配、执行事实不明，即记录并停止。未知停止 needs_reconcile，不能重发 |
| 需要的新确认 | 确切 Profile／工作区／账号用途与云同意；仅这 1 次启动／120s，以及内部调用和费用不可精确观察的风险；如需 SIWC，另外批准登录授权流程 |

G5B 第一候选是 Codex，因为历史原生文本成功、当前 refresh/初始化/schema 证据最完整，且无 Cursor 的 Trust 和退出阻塞、无 Agy 的网关用途条款冲突。此排序是工程判断，**不表示 Codex 现在已经 ready**。

## 独立方案 A：Antigravity

| 项目 | 计划值／要求 |
| --- | --- |
| 入口 | antigravity-print-v1，当前 1.5.2；ACP 只能在新增并独立验收 adapter 后另选 |
| 最小预算／并发 | L01-a 1 dispatch／1 顶层进程；全局、安装域和 Session 均 1；不预留其他 CLI 的重试额度 |
| 超时／清理 | HTTP Task 120s，与当前原生 --print-timeout 120s 一致；另最多 5s 本地清理，不声称原生/云超时等价 |
| 实际前置缺口 | S01–S06、A02/A03：session-only settings、认证与全局工具隔离；**供应商对本机多应用 HTTP 网关场景的授权范围必须先澄清** |
| 账号／费用 | 明确个人／企业合同、账号模型范围、credits/用量配置和数据条款；当前未知。不能因 refresh/ACP initialize 成功就确认计划／模型可用。一次进程可含子代理／多次内部推理，不能给出精确费用承诺 |
| 真实权限审批 | plan+sandbox 不自动扩大；headless 无法交互的 Ask 应 soft-deny，stderr/denied_actions 优先，即便 SUCCESS/exit 0 也 blocked。不向 stdin 注入 control_response 或写全局 allow |
| 固定任务／PASS | 使用独立随机合成标记，严格最终文本相等；要求 conversation_id 为本次真实观察、SUCCESS+exit 0、无拒绝／支付信号、final=true、预算 1/1，原键返回原 Task |
| 停止 | 条款／账号资格未确认则零推理；运行中遇 soft deny、ERROR/异常退出、额度、支付、部分成功或缺 conversation，立即停止该场景，不自动换 Gemini SDK/API 模型或重开 |
| 需要的新确认 | 先提供该用途的适用授权依据；再独立确认 1 次／120s、账号／费用／云数据同意、所用精确 Profile。本人同意一次调用不能替代供应商授权条件 |

## 独立方案 U：Cursor

| 项目 | 计划值／要求 |
| --- | --- |
| 入口 | cursor-headless-v1，当前 2026.10.22-27dcf5f；agent acp 存在但未接入 Gateway |
| 最小预算／并发 | L01-a 1 dispatch／1 顶层进程；全局、安装域和 Session 均 1。若首次仍 trust 拒绝，该次即结束，不能修 Trust 后自动重试 |
| 超时／清理 | 120s＋最多 5s 本地清理；先修 U02/U04 的停止证据，不能沿用“parent 已退出”替代进程组停止确认 |
| 实际前置缺口 | S01–S06、U01–U04。先固定整个 Node/JS bundle；修复并复核 Gateway refresh；确定**实际服务生成的 Session 工作区**，本人在官方流程审查最小 Trust，不能只信任 xcli 源码目录或父目录 |
| 账号／费用 | 确认当前账号／组织政策、模型访问和额度；不传 API Key/auth-token、不修改账号。模型内部次数与费用未知；一次进程不是一次模型计费 |
| 真实权限审批 | 维持 ask+sandbox；不加 force/yolo/trust/approve-mcps。若未来用 ACP，所有权限请求拒绝，阻塞问题跳过、计划拒绝；不新增 HTTP approve 路由 |
| 固定任务／PASS | 独立随机合成标记；本次实际 session_id、完整成功 result+exit 0、无 trust/权限/支付拒绝；文本相等、final=true、预算 1/1，持久 GET／原键无重复启动 |
| 停止 | Trust 未明确或 refresh/安装完整性未通过时零推理；首次受阻不改全局 Trust、不迁移到另一项目、不给 shell 权限、不换 CLI 补测 |
| 需要的新确认 | 单独 Workspace Trust 决定（精确服务目录、权限、有效范围）；安装／生命周期修复证据；然后另外确认 1 次／120s 真实调用及云／费用范围 |

## 获批后的 HTTP 步骤（目前不可执行成功）

1. 构建、停服 `refresh`、`preflight`，复算安装／源码摘要。按已实现的受限验收机制登记精确授权；该机制未完成前停止。启动独立 loopback，使用新的本人专用 Token，四应用 Token 不启用。
2. 全局／client／Session 上限均 1 dispatch、1 process_start、120 execution_seconds，Task timeout=120；云同意绑定本次上下文摘要。创建 Session 为零原生启动，冻结 Profile；不要填历史 native ID。
3. 以新稳定 Idempotency-Key POST `/agent/v1/tasks`，正文与 [USER_GUIDE](USER_GUIDE.md) 一致，operation=text_task，仅一个内联合成 text。不得在请求里带 argv/env/cwd/model 或权限标记。
4. 202 仅代表接受。只用同一 task_id GET 到终态；连接不明时原键／原正文重放查询，不用新键。首次完成前不追加续问，不因慢响应加第二次调用。
5. 核验上述逐 CLI PASS 条件，保存脱敏证据、正常停止服务。任何阻断或不完整都保持对应状态；不使用 fake/普通模型 API 补齐。

证据至少含：授权引用、UTC 起止、CLI/bundle/schema/adapter/policy/profile/source SHA、材料哈希、HTTP 状态／对象／revision／幂等键摘要、dispatch 预扣、native ID 实际观察布尔及服务端一致性判定、exit/terminal/protocol/denial、final 文本 SHA／匹配判定、usage 范围及 nullable 值、本地／云停止各自事实。Token、账号标识和原始工具载荷不落公开日志；不读取私人目录或原生历史追溯补证。

## 后续槽位：首个文本之后再决定

| 场景 | 单 CLI 最小新增预算 | 前置及结论边界 |
| --- | --- | --- |
| L01-b 同 Session 续接 | headless 1 次；若未来常驻传输则 1 dispatch、是否新进程另记 | 只在 L01-a PASS 后单独批准；不重供标记，实际 native ID 相同；不能用 fallback |
| 观察已有 Task 的 SSE 重放／幂等 | 0 次 | 可复用首个 Task 的持久事件，不制造 delta；与原生 delta 验收分开 |
| L03 cancel / timeout | 各 1 次 | 独立授权、合成活动；本地确认与云撤销分别记录 |
| L04 策略 canary | 暂估越界读／写／shell／网络／继承工具各 1，共至少 5 次 | 在可执行隔离工程完成后逐项申请；原文门槛不变，没有尝试到的边界不得 PASS；不足需新计划 |
| L02 生成／编辑 | 各 1 次，共 2 | 先补真实媒体 adapter、官方授权／产物绑定；参考不覆盖、真实字节/来源/视觉＋认证交付全部通过 |

表格不是合计授权。本轮上述真实执行均 0；原 Gate A 只继承历史特定场景，原 Gate B（native CLI→Model Gateway `/v1/*`）始终 BLOCKED，四应用/UI/生产部署仍独立。
