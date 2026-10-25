# Model Gateway 开发进度

基线：`8833d241c03c4a5040297f06cfa88540511dcce6`；2026-10-24；macOS arm64，Go 1.26.4。初始工作区干净。完整阅读 UPGRADE_PLAN.md；审查 README、R0–R3、配置、命令、Agent/Task/Runtime、MCP 与现有测试。仓库及父目录无 AGENTS.md。

## 状态与恢复入口

整体：M0 → M5 的本仓库网关工程实现、离线测试与本机生命周期验收已交付，**整体升级验收仍未完成**。真实模型/外部应用/异平台实机Gate开放。真实付费调用、用户登录/凭据变更、外部发布均未执行。测试只在临时目录生成合成Token。外部项目不在本工作区写权限内，已交付接入清单、Worklog可应用补丁与可执行验收，不宣称正式联调完成。

| 阶段 | 工程状态 | 验收状态 |
| --- | --- | --- |
| M0 | 已实现 | 离线 Gate 通过，见 acceptance/M0.md |
| M1 | 已实现，离线工程检查通过 | 真实 Worklog 调用未验证；外部写权限/计费授权待具备 |
| M2 | 已实现，离线工程检查通过 | 真实 Vision/生成/编辑未验证，未调用付费上游 |
| M3 | 已实现，离线工程检查通过 | Chat Insight应用内隐私修复与真实检索未验证 |
| M4 | 已交付协议验收与外部接入清单/补丁 | 四应用端到端未验证，外部写权限/发布门禁待满足 |
| M5 | Model Gateway工程与本机离线生命周期通过 | 其余实机/休眠恢复/真实部署Gate未验证；Windows Agent后台明确不支持 |

恢复：先读本文件、`git status --short`、`SUMMARY.md` 和 `api-contract.md`。已实现代码不重做；下一步只补下方真实/外部Gate。每阶段均有独立本地Git检查点，不覆盖用户改动。

## 实现决策与差异

- 新增独立 `internal/model`、`modelbackend`、`modelgateway`、`modelstore`；注册 `cmd/model.go`。现有 AgentAdapter、Task、Run、Session 不充当模型后端；不读原生 CLI 登录凭据。
- 复用 Cobra、yaml.v3 与标准 context/http；现有 stdio MCP、CLI JSONL 不能复用为 OpenAI 协议。独立 model-gateway.yaml 防止改变旧配置语义。
- 现有构建仅 Darwin/Linux；M5 需补 Windows 进程/锁实现及构建。交叉构建不等于实机通过。
- 强能力需可追踪验证声明，未验证图片/工具能力拒绝路由；不自动 fallback、不重试可能收费的请求。

## 检查记录

- 初始 `go test ./...` 在沙箱内被 Go 构建缓存权限阻止；同一命令沙箱外运行全通过。该问题不是源码测试失败。
- M0：`go test -race ./...` 全通过（Task 177.372s）；`go vet ./...`、`go mod tidy -diff`、`git diff --check` 通过；`go build -trimpath -o /tmp/xcli-model-m0 .` 与 model help 通过。
- M1：完整 `go test -race ./...`、Vet、构建通过；最终新增 CLI/Consent 变更经针对 model*/cmd 的 Race 契约复验通过。覆盖实时 HTTP SSE、取消传播、截断、401/403/429/5xx、并发、超时、请求预算、Token 轮换/撤销/并发写、私有凭据及 CLI 隔离。见 acceptance/M1.md。
- M2：完整 Race、Vet、mod tidy -diff、构建、diff check通过；最后补充的Vision成功转发/多图/非法上传再经Race通过。新增固定Go官方x/image v0.47.0用于WebP，不使用外部图像进程或真实图片模型。见acceptance/M2.md。
- M3：完整Race、Vet、构建、diff check通过，Python参考客户端4项隐私/索引策略测试通过。HTTP假上游验证remote/unknown拒绝授权时调用次数不增、local无云头可用；批量index/维度/base64/有限值均校验。见acceptance/M3.md。
- M4：完整Race、Vet、构建、diff check与Python策略测试通过；真实本地HTTP + Python四消费画像、JSON/Tool流、显式供应商切换/no fallback通过。Worklog补丁对外部当前源码git apply --check通过，但未应用。见acceptance/M4.md。
- M5：完整Race通过（Runtime变更后Task包160.232s）；最后服务事件审计经cmd/model*/四画像Race复验通过。Vet、mod tidy -diff、gofmt/diff check通过；Darwin arm64/amd64、Linux arm64/amd64、Windows amd64交叉构建通过。实际macOS arm64最终二进制生命周期6类检查通过；R3确定性25项、安装兼容12项通过。见acceptance/M5.md与offline-20261025.json。

## 本地恢复检查点

- M0：e8e5bcc
- M1：049a459
- M2：dc52dd2
- M3：573a012
- M4：c7b3f0b
- M5：本文件所在最终M5提交（`git log -1 --oneline`）；没有push/tag/release。

## 下一步

1. 获得四个外部仓库的写权限后，按integrations/逐一实施并运行其原生测试；Worklog补丁先复核Git状态和source drift。Chat Insight隐私修复未部署前禁止正式云路由；Contribution OS容器Bridge未实现前保持阻断。
2. 由用户选定真实上游/模型、显式API Key引用及合成请求预算，再执行smoke --execute；本轮没有代选/读取用户真实凭据。实际模型效果/JSON Schema/Tools/Vision/生成/编辑/向量验收仍空白。
3. 在Windows x64、Linux x64、macOS x64等目标实机运行native网关与生命周期检查；Windows ACL/Job Object仅编译，未实测；Agent后台Task/jobs在Windows明确拒绝。不能把新增CI定义当已执行成功。
4. 补OS休眠/恢复、重启/可选启动项、真实升级/回退及并发峰值内存/延迟基线；不执行未经授权的系统权限扩展或不可逆迁移。

全部真实Gate满足后才可改为整体完成。CLI会员登录桥接、图片URL下载、Responses/音视频、任意LAN/Host Bridge不在当前已支持声明中。
