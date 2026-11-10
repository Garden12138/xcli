# xcli

[English](README.md) | [简体中文](README.zh-CN.md)

xcli 是统一 AI Coding Agent CLI 调度与编排工具。

**各原生 CLI 自行安装、登录、维护账户与模型授权；xcli 只负责发现、选择、执行、编排、监控、恢复和交付任务结果。**

当前为未发布的 **R4 — CLI-First Product Realignment / `0.5.0-r4-dev`** 源码候选。明确支持 Codex CLI、Cursor CLI、Antigravity CLI，保留 Claude、Gemini、OpenCode 和 generic Adapter。已发布 v0.4.0 不含 Task；本轮不发布新包。见[定位与范围](docs/product-realignment/README.md)、[当前使用手册](docs/cli-first/XCLI_USER_MANUAL.md)和[历史实测支持范围](docs/r3/compatibility.zh-CN.md)。

## 发现与单次运行

先按各厂商原生方式安装并登录，再构建 xcli。以下示例的 `xcli` 指本轮构建的二进制；用户可自行放入 PATH，或使用完整路径。

```bash
go build -trimpath -o /tmp/xcli-r4 .
/tmp/xcli-r4 --version
/tmp/xcli-r4 agents --json
/tmp/xcli-r4 doctor codex --json

xcli run --agent codex --cwd ./project "说明这个项目的结构" --json
xcli run --agent cursor --cwd ./project "复核这次改动" --json
xcli run --agent antigravity --cwd ./project "分析这份材料" --json
```

`agents` 检查安装和版本，`doctor` 读取版本/help 与已声明能力；不据此推断账号、额度、工具权限或模型可用。原生登录、工作区信任和权限不足交由用户在原生 CLI 处理；xcli 不主动登录、切换账号、读取/复制/导出原生凭据或扩大长期权限。

## Session、Task 与后台恢复

```bash
xcli use codex --cwd ./project
xcli resume <run-id> "继续检查" --json
xcli resume <native-session-id> "继续" --agent cursor --cwd ./project --json

# Work：选定材料 → 文件产物 → 自查
xcli task "阅读材料生成 report.md 并核对来源" \
  --agent codex --input ./materials --output ./deliverables

# Project：干净且已有提交的本地仓库 → worktree → 实际验证 → 跨 CLI 复核
xcli task "修复问题并由另一个 CLI 复核" --project ./project \
  --verify '["go","test","-count=1","./..."]'

xcli task "整理材料形成报告" --agent antigravity \
  --input ./materials --output ./deliverables --detach --json
xcli task status <task-id> --json
xcli task status <task-id> --events --follow
xcli task stop <task-id> --json
xcli task continue <task-id> --detach --json

xcli runs list
xcli runs show <run-id>
xcli jobs list
xcli jobs logs <run-id> --follow
```

只有包含有效原生 Session ID 的记录可以按 Run ID 续接；交互 `use` 记录不保证包含该 ID。

`--agent` 将协调、执行、修复和自查锁到同一 CLI。不指定时，协调者在已安装且有结构化能力的候选中安排分工；跨 CLI 复核要求不同 Adapter，别名不算。应检查计划与复核记录；当前没有 `--worker/--reviewer` 固定角色参数。认证、额度或权限失败不会触发换账号/换 CLI 绕过限制。

Task 默认最多 6 次 Agent CLI 调用、累计执行 10 分钟、每次 120 秒及一次修复；分别通过 `--max-calls`、`--timeout`、`--call-timeout` 调整。继续不清零预算，不更换候选池和既定绑定。Session 丢失可从材料恢复，未知副作用先检查，再显式 `--reconcile`。详见[手册](docs/cli-first/XCLI_USER_MANUAL.md)与 [R2 生命周期](docs/r2/README.zh-CN.md)。

Agent 执行成功、文件验收通过和用户接收是不同状态。Task 保留真实文件、摘要、验证、交接与复核；Project 保留 worktree、diff 和未跟踪文件。图片可作为原生 CLI 的真实文件产物收集，但格式、来源和视觉接收仍需独立确认，不能把文本路径当图片。xcli 不提供图片模型服务，不自动提交、合并、部署或发布。

## 配置与数据

默认无需配置；可选 `~/.config/xcli/config.yaml`，支持 `XDG_CONFIG_HOME` 与 `--config`：

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

保留 `config path/init/validate`、旧配置字段和已有 Task/Run/Jobs、工作区、Session 与产物。旧 auth/routing/独立 workflow 管理命令不恢复，`mcp serve` 继续作为已有原生配置的兼容 launcher；不代管供应商登录。

私有数据位于 `${XDG_DATA_HOME:-$HOME/.local/share}/xcli`。原始输出默认关闭，但目标、选定材料、最终结果和必要验证诊断仍会保存。验证以 argv 执行受信项目代码；worktree 和原生权限标志不是通用 OS 沙箱。

## 开发验收

```bash
# 核心产品：CLI / Task / 配置 / 恢复，零模型调用
bash tests/product_realignment/verify-core.sh

# 保留源码的完整回归，额外覆盖冻结实验
bash tests/product_realignment/verify-offline.sh
```

核心 CI/打包使用 CLI/Task 门禁；冻结实验的回归单独手动触发。交叉构建不等于异平台原生实测，Windows 后台 Agent Task/jobs 仍不支持。R0–R3 的真实验证范围、权限/额度失败和协作收益未证实结论均保留。

Model Gateway、Native Agent Gateway HTTP、SIWC/OAuth/Responses 和通用 Media Gateway **冻结、默认关闭**，不再作为 xcli 产品目标或可用性前置门禁。代码、配置和证据保留，处置见[模块审查](docs/product-realignment/MODULE_DISPOSITION.md)。应用只按[明确授权的 Agent Task](docs/product-realignment/APPLICATION_INTEGRATION_GUIDE.md)设计接入，不把 CLI 订阅包装成通用模型 API。
