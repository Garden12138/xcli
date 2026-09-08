# A01：固定基线与任务归属

核对日期：2026-09-08。仅使用本地 Git 引用，未 fetch，也未声称远端没有更新。当前分支为 main；开工时 README.zh-CN.md 已修改，docs/plans 与 docs/reviews 未跟踪，这些内容保留。验证使用 git archive 导出的固定源码，原工作区不切换分支。

| 引用 | 完整源码 commit | 归属 |
| --- | --- | --- |
| main / origin/main | feae6070dfa63049cad51e9f37511650914618c3 | 稳定修复起点 |
| v0.4.0 | 70d8644f9daeb7f19d29c40a16a41bfcf1de7cfb | 最新本地发布 tag；与 main 仅手工测试指南不同 |
| origin/v.0.5.0 | 6d59f7fa2b3e83a89e9450927b507320f44264f7 | 未发布开发快照 |
| v0.3.0 | 0ba098a688bf691d5323cee188bcde9b6fb5d024 | 历史影响范围 |
| v0.2.0 | ee98e9322e640d4eb0ed4b981c3f6c8e4a3d3b43 | 历史影响范围 |
| v0.1.0 | a15a42ae20704d9c745b0d09ea901b9817e326ad | 历史影响范围 |

以下责任人为本轮实施者；后续接手者沿用 ID 更新状态与证据。历史版本影响依据原审查，本阶段重新复现的是两个固定分支快照。

| ID / 优先级 | 影响范围 | 最小复现入口 | 目标模块 | 修复任务 / 依赖 | 目标版本 |
| --- | --- | --- | --- | --- | --- |
| F01 / P1 | 仅开发线 worktree | workflow run --worktree --json，步骤 cwd 指向源仓库绝对路径；检查 escape-marker | cmd/workflow、internal/workflow、internal/worktree | B06；B03 与开发线 worktree 基础 | v0.5.0 |
| F02 / P1 | v0.3、v0.4、两个分支 | project Claude MCP sync → 原生加入 headers → plan → 删除声明 → sync | internal/mcp 的 manager、project_manager、engine、types、state | B02；A02、B01 | v0.4.1，并同步开发线 |
| F03 / P1 | v0.3、v0.4、两个分支 | Gemini wrapper import apply，源声明 targets=[claude] 且 env 非空 | internal/mcp/import_engine、import_apply | B01；A01 | v0.4.1，并同步开发线 |
| F04 / P1 | v0.3、v0.4、两个分支 | workflow step cwd=sub → resume ID --step task continue --json | internal/runstore、internal/workflow、cmd/workflow、cmd/resume | B03；A02 | v0.4.1，并同步开发线 |
| F05 / P1 | 四个 tag、两个分支 | run codex fixture --json；session → 5 MiB 单行 → usage | internal/agent、internal/runtime、cmd/run_exec、cmd/resume、internal/workflow | B04；A02 | v0.4.1，并同步开发线 |
| F06 / P2 | 四个 tag；并行示例始于 v0.2 | 从仓库根与其他目录执行两份原始 examples | examples、README、手工指南 | B05；A01 | v0.4.1，并同步开发线 |
| F07 / P2 | 四个 tag、两个分支 | 模拟 --version 退出 9，doctor codex，文本与 JSON | cmd/agents_doctor、internal/agent | B05；A01 | v0.4.1，并同步开发线 |

完整可执行命令保存在模拟结果的 commands 数组；具体夹具逻辑见 tests/phase0/cases.go。F01 在稳定线无入口，明确不执行。这里的最小复现不替代阶段 1 的扩展回归矩阵（前后台、重试、未知字段嵌套、符号链接、非法流、超时等）。

## 开发线接入清单

开发线相对 main 仅一个新增提交 6d59f7f，涉及 46 个文件，新增 2,984 行、删除 158 行。后续按以下功能拆分，不把整个提交当成验收单元。

| 功能组 | 已有内容与主要模块 | 后续接入条件 |
| --- | --- | --- |
| worktree | internal/worktree、cmd/worktrees；记录 workspace_mode/repository/worktree_path/worktree_branch/base_commit | 先 B03/B06，统一 cwd 映射，再 D01 集成 |
| command / plan | workflow command 步骤、Prepare/plan、verified-workflow 示例 | 保留依赖调度语义，D02 加验证证据；不能把退出 0 当测试通过 |
| 记录筛选 | runstore/filter、runs/usage 参数、Agent 版本字段 | C04/C05 接入；先核对字段兼容，仍缺 StepRecord.cwd |
| 诊断 | doctor deep、ACP 命令检查、MCP in_sync | B05 修版本错误退出；静态能力不等于实测能力；F02 未解决 |
| 分发 | release notes、CI、GoReleaser 的增量 | B07 重新核对同提交发布门禁；不随补丁版引入未发布能力 |

默认只维护稳定线和下一版本开发线；v0.1–v0.3 保留问题说明与升级路径。v0.4.1/v0.5.0 是当前本地 tag 核对后的规划版本，发布前再核对远端 tag。
