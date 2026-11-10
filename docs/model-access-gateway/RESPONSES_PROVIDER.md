# SIWC Responses Provider 契约

Provider 类型保留为 `siwc-responses`。当前是纯契约接口 `siwc.ResponsesProvider`，**没有生产实现**；配置始终拒绝，既不注册到 `ChatBackend`，也不伪装 `ImageBackend`。资格未明时不提供可启用骨架或隐蔽开关。

官方依据：[Models and inference](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)、[Preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)、[Errors and recovery](https://developers.openai.com/siwc/token-sharing-open-source/errors-and-recovery)。

## 目标传输

生产路径仅允许直接 `POST https://api.openai.com/v1/responses`，模型目录为相同账号的 `GET /v1/models`。不使用 ChatGPT backend-api、Codex 登录、app-server、Shell、MCP、Files upload 或其他工具服务。OAuth Bearer 只在 broker 内部附到固定官方 TLS 请求，不转发给本地 HTTP 消费者；不能用请求头替换固定注册。

未来 HTTP transport 应关闭环境代理和重定向，限定官方 origin、方法、path 与 TLS；拒绝 caller-supplied URL/headers。使用连接/首包/总时限，客户端断连或 context 取消关闭上游 body。没有自动重试或计费 fallback。G8 唯一 HTTP 客户端在测试文件中，DialContext 只允许 loopback，模拟 redirect 指向官方地址时也不跟随。

## 请求

每次发送完整上下文数组，固定 `store=false`、`stream=true`。最小合成示例（不是可运行生产请求）：

```json
{
  "model": "synthetic-model",
  "instructions": "Answer the selected user's question.",
  "input": [
    {"role": "developer", "content": "Use the supplied material."},
    {"role": "user", "content": "Summarize this synthetic paragraph."}
  ],
  "store": false,
  "stream": true
}
```

G8 顶层仅允许 model、instructions、input、store、stream。未知字段一律拒绝，绝不把不支持的参数静默丢弃后假装语义相同。完整请求不超过 4 MiB，消息数 1–4096；可用 developer 消息或 instructions，显式 system message 拒绝。模型必须经注册绑定和目录选择，不能硬编码文档示例模型为已有权益。

官方当前明确不支持的字段包括 background、conversation、max_output_tokens、max_tool_calls、metadata、moderation、multi_agent、prompt、prompt_cache_retention、safety_identifier、temperature、top_logprobs、top_p、truncation、user；HTTP previous_response_id 也不得发送。G8 使用更窄白名单，连未验收的可选 text/reasoning 控制和 service tier 也不接受。

不支持 image_generation、File Search、Code Interpreter、Native Computer Use、Hosted MCP/Connectors、tool_search。本阶段进一步禁止所有 tools，包括函数、自定义和 web search；官方在特定条件支持某些工具不意味着 xcli 启用它们。发现工具输出也返回错误，绝不执行或发送后续 tool result。

## Vision 与媒体分离

user content 支持 `input_text` 和 `input_image`；图片使用完整 PNG/JPEG/WebP 的 inline data URL，复用真实字节/MIME/像素/大小校验。拒绝网络 URL、本地路径、伪图、MIME 欺骗；不读取文件或自动下载。Vision 依赖所选模型和账号权限，本轮只验证输入合同，无真实视觉理解结果。

assistant 历史为明确文本，developer/user 为文字和允许的图片输入。Files、audio/video 和 opaque reasoning 的广义输入不在 G8 契约内。**图片输入 → 文本分析**不能描述成图片生成。

独立 `model.ImageBackend` 继续定义 Generate/Edit；现有 `/v1/images/*` 与 SIWC 不共享 Provider 或 OAuth。GardenFlow 后续可以选择已有 Qwen-Image 2.1 API 生成/编辑，使用它自身的凭据、费用、参数和能力验收。本轮未调用该 API，也不声称 xcli 新增或验证了其具体适配。

## SSE、终态与错误

`ConsumeStream` 验证 SSE 记录、sequence 单调、创建/完成 response ID 和终态；支持 CRLF、注释、跨网络分片、多行 data。流总计至多 32 MiB、单行至多 1 MiB。只交付受控的 text delta 与完成文本，不把原始 response 对象或工具数据透传给应用。

必须收到合法 `response.completed` 且 status completed 才成功。无终态 EOF、`[DONE]`、不完整 JSON、顺序错乱、failed/incomplete 和未知工具输出均失败。中途文本是暂存输出；只有最终成功才提交历史。生产 reader 必须由可取消 HTTP request 拥有；纯 parser 不能中断任意不可取消 io.Reader。

错误分类：

| 条件 | 处理 |
| --- | --- |
| 401 / 身份或 scope 不被接受 | 停止该请求，检查注册与授权；确认撤销或终结刷新错误后重登。 |
| 403 / user not eligible、policy、route、scope context | BLOCKED/权限拒绝；不循环 OAuth，不换账号绕过。 |
| 429 / usage limit | 暂停新请求，提供 ChatGPT 用量管理入口；不猜剩余额度或 reset 时间。 |
| 503 / route/user/usage unavailable | 保留凭据；未来由显式策略有限退避，不自动重放生成。 |
| unsupported capability | 本地拒绝或报告稳定错误；不带同一无效 body 重试。 |
| timeout / cancellation | 区分 context deadline 与 cancelled；关闭上游，历史不前进。 |
| 流中错误或断流 | 保持失败，不能因为收到若干 delta 而记成功。 |

已识别的官方错误 code 原样保留并映射类别；任意上游 message/detail/param 不进入应用错误或日志。生产需保留实际 HTTP status、受限 request ID、结构形态及经过白名单校验的 param 到受保护诊断记录；**G8 尚未实现这层生产诊断**。`UpstreamError` 只是收敛/脱敏契约，不等于完整官方诊断保真。

模型目录按 SIWC `models` 数组（不是普通 API Key 的 data 数组）解析、过滤 visibility=list，保留顺序/slug/display name。目录不会标为 inference_verified，也不能证明 Vision、区域或实时额度。当前不发送探测推理。

## HTTP 兼容性

本轮不新增 `/v1/responses`。未来资格确认后优先提供独立 Responses 合同；既有四应用使用的 Chat Completions 格式需要另行设计明确的映射、流语义及不支持字段拒绝矩阵。不能把任意 Chat 参数/工具/history 串成提示词并宣称兼容。四应用本轮完全不改动。
