# Native Agent Gateway V1 SPEC

> R4 / 2026-11-10：此模块冻结，默认关闭，不再作为 xcli 核心产品目标或可用性门禁。停止后续真实验收/OAuth/模型服务开发；下文仅保留历史方案和证据。当前入口见 [R4](../product-realignment/README.md) 和 [模块处置](../product-realignment/MODULE_DISPOSITION.md)，不按历史计划自动恢复实验。

G7 最新入口：[隔离运行时实验](G7_ISOLATED_RUNTIME_EVALUATION.md) → [凭据边界](G7_CREDENTIAL_BOUNDARY.md) → [最终决策与操作](G7_DECISION.md)。当前 **PARTIAL_HOST_ISOLATION_ONLY**：Linux 宿主隔离通过，凭据/全工具分离未完成，真实推理 0，真实 Profile 继续关闭；本轮结束，不自动进入 G8。

G6 历史入口：[app-server 实现](G6_APP_SERVER_IMPLEMENTATION.md) → [安全边界](G6_SECURITY_BOUNDARY.md) → [离线证据](G6_OFFLINE_ACCEPTANCE.md) → [G6 决策](G6_DECISION.md)。原 ISOLATED_RUNTIME_REQUIRED 的证据保留，不被 G7 假任务结果覆盖。

G5A.2 当前结论：**ENGINEERING_BLOCKED / USER_CONFIRMATION_REQUIRED**，真实模型调用 0。恢复从 [IMPLEMENTATION_PROGRESS](IMPLEMENTATION_PROGRESS.md)、[Codex 隔离边界](CODEX_ISOLATION_BOUNDARY.md)、[本人确认](CODEX_ACCOUNT_AND_USAGE_CONFIRMATION.md)进入；不要将原生命令 Canary PASS 视作真实 HTTP Gate B PASS。

日期：2026-11-06。状态：**G1–G4 离线实现交付；真实验收计划已备齐；真实原生 Gateway 仍 BLOCKED**。适用范围为本人控制的本机应用；不是共享推理服务或 OpenAI API 兼容层。

真实 CLI 基线为 `7e57c6774b4bcfd4735f0d4c35c7c7a3a58d2ac4`；验收证据及 Smoke 修复已单独提交为 `864a6b1`。本 SPEC 基于该提交的源码与[验收报告](../cli-first/ACCEPTANCE_REPORT.md)，没有新模型调用。

| 文档 | 内容 |
| --- | --- |
| [REQUIREMENTS.md](REQUIREMENTS.md) | 范围、硬性边界、需求编号与应用能力 |
| [DESIGN.md](DESIGN.md) | 架构、源码复用、生命周期、安全、工程量与风险 |
| [TASKS.md](TASKS.md) | 分阶段开发顺序、依赖和完成标准 |
| [ACCEPTANCE.md](ACCEPTANCE.md) | 需求追踪、离线/真实/应用验收方法 |
| [CLI_ADAPTER_MATRIX.md](CLI_ADAPTER_MATRIX.md) | 官方接口调研、真实证据、适配差异及选型 |
| [API_CONTRACT.md](API_CONTRACT.md) | 规范性 HTTP、Session、Task、SSE、Artifact 与错误合同 |
| [MIGRATION_AND_COMPATIBILITY.md](MIGRATION_AND_COMPATIBILITY.md) | 旧接口兼容、四应用接入协议与迁移/回退 |

阅读顺序：需求 → 适配矩阵 → 设计 → API → 任务与验收 → 迁移。字段和错误枚举以 API_CONTRACT 为准；任何示例均为拟议合同，不是可运行配置或真实响应。

**Gate B 保持 BLOCKED**：原 Gate B 专指原生 CLI → Model Gateway `/v1/*`，V1 不实现该兼容层。新 Agent 链路使用独立的 NAG 验收，不能借改名关闭 Gate B。Gate C 的四应用原生接入也仍为 Planned/BLOCKED。

2026-11-06 的 SPEC 阶段只读取本仓库源码、已有四应用接入记录及供应商公开官方文档，没有 native 探测。2026-11-08 G5A 按新授权完成版本／帮助/schema／无 turn 握手，仍无新推理；不读取原生登录凭据、不修改四应用、不部署、不发布、不 Push。

当前实现、提交、失败及恢复入口见 [IMPLEMENTATION_PROGRESS.md](IMPLEMENTATION_PROGRESS.md)。SPEC 的历史“本轮只设计”描述属于 2026-11-06；2026-11-07 的实现/离线测试单独留证，未启动新的真实模型调用。

实施交付：[验收报告](IMPLEMENTATION_REPORT.md) · [安装/API 手册](USER_GUIDE.md) · [实际支持矩阵](SUPPORT_MATRIX.md) · [真实验收预算计划（未授权）](REAL_HTTP_ACCEPTANCE_PLAN.md)。

G5A 最新入口：[逐 Profile 就绪与分项阻塞](G5A_PROFILE_READINESS.md) · [官方接口／账号／应用授权边界](OFFICIAL_INTEGRATION_REVIEW.md) · [G5B 每家首次 1 次文本计划（未授权）](G5B_REAL_HTTP_PLAN.md)。三个真实 Profile 仍关闭，app-server/ACP 尚未晋升。

G5A.1：[Codex 受限控制与真实命令沙箱证据](CODEX_LIMITED_PROFILE.md) · [首次 HTTP 脚本（默认 dry-run）](CODEX_FIRST_REAL_HTTP_TEST.md)。Native 全工具／认证隔离验证器仍 BLOCKED，不把部分原生 canary 通过当真实推理 Gate 通过。
