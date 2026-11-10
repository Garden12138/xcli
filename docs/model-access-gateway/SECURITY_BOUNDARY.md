# 安全边界与本地凭据方案

G8 没有获取、读取或持久化真实凭据。安全方案优先 OS 凭据库；当前适配未实现，所有平台拒绝打开 `OpenSystemStore()`。没有 0600 明文、YAML、环境变量或内存生产 fallback。

## 凭据与组件

| 材料 | 未来存放/持有者 | 不得出现的位置 |
| --- | --- | --- |
| OAuth access / refresh / retained ID token | 专用 broker 内存和 OS 凭据库中的一个原子记录 | 业务 HTTP response、普通配置、日志、Shell/Agent/MCP 环境、argv、临时文件、浏览器存储 |
| PKCE verifier、state、nonce、callback code | 单次授权内存，完成/超时即清理 | 日志、分析系统、业务端点；code/状态仅经必要 OAuth 回调使用 |
| issued client、验证身份、注册标签 | OS 库及最小私有注册索引 | 公共目录和跨客户端状态泄露 |
| host ID | 安装级私有非秘密元数据，写入先于首次授权 | 不把它当凭据、设备 attestation 或用户标识 |
| 本地每应用 Token | 各应用自己的安全存储；前门存摘要 | OAuth 存储域与上游 Authorization |
| 媒体上游凭据 | 独立 Media Provider 安全配置域 | SIWC 验收、SIWC 模型请求 |

不能读取或复用任何 Codex CLI 认证文件；也不探测 `~/.codex`、全局 keyring、现有 CLI 环境变量。禁止通过 app-server/env_key 传 Token。G8 生产包没有 os/net/http/os-exec 依赖，也没有 Token 导出 API；离线源码约束测试检查直接依赖。这是本阶段的能力裁剪，不是对未来任意依赖的 OS 隔离证明。

## macOS Keychain 方案

未来使用原生 Security.framework 的 SecItemAdd/CopyMatching/Update/Delete，Generic Password service 固定为 xcli SIWC broker 的专用 service，account 使用私有注册键摘要。整个 rotating token set 编码为一个 record 原子更新；host ID 元数据与 Token 分开。ACL 绑定专用签名身份，默认不共享 access group/同步/iCloud 导出；按本机保护策略处理用户解锁。

不使用 `/usr/bin/security` 子进程传入 Token，也不采用把密码写入命令行的包装库。专用 broker 不能启动任意子进程、执行模型工具或加载第三方代码。开发版未签名 xcli 与最终签名 broker 的凭据访问必须独立验收，不能把“macOS 有 Keychain”当作本工程已经安全接入。

Keychain 被锁、用户拒绝、entitlement/签名变化、不可访问或 API 错误时停止。没有后台放宽 ACL，也不提示将凭据改存 YAML。需要在不接触本人真实账号的独立合成 Keychain 条目上验证重启、轮换、锁定/拒绝、删除、升级和恢复；本轮没有创建这些条目。

## 其他平台

Windows 可评估 Credential Manager/DPAPI 的本机用户与应用访问边界；Linux 可评估 Secret Service 的受信会话和存储后端。必须独立测试访问控制、持久化和锁定行为；无安全服务的 headless 平台 fail closed。G8 没有跨平台生产适配，不用普通文件模拟安全库。

生产库需要跨进程独占 session refresh、CAS generation 和单记录替换。纯内存 CAS 测试仅验证契约，不证明 OS 库原子性或并发锁。若服务端已轮换但本地提交失败，停止使用该记录，保留安全错误状态并进入受控恢复，不重复旧 refresh。

## 数据、错误与审计

OAuth 与模型请求只发固定官方 HTTPS；禁止外部重定向、任意上游配置和代理继承。inline Vision 不触发 URL 下载；应用内容不能更改策略或执行工具。模型端可能处理用户数据，必须按应用和每次请求的 remote consent 管理，不因前门 localhost 而声称本地推理。

只记录本地请求 ID、固定路由、应用 ID、状态、耗时和稳定错误类别；不记录 auth URL、headers/form、原始 claims、prompt、图像、完整上游错误或 response dump。`Secret`/Credential/Attempt 格式化与 JSON 序列化做脱敏。显式读取内部 Secret 仍可接触明文，脱敏类型不等于不可窃取内存；Go 的 GC 也不能保证秘密字节已物理清零。

历史按应用+注册+会话隔离、有 TTL/上限，退出立即删除。没有跨业务拼接上下文，没有持久化明文对话日志。撤销暂时失败不得报告成功；网络临时故障不得擅自清除账号或切换商业 API。

## 已覆盖与未覆盖

离线覆盖 state/nonce/PKCE、签名/audience/issuer/subject、回调重放、scope 下降、旋转记录、旧写入拒绝、格式化脱敏、工具/URL/参数拒绝、history 跨客户端隔离、SSE 失败及取消。Mock 的外部 Dial 和 redirect 都被阻断。

尚未证明：官方资格、同意覆盖范围、真实 OS 凭据库、签名/ACL、生产 OIDC key rotation、跨进程 refresh、崩溃恢复、专用 broker 包装及真实账号/模型权益。当前只靠门禁保留安全状态，不能宣称全部生产隔离问题已解决。
