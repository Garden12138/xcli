# 实际支持矩阵（2026-11-07）

“实现并离线通过”与“真实已验收可用”分开。新真实模型调用 0；不推断账号额度、许可或费用。

| 能力 | Codex exec | Antigravity print | Cursor headless |
| --- | --- | --- | --- |
| 正式程序集成资料 | 已核查公开 exec/SDK/app-server | 已核查公开 headless；SDK 认证路线分开 | 已核查公开 headless/权限 |
| 固定 argv/恢复与解析代码 | 实现；假子进程通过 | 实现；假子进程通过 | 实现；假子进程通过 |
| split/terminal/会话观察 | 假进程通过，缺/错 ID 封存 | 假进程通过，soft-deny 不成功 | 假进程通过，trust 优先分类 |
| HTTP→CLI 新链路 | 真 HTTP→假 Codex 协议进程通过 | 协议/进程夹具通过；真实 HTTP 未验 | 协议/进程夹具通过；真实 HTTP 未验 |
| 既有原生 Gate A | 历史文本/续接/生成/编辑 PASS | 历史同上 PASS | 历史 workspace trust BLOCKED |
| 当前服务策略 | **policy_unenforceable，关闭** | **policy_unenforceable，关闭** | **policy_unenforceable，关闭；trust 另阻断** |
| 真实 HTTP 文本/续接/事件 | NOT_TESTED / BLOCKED | NOT_TESTED / BLOCKED | NOT_TESTED / BLOCKED |
| 真实取消/超时/并发 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 真实图片安全路径/来源/视觉交付 | NOT_TESTED / BLOCKED | NOT_TESTED / BLOCKED | NOT_TESTED / BLOCKED |
| 原生 delta/严格 Schema/Vision | delta/Schema=false；Vision 不提供 | 同左 | 同左；未开 partial |
| 原生 Embedding/tool approve | 不提供 | 不提供 | 不提供 |
| 认证/账户/条款适用性 | 未重新检查账号；不读取凭据 | 同左 | 同左；不修改 Trust |

完整的 HTTP 图片 upload→Task→review→download 闭环在假客户端/夹具上通过，不能归给任一真实厂商的媒体能力。旧 Gate A 图片不作为本次生成证据，也未拿来替代新结果。

当前验证平台：macOS arm64。Linux amd64/Windows amd64 仅交叉构建，不等于实机通过；Windows 媒体文件读取因缺少 reparse/handle parity 验收明确关闭。

[官方路径与条款核查](OFFICIAL_PROTOCOL_REVIEW.md)、[分项验收证据](IMPLEMENTATION_REPORT.md)、[真实新授权计划](REAL_HTTP_ACCEPTANCE_PLAN.md)、[四应用后续入口](MIGRATION_AND_COMPATIBILITY.md)。
