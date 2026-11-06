# Worklog 接入适配（Planned / Blocked）

2026-11-06，只读核对 HEAD `e38f2a4901d871777989ffee6684e836370e70aa`，初始干净。本轮未改业务代码、未运行 UI、未发日报数据或调用模型。

**CLI 接入 BLOCKED。** Worklog 现有 `xcli` provider 是 Model Gateway HTTP 客户端；配置中的 `xcli/text-default` 是 HTTP 模型 alias 的默认占位，不代表原生 Codex、Cursor 或 Antigravity 已可用。没有本轮可发布的 CLI endpoint/alias/Token 配置。

## 当前入口与拟议模式

已有入口在 `src-tauri/src/llm.rs`（报告、模板、JSON 请求）、`xcli.rs`（本机模型目录/HTTP 传输）、`commands.rs`（provider 配置/连接检查）、`secrets.rs`（凭据存储）。这些代码的存在不是当前真实模型或 UI 通过；本轮只核对路由边界。

终端可在本人合成材料上使用 xcli `run/task`，但该入口还不是 Worklog UI 的模型 provider。拟议新增独立 Agent 任务模式，不能将旧 provider 的 URL 改成不存在的 `/agent/v1`。设计合同见[架构](../INTEGRATION_ARCHITECTURE.md)，能力以[矩阵](../NATIVE_CLI_CAPABILITY_MATRIX.md)为准。

| 工作流 | 拟议输入与输出 | 验收条件 |
| --- | --- | --- |
| 日报 | 用户选择的合成日期/工作项 → 文本报告 Artifact 或最终文本 | 日期范围、事项可追溯；没有材料不编造产出 |
| 周报 | 明确周范围及合成日志 → 汇总、阻碍、下周计划 | 不跨周/混合用户数据；支持取消、显式重试 |
| 填写模板 | 已选择模板和允许变量 → 保留格式的正文 | 未知变量拒绝；内容不修改模板权限或启用工具 |
| 从示例生成/优化模板 | 合成示例 → 草稿模板 | 先预览后保存，不能因原生工具调用直接覆盖模板 |
| 结构化报告 | 文本任务 → 解析并本地校验 JSON | JSON 语法与业务 Schema 均通过才保存；不是原生严格 Schema 承诺 |

拟议应用侧 JSON 示例，仅用于本地 Schema 讨论，不是本轮模型输出：

```json
{
  "period_start": "2026-11-02",
  "period_end": "2026-11-06",
  "summary": "合成验收示例",
  "work_items": [{"text": "验证示例任务", "source_ids": ["synthetic-1"]}],
  "blockers": [],
  "next_steps": ["人工核对草稿"]
}
```

缺字段、类型不符或无法对应来源时拒绝落库。严格 Schema 未经真实验证，不声明 `json_schema` 能力；需要返修必须显示为新的收费调用，不能自动消耗六次文本 smoke 以外预算。

## 需要变更的模块

| 优先级 | 待确认工程 |
| --- | --- |
| P0（xcli） | 先验收原生文本，再实现独立 Agent API 的认证、限次、取消和会话归属 |
| P1（Worklog） | `llm.rs` 新增 Agent 任务结果转换，保留现有 HTTP 模型模式；`xcli.rs` 旁新增独立 Agent transport，不假装兼容 chat/completions |
| P1（Worklog） | `commands.rs` 及对应前端配置显示 Agent 入口、证据状态、每任务预算与同意；只读 capability 检查不能发模型探测 |
| P1（Worklog） | `secrets.rs` 及配置存储使用独立客户端 Token 的 Keyring 引用；不把原生 CLI 登录 Token 填入 api_key |
| P2（Worklog） | 任务进度、取消、草稿确认与 JSON/模板校验；后台执行和重连单独验收 |

这些尚未实施，不重复应用 `docs/model-gateway/integrations` 的历史完整补丁。每次改造前重新核对 HEAD/用户 diff。

## 断连、错误与安全

工作日志只在用户选择并同意后导出最小字段到私有任务工作区。推理位置 remote/unknown 需说明上云，不能因请求发往 loopback 就按本地处理。默认不开放 shell、MCP、网络抓取或全盘读取；日报内容中的路径/命令当普通数据。

客户端授权和原生账号认证分离。Keyring 失败时不静默退回前端明文/localStorage，不记录 Token、日志正文或原始 CLI stderr。Token 轮换后重新验证客户端权限，不主动重新调用模型。

断连不自动重发创建任务，重连只查状态/重放已有事件；取消后等待最终确认，未知状态保留草稿并提示核对。401 要修复本地客户端授权，403/approval_required 要核对权限，quota/timeout/parse/schema 错误保留脱敏原因；均不静默切换普通 API 或其他账号。

## 后续验证清单（均未执行）

- [ ] 先用确定性 transport 验证日报、周报、模板及 JSON 的成功/损坏/超限响应。
- [ ] 云拒绝、能力不足、未认证、预算耗尽时原生启动计数为 0。
- [ ] 每客户端会话隔离，重复提交不多启动；取消/断连/恢复不自动计费。
- [ ] Keyring 撤销、权限变化、无 Token 时失败关闭，日志无正文和秘密。
- [ ] 单独批准少量真实 CLI 文本调用，核对内容与 Schema；再授权桌面 UI 验收。

本轮交付只含设计；上述清单未勾选，不影响已记录的 xcli 离线 PASS，也不能借其关闭应用 Gate C。
