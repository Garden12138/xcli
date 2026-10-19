# R3 Cursor 额度恢复后补验 · 2026-10-19

结论：**Cursor 真实调用及 Codex 执行 / Cursor 复核的后台中断、材料恢复验收均通过。** 与[此前指定 Codex 的通过记录](followup-20261019.zh-CN.md)合并，两个后台验收缺口已补齐；独立用户观察和协作收益门槛仍未满足，R3 尚未完整退出。

## 冻结范围与预算

用户明确确认 Cursor 额度恢复后，本轮仅重测 Cursor 可用性和跨 CLI 后台恢复，不重复单 CLI 恢复，不覆盖旧额度失败与旧 Task。新增 `--cross-only --carry-budget` 入口，准备时校验旧预算未变，新账本逐字继承此前五条分派，继续受同一个 **12 次**补充预算总上限约束。新批次预算、祖先账本或继承前缀变化都会阻止新分派。

生产源码基线为 `f2bc6d3afafbda578b9b60fd24603eac7ac8e933`，其 [CI 已通过](https://github.com/Garden12138/xcli/actions/runs/39192177982)。在工作区干净时构建二进制 `0.5.0-r3-dev-f2bc6d3`，SHA-256 为 `06e5fcffe3931b931d538dc0dac032378e5d6b97ca54f222ff17ea1332063bd1`；本轮不修改生产 Task 行为。执行平台仍为 macOS arm64，Codex `0.163.0`、Cursor `2026.10.16-e3cb207`。工具先经过 19 项模拟回归；预算保护进一步收紧后，针对继承、跳过单 CLI 和预算篡改的集成回归再次通过。模拟结果不计作真实 CLI 证据。

## 真实结果

| 检查 | 结果 | 本轮调用 |
| --- | --- | --- |
| Cursor 可用性 | 真实只读请求成功，输出 R3_READY，原生流解析完整；约 30.768 秒 | 1 |
| Codex 执行 / Cursor 复核后台恢复 | 通过，同一 Task 完成实际 worker 中断、新 session 材料恢复与当前摘要的 Cursor 复核；约 89.377 秒 | 4 |

Task 为 `task-20261019T032056Z-39d25912`，没有操作员协调澄清。实际角色与状态为 Codex coordinator success、Codex worker canceled、另一个 Codex worker success、Cursor reviewer success。取消前观察到运行中的 worker 已写进度标记，stop 返回 paused 且 owner 不再活动；材料继续后的 worker 原生 session 与被取消 session 不同。

Task 身份、自动模式、候选池、配置绑定和累计预算保持一致；中断 attempt 保留，事件序号连续，输入未变，报告包含 North: 12、South: 9、Total: 21 及所选输入的来源。最终复核为 `cross_cli`，passed 且绑定当前产物摘要；完成后再次 continue 没有新增调用。只读 inspect 与原始补验报告完全相同，另行核对了实际 worker/reviewer 的 CLI。

见[脱敏原生报告与验后核对](evidence/cursor-restored-20261019.json)。本轮新增 **5 次**，补充预算累计 **10/12**，剩余两次未使用；原始 46/48 账本和旧补充 5/12 账本的摘要保持不变，新账本前五条与旧补充账本逐字一致。历史合计 **56 次原生分派**，不是为本轮重新分配了 12 次。取消及此前额度失败均保留计数；厂商内部请求数与完整成本仍未知。

## R3 剩余门槛

- 两个后台恢复技术缺口已关闭，其他平台、角色、真实重启和满三十分钟运行仍按[支持矩阵](compatibility.zh-CN.md)保留未验证范围。
- 仍未收到独立参与者的实际记录；本轮由维护者脚本执行，不能代填[三位用户观察表](observation.zh-CN.md)。
- 两个预定义 Work/Project 对照仍未建立协作收益。本轮恢复成功验证可靠性，不证明更高质量、更少人工交接、更快或更省。
- 正式发布仍待上述证据和候选评审；没有创建 tag 或发布安装包。
