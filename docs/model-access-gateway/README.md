# G8 — SIWC Model Access Gateway

> R4 / 2026-11-10：此模块冻结，默认关闭，不再作为 xcli 核心产品目标或可用性门禁。停止后续真实验收/OAuth/模型服务开发；下文仅保留历史方案和证据。当前入口见 [R4](../product-realignment/README.md) 和 [模块处置](../product-realignment/MODULE_DISPOSITION.md)，不按历史计划自动恢复实验。

2026-11-10，基线 `c03af29` / `e8cf6b4`。阶段结果：**OFFLINE_CONTRACTS_PASS / PRODUCTION_BLOCKED**。准确的生产门禁值为 `BLOCKED_ELIGIBILITY_AND_ARCHITECTURE`。

目标路径是应用自身获得明确授权的 SIWC OAuth → 官方 Responses API → 本地 Model Access Gateway。G8 与 Native Agent Gateway 独立；G7 停在 `PARTIAL_HOST_ISOLATION_ONLY`，真实 Profile 继续默认关闭。本轮没有延续容器、Seatbelt 或普通安全探针实验。

官方明确描述了本地开源客户端的直接 Responses 路线。但没有明确批准“多个独立产品共享 xcli 一个 OAuth 注册”的代理架构。**不能把本地、开源或个人使用推导成该架构已获资格。** [资格审查](SIWC_ELIGIBILITY.md)逐项区分官方事实、工程建议和待确认项。

本轮完成：

- [职责与信任边界设计](ARCHITECTURE.md)、[完整 OAuth 生命周期设计](OAUTH_DESIGN.md)、[凭据方案](SECURITY_BOUNDARY.md)。
- `internal/siwc` 的纯协议契约：PKCE、回调、RS256 身份校验、scope、记录轮换、严格请求、SSE、目录和有界历史。
- 仅存在于 `_test.go` 的 Mock OAuth / Responses HTTP Server、内存凭据库与 loopback 客户端。
- Model Gateway 识别保留类型 `siwc-responses` 后明确拒绝配置；不存在可打开的生产开关，也没有 SIWC Token HTTP 接口。
- [Responses Provider 契约](RESPONSES_PROVIDER.md)、[验收证据和最终八项决策](ACCEPTANCE.md)、[后续实施门禁](IMPLEMENTATION_PLAN.md)。

没有实现生产 Provider、真实浏览器登录、真实 Token、Keychain 适配或模型推理。`OpenSystemStore()` 在所有平台 fail closed；测试内存库不会进入生产构建。契约通过不能升级为真实 OAuth 或权益验收通过。

完整离线回归：

```bash
bash tests/model_access_gateway/verify-offline.sh
```

需要本机 loopback/socket 以及 Go/Python 测试环境；使用合成 Token 和假 CLI fixture，既不启动真实 Agent CLI，也不执行模型生成的工具。本轮不修改四个应用，不部署、不 Push、不 Tag/Release。

Media 保留 `model.ImageBackend` / `media` 边界。GardenFlow 以后仍可使用其已有 Qwen-Image 2.1 API 生成/编辑图片；SIWC Vision 只接收图片并生成文本。本轮没有调用、验证或迁移该媒体上游。
