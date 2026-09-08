# 阶段 0：基线、契约与验证

依据 2026-09-08 的[阶段计划](../plans/phased-roadmap.zh-CN.md)、[优化方案](../plans/optimization-proposal.zh-CN.md)、[主审查](../reviews/2026-09-07-product-and-code-review.zh-CN.md)及[tag 补审](../reviews/2026-09-08-tag-version-review.zh-CN.md)。本目录完成 A01–A03 的准备工作；缺陷修复属于阶段 1。负责人为本轮实施者（Codex，用户验收）。

| 任务 | 交付 | 验收证据 |
| --- | --- | --- |
| A01 | [固定基线与缺陷归属](baseline.zh-CN.md) | 完整 commit、复现入口、模块、责任归属与阶段 1 依赖 |
| A02 | [兼容契约决策](contracts.zh-CN.md) | [机器可读样例](../../tests/phase0/testdata/contracts.json)、样例检查 |
| A03 | [验证方式与矩阵](validation.zh-CN.md) | [本轮验收记录](results.zh-CN.md)、独立复现工具和脱敏结果 |

M0 的最终结论见验收记录。样例中的新增字段是冻结的后续实施目标，并非当前 CLI 已提供的接口。模拟缺陷复现成功也不代表问题已修复、真实 CLI 兼容或满足发布门槛。
