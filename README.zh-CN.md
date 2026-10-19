# xcli

[English](README.md) | [简体中文](README.zh-CN.md)

把 Work 或 Project 目标交给原生 Agent CLI，在有界执行、可追溯交接和真实验证后交付结果。

当前源码为 **R3 / `0.5.0-r3-dev` 候选版**，尚未正式发布。已发布的 v0.4.0 不包含 Task。实验、安装和阶段边界见 [R3 使用说明](docs/r3/README.zh-CN.md)与[候选验收记录](docs/r3/results.zh-CN.md)；后台恢复见 [R2 说明](docs/r2/README.zh-CN.md)，升级见[迁移说明](docs/r3/release-notes.md)。

## 快速开始

先按原生工具的方式安装并登录，xcli 复用已有认证、工具和 MCP 配置，不要求另开模型 API 账号。

```bash
go build -o xcli .
./xcli agents
./xcli doctor codex

# Work：指定单 CLI，产出文件并自查
./xcli task "读取材料，生成 report.md，覆盖关键结论并列出来源" \
  --input ./materials --output ./deliverables --agent codex

# Work：自动组织，另一 CLI 复核
./xcli task "整理材料形成报告，并由另一个 CLI 核对结论" \
  --input ./materials --output ./deliverables

# Project：从干净 Git 基线建立受管理 worktree
./xcli task "修复缓存失效问题并通过项目测试" --project . --agent codex

# 显式 argv 覆盖自动发现的验证命令，可重复传入
./xcli task "修复问题并由另一个 CLI 审查" --project . \
  --verify '["go","test","-count=1","./..."]' --json

# 后台执行、跨终端观察和同 Task 继续
./xcli task "继续完成项目并验证" --project . --detach
./xcli task status <task-id> --json
./xcli task status <task-id> --events --follow
./xcli task stop <task-id>
./xcli task continue <task-id> --detach
```

`--agent` 是整个任务的执行约束：协调、执行、修复和自查都使用指定 CLI，失败不会静默换工具。自动模式按配置默认偏好及稳定候选顺序过滤可用能力，由协调 CLI 提出分工。明确要求跨 CLI 复核时必须使用不同 Adapter，同 Adapter 的两个别名不算跨 CLI。

默认累计执行时间 10 分钟（不计暂停/等待）、最多 6 次 Agent CLI 调用、每次最多 120 秒，允许一次修复后重验。用 `--timeout`、`--max-calls` 修改总预算。原生内部模型轮次和子 Agent 用量可能不可观测，进程调用次数不能当成模型请求数。

## 结果与验收

Task 保存独立任务 ID、关联 Run/原生 Session、计划版本、交接包、产物摘要、真实命令验证和复核。运行成功、验收通过与用户接收分别表达；Agent 自称完成和原生退出 0 不能替代实际文件或测试证据。

Project 保留隔离 worktree、基线 diff、未跟踪文件及验证证据。协调者可以从项目配置发现本地测试，运行时检查 argv 和目录后直接执行；未知入口通过 `--verify` 显式提供。没有真实验证则进入 `waiting_user`。测试修改源码会使本轮证据过期，复核使用固定材料副本。

项目测试执行用户信任的代码。argv 检查、Git worktree 和原生权限参数均不等于通用操作系统沙箱。任务不会自动安装工具、修改长期权限或外部发布。失败后保留工作区与材料；检查完整 diff 和未跟踪文件，再通过正常 Git 流程人工接收。

进度写 stderr，最终摘要写 stdout。`--json` 输出一个 Task 对象；`--events` 输出带单调序号和最终结果的 JSONL，两者互斥。`task status` 检查 owner 与检查点，发现失活时保守更新状态。

Task 支持后台、stop/continue、检查点和失活恢复。Ctrl-C 取消当前执行并保留材料；`waiting_user` 可通过原 Task 的 continue 补充条件。continue 保留模式、候选池和累计预算；未知副作用进入 `needs_reconcile`，经检查后用 `--reconcile` 确认恢复。原生 session 不可用时可从材料新建 session。

## 配置与 Adapter

内置默认配置可以直接使用。可选配置文件为 `~/.config/xcli/config.yaml`，支持 `XDG_CONFIG_HOME` 和 `--config`：

```yaml
version: 1
default_agent: codex
agents:
  codex:
    adapter: codex
    command: codex
  cursor:
    adapter: cursor
    command: cursor-agent
  antigravity:
    adapter: antigravity
    command: agy
recording:
  output: false
```

现有 Adapter 包括 Codex、Cursor、Antigravity、Claude Code、Gemini、OpenCode；generic 别名参与 Task 需显式配置结构化输出。工具可发现、账号可用、额度可用和权限生效分别判断，实际支持范围以验收记录为准。Task 对 Codex 映射 read-only/workspace-write，对 Cursor 映射 ask/sandbox enabled，对 Antigravity 映射 plan/accept-edits；不自动信任新工作区。

保留 `config path/init/validate`、`agents`、`doctor`。Agent 的 `args`、`env`、网络和原生认证由用户配置。doctor 区分安装、版本和帮助能力，不把未知登录/执行策略写成通过。

## 私有数据与底层命令

Task 目标、输入/产物摘要、计划、交接、固定复核材料和有界验证诊断保存在 XDG 私有数据目录，默认 `~/.local/share/xcli`，目录 0700、文件 0600。原始 Agent 输出由 `recording.output` 控制且默认关闭；验证诊断是修复所需的私有材料。凭据和任意原始工具载荷不自动加入交接包。

```bash
./xcli run "解释这个项目" --agent codex --cwd . --json
./xcli use codex
./xcli resume <run-id或原生session> "继续检查兼容性"
./xcli runs list
./xcli runs show <run-id>
./xcli jobs list
```

保留 `run/use/resume`、`jobs`、`runs` 和发现/配置入口；底层 run 使用显式 Agent 或 `default_agent`，不再使用正则提示词路由。

R1 移除 `install`、`auth`、`default`、`acp`、`route`、独立 `workflow`、`usage` 和 MCP import/plan/sync。R0 提交 `5a2a676` 是最后保留这些入口的源码里程碑。替代路径为原生安装/登录、Task、配置和 Task/Run 用量；不删除旧配置、ownership、历史 Run 或原生文件。**`xcli mcp serve` 继续兼容旧 launcher 引用**，替换前按 [迁移说明](docs/r0/README.zh-CN.md)操作。

## 开发与验证

要求 Go 1.26。发布门禁包含格式、模块、vet、race、Darwin/Linux × amd64/arm64 构建；CI 另运行 GoReleaser snapshot。

```bash
go test -race ./...
go vet ./...
go mod tidy -diff

# R3 确定性夹具，不调用模型
XCLI_R3_BINARY=./xcli python3 -m unittest discover -s tests/r3 -v

# 显式真实对照：整批累计最多 48 次原生调用
python3 tests/r3/experiment.py prepare --study /tmp/r3-study --binary ./xcli --budget 48
python3 tests/r3/experiment.py run --study /tmp/r3-study --out /tmp/r3-comparison.json --execute
```

真实脚本沿用已有 Codex/Cursor 登录环境，创建临时项目/数据目录，只对生成的目录设置 Cursor trust。原始失败证据保留在私有目录；公开报告按 [R3 导出流程](docs/r3/README.zh-CN.md)脱敏。独立用户观察和正式发布与工程候选验收分别判断，历史 R0–R2 证据保留原范围。
