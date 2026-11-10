# 四应用 Agent Task 接入设计

**仅设计，本轮不修改四应用，也不发布新的 Task API/IPC。** 当前可用的产品执行路径仍是本人控制的 CLI `task/run/use/resume`；冻结的 `/agent/v1` HTTP 服务不升级为推荐接入点。历史 Model Gateway 补丁不回放，不提供通用推理代理。

## 共同合同

应用必须由用户明确发起任务，或持有可撤销、限定目标/材料/时间/额度的自动化授权。每个任务指定 Agent、已核准工作区、输入快照、产物目录、验收条件和进程/时间上限；本机发起不等于厂商模型在本机运行。

未来可评估 OS peer 身份或每应用独立客户端授权的受限本机 IPC/Task API。最小合同应覆盖 submit/status/events/cancel/result，具有应用 ownership、幂等键与预算；这些是拟议语义，**不是本轮新路由或 CLI flag**。应用不能提交任意 argv/env、供应商 Token 或把整个数据库/凭据目录暴露给 Agent。

流程：确认最小材料和云发送 → 检查安装/声明能力（不发模型探针）→ 固定任务身份和输入摘要 → 有界 dispatch → 观察状态/取消 → 收集真实文件和验证证据 → 应用本地业务校验 → 用户接收后写回业务对象。断连先查既有任务，不隐式重提；未知执行结果先 reconcile，不自动重放。

Session 归属到同应用、任务、CLI 与工作区；交接传材料摘要/文件副本，不把 Codex Session ID 给 Cursor。业务授权与供应商原生授权彼此独立，不能承诺不限量调用用户 CLI 订阅；部署形态和自动化用途必须在厂商允许范围内。

## Worklog

任务是用户选定日期和工作项生成报告/周报/模板草稿，或核对现有草稿。只导出批准的字段和 source IDs，不直接授予整个工作日志数据库。任务输出真实 Markdown/JSON 文件；日期、引用、模板变量与本地 Schema 由应用校验，再预览接收。

一个任务可能多次调用原生 CLI，应显示预算和状态，不包装成每次输入即透明调用的 chat/completions provider。失败保留材料和原因，不自动切账号/改模型。原始设计与静态模块定位见 [Worklog 历史文件](../cli-first/integrations/WORKLOG.md)。

## GardenFlow

任务是用户指定的文本素材整理、选定图片理解、或在原生 CLI 已获准工具能力下产生图片/代码等文件。指定输入和输出目录，保留原文件及 hash；生成后校验实际字节/格式/来源，完成视觉和业务接收才入库。

不暴露通用 `/images/*`、模型/Token 池或无限自动生成循环。现有应用自己的媒体功能保持应用自身责任，xcli 不建设通用媒体服务。CLI 图片能力不继承到所有模型/版本，权限拒绝不能自动改走另一 CLI。见 [GardenFlow 历史文件](../cli-first/integrations/GARDEN_FLOW.md)。

## Chat Insight

任务是对用户明确选定并同意导出的会话片段做总结/审阅/专题分析，或检查用户选定的项目文件。最小化个人信息，记录范围与授权，不读取完整聊天库或自动重建索引。

输出摘要/分析文件与来源引用，由应用预览接收。这里不是连续 token 推理 API，也不提供 CLI Embedding 或向量索引服务。仅凭 localhost 不免除云发送同意。见 [Chat Insight 历史文件](../cli-first/integrations/CHAT_INSIGHT.md)。

## Contribution OS

任务是用户选定仓库、issue 和变更范围下的 Plan/实现/Review；固定已审查的工作区/输入，使用 worktree、真实 diff/test/复核证据，再由应用确认是否接收。不得让 Agent 持有业务服务/部署密钥、直接部署或绕过原应用审批。

本轮不建设容器到宿主 CLI 的通用桥接。未来仅在部署边界和厂商用途明确后评估本机 Task IPC；没有接口时显示未接入，不能把旧模型 URL 改个 alias 冒充原生任务。见 [Contribution OS 历史文件](../cli-first/integrations/CONTRIBUTION_OS.md)。

## 独立接入验收

先以假 Agent 和临时业务副本检验云拒绝零 dispatch、任务/Session/文件归属、幂等、预算、取消、断线、崩溃恢复、格式与业务 Schema、无 Token/原文日志泄漏。然后另行审查各应用的真实调用与用户体验范围。

应用提交成功、Agent 进程成功、产物验证和用户接收必须分别记录。没有应用端代码与 UI 实测时，不能把 xcli 的离线通过升级为四应用已完成集成。
