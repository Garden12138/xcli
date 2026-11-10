# G8 实施与后续门禁

## 已完成范围

本轮仅完成官方研究、八份文档、总体架构更新、纯协议模块、Mock OAuth/Responses、离线安全及契约回归、生产禁用门禁。没有 G7 新隔离实验，没有真实 OAuth/Token/推理，没有改动业务应用、部署或 Push。

代码索引：

| 文件 | 内容 |
| --- | --- |
| `internal/siwc/policy.go` | 常量、Secret 脱敏、OS 凭据库接口与 fail closed。 |
| `oauth.go` / `identity.go` | PKCE、回调状态、受信 RSA key 的 ID 验证、scope、轮换/撤销 form 契约。 |
| `responses.go` | 独立 ResponsesProvider 接口、文本/Vision 白名单、SSE、错误与 SIWC 目录。 |
| `history.go` | 内存、完整显式消息、owner/注册隔离与 TTL/容量。 |
| `*_test.go` | 全合成 OAuth 生命周期、Mock Responses、负向安全和真实 loopback 取消测试。 |
| `internal/modelstore/siwc_test.go` | 保留 Provider 类型不可启用、不可用 API Key 替换。 |
| `internal/modelgateway/siwc_test.go` | 没有 Token/Login 路由，OAuth Token 不能当本地应用授权。 |

## 下一阶段顺序

1. **官方资格确认**：明确多应用、本地、同用户、非公网架构是否允许，统一或每产品 client、代理持有 Token、host 定义和商业边界。没有确认不写可启用的 Provider。
2. **专用 broker 与凭据库**：明确独立签名服务、原生 Keychain API/ACL、host 与注册索引持久化、刷新跨进程锁、原子提交及崩溃处理。先用独立合成凭据验收，不能读原有认证。
3. **生产 OAuth 控制面**：受信 discovery/JWKS、真实 key rotation、bounded callback HTTP server、安全系统浏览器启动、完整 refresh OIDC 语义、身份/权限降级、退出和远程撤销未确认状态。仍可在本地 Mock 完整验证。
4. **生产 Responses transport**：仅官方固定 origin，专用注册绑定；补真实错误 status/request ID/安全 param 诊断。启用前证明 no tools/no subprocess/no token egress；有界请求、取消、history 和账号切换一致性。
5. **有界真实授权验收**：上述门禁完成后，必须由用户另行授权真实 OAuth 与最小文本/图片输入调用范围；该授权不从本轮任务继承。先确定账号/workspace/地区和费用/计划提示。目录与成功终态分开记录，不伪造 Vision 权限。
6. **独立应用与媒体适配**：另立四应用改造范围及 Chat/Responses 映射；各自客户端安全存储、权限、UI、费用和 remote consent。GardenFlow 媒体调用继续独立验收，不用其结果补 SIWC 验收。

免费本地 OSS 分发与收费/远程产品不能共用一个未审结的资格结论。没有真实启用命令、预计 GO 日期或自动完成后的部署动作。

## 生产不可启用条件

配置类型为 `siwc-responses` 时，`modelstore.Decode` 返回 `siwc_eligibility_blocked`，不解析密钥引用或创建网络客户端。不存在 `enabled=true`、环境变量 override 或 app-server/API-Key fallback。下一阶段即使收到官方答复，也必须先实现并复核工程门禁；删掉错误分支不等于完成实现。
