# 01 需求与现状评估

## 1. 需求重述

| 编号 | 需求 | 具体含义 |
| --- | --- | --- |
| R1 | 能力整合 | 把 Codex CLI、Cursor CLI、Antigravity CLI 的编码、文本生成/分析、图片理解、生图/改图能力，通过统一的任务接口提供给你自己的桌面应用（GardenFlow、Worklog、Chat Insight、Contribution OS）。 |
| R2 | 真实使用 CLI | 每次能力调用都启动真实 CLI 进程（`codex exec`、`cursor-agent -p`、`agy --print`），使用 CLI 自己的登录、模型、工具和额度。不做 HTTP 模型反代、不提供 chat/completions、不代管或提取凭据。 |
| R3 | 智能编排 | 根据输入需求自动选择合适的 CLI、拆分步骤、互不依赖的步骤并行、代码改动由另一个 CLI 交叉审查并迭代修复、某个 CLI 不可用时自动切换，让任务完成得更好。 |
| R4 | 简单好用 | 装好并登录三个 CLI 后零配置可用；一条命令或一个 JSON 请求完成一件事；默认值合理，不需要预先建窗口、预算或授权回执。 |

已确认的决定：

- 首批调用方是自己的四个桌面应用（Electron / Tauri-Rust / Python），主要接入方式是**子进程 + JSON 协议**；本机 HTTP 任务接口作为最后阶段的可选项，主要给运行在 Docker 里的 Contribution OS 使用。
- 默认权限：**工作区内可写、可执行命令**；规划和审查类步骤自动降为只读。
- 在 `v2` 分支重建，旧成果打 tag `archive/r4.1-final` 归档。

## 2. 成功标准

1. 三个 CLI 已安装并登录后，不创建任何 xcli 配置文件，`xcli ask "你好" --json` 首次运行即成功并返回结果 JSON。
2. 应用只需一次子进程调用（stdin 写请求 JSON，stdout 读结果 JSON）即可拿到：文本、符合 Schema 的 JSON、经过校验的图片文件、代码改动与验证结果。
3. `xcli code ... --verify ...` 默认完成"实现 → 验证 → 另一个 CLI 审查 → 修复"的闭环，结果里能看到每一轮的验证和审查结论。
4. `xcli do "<复合需求>"` 由 CLI 生成多步计划（可能涉及多个 CLI、并行步骤），执行并汇总结果；`xcli plan` 可只看计划不执行。
5. 某个 CLI 未登录或额度耗尽时，任务自动换到另一个可用 CLI 继续，结果中记录切换原因。
6. 用户手册一页讲清楚日常用法；非测试 Go 代码目标不超过 6000 行（超过 8000 行必须停下来复盘设计）。

## 3. 非目标

- 不做 OpenAI 兼容接口、chat/completions、逐 token 流式、Embedding（Chat Insight 的向量能力继续用它现有的服务）。
- 不做 OAuth、订阅共享、账号池、凭据管理、额度查询。
- 不做启动窗口、启动账本、预算预留、外部启动计数之类的门禁。
- 不做操作系统级沙箱；隔离只依赖各 CLI 自带的沙箱与 git worktree。
- 不自动 commit / push / merge / 发布。
- v2.0 只支持 macOS 和 Linux；Windows 不支持。
- 第一期不做 MCP Server 和 Agent Skill（调用方是自有应用，不需要）；引擎接口稳定后可以很便宜地加上。

## 4. 现状评估

### 4.1 规模

当前分支 `feature/upgrade-chatgpt` 相对 v0.4.0：797 个文件变化、新增约 8.8 万行，其中 Go 代码新增约 2.9 万行；`docs/` 下有 481 个文件。非测试 Go 代码约 2.4 万行，分布在 30 个包里，其中约一半属于已冻结的实验模块。

### 4.2 需求对照

| 需求 | 当前实现 | 差距 |
| --- | --- | --- |
| R1 给应用调用 | 唯一对外服务是 Model Gateway（OpenAI 兼容 HTTP），本质是反代，已冻结；Native Agent Gateway HTTP 已冻结；`xcli mcp serve` 只是帮原生 CLI 启动 MCP 子进程 | **没有可用的应用接入接口**。四个应用的接入文档全部是 Planned/Blocked |
| R2 真实 CLI | `run`/`task` 确实调用真实 CLI，三个适配器的参数拼装与事件解析可用 | 可用，但被启动账本门禁包住；Antigravity 被硬编码 `--print-timeout 120s`；Cursor 不传 `--trust` 导致新目录必然失败（历史实测 BLOCKED） |
| R3 智能编排 | Task 固定线性流水线：规划 → 实现 → 验证 → 复核 → 最多一次修复；协调者 = 候选列表第一个；跨 CLI 只能靠在提示词里写"要求 Cursor 复核" | 没有按能力路由（生图/编码/审查）、没有并行、没有失败切换；默认最多 6 次调用、每次 2 分钟，复杂任务必然超时 |
| R4 易用 | 首次使用要先 `launches window create --limit --concurrency --external-agent --external-count --external-scope --external-source --confirm-external`；连 `--version` 探测都消耗启动额度；状态里有 `needs_reconcile`、`--reconcile --fresh-session` 等概念 | 学习成本极高；验证命令禁止 shell、禁止 git、限 120 秒；Project 模式要求仓库干净且已提交 |

### 4.3 难用的根因

1. **目标漂移**：从"统一调度 CLI"漂到"模型服务化网关"（M0–M5），再漂到"原生 Agent 网关 + 隔离 + OAuth"（G1–G8），再回到"CLI 编排"（R4）。每次转向都保留了上一阶段的代码、配置兼容和证据文档。
2. **安全与审计压过可用性**：每个环节都加了门禁、回执、证据和否定式说明（"不代表…"、"不能推导…"），却没有一条顺畅的主路径。
3. **编排抽象错位**：Task Runner 是为"可审计的单次交付"设计的状态机，不是按需求组合多个 CLI 的编排器。

### 4.4 模块去留

| 模块 | 处置 | 说明 |
| --- | --- | --- |
| `internal/agent`（`agent.go`、`stream.go`、`usage.go`） | **移植改写** | 三个 CLI 的参数拼装、事件解析、额度超限/重连/`denied_actions` 识别是踩坑经验，改写进新的 `internal/driver` |
| `internal/worktree` | **移植精简** | worktree 创建/清理逻辑改写进 `internal/workspace` |
| `internal/runtime/process*.go` | **移植精简** | 进程组启动、SIGTERM→SIGKILL 取消改写进 `internal/proc` |
| `internal/config` | 重写 | 新配置结构更小，不兼容旧字段 |
| `internal/task`、`internal/workflow`、`internal/jobs`、`internal/runstore` | 删除 | 由新的 `orchestrator` + `engine` + `store` 取代 |
| `internal/launch`、`internal/localstate` | 删除 | 启动账本与窗口机制整体取消 |
| `internal/mcp` | 删除 | 旧 MCP 启动器与新需求无关 |
| `model*`、`siwc`、`agent*`（`agentapi/agentstore/agentservice/agentpolicy/agentartifact/agentgateway`）、`nativeadapter`、`isolatedruntime`、`codexrpc`、`media` | 删除 | 冻结实验；源码保留在 tag `archive/r4.1-final` |
| `docs/`（除 `docs/v2/`）、`tests/`、`examples/`、`xcli-cli-first-acceptance/` | 删除 | 历史证据保留在 tag；v2 只维护 `docs/v2/` |
| `.github`、`.goreleaser.yaml`、`LICENSE`、`main.go`、`go.mod` | 保留并更新 | goreleaser 去掉 Windows 目标 |

需要旧代码时用 `git show archive/r4.1-final:internal/agent/stream.go` 查看。

## 5. 结论

需要重新设计。保留三个 CLI 的适配经验（约 3～4 千行可借鉴代码），其余按 [02 架构设计](02-architecture.md) 重建：

- 驱动层把"只读 / 可写 / 完全"三档权限映射到各 CLI 自己的参数，并统一解析事件流。
- 编排层用一个小 DAG 执行器覆盖单步、串行交接、并行、"实现-验证-审查-修复"循环和失败切换；`do` 请求由 CLI 自己按 JSON Schema 产出计划。
- 接入层以子进程 JSON 协议为主，HTTP 为辅。
