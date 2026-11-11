# xcli 本地体验交付

当前为 **`0.5.0-r4.1-dev`**：未发布的 CLI-first 开发版本。xcli 统一调度 Codex CLI、Cursor CLI、Antigravity CLI，负责 Run、Task、Session、恢复、监控和文件结果；各 CLI 自管安装、登录、账号、权限、模型和额度。

**第一次使用请直接看 [QUICK_START.md](QUICK_START.md)。** 本仓库已经生成 `bin/xcli`，可直接 `./bin/xcli --version`；加入 PATH 的步骤见安装说明。不需要学习或启动 Gateway/OAuth。

## 本次交付结果

| 关注点 | 结果与入口 |
| --- | --- |
| 相比主线 | 本地 main 原已位于完整 R4.1 HEAD `665ec15`，本次新增整理/文档与一处测试同步修复；相比 origin/main 快照含 21 个既有本地升级提交 |
| 相比稳定 Tag | 最近稳定发布 v0.4.0 不含 Task；当前新增 R0–R4 的 Cursor/agy、流诊断、Task 文件/Project 编排、后台/checkpoint 恢复及 R4.1 启动窗口等。详见 [VERSION_COMPARISON](VERSION_COMPARISON.md) |
| 自动验收 | 完整 Go 普通/race 各 560 通过、1 可选容器跳过；Python 54 项、安装12检查、5目标交叉构建通过。原核心门禁及最终核验见 [AUTOMATED_ACCEPTANCE](AUTOMATED_ACCEPTANCE.md) |
| 本地安装 | [LOCAL_INSTALLATION](LOCAL_INSTALLATION.md)：Go 1.26.0+、构建、个人 PATH、原生 CLI 发现/自管登录、配置、首次启动窗口 |
| 第一个任务 | [QUICK_START](QUICK_START.md)：独立小项目 → 用户主动调用自己的 Codex → 查看 Run/Session；可替换 Cursor/Antigravity |
| 三 CLI 与长任务 | [USER_MANUAL](USER_MANUAL.md)：分析/复核、单 CLI 修 Bug、多 CLI 分工、后台 Task、停止/继续、媒体文件与排障 |
| 文件清理 | [CLEANUP_REPORT](CLEANUP_REPORT.md)：删除55个可再生成文件，保留真实数据、历史证据和冻结代码 |
| Git 状态 | 整合并推送 feature/upgrade-chatgpt，保留 main、4个原Tag及原有远程分支；v.0.5.0 有独有提交受保护，无可删的其他本地开发分支。见 [BRANCH_CONSOLIDATION](BRANCH_CONSOLIDATION.md) |
| 最终完整 SHA | 运行 `git rev-parse feature/upgrade-chatgpt`；本机可查提交后生成且忽略于 Git 的 `FINAL_STATE.json`，该收据不上传，完整 SHA 同时在最终交付消息提供 |

## 最短构建入口

在当前本地完整仓库根目录：

```bash
git switch feature/upgrade-chatgpt
go build -trimpath -o bin/xcli .
./bin/xcli --version
./bin/xcli --help
```

要执行原生任务或 agents/doctor 探针，继续按快速开始设置**本次真实声明的启动窗口**；版本/help 也消耗启动次数。窗口计进程入口，不是模型调用/费用。不要凭历史 UNKNOWN 余额启动，也不要删除/换账本给原任务清零。

## 使用限制与真实验证边界

- 本轮新增真实模型调用和原生 CLI 启动均为 **0**；三 CLI 仅做 PATH/文件发现。真实账号、额度、Workspace Trust、网络和模型能力须用户在原生工具确认。Fake PASS 不等于真实推理 PASS。
- 仓库保留 10-15 三 CLI 图片和 R1–R3 Codex/Cursor Task/恢复的真实历史，11-06 Codex/agy 文本、Session/图片通过及 Cursor Trust 受阻；R4 真实验收的 NEEDS_FIXES/UNKNOWN 结论保持不变。
- macOS arm64 有本轮离线运行证据；Intel Mac/Linux 仅编译，Windows 后台 Task/Jobs 不支持。没有固定 worker/reviewer 参数、无限任务或自动 commit/merge/deploy。
- Model Gateway、Native Agent Gateway、SIWC/OAuth/Responses 是冻结实验，默认关闭。本轮保留源码、配置兼容和历史材料，未继续实验或启动服务。处置见 [MODULE_DISPOSITION](../product-realignment/MODULE_DISPOSITION.md)。
- 用户后续已授权提交推送及清理无用分支；完整成果推送到 feature/upgrade-chatgpt。联网复核没有可安全删除的分支，v.0.5.0 的独有提交保留；main、Tag 及历史未改。新 clone 应显式选择 feature/upgrade-chatgpt，默认 main 仍为旧版。

离线结果支持开始本地体验；尚不构成本轮真实模型验收或正式稳定发布。完成本次交付后停止，不进入 R5。
