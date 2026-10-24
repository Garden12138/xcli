# 桌面应用接入与验收

目前仅 xcli 工作区获写入授权。外部应用没有在本轮修改或启动；以下均为待联调，离线契约测试不能替代真实应用/模型验收。

| 应用 | 接入点与必须保留边界 | 真实 Gate |
| --- | --- | --- |
| Worklog | provider_defaults 增加 xcli；base /v1；本地 Token 存 Keyring；沿用业务 prompt | 日报、周报、模板导入/优化、JSON、fallback、断网、额度错误 |
| GardenFlow | 独立文本与 openai-images Source；模型能力与位置 UI；保留 Pi/媒体确认 | 文章/续写、中文 JSON、取消、Vision、封面落库、参考图编辑、工具流 |
| Chat Insight | /models + SSE + embeddings；effective_cloud 不能只看 endpoint；按模型/维度隔离索引 | 连接、批量索引、语义检索、取消；拒绝云授权不发送原文；未知路由不自动上传 |
| Contribution OS | 受控 xcli_local Provider；保留内部 Gateway、短期作用域 Token、冻结 Profile、普通公共 HTTPS 校验 | Plan/Review/JSON/Tools；重复/过期 Token 拒绝；沙箱/恶意 URL 拒绝 |

外部仓库访问/修改权限、付费模型请求与各 OS 实机验收分别记录，不以 curl 200 宣称集成通过。后续 M4 提供可执行验收程序与精确修改清单。
