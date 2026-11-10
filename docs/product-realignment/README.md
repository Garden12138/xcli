# R4 — CLI-First Product Realignment

2026-11-10，基线 `a3e258e9275838d46c4aac878757e05ab7881180`，开始时工作区干净。当前源码候选 `0.5.0-r4-dev`，未发布。

**xcli 是统一 AI Coding Agent CLI 调度与编排工具。各原生 CLI 自行安装、登录、维护账户与模型授权；xcli 负责发现、选择、执行、编排、监控、恢复和交付任务结果。**

本轮直接收敛产品，不再研究 G8.1 OAuth，不开发模型 Provider、订阅共享或通用 OpenAI API 包装。保留所有已实现 Adapter、配置、历史数据及 M0–M5/G1–G8 证据。

| 文档 | 内容 |
| --- | --- |
| [PRODUCT_POSITIONING](PRODUCT_POSITIONING.md) | 用户、核心价值、正式范围和明确排除项。 |
| [ARCHITECTURE](ARCHITECTURE.md) | 七个核心模块、执行链路与边界。 |
| [MODULE_DISPOSITION](MODULE_DISPOSITION.md) | M0–M5/G1–G8 审查、依赖、配置/数据、冻结与拆分建议。 |
| [CLI_AUTH_BOUNDARY](CLI_AUTH_BOUNDARY.md) | 原生认证与 xcli 本地客户端授权职责。 |
| [TASK_ORCHESTRATION_ROADMAP](TASK_ORCHESTRATION_ROADMAP.md) | 当前 A–G 能力和下一步三个重点。 |
| [APPLICATION_INTEGRATION_GUIDE](APPLICATION_INTEGRATION_GUIDE.md) | 四应用仅按授权 Agent Task 接入的设计。 |
| [ACCEPTANCE](ACCEPTANCE.md) | 改动、回归、证据、限制和最终回答。 |

日常入口：[使用手册](../cli-first/XCLI_USER_MANUAL.md)、[总体架构](../ARCHITECTURE.md)、[R0](../r0/README.zh-CN.md)、[R1](../r1/README.zh-CN.md)、[R2](../r2/README.zh-CN.md)、[R3 支持矩阵](../r3/compatibility.zh-CN.md)。旧阶段命令与证据维持原作用域，当前定位以 R4 为准。

Model Gateway、Native Agent Gateway HTTP、SIWC/OAuth/Responses、Credential Manager 与通用媒体模型服务冻结、默认关闭，不作为核心可用性的前置条件。旧实验服务不自动停止或清理；状态查询、停止、撤销与 disable 仍可使用。

核心产品检查为 `bash tests/product_realignment/verify-core.sh`。保留代码的全量离线检查为 `bash tests/product_realignment/verify-offline.sh`。二者都不调用真实模型；冻结实验的真实资格/隔离/账号门禁不再决定 CLI 产品能否使用。
