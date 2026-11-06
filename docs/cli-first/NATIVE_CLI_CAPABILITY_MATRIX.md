# 原生 CLI 能力矩阵

2026-11-06；[本轮证据](evidence/preflight.json)、[预算记录](evidence/native-budget.json)、[报告](ACCEPTANCE_REPORT.md)。官方资料只说明可集成形态，见[来源与边界](OFFICIAL_SOURCES.md)。

**本轮获批 12 次，实际启动 9 次：文本 5、媒体 4。Codex、Antigravity 各四项通过；Cursor 一次文本启动因 workspace trust BLOCKED，其余三项未执行。失败未重试，剩余 3 次不再使用。**

[文本证据](evidence/native-20261106/text.json)、[媒体与视觉复核](evidence/native-20261106/media.json)。 PASS/FAIL/BLOCKED/NOT_SUPPORTED/NOT_TESTED 的含义见报告；`declared/help` 不等于 runtime。

## 本轮

| 能力/检查 | Codex | Cursor | Antigravity |
| --- | --- | --- | --- |
| 已安装版本 | PASS：0.167.0 | PASS：2026.10.22-27dcf5f | PASS：1.5.2 |
| xcli doctor | PASS：帮助探测 | PASS：帮助探测 | PASS：帮助探测 |
| 原有登录状态只读检查 | PASS：报告 ChatGPT 登录 | PASS：宿主 status 已认证 | NOT_TESTED |
| 原有登录能实际推理 | PASS | BLOCKED：trust 前置，非账号失效证明 | PASS |
| headless 文本/独立标记 | PASS：10.600s | BLOCKED：0.623s，exit 1 | PASS：57.725s |
| 会话恢复/上下文标记 | PASS：5.747s，同 ID | NOT_TESTED：前置 BLOCKED | PASS：66.153s，同 ID |
| 原生结构化事件与终止结果 | PASS：JSONL/terminal | BLOCKED：未获得结果 | PASS：init/step_update/result |
| 图片输入理解（Vision） | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 真正文生图、有效字节 | PASS：PNG 1254×1254 | BLOCKED：未启动，trust 前置 | PASS：JPEG 1024×1024 |
| 参考图编辑、原图未覆盖/视觉变化 | PASS：原图未变，花盆改蓝 | BLOCKED：未启动，trust 前置 | PASS：原图未变，花盆改蓝 |
| 原生文本增量/首 token 延迟 | NOT_TESTED | NOT_TESTED | NOT_TESTED |
| 工具实际调用及权限执行 | PASS：本次文件读取/媒体工具；非完整权限审计 | BLOCKED | PASS：本次读取/媒体子代理；内部不透明 |
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

先前帮助耗时仅是帮助启动。本轮媒体观察器的 first_stdout_seconds 是首个协议事件（Codex 约 0.06–0.08s，Antigravity 约 3.0–3.3s），不是首 token；实际任务耗时含模型/工具/交付开销，样本不足以进行性能排名。

Codex 外层事件未直接暴露嵌套 image_gen 工具，已用本次隔离会话的工具标识补证；Antigravity 只观察到父级 invoke_subagent，不宣称内部调用次数。最初观察器误判记录和离线复核均保留；没有重新生成图片。图片输入仅在本次参考编辑中用到，独立 Vision 语义能力仍 NOT_TESTED。

## 历史证据（不计入本轮）

[2026-10-15 媒体验收](../r0/media-acceptance.zh-CN.md)记录三个 CLI 的生成与编辑有效文件，并有实际工具与视觉检查：

| CLI/历史版本 | 生图记录 | 编辑记录 | 适用边界 |
| --- | --- | --- | --- |
| Codex 0.162.1 | PNG 1254×1254 | PNG 1254×1254，花盆改蓝 | 历史使用的 CLI 登录环境与本轮不同；不能切换回旧账号复用证据 |
| Cursor 2026.10.14 | 原生 JPEG 1024×1024 | 有效编辑图，历史另作格式转换 | 历史含 workspace trust/自动审查条件，本轮不修改 trust 或套用旧权限 |
| Antigravity 历史环境 | 经内置 image-generator 生成 JPEG 1024×1024 | 参考编辑通过，曾需定向授权恢复交付 | 当前版本、账号、工具审批均需重验；原生 SUCCESS 不足以证明文件交付 |

历史 Antigravity 视频/音乐模型生成缺接口并被用户暂缓，FFmpeg 基础编辑不能替代模型能力；本轮不扩大这两项范围。历史 Task/Run 与应用假上游记录也只适用于原报告边界。

下一步：先由本人决定 Cursor 临时项目的最小 trust 授权，并另立新预算；再按需求单独批准 Vision/Schema/取消/并发。当前不自动重试任何已失败场景。每一类分别更新证据、模型/版本、时间及状态，不批量把未知项提升为 PASS。
