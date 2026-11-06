# CLI-first 验收与接入交付

核查日期：2026-11-06。基线：`f529e047ca03ef749e3ac49abe146f2ef4b46eab`。

**当前结论：离线门禁 PASS；原生真实文本/图片 NOT_TESTED（调用额度待明确授权）；CLI → Model Gateway 为 BLOCKED（未实现）。四应用只完成接入设计，不能启用 CLI 模型配置。**

| 文档 | 用途 |
| --- | --- |
| [验收报告](ACCEPTANCE_REPORT.md) | 命令、证据、修复、状态与剩余门禁 |
| [使用手册](XCLI_USER_MANUAL.md) | 构建、诊断、Agent 命令、重启及回退 |
| [能力矩阵](NATIVE_CLI_CAPABILITY_MATRIX.md) | 官方声明、历史记录与本轮实测分别展示 |
| [接入架构](INTEGRATION_ARCHITECTURE.md) | 待实现的受控 Agent Task API 和验收计划 |
| [官方资料核查](OFFICIAL_SOURCES.md) | 当前接口、授权、费用及账号边界 |
| [Worklog](integrations/WORKLOG.md) | 日报、周报、模板与 JSON |
| [GardenFlow](integrations/GARDEN_FLOW.md) | 文本、Vision、媒体文件及工具审批 |
| [Chat Insight](integrations/CHAT_INSIGHT.md) | 对话、云同意与独立 Embedding |
| [Contribution OS](integrations/CONTRIBUTION_OS.md) | 冻结 Profile、Run/Plan/Review 与容器边界 |

[证据清单](evidence/source-manifest.json)、[离线门禁](evidence/phase1-offline.log)、[预检查](evidence/preflight.json)、[Gate B 拒绝复现](evidence/gate-b.json)、[预算记录](evidence/native-budget.json)。所有证据仅保留脱敏检查结果；没有复制原生凭据或聊天记录。
