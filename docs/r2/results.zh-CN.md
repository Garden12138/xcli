# R2 实施与验收记录 · 2026-10-17

**R2 开发及约定的真实场景验收已完成。** 当前源码为 `0.5.0-r2-dev`，基于 R1 提交 `7f817aa`；已更新本地 `./xcli`，未打 tag 或发布。用法见 [R2 说明](README.zh-CN.md)。

## 交付与兼容

- Work/Project × 指定单 CLI/自动模式共享后台 worker、同 Task continue、stop、累计预算和持久检查点；模式、候选范围与配置的登录入口不在恢复时漂移。
- 运行中补充进入持久队列，到检查点才应用并形成新计划。waiting_user 可补验证 argv；已完成节点与当前材料关联，重复 continue 不增加调用或重复产出。
- 单 owner OS 锁、启动时锁传递、独立 attempt 监督、owner 存活管道及 attempt 锁，避免旧执行尚未退出便启动新执行。不使用残留 PID 作为发信号依据。
- 检查点日志先同步，再原子更新 Task 快照；记录执行意图和有界结果。末尾未完成写入可在独占恢复时截断，完整损坏记录报错。用量按 Run ID 重建，中断的未知用量明确标记。
- 可信检查点可继续；不确定执行或材料变化进入 needs_reconcile，确认后重新核对材料和验收。优先同角色、同 cwd 的原生 session；明确失效时可材料恢复，认证/额度/网络错误不自动回退。
- Task 存储版本 2；版本 1 继续可读，旧 Task 需显式材料恢复。旧 jobs/runs、底层执行及 MCP launcher 兼容路径保留，不删除历史数据。原始输出默认仍关闭。

## 真实验收

真实工具为 macOS arm64 上的 Codex `0.163.0`、Cursor `2026.10.14-0f9edde`，沿用现有原生登录环境。只使用生成的临时材料和 Git 项目；Codex reasoning effort 用单次配置参数设为 low，Cursor trust 仅用于生成目录。未登录/退出、切换账号、复制凭据或修改长期规则。

| 场景 | 调用数 | 结果 |
| --- | --- | --- |
| Work 指定 Codex，后台 stop → 同 session 继续 | 4 | 通过；保留取消 attempt，原 session 成功恢复，产物、来源和当前自查通过 |
| Work 自动，Codex 产出 / Cursor 复核 | 3 | 通过；跨终端后台、独立复核、事件与材料一致 |
| Project 指定 Codex，后台失败 → 修复 | 5 | 通过；实际测试首次退出 1，一次修复后重验及自查通过 |
| Project 自动，worker 强杀 → 材料恢复 → 修复 / Cursor 复核 | 7 | 通过；先拒绝不完整计划并保留等待记录，补充后仍在同一 Task；失活进入 needs_reconcile，未确认 continue 被拒绝，确认后验证与复核通过 |

四条路径共 19 次调用，逐项证据见 [四路径报告](evidence/native-four-paths.json)。两条 Project 均未修改原仓库或受保护测试文件，首轮失败日志独立保留，最终复核绑定当前内容摘要。所有任务都使用 30 分钟累计执行预算及每任务最多 8 次调用；实际累计执行耗时约 56、76、97、199 秒，**不是持续执行满 30 分钟的耐久测试**。

本轮总计 **24/24 次 CLI 执行**：较早失败候选 3 次、缺 rollout 诊断 1 次、四条路径 19 次、后续二进制原生 resume 检查 1 次。完整 [调用账本](evidence/native-summary.json)包含时间和 Adapter，失败及本地拒绝的恢复调用同样计数。厂商内部模型/工具请求不可完整观测，不将这一计数当成内部请求总量。

较早候选过早停止 Codex 后，已有 session ID 但尚无可恢复 rollout；恢复失败保留在 [初始报告](evidence/native-initial-failed.json)，明确的原生错误见 [脱敏诊断](evidence/native-resume-diagnostic.json)。监督进程现在传递并私有保存 stderr，完整模拟链确认此类拒绝触发材料恢复，认证/网络失败不触发同样回退。真实正常 session 恢复和强杀后的显式材料恢复分别在最终场景通过。

真实四路径使用的候选摘要、后续真实 resume 二进制摘要，以及最终交付二进制摘要分别保留，没有将旧二进制证据改写为最终二进制验收。四路径之后的后台提交退出码、控制错误、继承 FD、失活 owner 显示和 stop 幂等修正由进程级回归覆盖；[后续原生检查](evidence/native-final-binary-audit.json)通过。最终 CGO=0 二进制对这四个持久 Task 的无新增调用继续、状态、材料及事件复查见 [交付复查](evidence/final-delivery-audit.json)；未超额重跑模型任务。

## 工程验证

最终检查与生产源码文件摘要见 [validation.json](evidence/validation.json)，四平台临时归档及独立 SHA-256 校验见 [构建记录](evidence/build-and-archive.json)。本机 `./xcli` 与最终 Darwin arm64 归档中的二进制完全一致。

| 检查 | 最终结果 |
| --- | --- |
| `go test -race ./...` | 完整回归通过 |
| R1 旧 Task → 显式材料恢复的定向 race 回归 | 通过 |
| `go vet ./...`、`go mod tidy -diff`、gofmt、`git diff --check` | 通过 |
| Darwin/Linux × amd64/arm64，CGO=0 构建 | 全部通过 |
| 四平台归档内容、执行权限、二进制与独立校验 | 全部通过 |
| GoReleaser snapshot | 本机未运行：工具未安装；原固定版本 CI 门禁保留 |

自动化覆盖四条模式的检查点后续跑、实际 argv/cwd、补充排队、调用上限追加、未知执行确认、原 session 恢复、失效后材料恢复、禁止错误回退、控制文件损坏、日志尾截断与完整行损坏、快照滞后、存储/事件失败、路径与原仓库保护、stop 幂等、后台立即 stop，以及控制进程 SIGKILL 后原生子进程退出。

两种模式均有模拟重启测试：保留文件、释放 OS 锁、留下失活记录与复用 PID，从可信检查点继续且不重复 worker。该测试验证重建语义，**没有实际重启这台电脑**。

## 范围

本轮真实 Task 支持证据限于上述 macOS arm64 Codex/Cursor 环境。其他 Adapter、其他系统实机、实际机器重启、无人值守开机自启及更长耐久运行未验证。跨平台构建不能替代目标机器的原生 CLI 实测。

Task 结果、验证通过和用户接收仍分别表达；不保证任意外部副作用幂等，不自动回滚、合并、发布或清理工作区。独立用户观察、对照试验和正式发布仍属 R3。
