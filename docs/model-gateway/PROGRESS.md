# Model Gateway 开发进度

基线：`8833d241c03c4a5040297f06cfa88540511dcce6`；2026-10-24；macOS arm64，Go 1.26.4。初始工作区干净。完整阅读 UPGRADE_PLAN.md；审查 README、R0–R3、配置、命令、Agent/Task/Runtime、MCP 与现有测试。仓库及父目录无 AGENTS.md。

## 状态与恢复入口

整体：开发中，**未完成升级验收**。按 M0 → M5 实现。真实付费调用、登录/凭据变更、外部发布均未执行。外部项目不在本工作区写权限内，只能先交付接入清单与可执行验收，不能宣称完成联调。

| 阶段 | 工程状态 | 验收状态 |
| --- | --- | --- |
| M0 | 已实现 | 离线 Gate 通过，见 acceptance/M0.md |
| M1 | 实现中 | 真实 Worklog 调用未验证 |
| M2 | 未开始 | 真实 Vision/生成/编辑未验证 |
| M3 | 未开始 | Chat Insight 隐私修复与真实检索未验证 |
| M4 | 未开始 | 四应用端到端未验证 |
| M5 | 未开始 | 跨平台实机与休眠恢复未验证 |

恢复：先读本文件、`git status --short` 和 `api-contract.md`，继续当前阶段，不重做已通过检查。每阶段保留新增文件/本地 Git 检查点，不覆盖用户改动。

## 实现决策与差异

- 新增独立 `internal/model`、`modelbackend`、`modelgateway`、`modelstore`；注册 `cmd/model.go`。现有 AgentAdapter、Task、Run、Session 不充当模型后端；不读原生 CLI 登录凭据。
- 复用 Cobra、yaml.v3 与标准 context/http；现有 stdio MCP、CLI JSONL 不能复用为 OpenAI 协议。独立 model-gateway.yaml 防止改变旧配置语义。
- 现有构建仅 Darwin/Linux；M5 需补 Windows 进程/锁实现及构建。交叉构建不等于实机通过。
- 强能力需可追踪验证声明，未验证图片/工具能力拒绝路由；不自动 fallback、不重试可能收费的请求。

## 检查记录

- 初始 `go test ./...` 在沙箱内被 Go 构建缓存权限阻止；同一命令沙箱外运行全通过。该问题不是源码测试失败。
- M0：`go test -race ./...` 全通过（Task 177.372s）；`go vet ./...`、`go mod tidy -diff`、`git diff --check` 通过；`go build -trimpath -o /tmp/xcli-model-m0 .` 与 model help 通过。

## 下一步

继续 M1：实现标准 OpenAI-compatible backend、文本/SSE、独立 App Token、并发/超时/取消。Worklog 源码只读复核已确认 provider_defaults 白名单仍缺 xcli、请求路径仍为 base_url/chat/completions；需要交付接入补丁/测试，真实 App 联调等待外部仓库写权限及付费请求授权。
