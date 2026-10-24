# R3 实测支持范围 · 2026-10-19

此矩阵区分原生模型执行、安装执行和归档构建。当前验收不声明所有已有 Adapter、平台或角色组合均可用。详细结果以链接中的固定候选、原生版本和失败记录为准。

| 环境 / 角色 | 已验证 | 限制与证据 |
| --- | --- | --- |
| macOS arm64 / Codex 0.164.0、Cursor 2026.10.16-e3cb207，2026-10-24 修复候选 | chat-insight 的 Work/Project × 指定/自动四条路径；实际 worker 取消、同 Task 新 session 材料恢复与追加需求；Codex 执行、Cursor 复核 | [完整体验记录](chat-insight-experience-20261024.zh-CN.md)与[证据](evidence/chat-insight-experience-20261024.json)。保留超时、额度失败及重连中断；原项目未修改，未扩大到 Cursor 写入、实机重启或连续满三十分钟运行 |
| macOS arm64 / Codex 0.163.0：协调、执行、自查 | Work、Project、原生 resume、取消；实际 worker 中断及新 session 材料恢复 | [最终对照](evidence/comparison-final.json)、[原生兼容](evidence/compatibility.json)、[恢复补验](evidence/followup-native-20261019.json)；自查不等于跨 CLI 独立复核 |
| macOS arm64 / Cursor 2026.10.16-e3cb207：复核 | Work / Project 当前摘要复核；原生 resume 和取消；额度恢复后的真实请求与后台恢复最终复核 | [对照](evidence/comparison-final.json)、[兼容](evidence/compatibility.json)与[额度恢复后补验](cursor-restored-20261019.zh-CN.md)；此前额度失败保留，本次成功不保证未来额度 |
| Codex 执行 / Cursor 复核：后台材料恢复 | 实际 worker 取消、paused、同 Task 新 session 材料恢复，以及当前摘要的 Cursor 复核均通过 | [原生补验](evidence/cursor-restored-20261019.json)，Task 自动模式、候选池、绑定与累计预算保留；未覆盖此前失败 |
| macOS arm64：已提交候选的安装与故障诊断 | 实际归档解包、PATH/XDG、中文和空格路径、已有配置保护、损坏配置拒绝、缺失 CLI 明确失败、历史记录查询 | [安装](evidence/installation-ef7f76c.json)、[安装验收](evidence/installed-acceptance-ef7f76c.json)；本批安装检查未调用模型 |
| Darwin / Linux × amd64 / arm64：分发包 | 四组归档内容、执行权限、SHA-256 | 本机之外的三组只证明构建与归档。CI 的 Linux 安装检查也不证明 Linux 上真实 CLI 模型执行通过 |
| Cursor 写入、其他 Adapter 与角色组合 | 本阶段未做真实验证 | 代码中已有实现不自动扩大 R3 实测声明；R0–R2 的历史结果保留各自版本与角色范围 |
| 主机真实重启、开机自启、持续满三十分钟执行 | 未验证 | 模拟重启、跨终端继续、后台短任务或三十分钟预算不能替代这些证据 |

原生安装、登录、额度和权限分别是运行前提。`doctor` 的工具发现及帮助探测不证明账号或模型调用可用；缺少 CLI 时，Task 保存为 `failed`、调用数为零，方便诊断，不返回 completed。xcli 不登录、切换账号或替用户购买额度。

当前候选仍未正式发布。独立参与者试用和协作收益门槛保持开放，不能由安装检查或收窄支持声明代替。
