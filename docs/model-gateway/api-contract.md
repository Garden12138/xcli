# Model Gateway API v1

独立于 Task/Run/MCP；默认 loopback `127.0.0.1:18741`。配置使用独立 `model-gateway.yaml`，通过 `xcli model serve --config PATH` 指定。配置端口 0 可由 OS 分配端口。不会启动 Agent 或继承其工具权限。

`GET /healthz` 返回最小进程活性；其他接口要求每个 App 独立 Bearer Token。禁止所有带 Origin 的请求，无 CORS。Host 仅允许 localhost/loopback。错误为 `{"error":{"message":"...","type":"invalid_request_error","code":"...","param":null}}`。401 本地授权失败，403 权限/位置拒绝，404 模型或路由不存在，413 请求过大，422 能力不满足，429 并发/速率超限，502 上游协议错误，504 超时。

| 路由 | 合同 |
| --- | --- |
| GET /v1/models | OpenAI list，稳定 alias；只列 App 有权使用的模型；xcli 扩展见 capabilities.md |
| GET /readyz | 授权后的本地配置就绪情况，不代表真实上游登录/额度可用 |
| POST /v1/chat/completions | messages、model、stream；保留上游支持的参数。模型必须显式配置，无静默 fallback |
| POST /v1/images/generations | prompt、model、size、n、response_format；验证真实图片字节后返回 b64_json |
| POST /v1/images/edits | multipart image（或 image[]）、可选 mask、model、prompt；能力不支持返回 422 |
| POST /v1/embeddings | 透传向量，不生成替代向量；数量、index、维度需要一致 |

SSE 使用 `data: <chat.completion.chunk JSON>\n\n`，结束 `data: [DONE]\n\n`。保留 role/content/tool_calls、finish_reason 与合法 usage，不将未知 token 记为 0。必须收到真实上游 delta；断流报错且不发成功 [DONE]。客户端取消必须取消上游。stream/tool_calls/json_schema 按能力门控。

图片仅接收解码有效的 PNG/JPEG/WebP；Vision 使用受限 data URL，远程输入 URL 默认拒绝。禁止 file URL/路径读取。图片 URL 输出须受限下载验证，不能直接把上游鉴权 URL 交给客户端；初始实现允许只接收 b64_json 并明确拒绝 URL，不得伪造成功。尺寸/数量必须与请求一致。

此文件是实现合同，不代表每阶段或真实上游已验收；当前状态见 PROGRESS.md。
