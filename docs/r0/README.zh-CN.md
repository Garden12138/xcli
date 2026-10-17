# R0：可靠底座与真实 CLI 接入

R1 源码已完成管理入口收口；本文描述 R0 兼容过渡行为，当前命令及迁移见 [R1 说明](../r1/README.zh-CN.md)。`mcp serve` 继续保留。

本页描述当前源码改动，最低要求为包含本次 R0 改动的源码构建；已发布的 v0.4.0 不包含这些行为。产品路线以[产品调整方案](../plans/product-realignment.zh-CN.md)为准，验收结果见[本批记录](results.zh-CN.md)。

真实项目中的操作、账号选择和权限失败追查见[用户体验记录](user-experience.zh-CN.md)。Antigravity 明确报告 headless 工具权限自动拒绝且没有回答时，即使原生退出 0，也保存为 failed、process_failed，xcli 返回 1；完整协议仍可保留 parse_status=complete。

Antigravity 原生 result 的 denied_actions 非空时，即使有回答且 status=SUCCESS，也按权限失败返回 1，保留原生退出码和已解析回答。图片生成/编辑已通过原生 image-generator 子代理实测，接受原生文件格式；视频、音乐生成尚缺当前环境可调用的接口，见[媒体验收](media-acceptance.zh-CN.md)。

## 运行、事件和恢复

```bash
xcli agents --json
xcli doctor codex --json
xcli run --agent codex "说明这个项目的结构"
xcli run --agent cursor "分析这份材料" --events
xcli run --agent antigravity "分析这份材料" --json
xcli resume <run-id> "继续" --json
xcli resume <workflow-run-id> "继续" --step implement --cwd ./project
```

Cursor 默认命令为 `cursor-agent`，Antigravity 为 `agy`；可通过 `agents.<name>.command` 覆盖。使用已有原生登录环境，不复制凭据、不切换账号。适配器没有自动添加 force、跳过权限、信任工作区或自动批准 MCP 参数。Antigravity 单次 print 显式设置 120 秒时限；原生权限等待、额度不足和认证失败均不能当作成功。

`--json` 输出一个最终对象。`--events` 与 `--json` 互斥，stdout 为 version 1 JSONL，最后一条 completed/failed 包含结果。run、带提示词的 resume 和 workflow run 支持事件；`--detach --events` 不支持。普通回答写 stdout，活动写 stderr；非 TTY 不刷新光标，无输出不推断为等待输入。workflow 事件带 step_id 和从 1 开始的 attempt，另有工作流整体终结事件。

结果增加 run_id、parse_status、error_kind 和脱敏 error。非法 JSON、明确的协议错误、缺少适配器要求的完成事件、资源截断均使执行失败。原生退出码保存在 exit_code，xcli 另行合成返回状态：解析失败为 1，取消为 130，超时为 124；原生非零退出优先于解析错误。没有启动进程时原生退出码记为 -1。workflow 顶层 exit_code 为合成结果，每步保存原生结果；jobs wait 使用同一规则。

单事件和最终回答各限 8 MiB，stderr 诊断尾部限 64 KiB，事件队列限 64 项。超大事件会被排空，后续 session/usage 仍尽量提取，但整体保持 truncated 和失败。慢消费者可能合并活动事件，完成、失败及最终用量保留。完整原始输出仍默认不保存；显式 recording.output 或 workflow --record-output 使用私有文件流式落盘。失败时文件可能不完整，必须结合记录状态查看。后台归一化日志维持原有保留行为。

workflow 新记录保存每步实际 cwd；恢复优先显式 --cwd，其次步骤 cwd，再次旧 workflow 顶层 cwd。旧步骤回退会提示；失效目录必须显式覆盖，缺 session 无法直接恢复。旧记录缺解析状态读作 unknown，不把历史 success 推导为验证通过。

doctor 区分安装、版本检测及能力来源。help 只证明暴露了相应参数，declared 是适配器声明，runtime 是 xcli 进程机制，unknown 表示未验证。登录、额度、只读强制和完整原生工具能力不由版本/help 推导；真实验证单独记入验收证据。没有指定 Agent 时，doctor 汇总所有已配置 Agent，任一安装或诊断失败返回非零。

## 隔离工作区底座

过渡期可通过现有 `workflow run <file> --worktree` 验证内部工作区能力，支持前台和 --detach。未新增独立 worktree 管理或 workflow plan 产品入口。

要求干净、已有提交的本地单仓库，隔离模式只允许 max_parallel=1。YAML 顶层相对 cwd 基于 YAML 文件目录，步骤相对 cwd 基于 workflow cwd。仓库内绝对目录映射到 worktree；仓库外目录、符号链接越界、目录失效拒绝启动，重试前再次检查。Prepare 和内部 preflight 共用解析结果。记录保存仓库、基线 commit、分支、worktree 和步骤实际目录。

显式 command 步骤以 argv 执行，不自动使用 shell，也不根据命令退出 0 声称“验证通过”。只有用户显式配置 `sh -c` 等解释器时才具有其语义。执行后保留工作区；不自动提交、合并或删除用户改动，不复制环境密钥或安装依赖。Git worktree 约束的是 xcli 分配的 cwd，不限制原生工具自行访问其他路径。

## 旧能力过渡和配置保护

R0 兼容保留 install、auth、default、acp、route、workflow、usage、MCP 管理入口；文档和适用入口提示其退役状态。ACP 与 `mcp serve` 的协议透传不插入额外诊断。后续 Task 替代能力就绪的 R1 发布负责收口独立管理入口；R0 是本路线最后的兼容过渡里程碑，具体发布版本号尚未分配。

MCP sync 对现有原生条目的更新、删除、强制接管一律拒绝。`--force` 不能绕过；新条目仅在权威配置可读且执行前证明同名条目不存在时允许新增。无法确认原生配置来源的覆盖参数拒绝新增。未变化的已托管条目可跳过，但明确说明完整 ownership 未验证。

import 仍可把可表示的原生配置导入 xcli 配置，不修改原生文件；扩展 targets 保留 env/args/env_vars/cwd，来源变化使旧计划失效。含无法转换的厂商字段条目拒绝导入。import 的 --force 仅处理可表示条目的源声明冲突，不允许原生条目替换。

不新增 ownership v3。共享状态写锁与保存前版本检查阻止 xcli 实例覆盖其他 namespace 的新状态；直接文件写入前重检原文件。锁只协调 xcli，不能对外部编辑提供跨原生 CLI 的原子事务。原生操作成功后 ownership 保存失败必须报告失败，后续运行保守报冲突。

已有原生配置引用 `xcli mcp serve` 时继续保留该 launcher。迁移步骤：使用原生 CLI 检查条目及未知字段，按原生文档将 launcher 替换为真实 server 命令、argv、工作目录与必要环境，验证连接后再停用对应 xcli 管理。环境变量名称与固定值分别处理，不将凭据贴进日志。launcher 的移除不绑定 R1；需另行确认迁移路径可用。配置、ownership、源文件和历史数据不会因退役被自动删除。

## 可重复验收

```bash
go test -race ./...
go vet ./...
go mod tidy -diff
go test ./internal/agent -run '^$' -bench BenchmarkDecoderOutput -benchtime=1x
go build -trimpath -o /tmp/xcli-r0 .
# 显式真实调用；仅默认原生登录入口，上限 6 次原生进程启动，每次 120 秒，无自动重试
python3 tests/r0/smoke.py --binary /tmp/xcli-r0 --execute --out /tmp/r0-native-smoke.json
```

smoke 的预算计数是原生 CLI 启动次数；厂商 CLI 内部可能有多个模型/工具往返，不能据此声称内部模型请求也不超过 6 次。

smoke 在临时 Git 项目中验证固定材料读取、同 session 记忆继续及有剩余额度时的取消；不会登录、切换账号、复制凭据或发布。Cursor 只读登录检查失败时跳过；Antigravity 没有使用未经验证的账号探测入口，其首次可调用性由有预算的 smoke 确认。模拟测试、真实调用和未验证路径分别登记；历史阶段 0 的证据与旧调用预算保持为历史记录。
