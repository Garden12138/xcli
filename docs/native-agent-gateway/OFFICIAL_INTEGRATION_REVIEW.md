# G5A 官方接口与授权边界

核查日期：2026-11-08；工程起点 `f72e32a`。下列官方页面本轮实际打开。本轮只运行版本、帮助、schema 和无 turn 初始化；没有登录、读取认证文件、交换 Token、发送推理或修改 Trust。接口公开、安装支持、账号权益、应用授权和可执行隔离分别判断。

## 官方入口及适用范围

| 路径 | 官方提供的接口 | 本轮结论 |
| --- | --- | --- |
| Codex exec | 非交互 JSONL、按 ID resume；默认 read-only，需要 Git 项目。[官方文档](https://learn.chatgpt.com/docs/non-interactive-mode) | 继续保留现有 adapter；只读不是读取范围限制。不会采用文档中的认证文件复制或跳过安全检查做法 |
| Codex app-server | 双向 stdio、thread/turn、审批、interrupt 和版本对应 schema；本机 help 仍标 experimental。[官方协议与认证](https://learn.chatgpt.com/docs/app-server) | 已核实本机 initialize/schema；现有 `rpc_eval` 只是静态评估器，无运行 transport。官方说明本地／开源应用可继续使用 app-server authentication，建议迁移 SIWC，商业／托管服务不能据此使用该认证。xcli 的具体使用主体、分发模式和适用条件尚未确认 |
| Codex SDK | TypeScript 提供线程控制；Python 通过 app-server JSON-RPC，发布包带固定 CLI runtime，可显式选择本机 binary。[官方 SDK](https://learn.chatgpt.com/docs/codex-sdk) | 本仓库未引入或安装 SDK。它不自动解决 Go 的审批、隔离、预算或原生版本绑定 |
| Sign in with ChatGPT（SIWC） | 身份与可选 ChatGPT plan usage 是不同授权；官方示例面向符合条件的 Plus/Pro，开放给开源、本机个人项目及选定私有应用。[官方教程](https://developers.openai.com/cookbook/articles/sign-in-with-chatgpt) | 是应用自己的 OAuth/用户同意流程，不是读取 Codex 登录缓存。xcli 尚未实现应用注册、回调、授权与续期；当前账号和项目资格 NOT_TESTED，不能推定订阅已授权四应用或共享网关 |
| SIWC → app-server | 应用将自己 OAuth 交换得到的 access token 交给官方子进程；模型目录不能证明本次推理权益。[官方接法](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server) | 独立候选方案，本轮没有注入 Token、改 provider 或走普通模型 API。不能拿原生凭据替代应用授权 |
| Cursor headless | print/stream-json 支持脚本；partial 输出需显式开关及去重。[官方 headless](https://cursor.com/docs/cli/headless) | 当前固定 ask+sandbox，无 force/yolo/trust。文档中自动改文件的例子不作为本项目权限依据 |
| Cursor ACP | 官方 `agent acp`，stdio JSON-RPC；session/new/load/prompt/cancel、权限请求、阻塞问答/计划及图片通知。[官方 ACP](https://prod.cursor.com/docs/cli/acp) | 本机初始化成功；尚无 ACP adapter。权限请求必须回答，不能悬挂或默认允许；新 adapter 只能拒绝。全局／项目 MCP 不能因 ACP 公开就默认继承 |
| Antigravity headless | print、NDJSON、conversation；流式 stdin 仅文本，usage 为累计 Session 范围，soft deny 可 exit 0。[官方 headless](https://antigravity.google/docs/cli/headless/) | 现有单 turn print 保留；不将 control_response 当审批，不把 exit 0 当权限通过。当前 usage 保守为 unknown，未逐 turn 累加 |
| Antigravity ACP | `agy acp`，官方进程自行使用缓存登录；支持 Session、权限请求、取消、图片输入等。[官方 ACP](https://antigravity.google/docs/cli/acp/)与[完整客户端说明](https://antigravity.google/docs/ide/extensions/acp) | 本机初始化成功；尚无 ACP adapter。官方说明自定义 ACP 客户端可接入，但第三方模型的客户端范围有限，不能推定任意模型在 xcli 可用 |

SIWC 还有独立预览限制：该 route 不支持 hosted image generation，不能把原 Codex 原生图片历史 PASS 转为 SIWC 图片支持。[官方限制](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations) 即便未来选用 app-server，仍须分别冻结认证 route、模型与工具能力。

## 条款核查与账号证据

| CLI | 已核实的公开边界 | 当前账号／应用结论 |
| --- | --- | --- |
| Codex | [OpenAI 条款](https://openai.com/policies/terms-of-use/)区分个人及商业条款，限制凭据共享和规避限制；应结合上面的产品专属官方程序集成说明 | 未查询当前账户、计划、余额或组织合同；2026-11-06 原生成功仅为历史运行证据。不能宣称当前权益通过；本机个人用途与商业／托管用途必须分开审查 |
| Cursor | [Cursor 条款](https://cursor.com/terms-of-service)覆盖软件／API／文档，组织 MSA 可另行适用；要求输入数据权限并限制转售、提取等 | headless/ACP 的公开性不等于任意非编码业务、共享订阅或第三方服务均获授权。当前账号、额度、组织限制、四应用场景未确认 |
| Antigravity | [附加条款](https://antigravity.google/terms)区分个人及企业访问，并明确限制第三方软件访问服务；同时官方 ACP 文档允许自定义客户端 | 官方 ACP 客户端范围与多应用 HTTP 网关的关系仍需供应商说明／适用合同证明。**独立授权阻塞**，不能自行把 ACP 文档解释成通用网关豁免，也不据此断言所有官方 ACP 使用均被禁止 |

没有运行账号登录或余额请求：合成 HOME 的握手不证明现有账号身份，认证方式广告也不证明已认证。即使未来用官方 `account/read(refreshToken:false)` 或原生 status 查询登录状态，仍不能据此确认目标模型、剩余额度及应用场景全部获准。不得打开 Keychain、认证缓存或打印 native Token 来补证。

## 传输语义与现有边界

| 维度 | 当前 exec / print | app-server / ACP 增量要求 |
| --- | --- | --- |
| 工具／权限 | 固定安全 argv，已识别拒绝优先于成功；没有交互批准 | 双向请求关联；Codex decline、ACP 拒绝；Cursor 问答/计划也需明确取消／拒绝。不能让应用透传任意 RPC |
| Session | 每 turn 一个进程，显式 native ID；旧 runtime fallback 不计入 Gateway | 原生 thread/session 与逻辑 owner 绑定；load/resume 不导入其他人的历史，不 list 全部私有 Session |
| 实时事件 | 持久 SSE 观察任务；只有活动与 final 引用，无虚构 delta | 原生通知不是 HTTP SSE；需映射、去重、限额、先持久再发布。MCP 的 SSE 能力也不是 Gateway 的 text delta |
| 取消 | 停当前进程组；云撤销未知，不能称云任务已停 | interrupt/session cancel 后等原生终态，ack 不够；独占 worker 有界收尾，异常 needs_reconcile |
| 媒体 | 真实文件安全导入／复核合同已离线通过；真实 adapter 不返回 Candidates | 图片输入、图片生成通知和真实输出文件是不同能力；需 task/tool/file/hash 关联。不能读取任意 URI、扫私人目录或用 notification 宣称产物存在 |
| 错误／部分结果 | 已识别 trust/auth/quota/payment/deny → blocked，timeout/cancel 独立；部分文本 final=false | 还须适配 RPC errors、stopReason、服务请求超时、拒绝与晚到成功；这些真实 turn 行为本轮 NOT_TESTED |

本机证据与逐项工程缺口见 [G5A_PROFILE_READINESS](G5A_PROFILE_READINESS.md)。官方支持只能建立候选路径；账号、应用授权和隔离缺一仍不得晋升 Profile。
