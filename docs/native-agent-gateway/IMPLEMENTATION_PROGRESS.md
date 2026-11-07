# Native Agent Gateway V1 — 实施与恢复

开始日期：2026-11-07；基线 `40fc6ca`（前序真实验收 `864a6b1`）。本轮获授权：本仓库实现、离线/假 CLI 测试、独立本地提交。真实原生模型调用预算 **0**；不继承前轮 3 次余额；不 Push/Tag/Release/部署、不改四应用或原生认证/Trust。

## 当前入口

当前阶段：**G1 离线通过，准备 G2**（对应用户阶段 A，按原 SPEC 依赖推进）。

恢复先读本文件，再读 `TASKS.md`、`ACCEPTANCE.md` 和 `git log --oneline -8` / `git status --short`。不得重新执行历史 native smoke。

## 已确认决策

- 已完整阅读 README 与七份 SPEC，审查 agent decoder/argv/diagnostics/usage、runtime/process/environment、runstore、task lifecycle、Model Gateway/server/client/private/service/config 与图片校验代码。
- 独立 `/agent/v1`、Token、配置、端口、状态根；不接 `model.ChatBackend`，不调用多轮 Task Runner。
- 所有真实 profile 在逐次权限和 Gateway 验收前关闭；缺官方可执行隔离证据时 `policy_unenforceable`。先做 HTTP → 受控假进程，可复现的离线证据与真实验收分开。
- append-only fsync 事务先于 dispatch，重启仅恢复状态；不按历史 PID kill，不自动 retry/fallback。
- 官方文档核查进行中，app-server 仅离线协议评估，G5 晋升另立范围。

## 阶段记录

| 阶段 | 状态 | 本地提交 | 测试/证据 |
| --- | --- | --- | --- |
| G0 | 已继承设计 | `40fc6ca` | 七份 SPEC + README |
| G1 | PASS（离线范围） | 本条所在提交，见 git log | [核心 race](evidence/g1-core-race.log)、[旧回归 race](evidence/g1-regression-race.log)、vet/build |
| G2 | PENDING | — | 文本/续接/事件/停止 |
| G3 | PENDING | — | 输入/字节产物/复核 |
| G4 离线 | PENDING | — | 故障、安全、兼容、性能 |
| G4 真实 / 用户 F | BLOCKED | — | 无新授权；需计划与权限先决条件 |
| G5 / G6 | 后续独立范围 | — | app-server 晋升 / 四应用 |

## 门禁

- 本轮真实模型启动：0；真实 HTTP→native CLI：NOT_TESTED。
- 原 Gate A：继承 Codex/Agy 的特定历史场景 PASS，Cursor workspace trust BLOCKED；不作为新 Gateway 证据。
- 原 Gate B（native CLI→Model Gateway）：**BLOCKED**；V1 无该兼容层。
- NAG 离线：进行中；NAG 真实：BLOCKED；四应用 Gate C：Planned/BLOCKED。

## 下一任务与恢复命令

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
