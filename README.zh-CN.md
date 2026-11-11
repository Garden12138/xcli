# xcli

[English](README.md)

**统一 AI Coding Agent CLI 调度与编排工具。**

当前源码版本 **`0.5.0-r4.1-dev`**，尚未发布。Codex CLI、Cursor CLI、Antigravity CLI 各自维护安装、登录、账号、权限、模型和额度；xcli 负责发现、执行、Task 编排、Session、恢复、监控和结果交付。

**从 [本次统一交付入口](docs/release-handoff/README.md) 开始。** 已发布 v0.4.0 和未推送的远程 main 快照不含本次完整成果。

| 目的 | 文档 |
| --- | --- |
| 安装并尝试第一个任务 | [快速开始](docs/release-handoff/QUICK_START.md) |
| macOS Apple Silicon / Intel / Linux 部署 | [本地安装](docs/release-handoff/LOCAL_INSTALLATION.md) |
| 三 CLI、Task、Session、Jobs、媒体和排障 | [用户手册](docs/release-handoff/USER_MANUAL.md) |
| 相比主分支和稳定 Tag 的变化 | [版本差异](docs/release-handoff/VERSION_COMPARISON.md) |
| 测试结果及未验证项 | [自动验收](docs/release-handoff/AUTOMATED_ACCEPTANCE.md) |
| 清理和最终 Git 状态 | [清理报告](docs/release-handoff/CLEANUP_REPORT.md)、[分支整合](docs/release-handoff/BRANCH_CONSOLIDATION.md) |

当前本地源码构建：

```bash
go build -trimpath -o bin/xcli .
./bin/xcli --version
./bin/xcli --help
```

R4.1 执行及版本/help 探针要求用户明确的进程启动窗口，按快速开始建立自己的作用域；旧 UNKNOWN 历史不能视为可用余额。不需要启动守护服务。日常入口为 agents/doctor/run/use/resume/task/runs/jobs/config/launches 和兼容 mcp serve。

Task 支持 Work/Project、实际验证、交接、一次有界修复、后台和 checkpoint 恢复；--agent 锁定所有角色，自动跨 CLI 计划没有固定 worker/reviewer 参数。worktree 不是 OS 沙箱，不自动提交、合并、部署或发布。

核心开发验收：

```bash
bash tests/product_realignment/verify-core.sh
go test ./...
go test -race ./...
go vet ./...
```

测试使用 Fake CLI；HTTP 单元夹具需要本机回环权限。交叉构建不等于异设备实测，Windows 后台 Task/Jobs 暂不支持。本轮模型可用性与历史真实验收分开报告。

Model Gateway、Native Agent Gateway HTTP、SIWC/OAuth/Responses、通用 Media Gateway **冻结、默认关闭**，不属于使用前提。源码及证据保留，见 [模块处置](docs/product-realignment/MODULE_DISPOSITION.md)。旧全量 offline 脚本含显式启动合成实验服务，本次未执行这些服务步骤。

R0–R4 历史材料保留在原目录；当前用户说明统一维护在 `docs/release-handoff/`。
