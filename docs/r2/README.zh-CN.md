# R2：后台与同 Task 继续

对应 `0.5.0-r2-dev` 源码构建；未打 tag 或发布安装包。R1 的 Work/Project、指定单 CLI/自动分工、真实验证和一次修复继续保留。真实范围与最终构建见 [验收记录](results.zh-CN.md)。

## 开始、观察与停止

```bash
xcli task "根据材料形成 report.md 并核对来源" \
  --agent codex --input ./materials --output ./deliverables --detach

xcli task "修复缓存失效问题，通过测试并由另一 CLI 复核" \
  --project . --timeout 30m --max-calls 8 --detach --json

xcli task status <task-id> --json
xcli task status <task-id> --events --after 0 --follow
xcli task stop <task-id> --json
xcli task continue <task-id> --detach --json
```

后台 worker 脱离调用终端，启动后返回 Task ID、PID 和工作位置。成功提交退出 0，最终结果由 status 查询；提交成功不等于任务完成。`--detach` 不与 `--events` 混用。前台进度写 stderr，`--json` 保持单个 Task 对象；`--events` 保持单调 JSONL。`status --follow` 需要 `--events`，取消观察不停止 worker。

stop 将请求写入私有控制文件，取消当前 attempt，确认进程退出后保存 `paused`；重复 stop 不启动或重跑工作。`stop --timeout` 默认等待 10 秒，超时返回错误并保留待处理请求，不凭旧 PID 强杀进程。Ctrl-C、SIGTERM 或 SIGHUP 取消前台任务，保留 `canceled` 与退出码 130；总执行时间耗尽为 124。

## 补充与预算

```bash
# 空闲时直接继续；运行中保存为尚未应用的补充，当前 attempt 结束后处理
xcli task continue <task-id> "保留原结论，再检查兼容问题" --detach

# waiting_user 缺少确定性验证时，在原 Task 内补充
xcli task continue <task-id> --verify '["go","test","-count=1","./..."]'

# 提高整个 Task 的上限，而不是新增一份额度
xcli task continue <task-id> --max-calls 10 --timeout 45m --detach
```

原始目标和每次用户补充分别保留。补充形成新计划，status 在应用之前显示 `pending_updates`；不能将排队当作已经插入原生执行。无补充的重复 continue 不生成第二个 owner。已完成 Task 无补充时返回原结果；新目标沿用同一 Task、原候选池和既有工作区重新规划。

默认整个 Task 最多 6 次 Agent CLI 调用、10 分钟执行时间及一次修复；继续不会重置这些计数。暂停、等待输入和机器停机不计入执行时间，失活前的心跳与已保存结果用于恢复可观察的累计耗时。预算参数只能维持或提高总上限，不能降到已用量以下。协调、执行、复核、修复和失败的恢复调用均计数；验证命令不计为 Agent 调用。原生内部请求和中断时未返回的用量可能不可观测，`usage_incomplete` 表示用量不完整。

`--agent`、模式、候选范围和原生登录入口约束跨 continue 保留，不提供恢复时换队或切换账号的参数。配置入口、相关网络配置或登录目录环境改变时拒绝继续，要求恢复原环境；xcli 不复制凭据，也不保证能识别用户在同一原生入口内手动切换了账号。

## 异常、检查点与材料恢复

```bash
# 先检查未知步骤、保留的目录和证据，再明确接受当前材料
xcli task status <task-id> --json
xcli task continue <task-id> --reconcile --fresh-session --detach
```

控制进程持有 OS 文件锁；残留锁文件或复用的 PID 不代表 owner 存活。原生命令由独立监督进程运行，owner 消失会关闭存活管道并取消原生进程组。上一次 attempt 尚未退出时拒绝新执行。status 在取得空闲 owner 锁后记录失活：可信检查点可进入 `paused`，状态不明的执行进入 `needs_reconcile`。

继续前重新检查 cwd、输入、产物、worktree 身份和 Git 基线。检查点保存下一节点与内容摘要，已确认产出不会因 continue 重跑。原生已结束而 Task 尚未推进时，可以核对持久结果和材料摘要恢复该节点；不能仅凭退出 0 判定 Task 完成。材料改变使旧验收失效并要求确认、重新规划和验证。工作区丢失、路径越界或原仓库保护失败不能通过 `--reconcile` 绕过。

优先复用相同角色、CLI、实际 cwd 与登录入口下的原生 session。明确的 session 不存在/过期错误允许改用 Task 简报、交接和产物新建 session；失败尝试保留并计数。认证、额度、网络或解析失败不触发这种自动回退。`--fresh-session` 显式选择材料恢复，不能绕过不确定副作用确认。确认后仍需验证当前文件；这不是任意外部操作的回滚或幂等保证。

R1 的存储版本 1 可以读取，但没有续跑位置；需要 `--reconcile --fresh-session` 从材料重新规划。未成功准备工作区的旧 Task 要新建任务。任务目录不得在原生进程运行中手工编辑或清理；损坏的完整日志记录明确报错，只有末尾未完成写入在独占恢复时截断。

## 持久化与边界

Task 存储版本为 2，保留 Task/Run/Session 身份分离。私有目录中新增检查点日志、原子 Task 快照、控制队列、心跳及有界 attempt 结果。结果包含必要的最终响应和 stderr 诊断，属于新增的持久化内容；不加入公开事件或自动广播给其他 CLI。目录 0700，xcli 创建的元数据文件 0600。原始流仍由 `recording.output` 控制，默认关闭。

保留旧 jobs/runs 入口和记录，不删除用户数据。后台运行限本机；本阶段不包含开机自启、跨机器调度、自动合并、任意循环或通用 OS 沙箱。模拟机器重启与真实重启不是同一验收；真实 30 分钟预算任务也不表示实际持续执行满 30 分钟。
