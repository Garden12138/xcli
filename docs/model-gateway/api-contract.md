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

文本 MVP 配置见 `examples/model-gateway.yaml`。`model client issue <id> --model <alias> --token-file <new-private-file>` 交付 256-bit Token；不会打印。服务端只留 SHA-256 摘要。`rotate` 保留范围并使旧 Token 失效，`revoke` 对新请求立即生效；在途请求仍受原超时/取消约束。使用 `--allow-remote` 才能访问 remote/unknown 模型；`--require-consent` 客户端还需每个请求携带 `X-Xcli-Remote-Consent: allow`。`chat-insight` 身份强制要求该字段。此头不转发上游。

上游适配仅使用明确配置的 endpoint、API Key 引用；不继承环境 HTTP 代理，不跟随重定向。没有自动重试。JSON 请求 4 MiB，响应 32 MiB，SSE 单事件 1 MiB/全流 32 MiB，默认 3 并发/180 秒。每客户端默认 60 请求/分钟。超限明确失败。`/readyz` 仅报告本地配置而非实际上游额度/模型可用性。

协议核对依据：[Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)、[Streaming events](https://developers.openai.com/api/reference/resources/chat/subresources/completions/streaming-events)。本地适配保留合法 usage/tool 参数，不推断未知计费用量。

M2：Vision 单请求仍为 4 MiB JSON（base64 也计入预算），仅 text/image_url 内容部件；图片上传/结果单图 20 MiB、16,777,216 像素，总 multipart/图片结果 32 MiB，最多 4 张。先 DecodeConfig 再完整 Decode，支持 PNG/JPEG/WebP。编辑文件从 multipart 流有界读入内存，不用临时路径，客户端文件名不会落盘或转发。mask 必须是与第一张参考图等尺寸的 PNG；多图/mask 分别要求 image.multi_reference/image.mask，未经验证返回422。

图片 size 接受正数 WxH 或 auto（同时受像素限制），n=1..4；实际输出数量与指定尺寸必须匹配。只接收 b64_json 或原生默认 base64；请求 response_format=url 返回422，上游仅URL返回502，均不发起下载。配置真实供应商时应选支持该返回方式的模型；上游不支持的参数会明确失败，不模拟或自动换模型。协议参考：[Image edit](https://developers.openai.com/api/reference/resources/images/methods/edit)。WebP 解码来自 [Go x/image](https://pkg.go.dev/golang.org/x/image/webp)。

M3：Embedding支持字符串、字符串批量、token ID数组/批量（批次最多2048），float/base64结果严格透传，保留上游usage。模型必须配置`dimensions`与`index_version`；请求dimensions必须等于alias配置，另一维度使用独立alias。校验数量/index顺序/有限值/维度，拒绝空或伪造向量。目录与响应返回`index_fingerprint`，覆盖实际上游endpoint、模型名、维度和版本（不含密钥）；客户端检测变化并要求显式重建。推理位置是配置者的声明；loopback上游本身仍可能是云代理，配置为local前需独立核实。无法核实必须用unknown。

结构化JSON：能力门控后原样转发response_format/json_schema/strict。非流式结果还校验JSON语法；严格schema语义由经验证上游与消费端共同验收，网关不声称独立实现完整JSON Schema引擎。SSE由客户端收齐后解析JSON，网关不在内存累积全部内容。Tool Calling只传递函数声明与结果，不执行工具。audio/modalities非text请求明确422。
