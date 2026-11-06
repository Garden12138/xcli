# xcli CLI-first 第一阶段验收包

目标：真实验证 **Codex CLI / Cursor CLI / Antigravity CLI → xcli** 的能力，以及是否存在可供桌面应用使用的官方支持集成接口。**不使用 Ollama / 假上游 / 普通 API 供应商来冒充 CLI 订阅模型服务。**

## 使用（macOS，xcli 仓库根目录）

先把本文件夹解压到 xcli 仓库根目录。检测不会调用外部模型：

```bash
bash xcli-cli-first-acceptance/verify-phase1.sh
```

更完整的离线竞态测试：

```bash
bash xcli-cli-first-acceptance/verify-phase1.sh --race
```

准备好使用本机已有原生 CLI 登录、允许最多 6 次原生 CLI **进程**调用时，再明确选择：

```bash
bash xcli-cli-first-acceptance/verify-phase1.sh --native-smoke
```

**提示**：一个 CLI 进程可能启动多个模型轮次或工具调用，6 次进程并不保证仅消耗 6 个模型请求。脚本不登录、不切换账号、不复制原生凭据、不自动重试。真实图片生成/编辑需要额外明确同意，并由 Codex 按主任务组织和实测；脚本自身不做生图。

若希望 Codex 全程负责：打开 xcli 仓库，把 `CODEX_PROMPT.md` 的内容作为 Codex 主任务。Codex 应先跑离线项、记录真实验收并生成 `docs/cli-first/` 手册和四应用集成文档，真实 CLI 调用要按预算授权，未完成的 Gate 不可标通过。

## 这套测试能证明什么

- 默认：源码编译、部分测试、CLI 安装与命令帮助、旧 Gateway 生命周期（离线）。
- `--native-smoke`：原生登录环境下 CLI headless / resume / 部分取消情况（只读合成材料）。
- **不能仅靠这套脚本证明** `/v1/chat/completions` 已使用原生 CLI 会员能力：当前仓库此类 Gateway backend 尚未实现，必须先通过独立设计/合法性审查/代码实现/真实 API 验收。

## 结果与安全

验收输出位于系统临时私有目录 `xcli-cli-phase1.*`，不会自动提交。只有脱敏的 PASS/FAIL/BLOCKED 汇总可以公开。`docs/r0/media-acceptance.zh-CN.md` 中的历史图片验收只作为回归参考，不作为本轮成功声明。

远端源码主分支和用户本机四应用的未提交适配代码可能不同；本轮不修改四个应用仓库。若后续产品采用官方 Agent SDK/app-server，而不是模拟通用 OpenAI Chat Completion 接口，也属于需要明确评估的正确方向。
