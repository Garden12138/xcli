# Model Gateway 开发进度

基线：`8833d241c03c4a5040297f06cfa88540511dcce6`；2026-10-24；macOS arm64，Go 1.26.4。初始工作区干净。完整阅读 UPGRADE_PLAN.md；审查 README、R0–R3、配置、命令、Agent/Task/Runtime、MCP 与现有测试。仓库及父目录无 AGENTS.md。

## 状态与恢复入口

整体：M0 → M5本仓库网关工程、四个外部应用的接入源码、原生客户端离线HTTP联通与本机性能基线均已交付，**完整真实验收仍未完成**。Worklog、Chat Insight、GardenFlow、Contribution OS四仓库均已具备接入代码，既有用户改动已保留。

**2026-10-27交接：后续由用户按[真实验收步骤](REAL_ACCEPTANCE.md)跟进，使用[记录模板](acceptance/real-acceptance-record.template.md)逐项留证。** 用户已授权本仓库集中提交、推送；外部四仓库本轮不代为提交/推送，不创建tag/Release。桥接仍只是默认关闭的隔离补丁，实际应用UI、真实模型、生产桥接和异平台Gate保持开放。

2026-10-26最新结果：GardenFlow 780项/两类TypeScript检查/生产构建通过，旧基线回归已消除；Worklog、Chat Insight可执行文件编译通过；Contribution OS全量1403项通过/45跳过，更新的四个用户源码模块再经504项通过/28跳过。四应用真实原生客户端代码经临时xcli和假上游完成10次合成模型请求，拒绝未经确认的云上传与未批准的向量上传，真实模型请求0次。详见acceptance/native-consumers-20261026.json。

**Contribution OS历史部署与后续修复**：用户明确确认后，早前200文件镜像曾部署至六服务并通过当时的健康检查；随后发现快照遗漏运行依赖，应用仓库ENV-R08已恢复完整202文件运行时并增加构建时导入检查。旧镜像及旧rollout/rollback只保留历史证据，不得用于覆盖该修复；没有把历史健康观测当作当前持续健康保证。最新桥接源码与回归见下方记录，生产启用、真实模型/预算及异机验收移交用户。

| 阶段 | 工程状态 | 尚未满足的验收 |
| --- | --- | --- |
| M0 | 骨架、契约及离线Gate通过 | 无新增工程缺口 |
| M1 | 文本网关、Worklog源码/原生HTTP报告/模板/日报JSON联通通过 | 真实模型与实际桌面UI流程 |
| M2 | Vision与图片API已实现；GardenFlow原生客户端生成/编辑HTTP联通通过 | 真实Vision/图片效果、媒体入库与Pi工具UI流程 |
| M3 | Chat Insight隐私与向量身份源码/原生SSE与Embedding联通通过 | 真实索引/检索与桌面确认流程；未部署前不启用正式云路由 |
| M4 | 四仓库接入源码已写回；Contribution OS冻结Profile/短期Token/JSON Schema原生联通通过；认证桥接隔离补丁合成验收通过 | 桥接生产启用、四应用真实UI/模型验收 |
| M5 | macOS本机生命周期、Linux arm64隔离容器生命周期、延迟/RSS基线通过 | Windows/Linux x64/macOS x64实机、休眠/重启/真实升级回退；Windows Agent后台仍不支持 |

恢复：先读本文件、`git status --short`、`SUMMARY.md` 和 `api-contract.md`。已实现代码不重做；下一步只补下方真实/外部Gate。每阶段均有独立本地Git检查点，不覆盖用户改动。

## 实现决策与差异

- 新增独立 `internal/model`、`modelbackend`、`modelgateway`、`modelstore`；注册 `cmd/model.go`。现有 AgentAdapter、Task、Run、Session 不充当模型后端；不读原生 CLI 登录凭据。
- 复用 Cobra、yaml.v3 与标准 context/http；现有 stdio MCP、CLI JSONL 不能复用为 OpenAI 协议。独立 model-gateway.yaml 防止改变旧配置语义。
- 初始构建仅 Darwin/Linux；M5 已补 Windows 进程/锁实现及构建。交叉构建不等于实机通过。
- 强能力需可追踪验证声明，未验证图片/工具能力拒绝路由；不自动 fallback、不重试可能收费的请求。

## 检查记录

- 初始 `go test ./...` 在沙箱内被 Go 构建缓存权限阻止；同一命令沙箱外运行全通过。该问题不是源码测试失败。
- M0：`go test -race ./...` 全通过（Task 177.372s）；`go vet ./...`、`go mod tidy -diff`、`git diff --check` 通过；`go build -trimpath -o /tmp/xcli-model-m0 .` 与 model help 通过。
- M1：完整 `go test -race ./...`、Vet、构建通过；最终新增 CLI/Consent 变更经针对 model*/cmd 的 Race 契约复验通过。覆盖实时 HTTP SSE、取消传播、截断、401/403/429/5xx、并发、超时、请求预算、Token 轮换/撤销/并发写、私有凭据及 CLI 隔离。见 acceptance/M1.md。
- M2：完整 Race、Vet、mod tidy -diff、构建、diff check通过；最后补充的Vision成功转发/多图/非法上传再经Race通过。新增固定Go官方x/image v0.47.0用于WebP，不使用外部图像进程或真实图片模型。见acceptance/M2.md。
- M3：完整Race、Vet、构建、diff check通过，Python参考客户端4项隐私/索引策略测试通过。HTTP假上游验证remote/unknown拒绝授权时调用次数不增、local无云头可用；批量index/维度/base64/有限值均校验。见acceptance/M3.md。
- M4：完整Race、Vet、构建、diff check与Python策略测试通过；真实本地HTTP + Python四消费画像、JSON/Tool流、显式供应商切换/no fallback通过。Worklog补丁对外部当前源码git apply --check通过，但未应用。见acceptance/M4.md。
- M5：完整Race通过（Runtime变更后Task包160.232s）；最后服务事件审计经cmd/model*/四画像Race复验通过。Vet、mod tidy -diff、gofmt/diff check通过；Darwin arm64/amd64、Linux arm64/amd64、Windows amd64交叉构建通过。实际macOS arm64最终二进制生命周期6类检查通过；R3确定性25项、安装兼容12项通过。见acceptance/M5.md与offline-20261025.json。
- 2026-10-25接入续轮：Worklog Rust40项和前端build通过；Chat Insight Rust96项通过/2项真实模型评估忽略、React56项/build/lint通过。GardenFlow早期快照763项及两类类型检查通过；保留新用户改动后，当前快照762/763通过、xcli相关26/26通过，导航测试及三处类型错误均在未加网关改动的基线复现。Contribution OS相关56项通过/1项真实Docker验收跳过，Python编译/JS语法通过。补丁工具8项测试通过，四补丁在独立Git副本中通过受保护apply及逐字节结果核对。见acceptance/integrations-20261025.md。

- 2026-10-26补充：四原生客户端经已构建网关的离线联通通过；Worklog/Chat Insight可执行构建、GardenFlow 780项及生产构建通过；Contribution OS全套1403/45跳过、增量504/28跳过。独立Linux arm64容器6类生命周期通过，不能替代目标物理机。单客户端小文本p95 3.915ms，三客户端8.003ms；含1024x1024合成PNG负载采样峰值RSS 104,992KiB。均为假上游基线，不是模型速度承诺。证据见acceptance/closeout-20261026.md。

## 本地恢复检查点

- M0：e8e5bcc
- M1：049a459
- M2：dc52dd2
- M3：573a012
- M4：c7b3f0b
- M5：20269e4；后续提交交付接入补丁、离线证据与真实验收交接，不改变真实Gate状态。

## 用户后续跟进

按[真实验收步骤](REAL_ACCEPTANCE.md)顺序执行；本节保留恢复背景，不依赖下列临时目录仍然存在。

四仓库源码均已写回，不重复apply完整provider补丁。Worklog/Chat Insight恢复副本为`/tmp/xcli-gateway-integration-<repo>`；GardenFlow/Contribution OS最新副本为`/tmp/xcli-gateway-integration-<repo>-finish`。GardenFlow后续用户提交将HEAD推进到7539d51（接入补丁的source_head仍是应用前基线）；保留补丁作为审查/恢复记录，不能据旧HEAD强行重放。补丁侧车与源码摘要工具仍用于防漂移，不覆盖用户工作。

1. Contribution OS早前六服务部署已完成；其200文件快照遗漏运行依赖，应用仓库后续ENV-R08已恢复完整202文件并增加构建时导入检查。旧部署/rollout/rollback仅作历史记录，不得覆盖修复。此次桥接基于完整202文件源码，现有部署未变更。
2. 已在隔离副本实现Contribution OS认证宿主桥接，默认关闭。真实mTLS本机与OrbStack临时容器合成链路通过，4次假推理/0真实调用；冻结实例/证书、Token过期/限次、未登记证书均验证。13文件补丁尚未写回或启用；完整回归1460通过/46跳过，保留最新用户源码的增量162通过/1跳过；证书/挂载/网络的生产启用审查见integrations/contribution-os-host-bridge.md。
3. 等待真实上游/各能力模型、显式API Key引用及合成请求预算。只接收引用名，不索取或导出原生CLI登录；批准前不发真实计费请求。桌面程序尚未替换安装或启动，不能把原生客户端HTTP联通称为真实UI验收。
4. 等待Windows x64、Linux x64、macOS x64目标实机；Linux arm64隔离容器的6类生命周期只作为补充证据。OS休眠/重启/启动项、真实升级/回退仍开放；不影响用户正在进行的工作来伪造此类验收。

可复验入口：`tests/model_gateway/native_consumers.py`、`performance.py`、`lifecycle.py`；完整命令与证据边界见acceptance/closeout-20261026.md。全部真实Gate满足后才可改为整体完成。CLI会员登录桥接、图片URL下载、Responses/音视频、任意LAN访问均不在当前支持声明中。

2026-10-26桥接续轮：新增`host_bridge.py`和冻结配置探针，204个应用文件的内部导入完整；根仓库Python工具9项通过。最新完整回归结果见`acceptance/host-bridge-regression-20261026.json`。
