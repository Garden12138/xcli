# Worklog 接入修改清单（未联调）

只读核对当前 `src-tauri/src/commands.rs::provider_defaults`、`llm.rs::build_chat_request`、`frontend/src/types.ts` 与 `main.tsx`；本轮没有修改外部仓库。现有非流式 Bearer 和 `choices[0].message.content` 可直接匹配网关。

1. `commands.rs::provider_defaults` 增加 `"xcli" => Ok(("http://127.0.0.1:18741/v1", "xcli/text-default", 180))`。默认 URL 仅初始化值，用户配置/部署发现的端口优先。
2. `frontend/src/types.ts::Provider` 增加 `"xcli"`；`main.tsx` 的 providerLabels/providerDefaults 和设置页 select 增加 xcli。显示“xcli 本机网关；推理位置由所选上游决定”，不能称所有数据在本机推理。
3. 本地 Token 沿用 Keyring；不使用上游 API Key，不改变日报/周报业务 prompt。增加授权的 GET /v1/models 检查服务和 alias；真实最小推理测试只能由用户主动触发。
4. 保留旧 provider、无配置 fallback、模板流程、超时配置；HTTP 401/403/429/502/504 保持失败并展示本地稳定错误，不当作生成成功，不把凭据或原文写日志。

实施前先核对外部 Git 状态与 HEAD，获得外部写权限；补丁需同时覆盖 Rust 默认值测试和前端设置选择。没有复制 Worklog 数据库或真实日志。

离线网关验收：`go test -race ./internal/modelgateway -run 'TestChat|TestRealSSE|TestSSE|TestGatewayBody'`。这些只证明协议和边界，不证明 Worklog 日报/周报真实成功。

真实验收待执行：日报、周报、模板导入/优化、日报 JSON、fallback、断网、额度错误、Token 撤销与 Keyring 隔离。需要非敏感合成材料及用户授权的请求预算；本轮请求次数为零。
