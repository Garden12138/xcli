# xcli v2 设计文档

xcli v2 是一个**本机多 CLI 编排引擎**：把 Codex CLI、Cursor CLI、Antigravity CLI 当作真实的"工人"，给你自己的桌面应用（GardenFlow、Worklog、Chat Insight、Contribution OS）提供任务级能力——问答/文本生成、编码、代码审查、生图/改图，以及根据需求自动编排多个 CLI 协作完成任务。

每次能力调用都是启动真实的 CLI 进程（非交互模式），用 CLI 自己的登录、模型和工具；xcli 不代理模型 API，不提供 chat/completions，不碰凭据。

## 一眼看懂

```bash
xcli ask   "总结 notes.md 的要点" -i notes.md --json            # 文本/分析，只读
xcli image "米白背景上的绿植插画，方形" -o ./covers --json      # 生图，返回真实图片文件
xcli code  "修复 TTL 到期判断的 Bug" -C ~/repo --verify "go test ./..." --json
                                                                # Codex 实现 → Cursor 审查 → Codex 修复
xcli do    "给 README 写一段介绍并配一张封面图" -C ~/repo        # 自动规划：多 CLI 并行/串行
echo '{"capability":"ask","goal":"..."}' | xcli submit --request - --json   # 应用接入方式
```

## 文档

| 文档 | 内容 | 读者 |
| --- | --- | --- |
| [用户手册](USER_GUIDE.md) | 安装、五种能力、后台任务、配置、排错与冒烟 | 使用者、应用开发 |
| [01 需求与现状评估](01-requirements-and-assessment.md) | 需求重述、成功标准、非目标、现状问题、模块去留 | 所有人 |
| [02 架构设计](02-architecture.md) | 模块、驱动层、编排、权限、生图、进程、存储、配置 | 实现者 |
| [03 接口规范](03-interfaces.md) | 命令行、请求/结果/事件 JSON 协议、HTTP、四个应用的接入示例 | 实现者、应用开发 |
| [04 实施计划](04-implementation-plan.md) | 分阶段任务、文件、接口签名、测试用例、验收命令 | Codex |
| [05 接入应用统计](05-app-stats.md) | `xcli stats`：统计账本、调用方标识、统计口径、实施任务（v2.1.0） | 实现者、Codex |
| [驱动实测记录](driver-notes.md) | 三个 CLI 的版本、参数、事件、权限与产物行为 | 实现者 |
| [Codex 交接说明](CODEX_HANDOFF.md) | 直接粘贴给 Codex 的提示词与守则 | 你 |

真实冒烟入口是 `scripts/smoke.sh`；运行前会提示额度消耗并等待回车。

## 版本与分支

- 旧成果（R0–R4.1、Model Gateway、Agent Gateway、SIWC 等）完整保留在 tag `archive/r4.1-final`。
- v2 当前版本为 `2.0.1`，任务 JSON 协议版本为 `1`。从 v0.x 升级时请按 [接口规范](03-interfaces.md) 接入。
