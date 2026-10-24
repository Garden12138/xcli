# xcli 模型服务化升级：现状审查与分阶段实现方案

> 版本：设计稿 v1.0  
> 审查日期：2026-10-24  
> 适用对象：xcli、GardenFlow、Worklog、Chat Insight、AI Open Source Contribution OS  
> 本文性质：**基于 GitHub main 分支源码的静态审查及现有测试记录的分析；未实际登录模型账号、运行四个桌面应用或完成端到端联调。**

## 0. 执行摘要与最终决策

**结论：当前 xcli 尚不能直接充当上述四个应用可复用的文本/图像模型服务。** xcli 已有原生 Coding Agent 适配、Task/Run/Session、流事件解析、取消、后台及验收等资产；但缺少面向桌面应用的模型 HTTP 服务、能力分流、标准多模态消息、图片数据输出、统一安全与流式兼容契约。历史图片生成验收是 Agent 使用工具生成文件，不代表有文生图/图生图 API。

**建议：保留现有 xcli Task Runtime，新增独立 `Model Gateway`，通过 `xcli model serve` 向受控桌面客户端提供 OpenAI-compatible 接口。**

1. **模型 API/本地推理后端**承担文本生成、视觉理解、图片生成/编辑；xcli 负责适配与路由，不必自己运行模型权重。
2. **原生 Agent CLI**仍承担 Coding Agent 工作及 Task 执行。只有通过安全、兼容与条款检查的部分 CLI 文本能力可作为实验性后端；绝不默认把会员登录凭据封装成可被其他应用随意调用的通用 API。
3. **Embedding** 不开发新的向量推理引擎，但为了让 Chat Insight 现有功能实际可用，在第一阶段提供 `POST /v1/embeddings` **严格转发**至独立、已配置的向量服务；否则应明确保持 Chat Insight 原有向量服务配置，不能称已完成整合。
4. **先解决服务接口与安全，再做图片编辑和应用接入，最后按验收结果考虑 CLI 受限桥接。** 与 R3 是否正式发布分开跟踪；建议新阶段称 **M0–M5**，避免误用尚未批准的 R4 名称。

### 范围边界

| 纳入本期 | 暂不纳入 |
| --- | --- |
| 文本生成、文本流式输出、受支持的结构化 JSON | 音乐、语音、视频生成 |
| 图片输入理解（Vision）、文生图、图片编辑 | 把 CLI 无法保证的媒体能力宣传成已支持 |
| 已有服务的 Embedding 转发，满足集成前提 | 自研向量模型、自动重建整套 RAG 引擎 |
| 本机可信应用接入、配置、路由、限流、安全审计 | 公网多租户售卖、会员额度共享、账号池轮转 |
| 对已有原生 CLI Task 的可选关联 | 任意桌面应用通过提示词获得 shell/文件系统权限 |

## 1. 审查基线、方法与证据等级

以下 commit 是 **2026-10-24 读取时各仓库 `main` 的 HEAD**。修改继续推进后应重新锁定 commit 并复核变更。源码证据优先于历史 README；文档或测试报告的“通过”仅反映其标注环境，不等于本次重新实测。

| 仓库 | 审查 commit | 核心证据 |
| --- | --- | --- |
| `xcli` | `8db421f1baf0afd6ce07dac309310a4844ea0a13` | `cmd/root.go`、`cmd/run_use.go`、`internal/agent/agent.go`、`internal/agent/stream.go`、`internal/mcp/serve.go`、`docs/r0/results.zh-CN.md`、`docs/r3/compatibility.zh-CN.md` |
| `garden-flow` | `4bb035c5ad37c1484da6b91bd78ec7a23dd5c3b5` | `Docs/AI_PROVIDERS.md`、`desktop/electron/core/aiModelRouteResolver.ts`、`imageProviderAdapters.ts`、`imageGenerationService.ts`、`desktop/electron/pi/agent.ts` |
| `worklog` | `861156e987f97ad52b59daae8cd7ec8b9f02fea1` | `src-tauri/src/llm.rs`、`src-tauri/src/commands.rs`、`src-tauri/src/secrets.rs` |
| `chat-insight` | `b20ad1e276f3b5566585be50f96549171dbf107c` | `src-tauri/src/ai/client.rs`、`ai/settings.rs`、`ai/commands.rs`、`src/ai.ts` |
| `ai-open-source-contribution-os-summary` | `73e6341d8aebb72cc7ba4d35150ef301b2185223` | `app/providers/gateway_http.py`、`studio.py`、`codex_cli.py`、`app/model_settings.py` |

**证据等级**：
- **S（源码确认）**：存在明确的数据结构/路由/限制代码；本报告多数技术判断属于 S。
- **D（文档/既有验证记录）**：仓库自行记录某平台、版本下的真实结果；不是我在本轮重新执行。
- **I（方案推断）**：依赖具体模型、授权、外部环境或下一版本实现，必须单独验收。

参考固定版本链接：
- [xcli 命令入口](https://github.com/Garden12138/xcli/blob/8db421f1baf0afd6ce07dac309310a4844ea0a13/cmd/root.go)；[Agent 适配器](https://github.com/Garden12138/xcli/blob/8db421f1baf0afd6ce07dac309310a4844ea0a13/internal/agent/agent.go)；[结构化解码](https://github.com/Garden12138/xcli/blob/8db421f1baf0afd6ce07dac309310a4844ea0a13/internal/agent/stream.go)；[原有 MCP Serve](https://github.com/Garden12138/xcli/blob/8db421f1baf0afd6ce07dac309310a4844ea0a13/internal/mcp/serve.go)
- [GardenFlow 图片 Adapter](https://github.com/Garden12138/garden-flow/blob/4bb035c5ad37c1484da6b91bd78ec7a23dd5c3b5/desktop/electron/core/imageProviderAdapters.ts)；[AI 供应商说明](https://github.com/Garden12138/garden-flow/blob/4bb035c5ad37c1484da6b91bd78ec7a23dd5c3b5/Docs/AI_PROVIDERS.md)
- [Worklog LLM 调用](https://github.com/Garden12138/worklog/blob/861156e987f97ad52b59daae8cd7ec8b9f02fea1/src-tauri/src/llm.rs)
- [Chat Insight LLM/Embedding 客户端](https://github.com/Garden12138/chat-insight/blob/b20ad1e276f3b5566585be50f96549171dbf107c/src-tauri/src/ai/client.rs)；[云端授权逻辑](https://github.com/Garden12138/chat-insight/blob/b20ad1e276f3b5566585be50f96549171dbf107c/src-tauri/src/ai/settings.rs)
- [Contribution OS 本地模型网关](https://github.com/Garden12138/ai-open-source-contribution-os-summary/blob/73e6341d8aebb72cc7ba4d35150ef301b2185223/app/providers/gateway_http.py)；[URL 校验](https://github.com/Garden12138/ai-open-source-contribution-os-summary/blob/73e6341d8aebb72cc7ba4d35150ef301b2185223/app/model_settings.py)

## 2. xcli 现有能力与缺口

### 2.1 可复用资产

- `cmd/root.go` 注册 `task`、`agents`、`doctor`、`run`、`use`、`resume`、`runs`、`jobs`、`config`、兼容用 `mcp`；**未注册 `model serve`、`serve` 或 HTTP 模型 API**。
- `internal/agent/agent.go` 支持 `codex`、`cursor`、`antigravity`、`claude`、`gemini`、`opencode` 及 `generic`；具备原生命令构造、恢复和安装探测。
- `internal/agent/stream.go` 解析原生 JSON/JSONL、运行结果、session、用量、失败/重连；`RunResult.Output` 为文本，不是标准 OpenAI ChatCompletion；`MaxOutputBytes = 8 MiB`、`MaxEventBytes = 8 MiB`，不适合作大体积图片通道。
- `internal/runtime`、`internal/runstore`、`internal/task` 已有执行、环境、记录、取消、任务边界；可以复用诊断抽象，但**不能直接把 task 计划/修复/验证循环当一次 chat completion**。
- `xcli mcp serve` 只负责启动已配置的 **stdio MCP 子进程**，既不是 MCP 模型代理，也不是 HTTP AI Gateway。
- 历史 `docs/r0/results.zh-CN.md` 报告了多 CLI 借助本身工具的图片生成和编辑验收；这是**文件产物级能力**，不能推导出稳定的 `image/png` 字节流、尺寸参数、参考图、并发和跨平台兼容。

### 2.2 能力差距矩阵

| 能力 | 当前 xcli | 升级目标 | 验收要求 |
| --- | --- | --- | --- |
| 文本任务 | 有，CLI Task/Run | 保留 | 原有命令与数据不回归 |
| HTTP 模型接口 | 无 | `/v1/chat/completions` | SDK 与四应用联调 |
| SSE 文本流 | 有内部 CLI 事件，不是 ChatCompletion chunks | 标准 Chat SSE | 连续 delta、done、取消可测 |
| 多轮 messages | 原生 session/Task，不等价于传入 messages | 无状态消息映射 | role、顺序、system 消息正确 |
| Tool Calling | CLI 内部工具≠客户端 `tool_calls` | 能力门控透传 | tool_call_id、arguments、finish_reason 一致 |
| 结构化 JSON | 原生结果可解析，不等于 schema 约束 | 支持上游能力时透传 | 无法保证时拒绝而非伪造成功 |
| Vision 输入 | 不存在标准图像输入网关 | 多模态消息转发 | data URL / URL 策略、限额、泄露防护 |
| 文生图 | 无标准 API | `/v1/images/generations` | 图片字节 MIME、尺寸、数量一致 |
| 图生图/编辑 | 无标准 API | `/v1/images/edits` | multipart、参考图、mask 按支持程度验证 |
| Embedding | 无 | `/v1/embeddings` 原样转发 | 维度、数量、顺序和模型稳定 |
| 模型枚举 | CLI 枚举 Agent，不是模型目录 | `/v1/models` + 能力说明 | 不宣传未经验证能力 |
| 本地应用鉴权 | CLI 使用自身凭据 | 独立本地客户端令牌 | 非授权进程不能提交任务 |
| 上游路由/健康 | 原生 Agent 探测 | API/CLI 双后端注册 | 不同错误分类与探针 |
| 桌面跨进程管理 | CLI 手动执行 | 本机 daemon 生命周期 | 端口冲突、重启、休眠恢复 |

**核心取舍**：`Task` 和 `ChatCompletion` 是两种产品契约。前者可以思考、读写、执行测试并耗时很长；后者需要受控的输入输出、可取消的响应和明确工具/多模态能力，必须拆分。

## 3. 四个消费端的精确改造

### 3.1 GardenFlow：双独立路由，最有价值的首个多模态验证端

**已存在（S）**：AI Sources/Models 可按文本、图片、Embedding 分路由，文本可以选择 OpenAI-compatible；图片具备 `openai-images`、`openai-chat-completions-image`、Gemini Native、Wan、Seedream 等模板、尺寸、参考图、模式与返回资源解析。`imageGenerationService.ts` 要求有效 endpoint 和 apiKey。Pi Agent Runtime 使用 `@mariozechner/pi-ai`/`pi-agent-core`，**工具调用对其相当重要**。`acpGatewayService.ts` 的 `/acp/v1` 是自身应用的代理/会话接入服务，不要与 xcli Model Gateway 或标准 ACP 模型推理接口混淆。

**建议改动**：
1. 最小集成：新增一个可选 xcli 文本源 `http://127.0.0.1:18741/v1`、真实 xcli 本地授权 Token、模型别名 `xcli/text-default`；与现有供应商并存。
2. 独立图片源选 `openai-images`，model 选 `xcli/image-default`，同一 base URL；未通过兼容测试前先只打开文生图。图片编辑、参考图根据 xcli `capabilities` 判定，不能默默降级到纯文生图。
3. Vision 使用支持图像输入的文本模型，统一处理 `image_url`（包括受限 data URL）及消息数组；绝不能把 Vision 和 Image Generation 当同一能力。
4. 对 Pi Agent 的 `tools`/`tool_choice`、`parallel_tool_calls` 做真实兼容测试；如果模型不支持 tool calls，仅在普通写作/总结场景显示该模型，Agent 自动化场景必须禁用或提示能力不满足。
5. UI 清晰显示实际上游是“本机推理”还是“xcli 转发云端”，沿用用户已有数据授权与媒体确认机制。

**验收**：文章生成/续写、中文 JSON 稿件、长文中断、携带图像的视觉分析、文生封面、编辑一张参考图、错误模型禁用工具流；图片必须落库成实际可展示的媒体资产。

### 3.2 Worklog：最适合作首个文本 MVP

**已存在（S）**：`llm.rs::build_chat_request` 拼接 `{base_url}/chat/completions`，请求 `Authorization: Bearer`，读取非流式 JSON `choices[0].message.content`；`commands.rs::provider_defaults` 白名单仅 `openai/nvidia/openrouter/minimax/bailian/dashscope/aliyun`，当前 `xcli` 名称会被拒绝；`secrets.rs` 使用系统 Keyring 保存模型凭据。无必需图片模型调用。

**建议改动**：
1. `provider_defaults` 增加 `xcli`，默认 `http://127.0.0.1:18741/v1`，默认模型 `xcli/text-default`，超时时间可以自定义（例如 180 秒）；保留其他 provider。
2. 沿用 Bearer，本地 Token 存 Keyring；增加 `GET /v1/models` 和一次最小可计费测试（需用户主动触发），返回友好的未启动、无权限、配额耗尽提示。
3. 不改变日报/周报/模板的业务 Prompt；先使用非流式 completion 兼容，收到失败不得悄悄当作大模型生成成功。

**验收**：日报、周报、模板导入、模板优化、日报结构化 JSON、无模型配置 fallback、断网/额度耗尽异常、机密字段不入日志。

### 3.3 Chat Insight：额外需要 Embedding 与数据出境提示

**已存在（S）**：`OpenAiCompatClient` 调用 `/chat/completions`（`stream=true`，支持 SSE 取消）、`/embeddings`、可选 `/models`；`ai_test_connection` 实际发起 embedding 检测。`AiSettings::is_local` 仅判断 endpoint hostname 为本机；`allow_cloud` 与敏感消息发送确认依赖这个判断。

**阻断问题 A：Embedding**。仅给 xcli 添加文本 API，Chat Insight 的连接测试与语义索引仍不能使用；若要支持原有应用流程，必须提供向量 passthrough 或由应用独立配置向量服务。

**阻断问题 B：本机代理 ≠ 本机推理。** 如果地址是 `127.0.0.1`，但 xcli 实际转发 OpenAI/Gemini 等云模型，旧的 `is_local()` 可能绕过聊天原文上云确认。这是高优先级隐私缺口，**禁止在未修复前将 xcli 云端路由直接接入正式版 Chat Insight**。

**建议改动**：
1. xcli `GET /v1/models` 返回能力信息、`inference_location: local|remote`、上游类别和是否可能转发原始内容，不返回用户密钥或内部账号标识。客户端不能只信 URL；须通过可信连接与经验证的响应识别 xcli，信息获取失败则默认视为云端/未知。
2. Chat Insight 新增 provider 来源与 `effective_cloud` 判定：`!isLocalEndpoint || upstreamRemote || upstreamUnknown`；用户每次发送包含聊天正文的请求仍需相应确认，后台批处理不得通过本地地址绕过权限。
3. `GET /v1/models`、`POST /v1/chat/completions` SSE、`POST /v1/embeddings` 在配置和验收中视为一组。向量模型 ID、维度变化触发显式重建索引，不混用旧 embedding。
4. 如果本期不想做 embeddings passthrough，则 Chat Insight 保持原向量服务直连；xcli 只接其聊天功能，需要 UI 显示“部分接入”。

**验收**：连接测试、向量批量索引、聊天流式和取消、语义问答、聊天内容上云确认/拒绝、未知路由默认拒绝自动上传、向量模型切换重建。

### 3.4 AI Open Source Contribution OS：不要推翻原有安全网关

**已存在（S）**：FastAPI 及独立内部 Model Gateway 有 `/v1/responses`、`/v1/chat/completions`，调用需要短期、限次、任务/配置绑定 Token；`app/providers/studio.py` 通过冻结 Profile、tool/json_schema 等结构执行；`app/model_settings.py::validate_base_url` 强制公共 HTTPS 域名，拒绝 localhost、私网和非标准端口。仓库已有单独的 `codex_cli.py` 适配器、不可变模型绑定与任务证据链。

**建议改动**：
1. **不要取消原有 Gateway、签名令牌、任务冻结、审计和沙箱边界。** 以受限 Provider Adapter 的身份接入 xcli，保留工作台的内部 Gateway 与授权语义。
2. 增加独立 `xcli_local` connection 类型/路由策略，专门信任已安装本机 xcli 端点，不放宽普通 `openai_compatible` 的公共 HTTPS 校验；禁止使用用户任意指定的私网 URL，以免 SSRF。
3. 如果后续桌面版与 xcli 在同机，走 loopback + 本机授权；若 Provider Worker 在容器中，使用单独受控 Host Bridge 或 UDS 转发，不开放 xcli 给整个 LAN。桥接目的地址白名单、客户端身份和网络拓扑必须通过测试。
4. `tools`、严格 JSON Schema、最大 Token、返回校验与计费/审计字段需真实透传；不能把一次 Coding Agent 文本总结冒充具备严格 JSON Schema 的模型结果。
5. 规划、实现与 Review 的 provider 改动要与原项目现有冻结快照和错误回滚测试一起实施；本期不需要从 Contribution OS 移走 Coding 工作流到 xcli Task。

**验收**：冻结 Profile、结构化决策、Plan 生成、Review、API Token 作用域、失效/重复 Token、沙箱网络不能任意访问 xcli、非受信任本地 URL 仍拒绝。

## 4. 目标架构（两个运行时，一套注册中心）

```text
GardenFlow   Worklog   Chat Insight   Contribution OS（其原 Gateway）
    |           |           |             |
    +-----------+-----------+-------------+
                        |
              loopback + client token
                        |
           xcli Model Gateway   [新增]
           /v1/chat/completions
           /v1/images/generations
           /v1/images/edits
           /v1/embeddings (passthrough)
           /v1/models, /healthz, /readyz
                        |
      Request Guard / Capability Registry / Router
                        |
        +---------------+----------------+
        |               |                |
   Native Model API  Local Model API   CLI Bridge（实验）
   OpenAI-compatible  Ollama/vLLM      限定 text-only
   Gemini / Anthropic                    | 
   Image backends                原生 CLI Tool/Session
        |               |                |
        +-------Usage / Errors / Audit---+
                        |
     独立保留：现有 xcli Task/Run/Session Runtime
```

**内部模型类型**：
- `ModelAdapter`：标准模型请求，输入为 messages 或 image prompt，返回结构化文本/图片字节，声明能力、上游隐私位置。
- `AgentAdapter`：当前 Coding CLI，输入为执行目标、cwd、权限及 NativeArgs，返回任务事件/文件产物。
- `CliTextBridge`：可选、受限、用户明确授权，**不是把 AgentAdapter 改名成 ModelAdapter**；默认关闭。只有经厂商授权允许的本机脚本式个人交互、无副作用、无 Tool Calling、非流式文本路径才允许实验。

服务不要在每个请求中启动完整 Task Coordinator：文本推理预期是单次 prompt→response，而 Task Coordinator 会引入规划、多轮工具、审核与难以保证的输出成本。

## 5. 本地 HTTP API 契约

### 5.1 服务基本原则

- 服务默认绑定 `127.0.0.1` 和 `::1`（在平台支持且经过配置时）；示例端口 `18741` **仅为配置建议，必须处理端口占用**。默认不得绑定 `0.0.0.0`。
- 使用本地随机客户端 Token（不少于 256-bit 熵）；令牌存 OS Keychain/credential store 或用户权限严格限制的私有配置，不放在命令行参数、git、截图、运行日志。
- 请求必须带 `Authorization: Bearer <client-token>`；即使 localhost 也需要认证。默认不允许浏览器跨域 CORS，不信任任意 Origin；`Host` 必须是本机允许值并防 DNS rebinding。
- `/healthz` 可以只返回最小状态而不泄露模型；`/readyz`、`/v1/models` 及推理接口需授权。客户端按 app 身份签发/撤销独立 key，支持限速和清除。
- 敏感文本/聊天图片日志默认不留原文。输出可选短期保留，自动清理，保留摘要哈希与安全事件；代理不自行上传遥测。
- 内容中的路径是**不可信文本**。默认不向模型 CLI 提供业务源码之外的文件系统路径或任意工具权限。

### 5.2 路由及支持矩阵

| HTTP API | 本期实现 | 说明 |
| --- | --- | --- |
| `GET /v1/models` | 必须 | 返回稳定 alias、type、capabilities；不要暴露原始账号凭据 |
| `POST /v1/chat/completions` | 必须 | 非流式文本；支持 `messages` 与上游可用参数 |
| `POST /v1/chat/completions` (`stream=true`) | 必须 | SSE 标准 delta、结束 `[DONE]`；至少一条真实支持 streaming 的 API 后端 |
| `POST /v1/images/generations` | 必须 | `prompt/model/size/n/response_format`；服务真实返回 `b64_json`/支持的 URL |
| `POST /v1/images/edits` | 必须（能力门控） | `multipart/form-data` 中 `image`、可选 `mask`、`prompt`、model；多参考图仅当上游支持 |
| `POST /v1/embeddings` | 必须（转发） | 仅转发真实向量结果，满足 Chat Insight 兼容，不伪造向量 |
| `GET /healthz` | 必须 | 进程活性，不代表上游模型可用 |
| `GET /readyz` | 必须 | 鉴权后显示服务及默认路由是否就绪 |
| `POST /v1/responses` | 本期可选 | Contribution OS 已有自身接口，先经其 Adapter 使用 chat 兼容面；需要时另立阶段 |
| `POST /v1/audio/*`、视频 | 不支持 | 返回 404/标准不支持错误，不宣传 |

### 5.3 Chat 请求及非流式响应样例

```http
POST /v1/chat/completions
Authorization: Bearer <xcli-local-app-key>
Content-Type: application/json

{
  "model": "xcli/text-default",
  "messages": [
    {"role":"system","content":"你是简洁的中文写作助手"},
    {"role":"user","content":"给这段项目介绍写一条摘要"}
  ],
  "temperature":0.3,
  "stream":false
}
```

```json
{
  "id":"chatcmpl_xcli_example",
  "object":"chat.completion",
  "created":1792828800,
  "model":"xcli/text-default",
  "choices":[{"index":0,"message":{"role":"assistant","content":"项目摘要示例。"},"finish_reason":"stop"}],
  "usage":{"prompt_tokens":null,"completion_tokens":null,"total_tokens":null}
}
```

> 以上为协议**示例响应**，不是来自真实模型。用量不可观测时不要以 `0` 冒充，按客户端 SDK 的类型要求选择省略未知字段或公开声明扩展字段；严格 OpenAI-compatible 模式应使用客户端接受的合法格式。CLI token 数不能推导为真实模型内部计费 token。

### 5.4 Streaming

SSE 输出按 OpenAI ChatCompletionChunk 结构生成： `choices[0].delta.role`、`delta.content`、`finish_reason`、尾部 `data: [DONE]`。要求：

1. 真实上游流式 token/delta 经过有界解析后转发，保留 UTF-8 边界和取消信号。
2. `tools` 请求若不支持，返回明确 `unsupported_capability`；流式 tool call 必须保持 `index/id/function.arguments` 累积顺序。
3. 若底层仅能拿到 CLI 完成后的全文，**不得对外宣传为真流式**；可在显式的降级策略下提供单块结果，或针对 `stream=true` 拒绝，让客户端选择非流式。
4. 客户端断开后传播 `context.CancelFunc`，终止上游流/子进程，记录 `cancelled` 而不是 `success`。
5. 429/401/403/5xx 原样归类成稳定的 OpenAI-compatible 错误 envelope；流开始后的失败必须有可解析的 error 事件或关闭/记录，不能假装完整。

### 5.5 Vision / 图片能力

- **Vision（看图回答）**走支持多模态的 `/chat/completions`，`content` 数组包含 `text` 和 `image_url`；限制 data URL 体积，校验 MIME、魔数、像素总量，禁止任意远程 URL SSRF 与本地文件 URL 自动读取。需要云传输时显示数据位置。
- **Text-to-Image（生成图片）**走 `/images/generations`，必须拿到有效 PNG/JPEG/WebP 字节后才可返回成功；服务端统一验证图片可解码、最大输出字节数和 MIME，不以模型回答中的“已生成”替代。
- **Image Edit（编辑图片）**走 `/images/edits`；mask、multi-reference、保留透明通道只对上游真实支持的路由开放；不支持就返回 422，绝不悄悄忽略参考图。
- 上游图片仅给 URL 时，默认由 xcli 受限下载后验证，并返回短寿命本地资源或 `b64_json`；对 URL host 做白名单、DNS/IP 检查、重定向重校验和大小/超时限制，避免 SSRF。不要把上游带鉴权的原始 URL泄露给桌面应用。
- 建议响应大小：API JSON request 默认不超过 4 MiB（Vision 可单独提高并明确限制），图片单张上限由配置约束（例如 20 MiB），必须流式读入并做总请求预算，避免 base64 内存激增。

### 5.6 能力枚举与隐私元数据

可在 `GET /v1/models` 的每个模型条目增加 xcli 扩展字段（不影响标准字段）：

```json
{
  "id":"xcli/image-default",
  "object":"model",
  "owned_by":"xcli",
  "xcli":{
    "kind":"image",
    "capabilities":["image.generate","image.edit"],
    "supports_stream":false,
    "inference_location":"remote",
    "provider_family":"user-configured-image-provider",
    "validated":true
  }
}
```

能力来源区分 `declared` / `probe_verified` / `runtime_observed`，时间及版本可追踪。对未知模型的 `validated=false` 默认不允许涉及图片/工具的强能力路由。**本机服务地址和推理位置必须分别显示**。

## 6. Go 代码设计（以现有目录最小侵入）

建议新加：

```text
xcli/
  cmd/
    model.go                    # model serve / list / doctor / status / stop
  internal/
    model/
      types.go                  # Message, ContentPart, ModelCapability, Result
      registry.go               # alias、capabilities、上游元数据
      router.go                 # 同一能力过滤与允许的 fallback
      errors.go                 # 统一错误分类
      limits.go                 # 输入、请求量、并发预算
    modelgateway/
      server.go                 # net/http、生命周期、信号/关闭
      auth.go                   # app token、鉴权、Origin/Host
      openai_chat.go            # ChatCompletion 请求/响应适配
      sse.go                    # 事件流与取消
      images.go                 # 文生图和文件校验
      image_edits.go           # multipart
      embeddings.go            # passthrough
      models.go                # 能力发现
    modelbackend/
      backend.go                # 接口定义
      openai_compatible.go     # 标准 API
      gemini.go                # 只有需要原生协议时实现
      image_compatible.go      # 兼容图片接口
      embeddings_passthrough.go
      cli_text_experimental.go # 默认关闭，不进入生产路由
    modelstore/
      config.go                 # 模型设置 + 非敏感元数据
      credentials.go            # OS 安全存储/引用
      requestlog.go             # 无原始 Prompt 的本地状态
  internal/agent/              # 保持已有 Agent 适配器语义
  internal/task/               # 保持已有 Task 行为与持久化
  internal/runtime/            # 提取可复用的上下文/进程取消
```

接口示意（设计合同，非已实现代码）：

```go
type Capability string
const (
    CapText Capability = "text.generate"
    CapVision Capability = "vision.analyze"
    CapImageGenerate Capability = "image.generate"
    CapImageEdit Capability = "image.edit"
    CapEmbeddings Capability = "embeddings"
    CapToolCalls Capability = "tool_calls"
    CapJSONSchema Capability = "json_schema"
)

type ModelDescriptor struct {
    ID string
    BackendID string
    Kind string
    Capabilities map[Capability]bool
    InferenceLocation string // local | remote | unknown
    Verification string      // declared | probed | observed
}

type ChatBackend interface {
    Complete(ctx context.Context, req ChatRequest) (ChatResult, error)
    Stream(ctx context.Context, req ChatRequest, send func(ChatDelta) error) error
}

type ImageBackend interface {
    Generate(ctx context.Context, req ImageGenerateRequest) ([]ImageArtifact, error)
    Edit(ctx context.Context, req ImageEditRequest) ([]ImageArtifact, error)
}

type EmbeddingBackend interface {
    Embed(ctx context.Context, req EmbeddingRequest) (EmbeddingResult, error)
}
```

拆开 Chat/Image/Embedding 小接口，不要把所有 Agent 硬塞进巨大 interface。Router 使用**硬过滤**：所需能力、服务许可、数据位置授权、模型配置、并发槽、是否明示允许 fallback；优先固定 alias/显式指定，禁止同一请求中悄悄把图片服务切到文本服务。默认别名变化必须可审计。

### 6.1 模型配置参考

```yaml
# ~/.config/xcli/model-gateway.yaml
version: 1
server:
  host: 127.0.0.1
  port: 18741
  auth: required
  access: local-app-only
  cors: disabled
  max_concurrent_requests: 3

clients:
  - id: garden-flow
    key_ref: keychain:xcli/garden-flow
    allowed_models: [xcli/text-default, xcli/image-default]
  - id: worklog
    key_ref: keychain:xcli/worklog
    allowed_models: [xcli/text-default]
  - id: chat-insight
    key_ref: keychain:xcli/chat-insight
    allowed_models: [xcli/text-default, xcli/embed-default]
  - id: contribution-os
    key_ref: keychain:xcli/contribution-os
    allowed_models: [xcli/text-default]

backends:
  text-main:
    type: openai-compatible
    endpoint: https://example-llm-provider.invalid/v1
    api_key_ref: keychain:xcli/text-main
    inference_location: remote
  image-main:
    type: openai-images
    endpoint: https://example-image-provider.invalid/v1
    api_key_ref: keychain:xcli/image-main
    inference_location: remote
  embedding-local:
    type: openai-compatible-embeddings
    endpoint: http://127.0.0.1:11434/v1
    inference_location: local
  # 原生 Agent CLI 的通用模型桥接默认关闭
  cli-text-experiment:
    type: native-cli-text
    enabled: false

models:
  xcli/text-default:
    backend: text-main
    upstream_model: user-selected-text-model
    capabilities: [text.generate, tool_calls]
  xcli/image-default:
    backend: image-main
    upstream_model: user-selected-image-model
    capabilities: [image.generate, image.edit]
  xcli/embed-default:
    backend: embedding-local
    upstream_model: bge-m3
    capabilities: [embeddings]
```

> Endpoint 与模型名均为**占位示例**，不能直接拿示例联网使用。`cli-text-experiment` 的实际使用需要在安全/授权验证后另行开启。用户当前已安装的原生 CLI 登录信息由 CLI 自己管理，**绝不读取/导出/转存到模型网关**。

## 7. 安全、合规与失败处理的硬门禁

### 7.1 产品使用边界

**会员账号 ≠ 开发者 API 账户。** 不应默认把 Codex/Claude/Cursor/Antigravity 的订阅登录包装成供多个独立 App 或其他用户任意调用的通用模型服务，更不应通过共享/轮转账号绕过使用量限制。是否允许特定程序化调用应按具体厂商、认证方式和许可判断；无法确认就使用官方 API Key 或用户自建的开放模型服务。即使官方 CLI 自带 headless 模式，也不能据此自动推导可对外二次分发“模型额度”。

相关官方材料：
- [OpenAI 使用条款（2026-01-01）](https://openai.com/zh-Hans-CN/policies/terms-of-use/) 对自动程序化提取/限额规避有限制。
- [Sign in with ChatGPT Terms](https://openai.com/policies/sign-in-with-chatgpt-terms/) 对该集成类别的登录凭据、本地控制、关联应用及禁止通用 API 转发有明确要求；这些条款**只直接约束其适用的 SIWC 集成**，不能机械套用到所有原生 CLI，但显示不能把订阅视作可任意共享的 API。
- [Gemini API 文档](https://ai.google.dev/api)、[Gemini API 使用条款](https://ai.google.dev/gemini-api/terms?hl=en) 区分开发者 API 的认证/地区/服务条件。
- [Claude Code CLI 参考](https://docs.anthropic.com/en/docs/claude-code/cli-usage) 说明 headless，但具体协议/订阅使用范围仍需另核对。

### 7.2 安全检查清单

- [ ] 不绑定公网/LAN，默认端口只有本机可连，外部请求及浏览器恶意页面不可读取 API。
- [ ] 每个 App 独立 Token、最小模型权限、令牌轮换/撤销；本机管理命令显示 ID，不显示 Key。
- [ ] UI 显示真实数据发送位置，`localhost` 代理不能绕过 Chat Insight 的云端同意。
- [ ] 图片下载防 SSRF：拒绝 localhost/私网/metadata IP、DNS rebinding、重定向改址；文件输入拒绝路径穿越及符号链接逃逸。
- [ ] 不自动继承原生 Coding CLI 的执行工具、全局 MCP 配置、工作区信任和 shell 权限到“普通文本聊天”。
- [ ] 无密钥硬编码、无原文默认录制、无 token 出现在启动参数/进程列表/HTTP 错误。
- [ ] 模型请求、用户取消、上游超时/401/429、流错误与耗尽都可定位；未知用量标未知。
- [ ] 单请求并发槽、长度与时间预算；失败自动重试仅限幂等、未开始输出、未发生计费歧义的场景，图片默认不自动重试。
- [ ] 日志与上游地址脱敏；敏感资料不落 Task artifact 或原生会话，除非用户明确授予相应访问权限。

## 8. 分阶段执行计划（按阶段验收后继续）

### M0：冻结兼容合同与最小骨架

**改动**：新增 `docs/model-gateway/`，固定 API 契约、图像返回形态、能力注册规则、服务条款判定流程；建立本地 Fake Backend 测试夹具。添加 `xcli model` 命令组，不改 `task/run/use/resume` 行为。

**交付**：`docs/model-gateway/api-contract.md`、`capabilities.md`、`security.md`、`integration-matrix.md`；新目录空实现与可执行健康检查。

**Gate**：现有 `go test -race ./...`、`go vet ./...`、格式及构建通过；新增契约测试用 Fake Backend 可独立运行，不需要账号。

### M1：HTTP 文本 MVP + Worklog 联调（P0）

**改动**：local-only `serve`；本地 Token；模型别名路由；`/v1/models`、`/chat/completions` 非流式、SSE；原生 OpenAI-compatible backend；结构化错误、超时和取消；Worklog 增加 `xcli` provider。

**Gate**：Worklog 日报/周报/JSON 汇总真实调用成功；两请求并发有界；未授权、Token 撤销、客户端断开可复现；SSE 在真正流式上游做到实际 delta，不做伪流。

### M2：Vision + 图片生成/编辑（P0）

**改动**：多模态 content parts；`/images/generations`、`/images/edits`；按模板的 ImageBackend；mime/image sniff、尺寸和大小约束，参考图与 mask 能力标注。先支持**一个真实可用的 OpenAI-compatible 图片后端**，再扩 native 适配，不为支持数量牺牲可靠性。

**Gate**：4 类真实输入输出用例：看图问答、文字生成封面、参考图编辑、非法/过大图片拒绝。输出文件可解码、尺寸正确、不留临时文件；不支持的 mask 不会被静默忽略。

### M3：Embedding 透传 + Chat Insight 的隐私修复（P0）

**改动**：`/embeddings` passthrough、模型维度元数据，Chat Insight xcli 供应商、流式消费、云端真实位置识别、嵌入索引版本隔离。可继续使用当前 Ollama/vLLM/其他独立向量服务。

**Gate**：Chat Insight 连接测试、批量索引、知识检索/对话全通过；云代理时拒绝授权不向上游发送聊天原文；换模型维度触发重建；无 Embedding 服务明确提示而不是生成假的向量。

### M4：GardenFlow + Contribution OS 正式接入（P1）

**GardenFlow**：文本/图片两个独立 Source 模板；Capability-aware UI；Vision、Tool Calls、结构化输出及媒体库结果的端到端回归。

**Contribution OS**：基于原有内部 Gateway 新增受控 xcli Local Provider，不放宽普通用户输入 URL 的公共 HTTPS 约束；保障冻结 Profile、证据审计和沙箱隔离。仅在支持 JSON Schema/Tool Calling 的后端上启用相应 Agent 阶段。

**Gate**：两项目完整最小工作流通过；保留原服务商接入；不改变现有历史任务、缓存和密钥隔离；恶意本地 URL 和未授权 Token 用例全部拒绝。

### M5：桌面分发与可观测性（P1）；CLI 模型桥接为单独实验（P2）

**改动**：跨平台安装/升级/卸载、启动项（用户选择）、单实例与端口发现、服务状态/模型诊断、App Token 管理、可中断任务运行记录；兼容 macOS arm64/x64、Windows x64、Linux x64 的构建及目标机器真实验证。

CLI Bridge 如需实验，必须单独建立品牌-登录方式-条款-权限-环境-请求能力矩阵；仅默认本机个人、单一显式授权 App、`text.generate` 无工具调用，且用户可随时停止。不通过安全/条款门禁就不实现/不发布此桥接。

**Gate**：停止/启动/OS 休眠/恢复、端口占用、版本升级、数据迁移、单/多客户端隔离、原生 CLI 原有 Task 功能回归。不得用“跨平台可编译”代替目标机实测。

### 依赖排序

```text
M0 → M1 → M2 ───────→ M4 → M5
        └→ M3 ───────→ M4

可早于 M2 合入 M3 的部分 Embedding Backend，
但 Chat Insight 上云授权修复是它独立的强制 Gate。
```

**优先开发顺序**：Worklog（低摩擦真实文本闭环）→ GardenFlow 图片/视觉 → Chat Insight 隐私 + Embedding → Contribution OS 严格结构化与内网边界。按工程依赖，M3 可与 M2 在隔离分支并行，但最终验收需共同覆盖。

## 9. 自动化测试、兼容性、质量门槛

| 类型 | 重点测试 | 必须通过的结果 |
| --- | --- | --- |
| Contract | chat 非流式/SSE、工具 JSON、模型枚举 | SDK 解析成功，字段合法，错误一致 |
| Vision | PNG/JPEG/WebP、data URL、恶意 URL | 合规转发，非法拒绝，不卡死 |
| Image | 生成/编辑、mask、n、尺寸、真实图片字节 | 图片可解码，无静默降级 |
| Embedding | 多条输入、维度切换、重复请求 | 数量顺序一致；模型变更可见 |
| Security | Bearer 缺失/撤销、Host/Origin、SSRF、路径逃逸 | 全部拒绝并脱敏记录 |
| Process | Ctrl+C、HTTP 断开、CLI 不返回、超时 | 无孤儿进程，有限内存，正确状态 |
| Routing | 模型无能力、额度耗尽、网络错误 | 不静默换模型，分类准确 |
| Regression | xcli Task/Run/Resume 和四客户端旧 provider | 已有命令/数据可回退、无变更 |
| OS | Darwin arm64/x64、Windows、Linux | 各真实主机 smoke，而不只是编译 |

可记录但需实测建立基线的目标：1）本机 Gateway 本身的空载响应额外开销；2）并发 2–3 个桌面客户端、请求峰值内存；3）长文本与图片大响应的峰值内存；4）取消传播时延。**不要承诺模型生成耗时或费用低于直接 API**——实际取决于上游。

测试必须有两套：
1. **离线 deterministic Fake Backend**：CI 必跑，使用假流/假图片/假向量，不消耗模型额度。
2. **选择性真实 Provider smoke**：显式开关、明示成本和调用次数，写入 `docs/model-gateway/acceptance/<date>.md`，记录版本、平台、模型类别、通过/失败与数据是否上传；不保留密钥、私人 Prompt、原生会话 ID。

## 10. 首个联调场景（示例命令）

```bash
# 设定有效的模型网关配置后启动（命令为计划实现的接口）
xcli model serve --config ~/.config/xcli/model-gateway.yaml

# 查看实际能力与服务状态
xcli model list
xcli model doctor --no-billable-requests

# 凭据由 `xcli model client issue worklog` 以安全方式发给用户或 OS keychain；
# 不应把真正的 token 提交到仓库或写进 README。
export XCLI_APP_TOKEN='<locally-issued-token>'

curl --fail-with-body http://127.0.0.1:18741/v1/models \
  -H "Authorization: Bearer $XCLI_APP_TOKEN"

curl --fail-with-body http://127.0.0.1:18741/v1/chat/completions \
  -H "Authorization: Bearer $XCLI_APP_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"model":"xcli/text-default","messages":[{"role":"user","content":"你好，请用一句话介绍这个服务"}],"stream":false}'
```

图片 Smoke 用本地最小合法输入与经授权的非敏感测试图片；网络测试只发送合成材料。部署地址根据每台机器的实际端口发现机制读取，`18741` 不应硬编码在应用核心逻辑里。

## 11. Codex 分阶段开发指令（可直接复制）

```text
你现在位于 Garden12138/xcli 仓库。请先完整阅读 README、docs/r3、cmd、internal/agent、internal/task 和现有测试，不要假设旧的 install/acp/workflow 命令仍然存在。

项目目标：实现《xcli 模型服务化升级：现状审查与分阶段实现方案》（放入 docs/model-gateway/UPGRADE_PLAN.md）中 M0–M5 的内容。保持原 xcli Task Runtime 不回归，新建隔离的 Model Gateway。不可把原生 Coding Agent 的 Task 输出直接当成具有完整能力的模型 API。

执行纪律：
1. 从 M0 开始；每阶段先建/更新规格与测试，再完成实现、Go 检查、契约测试和阶段验收记录。
2. 每阶段完成后自动开始下一阶段；但涉及真实付费请求、账号登录/权限变更、外部发布、LAN 开放、安全条款不明确、已有数据库不可逆迁移的动作必须停下请求明确授权。不能因为这条指令跳过用户审批。
3. 默认只跑离线 Fake Backend；未经授权不得消耗真实模型额度、读取其他桌面应用私人数据或更换/复制原生 CLI 登录凭据。
4. 未验证能力不得宣称已支持；若上游不支持 tool_calls、json_schema、image edit，应显式拒绝而非模拟/降级。
5. 每阶段保留回滚点，优先改 xcli；对四个消费端先提交精确的集成变更建议，不在未获授权的仓库自动推送。
6. 每阶段输出：代码改动清单、API 契约变更、测试命令与结果、尚未完成的风险、下一阶段入口；在仓库 docs/model-gateway/ 中持续更新 PROGRESS.md。

请从 M0 开始执行，不要仅输出规划或空目录。严守安全与隐私门禁。遇到真实阻塞就明确停止该能力的推进，其他不依赖部分可以继续。
```

> 这段 Prompt 的执行能力取决于 Codex 会话时限、权限和工具运行状况；“自动继续”不代表保证无需人工授权就能无期限后台运行。对四个仓库的代码改动应分别创建 PR 或独立补丁，避免跨仓库状态混乱。

## 12. 推荐最小可发布版本与非目标

**第一可用版本目标**：`xcli model serve` + OpenAI-compatible 文本非流/流式 + 图片生成/编辑 + 受限 Embedding passthrough + 本机授权 + Model Catalog + Worklog 与 GardenFlow 两个真实应用成功使用。Chat Insight 的**完整集成发布必须等待隐私修复**；Contribution OS 的完整集成必须等待它的安全 Gateway 与 JSON/Tool 兼容验收。这里不指定或暗示固定版本号已经发布。

**为什么不直接把 xcli 做成万能模型 API 反代？** 现有优势是 Agent Runtime/任务完成，而真正有产品价值的升级是 **为桌面智能应用统一“可验证能力、使用位置、安全边界、错误契约与模型切换”**。即便未来换掉 Codex/Claude/Gemini/本地模型，上层应用也只需遵守统一契约。

## 附录：待验证的关键事实

- R3 是否打 tag/发布和实际用户安装包版本（现 README 说明 R3 候选尚未正式发布）。
- 本机各真实 CLI 的非交互命令、工具禁用、图片字节输出、模型登录方式是否允许被复用；不能沿用历史一次成功就扩大支持矩阵。
- GardenFlow 的 Pi runtime 在当前版本对 `/v1/chat/completions` 的工具/流式返回具体要求。
- Contribution OS 从现有 HTTPS 外部 Provider 转为受限本机 Bridge 的审计/容器边界与桌面架构最终方案。
- 真实图片上游支持的编辑/mask/reference 数量、返回 URL/二进制规则、尺寸与收费策略。
- Chat Insight 当前向量索引模型、维度和重新构建策略，及是否具备独立文本/Embedding URL 配置入口。

**最终验收标准以真实消费端行为为准，而不是只看 curl 返回 200。**
