# Chat Insight 接入适配（Planned / Blocked）

2026-11-06，只读核对 HEAD `920c900a2c8685cc92e9ba4b065cc77216e1b99d`；已有 11 项用户修改，本轮全部保留。未读取聊天数据库、运行 UI、重建索引或修改业务代码。

**原生 CLI 对话接入 BLOCKED，原生 CLI Embedding 能力缺失。** 三个 Coding CLI 的任务/图片能力不能证明存在批量向量接口。没有可发布的 CLI chat/embedding alias 配置。

## 现有代码与新接口边界

`src-tauri/src/ai/client.rs` 和 `gateway.rs` 已有 Model Gateway 目录、聊天/向量路由、云同意与向量身份检查；`settings.rs`、`commands.rs`、`indexer.rs`、`retrieval.rs` 及 `src/ai.ts` 分别承担设置、执行、索引、检索与前端接口。现有 `/v1` 代码是 HTTP 模型客户端，不会因 endpoint 在 localhost 就执行原生 CLI。

本轮静态核对到 `effective_cloud` 把除明确 local 外的位置视为云端；`ensure_consent` 同时检查允许上云和本次同意；索引身份包含 alias、dimensions、version、fingerprint。这些是已存在的源码边界，本轮未运行应用测试或证明真实隐私流程通过。

拟议增加独立 Agent 对话模式，由 Tauri 后台持有客户端授权并提交受控任务。SSE 需使用[拟议 Agent 事件](../INTEGRATION_ARCHITECTURE.md)，不能把其 `task.progress` 当 Chat delta。没有原生增量能力时显示活动状态，等最终文本；不能人工切片伪造逐 token 流。

## 数据同意与失败关闭

每次发送前分别确认聊天模型和 Embedding 模型的真实推理位置；remote/unknown 都需要允许云服务及本次确认。Agent 进程在本机、模型请求经 loopback，均不代表聊天内容不出机器。原生 CLI 可能保存会话、厂商可能处理交互数据；xcli 关闭 raw recording 不覆盖这些行为。

仅选择明确消息范围并导出最小内容到私有任务目录。不得把聊天数据库路径交给 CLI，或允许其自行检索整个数据目录；不得把对话中的指令当作改变权限/云同意的命令。

无目录、能力未知、Token 缺失、同意拒绝、推理位置未知且未授权、会话归属不匹配时，在读私有消息和启动 CLI 之前拒绝。不得回退到普通云 API、其他账号或不受约束的 MCP。输出错误只含类别，不含原始聊天、图片、Token 或 CLI 原始日志。

## Embedding 是独立前置条件

| 项目 | 要求 |
| --- | --- |
| 原生三 CLI 批量向量接口 | 本轮无证据，不作为已支持能力；不能让 Agent 输出随机数/数字列表代替 |
| 可接受的未来接口 | 独立、合法、实际验证的 Embedding backend；明确服务、模型身份、维度、费用和数据政策 |
| 本任务如何处理 | 保持该能力缺失/BLOCKED；不接入普通 API 或 Ollama 冒充 CLI 验收成功 |
| 索引合同 | 向量数量、顺序/index、维度、有限数值、模型 fingerprint 与请求身份一致 |
| 模型切换 | 明确提示重建；禁止把新向量追加到旧模型索引或只改 alias 掩盖身份变化 |

即使原生对话通过，完整 RAG/批量索引仍可能 BLOCKED。可以设计不依赖向量的用户选段总结，但必须在产品中明确其范围；不能以该降级路径声称已完成 Embedding 接入。

重建需要用户单独决定范围、云同意和预算。先建隔离新索引，记录模型身份/维度/版本/fingerprint，完整验证后原子切换；失败/取消保留旧索引。恢复前重新验证身份；请求中路由发生变化时丢弃本批结果，不自动合并。索引指纹不是认证或数据本地性的证明。

## 待确认的改造

| 优先级 | 变更 |
| --- | --- |
| P0（xcli） | Agent API、客户端/会话隔离、真实流事件和取消；原生文本少量真实验收 |
| P1（Chat Insight） | `client.rs` 新增独立 Agent 适配；`gateway.rs` 保留既有 HTTP 边界并为 Agent capability/云位置单独建校验 |
| P1（Chat Insight） | `settings.rs`/`commands.rs`/`src/ai.ts` 增加任务预算、逐请求同意、进度/终止状态，凭据经安全存储引用 |
| P2（Chat Insight） | `indexer.rs`/`retrieval.rs` 分离原生文本任务与独立向量服务，保持索引版本/原子切换和失败关闭 |
| 独立前置 | 获得合法 Embedding 接口及专属预算；未满足前不开放完整原生 CLI RAG 模式 |

Token 只在 Tauri 后台的系统安全存储/受限引用中持有，不交 renderer，不复用原生登录 Cookie/Token。断连只重连已有事件或查任务状态，不自动重新发送聊天/重建索引；部分文本不作为最终摘要写入。取消后确认状态再释放材料，未知副作用不自动 resume。

## 后续验证清单（未执行）

- [ ] 关闭云权限、缺本次同意、unknown、能力目录损坏时私有材料发送和 CLI 启动次数均为 0。
- [ ] 会话跨客户端拒绝；SSE 截断/重复/乱序/无终止与取消正确处理，无假增量。
- [ ] Embedding 缺失时明确禁用；错误数量/维度/NaN/身份变化拒绝，索引保持不变。
- [ ] 用户批准重建后，取消/崩溃/恢复都不混入旧索引，不暗中切换到其他模型。
- [ ] Keyring/Token 撤销和服务断连失败关闭；先合成数据测试，真实聊天和桌面 UI 另行授权。

现有用户修改及历史接入补丁都不是本轮 UI/真实模型证据，不重放旧补丁覆盖当前工作区。
