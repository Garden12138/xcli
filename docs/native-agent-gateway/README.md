# Native Agent Gateway V1 SPEC

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

本轮只读取本仓库源码、已有四应用接入记录及供应商公开官方文档；不读取原生登录凭据，不运行 native 版本/认证/推理探测，不修改四应用、不部署、不发布、不 Push。

当前实现、提交、失败及恢复入口见 [IMPLEMENTATION_PROGRESS.md](IMPLEMENTATION_PROGRESS.md)。SPEC 的历史“本轮只设计”描述属于 2026-11-06；2026-11-07 的实现/离线测试单独留证，未启动新的真实模型调用。

实施交付：[验收报告](IMPLEMENTATION_REPORT.md) · [安装/API 手册](USER_GUIDE.md) · [实际支持矩阵](SUPPORT_MATRIX.md) · [真实验收预算计划（未授权）](REAL_HTTP_ACCEPTANCE_PLAN.md)。
