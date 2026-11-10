# OAuth 管理设计

本设计与 CLI 原生认证独立。官方依据：[注册](https://developers.openai.com/siwc/token-sharing-open-source/sign-in)、[账号与会话](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions)、[Token reference](https://developers.openai.com/siwc/token-sharing-open-source/token-reference)。以下完整生命周期是目标设计；实现差异见文末。

## 首次启动、注册与回调

1. 先满足资格门禁及 OS 凭据库可用性，不可用立即拒绝。首次安装生成一次 opaque host ID，原子持久化后才允许登录。优先官方建议的公钥 JWK thumbprint URI；G8 离线合同采用官方同样支持的 `urn:uuid:` + UUIDv4，避免无用的私钥材料。重启、登录、刷新都复用该值；克隆安装需重新生成。
2. 显示明确的 Continue with ChatGPT 入口及实际产品名；说明计划消耗、云发送与用量管理。新账号/workspace 流程独立于当前有效账号，禁止先覆盖现用凭据。当前 `agent_name_hint=xcli` 只是离线单产品候选，不能证明覆盖四产品。
3. 先绑定 `http://127.0.0.1:<available-port>/auth/callback`，再开系统浏览器。每次生成独立的 256-bit state、nonce、PKCE verifier；`S256` challenge 使用 SHA-256/base64url 无填充。5 分钟有效，保存精确 redirect URI。只允许端口随登录改变，scheme/host/path 固定。
4. 首次使用 `client_id=dynamic_agent_client`；返回登录使用该注册保存的 issued client，且省略 agent name。携带稳定 host。请求 identity 与 plan scopes（代码常量 `Scopes`）以及 `resource=https://api.openai.com/v1`。不使用 client secret、伙伴 API Key 或 native CLI Token。
5. 回调限定 GET/固定 path/精确 Host/有限 query 长度、拒绝重复参数。先检查 state 和有效期，再处理拒绝/错误；错误时不换 code。首次必须收到 issued client，不能持久化 placeholder；返回登录可以省略 client，但显式不同必须拒绝。Code 一次性使用，回调页无 query 反射、无第三方资源、无 referrer，并立即关闭 listener。
6. 仅向官方 Token endpoint form POST authorization-code grant，携带 issued client、code、verifier、相同 redirect/resource。禁止重定向、环境代理、TLS 降级；网络错误不打印 URL/form。`invalid_grant` 丢弃 code，保留该次 issued client 以新授权流程恢复。

回访时可把此前验证的 ID Token 作为仅发给官方 authorize endpoint 的 hint；退出后清除。登录 URL 不能记录。本轮模拟不实现 hint，以避免增加无必要凭据流。

## ID Token 与权限验证

通过受信 OIDC discovery 获取 issuer/JWKS（只允许官方 HTTPS，限制重定向、大小和缓存有效期）；固定允许的算法，拒绝 `none`、HMAC 替换、未知 critical extension 和任意 token 提供的 key URL。kid 轮换只允许有界重取官方 JWKS。

验证签名、精确 issuer、issued client audience、必要的 azp、exp、iat/nbf 和本次 nonce；使用验证后的 sub。返回账号必须匹配原验证身份，不能按 email 合并。注册键 `{issuer,subject,client_id}` 区分同 email 的不同 workspace。workspace 取注册绑定，不从用户输入或 opaque access metadata 推导。

随后检查 **Token 响应的 granted scopes**，而不是 callback scope 或仅有 ID Token。缺少 `chatgpt.tokens.use.direct` 或 resource invoke 不能进入模型请求；保留 identity-only 登录状态并明确 plan disabled。本轮不提供 API Key 替代。用户以后主动启用计划权限才重新请求 consent；不会在每次启动强制 consent。生产需按官方 rollout 规则使用 consent 参数。

## 保存、刷新与账号选择

OS 凭据库中的单个原子记录保存：验证身份/issued client、Access/Refresh/retained ID Token、granted scopes、expires/saved time、earliest refresh、generation。非秘密注册索引只保存受限元数据/标签和 host ID，不放 Token。详情见 [安全边界](SECURITY_BOUNDARY.md)。

每个注册的刷新必须由单一 broker owner 和跨进程锁串行执行；重读最新 generation 后，只使用最新 Refresh Token。遵守服务端 earliest refresh 和到期时间，使用 issued client 和相同 resource，省略 scope。成功后原子替换 access、expiry、scope 和 rotating refresh，持久化成功前不使用新凭据；CAS 拒绝旧写入。刷新已发生而保存失败时停止新请求并要求恢复/重登，绝不能重新用旧 Refresh Token。

授权响应中的 expires 为准，不把文档示例常数当永恒配置。当前文档描述 Access Token 为一小时、Refresh Token 为可轮换的 30 天。刷新响应如含新 ID Token，生产需验证签名、issuer/aud/subject，按 OIDC refresh 的 nonce 语义处理；不能直接用初次 nonce 必填规则替代。

终结性 refresh 错误（例如 invalid_grant、invalid_refresh_token、expired/invalidated/reused）停止该注册、清除不可用 Token，保留 client/host 映射并要求重登。invalid_client 属配置错误。超时、临时网络和 5xx 保留记录，有限退避；不静默切账号/计费或自动重新执行模型请求。

## 退出、撤销与状态

退出先冻结该注册新请求、取消在途请求，调用 discovery 提供且已验证为官方 HTTPS 的 revocation endpoint，发送 Refresh Token、type hint 和 issued client。HTTP 200 空 body 表示确认；撤销成功后清除所有三类 Token、hint 和该注册历史，保留注册/client 与 host。临时错误可有限退避；用户选择本地退出时也清除 Token，但标明 **remote_revocation_unconfirmed**，提示可到 ChatGPT Settings 断开。不能谎称已远程撤销。

没有服务端主动撤销通知保证；下次 request/refresh 确认失效时收敛为需登录。普通 403/503 不等于已经撤销，不随意擦除有效凭据。

控制面查询仅返回 signed-in、plan enabled、expired/refresh-required、secure-store availability、revocation state、catalog/inference verification 等安全状态。四应用不能查询 Token、原始 ID claims、账号邮件或授权 URL。模型目录通过选定注册的 Token 获取；换账号失效缓存，仅列可显示 slug/name，不承诺该次推理权限或 Vision 已验收。

## G8 实现与模拟边界

已实现可离线执行的 PKCE/回调状态机、RS256 受信公钥验证、初次 code 响应验证、scope、过期状态、轮换记录和 form 契约。测试生成独立 RSA key，Mock authorize/token/revoke/models/responses 全程只用合成记录；不存在生产 callback listener、浏览器启动器或真实登录命令。

`SecureStore` 的内存实现只在测试中；CAS 测试不是跨进程锁证明。`OpenSystemStore()` 始终报不可用；没有真实 host 持久化、discovery/JWKS 拉取、Keychain、刷新的定时任务或撤销重试管理器。本轮 `Rotate` 只接受保留原 ID Token（响应可省略），新 ID Token 返回 validation-required 并拒绝；不把简化契约冒充完整生产 OIDC。
