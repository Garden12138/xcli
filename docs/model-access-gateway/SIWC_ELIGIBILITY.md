# SIWC 资格审查

审查时间：2026-11-10。结论：**BLOCKED_ELIGIBILITY_AND_ARCHITECTURE**。这是“官方证据不足以确认当前多应用架构”，不是认定 SIWC 普遍禁止本地 Gateway。

## 官方依据

本轮实际读取了用户指定的三页，并沿官方链接核对协议细节。以下为释义，不是资格批准函：

| 官方资料 | 与本阶段相关的事实 |
| --- | --- |
| [Overview](https://developers.openai.com/siwc/token-sharing-open-source) | 文档面向开源、本地托管应用；付费或远程托管用途走 interest form。client 表示用户授权的注册，host 表示运行环境。 |
| [Preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations) | 同时约束直接 Responses 和 app-server 路线；多模态输入不等于图片生成权限。 |
| [Codex app-server](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server) | 是可选集成路径，并非直接调用的必要组件；该页的子进程环境认证方式不用于 G8。 |
| [Registration and sign-in](https://developers.openai.com/siwc/token-sharing-open-source/sign-in) | 动态 public client、真实应用名称、PKCE 和 ID Token/granted scope 验证。 |
| [Accounts and sessions](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions) | 注册隔离、刷新轮换、撤销；不能通过创建更多客户端获得独立的订阅额度。 |
| [Models and inference](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference) | 官方公开 Responses 路由及账号目录；完成一次请求才证明该次可访问。 |
| [Token reference](https://developers.openai.com/siwc/token-sharing-open-source/token-reference) | Token 响应和生命周期；认证元数据为 opaque，不能自行解析出 workspace。 |
| [Errors and recovery](https://developers.openai.com/siwc/token-sharing-open-source/errors-and-recovery) | 资格/权限/额度/路由错误与撤销恢复，不允许静默切换计费。 |
| [UI/UX guidelines](https://developers.openai.com/siwc/ui-ux-guidelines) | 授权和使用提示、用量管理入口；示意图片不能证明某项 Preview 能力被支持。 |

## 逐项资格结论

| 问题 | 可确认部分 | 当前决定 |
| --- | --- | --- |
| xcli 是合格开源客户端吗？ | 本地 `LICENSE` 为 MIT，Git remote 为公开 GitHub URL；按本轮用户提供的信息是公开开源、本地运行。产品形态与文档目标相符。仅 URL 不是独立的线上可见性核验，也不是官方批准。 | **一般形态相符；当前完整用途资格 BLOCKED**。未注册、未核实账号/地区/workspace 权益。 |
| 四个本地应用共用 xcli 是否合规？ | 官方讨论同一客户端的多 host，并未明确讨论四个独立产品通过单一 client 的 Gateway 复用。 | **BLOCKED**。多 host 许可不能外推为多产品代理许可；需要官方明确答复。 |
| 一个 client 还是每应用一个？ | issued client 绑定认证用户和注册时 workspace，应用名必须真实。 | **未定**。不能先认定一个 client 覆盖全部应用，也不能用四次注册规避资格审查。 |
| 可以统一保管 OAuth 吗？ | 本地安全 broker 在工程上可以隐藏凭据，公开文档没有确认独立产品把授权统一委托给 xcli 的范围。 | **工程可设计，产品授权归属 BLOCKED**；不得据此接通真实登录。 |
| 可以不用 app-server 吗？ | 直接 OAuth Bearer → `POST https://api.openai.com/v1/responses` 有官方文档。 | **YES，协议明确**；仍须满足客户端与账号资格。 |
| 个人与商业分发有什么不同？ | 个人使用仍需合规应用、有效授权和权益；付费/远程托管用途另走官方 interest form。 | 免费本地开源分发不能等同商业 SaaS/收费分发已获批；未来产品收费、托管、租户或中转拓扑改变必须重审。 |

不把 SIWC 用作通用商业 API、任意订阅共享、服务账号池或共享充值服务。不使用 Codex 的认证文件、app-server、普通 API Key 或 Ollama 作本轮替代验收。

## 注册身份管理建议（待官方确认适用架构）

如果官方认可 xcli 自身就是用户授权的统一产品，`agent_name_hint=xcli` 在首次注册保持一致；下游应用通过各自本地授权接入，UI 必须清楚说明消耗的是 xcli 这项授权，并覆盖实际获准应用。它们不获得 OAuth Token，也不能自行选择其他账号、host 或 client。

如果官方要求各应用独立注册，各应用用自己的真实产品名和授权体验；xcli 的代理保管资格也须一并确认。不可把四个应用都冒充 `xcli`，也不可由 Gateway 批量生成注册冒充独立批准。此方案同样不是本轮获准的绕行办法。

`ext_agent_host_id` 对应实际安装环境，重启/刷新/切换应用不更换。`client_id` 对应用户与 workspace 的授权边界，不能当应用安装 ID、租户路由字段或共享全局常量。内部键为 `{validated iss, validated sub, issued client_id}`；workspace 使用注册绑定，不从 email/sub/加密 metadata 猜测。更换 workspace 必须创建或选择对应的独立注册。

## 需官方答复的问题

1. 本机、同一 OS 用户、无公网、多产品消费的 xcli 模型 broker 是否属于允许的 OSS client，而非未批准的通用推理代理？
2. 用户在 xcli 的一次同意，能否覆盖 Worklog、GardenFlow、Chat Insight、Contribution OS？哪些命名、归因、用量控制和每应用 consent 必须独立？
3. 应采用统一 client，还是每产品 client？后者能否由独立 broker 代管登录与凭据；其 host 生命周期如何定义？
4. 免费开源本地分发、收费桌面分发、企业管理安装及远程服务分别需要哪些批准？

本轮没有提交 interest form 或联系官方，也没有声称收到许可。拿到具体答复后，需记录来源、日期、获准产品/版本/部署边界，不能仅凭成功返回 Token 消除资格门禁。
