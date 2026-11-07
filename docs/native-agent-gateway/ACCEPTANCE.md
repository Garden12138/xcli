# Native Agent Gateway V1 — Acceptance

原始门禁定义：2026-11-06。2026-11-07 已完成 N 类的离线/假进程范围验收，详见 [IMPLEMENTATION_REPORT.md](IMPLEMENTATION_REPORT.md) 和 [IMPLEMENTATION_PROGRESS.md](IMPLEMENTATION_PROGRESS.md)。L/A/BHOST 未执行；真实 NAG 仍 BLOCKED。下方历史基线表描述设计交付当时的事实，不作为当前实现状态。

## 证据分层与当前状态

| 层 | 当前事实 | 本 SPEC 的判定 |
| --- | --- | --- |
| 本地 native CLI | 2026-11-06：Codex/Agy 各四项 PASS，Cursor trust 阻断；共 9 次 | 继承为能力选型基线，不重跑 |
| 原 HTTP Model Gateway | HTTP 假上游、协议与应用历史离线证据 | 不证明原生 Agent 服务 |
| 原 Gate B：native CLI→`/v1/*` | 缺 backend | **BLOCKED，不改名、不变 PASS** |
| NAG：`/agent/v1`→CLI | 本轮只设计，无产品源码 | NOT_IMPLEMENTED / BLOCKED |
| Gate C：四应用 native 接入 | 仅既有 HTTP 接入及 Agent 草案 | Planned / BLOCKED |
| 新真实模型/应用/UI/部署 | 本轮 0 | 未执行 |

状态定义：PASS=该具体场景有执行与结果证据；FAIL=已执行未满足；BLOCKED=权限/网络/实现/额度前置受阻；NOT_TESTED=未执行；NOT_SUPPORTED=正式接口或明确运行结果证实不支持。安装、help、事件 JSON、exit 0、模型目录均不能单独视作任务成功。

## D 类：本轮设计交付

| ID | 检查 | 本轮结果 |
| --- | --- | --- |
| D01 | 五份指定文档、核心源码与官方 SDK/headless/app-server 对照；七份 SPEC 齐全、链接存在 | 已完成文档/源码审查；交付检查见文末 |
| D02 | 未提交验收修改与摘要、账本/图像一致，Smoke 修复有离线回归 | 11/11 通过；源清单、证据、5 文本+4 媒体、四图解码通过 |
| D03 | 独立 Git 提交、设计与原证据可区分、无 Push | `864a6b1` 为验收独立提交；SPEC 另作设计变更 |
| D04 | 无新 native 调用/凭据读取/四应用写入/部署，Gate B 未变 PASS | 本轮遵守；预算剩余 3 次不使用 |

D 类检查使用静态源码/JSON/Markdown/已有图片与 mock 测试，不启动已安装的 native 程序。当前无需重跑全部 Go test/race：没有 Go 产品代码变更；历史通过与本轮检查分开记录。

## N 类：离线实现验收与需求追踪（未来）

| ID / 需求 | 方法、夹具/故障注入 | 明确通过标准 |
| --- | --- | --- |
| N01 / R01,R13 | 独立 Agent server 与旧 Model server；未知参数/路由、旧配置对照 | `/agent/v1` 只在新服务；`/v1` 无 native 路由；native-cli 配置仍拒绝；不自动 fallback |
| N02 / R03,R11 | client A/B、伪造 Session/Task/Input/Artifact、轮换撤销、Origin/Host/代理请求 | 跨用户均 404；未认证 401；拒绝云/Origin/错误 scope 先于材料导出/dispatch；Token 不入 argv/env/log |
| N03 / R03,R14 | 逻辑创建、首次绑定、同 ID、缺 ID、错 ID、未知 Session/版本漂移 | 创建 0 启动；续接仅同 owner/profile；fallback ID 不证明 observed；丢失不建新会话冒充续接 |
| N04 / R04,R13 | 缺 executable、旧帮助、认证未知、不同 binary/schema、Cursor trust | GET 0 native 启动；文档/native/Gateway 证据分开；阻断原因准确；未知不标 available |
| N05 / R02,R05 | 各厂商 split/unknown/overlong/截断 JSONL、无 terminal、exit 非零、Agy soft-deny、Codex quota/reconnect | 每个状态可判定；信封坏与运营拒绝分开；拒绝后有效文本/图片仍不能 succeed；输出超限失败 |
| N06 / R06,R14 | SSE 断开、重连、重复/乱序、游标过期、慢客户端、journal 写失败 | 不新增 dispatch；seq 去重、terminal 一次且持久；journal 失败中止；终止事实不丢；无假 delta |
| N07 / R07 | 伪图、错误 MIME、畸形/解码炸弹、size/pixel/数量超限；真实 PNG/JPEG/WebP | 完整解码/格式/尺寸/hash 一致才可预览；保留真实格式；URL/文案/占位图不能成功 |
| N08 / R08 | fake worker 派生子进程、忽略 TERM、持有 pipe；queued/running/validating/review 中 cancel；deadline race | 取消幂等；全组停止或 needs_reconcile；无无限 wait；late success/文件不翻终态；不会撤销别人进程 |
| N09 / R08,R10,R14 | 同 key 并发 100 次、budget 临界值、不同 Session 竞争、队列满/同 Session busy | 只创建/dispatch 1 次；启动前持久扣减；最多授权上限；失败不返次数；排队取消零调用 |
| N10 / R09,R11 | 合成 canary：输入外文本、其他工作区、伪凭据目录；只记录访问事件；全局工具/注入 env/越权参数 | 无真实凭据测试；禁止访问 canary、shell/网络/写入越界；不能约束则 policy_unenforceable，0 真启动 |
| N11 / R07,R09 | symlink 每层/硬链接、目录替换竞态、FD 后源变更、旧文件/别的 Task、原图覆盖 | 仅任务绑定普通文件可导入；副本不可变；不能扫描原生根；输入哈希始终不变；下载不接本机路径 |
| N12 / R07,R12 | 初始 tool catalog、隐藏子代理、有效图无来源、视觉拒绝、缺 attest scope、超时后晚图、重复 review | pending 不等于 ready；人工不能覆盖字节/权限/账单；只记录所知来源；不调用模型修复 |
| N13 / R10 | usage 缺字段/累计/缓存、余额未知、额外支付提示、硬 max_cost_usd、服务日预算 0 | 缺失为 null、累计不重复相加；硬金额要求拒绝；不购额度/换账号；本地上限可强制、内部模型次数不伪造 |
| N14 / R14 | 在 accepted/dispatch commit/spawn/terminal/fsync/checkpoint 等边界崩溃；损坏日志；磁盘满；进程重用 | 只重放状态不重放执行；已扣不退；未知封存 Session；无 PID 误杀；损坏失败关闭；服务单写者锁 |
| N15 / R15,R01 | 原 run/resume/task/jobs、Model HTTP Chat/Images/Embeddings、跨平台构建的旧确定性回归 | 原合同/Token/配置无漂移；不动旧数据；交叉构建不写作实机通过 |

N10 fake 验证只证明 service 拒绝逻辑；CLI 的真实沙箱/工具执行仍须 L04 原生小额实测，不能仅靠 argv 断言安全。N07 历史图片只能测字节校验，不产生新能力证据。

未来离线命令应采用可写临时 Go cache、GOPROXY=off/GOSUMDB=off，并明确配置只调用 fake Driver。新增包完成后运行对应 `go test -race`、`go vet`、`go build`，再跑旧确定性回归。HTTP 测试需本机 loopback 权限时只申请精确离线命令；不能把失败的沙箱 setup 记作 PASS，也不能借机启动模型。

## L 类：新的真实 HTTP→CLI 验收（未授权、未执行）

每个场景先写执行计划：Agent/适配器/版本/摘要、精确策略与材料、预计新进程/turn 数、timeout、终止条件、产物/视觉要求和预算。只对用户另行批准的场景启动，不自动补测、换 CLI 或失败重试。每次失败和非模型探测都进相应账本；原生内部请求数不可观测时如实记录。

| ID | 真实场景 | 通过标准/预算建议（不是授权） |
| --- | --- | --- |
| L01 | HTTP 文本新 Task + 同逻辑 Session 续接 | 建 Session 0；2 次原生 dispatch/单次适配器 2 进程；独立随机标记，续接不重供/读标记，原生 ID 实际相同；无 fallback 补证 |
| L02 | HTTP 图片生成 + 对其真实原图编辑 | 2 次；每次 1 图，不重生成；官方工具来源/合法任务输出根/真实字节/视觉/原图不变；完整 ready 下载后 hash 一致 |
| L03 | 主动取消与短 deadline | 通常各 1 次；启动后真实收到活动再取消，确认本地停止、描述云撤销可观测性，晚图不入库；不把本地 kill 当云撤销 |
| L04 | 最小策略与原生权限执行 | 合成 canary/无真实秘密；用户明确所需额外调用槽；禁止越界读写、未批准工具/网络/全局插件；做不到则 profile 不启用 |
| L05 | 并发/重连/服务重启 | 同 Session 冲突零新增 dispatch；独立并发至少需 2 次另批预算；断线/重启后零自动新调用，未知需 reconcile |

建议首先批准一个 CLI 的文本小闭环，再决定下一组；不得将表中建议相加为已获授权。Cursor 先停在 workspace trust；由本人通过官方方式审核隔离项目，服务不能写 trust 或调用 force 来填 PASS。某 CLI 未通过时精确标记该版本/operation，其他 CLI 成功不能代替。

NAG 最低完成条件：所有必需 N 项通过，至少一个启用的官方适配器完整通过 L01–L04，且单并发政策下 L05 的拒绝/重连/重启部分成立；要宣称并发推理或三个 CLI 全支持，须各自补齐真实 Gate。关闭的 capability 仍必须返回明确错误，不将整产品标签写作所有 CLI 能力均通过。

## A 类：四应用适配验收（后续独立授权）

| ID | 合成测试与真实/UI 分层 | 通过标准 |
| --- | --- | --- |
| A01 Worklog | fake 日/周报、模板、坏 JSON；之后另批合成真实任务和桌面流程 | 最小选材、scope 同意、local schema/日期/引用校验；仅最终草稿显式保存，取消/断线不重做 |
| A02 GardenFlow | fake review/下载/媒体事务/重启；之后另批真实生成编辑及 UI | ready 后校验字节/摘要、本人确认、一次入库；引用为应用媒体 ID；原图不覆盖/晚到不自动入库 |
| A03 Chat Insight | 合成选段、拒云、错误能力、SSE 中断；Embedding 缺失与旧索引保护 | 拒绝前不读真实消息/导出；仅选段对话；不读取 DB 路径给 Agent、不调用原生 Embedding、不混改索引 |
| A04 Contribution OS | 合成只读 Plan/Review、冻结 Profile、坏 JSON/身份变化/发布指令 | 本地业务 Schema/语义通过才消费；无自动执行/merge/push；Docker Agent 路径明确拒绝 |
| BHOST 独立桥接 | 另立宿主/容器认证、重放、实例/证书/Token/预算/Artifact/SSRF/取消测试 | 失败关闭，不挂登录目录或全盘，不沿用旧 HTTP 证据关闭 native Gate；部署另批 |

每个应用还必须证明：独立后台安全存储 Token、无渲染层泄漏、未认证/云拒绝/能力不足时 0 dispatch、幂等/事件恢复无重复收费、原 HTTP 模式回归通过。应用本地业务验证不等于 native strict Schema，原生文本成功不等于 Embedding/RAG 可用。

## 记录模板与发布门槛

每项记录至少包含：测试 ID、实现 commit、平台、CLI/adapter/schema/profile 摘要、真实/假上游标志、开始结束 UTC、授权引用、本地启动/dispatch 数、退出/协议/状态/错误、Session 观察来源、usage 范围、结果/产物哈希、人工复核证据及结论。仅保留脱敏元数据，不上传原始聊天/工具载荷或凭据。

实现、离线通过、真实 Agent 链路、应用接入、生产发布各自有状态；只有实际完成的项目可改 PASS。未满足真实条件时允许交付代码和 BLOCKED 报告，不能改 Gate B 或模糊宣称完成。

本轮交付复核：7 份必需 SPEC + 1 份索引，连同 2 份交接文档共检查 35 个相对链接、6 个 JSON 示例全部通过；R01–R15 均映射到未来 N 类验收。`git diff --check` 通过，设计阶段只改文档。未创建/运行上述新服务或测试包，未增加 native 启动；旧验收 evidence 不改写为本轮新测试。
