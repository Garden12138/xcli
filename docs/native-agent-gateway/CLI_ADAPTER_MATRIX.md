# Native Agent Gateway V1 — CLI Adapter Matrix

核查日期：2026-11-06。以下官方页面已实际打开；网页描述可能晚于本机安装版本。`Documented` 不是账号权限、当前版本可用性或本仓库实现证明；工程判断另列。

## 官方程序集成调查

| 入口 | 官方已文档化的范围 | 本仓库复用性与工程判断 | V1 决策 |
| --- | --- | --- | --- |
| Codex exec | 单任务、JSONL、按 ID 恢复和输出 Schema；默认只读。[官方非交互文档](https://learn.chatgpt.com/docs/non-interactive-mode) | `Definition.Run/Resume`、`StreamDecoder`、`runtime.Execute` 已覆盖主要路径；没有 HTTP 身份和可靠原生 delta | 第一条完整任务路径，保留已验收基线 |
| Codex SDK | TypeScript 库支持本地线程创建/继续/恢复，需要 Node；Python 库控制 app-server，并携带固定 runtime 依赖。[官方 SDK](https://learn.chatgpt.com/docs/codex-sdk) | Go 主程序新增 Node/Python sidecar、包/运行时版本和取消桥接；不能假设两种 SDK 传输相同，也不能让依赖悄悄替换本地 CLI | 非 V1 必选依赖；记录为官方候选，暂不安装 |
| Codex app-server | 公开双向协议，初始化、thread/turn、消息 delta、审批和 interrupt；可生成与 CLI 版本对应的 schema。[官方 App Server](https://learn.chatgpt.com/docs/app-server) | Go 可通过 stdio 对接，无需引入 SDK sidecar；现有单向 stdout 解码器不能直接处理双向 RPC | 提前做离线协议评估；通过权限/取消/媒体同等验收后才可显式选择，不能自动替代 exec |
| Cursor Headless CLI | print 面向脚本自动化；stream-json 可报活动，partial 需另开开关。[官方 Headless](https://cursor.com/docs/cli/headless) | 已有 argv/Session/result 适配；本轮 trust 失败，当前不能部署为可用路由 | 实现错误及发现路径；真实服务启用等本人审查 trust 与新预算 |
| Antigravity Headless CLI | JSON/NDJSON、conversation 续接、连续 stdin；连续输入仅 text，headless 审批可能 soft-deny 后 exit 0。[官方 Headless](https://antigravity.google/docs/cli/headless/) | 单次 print 可复用；持续 stdin 需要独立 turn 预算、输入背压和退出状态机；不能把 control_response 当审批接口 | V1 单次 print；持续进程推迟 |
| Antigravity SDK | 本地 Python Agent harness；快速入门使用 Gemini API Key，企业路径可使用 GCP 认证。[官方 SDK](https://antigravity.google/docs/sdk/overview/) | 是另一运行与认证路线；未证明等价于本机已登录 CLI 或同一额度，SDK session 也不能直接当 CLI conversation | 不替代本轮 CLI 路线；只有单独选型、授权、计费与验收后再增加 |

认证边界：Codex 官方区分 ChatGPT 登录与 API Key 登录；通用 OpenAI API 仍使用 Platform API Key。[官方认证](https://learn.chatgpt.com/docs/auth) 本设计只让官方进程按已有认证运行，不提取凭据、不把订阅访问包装为任意 API。另有应用专属授权路线时应独立设计，不能用原生登录缓存代替应用授权。

Cursor partial 输出的流片段与 buffered/final flush 需按官方规则去重，失败可能没有终止 result。[官方输出格式](https://cursor.com/docs/cli/reference/output-format) Cursor 的权限存在全局和项目范围，Gateway 不能仅靠工作目录实现隔离。[官方权限](https://cursor.com/docs/cli/reference/permissions)

Antigravity 的权限、模式和 sandbox 分别影响执行；V1 不采用自动批准模式。[官方权限](https://antigravity.google/docs/permissions/) 官方说明额度/credits 可在 CLI 界面查看，并存在 credits 使用配置；这不构成 xcli 可稳定查询或强制金额上限的接口。[官方额度说明](https://antigravity.google/docs/cli/credits/)

## 本机真实证据与适配覆盖

| 维度 | Codex 0.167.0 | Cursor 2026.10.22-27dcf5f | Antigravity 1.5.2 |
| --- | --- | --- | --- |
| 本轮新文本 | PASS | BLOCKED：workspace_trust，exit 1 | PASS |
| 同 Session 续接 | PASS | NOT_TESTED，前置阻断 | PASS |
| 单张原生图片生成/编辑 | 各 PASS，有 PNG 文件/视觉 | NOT_TESTED，前置阻断；不能说不支持 | 各 PASS，有 JPEG 文件/视觉 |
| 当前终止事实 | `turn.completed` + exit 0 | 成功协议需 terminal result；本轮无结果 | `result.status=SUCCESS` + exit 0 + 无 denied_actions |
| 当前会话字段 | thread_id | session_id | conversation_id |
| 原生细粒度文本 delta | 本轮 NOT_TESTED；exec item 不作 delta | NOT_TESTED；当前 xcli 未开 partial | NOT_TESTED；当前 decoder 只归一化最终文本 |
| 独立 Vision/严格 Schema | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 真实主动取消/超时/并发 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| Gateway HTTP→CLI | 未实现 | 未实现 | 未实现 |
| 图片工具来源 | 本轮隔离会话提取工具名补证，外层没直接显示 | 本轮未执行 | 父级 invoke_subagent 可见，内部调用数不透明 |

证据只使用[本轮矩阵](../cli-first/NATIVE_CLI_CAPABILITY_MATRIX.md)、[文本](../cli-first/evidence/native-20261106/text.json)、[媒体](../cli-first/evidence/native-20261106/media.json)及[来源补证](../cli-first/evidence/native-20261106/codex-native-provenance.json)。旧版本媒体记录保留为历史，不能使当前 Cursor 通过。

## Adapter 规范化要求

| 项目 | 必须实现的规则 |
| --- | --- |
| 安装与版本 | 用配置的精确 executable 摘要和版本匹配 profile；GET 能力只读缓存；非推理探测由本人显式 refresh，设超时/上限，不登录 |
| 原生命令 | server 生成固定 argv，用户 prompt 为数据参数/受控 stdin；API 不接受 native args；Codex resume 的 sandbox 参数差异沿用已有转换并做回归 |
| Session | 捕获原生明确返回的 ID 并记录来源；恢复后缺 ID 不能拿 runtime fallback 证明同会话；不使用 --last/--continue latest |
| 终止与错误 | native exit、terminal、parse、denial、validation 各自保存；trust/认证/额度先于缺终止 JSON 分类；无凭据错误原文回传 |
| 工具 | declared tool catalog 不算调用证据；未知全局 MCP/hooks/plugins 不可隐式启用；需要审批时阻断，不写 allow 或发送 approve |
| 实时流 | 既有 activity 可先复用；新增 typed observer 才能暴露 delta/model/denial/artifact 候选；final 文本绝不重切成流 |
| Usage | 保留上报来源和 turn/session/unknown 范围，缺失字段为 null；累计值不能反复相加；重连次数不等于模型账单 |
| 图片 | 只导入绑定当前任务的允许文件；路径/mtime/工具名均不能单独证明合法产物；外层不可见时进入复核，不能直接制造失败或成功 |
| 取消 | exec 使用进程组；app-server 需要 interrupt 后等 terminal，再按独占 worker 终止；本地停止与云端撤销分离 |

`internal/agent.Capabilities()` 的 `supported:true, source:declared/runtime` 只可作为调查输入；不能直接发布成新服务的 verified 能力。`structured_output` 在旧配置里主要指事件信封，不能映射为业务 `native_json_schema`。

## 决策与待验证问题

建议先固定 `codex-exec-v1` 与 `antigravity-print-v1`；`cursor-headless-v1` 保留实现和明确不可用状态。所有传输都只用公开官方入口。不是因为 exec 已实测就跳过 app-server：G1 先确认双向协议、schema 版本和审批边界；再决定是否在独立里程碑替换。

启用 app-server 前必须证明：同等权限不扩大、每 turn 单独扣 dispatch 预算、会话归属不混用、interrupt 终止可确认、动态工具不泄漏、图片可安全交付、版本漂移关闭。任一不满足维持 exec；不在执行失败后切换传输重试。

仍需工程实测的问题包括：各版本对全局插件/网络/文件读取的可约束程度；当前登录是否可在服务工作区内运行；媒体是否能固定输出到任务目录；原生存储丢失后的恢复行为；内部重试及云任务是否有可观测取消。全部保持待验证，不能用本轮已有成功补齐。
