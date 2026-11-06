# 原生 CLI 能力矩阵

2026-11-06；[本轮证据](evidence/preflight.json)、[预算记录](evidence/native-budget.json)、[报告](ACCEPTANCE_REPORT.md)。官方资料只说明可集成形态，见[来源与边界](OFFICIAL_SOURCES.md)。

**本轮真实模型启动 0 次。文本和媒体均未获明确数额回复，以下 NOT_TESTED 不表示产品不支持。** PASS/FAIL/BLOCKED/NOT_TESTED 的含义见报告；`declared/help` 不等于 runtime。

## 本轮

| 能力/检查 | Codex | Cursor | Antigravity |
| --- | --- | --- | --- |
| 已安装版本 | PASS：0.167.0 | PASS：2026.10.22-27dcf5f | PASS：1.5.2 |
| xcli doctor | PASS：帮助探测 | PASS：帮助探测 | PASS：帮助探测 |
| 原有登录状态只读检查 | PASS：报告 ChatGPT 登录 | PASS：宿主 status 已认证 | NOT_TESTED |
| 原有登录能实际推理 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| headless 文本/独立标记 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 会话恢复/上下文标记 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 原生结构化事件与终止结果 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 图片输入理解（Vision） | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 真正文生图、有效字节 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 参考图编辑、原图未覆盖/视觉变化 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 原生文本增量/首 token 延迟 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 工具实际调用及权限执行 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| JSON 对象内容 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 严格 JSON Schema 约束 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 原生运行中取消/超时 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 并发推理与会话隔离 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| CLI → 现有 Model Gateway | BLOCKED：无 backend | BLOCKED：无 backend | BLOCKED：无 backend |
| 四应用原生能力接入 | BLOCKED：仅设计 | BLOCKED：仅设计 | BLOCKED：仅设计 |

离线 Go 测试中协议解析、取消/超时、用量归一化及 Gateway HTTP/SSE 等通过；这些夹具测试与上表真实 CLI 推理分开。没有为了填满矩阵启动额外并发、Vision、Schema 或取消请求。

## 官方形态与源码映射

| 项目 | Codex | Cursor | Antigravity |
| --- | --- | --- | --- |
| 非交互入口 | `exec --json` | `--print --output-format stream-json` | `--print --output-format stream-json` |
| 原生会话恢复 | `exec resume ID` | `--resume ID` | `--conversation ID` |
| 更完整应用接口 | 官方 SDK/app-server | 官方 headless；其他接口需独立选型 | 官方 SDK；headless 可连续输入 |
| Schema 声明/帮助 | `--output-schema FILE` | 本轮帮助未确认严格 Schema 参数 | `--json-schema` |
| 原生流 | exec JSONL 主要是任务/项目事件；app-server 有消息 delta | 默认整段 assistant；partial 需显式开关并去重 | `step_update` 可含 text_delta，result 是终止事实 |
| 同进程连续输入 | 优先 SDK/app-server，当前 xcli 未暴露 | 本轮未核验 | 官方 `--input-format stream-json`；当前 xcli 未暴露 |
| 图片入口边界 | 帮助确认 `--image` 输入；输出另看工具与实际产物 | Agent 资料描述生图/参考图；CLI 是否可用须实测 | SDK 工具表含 generate_image；实际模型/子代理可用性另验 |
| 当前 xcli 暴露 | 归一化最终文本、会话、已上报用量和活动事件 | 同左；不能把默认 assistant 事件当 token 流 | 同左；需拒绝 denied_actions 假成功 |

JSONL/stream-json 是事件信封；生成的文本是否符合业务 Schema 是另一项检查。CLI 自己执行的工具也不等于可交给消费应用执行的 Chat `tool_calls`。不能从工具清单推导可调用、从帮助参数推导权限生效，或从模型目录推导账号额度可用。

本轮记录的帮助调用耗时见 preflight（约 0.01–0.4 秒量级），仅是帮助启动，不包含认证、网络、推理、媒体工具或交付，不能宣传为原生模型启动性能。

## 历史证据（不计入本轮）

[2026-10-15 媒体验收](../r0/media-acceptance.zh-CN.md)记录三个 CLI 的生成与编辑有效文件，并有实际工具与视觉检查：

| CLI/历史版本 | 生图记录 | 编辑记录 | 适用边界 |
| --- | --- | --- | --- |
| Codex 0.162.1 | PNG 1254×1254 | PNG 1254×1254，花盆改蓝 | 历史使用的 CLI 登录环境与本轮不同；不能切换回旧账号复用证据 |
| Cursor 2026.10.14 | 原生 JPEG 1024×1024 | 有效编辑图，历史另作格式转换 | 历史含 workspace trust/自动审查条件，本轮不修改 trust 或套用旧权限 |
| Antigravity 历史环境 | 经内置 image-generator 生成 JPEG 1024×1024 | 参考编辑通过，曾需定向授权恢复交付 | 当前版本、账号、工具审批均需重验；原生 SUCCESS 不足以证明文件交付 |

历史 Antigravity 视频/音乐模型生成缺接口并被用户暂缓，FFmpeg 基础编辑不能替代模型能力；本轮不扩大这两项范围。历史 Task/Run 与应用假上游记录也只适用于原报告边界。

补测顺序：批准的文本新会话/恢复 → 额外批准的图片生成/参考编辑 → 按实际缺口另申请 Vision/Schema/取消/并发预算。每一类分别更新证据、模型/版本、时间及状态，不批量把未知项提升为 PASS。
