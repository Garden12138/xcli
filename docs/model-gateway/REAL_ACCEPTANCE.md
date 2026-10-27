# 真实应用与实机验收交接

更新：2026-10-27。后续由用户跟进。本文件列出尚未完成的操作、通过标准和留证方式；**所有待验项保持未勾选**。提交、推送代码不代表这些验收已通过，也不代表已发布安装包。

## 先确认交付范围

| 项目 | 已完成的证据 | 后续仍需完成 |
| --- | --- | --- |
| xcli 网关 | M0–M5 工程、离线协议/生命周期、交叉构建、本机性能基线 | 真实模型能力与效果、目标实机、休眠/重启/真实升级回退 |
| 四个应用 | 接入源码已写回对应仓库；实际客户端代码经假上游联通 | 构建并安装实际应用，操作界面和业务流程，验证实际安全存储 |
| Contribution OS Docker 桥接 | 独立 `xcli_bridge` 的 13 文件补丁、本机/OrbStack 双向 TLS 合成验收 | 补丁尚未写回应用仓库；生产证书、私有挂载、部署、真实 Plan/Review 与物理 Linux Docker |

完整离线回归为 1460 通过 / 46 跳过；保留后续独立用户改动的补充回归为 162 通过 / 1 跳过。它们不能替代本文件中的实际操作。[回归记录](acceptance/host-bridge-regression-20261026.json)、[应用矩阵](integration-matrix.md)、[进度](PROGRESS.md)。

本仓库提交包含应用接入补丁及侧车摘要；四个外部应用仓库的提交、推送、安装和部署需在各仓库单独跟进。此前已写回的四个完整 provider 补丁不要重复应用。临时 `/tmp` 副本、二进制、虚拟环境和旧镜像 ID 都不是持久交付物。

## 1. 准备一次验收

- [ ] **PRE-01 版本与恢复点。** 记录 xcli 和各应用实际采用的 Git SHA、二进制/镜像摘要、系统架构、安装版本、配置摘要。确认启动的是本次构建。按应用既有方式备份本地数据、索引和当前可用程序；私有备份不提交 Git。
- [ ] **PRE-02 模型登记。** 选定上游、真实模型名、网关 alias、能力、推理位置；Embedding 另记维度和索引版本。无法核实推理位置时使用 `unknown`。供应商密钥只记录 `env:NAME` / `file:PATH` 等引用；每个应用使用独立 xcli App Token。
- [ ] **PRE-03 明确预算。** 为每轮写明最大模型请求数、输出 Token、图片张数、Embedding 批量、超时和费用上限；确认上游限额/停止方式。一项 UI 操作可能产生多次模型调用；目录读取不算推理。脚本的请求计数不等于费用上限，也不限制应用 UI 的后续调用。
- [ ] **PRE-04 合成材料。** 使用自建日报、测试聊天/文档、合成图片和本地测试项目。先验证应用流程，再按预算测试真实模型；不用真实私密聊天或工作日志作为首轮材料。
- [ ] **PRE-05 应用安装。** 分别按 Worklog、Chat Insight、GardenFlow、Contribution OS 当前仓库说明构建/安装或部署。尤其先安装 Chat Insight 隐私修复，再启用其正式云路由。保留各仓库既有用户改动。

从 xcli 仓库根目录执行以下配置检查，不发模型推理。所有命令指向同一份明确配置与同一个独立状态目录，替换示例绝对路径：

```sh
xcli --config /absolute/private/model-gateway.yaml model --state-dir /absolute/private/xcli-state doctor --no-billable-requests
xcli --config /absolute/private/model-gateway.yaml model --state-dir /absolute/private/xcli-state list
xcli --config /absolute/private/model-gateway.yaml model --state-dir /absolute/private/xcli-state start
xcli --config /absolute/private/model-gateway.yaml model --state-dir /absolute/private/xcli-state status
```

`doctor`、健康检查和目录通过，只证明相应配置/进程可用。[配置与 Token 操作](README.md)、[API 合同](api-contract.md)、[能力声明](capabilities.md)。

## 2. 验证真实上游能力

- [ ] **MODEL-01 文本与流式。** 在批准预算内验证非流式内容、SSE 增量/结束、取消和超时；取消后本地不再生成结果，核对上游取消观测。中断流不能显示成功；已产生用量如实保留，未知用量不能记为 0。
- [ ] **MODEL-02 JSON 与工具。** 分别验证选定模型的 JSON Object、JSON Schema 和工具名称/调用 ID/参数/结果关联。检查实际输出符合消费端 Schema；“可解析 JSON”不足以证明严格 Schema 合格。网关只传递工具调用，工具执行仍由应用自己的权限控制。
- [ ] **MODEL-03 Vision 与图片。** 用已知内容图片验证识图；验证生成及参考图编辑得到有效、可打开的图片，尺寸/数量符合请求。仅返回 URL 的上游当前不支持；不能以下载 URL 或换模型绕过。mask 不在 GardenFlow 当前 UI 范围。
- [ ] **MODEL-04 Embedding。** 验证批量数量、index、有限数值、维度、模型/索引身份与检索结果。更换模型或维度使用独立 alias 或显式重建索引。

强能力需先以真实上游的受控验证取得证据，再据实登记 `probe_verified` / `runtime_observed`、时间及证据版本。假上游声明只对夹具有效，不能复制成真实模型声明；未经验证的强能力被拒绝是预期行为。

可选协议 smoke 使用下面的现有脚本。先由本机安全存储或私有 Token 文件向当前终端设置 `XCLI_APP_TOKEN`，不要将值放进命令参数、文档或日志。默认命令只读目录：

```sh
python3 examples/model-gateway/smoke.py --base-url http://127.0.0.1:18741/v1 --profile worklog
```

**下列命令会请求模型，可能计费；仅在 PRE-03 已记录并批准本轮预算后执行。** 示例为每类最小脚本调用数；替换 alias、端口与模型支持的图片尺寸。需要 remote/unknown 时才显式加 `--allow-cloud`，相应 App Token 也必须有明确远程权限。

```sh
python3 examples/model-gateway/smoke.py --base-url http://127.0.0.1:18741/v1 --profile worklog --text-model xcli/text-default --execute --max-model-requests 1
python3 examples/model-gateway/smoke.py --base-url http://127.0.0.1:18741/v1 --profile garden-flow --text-model xcli/text-default --image-model xcli/image-default --image-size 1024x1024 --execute --max-model-requests 3
python3 examples/model-gateway/smoke.py --base-url http://127.0.0.1:18741/v1 --profile chat-insight --text-model xcli/text-default --embed-model xcli/embed-default --execute --max-model-requests 2
python3 examples/model-gateway/smoke.py --base-url http://127.0.0.1:18741/v1 --profile contribution-os --text-model xcli/text-default --execute --max-model-requests 2
```

脚本的 Contribution OS 画像要求同一 alias 有已验证的 `json_schema` 和 `tool_calls`。这些仍是协议检查；应用 UI、内部冻结 Profile 和任务授权要继续按后续步骤操作。终端会话结束时清除该临时环境变量。

## 3. Worklog 实际操作

- [ ] **WL-01 设置与 Keyring。** 在实际设置页选择 xcli，保存独立 App Token，读取授权目录并确认 alias/位置。重启应用后仍可调用；从其他 provider 切换时要求独立凭据。目录检查不触发模型推理，设置页/日志不暴露 Token。
- [ ] **WL-02 日报与周报。** 输入两至三条合成工作记录，分别生成日报、跨日周报；检查实际界面展示/保存/再次打开，日期、事实与模板字段正确，不虚构输入中没有的成果。
- [ ] **WL-03 模板和 JSON。** 选择一个现有模板并完成填写；验证结构化输出被应用解析，合法结果能显示/保存，坏 JSON/缺字段显示可操作错误，不保存“成功”假结果。
- [ ] **WL-04 失败与撤销。** 测试错误端口、未授权 alias、过期/撤销 Token、网关停止和超时。错误不含凭据；不静默切换到其他 provider，不产生额外自动模型请求。最后恢复正确凭据并成功调用一次。

## 4. GardenFlow 实际操作

- [ ] **GF-01 Source 与能力。** 新建独立 xcli Source，分别选择文本与图片 alias；确认位置提示。缺少经验证的 Vision、Tool Calling 或所需流式能力时，在发送上下文前拒绝相关操作。
- [ ] **GF-02 Vision。** 上传已知内容的合成 PNG/JPEG/WebP，使用支持 Vision 的 alias 识别其中内容；验证请求经过所选 Source，结果与图片相符，失败不换模型。
- [ ] **GF-03 封面生成和落库。** 生成一张指定尺寸封面，检查画面可打开、数量/尺寸正确、进入实际媒体库/内容关联，重启后仍能查看；记录素材摘要与媒体引用，不只看 HTTP 成功。
- [ ] **GF-04 参考图编辑。** 先用一张合成参考图编辑；需要多图时再测试已声明且已验证的多参考图能力。确认参考图没有被截断，编辑失败不会退化成生成；一次操作只发一次图片请求，不自动删参、调整 size/n 或重试。
- [ ] **GF-05 Pi 工具流。** 选择已验证支持工具调用和流式输出的文本 alias，用无外部写入的测试操作验证工具名、参数、调用结果及 UI 回显；失败/取消后不继续执行工具。
- [ ] **GF-06 媒体异常。** 用合成数据验证非法图片、超量参考图/结果、超大文件、URL-only 输出被拒绝，媒体库没有成功假记录。当前无 mask 控件，不能将 mask UI 标为通过。

单图上限 20 MiB、像素 16,777,216，最多 4 张参考图/结果、总预算 32 MiB；Vision 的 base64 仍计入 4 MiB JSON 请求预算。要求逐请求同意头的 Token 在此客户端会明确拒绝；不靠自动补授权通过测试。

## 5. Chat Insight 隐私、索引与问答

- [ ] **CI-01 实际安装和后台行为。** 确认启动已含隐私修复的版本，选择 xcli 后后台自动扫描关闭。重启、空闲及未发起操作时，不上传正文；界面不以 loopback 地址宣称本机推理。
- [ ] **CI-02 拒绝上云。** 准备单独的 local、remote、unknown 测试 alias，分别检查聊天和向量模型的位置。云授权关闭、单次确认拒绝、目录失败或位置缺失时，不上传任何正文或向量输入；授权目录读取可以存在。用受控请求计数/脱敏观测证明，不能仅凭 UI 提示或用量为 0 推断。
- [ ] **CI-03 显式同意。** 授予云权限后，仅对一次合成请求确认，检查该请求携带同意并成功，下一次仍按自身确认策略执行。拒绝下一次请求时正文调用不增加，取消/截断流不显示成功。其他应用的授权不能替代本应用同意。
- [ ] **CI-04 索引首次接入和重建。** 导入少量合成聊天/文档，显式创建测试索引。改变 endpoint、alias、维度、`index_version` 或 `index_fingerprint` 后提示重新确认重建；旧索引不自动迁移/混用。错误响应身份应被丢弃，不污染现有索引。
- [ ] **CI-05 检索和问答。** 对合成语料提出答案已知的问题，检查批量向量、检索片段、来源引用和最终回答；重新打开应用后结果/索引身份一致。失败、取消和恢复后的行为可解释，原始聊天与密钥不进入网关审计。

云端模型的位置取决于登记的实际推理位置；网关在本机不改变数据出境性质。真实用户聊天的使用需由用户后续单独决定，本轮以合成语料完成上述验收即可。

## 6. Contribution OS 部署、Plan 与 Review

### 先选择实际部署路径

- [ ] **CO-01 运行时完整性。** 在最新应用仓库确认源码、构建上下文和镜像一致，执行其当前内部导入/运行时完整性检查及相应回归。保留 ENV-R08 运行依赖修复；202/204 文件只是历史版本的计数，应核对当前完整清单，不能固定数量冒充完整性。旧 `xcli-integration-75d9cea7fbde` 和旧 rollout/rollback 文件已被修复取代，不要重放。
- [ ] **CO-02 选择 native 或 Docker。** native 使用已登记的 `xcli_local`，API/Model Gateway 为本机进程。Docker 使用单独的 `xcli_bridge`，先完成下面桥接步骤；不能把 native provider 的容器拒绝改成直通私网 URL。

Docker 桥接尚未写回。先阅读[桥接实现与验收](integrations/contribution-os-host-bridge.md)及[补丁入口](integrations/README.md)，检查目标仓库最新 HEAD/脏文件，再执行防漂移应用：

```sh
python3 tests/model_gateway/integration_workspace.py apply \
  --patch docs/model-gateway/integrations/contribution-os-host-bridge.patch \
  --repository /absolute/path/ai-open-source-contribution-os-summary
```

该命令只写源码，不部署。HEAD/前置摘要漂移时会拒绝；重新创建快照、合并最新用户改动、回归并生成新补丁，不能强行忽略摘要。应用后阅读新增的应用仓库 `docs/deployment/xcli-host-bridge.md`，配置字段与启用条件均在该文档及补丁中，不依赖临时目录。

- [ ] **CO-03 证书与部署清单。** 记录独立安装实例 ID、端口、服务端叶证书指纹、允许的客户端叶证书指纹、模型白名单和预算。宿主 relay 只监听 loopback；Model Gateway 使用专用客户端证书，并固定服务端叶证书。只有 Model Gateway 获得只读私有证书挂载及独立 App Token；API 只获得公共登记元数据。按实际进程 UID 检查 Unix 0700/0600，不通过放宽权限修复挂载问题。
- [ ] **CO-04 实际 Docker 通路。** 在将要使用的 macOS arm64/OrbStack 或物理 Linux amd64/Docker 上验收容器到宿主 mTLS，核对实际证书/实例/模型及目录。Sandbox Worker/Runner、Publisher 不持有桥接证书/Token/宿主 socket；不启用 host networking 或 LAN listener。生产启用前确认精确镜像、挂载、预算及保留完整运行时的回退清单。
- [ ] **CO-05 拒绝与轮换。** 验证缺失/错误/过期证书、同 CA 但未登记的客户端、错误实例、错误服务端指纹、错误 App Token、未授权 alias/路径均不能发正文。换证书或实例后旧冻结连接失效，保存新的连接/Profile；不要改写历史 Profile 使其恢复可用。物理 Linux 的结果单独留证。

### 在实际应用走业务流程

- [ ] **CO-06 模型设置。** 在实际设置 UI 保存独立加密 Token 引用及所选连接/Profile，读取模型目录并测试连接。冻结地址、alias、实例/证书（桥接路径）与 Profile hash 一致；更改默认配置只影响后续任务。
- [ ] **CO-07 Plan。** 用合成任务和专门测试项目生成计划，在 UI 查看/保存，核对模型、来源、版本、预算和用量。后续执行所需的人类计划确认由用户在应用中完成。
- [ ] **CO-08 Verify 与 Review。** 如 Review 需要执行产物，在应用既有隔离 Sandbox 中完成最小修改/验证，再运行独立 Review；不在宿主运行不可信项目。核对 Plan → 执行 → 测试 → Review 的不可变引用、结果和用量。Review 失败应保留失败状态，不能以请求成功或工作进程健康代替。
- [ ] **CO-09 授权和恢复。** 验证篡改冻结 Profile、过期/超额任务 Token、预算耗尽、模型不可达和任务重启恢复。未授权请求不触达上游；失败历史、用量、已批准计划不被重置或覆盖；无 legacy provider fallback。

验收在 Review 完成后结束。Fork、Push、PR、评论、CLA 等外部写入不属于本轮验收；若以后需要，沿用应用对应的一次性确认流程。真实 Plan/Review 的费用在 PRE-03 单独预算，协议 smoke 的两次调用不能覆盖这段业务流程。

## 7. 目标实机与生命周期

| 目标 | 目前证据 | 后续验收 |
| --- | --- | --- |
| macOS arm64 | 本机假上游生命周期、OrbStack 合成桥接 | 实际安装、休眠/唤醒、系统重启、可选启动项、真实升级回退 |
| macOS x64 | 交叉构建 | 对应实机的全部生命周期及实际应用 |
| Linux amd64 | 交叉构建；Linux arm64 容器补充检查 | 物理 Linux/Docker 的生命周期、桥接及安全边界 |
| Windows x64 | 交叉构建与实现代码 | 实机网关、DACL、取消/进程、重启/启动项；原有 Agent 后台 Task/jobs 不受支持；当前 Contrib 桥接不支持 Windows ACL |

- [ ] **OS-01 本机二进制。** 在每个目标系统记录版本/摘要，执行该系统的 `go build` 或安装相应构建产物，运行下面现有脚本；它使用临时状态和假上游，不需要真实模型。
- [ ] **OS-02 单实例和隔离。** 检查并发 start、实际端口发现、端口冲突、重复 stop、停止后重启、旧实例停止不能影响新实例；不影响另一个用户/安装实例。
- [ ] **OS-03 权限与取消。** 验证本机状态/Token 权限，Windows 特别检查 DACL 和实际取消/进程结束；不把交叉编译当作实测。Windows 不支持的 Agent 后台功能如实记录。
- [ ] **OS-04 休眠和重启。** 在可中断的测试环境运行网关和应用，休眠/唤醒后重新读取状态/目录并完成一次受控请求；重启系统后验证手动启动，若启用用户启动项则单独验证。不能留下“状态 active 但不可达”的成功记录。
- [ ] **OS-05 升级和回退。** 记录旧二进制/镜像及配置/业务数据恢复点，停止旧版后替换，doctor → start → status → 目录 → 应用受控请求；再停止新版恢复旧版并重复。Task/Run、应用数据、索引、授权与审计历史保留，不能用旧的缺依赖 Contrib 镜像回退。

```sh
python3 tests/model_gateway/lifecycle.py --binary /absolute/path/to/native/xcli
```

Windows 的 `--binary` 指向 `xcli.exe`。[脚本](../../tests/model_gateway/lifecycle.py)只覆盖离线生命周期；休眠/系统重启/安装升级需手动操作并另记。不要在用户正在工作的机器上未经安排重启。可选启动项配置见 [README](README.md)。

## 8. 留证、失败处理与关闭条件

复制[验收记录模板](acceptance/real-acceptance-record.template.md)为一次独立记录，填写日期、版本、配置摘要、用例 ID、实际步骤、预期/观察、请求数/费用、截图或脱敏证据路径。假模型、实际应用 UI、真实模型、实机结果分别记录，不能用一种替代另一种。

失败时停止该项后续调用，保留首次错误与实际已发生用量；不要自动重试计费请求。恢复当前可用配置/程序，撤销本轮临时 Token/证书授权；不要删除业务历史或覆盖旧索引。修复后用新记录复验。

完整验收的关闭条件：所有适用的 MODEL/WL/GF/CI/CO/OS 项都有对应版本的通过证据；未完成项、被跳过项及不支持功能明确保留。某能力决定不启用时，记录缩小后的交付范围，不能把它记为通过或宣称完整 M0–M5 真实验收完成。由用户确认结果后再更新 [PROGRESS](PROGRESS.md)、[SUMMARY](SUMMARY.md) 和[应用矩阵](integration-matrix.md)。
