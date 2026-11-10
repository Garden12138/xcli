# G7：认证控制面与工具执行面

2026-11-10 官方文档复核及本机实验。路线 A 仅宿主隔离成功；路线 B 判为 **NOT_FEASIBLE_UNDER_CURRENT_CONSTRAINTS**，整体 **PARTIAL_HOST_ISOLATION_ONLY**。没有真实登录、真实凭据读取或推理。

## 官方机制及其边界

| 问题 | 核实结果 | 对当前工程的影响 |
| --- | --- | --- |
| app-server 是否必须用环境变量/文件保存凭据 | 不一定：官方支持 file/keyring/auto/ephemeral；ephemeral 仅在当前进程内存保存。但完成认证推理的 app-server 仍须有可用的授权状态 | 不可把“没有 auth.json”解释为可信控制面已与工具隔离。[认证文档](https://learn.chatgpt.com/docs/auth) |
| 原生 managed ChatGPT auth | Codex 管理 OAuth、持久化与刷新；外部 ChatGPT tokens 模式是 experimental | 本轮未执行这些认证方法，不从原生缓存取 Token；NAG 不向 HTTP 暴露 auth RPC。[app-server auth](https://learn.chatgpt.com/docs/app-server#auth-endpoints) |
| SIWC 接入 app-server | 应用经自己的 OAuth 获 plan usage 授权，把自己的 Access Token 交给 app-server 子进程；更新 Token 后重启并 resume | 官方环境变量示例只是认证方法，不能证明工具隔离。model/list 可以只是目录，非权益证明。[官方 SIWC app-server](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server) |
| 自托管 VM | 官方描述同一用户/工作区的应用 SIWC 注册与受保护凭据转移、VM host ID 和 VM 刷新 | 不等于工具不能读取 VM 内凭据，也不授权复制 Codex CLI 私有缓存。本轮未转移任何凭据。[官方 VM 指南](https://developers.openai.com/siwc/token-sharing-open-source/self-hosted-vms) |
| 工具执行分离 | 官方有沙箱机制和 remote Code Mode host gRPC 入口 | 这是有限机制，不能推导所有直接 fs、内置工具、MCP 都移出认证进程；本机版本/全工具覆盖没有验证，未实现远程 host。[app-server remote Code Mode](https://learn.chatgpt.com/docs/app-server#connect-a-remote-code-mode-host) |
| 应用场景授权 | 本地/开源既有 app-server auth 与商业/托管服务有明确区别；SIWC plan usage 对符合条件的开源、本地应用有独立授权流程 | 不能以程序可执行判定当前部署/账号/用途已授权；本人确认仍缺失。[app-server 授权说明](https://learn.chatgpt.com/docs/app-server#auth-endpoints)、[SIWC 适用条件](https://developers.openai.com/siwc/token-sharing-open-source) |

没有发明认证代理、替换上游端点或注入真实 Bearer。自定义 provider、command-backed secret helper 即使能向 app-server 提供凭据，也没有自动隔离消费凭据的进程和所有工具。本阶段不将它们作为已验证方案。

## 三个角色与实际部署

| 角色 | 目前能保证的事实 | 未满足的边界 |
| --- | --- | --- |
| NAG Gateway | 本机客户端 Token、owner、持久预算/幂等、RPC allowlist；容器配置只有 Go owner 可选，生产不选 | 不代替用户作账号用途、云发送及费用确认 |
| 可信认证控制面 | 实验只用宿主外部假长期凭据文件与 mock 服务；文件不挂载、mock 对容器不可达 | 没有官方、完整验证的实际凭据控制面；mock 403 不证明原生 OAuth 撤销 |
| 不可信工具执行面 | 容器 UID 65532、无宿主 HOME、只读合成 bind、断网；代表性同 UID 子进程实际运行 | 与 app-server 同容器、同 UID 时仍能读 app-server 环境中的模拟 Access Token |

## 核心性质判定

| 性质 | 状态 | 实际证据/限制 |
| --- | --- | --- |
| 不读外部长期认证文件 | PASS_HOST_BOUNDARY_ONLY | 宿主假长期凭据未挂载，实际直接 fs 和链接访问都失败；真实凭据从未参与 |
| 不读控制面进程环境及私有状态 | **FAIL_SIMULATED_ACCESS_DOMAIN** | `fs/readFile /proc/self/environ` 和同 UID 代表子进程 `/proc/1/environ` 均观察到模拟 Access Token 标记 |
| 任意本地文件操作不能取认证材料 | NOT_PROVEN，不能晋升 | 外部假文件隔离成功，但同域 env 可读；没有证明 native 全部工具的额外边界 |
| 工具不能任意网络访问认证控制面 | PASS_HOST_BOUNDARY_ONLY | network none，已验证 mock 阳性对照与容器不可达；并非带模型网络的可用生产策略 |
| 失效/撤销/销毁阻止新请求 | PARTIAL | NAG 原有 Profile/Token 拒绝回归通过，假运行时销毁持久 failed、不重执行；mock 撤销只属模拟；真实 OAuth/云请求 NOT_TESTED |
| 官方协议支持必要分离 | NOT_VERIFIED_FOR_REQUIRED_ALL_TOOL_SCOPE | SIWC、ephemeral、remote Code Mode 各有文档，但未有当前版本全执行路径的正式且实证可用组合；不能推断通过 |

上述失败只使用固定字符串 `XCLI_G7_SIMULATED_ACCESS_NOT_A_REAL_TOKEN`，不具有 JWT/账号凭据性质。变量名为 `ACCESS_TOKEN` 以验证公开示例所使用的载体；**没有设置模型 provider、没有向认证 RPC 提交它、没有发网络推理请求**。因此不是“假 Token 也登录成功”的证据。

同 UID 子进程是代表性不可信执行进程，**不是模型触发的 Codex 内置工具**；直接 fs 是管理接口，未透传给 NAG HTTP 客户端。这个反例证明“只把 app-server 和工具放进同一容器”不足以完成要求的凭据分离，不能证明已绕过 Codex 内置沙箱。全部真实 builtin、内存型凭据读取、MCP、带云端出口的行为均 NOT_TESTED。

## 有界停止与未实现部分

当前要求是全工具执行面与认证域可靠分离。一次环境载体反例加上缺少当前版本全工具覆盖的正式证据，足以停止路线 B；不继续叠加 Canary，也不把环境过滤当成全部文件/内存边界的替代。独立 UID/namespace、远程 Code Mode 或服务端受控执行若未来被选择，必须先有新的产品范围、正式协议覆盖说明及独立验收，不能只把 `ACCESS_TOKEN` 改成文件或内存后宣告成功。

本人仍须确认：本人账号与授权、计划能力、仅个人本地开发用途、是否存在共享/商业/远程托管、合成文本云发送同意、内部模型次数和费用不能由一次进程精确限制。见 [CODEX_ACCOUNT_AND_USAGE_CONFIRMATION](CODEX_ACCOUNT_AND_USAGE_CONFIRMATION.md)。本轮没有要求邮箱、Token、认证文件或账户资料，也未替用户填写确认。
