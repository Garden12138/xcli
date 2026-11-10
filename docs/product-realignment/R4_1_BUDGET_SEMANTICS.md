# R4.1 启动预算语义

## 单位与窗口

预算单位是 **xcli 直接启动一次原生 CLI 入口可执行文件**，不是任务命令数、模型请求、Token、费用或供应商 wrapper 内部进程数。Codex、Cursor、Antigravity、其他已配置 Adapter 均遵循同一规则。版本、help、只读登录状态与立即失败的进程同样是一次；纯 LookPath/文件发现为零。普通 xcli worker、git 和确定性验证工具不属于这个单位。

启动必须指定不可变窗口。窗口 limit 包含 `included` 外部基线和该窗口 xcli 保守扣额；剩余量最小为 0。没有自动无限窗口，没有缺省外部计数 0，不把旧 Task.Calls 当作进程数。耗尽直接失败，不自动排队/重试/跨 CLI fallback。同一持久 store 内已绑定 Task、Run/Session 续接拒绝切换窗口；新窗口只用于新的明确授权任务，不能给原任务重置额度。不同 store/未观察过的原生 Session 不能做全局关联，因此操作者须持续使用同一 store，不得将路径替换当作绕过手段。

## 外部控制器

`external` 保存 controller agent、count（可 null）、source、user_confirmed、scope。单个 baseline 是窗口内所有外部启动的汇总，来源应引用逐项审计证据；多种控制器的清单放入该证据。`included` 只有来源、Agent、明确计数、用户确认都存在才可启动；有 Codex 外部控制器时必须纳入，而不是让开发中的 Codex 隐身。未知次数保持 null，整窗 remaining=null、status=UNKNOWN，拒绝 dispatch。

`excluded` 只适用于用户明确批准的独立新作用域：仍须 Agent、来源与用户确认；次数可保留 null，不被改成 0。它不适用于旧 R4“包含当前控制器”的授权。xcli 不会自行排除控制器，也无法验证用户声明是否覆盖全部历史。预算窗口建立后不得再有未计账外部启动；新外部启动需停止验收、重新审计/授权新范围，不能修改原窗基线或从原窗推导余额。并发上限只监督 xcli 受控的 active/unknown 记录；不能保证外部控制器的 OS 并发，严格全局并发验收须使用独立、不产生日后原生启动的控制入口，并由用户确认范围。

旧 R4 `ae0b4be` 始终 UNKNOWN，不从祖先 PID 或“本轮新增 0”推断累计数，更不推断还剩 5 次。`record-legacy` 只记录未知历史来源，不建立可用余额。

## 原子性、消耗与恢复

| 持久状态 | 启动事实 | budget_consumed | 并发占位 | 恢复/释放 |
| --- | --- | --- | --- | --- |
| reserved | 尚未写入 Start 意图 | 1（先保守预留） | 1 | owner lease 丢失且仍在此状态，协议证明从未执行 Start，可释放为 not_started。 |
| starting | 意图已落盘，OS Start 结果未持久确认 | 1 | 1 | 崩溃后 unknown；即使怀疑未启动也不退额。 |
| started | OS Start 成功，PID 已记录 | 1 | 1 | owner 崩溃后终止未确认，unknown 并保留确认已启动的事实。 |
| finished | 已确认启动并退出/取消/超时 | 1 | 0 | 立即退出、非零退出、取消和超时均不退款。 |
| not_started | 明确 OS Start 失败且无 Process，或启动前已取消/恢复可证明未执行 Start | 0，released=1 | 0 | 保留 Launch ID 与原因，不删除历史预留记录。 |
| unknown | 启动或停止不确定 | 1 | 1 | 不根据旧 PID 存在/不存在自动杀进程或退额，避免 PID 复用误判。 |
| unknown_stopped | 用户确认明确停止证据 | 1 | 0 | 只清并发占位，启动事实仍 unknown/started，消耗不变。 |

每次 Reserve 都 `reserved=1`；任一记录 `consumed+released=1`。统计 `xcli_charged` 包含预留与 unknown，并非声称全部确认已启动；另有 `confirmed_started`、`unknown_starts`、`released` 和 `active_or_unconfirmed_stopped`。`start_state` 与生命周期 state 分开：启动已确认后崩溃，不能改写为从未启动。未知结束时间/退出码保持 null，不伪造完成。

全局 OS 锁内读取、检查窗口/幂等/额度/并发、分配唯一 ID、持有 owner lease、写入预留，一个事务完成。所有 Task/Agent 竞争同一窗口。`Start` 只能发生于持久意图成功后。替换或 fsync 错误即停止；若错误发生于可能启动之后，保持扣额/未知。owner lease 随进程退出由操作系统释放，重启后的查询/Reserve 会保守恢复。

ID 不因 retry、Session resume 或新鲜 Session 重建而复用。每次重试单独启动、单独预留；额度失败不能被自动 fallback 掩盖。幂等 key 重放返回已有 ID 而非启动第二次，已证明未启动的原 key 也不能原地重启；显式新尝试需新 key/ID。

## 新的真实验收门槛

1. 用户重新授权全新窗口的总启动 limit、并发与时间限制、探针是否包含、外部控制器范围；不继承 R4 UNKNOWN 余额。
2. 使用独立控制入口，或先提供完整且经用户确认的外部启动来源/次数。无法确认就保持 UNKNOWN。示例中的 `none/0` 只有确无 scoped 外部启动时才成立。
3. 使用同一私有本机 store 建窗；先查询 budget。预检每个原生探针也会花额度：若只授权 6 次且要做三组首次+续接，应另行授权探针额度或使用明确标注时效的已有文件证据，不执行未计账探针。
4. 只在隔离合成 Git 项目测试，串行、单次 120 秒、不重试、不跨 CLI fallback。Task 的内部发现/版本/阶段全算入同一窗口；顶层一次 task 不等于一次原生启动。
5. 每次后导出 budget/list/show 及 Run/Session/事件证据。UNKNOWN/权限不足/额度耗尽立即停止。内部模型请求与费用仍报告不可精确观测。

本轮没有建立真实验收授权窗口，没有运行本节的真实任务，也不自动进入 R5。

## 已执行的 Fake 统计示例

本轮黑盒夹具 limit=2，首次 run 和同 Session resume 分别各占一次，第三次被拒绝。查询结果（节选）：

```json
{"status":"EXHAUSTED","xcli_charged":2,"confirmed_started":2,"unknown_starts":0,"released":0,"active_or_unconfirmed_stopped":0,"remaining":0,"legacy_history":"UNKNOWN"}
```

对 `record-legacy` 的查询保持 `status=UNKNOWN`、`external.count=null`、`remaining=null`。这两个示例没有记录真实供应商启动，也不提供旧 R4 的可用余额。完整证据见 [blackbox.json](evidence/r4_1/blackbox.json)。
