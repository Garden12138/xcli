# Chat Insight 接入修改清单（发布阻断仍在）

只读复核 `src-tauri/src/ai/client.rs` 与 `ai/settings.rs`：现有 `is_local()` 仍只看URL、`auto_scan_commitments` 仍按本地endpoint启用；本轮没有写入应用、读取聊天数据库或调用真实模型。

接入顺序与精确修改点：

1. `ai/settings.rs::AiSettings` 增加 provider=`xcli` 和受信任端口配置；Keyring存独立Token，不使用用户任意局域网地址。`is_local` 继续表示传输地址，新增每模型`effective_cloud`表示推理位置；两者不能混用。
2. `ai/client.rs::OpenAiCompatClient` 授权读取 `/models`，分别解析当前chat/embedding模型的`xcli.inference_location`。metadata缺失、目录失败、模型缺失或位置unknown均默认cloud；在此之前不创建任何携带聊天正文的请求。
3. `settings.rs` 的allow_cloud检查、敏感原文确认与后台扫描判断，全部改用effective_cloud。切换模型/路由需重新检查，禁止沿用endpoint的旧判断；不要自动勾选云授权。仅用户已同意该请求时附`X-Xcli-Remote-Consent: allow`。为`chat-insight`签发的网关Token强制逐请求同意，网关remote/unknown路由无头即拒绝，且拒绝发生在上游调用前。
4. embedding索引保存model alias、dimensions、index_version、index_fingerprint；任一变化要求显式重建，不自动迁移/覆盖旧索引。网关响应的xcli标识与目录不一致时丢弃结果，处理请求间配置变化。
5. 保留现有SSE消费/取消和原有直连provider。embedding缺失503要明确提示；不能生成替代向量。不要以本机网关URL让后台聊天数据绕过确认。

可执行参考：`examples/model-gateway/client.py` 包含严格loopback端点、目录授权、有效推理位置、索引身份、非流/SSE/Embedding适配；是独立参考，**尚未移植到Chat Insight Rust/React流程**。

离线验收：`python3 -m unittest discover -s tests/model_gateway -v` 与 `go test -race ./internal/modelgateway -run 'TestEmbedding'`。覆盖拒绝授权/未知位置不调用上游、批量顺序/维度、base64向量、旧索引身份变化、缺少向量服务。只使用合成文本。

真实Gate未执行：应用连接、真实索引/知识检索/SSE取消、敏感确认/拒绝与后台扫描、实际模型维度切换。需要对应仓库写权限并完成应用回归后才可正式接入云端xcli；本轮不宣称修复已部署。
