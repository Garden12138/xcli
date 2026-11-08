# Native Agent Gateway V1 — 实施与恢复

开始日期：2026-11-07；基线 `40fc6ca`（前序真实验收 `864a6b1`）。本轮获授权：本仓库实现、离线/假 CLI 测试、独立本地提交。真实原生模型调用预算 **0**；不继承前轮 3 次余额；不 Push/Tag/Release/部署、不改四应用或原生认证/Trust。

## 当前入口

当前阶段：**G5A.2 非推理工程检查及最小修复完成；ENGINEERING_BLOCKED，用户确认轴 USER_CONFIRMATION_REQUIRED。** 起点 `3494756`，实现 SHA 见末尾交接。本轮真实推理 0；仅 Codex、合成 Canary／假凭据和非推理探针。恢复先读文末 G5A.2 记录及新隔离／用户确认文档。G1–G4、G5A.1 的控制、预算和旧证据不重做、不覆盖；三家真实 Profile 继续关闭。

最新恢复入口：[G5A 分项状态](G5A_PROFILE_READINESS.md) → [官方集成与授权](OFFICIAL_INTEGRATION_REVIEW.md) → [G5B 每家首次 1 次文本计划](G5B_REAL_HTTP_PLAN.md)。G5A 本地提交定位见本文件末尾；不得将审查完成解读为已具备直接推理条件。

恢复先读本文件，再读 `TASKS.md`、`ACCEPTANCE.md` 和 `git log --oneline -8` / `git status --short`。不得重新执行历史 native smoke。

## 已确认决策

- 已完整阅读 README 与七份 SPEC，审查 agent decoder/argv/diagnostics/usage、runtime/process/environment、runstore、task lifecycle、Model Gateway/server/client/private/service/config 与图片校验代码。
- 独立 `/agent/v1`、Token、配置、端口、状态根；不接 `model.ChatBackend`，不调用多轮 Task Runner。
- 所有真实 profile 在逐次权限和 Gateway 验收前关闭；缺官方可执行隔离证据时 `policy_unenforceable`。先做 HTTP → 受控假进程，可复现的离线证据与真实验收分开。
- append-only fsync 事务先于 dispatch，重启仅恢复状态；不按历史 PID kill，不自动 retry/fallback。
- 官方文档核查已记录，app-server 仅离线协议评估，G5 晋升另立范围。

## 阶段记录

| 阶段 | 状态 | 本地提交 | 测试/证据 |
| --- | --- | --- | --- |
| G0 | 已继承设计 | `40fc6ca` | 七份 SPEC + README |
| G1 | PASS（离线范围） | `86717e6` | [核心 race](evidence/g1-core-race.log)、[旧回归 race](evidence/g1-regression-race.log)、vet/build |
| G2 | PASS（离线范围，覆盖用户 B/C 的实现） | `25201e1` | [全量 race](evidence/g2-all-race.log)、[最终补充](evidence/g2-additions-race.log)、[CLI 控制](evidence/g2-control.json) |
| G3 | PASS（macOS 离线范围） | `01581bf` | [全量 race](evidence/g3-all-race.log)、[最终核心](evidence/g3-core-final.log) |
| G4 离线 | PASS | `8526361` | [完整离线](evidence/g4-offline.log)、[报告](IMPLEMENTATION_REPORT.md)、[源摘要](evidence/source-manifest.json) |
| G4 真实 / 用户 F | 计划交付；执行 BLOCKED | `8526361` | [真实计划/每 CLI 最小预算](REAL_HTTP_ACCEPTANCE_PLAN.md)，未执行 |
| G5A | PASS（审查／最小修复／非推理范围） | 见末尾提交定位 | [就绪审查与实际证据](G5A_PROFILE_READINESS.md)；三个真实 Profile 仍关闭 |
| G5A.1 | 控制工程／离线交付完成；全部启用 Gate BLOCKED | `e742590` | [受限工程与剩余验证器](CODEX_LIMITED_PROFILE.md)、[完整离线](evidence/g5a1/offline.log)；不标 READY |
| G5B / G5 transport / G6 | 后续独立范围 | — | 真实调用 / app-server、ACP 晋升 / 四应用 |

## 门禁

- 本轮真实模型启动：0；真实 HTTP→native CLI：NOT_TESTED。
- 原 Gate A：继承 Codex/Agy 的特定历史场景 PASS，Cursor workspace trust BLOCKED；不作为新 Gateway 证据。
- 原 Gate B（native CLI→Model Gateway）：**BLOCKED**；V1 无该兼容层。
- NAG 离线：PASS（仅离线范围）；NAG 真实：BLOCKED；四应用 Gate C：Planned/BLOCKED。

## 初始任务入口（历史，已完成）

G1：实现闭合 DTO、私有事务存储、客户端授权、Session/CAS、预算/幂等和 HTTP 安全检查，新增 fake-only contract tests。所有 Go 命令使用 `GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off`；只运行明确的离线测试，不运行 native smoke。

## G1 实际交付（2026-11-07）

- 独立 wire DTO、严格 UTF-8/JSON/重复 key/未知字段/必填校验；闭合路由与稳定脱敏错误。
- literal loopback + 固定 Host、拒 Origin/转发头/URL Token；独立随机 256-bit Token namespace 与摘要、scope/Agent/operation/policy/remote/预算授权；issue/rotate/revoke 库。
- 单写者带校验 fsync journal、缓存 checkpoint、尾部恢复、完整损坏拒绝、写失败 poison；持久实例 ID。
- Session 所有权/CAS/冻结 profile、幂等同键 100 并发仅一个 Task、启动前持久次数扣减/墙钟预留、无自动退款/重发；默认日预算 0。
- HTTP→fake Driver→持久结果/SSE 全链路；原生 ID 不公开；GET 零 native 探测；静态 app-server 请求关联/deny-only/interrupt 评估。
- 核心新增测试全部 race PASS；旧 agent/runtime/runstore/modelgateway/modelstore/task/cmd race PASS（Task 164.784s，CLI 46.172s）。vet 与 build 退出 0。build 有只读 Go module stat cache 警告，未影响产物生成。
- 首次新增测试构建有 unused import，已修复；随后沙箱禁止旧 Model Gateway 绑定端口，保留原始失败日志，在精确离线测试获批 loopback 权限后通过。没有删测试/放宽安全条件。
- 原生 profile 均关闭；G1 策略测试只证明拒绝、环境清洗，CLI 实际边界 L04 未验；[公开接口/条款与调整记录](OFFICIAL_PROTOCOL_REVIEW.md)。G1 不声称 NAG 真实 Gate PASS。

下一入口：G2 typed observer、受控原生 argv/最小环境、实际假子进程 HTTP 链路、续接/错 ID、撤销、主动取消、deadline 和后代 pipe；新真实模型调用仍为 0。

## G2 实际交付（2026-11-07）

- 可选 typed observer 保留本次 native ID/terminal/diagnostic/nullable usage，旧 Event/Normalize/fallback 合同保持；Gateway 不读取 fallback。
- Codex exec、Cursor headless、Agy print 的固定 argv/原生 ID 恢复和错误优先级；三者均用新建假进程验证，Cursor trust、Agy SUCCESS+soft deny、Codex reconnect/quota/截断/超长覆盖。三家真实 profile 仍关闭。
- 实际 HTTP Client → Gateway → 原生协议假 CLI 子进程 → 持久任务/结果/SSE；split 字节、同 ID、错/缺 ID、无终止均有测试；没有 final 切片 delta。
- 服务单并发、Session 串行、排队/期限、撤销 epoch（重新授权不能复活旧队列）、授权变更与 dispatch 持久事务锁；取消与上游撤销独立。原生 workspace/HOME 都是服务私有合成根，不复制认证资料。
- Gateway opt-in WaitDelay/有界清理，TERM 最多 2 秒、KILL/管道收尾受 5 秒硬上限；后代持有 pipe 无无限等待，无法确认停止则 needs_reconcile/全局隔离。旧 CLI 仍采用原有默认进程等待行为。
- `xcli agent-gateway serve/status/stop/client issue/rotate/revoke/list/refresh`；独立 `--gateway-config`。refresh 是本人显式且停服时的有界 `--version` 缓存，不能晋升 profile；本轮只用假版本程序测试，未探测已安装 CLI。
- 完整 `go test -race ./...` PASS；最终相关包/CLI 补充 PASS；vet/build 退出 0。CLI 服务两次实际启动/HTTP/stop/重启验证实例 ID 不变、generation 不同、native 拒绝。[证据目录](evidence/)。
- 失败修复如实保留：新增 token 测试最初用了公开临时目录，改成 0700 私有子目录；零退出等待暴露旧 Jobs 先写成功再写最终日志的竞态，调整发布顺序并连续 10 次 race 回归；status 与启动锁的竞争用单独 probe 锁串行，新增并发回归。未删测试或降权限。

下一入口：G3 原始字节上传、安全 FD 导入/格式解码/哈希/输入保护、待审→来源+语义复核→认证交付。图片夹具只证明离线数据流程，绝不作为新 native 图片成功证据。

## G3 实际交付（2026-11-07）

- 完成合同内的原始字节 `/inputs`、Artifact 元数据/认证内容、Task review；PNG/JPEG/WebP 真实格式、完整解码、像素/字节上限复用提取后的纯 `internal/media` 校验器，旧 Model error/响应语义保持。
- Input owner/Session/哈希绑定，Task 持久记录全部输入哈希及 consent 摘要；原始 blob 与工作区只读副本都复核。生成固定一图；编辑固定一个参考，结果单独输出。API 不接路径/URL/文件名。
- macOS/Linux 实现 openat + 每层 no-follow + 普通文件/nlink/FD 前后 identity/size/mtime/ctime 检查，再复制/校验/私有原子发布；只读服务副本下载。Windows 缺等价 handle 验收时媒体读取明确失败关闭，后续只做交叉构建。
- review_only 认证预览→已观测来源或本人额外 attest scope→语义接收→ready；来源与视觉复核各记摘要/审核者。新增可选响应字段 `semantic_review` 用于审计，不改变请求合同。
- 取消/拒绝/过期产物隔离；待审 24h/Session 绝对期限；私有 blob 限额 256 MiB；HTTP 普通请求/SSE/大 body 并发分别限制，上传流式上限按 MIME 施加。
- 新状态根 namespace 拒绝复用 Model 根或任意非空目录。G1/G2 未发布的无 namespace 预览根也拒绝自动迁移；使用新私有根或由本人单独审核迁移，绝不自动删除旧状态。
- GardenFlow 风格假客户端通过上传→Task→待审→来源拒绝/attest→ready→认证下载 SHA/ETag 闭环；仅改 xcli 测试，没有触碰 GardenFlow 业务代码。
- 全量 `go test -race ./...` 与最后核心/Model 回归 PASS；vet/build 退出 0。所有图片是明确的测试夹具，仅证明字节/状态流程，不是新模型图片证据。
- 保留初次失败：媒体夹具扩展了内存 client scope 但未更新持久授权，执行前正确拒绝；修正为显式登记独立假客户端。一次全量负载下的有界假版本探测 3s 超时，未放宽条件，连续 10 次探测及完整回归随后通过。

G4 下一任务：journal 压缩与实际留存、fsync/crash 故障点、SSE 重连/过窗/慢读/撤销、预算竞争与安装域监督、完整离线门禁/交叉构建/性能报告、用户操作手册与真实 L01–L05 计划。原生调用仍 0，原 Gate B 仍 BLOCKED。

## G4 最终离线交付（2026-11-07）

- 真正杀死测试服务进程的 accepted/dispatch-write/driver-start/terminal-write/checkpoint 故障点恢复通过；任一分支没有第 2 个 dispatch。不确定 fsync/ENOSPC 中止 HTTP 响应，原键恢复，无虚假拒绝 ACK。
- 新增 watermark 最低序号/实例/摘要，已确认尾部截断不再当成正常尾部丢弃，完整损坏原样保留并拒绝启动。原子压缩持续递增序号，清理过期正文/旧 checkpoint/本服务临时文件，不清零预算；仅扫描本服务 blob 做 GC。
- UTC+单调时限、deadline 优先于晚取消，未确认停止全根隔离，版本漂移执行前拒绝；idle/worker 状态查询避免复制全部结果。活动每类每秒聚合，终态不丢。
- 原生未知事件不能提供 Gateway 的会话/最终文本/用量；固定 prompt 数据前缀阻止 print/resume 将 `--force` 等输入误作 native 选项；旧 run/resume 不变。
- 当前适配器无法证明文件绑定的原生图片工具来源，工具名/catalog/父 subagent 均不自动通过，真实来源需本人 trace attestation；没有生产测试专用强开分支。媒体的 documented 不从 headless 文档泛化。
- 100 并发 HTTP 同键、跨 Session 预算 race、队列满/busy、零调用排队取消、日志故障中止、SSE 重连/过窗/慢读/撤销、取消后晚图、过期 input tombstone 与隐私正文清理等均通过。
- 最终完整脚本 `bash tests/native_agent_gateway/verify-offline.sh` 退出 0：全量 race、vet、宿主 build、两次实际 xcli 服务生命周期/HTTP、旧 Python harness 11/11、diff 检查。另 `go mod tidy -diff` 退出 0；Darwin amd64、Linux amd64/arm64、Windows amd64 交叉构建退出 0（不是实机验收）。
- 基准仅缓存目录与 1 KiB 假任务的进程内 HTTP handler：约 24.5/40.9 μs/op；不是 native 推理/网络吞吐。状态帧 32 MiB、journal 256 MiB/16 MiB 自动压缩、blob 256 MiB，容量不足失败关闭。
- 各阶段初始失败日志保留。G4 一次未带 loopback 权限的专项命令失败，最终完整宿主离线门禁通过；没有删除/跳过测试或放宽权限。
- [安装/启动/停止/Token/API 与恢复](USER_GUIDE.md)、[支持矩阵](SUPPORT_MATRIX.md)、[完整报告](IMPLEMENTATION_REPORT.md)、[真实计划](REAL_HTTP_ACCEPTANCE_PLAN.md)已交付；四应用只提供 APP-NAG-1/模块入口，没有写业务代码。

## G4 最终阻塞与恢复（历史）

真实 native 启动/模型请求仍 **0**。三家 profile 均因逐次策略、账号条款适用性、真实认证/安全媒体路径未证明而关闭；Cursor Trust 另由本人处理。授权本身不等于策略已实现；真实入口须先补可执行隔离工程、逐项合成 L04 计划和明确新预算，不能从普通配置/Token 强开。G5 app-server 晋升、G6 四应用/UI、Docker 桥接、生产部署均未实施。

恢复命令：

```bash
git status --short
git log -8 --oneline
bash tests/native_agent_gateway/verify-offline.sh
```

恢复先读本文件、IMPLEMENTATION_REPORT、REAL_HTTP_ACCEPTANCE_PLAN；仅在要复验离线代码时运行上述脚本。不得运行旧 native smoke 的 --execute，不读原生登录缓存，不改 Cursor Trust，不按历史 PID kill，不 Push。真实 NAG Gate 和原 Gate B 均保持 BLOCKED；原 Gate A 仅继承历史特定场景。

## 最终交接定位

产品代码提交：`8526361f8d62b7dd5058096aa69a21a121fa3c3b`。本节为仅文档交接补充，不修改已验收代码；当前报告版本用 `git log -1 -- docs/native-agent-gateway/IMPLEMENTATION_PROGRESS.md` 定位，避免自引用 SHA。

本轮阶段提交依次为 `86717e6`（G1）、`25201e1`（G2）、`01581bf`（G3）、`8526361`（G4 离线与真实计划），均只在本地；前序 `864a6b1` / `40fc6ca` 完整保留。最终产品/测试源码清单 SHA-256：`aca0d152e5ac43cec6dd85c98faa931fd74af496ed72ad89ac00736d14165eab`。文档交接后不需要重复执行已经通过的完整测试；只有代码/约束改变或复验需要时才运行离线门禁。

## G5A 实际交付（2026-11-08）

- 完整恢复用户指定八份文档，复核生产命令、Profile.Check/Defaults、缓存、Exec、observer、service worker/Workspace/媒体和既有回归。没有重做 G1–G4，也没有更改 Model Gateway 或四应用。
- 官方页面实际核查：Codex app-server/SDK/SIWC/plan usage，Cursor headless/ACP，Agy headless/ACP，三家条款。区分接口存在、当前安装、账号、应用场景及可执行隔离；Agy 条款与多应用网关用途关系独立 BLOCKED，未假定通用豁免。
- 本机版本 Codex 0.167.0、Agy 1.5.2、Cursor 2026.10.22-27dcf5f。三家无 turn initialize 成功；Codex 322 个 schema 文件摘要/所需方法存在。Cursor ACP EOF 后超时、TERM 143；不能升级为退出行为通过。
- 共 21 次顶层非模型探针调用：Codex 8、Agy 7、Cursor 6；[独立账本](evidence/g5a/non-model-ledger.json)。没有认证/Session/turn 请求、推理 prompt 或真实 HTTP Task。未读取、复制、打印认证 Token；未改 Trust、账号、模型、长期配置或开启付费额度。
- 新增 owner-only 停服 `preflight --agent`：配置／缓存／选中文件 hash／策略／认证／账号／授权／Trust／真实 Gate 分项；零 spawn、不可晋升。Native 无条件拒绝仍在，非 native 夹具不能进入真实就绪检查。
- 修复 Exec 已知拒绝被后续 truncated 覆盖的问题；保留协议 truncated、部分文本 final=false，auth/quota＋9 MiB 假输出回归通过。refresh 失败固定脱敏分类、IOError 失败关闭，3s/停止条件未放宽。
- 真实 Gateway refresh：Codex/Agy PASS；Cursor 两次尝试均未写缓存，第二次明确 `version_probe_stop_unconfirmed`。第一次 harness 在 Cursor 失败即终止，保留[初次日志](evidence/g5a/gateway-preflight-initial.log)；改为独立记录每家后完成[三家 HTTP 拒绝](evidence/g5a/gateway-preflight.json)，没有把 Cursor 写为通过。
- 完整离线最终 PASS：[offline.log](evidence/g5a/offline.log)。`go test -race -p 1 -count=1 -timeout 240s ./...` 全量实际重跑、vet、build、Gateway 两次生命周期／三家拒绝／预检、旧 harness 11/11、新探针测试 2/2。Token/Session owner、幂等、预算、超时／取消、进程崩溃／恢复、SSE 和媒体路径攻击等既有测试全部保留。
- 早期[并行无缓存失败](evidence/g5a/offline-parallel-startup-failure.log)：两个固定版本夹具在包级并发启动负载下超出 3s；改测试包串行分配资源后完整通过，包内并发和生产超时／安全条件不变。[初次含缓存通过](evidence/g5a/offline-initial.log)也保留。没有删测试或把失败算 PASS。
- [G5A 源码清单](evidence/g5a/source-manifest.json)：171 文件，SHA-256 `176b75449d36fca750711ec3bed248e9b889798ffd7d2cf2cb3163e741dbcf83`。文档不参与清单以免自引用；G4 旧清单／日志不覆盖。

### 当前尚未完成的 Gate

| Gate／任务 | 状态与证据范围 |
| --- | --- |
| G5A 工程／非推理审查 | PASS（上述范围）；不含 real turn／账号权益／原生隔离证明 |
| 原 Gate A | 仅继承历史 Codex/Agy 的特定场景 PASS；Cursor Trust BLOCKED；G5A 未重新推理 |
| NAG 真实最低 Gate / L01–L05 | BLOCKED；所有真实场景 NOT_TESTED，S01–S08 + 各 CLI 独立原因见 readiness，不能归为只差用户调用许可 |
| 原 Gate B | **BLOCKED**；native CLI→Model Gateway `/v1/*` 未实现且不属于本服务 |
| 官方 app-server / ACP 运行 adapter | NOT_IMPLEMENTED；仅本机 initialize/schema 评估。增量 SPEC 与估算已记录，未重构 |
| 四应用／UI／Docker／生产 | 原计划不变，未实施 |

### 后续恢复与具体入口

先检查 `git status --short`、`git log -5 --oneline`，读本节和 G5A/G5B 文件。下一工程是 Codex S01–S03 + C01：受限验收许可、原生认证与逐次工具／文件／网络隔离、服务合成 Git 工作区，约 2–4 人日；做不到安全边界就保持关闭。app-server（5–9 人日）和 ACP 为独立可选 transport，不能为省事改 Native=false、添加 enable bool 或透传 RPC。

仅复验已完成离线代码：`bash tests/native_agent_gateway/verify-offline.sh`。停服只读就绪报告：`/tmp/xcli-g5a agent-gateway --state-dir <独立私有根> preflight --agent codex`，当前 can_execute=false。二进制为临时构建，下一会话必要时按 USER_GUIDE 重建。

不要自动重跑 `probe_protocol.py --execute-non-model` 或 `profile_preflight.py --execute-non-model` 来增加无必要启动；已有证据足够。后续若安装变动需重新非模型检查，单独记账；所有真实推理仍需要新明确授权。Cursor 另需具体服务 workspace 的官方 Trust 审查、bundle 完整性和生命周期修复；Agy 另需供应商用途授权依据。未获授权前不运行 G5B／旧 native smoke。

G5A 实现、审查、测试及证据提交：`33ac9273f25dce9574b0f82af8c1e4f5dc413580`（`feat(agent-gateway): review G5A native profile readiness`）。提交后再次核对 171 个源码摘要一致，工作区干净。本段随后仅补录实际 SHA；交接文档自身提交由 `git log -1 -- docs/native-agent-gateway/IMPLEMENTATION_PROGRESS.md` 定位，避免自引用。不 Push/Tag/Release/部署。

## G5A.1 Codex 最小受限启用工程（2026-11-08）

基线 HEAD `6e2ecd77c0c4`（完整值以 Git 为准），工作区开始干净。已重新阅读用户指定七份文档及相关配置／Profile／service／worker／Exec／私有存储代码；基于原架构增量实现。未做 Cursor/Agy 生产改造，未修改四应用，未部署、Push 或发布。

已实现：

- 默认关闭的 Codex-only 配置和 owner 控制面；prepare 新合成 Git 注册、显式非模型 probe、readiness、启用、独立调用授权、运行期 disable。未知状态／字段、版本／摘要变化、未完成证明均关闭，不能用配置 true 或伪造 JSON 越过 native verifier。
- 固定 workspace/材料/prompt、text-only、指定客户端、禁止续接／文件／HTTP argv/env/binary/cwd；1/1/120 Session 和 Task 限制，复用单并发、owner、scope、grant epoch、CAS、幂等、deadline、cancel、重启恢复。
- journal lifetime 计数每登记 Profile 最多一次启动，不随新 Session／重启／跨日恢复。新 Task 记录调用授权摘要；dispatch 再核对、活动监督器检查关闭／授权撤回或过期。控制文件管理锁、fsync/rename/目录 fsync，不靠清空预算恢复。
- [首次 HTTP 脚本](CODEX_FIRST_REAL_HTTP_TEST.md)：默认 dry-run，明确 execute＋新授权引用才检查进入；一次 Task POST、无不确定结果重试、SSE／终态／内容校验、停止重启后的持久查询、脱敏证据。当前真实就绪门禁不满足，不能执行成功。

原生实际证据：[final-probe.json](evidence/g5a1/final-probe.json)。Codex 0.167.0，登录类别 AUTHENTICATED_CHATGPT；登记材料读取阳性对照、越界读拒绝、工作区写拒绝、symlink 逃逸拒绝、工具 loopback 网络拒绝均通过。官方 `sandbox` helper 执行固定系统命令，没有 exec turn 或推理 prompt。首次尝试按网站旧 `sandbox macos` 形态被环境拒绝，保留 [help.json](evidence/g5a1/help.json)，之后以本机 `sandbox --help` 正式格式执行；没有放宽策略。

独立合成项目 `/private/tmp/xcli-g5a1-state-9a6vyye3/codex-limited/workspace`；没有将 xcli 或其他仓库作 Agent 工作区。只登记 native auth-home 路径，登录凭据始终由官方进程读取；报告不保存账户、Token 或原始诊断。

**当前工程 Gate：BLOCKED，不是 READY_FOR_AUTHORIZATION。** 实际 kernel canary 只证明本地命令边界。现有单向 exec 适配器尚无完整有效工具清单／内置工具与认证目录访问隔离的证明；`nativeToolBoundaryVerified()` 保持 false，是需补齐的真实验证器工程，不是再给一次调用授权可消除的问题。账号所有权、计划／目标模型权益、个人本地用途适用性也未全部确认。Native exec 未启动推理；不自动切换 app-server 或 SIWC 来绕过。

最终完整离线回归 PASS：[offline.log](evidence/g5a1/offline.log)、[验收摘要](evidence/g5a1/validation.json)。全量 `go test -race -p 1 -count=1 -timeout 240s ./...`、vet、build、既有 Python 11/11、新脚本 4/4；HTTP→假 Codex／持久结果、100 同键竞争、跨 owner、一次 lifetime 启动、关闭／撤销／超时与重启均有回归。原有媒体路径攻击、晚到、故障／旧 Runtime 检查保留且通过。源码变更后早期完整通过日志另存 before-receipt-binding / before-authorization-snapshot，不冒充最终源码验证。

真实预检另有 [HTTP 拒绝证据](evidence/g5a1/native-gateway-rejection.json)：独立本地 codex-acceptance 客户端准备就绪；即使暂时配置 enabled=true，enable 命令仍拒绝，HTTP 建 Session 仍 503 policy_unenforceable，0 Task POST。临时启用意向配置已删，持久开关仍 false、调用授权为空；实际验收 Token 仅存该临时状态根的私有 client-secrets 目录，没有导出到仓库或四应用。[默认 dry-run](evidence/g5a1/script-dry-run.json) 已执行，0 模型 Task POST。

本轮顶层原生非模型调用共 **18**：[账本](evidence/g5a1/non-model-ledger.json)，不包括 Git、xcli 管理命令或明确的假进程测试；真实推理／HTTP 真实 Task 仍 **0**。失败记录保留：初次 Events 返回值编译错误；HTTP fixture 缺固定 Host、错误尝试用 rotate 改授权（生产代码正确保留原授权）等，在独立新客户端 fixture 修复；没有删测试／放宽 Host 或 grant 检查。

[最终源码清单](evidence/g5a1/source-manifest.json)：179 文件，SHA-256 `ac62544bea11f79f9f5a5309f59dfa35a9225729078692d6ae1c05f465d0d417`；文档链接与新增证据秘密形态检查通过，旧证据未覆盖。本地实现提交随后由 `git log -1 -- internal/agentpolicy/limited.go` 定位，实际 SHA 在交接段补录。

恢复：先读本节、CODEX_LIMITED_PROFILE、CODEX_FIRST_REAL_HTTP_TEST，检查 git status/log。离线复验 `bash tests/native_agent_gateway/verify-offline.sh`；只读报告用 `codex-limited readiness`。不要运行真实脚本的 execute 或旧 native smoke；不要手改 Native/控制状态强开。要继续启用，先补齐全工具／认证根隔离验证器及其真实非模型证据；账号／用途确认和独立一次调用授权随后分别处理。现阶段即使批准 1 次调用，脚本仍应拒绝。

G5A.1 最终代码、测试、证据及主体文档提交：`e742590dc7edda55dc6943b2f17e726f2bce0909`（`feat(agent-gateway): add fail-closed limited Codex controls`）。提交后确认工作区干净，179 个源码摘要与完整测试版本一致。本段仅补录实际 SHA；交接文档自身版本由 `git log -1 -- docs/native-agent-gateway/IMPLEMENTATION_PROGRESS.md` 定位。真实启用和 G5B 均未执行，原 Gate B 仍 BLOCKED。

## G5A.2 实际交付（2026-11-08）

起点 `3494756d2dda9e2b76c94ff4254b3c92db943bc9`。已读本轮指定六份文档、复核 limited policy/adapter/control/worker/旧证据，增量检查官方配置、权限、SIWC app-server；没有重做 G1–G4、迁移 Adapter 或改四应用。

- 新 `codex-limited boundary-probe` 只用程序生成的假 HOME/CODEX_HOME、假 auth.json、无害 Canary 和 mock 服务，不使用登记的真实 AuthHome，不运行 login/exec turn。实际原生 helper 证明同 UID 未沙箱可读假凭据、所选沙箱命令不能绝对／相对／symlink 读假凭据、越界读写或访问 mock 网络。当前版本 resolver 的 10 项 false 通过，但不代表全部工具不存在。
- 补齐目录替换检查：登记 dev/inode + no-follow 内容摘要；同字节目录复制替换也被拒。旧登记缺身份 fail closed、历史日志不覆盖；没有将静态检查声称为 check→spawn 持续 OS 隔离。
- 独立状态轴：ENGINEERING_BLOCKED / ENGINEERING_READY 与 USER_CONFIRMATION_REQUIRED / CONFIRMED；仅工程与本人声明齐备时总体 READY_FOR_AUTHORIZATION，仍不授权推理。新本人六项声明私有 JSON 导入，登记/策略/client 绑定、24h、严格 UTF-8/重复键/未知字段拒绝。旧 YAML 引用不代替同意；本轮真实声明全未确认，正向记录只在单元夹具中。
- 伪造全 PASS 探针／手写运行开关／用途引用不能补出缺失的有效工具目录、builtin 直接 IO 独立边界和 exec 工具环境证明；enable/authorize/native Session/dispatch/spawn 保持阻断。
- 额外真实安装链路检查：配置 enabled 意向、专用客户端、1/1/120 预算具备，HTTP Session 仍 503 policy_unenforceable；无推理 Task POST。默认 dry-run 与无前置许可的 --execute 都为 0 Task。[控制证据](evidence/g5a2/control-boundary.json)、[readiness](evidence/g5a2/readiness.json)。
- 首次脚本在启动／重启前记住旧 generation，只接受新代次发布的地址；修复补充检查暴露的旧状态端口竞争，不重试任何模型 POST。新增单元回归。HTTP 初次检查失败和第二次连接旧端口失败均保留[原因记录](evidence/g5a2/initial-check-failures.json)，修复后同一状态根负向验证通过，没有重复原生探针。
- 完整最终 [offline.log](evidence/g5a2/offline.log)退出 0：全量 go test -race -p 1 -count=1 -timeout 240s ./...、go vet ./...、go build、两次服务生命周期/三家 native 拒绝、旧 Python 11/11、新 Python 5/5、diff 检查。首次全量通过后因脚本新修复重跑，原始 PASS 保存在 offline-initial.log。初步默认 Go cache／loopback 权限失败已记录；未删测试或放宽条件。
- [源码清单](evidence/g5a2/source-manifest.json)：184 文件，SHA256 `bcbd8d5134bf6facd63a49c0db1f4ff929f6d649be46444380a3a9cb79b4962d`；[验证索引](evidence/g5a2/validation.json)。[最终 OS probe](evidence/g5a2/final-boundary-probe.json)23 项 PASS、3 项 NOT_PROVEN、1 项 NOT_TESTED，明确区分 static/mock/helper/真实 turn。
- 本轮 22 次顶层 Codex 非模型启动：3 features 检查、9 初次 boundary、1 新 prepare 的 --version、9 最终 boundary；[账本](evidence/g5a2/non-model-ledger.json)。**真实推理请求 0、真实 HTTP Task 0、真实认证材料使用 0**；没有复用剩余额度或修改原生配置、Trust、账号。顶层次数不是全部 OS 进程数或模型内部调用／费用。

### 当前未完成门禁

| 门禁 | 结果／独立原因 |
| --- | --- |
| G5A.2 工程检查及离线增量 | PASS（上述限定范围），不代表安全工程整体已就绪 |
| Codex 工程就绪 | ENGINEERING_BLOCKED：完整工具目录、直接 builtin IO 独立边界、实际 exec 工具环境未证明；目录 check→spawn 持续边界／所有 builtin 网络也未证明 |
| 本人账号／计划／用途／云发送／费用 | USER_CONFIRMATION_REQUIRED；本轮未读真实登录，历史登录不是权益或同意 |
| 新的一次真实调用授权 | 未授予，enable 与 authorize 仍拒绝；不能只授权一次便跨过工程缺口 |
| Codex/Cursor/Agy 真实 Gate B、NAG 真实 L01-a | BLOCKED／NOT_TESTED，均非 PASS；Cursor/Agy 没有新增工程或 Trust 变化 |
| 原 Gate A / Model Gateway Gate B | 只继承原特定历史证据；不增加模型 API 兼容、原 Gate B 仍 BLOCKED |

### 恢复入口与后续工程

先读 [CODEX_ISOLATION_BOUNDARY](CODEX_ISOLATION_BOUNDARY.md)及[本人确认](CODEX_ACCOUNT_AND_USAGE_CONFIRMATION.md)，再读 CODEX_LIMITED_PROFILE、CODEX_FIRST_REAL_HTTP_TEST、G5B_REAL_HTTP_PLAN。下一任务是补独立执行边界：exec 专用环境初估 3–6 人日，app-server Adapter 5–9 人日，SIWC 在其上另 3–5 人日及资格审核等待。只是增量评估，没有自动创建系统用户、环境、OAuth 或切换后端。

```bash
git status --short
git log -6 --oneline
GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off go build -trimpath -o /tmp/xcli-nag .
/tmp/xcli-nag agent-gateway --state-dir /private/tmp/xcli-g5a2-pgnzz8as/state \
  --gateway-config /private/tmp/xcli-g5a2-pgnzz8as/gateway.yaml codex-limited readiness
```

新合成工作区 `/private/tmp/xcli-g5a2-pgnzz8as/state/codex-limited/workspace`，auth-home 是空假目录，最终配置和运行开关关闭、服务已停止；不能用于真实推理。临时目录可能被清理，证据已脱敏保存在仓库；不要靠新建根重置预算。历史 G5A.1 根未改认证引用、未补写目录身份。完整离线复验入口仍为 `bash tests/native_agent_gateway/verify-offline.sh`，只在需要复验或源码修改后重跑；不自动重跑原生 probe。

未来工程和本人确认齐备，再按 CODEX_FIRST_REAL_HTTP_TEST 的 enable→本轮授权 authorize→--execute 命令处理。当前任何一次调用同意都不足以运行。禁止 Push、真实推理、原生认证资料读取／复制和 Cursor Trust 自动改动仍有效。
