# Native Agent Gateway V1 — API Contract

规范性 V1 合同，2026-11-06；实现与验收状态见 [IMPLEMENTATION_PROGRESS.md](IMPLEMENTATION_PROGRESS.md)。2026-11-07 已有独立路由，真实 native profile 仍关闭。所有示例是合成数据，不是本轮真实调用结果。协议不接受 OpenAI `messages/model/response_format/tool_calls` 的替代映射。

## 传输、身份和通用规则

独立 loopback listener，前缀 `/agent/v1`；与 Model Gateway 分端口/进程/配置/Token。所有业务端点要求且只接受一个 `Authorization: Bearer <application-token>`；Token 不可放 URL。只允许本人固定的 literal loopback endpoint，无重定向、环境代理、CORS 或任意 Host/Origin。`GET /healthz` 可未认证，只返回存活，不暴露账户、安装或任务。

JSON UTF-8，响应包含 `api_version:1`；ID 为服务生成至少 128-bit 随机 opaque 字符串。时间为 UTC RFC3339；金额不做浮点推算。请求对象字段闭合、拒绝未知字段、重复 JSON key、尾随 JSON、非法 Unicode/非有限数字。枚举未知值拒绝；客户端可忽略响应新增字段及未知非终止事件，但不能据未知事件推断完成。

所有响应 `Cache-Control:no-store`、`X-Content-Type-Options:nosniff`。JSON 请求 ≤1 MiB，单 prompt ≤256 KiB，最终文本 ≤1 MiB；查询 Task 序列化响应 ≤8 MiB。超限失败，不默默截掉正文后返回成功。内部 native event 仍受有界解析；公开事件 ≤64 KiB。

所有创建/修改请求要求 `Idempotency-Key`（8–128 ASCII 可打印字符，不含秘密）；取消和 close 也可重复。键按 client_id + method + route + key 绑定规范化请求摘要和全部输入哈希。保留 30 天：同键同摘要返回原对象及 `Idempotency-Replayed:true`，同键异摘要为 409；事务恢复不能多 dispatch。已执行且业务结果过期时重放返回 410，绝不重新执行。客户端不在保留期后重放旧动作；服务不承诺无限期 exactly-once。

对象归属来自 Token，绝不信任 body 中 client_id；跨客户端 Session/Task/Input/Artifact 统一 404。权限 scope 为 session:create/read/close、input:write、task:create/read/cancel/review、artifact:read，另有本人单独授予的 artifact:attest-origin。允许的 Agent、operation、policy、remote 与累计预算也属于服务端授权。默认四应用无来源证明的代签权限。

## 路由

| 路由 | 输入 | 成功响应/副作用 |
| --- | --- | --- |
| GET `/agent/v1/capabilities` | 无 body | 200 仅当前客户端可见缓存，无 native 启动 |
| POST `/agent/v1/sessions` | SessionCreate | 201 Session；无 native 启动 |
| GET `/agent/v1/sessions/{id}` | 无 body | 200 Session，不返回原生 ID 或历史正文 |
| POST `/agent/v1/sessions/{id}/close` | `{}` | 200 已关闭，或 202 正在取消活跃任务；重复不新执行 |
| POST `/agent/v1/inputs` | 受限原始字节，见下文 | 201 Input；只本地暂存，无推理 |
| POST `/agent/v1/tasks` | TaskCreate | 202 Task；事务接受，排队，非成功推理证明 |
| GET `/agent/v1/tasks/{id}` | 无 body | 200 Task，包含当前最终/部分结果及验证事实 |
| GET `/agent/v1/tasks/{id}/events` | 可选 Last-Event-ID | 200 认证 SSE，只重放/观察 |
| POST `/agent/v1/tasks/{id}/cancel` | `{}` | 活跃 202；终态 200 原状态；不保证云撤销 |
| POST `/agent/v1/tasks/{id}/review` | Review | 200 终态；不完整为 422/403，保持待复核；不调用模型 |
| GET `/agent/v1/artifacts/{id}` | 无 body | 200 元数据，含 review_only/ready 等交付状态 |
| GET `/agent/v1/artifacts/{id}/content` | 无 body | 200 原始字节；需授权且未过期；preview 也必须认证 |

V1 无任意 native-session import、`tasks/{id}/resume`、在线工具 approve、金额查询、embedding 或模型兼容路由。续接是对同一个逻辑 Session 创建**新 Task**；每个新 Task 单独消耗预算。旧[架构草案](../cli-first/INTEGRATION_ARCHITECTURE.md)中的 task resume 路由由本合同取代，旧草案不是已有客户端协议。

## 能力目录

```json
{
  "api_version": 1,
  "instance_id": "agt_instance_example",
  "revision": "cap-example-1",
  "generated_at": "2026-11-06T14:00:00Z",
  "agents": [
    {
      "agent_id": "codex",
      "adapter_id": "codex-exec-v1",
      "installed_version": "0.167.0",
      "binary_sha256": "example-digest",
      "inference_location": "remote",
      "authentication": "unknown",
      "remaining_quota": null,
      "native_model": null,
      "operations": {
        "text_task": {
          "documented": true,
          "native_evidence": "verified",
          "gateway_evidence": "unverified",
          "availability": "unverified",
          "reason": "gateway_not_accepted",
          "evidence_ref": "cli-first-20261106-text",
          "verified_at": "2026-11-06T12:50:22Z"
        }
      },
      "features": {
        "activity_events": true,
        "native_text_delta": false,
        "native_json_schema": false,
        "embedding": false,
        "tool_approval": "deny_only"
      },
      "policies": ["synthetic-text-readonly-v1"]
    }
  ]
}
```

`binary_sha256` 生产必须为 64 位十六进制；示例值仅作占位。安装版本或模型未知为 null，不由 Agent 名推算。`inference_location=local|remote|unknown`，unknown 按云处理。`authentication=authenticated|unauthenticated|unknown` 只来自允许的官方状态/本次观察，不读凭据，不保证未来请求成功。

每个 operation 必须列 `documented`、`native_evidence`/`gateway_evidence`（verified/unverified/unsupported）、`availability`（available/blocked/unverified/unsupported）、`reason`、nullable evidence_ref/verified_at。证据另含版本/适配 revision/策略 digest/场景/来源范围；单张编辑不能验证 Vision，native 通过不能验证 Gateway。只对本人注册且 Gateway 验收通过的 profile 标 available；explicit blocker 优先于旧 PASS。版本或策略漂移使 revision 变化并退回 unverified。

`instance_id` 是服务数据根的持久身份，重启不变；重建/切换状态根必须改变，应用不能跨实例续接。`GET` 不运行 version/login/models 或小提示测试。本人控制面的显式 refresh 可执行有界官方非模型探测，和 dispatch 账本分开记录；不启动登录。余额面板或帮助输出不冒充机器可强制额度。

## Session

SessionCreate 必填 agent_id、adapter_id、capability_revision、policy_id、budget、context；各字符串只接受目录/授权存在的值。context 仅 `app`（worklog/gardenflow/chat-insight/contribution-os）、`workflow`（应用自定义短标签，≤64 字符）、`opaque_ref`（≤128，不含业务正文或路径）。

```json
{
  "agent_id": "codex",
  "adapter_id": "codex-exec-v1",
  "capability_revision": "cap-example-1",
  "policy_id": "synthetic-text-readonly-v1",
  "budget": {
    "max_dispatches": 2,
    "max_process_starts": 2,
    "max_execution_seconds": 300
  },
  "context": {
    "app": "worklog",
    "workflow": "report-draft",
    "opaque_ref": "synthetic-period-1"
  }
}
```

Session 响应必含 id、revision（从 1 起）、state、binding_state、agent_id、adapter_id、capability_revision、policy_id、policy_revision、profile_sha256、context_sha256、budget（上限/已用/预留）、active_task_id（nullable）、created_at、expires_at、inference_location。native_model 为 nullable，未知/未观察不填；native_session_id 永不公开。

`state=open|closing|closed|needs_reconcile|expired`；`binding_state=unbound|bound|lost`。新建为 open/unbound，首次成功观察原生会话 ID 后 bound；创建本身零模型调用。首次成功结果缺 ID 为 failed/protocol_error 并封存 Session；续接时丢失/不同原生 ID 为 failed/native_session_lost 并封存，不能创建新原生会话却保持“已续接”。任何 dispatch 后缺完整会话/终止事实的失败也封存为 needs_reconcile。

revision 每次接受任务或改变上下文/状态时递增，Task 创建必须 CAS 匹配。context_sha256 是服务对本 Session 已授权输入和任务上下文的摘要，不向应用返回原生历史正文。续接携带最新摘要并明确同意该累计范围。上传输入本身不加入原生上下文，只有 dispatch 后才纳入。

Session 同时至多一个非终态 Task，含 awaiting_review；冲突 409 session_busy。close 禁止新 Task，取消已有 Task 后关闭；遇未知副作用保持 needs_reconcile，不能伪报安全关闭。V1 对 needs_reconcile 不提供自动清除/继续 API，需本人检查并关闭，另建新 Session；不自动复制旧上下文。

Session idle TTL 24 小时，绝对最长 7 天；活跃任务不在清理中被直接删除。到绝对上限先停止执行/结束待审，再按停止事实封存或过期；不直接删活 worker。过期不再续接，原生历史不保证随之删除。冻结版本/适配器/策略不可修改，变化需新 Session。

## 输入上传

POST `/inputs` 要求 Content-Type、X-Xcli-Session-ID 和 X-Xcli-Content-SHA256（原始字节 SHA-256），不接受文件名、URL、原生路径或 multipart 的附加字段。body 为原始字节；未知长度也必须流式限额，不能先无界读取再判大。

| 类型 | 上限/验证 | 用途 |
| --- | --- | --- |
| text/plain; charset=utf-8 | 单文件 1 MiB、有效 UTF-8；每 task 最多 4 个，文本总量 2 MiB（含 prompt） | text_task 的显式材料 |
| image/png、image/jpeg、image/webp | 单文件 20 MiB、16,777,216 像素、完整解码/哈希/真实 MIME 一致 | image_edit 唯一参考图 |

Input 响应含 api_version、id、session_id、media_type、byte_length、sha256、validation（passed）、created_at、expires_at；图片另含 width/height。任何未知/跨 Session 的输入 ID 404，已过期 410。输入属于同 client + session，服务不可变，最多保留到 Session 清理；不提供任意文件读取 API。

从已生成 Artifact 再编辑时，V1 由应用认证下载并核对字节后重新上传；不添加任意服务器复制路径接口。应用应在导出私有材料之前取得云同意；服务对未获 dispatch 同意的材料只作本地暂存，绝不送原生 CLI。

## Task 请求

TaskCreate 必填 session_id、expected_session_revision、operation、input、budget、consent。operation 为 text_task/image_generate/image_edit。input 为 1–5 项的有序数组：`{type:"text",text:string}` 或 `{type:"file",input_id:string}`，两类字段互斥；必须恰有一个非空 text，文件必须属于本 Session。纯文本任务只接受文本文件；生成不接受参考；编辑必须且只能一个图片输入，不接受文本附件。独立 Vision 不在 V1。

```json
{
  "session_id": "ses_example",
  "expected_session_revision": 1,
  "operation": "text_task",
  "input": [{"type": "text", "text": "总结这份合成材料，不执行外部动作。"}],
  "budget": {"max_dispatches": 1, "max_process_starts": 1, "timeout_seconds": 120},
  "consent": {"allow_remote": true, "context_sha256": "example-context-digest"}
}
```

task budget 两个 max 字段固定为 1；不允许多轮协调/自动修复。timeout_seconds 为 1–600 的整数，文本建议 120、媒体建议 180，包含排队/启动/执行/自动验证。最长排队 min(30 秒, task 剩余时间)。停止清理额外硬上限 5 秒，含最多 2 秒 TERM grace 和剩余 KILL/管道收尾；不含人工复核等待。

Session budget：max_dispatches/max_process_starts 为 1–6；max_execution_seconds 为 1–3600；默认申请建议 2/2/300，具体不得超过客户端/本人全局上限。创建时声明预算不等于预扣全部额度；每 dispatch 前在所有级别原子预扣次数并预留本次墙钟上限，失败不返次数。排队期间未 dispatch 的取消不扣次数。原生内部推理/重试不受次数精确约束。

`max_cost_usd` 为唯一保留的硬金额字段（Task/Session budget 均可出现）：原生额度不可机器强制时，无论值多少都返回 422 budget_unenforceable，零调用；不能静默忽略。temperature/max_tokens/seed/stop/n/size/mask/models 等未知字段返回 unsupported_parameter。所有上限从本人控制面设置；未配置服务日预算时为 0，不从本轮验收额度继承。

consent 必须含 allow_remote（布尔）与当前 context_sha256；digest 生产为 SHA-256。remote/unknown 必须 allow_remote=true 且客户端有 AllowRemote。服务在接受事务内把该同意绑定本次规范化请求、所有新输入哈希和原累计上下文；并发/context 变化拒绝 consent_scope_changed。创建 Task 后 body/输入/策略不可变；应用不得将“总是同意”隐藏在重试中。

不支持通过请求指定 cwd、env、proxy、native_args、原生模型名/账号、工具权限或任意原生 Session。模型选择未来若增加，须本人登记 profile 后新建 Session。

## Task 响应与状态机

```json
{
  "api_version": 1,
  "id": "tsk_example",
  "session_id": "ses_example",
  "operation": "text_task",
  "status": "succeeded",
  "revision": 4,
  "execution": {
    "state": "completed",
    "native_exit_code": 0,
    "native_terminal": "success",
    "local_stop_confirmed": true,
    "upstream_cancel_confirmed": null,
    "side_effects": "none_observed"
  },
  "protocol": {"status": "complete", "native_session_observed": true},
  "validation": {"status": "passed", "bytes": "not_applicable", "provenance": "not_applicable", "semantics": "not_applicable"},
  "result": {"text": "合成摘要", "final": true, "artifacts": [], "native_model": null},
  "usage": {"source": "native_cli", "scope": "unknown", "input_tokens": null, "output_tokens": null, "total_tokens": null, "cache_read_tokens": null, "reasoning_tokens": null, "cost_usd": null, "remaining_quota": null},
  "budget": {"dispatches_consumed": 1, "process_starts_consumed": 1, "execution_seconds": 10.6},
  "error": null,
  "created_at": "2026-11-06T14:00:00Z",
  "deadline_at": "2026-11-06T14:02:00Z",
  "ended_at": "2026-11-06T14:00:11Z",
  "last_event_seq": 5
}
```

ID/时间/digest 为示例。Task 还必须返回 agent_id、adapter_id、profile_sha256、capability_revision 和结果 expires_at；Task/Session revision 分开。execution/protocol/validation/result/usage/budget/error 全部存在，可为明确 nullable 字段，不依赖 omission 区分失败。新 accepted Task result.text 为 null、final=false、artifacts=[]，native_exit_code 为 null。

| status | 合法后继 | 含义 |
| --- | --- | --- |
| queued | running / blocked / canceled / timed_out | 尚未 dispatch；策略/预算再检查，拒绝零启动 |
| running | validating / cancel_requested / blocked / failed / timed_out / needs_reconcile | 原生执行；观察到完整 terminal 与退出后才验证 |
| cancel_requested | canceled / timed_out / needs_reconcile | 请求已记账，正在停止；不能因晚到成功变 succeeded |
| validating | succeeded / awaiting_review / failed / blocked / cancel_requested / timed_out / needs_reconcile | 自动校验结果/输入完整性/来源 |
| awaiting_review | succeeded / failed / canceled / expired / blocked | 原生已结束，图片仅可预览；review 不调用模型 |
| succeeded / blocked / failed / canceled / timed_out / expired / needs_reconcile | 无 | 固定终态；GET 永不触发继续 |

deadline 与 cancel 按同一状态锁排序：截止时间已到则 timeout 为停止原因；否则取消先提交后不能成功。终态持久化与 terminal 事件同事务；最多一条任务终止事件。任何已启动 worker 在 blocked/failed 等终态发布前也必须停止确认；5 秒清理界限内无法确认则使用 needs_reconcile，不伪报已安全收敛。已知本地停止且云副作用未知时允许 canceled/timed_out，同时 `side_effects=unknown` 并封存 Session 为 needs_reconcile。

execution.state 为 not_started/running/completed/failed/stopped/unknown；native_terminal 为 success/failure/interrupted/missing/unknown；local_stop_confirmed 为 true/false/null；upstream_cancel_confirmed 为 true/false/null；side_effects 为 none_observed/observed/unknown。`none_observed` 不保证云无费用。

protocol.status 为 not_started/complete/parse_error/truncated/unknown；native_session_observed 是原生本次真实返回布尔，不能由 fallback 填 true。native 缺 terminal/坏 JSON 为 failed/protocol_error，除非已识别 trust/auth/quota 等阻断优先分类；不把权限拒绝描述成“模型 JSON 格式错误”。

validation.status 为 not_run/running/passed/failed/pending_review；bytes/provenance/semantics 为 not_applicable/unverified/passed/failed。succeeded 必须执行/协议完整且所有适用验证 passed；native exit 0、文字“完成”、合法路径或文件哈希不能独自证明成功。文本只验证成功结果存在/上限/本地最低合同；业务正确性由应用二次验收。

result.text 可以保留部分文本但 final=false，应用不能把它当最终报告。Usage 缺失为 null，scope 为 turn/session/unknown，source 为 native_cli/native_app_server/none；本机不得伪造 usage、模型身份或费用。未来可加入原生累计基线，不能反复累加 session 总量。

## 事件与 SSE

SSE 使用 `text/event-stream`，id 为本 Task 的单调 seq，从 1 起；每条 data 为对象，必含 api_version/task_id/session_id/seq/time/type/payload。服务先记 journal 再通知 SSE；重连 `Last-Event-ID` 仅重放更大 seq，不新建 Task。应用按 task_id+seq 去重，以 GET Task 和终止事件确认完成。

| type | payload 最小字段 | 语义 |
| --- | --- | --- |
| task.queued / task.started | task_revision | 调度/已 dispatch 事实 |
| task.progress | category（session/tool/response/activity） | 脱敏活动，可聚合但不称 token |
| text.delta | message_id、delta、source_event_id | 仅有真实增量且能力启用；过大公开片段拒绝为 output_limit，禁止从 final 制造 |
| text.completed | result_sha256、utf8_bytes、result_path | 最终文本引用；result_path 仅同 origin 固定 Task 路由，不传任意 URL |
| approval.required | category、can_approve:false | 必要工具被拒/待审批，随后 blocked；无 approve 接口 |
| artifact.reviewable | artifact_id、sha256 | 字节可预览，未获最终接收 |
| artifact.ready | artifact_id、sha256 | 来源/视觉通过，允许应用显式入库 |
| task.awaiting_review | task_revision、review_deadline_at | 等人工复核；不是成功 |
| task.completed | task_revision、result_sha256 | 仅 succeeded，terminal |
| task.blocked / task.failed / task.canceled / task.timed_out / task.expired / task.needs_reconcile | task_revision、error_code | 对应固定终态，terminal |

```text
id: 2
event: task.progress
data: {"api_version":1,"task_id":"tsk_example","session_id":"ses_example","seq":2,"time":"2026-11-06T14:00:01Z","type":"task.progress","payload":{"category":"tool"}}
```

没有原生 delta 时只有活动与最终文本引用，UI 可自行展示等待，不能假装逐 token 输出。delta 与最终结果需按 message_id 归并；最终结果是完整权威正文，不能把两者再次拼接。最终文本只在已完整观察时发 text.completed，仍不代表业务验证/图片任务成功。

每 15 秒 `: heartbeat`（无 seq）；认证失效则关闭连接。客户端慢时断开观察者，任务继续；不让网络 sink 错误变成 native 重跑。日志每 Task 最多保留最近连续 10,000 事件/8 MiB（较早进度可压缩），终态必须保留。旧 cursor 不在保留窗为 410 event_cursor_expired，客户端 GET 状态后从服务给定游标继续；cursor 大于 latest/非法为 400 invalid_cursor。事件与结果默认保留 7 天；终态后 SSE 正常关闭，意外 EOF 不是成功。

## 图片元数据、审查和交付

Artifact 必含 id、task_id、session_id、media_type、byte_length、width、height、sha256、created_at、expires_at、delivery_state、provenance、validation、reference_sha256（nullable）；不返回 native 路径或可匿名下载 URL。media_type 只允许 image/png、image/jpeg、image/webp；只保留原始真实格式，无重命名转码。

delivery_state 为 review_only/ready/quarantined/expired。仅字节检查 passed 且任务未 canceled/timed_out/blocked 时允许 review_only 预览；ready 还要求来源与语义 passed。quarantined 不通过内容端点交付；不存在/跨客户端 404，过期 410，尚未可预览 409 artifact_not_ready。下载按实际 MIME、Content-Length、强 ETag（SHA-256）及服务生成的 Content-Disposition 文件名返回，不接受用户文件名/路径。应用应再次校验 SHA/MIME/大小；禁止重定向下载。

provenance 包含 status（unverified/passed/failed）、method（native_event/task_trace_attestation/null）、observed_tool（nullable，仅实际观测）、evidence_sha256（nullable）、reviewer_id（nullable）。外层不见 image 工具不直接 FAIL；必须按以下明确复核，不能仅从文件存在推导 native_tool。

Review 请求必填 decision（accept/reject）、artifact_id、expected_sha256；accept 还须 semantic_check（generated_as_requested 或 edit_matches_requested_change），必须符合 operation。编辑参考哈希不变、非输入覆盖、完整字节检查是机器硬条件，任何人工确认不能覆盖。

来源已经由当前任务实际工具事件证明时，普通 task:review 可以做语义接收。来源未证明时，accept 必须另有 provenance_attestation 对象（source 固定 native_task_trace、evidence_sha256 为本人检查本任务原生工具痕迹的摘要），并且 Token 有 artifact:attest-origin；记录人工证明方法，不能伪写成原生事件已观测。普通应用无该权限时返回 provenance_unverified 并保持 awaiting_review。

拒绝则 failed/validation_failed；接受要求 native 终止、无权限/账单阻断、文件归属/真实字节/参考完整性均 passed，不能代替原生工具权限批准。复核截止取进入待审后 24 小时与 Session 绝对到期时间的较早者，届时 expired，不触发修复/重生成。用户未能确认来源就不能发布 ready。取消后晚到图片保持隔离，应用不得导入。

## 错误与失败语义

同步错误表示请求未被接受，无新 Task/dispatch；异步错误在已创建 Task.error 中使用同样 Error 对象，并通过状态区分 blocked/failed。错误中的 accepted/task_id 明确消除网络重试歧义；连接丢失时按原幂等键查询/重放接受响应，不生成新键重做。

```json
{
  "api_version": 1,
  "error": {
    "code": "capability_unverified",
    "category": "capability",
    "message": "此 Agent 能力尚未通过当前 Gateway 的验收。",
    "accepted": false,
    "task_id": null,
    "automatic_retry_allowed": false,
    "remediation": "owner_review_required"
  }
}
```

Error 字段必含 code/category/message/accepted/task_id/automatic_retry_allowed/remediation；message 为固定脱敏文本。category 为 validation/auth/permission/capability/budget/availability/execution/protocol/artifact/state/internal。automatic_retry_allowed 永远 false，不表达“换账号再试”。remediation 为 none/refresh_capabilities/owner_review_required/retry_as_explicit_new_task/reconcile_required；后两者不是服务自动动作。Task.error 中 accepted=true、task_id 为当前 ID。

| HTTP（同步） | code | 行为/异步归类 |
| --- | --- | --- |
| 400 | invalid_request / invalid_cursor | 合同无效；零启动 |
| 401 | invalid_client_token | 本地应用授权错误，不等于原生登录过期 |
| 403 | scope_denied / cloud_consent_required / policy_denied / origin_denied | 授权拒绝；零启动 |
| 404 | not_found / agent_not_found | 不泄露其他客户端对象 |
| 409 | idempotency_conflict / session_busy / session_revision_changed / capability_changed / consent_scope_changed / profile_changed / artifact_not_ready | 不新 dispatch；需重读状态，不能默认重试推理 |
| 410 | object_expired / event_cursor_expired / idempotent_result_expired | 不复活旧任务 |
| 413 | request_too_large / input_too_large / image_dimensions_exceeded | 不截断成功 |
| 422 | unsupported_parameter / capability_missing / capability_unverified / budget_unenforceable / invalid_image / input_hash_mismatch / provenance_unverified / review_incomplete | 明确能力或输入不足；零启动，review 保留原 Task |
| 429 | client_rate_limit / queue_full / budget_exhausted / concurrency_limit | 服务本地限制；不作为厂商配额证明 |
| 503 | agent_not_installed / adapter_unavailable / policy_unenforceable / service_stopping / store_unavailable / store_corrupt | 当前不可执行，零启动 |

已接受后才发现的阻断：workspace_untrusted、native_auth_required、native_quota_exceeded、extra_payment_required、approval_required、native_tool_unavailable、artifact_access_blocked、service_restarted、budget_exhausted → blocked。实际启动已扣预算，不能因 blocked 返还。明确 NOT_SUPPORTED 的 operation 为 capability_missing，未知用 capability_unverified，不从 trust 误推不支持。

运行错误：start_failed、native_process_failed、native_session_lost、protocol_error、output_limit、validation_failed → failed；自动 deadline 为 timeout/queue_timeout → timed_out；用户取消为 canceled → canceled；状态/副作用不能确认用 execution_state_unknown → needs_reconcile。拒绝/额度诊断不能被后续 exit 0、原先的成功文本或有效图片盖过。

所有业务结果、事件、Artifact 和幂等记录均经归属/撤销检查。Token 轮换保留相同 client_id 的可见对象；撤销取消活跃任务并拒绝访问，重新授权也不自动执行旧任务。默认结果/事件/Artifact 7 天，Input/工作区随 Session 24 小时 idle/7 天绝对上限清理；本地清理不能承诺删除厂商历史。
