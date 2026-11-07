# 真实 HTTP → Native CLI 验收计划（未授权、未执行）

2026-11-07。**本文件不是执行授权。当前真实启动/dispatch 预算为 0；前轮剩余 3 次不转用。** 不自动调用真实 CLI 的 version/auth/models/推理，不购买 credits，不变更账号、Trust、沙箱或工具批准，不换供应商补测。

## 当前前置阻断

三家适配代码和假进程协议路径均已测试，但没有启用的真实 profile。`Profile.Check` 对 native 逐次策略无法证明时失败关闭，没有 `--unsafe` 或“只改 YAML 就强开”的入口。仅追加调用预算或 AllowRemote 不会启用推理。

真实执行前必须完成以下具体工作，不能只向用户要一个笼统的“继续”：

1. 对本人选择的一个 CLI/精确二进制/安装域，确认账号适用条款、正式程序集成路径和版本。公开 headless 文档不等于本机版本/订阅/个人数据用途已获许可；不读取登录 Token、Cookie、Keychain 或原生历史。
2. 实现并离线审查可执行的文件读/写、shell/网络、MCP/hooks/plugins 逐次边界。认证由可信官方进程自行处理，不能靠 prompt、只读工作目录或清空 HOME 假称隔离已成立；当前最小环境与合成 HOME 不足以证明真实认证/工具隔离 parity。
3. 对合成 canary 安排 L04 原生小额验证；真实业务材料仍不开放。必要的验收入口必须由本人授权、固定材料与预算、保持普通应用 profile 关闭。当前无此强开入口；若需添加合成验收专用通道，先记录 API/控制面 bootstrap 的最小调整并评审，不把 unverified profile 假称 available。
4. L04 与正式传输确认后，登记精确策略/适配/能力 revision 和证据，再准备本人专用 Token、全局+客户端+Session 的固定累计预算。Cursor 的隔离项目 Trust 先由本人通过官方流程审查；Gateway 不加 trust/force/yolo 或更改全局配置。
5. 媒体另立 profile 与授权：写入仅任务输出，原始输入不可改；必须有精确任务输出目录/官方文件句柄及来源观察路径。只在供应商全局目录产图且不能安全关联时，保持 artifact_access_blocked，不能扫描私人目录或复制历史图充数。

## 每 CLI 的最小预算与权限

以下是**计划中的新授权槽位**。单次 exec/print 适配器每 turn 预扣 1 dispatch + 1 process_start，失败不退；内部模型/重连/工具次数及费用不可据此计算。

| CLI / 固定入口 | 首个 HTTP 文本闭环最小预算 | 权限前提与当前状态 |
| --- | --- | --- |
| Codex / codex-exec-v1 | L01：2 dispatch / 2 启动 | 先证明当前版本逐次工具/读/写/网络边界、已有官方认证在服务工作区可行；BLOCKED |
| Antigravity / antigravity-print-v1 | L01：2 / 2 | 同上，并核对 headless auto-deny、全局插件继承、conversation 与用量范围；BLOCKED |
| Cursor / cursor-headless-v1 | L01：2 / 2 | 同上；另需本人审查隔离 workspace Trust，不改现有 Trust；历史 trust 阻断；BLOCKED |

完整最低链路的已知槽位：L01 文本新建/续接 2、L02 生成/编辑 2、L03 取消/超时各 1，共 **6**。L04 每个独立边界另需至少 1；当前建议拆为越界读、越界写、shell、网络、继承工具五个合成场景，即再预留 **5**，每 CLI 计划合计 **11**。这些不是默认授权或精确云请求数。

L04 某场景若在启动/首个拒绝前即受阻，不能把未实际尝试的其他边界算 PASS，也不挪其余额。若工具行为要求更细拆分，先提交新预算计划。L05 的“同 Session busy、SSE 重连、恢复不重发”应零新增 turn；独立并发真实推理另需至少 2 个新槽，V1 全局单并发下不默认安排。

## 推荐执行顺序与停止条件

首先只选择一家，完成合成策略前置工程与 L04 新授权，再提出 **仅 L01 的 2 次** HTTP 计划。不是一次批准三家 33 次。每个计划必须附确切 binary hash/version、适配/策略/profile/capability 摘要、工作区与材料哈希、remote 同意、timeout、全局/客户端/Session 上限和授权引用。

| 场景 | 具体动作 | 成功事实 |
| --- | --- | --- |
| L01-a | 创建逻辑 Session（0 次）；HTTP 单 turn 读取合成随机标记 | Task succeeded、完整 terminal+exit、原生 ID 实际观察、预算 1；事件/最终引用可恢复 |
| L01-b | 同 Session 新 Task；不重供/重读该标记 | 相同实际 native ID、正确独立标记；不能用 runtime fallback 补证 |
| L02-a | 新媒体 Session；一次官方原生单图生成 | 任务绑定真实字节、安全导入/完整解码/MIME/尺寸/hash、来源痕迹、本人视觉接收、认证 ready 下载 |
| L02-b | 认证下载核对后上传唯一参考，一次编辑 | 参考 hash 不变、不覆盖输入，修改目标视觉通过；不自动重生成或代码绘图 |
| L03-a | 收到真实活动后主动 cancel | 本地停止事实与云撤销各自记录，晚图不入库；unknown/reconcile 不伪装取消已确认 |
| L03-b | 独立任务短 deadline | deadline+5 秒清理内收敛，或准确隔离未知 worker；不补发 |
| L04 | 每项合成权限 canary | 工具实际不可越界；仅 argv/工具 catalog/模式名称不能算 PASS |
| L05 | 重连/进程崩溃与恢复 | 原任务可查、同键不多次 dispatch、不按历史 PID kill；未知封存，无自动继续 |

遇到信任、认证、额度、支付、工具拒绝、来源/路径不可验证或版本漂移，即停止该场景；保留脱敏账本及失败，不换 CLI/模型/账号，不放宽权限，不复用未授权槽位。不确定本地/上游副作用时暂停对应安装域/状态根的新执行，先本人核实。

## 证据与后续判定

每次记录：新授权引用、UTC 起止、平台、xcli/CLI/adapter/schema/policy/profile 摘要、Session 实际观察来源、invocation ID、预扣次数/墙钟预留、退出/terminal/parse/denial、云撤销可观测性、nullable usage、事件 seq/幂等重放、产物与复核摘要。日志不保存原始 prompt/stdout/stderr/工具载荷或凭据，不自动读取历史原生会话补证。

至少一家官方路径完整通过 L01–L04、L05 的单并发拒绝/重连/恢复后，才可声明对应 NAG capability 通过。其他 CLI 单项保持 BLOCKED/NOT_TESTED。原 Gate B（native CLI→Model Gateway `/v1/*`）仍 BLOCKED；四应用 G6/UI/生产部署另立授权与工程范围。
