# Native Agent Gateway V1 实施验收报告

G5A.1 交付：[Codex 受限控制工程](CODEX_LIMITED_PROFILE.md)、[首次 HTTP 脚本](CODEX_FIRST_REAL_HTTP_TEST.md)、[最终离线日志](evidence/g5a1/offline.log)。控制工程与非模型沙箱 canary 有实际证据，全工具／认证隔离仍为工程 BLOCKED；没有标记 READY_FOR_AUTHORIZATION 或真实 Gate PASS。

2026-11-08 G5A 补充见 [Profile 就绪审查](G5A_PROFILE_READINESS.md)、[官方集成对照](OFFICIAL_INTEGRATION_REVIEW.md)及 [G5B 首次文本计划](G5B_REAL_HTTP_PLAN.md)。下文保留 G1–G4 历史证据；本轮没有将真实 Gate 升为 PASS。

日期：2026-11-07。基线 `40fc6ca` / 前序真实证据 `864a6b1`，本轮产品代码 `8526361`。**已完成本轮获授权的实现与离线交付；真实 NAG 最低验收仍 BLOCKED，不能宣称三个 CLI 已在 HTTP 服务可用。** 新真实 native 启动/模型请求 0，没有沿用前轮 3 次余额，没有 Push/Tag/Release/部署或四应用改动。

## 实现范围

独立 `agent-gateway` 命令、配置、literal loopback listener、Token namespace 与状态根；实现合同全部 12 个业务路由及 health。单 turn 调度不调用旧 Task Runner，也不实现 model.ChatBackend。原 `/v1/*` 与 native-cli backend 拒绝语义保持。

客户端 scope/Agent/operation/policy/云权限、Session owner/CAS/冻结 revision、输入哈希与累计 consent、幂等与预算均由服务检查。dispatch 次数/启动次数和墙钟预留在任何 spawn 前持久化，失败不退次数，无重试/fallback/账号切换。全服务并发 1，因此安装域和 Session 也最多 1；有限队列 16（可设 1–128），最长排队 30 秒，task deadline 1–600 秒并含排队/验证，清理额外最多 5 秒。墙钟计费记录采用单调耗时，期限同时受持久 UTC 和进程内单调时限约束。

存储为私有单写者、带头/正文校验与 commit trailer 的 fsync journal；watermark 约束已确认序号/实例/摘要，原子压缩保持序号递增和当前预算/幂等/事件，checkpoint 只作缓存。不确定写入 poison 并中止 HTTP 响应，按原键恢复；不谎报 accepted:false。重启仅恢复状态：未 dispatch 封存 service_restarted，已 dispatch 未完成 needs_reconcile/隔离，既不补发也不按历史 PID kill。

三家固定官方 exec/print argv、typed session/terminal/diagnostic/nullable usage/最终文本映射已实现。未知原生事件不能制造 Gateway 会话、最终文本或用量；旧 decoder/RunResult/CLI fallback 不变。活动可聚合（每类最多每秒一次），无虚构 text.delta；SSE 是持久 journal 的观察者，断开不会停止任务。取消本地与云端撤销分开记录。

图片仅上传原始字节，PNG/JPEG/WebP 完整解码/格式/尺寸/hash；目录 FD、每层 no-follow、普通文件/nlink/复制前后身份、私有副本发布，下载不读 native 路径。参考 blob 与工作区只读副本都校验，输入不可覆盖；权限/路径/账单阻断不能由 review 覆盖。review_only → 来源证明 + 语义接收 → ready 的协议在假客户端通过。当前 exec/print 适配器不能证明文件绑定的图片工具调用，外层工具名/subagent 只保留为未验证观测，图源需本人 trace attestation；未启用 native_event 自动图源通过。

## 逐项离线证据

以下 PASS **仅指离线/假进程范围**；N10 的真实沙箱执行、L01–L05、应用 A01–A04/BHOST 都没有借此升级。

| 验收 | 本轮实际覆盖 | 结果 |
| --- | --- | --- |
| N01 | 新旧服务/路由/Token/根独立；旧 native-cli 仍拒；未知字段/Unicode/重复 key/尾随 JSON/模式专用 budget 字段拒绝 | PASS |
| N02 | A/B 的 Session/Task/Input/Artifact 归属，scope/轮换/撤销 epoch，Host/Origin/代理头，argv/env 无应用凭据 | PASS |
| N03 | 零启动建 Session，CAS/context/profile 固定；真实协议假进程同 ID、缺 ID、错 ID；无 fallback 补证 | PASS |
| N04 | GET 缓存零探测；显式 fake --version；观测到二进制漂移时零 dispatch 并退回 blocked；缺云权限/隔离可见 | PASS（真实账号/安装未验） |
| N05 | split/unknown/损坏/超长/无终止，三家假 CLI，Agy soft-deny、Cursor trust、Codex quota/reconnect，1 MiB 最终文本上限 | PASS |
| N06 | SSE 断连不取消，Last-Event-ID 重放/非法/过窗，连续最近 10,000 条与唯一 terminal，慢读断开、journal 失败停止 | PASS |
| N07 | PNG/JPEG/WebP、伪 MIME/文本、截断与像素炸弹；原始格式/hash/ETag 保持 | PASS（夹具字节） |
| N08 | queued/running/review 取消、deadline 优先、忽略 TERM/后代 pipe、有界收尾、未知停止隔离、取消后晚图不发布 | PASS（假进程；真实云撤销未验） |
| N09 | 100 并发 service 与真实 HTTP 同键各一个 Task；不同 Session 预算 race、busy/queue_full、预扣与取消零调用 | PASS |
| N10 | 未授权/不可执行 native profile 先拒绝；最小环境不继承 Key/代理/loader/应用 Token；没有强开参数 | PASS（拒绝面；真实 canary 为 BLOCKED） |
| N11 | 任务绑定安全根、每层 symlink/硬链接/替换竞态、跨 Session/Task 引用、输入覆盖与内容哈希失败关闭 | PASS（macOS；Windows 读取关闭） |
| N12 | 无来源仅预览、缺 attest 拒绝、来源/视觉分别记录、reject/expire/cancel 隔离、重复 review 无新调用 | PASS（假客户端） |
| N13 | 字段缺失为 null、真实零值保留、不从累计/启动数推费用、硬金额字段一律拒绝、零默认日预算 | PASS |
| N14 | 实际测试进程在 accepted/dispatch-write/spawn/terminal-write/checkpoint 崩溃；fsync ENOSPC、损坏/截断/压缩/单写者/状态探测竞争；恢复零重发 | PASS |
| N15 | 完整 Go race/vet/build，旧 Agent/Runtime/Task/run/resume/jobs/Model Chat/Images/Embeddings、11 项旧 Python harness、交叉构建 | PASS（交叉构建不是实机） |

主入口：`bash tests/native_agent_gateway/verify-offline.sh`。主要新增测试位于 `internal/agentapi`、`agentstore`、`agentservice`、`agentgateway`、`agentartifact`、`nativeadapter` 和 `internal/runtime/bounded_test.go`。真实 HTTP→假 CLI 可单独运行 `TestLoopbackHTTPToCLIToPersistedResult`；GardenFlow 风格模拟接收为 `TestGardenFlowStyleHTTPPreviewReviewDownload`；真实进程崩溃为 `TestActualProcessCrashNeverRedispatches`。

[最终完整离线日志](evidence/g4-offline.log)、[边界补充](evidence/g4-boundaries.log)、[G4 事件与崩溃](evidence/g4-events-crash.log)、[存储故障](evidence/g4-store.log)、[基准](evidence/g4-benchmark.log)、[构建与文档校验](evidence/g4-builds.log)。代码摘要见 [source-manifest.json](evidence/source-manifest.json)。G1–G3 的失败与通过日志一并保留，没有删除测试或放宽安全条件。

## 性能和容量的实际边界

基线使用 macOS arm64 / Apple M4 Pro / Go 1.26.4，单个 1 KiB 假任务与进程内 HTTP handler；缓存目录约 24.5 μs/op，持久假任务查询约 40.9 μs/op，见基准原始日志。它衡量缓存目录/持久任务查询，不是原生推理吞吐、网络负载或生产 SLA。真实供应商延迟/费用/并发均 NOT_TESTED。

普通 HTTP 并发上限 8，大 body/内容交付 2，SSE 观察者 8；慢 SSE 写入期限 1 秒，普通连接有读写期限。事件窗最多 10,000，固定活动载荷使当前类型低于 8 MiB；公开事件不超过 64 KiB。状态帧最多 32 MiB，journal 最多 256 MiB，16 MiB 后自动压缩；服务 blob 共 256 MiB。容量或磁盘异常失败关闭，不自动删预算/幂等来扩大可用性。

输入/工作区按 Session 的 24h idle/7d 绝对上限清理；结果/事件/产物 7d、幂等 30d，过期正文从 journal 和缓存实际移除，已确认停止的工作区可删。无法确认活 worker 时保留隔离工作区/必要审计，不用删除目录冒充取消，需本人处理。不能据此承诺供应商/系统备份/云端历史被删除，也未证明 native 工具磁盘硬限额。

## Gate 与未验收项

| Gate | 本轮判定 | 证据/限制 |
| --- | --- | --- |
| NAG 离线实现 | PASS（上述离线范围） | 真 HTTP→原生协议假进程、真实文件字节、故障/安全/兼容 |
| NAG 真实最低 Gate | **BLOCKED** | 无新授权；三家逐次权限/账号条款适用性/安全输出根未确认，所有 native profile 关闭；L01–L05 NOT_TESTED |
| 原 Gate A | 仅继承历史结果 | Codex/Agy 各文本/续接/生成/编辑历史 PASS，Cursor trust BLOCKED；[原报告](../cli-first/ACCEPTANCE_REPORT.md)，本轮无新证明 |
| 原 Gate B | **BLOCKED** | native CLI 不接 Model Gateway `/v1/*`；拒绝测试仍通过，不能用 NAG 改名关闭 |
| 原 Gate C / G6 | Planned / BLOCKED | 四应用未修改、无 native UI 联调；Docker 桥接未开放 |
| G5 app-server 晋升 | 未实施 | 仅静态请求关联/deny-only/interrupt 评估；无本机 schema/权限 parity，未替代 exec |

[支持矩阵](SUPPORT_MATRIX.md)、[API 与安装/调用手册](USER_GUIDE.md)、[真实新授权计划与每 CLI 预算](REAL_HTTP_ACCEPTANCE_PLAN.md)、[四应用模块入口与 APP-NAG-1](MIGRATION_AND_COMPATIBILITY.md)。当前没有受支持的“一键启用真实 CLI”配置；需要先完成可执行隔离工程和获批合成验收，不能用预算或普通模型 API绕过。
