# A02：兼容契约决策（冻结于 2026-09-08）

本文件约束后续实施，不改变当前生产行为。`tests/phase0/testdata/contracts.json` 中 old 是现有格式，new 是目标格式；检查只验证样例与旧读取能力，不假称新行为已经实现。

## 结果、身份与退出码

现有 RunResult 为 agent/output/session_id/exit_code/status/usage；Record 保留 id、resumed_from、resumed_step。workflow Execution 现有 id、name、cwd、steps 和汇总 exit_code 继续保留。

新增字段均为可选：最终结果 run_id；结果、记录与步骤的 parse_status、error_kind、error；StepRecord 的 cwd。workflow 顶层 exit_code 仍是合成值，原生退出码保存在对应步骤；新增 run_id 与现有 workflow id 相同。Job 的初始响应不伪造最终结果、解析状态或完成时间。

`parse_status`：complete（按该适配器规则完整结束）、parse_error（非法协议、显式失败事件或缺完成信号）、truncated（解析资源限制使完整性丢失）、not_applicable（generic 文本、原生交互透传）、unknown（旧记录或尚未获得结论）。未知合法事件可以忽略；非法 JSON 不能以最后一行文本当作完整成功。F05 修复可保留缓冲，但提高 Scanner 上限不能代替错误检查。

`error_kind` 固定为 start_failed、canceled、timeout、process_failed、parse_error、truncated；成功时省略。error 是可读的脱敏诊断，协议错误只记录适配器、事件/行号、字节位置及原因，不附原始事件正文。已采集 session/usage 尽量保留，parse_status 标示整体完整性，缺费用仍为缺失。

| 判定顺序 | status | error_kind | 保存的原生 exit_code | xcli 退出 |
| --- | --- | --- | --- | --- |
| 进程未启动 | failed | start_failed | 新记录 -1（不存在原生退出）；旧记录中的 1 不追溯修改 | 1 |
| 明确取消 | canceled | canceled | 保留进程结果（信号终止可为 -1） | 130 |
| 明确超时且无外部取消 | timed_out | timeout | 保留进程结果 | 124 |
| 原生非零退出 | failed | process_failed | 保留原值 | 正整数原值；无法映射的负值用 1 |
| 原生退出 0、解析失败/截断 | failed | parse_error / truncated | 0 | 1 |
| 原生退出 0、完整或不适用解析 | success | 省略 | 0 | 0 |

状态合成用于 run/resume/workflow/后台；jobs wait 应返回相同的 xcli 任务退出状态，不能因记录原生 exit_code=0 而把解析失败返回 0。等待者超时仍不终止后台任务。当前 runtime 在取消/超时路径写的是合成 130/124；阶段 1 应保留等待子进程获得的原生结果，再单独合成 CLI 退出状态，不把这两个数字描述成已观测到的原生退出码。timeout 同时设置内部 Canceled 的现有实现也不能当作外部取消证据。阶段 1 统一合成函数及路径测试；不把新字段硬塞入旧函数后继续沿用按原生码判断的逻辑。

旧记录缺 parse_status 读作 unknown；历史 success 不推出验证 passed 或用户 accepted。验证、接收及 attempt 明细在后续阶段定义接口，本阶段只冻结这一不变量。

## stdout / stderr 与事件

普通 `--json` 严格输出一个最终 JSON 对象，不夹活动文本；错误诊断写 stderr。`--detach --json` 仍输出初始 Job；`runs show` 仍默认 JSON。已有字段不重命名，新增字段不要求旧消费者识别。

阶段 2 新增 `--events`，与 `--json` 互斥：显式选择后 stdout 为 JSONL，最后一条 completed/failed 含规范化 result。事件公共字段为 version:1、run_id、time（RFC3339 UTC）、type；适用时附 step_id、attempt（从 1 开始）。type 为 started/progress/completed/failed/waiting；progress 使用 message，waiting 仅在原生协议有明确信号时发出。没有活动不能被猜成等待输入。后台 worker 使用同一事件语义，但 `--detach --events` 首版拒绝，避免把初始 Job 误当订阅。

普通文本模式最终回答写 stdout，活动和任务摘要写 stderr；非 TTY 不用光标刷新。交互 use/ACP 保留原生透传。事件样例固定协议版本和终结事件形状，流式实现及资源限制属于 C02/C03。

## 实际 cwd 与恢复

workflow 顶层相对 cwd 基于 YAML 所在目录；步骤相对 cwd 基于 workflow 实际 cwd。不改普通非隔离模式既有绝对路径语义。新步骤在准备/启动及每次重试时持久化最终实际 cwd，前后台一致。

恢复优先级：显式 --cwd → 所选步骤保存的 cwd → 旧 workflow 顶层 cwd；普通旧 run 用 record.cwd。旧 workflow 回退提示写 stderr，并指明目录来源，--json 仍只有一个对象。目录不存在/非目录则报错，要求 --cwd，不回退调用者当前目录；缺 session_id 则明确不可直接恢复。未来 --last 不自动选择 cwd 不确定的旧 workflow 步骤。

同一 session 只能由其原生 Agent 恢复。跨 Agent 只交接用户选择的摘要、diff 与验证证据，不承诺 session 移植。worktree 路径映射由 B06 实施，必须在 plan 与启动时使用一致结果，原仓库绝对 cwd 映射入 worktree，默认拒绝仓库外及符号链接越界路径。

## MCP ownership v3

当前 state v2 的 fingerprint 是 xcli Entry 可编辑字段的语义指纹，不能证明原生完整条目未改变。v3 保留它，新增 native_fingerprint（SHA-256 小写十六进制），版本覆盖整个 state；不改变 namespace/source_config 的含义。

native_fingerprint 对完整原生条目规范化后计算：包含未知字段与嵌套字段、保留值类型和数组顺序，忽略对象键顺序、空白和注释。规范化输入是解析后的 JSON/JSONC/TOML 条目而非原文件文本；无法无损取得完整条目时拒绝自动更新/删除。B02 应为每种 manager 固定规范化样本，不使用部分 CLI 查询冒充权威快照。

读取 v1/v2 时仅迁移结构，不自动填写 native_fingerprint。缺保护指纹的现有托管条目产生 conflict，即使语义一致也不显示已受完整保护的 noop。只有用户用现有 sync --force 显式接管后，才能根据重读的完整原生条目建立指纹；不能为了迁移删除未知字段。源声明已移除的旧条目同样阻止隐式删除；force 删除遵循原有显式接管语义。

执行时加 xcli 写锁，重读原生条目和 ownership，校验计划前置条件；完整条目未变才更新/删除。写入、读回确认后保存对应 ownership。原生已写入但 state 保存失败必须返回失败，重跑保守报冲突/待恢复；不承诺跨 CLI 原子事务。新增 v3 状态旧二进制会拒绝读取，因此写 v3 前保留私有 v1/v2 备份；降级只能在停止新同步并核对原生配置后人工恢复，不能直接修改 version 数字。

样例里的指纹是明确标记的合成值，不是权威原生快照的哈希；用于固定字段与迁移状态，不作为哈希算法回归数据。
