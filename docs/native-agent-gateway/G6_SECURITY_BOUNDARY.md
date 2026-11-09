# G6：app-server 安全边界和认证审查

2026-11-09；真实推理 0；结论 **ISOLATED_RUNTIME_REQUIRED**。本轮不再以 exec shell Canary 泛化所有工具，也不把 app-server 当作完整安全沙箱。

## 本机版本与字段实证

已安装 `codex-cli 0.167.0`，binary SHA256 `030316ecfbb41f4e7946e584b0063ef333b7a36cd312ba349f2f6869e39b62cc`。本轮重新生成 322 个版本绑定 schema：[原始所选 schema](evidence/g6/schema.json)、[字段与摘要索引](evidence/g6/schema-capabilities.json)。只读版本和 schema 生成共 2 次非推理启动。

官方当前文档描述 `sandboxPolicy.type=readOnly` 的 restricted access、readableRoots 和 includePlatformDefaults；本机 `CommandExecParams`／`TurnStartParams` 生成的 ReadOnly 变体只有 type/networkAccess，没有 ReadOnlyAccess 定义。带 restricted access 的 command/exec 请求，defaults=false 与 true 均实际 RPC_REJECTED。**不能证明这些字段在本机生效，也没有退回更宽策略来过关。** 官方页面存在不等于当前安装版本支持。[官方字段说明](https://learn.chatgpt.com/docs/app-server)。

## 新路径的实际观测

真实探针的 HOME、CODEX_HOME、auth.json 和 material.txt 全部刚构造；没有使用任何真实账户、凭据或个人项目。它的传输白名单禁止 thread、turn、account、config 等方法。fs/readFile 只读取自己创建的三个路径，不扫描目录寻找文件。

| 项目 | 结果与范围 |
| --- | --- |
| 普通 stdio initialize/initialized、EOF 退出 | PASS，exit 0，原生进程组停止已确认 |
| 直接 fs/readFile 读登记材料 | READ_EXACT_CANARY，阳性对照 |
| 直接 fs/readFile 读工作区外假 auth.json | READ_EXACT_CANARY |
| 直接 fs/readFile 沿工作区链接读该假文件 | READ_EXACT_CANARY |
| command/exec restricted access + defaults=false/true | 两者 RPC_REJECTED；没有阳性命令对照，**不算隔离 PASS** |
| 工具 UID／真实工具环境继承 | NOT_TESTED：严格请求不能启动；Go allowlist 静态实现不等于工具实证 |
| 工具网络、MCP、所有 builtin／目录替换 | NOT_TESTED：没有真实 turn；不由 fs admin 或 mock 证据推断 |
| 撤销／关闭／跨客户端／恶意参数 | 假 peer＋HTTP／race；仅网关机制证据 |
| 所有 builtin 不能读取 app-server 认证 | NOT_PROVEN；不能用真实凭据补测 |

见 [最终本机 probe](evidence/g6/local-app-server-final.json)。直接 fs API 是 app-server 的管理面，不是“模型已经调用了该工具”的证据。本轮未创建真实 Thread；也未证明某个 restricted turn 内的 builtin 越权。观测证明的是：服务本身持有工作区之外的读取能力，命令策略无法作为其整个进程的边界；HTTP 禁止透传这些方法仍须保留。

## 优先实现的最小替代：外层 OS 边界原型

新增 `internal/isolatedruntime.Seatbelt`，对**整个 app-server 及其后代**施加内核策略。只接受同一私有临时父目录下的独立 HOME／workspace；拒绝根目录、公开目录、链接根和宽路径；环境不继承用户 Key、proxy 或 loader。策略默认 deny，工作区只读，写入只限新 HOME；另有系统运行库读取及全局 file-read-metadata 例外，不能声称工作区外任何信息都不可见；不授予网络或 Mach／Keychain 服务，不挂载个人目录，不复制认证。

原型实际启动官方 app-server 后在 initialize 前终止：独立诊断为 SIGABRT (-6)、stdout/stderr 无协议证据；[外层初次 receipt](evidence/g6/outer-app-server.json)、[诊断索引](evidence/g6/protocol-diagnostics.json)。**外层 app-server 初始化 FAIL，不是隔离已完成。** 因无有效阳性运行，不能把“读取失败”标为安全 PASS。源码路径／策略测试只证明参数拒绝和静态构造，不证明所有 OS 控制实际有效。

没有继续放宽个人目录、Keychain 或网络，也没有追加同类 shell Canary。保留这个默认关闭、无认证、全网络拒绝的原型供后续受控环境工作使用；当前不能用于真实模型。没有创建系统用户、安装 VM、下载镜像或修改系统配置。

## 官方认证及用途边界

app-server 官方支持 Codex 管理的 ChatGPT 登录及其他认证模式；外部 ChatGPT token 模式有实验性条件。当前实现的任务方法白名单不含登录、账号读取或 token 刷新；收到认证刷新请求返回拒绝并记录 native_auth_required，不代办登录。本轮没有 account/read，也没有取得账号、计划或权益事实。[官方认证接口](https://learn.chatgpt.com/docs/app-server)。

必须区分本人本机 CLI 使用、应用独立 SIWC 授权、付费／远程托管服务。SIWC plan usage 的公开文档针对符合条件的开源／本地应用，付费或远程托管场景另有供应商入口；用途和适用性未由本轮确认。[官方范围](https://developers.openai.com/siwc/token-sharing-open-source)。

若选择 SIWC，应用必须取得自己的 OAuth、plan usage 同意并管理续期；不能挪用 Codex CLI 的私有登录文件。官方 app-server 接法可由应用供应其 OAuth token，但把真实 token 直接放进未隔离进程环境不满足本项目的工具防泄漏要求；还需要外部可信 broker。模型目录也不构成权益证明。[官方 app-server 授权接法](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server)。本轮只设计授权边界，没有发起 OAuth、登录、申请权益或读取真实认证。

[Agents API 安全文档](https://developers.openai.com/api/docs/guides/agents-api/environments/security)强调工作负载隔离、网络和凭据分离；它不是本机 app-server 已获得等价沙箱的证明，也不是改用普通模型 API 的依据。

## 下一最小执行环境（独立增量，不在本轮迁移）

优先专用 VM／受控容器：只读合成 workspace；无个人 HOME、认证目录、宿主 socket 或额外挂载；所有内置路径均受外层环境限制；默认断网。应用自己的授权材料留在环境外的可信 broker，由 broker 只向官方批准端点发送 app-server 产生的原生模型协议请求。环境内只能有作用域受限的连接标识，不能放真实 OAuth／API 凭据。

需要单独确认供应商对这种 host/broker 组合的适用性，再实现任务授权绑定、允许目的地、Token 生命周期、断连／撤销和审计。不能把官方直接传 env 的示例视为本项目凭据隔离已解决；不能让 broker 成为通用推理服务。

估算：环境／包版本固定与 stdio 桥接 2–3 人日；broker 和授权绑定 2–4 人日；覆盖直接 IO、后代、链接／目录替换、网络、撤销及恢复的环境验收 1–2 人日。若需应用 SIWC，再加 3–5 人日及供应商资格／用户授权等待。不在本阶段继续增加相似验证器或自动安装环境。
