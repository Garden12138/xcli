# 官方资料核查记录

核查日期：**2026-11-06**。网页当日打开核对；本机版本和帮助以[预检查](evidence/preflight.json)为准。网页可能覆盖比本机更新的版本，公开文档不等于当前账号 entitlement 或生产接入许可。

## Codex / OpenAI Docs

| 官方页面 | 已核对要点与适用边界 |
| --- | --- |
| [非交互模式](https://learn.chatgpt.com/docs/non-interactive-mode) | `exec`、JSONL、Schema 文件、按 ID 恢复；适合有边界的脚本任务。JSONL 不是 Chat Completions SSE。 |
| [Codex SDK](https://learn.chatgpt.com/docs/codex-sdk) | 官方库可启动、继续、恢复本地线程；是可评估的程序集成入口，不证明本仓库已接入。 |
| [App Server](https://learn.chatgpt.com/docs/app-server) | 应用可用初始化、thread/turn、消息 delta、审批与 interrupt 生命周期。应使用公开 RPC，不能读取客户端私有接口。 |
| [认证](https://learn.chatgpt.com/docs/auth) | ChatGPT 订阅登录和 API Key 的计费及 workspace 数据政策不同。本轮只使用只读登录状态，不读取或迁移凭据。 |
| [正式 ChatGPT plan app-server 路径](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server) | 另有需要应用自身 OAuth 授权的集成流程；目录不是访问权证明。不得从现有 CLI 登录文件提取 Token来替代该流程。 |
| [该路径的预览限制](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations) | 使用 Responses 的受限合同，部分参数/托管工具不支持，含 hosted image generation 不支持；不能据此宣传标准 Chat/Images 全兼容，也不能把该限制套到所有原生 Codex 图片工具。 |

结论是优先评估 `exec` 或 SDK/app-server 的用户本地 Agent 任务。正式 OAuth 集成可作为独立工程选项，需要相应登记、用户授权和实际验收；本轮不实施、不提取现有 Token，也不改用普通 API 请求充当原生 CLI 测试。

## Cursor

| 官方页面 | 已核对要点与适用边界 |
| --- | --- |
| [Headless CLI](https://cursor.com/docs/cli/headless) | 官方支持脚本/自动化 print 模式。文档包含 force 示例，本任务明确禁止使用。 |
| [输出格式](https://cursor.com/docs/cli/reference/output-format) | 默认 text；stream-json 默认按 assistant 消息聚合，partial 输出另有开关和重复 flush 规则；失败可能无 terminal result。 |
| [权限](https://cursor.com/docs/cli/reference/permissions) | Read/Write/Shell/MCP 权限与全局/项目配置；本轮不修改这些长期设置。 |
| [认证](https://cursor.com/docs/cli/reference/authentication) | 已有浏览器登录可查 status，另有 API key 途径；本轮只用已有登录，后者不是本任务替代方案。 |
| [Agent 概览](https://cursor.com/docs/agent/overview) | 描述文本/参考图生成图片并保存文件；不能从产品概览直接推导本机 headless 已可调用。 |
| [服务条款](https://cursor.com/terms-of-service) | 页面标注更新于 2026-09-03；有服务使用、出租/出售、内容、测试结果发布等约束，企业 MSA 可能另行适用。没有将普通 headless 文档解释为通用模型转售授权。 |

建议仅为本人本地应用封装明确的 Agent 工作任务。缺少审批时停下，不能把原生工具结果变成未经用户授权的应用副作用。若要分发或提供多人服务，需重新核对实际合同与正式集成许可，不复用个人额度做共享服务。

## Antigravity

| 官方页面 | 已核对要点与适用边界 |
| --- | --- |
| [Headless](https://antigravity.google/docs/cli/headless/) | print 的 JSON/stream-json、会话恢复、Schema 与 stream-json stdin；连续输入当前只支持 text block。审批无法交互时可能 soft-deny，不能只看 exit 0。 |
| [认证](https://antigravity.google/docs/install/) | 本机共享认证层/系统 keyring；API key 与 ADC 是其他路由。本轮不启用它们。 |
| [SDK 工具](https://antigravity.google/docs/sdk/tools/) | 工具表包含 `generate_image` 与工具过滤；SDK 列表不保证特定 CLI 模型/账号或子代理可用。 |
| [权限](https://antigravity.google/docs/permissions/) | 原生权限、执行模式及终端沙箱有各自边界；本轮不改全局 allow、trust 或 sandbox 设置。 |
| [CLI credits](https://antigravity.google/docs/cli/credits/) | 订阅额度及个人 credits 可能共同影响成本；本轮不购买或启用额外 credits，不宣称进程次数等于模型请求/费用。 |
| [附加条款](https://antigravity.google/terms) | 关联 Google 条款/隐私及企业适用合同；明确 Agent 访问授权、任务和监督由用户负责，并有交互数据处理条款。 |

优先评估公开 headless/SDK，用本人现有登录且每个任务明确数据和权限。连续 stdin 并非 Chat 任意多模态输入；图片工具的文件产物需另外验证与交付。

## 三者共同的适用范围

本地进程执行不代表本地推理；合成提示、文件内容或图片可能发送到厂商云端。不得共享账号、轮换绕限、复制登录 Cookie/Token、调用私有接口或冒充其他客户端。当前资料支持有边界的 Agent 集成选型，未证明“CLI 会员 → 任意应用通用 Chat/Images/Embedding API”可以原样提供。

这些是基于所列公开资料的工程判断；实际账号合同、地区/组织政策、额度与媒体权限没有经过本轮模型调用验证。遇到不能建立可靠映射的能力，应保持 BLOCKED，使用[拟议 Agent 合同](INTEGRATION_ARCHITECTURE.md)，而非虚构标准 API 支持。
