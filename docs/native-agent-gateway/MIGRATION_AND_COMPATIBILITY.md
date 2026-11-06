# Native Agent Gateway V1 — Migration & Compatibility

状态：设计，2026-11-06；四应用不在本轮写入范围。应用模块定位来自已提交的 [Worklog](../cli-first/integrations/WORKLOG.md)、[GardenFlow](../cli-first/integrations/GARDEN_FLOW.md)、[Chat Insight](../cli-first/integrations/CHAT_INSIGHT.md)、[Contribution OS](../cli-first/integrations/CONTRIBUTION_OS.md) 静态核对记录，不声称其当前工作区重新验收通过。

## 与既有 xcli 的关系

| 既有合同 | V1 迁移规则 |
| --- | --- |
| `run/resume --json`、`--events`、Task/Jobs | 保留 argv、输出/退出和默认权限；新 observer 采用可选扩展，不能改变旧事件语义 |
| 原生 Session ID / Run ID | 不直接导入 HTTP Session；只有服务自己创建并持有的绑定可续接，不读取历史聊天迁移 |
| `internal/task` 工作流 | 保留原多轮预算与恢复；新 HTTP Task 为单 turn，不共享状态文件和 OwnerToken |
| Model Gateway `/v1/chat/completions`、Images、Embedding | 继续使用显式合法 HTTP 上游、原配置/能力/Token；不添加 CLI alias 或 native-cli 类型 |
| Model Gateway 客户端 Token | 不授予 Agent 权限；新 issue/rotate/revoke 使用独立存储和 scope；不迁移 secret 明文 |
| 旧模型 metadata/能力目录 | Agent 目录独立，不能把 structured_output 映射为 strict Schema、工具列表映射为 app tool_calls |
| 旧 CLI-first Agent API 草案 | 本 SPEC 首次冻结细节；以 Session 创建+新 Task 续接代替 task resume 示例；旧路由未实现，无兼容债务 |
| 历史四应用补丁/宿主桥接 | 只作参考，不回放整包；每次重新检查实际 HEAD/diff/来源摘要 |

V1 不提供任何“无代码替换 base_url/model 即可用原生 CLI”的配置。Agent 任务与 Chat 模型调用在应用 provider 类型、UI 表达、预算、取消和结果验收上均分开。未来若再做最小兼容层，需要另一 SPEC、参数拒绝矩阵及真实协议验收；不属于本次 V1。

## 共用应用适配协议 APP-NAG-1

只允许应用后台（Tauri/Electron/Python 宿主）持有独立 Token；renderer/网页只获得业务 Task 状态与结果，不接触 Token、CLI 凭据、native session、任意本机路径。Token 用系统安全存储或本人批准的 0600 文件引用保存，失败关闭，不能退回 localStorage/前端明文。

顺序固定如下：

1. 后台加载本人登记的精确 loopback Agent endpoint，GET 目录并校验 Agent/operation/Gateway 证据/策略/推理位置；此步骤零 native 探测，不用小提示测试连接。
2. 在读取/导出用户私有材料前，取得本次选择范围、云发送与本地预算同意。remote/unknown 均按云处理，说明原生会话可能留存；仅导出最小字段和选定图片，不能给数据库/媒体库/仓库根路径。
3. 建立专用 Session；保存逻辑 ID、revision、context_sha256、profile/capability revision；仅将同一材料领域的续问放在该 Session。
4. 上传选定材料字节，核对 hash；提交一个带稳定幂等键的 Task。保存 task_id 和最后 seq，不以 HTTP 202 显示“成功”。
5. 观察 SSE；仅 text.delta 才显示原生增量，否则活动+等待最终结果；EOF/断线先查原 Task，不生成新键重发。跨设备/服务实例变化禁止自动续跑。
6. 文本必须 task succeeded 且 final=true，再经应用业务校验作为草稿；图片必须从 review_only 预览、完成来源/视觉接收、变成 ready，认证下载后再次核对字节。
7. 由应用原有用户确认/事务写入最终业务对象，并记录 task_id/result hash 或 artifact_id/hash 作为去重来源。Agent 工具成功不直接写业务库。
8. 用户取消调用 cancel 并观察终态；unknown/reconcile 封存上下文，保留必要审查信息，不自动新建 Session 重做。显式修复/新任务使用新键，重新检查同意与剩余预算。

scope、接口对象、错误、原子幂等和保留期遵循 [API_CONTRACT.md](API_CONTRACT.md)。应用必须先拒绝再读取/导出；服务本身也在 dispatch 前再检查，两层各自不能替代。业务材料中的指令作为低信任数据，不可改变文件/工具策略。

## Worklog — 文本草稿与本地 Schema

未来新增独立 `xcli_agent` 模式，既有 xcli HTTP provider 保持原状。候选边界：`src-tauri/src/llm.rs` 增加单任务结果转换；独立 transport 放在 `xcli.rs` 旁；`commands.rs` 管选择/进度/取消；`secrets.rs` 存独立 Token 引用。实施前重查模块和用户 diff。

| workflow | 输入 | 输出与应用验收 | 当前可用性 |
| --- | --- | --- | --- |
| report-draft | 明确日期范围、选定工作项及稳定 source_id | 最终文本草稿；日期不越界、每个工作项有来源，不编造空材料产出 | native 文本基线可参考；应用 Agent transport 未实现 |
| template-draft | 选定模板/变量或合成示例 | 草稿保留合法变量/格式；未知变量拒绝；预览确认后另存 | 未实现 |
| structured-report-draft | 同上，prompt 可描述业务 JSON | 本地解析和业务 Schema 校验，失败不保存、不自动模型修复 | strict native Schema 未验收；本地校验不等于模型约束 |

业务报告 envelope 建议为 period_start/period_end（ISO 日期）、summary（字符串）、work_items（text/source_ids 数组）、blockers/next_steps（字符串数组），必填字段且拒绝未知字段，source_ids 必须来自本次输入集合。该 envelope 在应用本地验证，不作为 Gateway 的 response_format 参数。

取消、部分结果、JSON 损坏、日期/引用不合均保留为草稿错误；不自动提交日报或覆盖模板。保存事务以 task_id+结果摘要去重；事件重连不产生另一份日报。后续验收 A01，不能把现有 HTTP 模板/日报联通记为 native UI 已通过。

## GardenFlow — 任务与媒体两阶段接收

未来在 `desktop/electron/core/aiModelRouteResolver.ts` 注册独立 Agent route，transport 放在 `xcliGateway.ts` 旁；不要扩大原 `/v1` transport 白名单。`imageProviderAdapters.ts` / `imageGenerationService.ts` 增加 Task→Artifact 状态机，媒体库/IPC 保存 app media ID；`desktop/electron/pi/agent.ts` 仅消费本模式明确批准的任务，不能把 Pi 全部工具权限交给原生 Agent。

| operation | APP-NAG-1 特定规则 | 不能推导的能力 |
| --- | --- | --- |
| text_task | 用户选定描述与文本，最终草稿经确认 | 任意图片理解、app tool_calls |
| image_generate | 请求 1 图；review_only 可临时预览；来源和视觉通过后才 ready | size/n/mask/指定模型/透明背景/完整 Images API |
| image_edit | 导出唯一选定原图的精确字节，上传 hash；保留原媒体引用；新图另存 | 多参考、mask、精确尺寸控制、任意 Vision |

入库事务必须满足：Task succeeded、Artifact ready、认证下载完成、MIME/长度/尺寸/hash 一致、用户确认目标库；以 artifact_id+sha256 幂等创建媒体对象。临时 CLI 路径、预览文件、文字 URL 不能作为永久媒体引用，不能塞进 b64_json。保留真实 PNG/JPEG/WebP，应用若另做格式转换需独立记录派生来源。

来源不可见时普通应用仅能提示本人复核；不自动代签 artifact:attest-origin。取消后的晚图或 failed 结果只隔离，不进入媒体库。下载/磁盘/入库失败允许重取同一已存在 Artifact（不产生推理），不允许自动重生成；重启后应用自己保存的媒体 ID 仍可用。后续验收 A02；本轮未导入任何真实媒体。

## Chat Insight — 选段 Agent 与向量能力分离

未来在 `src-tauri/src/ai/client.rs` 附近新增独立 Agent adapter，`gateway.rs` 保留原模型目录/云同意/向量身份语义；`settings.rs`、`commands.rs`、`src/ai.ts` 增加逻辑 Session/任务/进度。只导出用户选定消息，不把聊天数据库路径交给 Agent，不允许它自行搜全库。

| 模式 | V1 范围 | 必须拒绝/保留 |
| --- | --- | --- |
| selected-messages-summary | 选段总结、同一选段范围的续问，业务 source_id 核对 | 未经同意的原始聊天导出/跨会话历史注入 |
| raw agent conversation | 用户直接输入并明确同意的文本任务 | 不把 Agent 进度当 Chat token，不自动拼任意 system/developer history |
| embedding / index / retrieval | 不由原生 CLI 提供；保留原合法独立向量 backend | 原生 embedding capability 缺失，完整 CLI RAG 不能开启 |

`indexer.rs` / `retrieval.rs` 不因新 Agent 文本模式改变旧索引。以后向量服务切换仍要检查模型/维度/version/fingerprint，单独授权重建，先新索引验证再原子切换；不能让 Agent 输出数字代替向量或将新向量混入旧索引。不开向量的选段总结必须用准确产品名称，不能显示 RAG 已就绪。

AllowRemote=false、缺本次同意、unknown 未获准、能力目录异常时，私有消息读取/导出/CLI 启动都必须为 0。部分文本不落为最终摘要，断连只查状态。后续 A03；本轮不打开聊天库、不重建索引、不运行桌面 UI。

## Contribution OS — 冻结 Profile 与只读 Plan/Review

未来新增 `xcli_agent` provider，不复用 `app/providers/xcli_local.py` 的 OpenAI HTTP 协议名。`app/model_settings.py` 保存独立类型/授权引用，`contracts.py` / `runtime.py` / `workflow.py` 适配单 turn 状态和本地业务验证。应用已有 `codex_cli.py` 路径保持独立，不将其存在当成 xcli Agent API 已完成。

每个流程冻结：endpoint 实例、client_id/授权版本引用、agent_id、adapter_id、CLI 版本/摘要、policy/profile/capability revision、推理位置、native_model（未知为 null）、合同/业务 Schema 版本、逻辑 Session、累计预算。任务之间不能无声改变 Profile 或 Token scope，不存 Token 明文或登录资料。

| 阶段 | V1 适配协议 | 尚未实现/禁止自动执行 |
| --- | --- | --- |
| Plan | 用户选定合成材料/差异的只读任务；最终 JSON 在应用校验 step_id、输入引用、动作类型、验收条件 | 无 native strict Schema 保证；计划里的命令/路径只是待审数据 |
| Review | 固定 diff/验证记录输入、独立只读 Session；JSON 校验结论/理由/来源 | 审查者不能经工具修改被审内容或发布 |
| Run | 只预留独立 Agent provider 与任务/预算合同 | 真仓库写入、Task 多轮协调、真实命令执行需额外 profile/工作流工程，不在最小 V1 开放 |
| 接收/发布 | 复用应用原有人工接收流程 | xcli succeeded 不授予 merge/push/Issue/PR/部署凭据或工具调用权 |

Plan/Review 的业务 Schema 由应用版本化管理；若现有流程硬性要求原生严格结构化输出，必须拒绝 capability_unverified，不偷降为“解析出来就算严格约束”。业务 JSON 错误不得自动启动修复调用。后续 A04；真实仓库 Run 与发布另立授权。

## Docker / 宿主桥接

V1 仅本机宿主应用，容器中的 Contribution OS 原生路径返回 adapter_unavailable，并显示“需要独立宿主 Agent 桥接”，不能把旧 Model Gateway 的 host bridge 当作可用入口。容器 127.0.0.1 指自身，不能通过改成 host.docker.internal/LAN/0.0.0.0 或放宽 SSRF 来绕过本机边界。

后续桥接至少需要：固定宿主服务实例和双向强认证、应用专属短期任务授权/重放保护、两端预算/步骤/profile 校验、安全 Artifact 通道、取消归属、无代理/重定向、失败关闭及独立 BHOST 验收。不得挂原生登录目录、Keychain、Docker socket 或全盘，不允许容器传任意 shell/cwd/env/原生 ID。旧 13 文件 HTTP 桥接补丁仅供设计参考，不能覆盖当前应用修改。

## 渐进迁移、回退与数据版本

顺序为 Worklog 文本 → GardenFlow 媒体 → Chat Insight 选段 → Contribution OS 只读；每次只新增显式选择的 Agent 模式，不修改既有 HTTP provider 的配置含义。初期 feature flag 默认关闭，发现能力未通过时禁用对应按钮并返回明确原因，不切 HTTP API/其他账号代替。

新服务数据根建议为 XDG_DATA_HOME/xcli/agent-gateway，配置为独立 agent-gateway.yaml；schema_version 从 1 起。新增独立 CLI issue/rotate/revoke/status/serve 为未来控制面，不能把本轮文档当已安装命令。Runstore 只作可选脱敏关联索引，不迁移旧历史；新主存储负责完整结果、归属和事务。

升级前停止接收任务、检查/取消活跃 worker、保留私有状态快照，迁移先验证再原子提交 schema 版本；不支持新版本的旧二进制必须拒绝读写。回退可关闭 Agent 功能并继续旧 Model Gateway，不清空原生认证或复制历史 Session；有未知副作用时先 reconcile，不用回退当作任务取消成功。迁移与回退测试都用合成私有数据，真实重启/异平台/生产启用另留 Gate。

本轮没有改变任何应用源码、配置、数据库或部署。所有能力都需按 [阶段任务](TASKS.md) 和 [应用验收](ACCEPTANCE.md) 分别落地，不能由本 SPEC 的完成推导为应用已可用。
