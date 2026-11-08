# G5A：真实 Profile 启用前审查

2026-11-08 G5A.1 增量：[Codex 受限控制工程与原生 canary](CODEX_LIMITED_PROFILE.md)、[首次 HTTP 脚本](CODEX_FIRST_REAL_HTTP_TEST.md)。单个合成 Git 项目、固定任务／客户端、一次性持久预算、独立开关／调用许可已实现；官方 CLI 报告 ChatGPT 登录，原生命令沙箱读写／链接／网络 canary 通过。**全量内置工具与认证目录隔离验证器仍缺失，原生 Profile 继续 BLOCKED，未标 READY_FOR_AUTHORIZATION。** 下文保留 G5A 历史结论与证据；S01/C01 已有增量，S03 仅部分有证据，S04/S05/S06 未被替代。

2026-11-08，起点 `f72e32a`。**G5A 审查和最小修复不等于真实验收完成；三个真实 Profile 均保持关闭。** 当前没有具备直接执行 G5B 条件的 CLI；优先补齐 Codex 的文本路线。新推理请求 0，不沿用此前 3 次余额。

## 实际证据

| 项目 | Codex | Antigravity | Cursor |
| --- | --- | --- | --- |
| 本机版本 | 0.167.0 | 1.5.2 | 2026.10.22-27dcf5f |
| 公开程序化入口 | exec / app-server / SDK | print / ACP | print / ACP |
| 本机命令帮助 | PASS：exec、app-server | PASS：print 参数、acp | PASS：print 参数、acp；根 help 未列 acp 不能反推不支持 |
| 无 turn 初始化 | PASS：initialize + initialized | PASS：initialize | PASS：initialize |
| 握手进程退出 | parent exit 0 | parent exit 0 | 5s 内未随 stdin EOF 退出；TERM 后 143，**非正常 EOF 清理通过** |
| 本机版本 schema | PASS：生成 322 个文件，所需 RPC 方法存在；仅保存清单/hash | NOT_TESTED：未发现等价生成入口 | NOT_TESTED：未发现等价生成入口 |
| Gateway 原生 refresh | PASS | PASS | **FAIL：version_probe_stop_unconfirmed**；独立版本返回成功不等于进程组停止已确认 |
| 临时 HTTP 入口预检 | 503 policy_unenforceable | 同左 | 同左；refresh 失败没有生成缓存 |
| 当前账号／模型权限／余额 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| HTTP 真实文本、Session、原生 delta、取消、图片 | 全部 NOT_TESTED | 全部 NOT_TESTED | 全部 NOT_TESTED |
| 真实 Profile | BLOCKED | BLOCKED | BLOCKED |

[帮助与二进制摘要](evidence/g5a/help-probes.json)、[Codex 初始化](evidence/g5a/codex-initialize.json)、[Agy 初始化](evidence/g5a/antigravity-initialize.json)、[Cursor 初始化](evidence/g5a/cursor-initialize.json)、[本机 schema 清单](evidence/g5a/codex-schema.json)、[HTTP/refresh/preflight](evidence/g5a/gateway-preflight.json)。初始化证据仅保存结构和能力布尔值；声明 image/loadSession 不代表实际任务通过。Python 握手的 parent exit 0 也未证明整个进程组已停止。

共 21 次顶层原生非模型探针调用：Codex 8、Agy 7、Cursor 6，见[账本](evidence/g5a/non-model-ledger.json)。Cursor wrapper 内部还会执行 Node 检查，不能把顶层次数称为全部 OS 子进程数。没有 authenticate、thread/start、session/new、turn/start 或 session/prompt；没有真实 Task 提交。

## 默认关闭的确切路径

1. [policy.go](../../internal/agentpolicy/policy.go) 的 `Defaults()`：Native=true、Blocker=policy_unenforceable。`Profile.Check()` 即使清空 Blocker，仍无条件拒绝 Native；**尚无受支持的真实晋升机制**。
2. `agentservice.CreateSession`、`CreateTask` 和 `ExecuteNext` 检查 Profile；[Exec.StartTurn](../../internal/nativeadapter/exec.go) 在 CheckBinary、生成 argv 和 spawn 前再次 Check。
3. [agent_gateway.go](../../cmd/agent_gateway.go) 生产入口只加载 CachedProfiles，worker 固定 Exec；YAML 没有 enable/native/ACP 开关。全局、客户端默认预算为 0；预算非零和 AllowRemote 也不能启用。
4. [cache.go](../../internal/agentpolicy/cache.go) 的 refresh 仅有界 `--version`，缓存 hash/version，停服取单写者锁；不写可用性／授权证明。GET capabilities 不启动探针。
5. 新增 `agent-gateway preflight --agent ...`：停服控制面读取配置／缓存／选中 executable，零 spawn，分别输出阻塞和 NOT_TESTED；退出 0 只表示报告生成，`can_execute:false` 才是执行结论。

HTTP 预检使用真实安装的缓存和生产服务，预算故意设 1/1/120，仍在建 Session 阶段拒绝；**没有进入真实 Exec 推理启动**。实际协议启动由独立无 turn 探针验证，不把两段证据拼成真实 HTTP→CLI 成功。

## 共同阻塞：逐项保留

| ID | 条件／事实 | 类型／状态 | 解锁所需工作 |
| --- | --- | --- | --- |
| S01 | Native 硬拒绝且无证据登记／受限验收通道 | 工程缺失，BLOCKED | 实现精确 binary/adapter/policy/evidence 绑定和 owner-only 合成验收许可；普通应用仍关闭。不能只加 bool |
| S02 | Environment 使用私有合成 HOME，去掉 CODEX_HOME/XDG/认证及代理变量 | 认证与配置隔离 parity 缺失，BLOCKED | 让官方进程自行认证，同时可执行地隔离全局 MCP/hooks/plugins/文件/网络；不恢复整份父环境或复制凭据 |
| S03 | read-only/plan/ask+sandbox 并非全盘读取、网络及继承工具的逐次边界 | 策略实现和 L04 证据均缺失，BLOCKED | 版本对应权限 schema、强制读写范围及工具策略；分别合成 canary 验证，不能靠 prompt |
| S04 | 当前账号、组织规则、模型权益、额度未知 | 缺账号证据，NOT_TESTED | 官方非秘密状态＋实际新授权场景；状态或模型目录不能替代推理权益证明 |
| S05 | 公开接口不等于所有应用模式获准 | 缺供应商／适用合同证据，BLOCKED | 明确本人本机、是否分发／商业／共享，逐供应商核实；见官方审查 |
| S06 | 无本轮真实模型请求授权 | 用户授权边界，BLOCKED | 固定一家、材料/hash、预算、期限、云同意；新确认后才执行 |
| S07 | 未执行 HTTP 真实 turn 与 L01–L05 | 缺真实验收证据，NOT_TESTED | 首先独立 L01-a，再分别续接、取消、媒体；不以 fake 结果补齐 |
| S08 | image_edit 未将 ReferencePath 附加到官方输入；OutputRoot 未映射原生输出、Exec 不产出 Candidates | 真实媒体 adapter 功能缺失，BLOCKED（不阻塞纯文本的独立开发） | 官方输入／文件事件绑定及允许输出根；来源与字节／语义独立。禁止扫私有目录 |

三家的 deny-only 拒绝合同已离线验证；**缺“批准 API”不是纯文本必选功能缺失**。遇需要批准的工具必须 blocked，用户另开官方流程审查；不在 HTTP 端批准，也不自动重发该 Task。真实拒绝文案和取消行为仍需逐版本验收。

## 各 CLI 独立阻塞

| CLI / ID | 原因与证据 | 状态与后续 |
| --- | --- | --- |
| Codex C01 | 服务 Workspace 仅建目录，不初始化 Git；固定 argv 未指定现成可信 repo。官方 exec 有 Git 前置检查 | 工程缺失，BLOCKED；先实现服务创建的无 hooks 合成 Git workspace 或经审查正式路径，不擅加 skip-git-repo-check |
| Codex C02 | 本机 help 确有 ignore-user-config/ignore-rules；当前 Gateway 未用。能否同时保持官方认证与逐次权限不明 | 能力已发现，执行效果 NOT_TESTED；不能把参数存在当隔离完成 |
| Codex C03 | app-server 初始化/schema 成功，`rpc_eval` 无 stdio pump、turn、审批／终态／interrupt 实现 | 新 transport 工程缺失，BLOCKED；不强制替代 exec，另立增量 |
| Codex C04 | SIWC 注册、OAuth、plan usage 同意／续期未实现；本机 CLI 登录不代表应用授权 | 独立路线 BLOCKED；若选此路线另立工程与新用户授权。非本轮普通 API 替代路线 |
| Agy A01 | print 可能软拒绝后 SUCCESS/exit 0；当前仅已知 denied_actions 与有限 stderr 模式识别 | 已有离线拒绝面，真实文案／错误码、部分成功 NOT_TESTED；不以 exit 0 放行 |
| Agy A02 | 本机支持 session-only --settings、ACP sandbox/default-customizations 等参数；没有冻结、审查及执行证据 | 真实配置隔离工程／证据缺失，BLOCKED；不能直接继承用户全局 allow |
| Agy A03 | 官方 ACP 支持客户端，但条款第三方访问限制与 HTTP 网关用途适用性未明确 | 独立授权 BLOCKED；需供应商明确解释或适用合同，用户一次调用同意也不能代替 |
| Agy A04 | ACP 权限／Session／cancel 仍无 Go adapter | 新 transport 工程缺失；初始化不等于认证或 turn 通过 |
| Cursor U01 | 2026-11-06 隔离项目被 workspace trust 拒绝；本轮未改 Trust、未发送 prompt 验证新 workspace | BLOCKED；仍需本人单独审查**具体服务 workspace**及官方最小 Trust。ACP 初始化不检查这个前提 |
| Cursor U02 | 两次 Gateway refresh 尝试未写入缓存；最终报告为进程组停止无法确认 | BLOCKED；需定位 wrapper/子进程生命周期并维持停止确认条件；不能只因 parent --version=0 忽略 |
| Cursor U03 | 文件 hash 目前只固定 1096-byte launcher，未固定旁边 node/index.js 和其他依赖 | 安装完整性工程缺失，BLOCKED；启用前需要 bundle 摘要／版本依赖验证，不宣称 wrapper hash 代表整体 |
| Cursor U04 | ACP EOF 超时后 TERM，且无 ACP pump；阻塞问答／计划／权限未实现 | 生命周期／transport 工程缺失，BLOCKED；无 force/yolo/trust/approve-mcps 补救 |

## 最小源码修复及测试

- 新增不执行 CLI 的停服 `preflight`，闭合三家 native profile，配置／缓存／完整性／策略／账号／授权／Trust 分项输出；非 native 夹具不能作真实就绪证明，增加 marker trap 证明零启动。
- `Exec` 不再让后续 truncated 覆盖已知 auth/quota/denial；协议仍记 truncated，partial result 保持 final=false。假 CLI 的 auth/quota＋9 MiB 输出契约测试通过。
- refresh 增加固定脱敏失败分类及 IOError 失败关闭；不输出 stderr、不缓存失败，不扩大 3 秒／进程停止条件。真实 Cursor 失败因此可独立定位。
- 控制面 HTTP smoke 扩展三家拒绝和 preflight；无模型探针 request 白名单／字符串脱敏离线测试；完整回归见 [offline.log](evidence/g5a/offline.log)。初次离线通过日志与 Cursor 预检失败保留，没有删除测试。

## 增量 SPEC 与估算（未实施，不自动切换）

| 增量 | 入口及合同 | 粗估人日（不含供应商／用户等待） |
| --- | --- | --- |
| 受限文本 exec 启用底座 | 先解决 S01–S03/C01，精确安装依赖摘要；验收许可绑定单 owner、synthetic-only、text-only、1 dispatch、期限与策略摘要，GET 不宣称 available；通过必需真实 Gate 后另行晋升。API V1 不变 | 2–4；若原生边界不能强制，则维持关闭，重新估算 OS 隔离 |
| codex-app-server-v1 | 沿 TASKS G5，Go stdio pump、版本 schema、请求关联／有界通知、deny-only server requests、thread owner、每 turn/process 独立预扣、interrupt 等 terminal、native delta 一致性。新 Session 显式选用，禁止无声迁移／失败重试切换 | 5–9，与原 SPEC 一致；不是本轮重构 |
| ACP 共用 transport | JSON-RPC framing、pending IDs、cancel、未知方法拒绝；客户端文件／terminal 能力先关闭，不能把 native 请求变成任意文件代理 | 3–5 |
| cursor-acp-v1 | 加权限拒绝、问答跳过、计划拒绝、Session load owner、EOF/进程组收敛、工具／图片扩展约束；仍需 U01–U03 | 共用层后 2–4，未含信任／安装修复 1–2 |
| antigravity-acp-v1 | 权限拒绝、Session/取消/stopReason、客户端能力与模型范围约束；仍需 A03 | 共用层后 2–4；授权不明时不执行推理 |
| 真实媒体 adapter | 精确参考输入、任务输出授权、tool/turn/file/hash 来源关联，既有 FD 校验和 review 复用；不扫描外部产物 | 每家另估 2–4，需独立真实媒体授权 |

是否新增 app-server/ACP 取决于选定能力；**首个文本 HTTP turn 不强制要求新传输，但当前工程仍未到“只差批准一次调用”的状态**。下一入口为 S01–S03 + C01 的最小文本启用工程，再按 [G5B 计划](G5B_REAL_HTTP_PLAN.md)申请固定新场景。
