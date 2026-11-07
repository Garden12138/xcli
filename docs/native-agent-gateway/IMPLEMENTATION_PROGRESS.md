# Native Agent Gateway V1 — 实施与恢复

开始日期：2026-11-07；基线 `40fc6ca`（前序真实验收 `864a6b1`）。本轮获授权：本仓库实现、离线/假 CLI 测试、独立本地提交。真实原生模型调用预算 **0**；不继承前轮 3 次余额；不 Push/Tag/Release/部署、不改四应用或原生认证/Trust。

## 当前入口

当前阶段：**G4 离线交付完成；真实验收计划已提交文档，执行 BLOCKED（授权/策略前置未满足）**（对应用户阶段 A–F 的获授权部分；真实执行保持阻塞）。

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
| G5 / G6 | 后续独立范围 | — | app-server 晋升 / 四应用 |

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

## 最终阻塞与恢复

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
