# 自动化验收

2026-11-11。**离线产品与保留源码回归 PASS，适合用户开始受控本地体验；本轮真实模型/账号/原生权限可用性 NOT_TESTED，不是正式发布或日用就绪声明。** 源码为 `665ec15` 加本次测试同步修复/文档；正式提交及构建收据见 [分支报告](BRANCH_CONSOLIDATION.md)。

PASS=本轮实际执行成功；FAIL=实际失败；BLOCKED=环境或授权条件阻断；NOT_TESTED=本轮未执行。Mock/Fake 成功不表示真实模型成功。机器可读统计、安装检查和日志摘要见 [acceptance-results.json](acceptance-results.json)。

## 环境与边界

macOS 26.5.1 / Darwin arm64，Go 1.26.4，Python 3.10.9，Git 2.48.1，Pillow 12.2.0。版本实际输出 `xcli version 0.5.0-r4.1-dev`。构建时位于 main/U + 整理工作树，后续从相同源码在整合分支交付。离线变量使用 `GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off PYTHONDONTWRITEBYTECODE=1 GORACE=atexit_sleep_ms=0`。

阅读并复用 R0–R4、cli-first/product-realignment 说明与测试入口。Go 使用原有 launchtest 的真实命令拒绝桩，Python 额外将六个原生命令置为拒绝桩；R3 集成测试显式覆盖为自己的 Fake。新增真实模型请求 **0**、新增真实原生 CLI 启动 **0**、原生登录/认证/Workspace Trust 修改 **0**。R3 复用脚本在 simulation 中也会打印模板中的“one real read-only invocation”，实际 PATH 指向 Fake，不能把该字符串解释为本轮原生/模型实测。只读 PATH/文件摘要发现三 CLI，见 [native-discovery.json](native-discovery.json)；未启动其版本/help/status。

完整 Go 套件有临时回环 HTTP 单元夹具及合成 OAuth/协议合同；它们使用 Fake、临时状态，不连接真实供应商，不启用配置中的正式 Gateway。没有执行会显式启动冻结服务的旧生命周期 smoke。

## 执行命令与统计

| 检查 | 实际命令/入口 | 最终结果 |
| --- | --- | --- |
| 全部跟踪 Go 文件格式 | `gofmt -l <git ls-files '*.go'>` | PASS，无未格式化文件 |
| 完整普通 Go | `go test -json -p 1 -count=1 -timeout 240s ./...` | PASS，560 pass / 0 fail / 1 skip，含子测试 |
| 完整 Go race | `go test -race -json -p 1 -count=1 -timeout 240s ./...` | PASS，同为 560 pass / 0 fail / 1 skip，无 race 报告 |
| 包范围 | 两个完整 Go 套件 | 各 27 个测试包通过；6 个包无测试（Go JSON 记 skip） |
| 静态检查 | `go vet ./...` | PASS |
| 模块一致性 | `go mod tidy -diff` | PASS，go.mod/go.sum 未变 |
| 依赖缓存完整性 | `go mod verify` | PASS，all modules verified |
| 构建 | `go build -trimpath -o bin/xcli .` | PASS，macOS arm64 可执行 |
| 修复专项 | `go test -race -count=3 ./internal/launch` | PASS，保留所有原断言 |
| 原有核心门禁 | `bash tests/product_realignment/verify-core.sh` | PASS，退出0，R4_CORE_REGRESSION_PASS；核心 race/vet/tidy/build/Python/安装通过 |
| R0 Python | `python3 -m unittest discover -s tests/r0 -p 'test_*.py'` | PASS，11 项 |
| R3 Python（开启进程集成） | `XCLI_R3_BINARY=<built-xcli> python3 -m unittest discover -s tests/r3 -p 'test_*.py'` | PASS，25 项；没有因缺 binary 跳过集成 |
| 冻结源码 Python 合同 | 分别 discover tests/model_gateway 和 tests/native_agent_gateway | PASS，9+9 项；不启动正式服务 |
| 安装验收 | `python3 tests/r3/install_acceptance.py --binary <built-xcli> --out <temporary>/installation.json` | PASS，12 检查；空间/中文 PATH、XDG、配置保护、缺 CLI 的真实失败 |
| 旧状态兼容 | Go Task v1/v2/Run/workflow/配置合成夹具 | PASS；未访问用户历史运行目录，安装脚本 --study 历史真实副本 NOT_TESTED |
| macOS/Linux 构建与验包 | `python3 tests/r0/package.py --out <temporary>/package.json` | PASS，darwin/amd64、darwin/arm64、linux/amd64、linux/arm64，4 个包及 checksum/成员/执行位检查 |
| Windows 编译 | `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o <temporary>/xcli-windows-amd64.exe .` | PASS，仅编译；后台 Task/Jobs 不支持 |
| GoReleaser snapshot | 未安装 goreleaser，未执行 release/snapshot 流程 | NOT_TESTED；用既有 R0 打包脚本及 R3 archive 单测验证，不冒充实际 GoReleaser 安装 |
| 命令语法/帮助 | 新二进制 31 个当前命令路径的 --help | PASS；只调用 xcli 帮助，无原生探针 |
| 历史媒体文件 | 复用 tests/r0/native_media.py 的 image_info，加历史清单 SHA 校验 | PASS，13 文件摘要，11 图片完整解码；2 音视频仅摘要，视觉/音视频重解码 NOT_TESTED |
| 空白与 patch | `git diff --check` / 最终 staged 检查 | PASS |

Go 的 -p 1 只串行包，包内并发/race/100 路预算测试未关闭；这与仓库现有 offline 门禁策略一致。560 是包含子测试的通过结果条数；各为303个顶层通过、1个顶层跳过，不将父用例和子用例重复视为独立场景。核心门禁重复已有子集，不与完整套件累加。唯一用例 skip 是 `TestG7ContainerHTTPFakePeerAndRuntimeLoss`，需要显式容器开关，本轮未进入冻结隔离实验。

## CLI 与能力覆盖矩阵

| 能力 | 现有主要测试/证据 | 本轮离线 | 本轮真实模型 |
| --- | --- | --- | --- |
| agents/doctor/安装/原生 auth 不执行 | R0 Detect、DoctorVersionFailure、R4DoctorNeverUsesLegacyLogin、安装12检查 | PASS | NOT_TESTED |
| run/JSON/Events/流终态 | RunCommand、EventsAreVisibleBeforeProcessFinishes、BrokenStream 全入口、diagnostics | PASS | NOT_TESTED |
| Adapter 选择/Codex/Cursor/agy argv | BuiltinRun/Resume、NewAdapterCommandsAndResume、RunAgentSelectionPrecedence | PASS | NOT_TESTED |
| resume/Session/cwd/失败 | ResumeNonInteractive/NativeSession/Validation、session 分类 | PASS | NOT_TESTED |
| Task Work/Project × 单/多 CLI | TaskFourPathsAndRepair、R3 integration、Task CLI | PASS | NOT_TESTED |
| 阶段交接、真实合成文件及来源 | handoff consumption、SelectedSourceAndEvidenceIntegrity、MissingArtifact 拒绝 | PASS | NOT_TESTED |
| 测试命令、worktree、路径约束 | TaskCommandAndMaterialProtection、worktree 包、实际合成项目测试 | PASS | NOT_TESTED |
| Task stop/continue/events/后台/owner 崩溃 | TaskDetachedStopAndNativeResume、OwnerKill、LiveUpdate、CheckpointContinue | PASS | NOT_TESTED |
| 材料恢复/不确定副作用/预算累计 | MissingNativeSessionRestoresMaterials、InterruptedAttemptReconciliation、CumulativeBudget | PASS | NOT_TESTED |
| Jobs 状态/logs/wait/stop/崩溃核对 | cmd/jobs 与 internal/jobs，失败退出/删除保护测试 | PASS | NOT_TESTED |
| 配置及兼容/原数据保护 | config tests、installed-candidate、旧 Task 合成恢复 | PASS | NOT_TESTED |
| MCP 兼容 launcher | MCPServe 项目查找/环境/取消、mcp 包 retained tests | PASS | NOT_TESTED |
| 失败/取消/超时/权限/额度诊断 | R0、diagnostics、runtime/process、launch timeout/cancel | PASS | NOT_TESTED |
| 启动额度/并发/幂等/unknown | HundredConcurrentTasksCannotOverdraw、CrashRecovery、ExternalBaseline 等 | PASS | NOT_TESTED |
| 冻结模块默认不启用 | R4FrozenGatewaysDefaultClosedBeforeIO（10 个动作）及帮助隐藏/收尾兼容 | PASS | NOT_TESTED |
| 媒体路径/普通文件/格式/来源 | native_media 单测与 13 个历史实文件摘要/11 个解码 | PASS | NOT_TESTED |

## 失败、环境阻断与修复记录

1. 首轮沙箱 `go test ./...`：部分核心测试通过，但 httptest 无法 bind loopback（operation not permitted），该范围 **BLOCKED**。改在已批准的宿主离线测试环境复验，没有放宽原生 CLI 权限。另有 Go 自身尝试写只读模块 stat cache 的警告，交叉构建退出仍为 0；依赖 verify/tidy 正常，未为此改依赖。
2. 宿主首轮：只有 `internal/launch/TestProbeTimeoutAndCancellationAreCharged` 失败。取消固定 sleep 20ms，而预留/fsync 可能耗时更长，进程尚未启动即被取消。产品正确留下 not_started/released，测试错误假设已启动。
3. 最小修复只在 `internal/launch/ledger_test.go`：Fake 子进程输出 ready 标记，取消分支等观察到子进程；保留 consumed/canceled/timed_out 原断言，并用 3 秒的有界测试上下文给启动留出时间。没有改产品参数/退款逻辑/错误分类，没有删测试或放宽断言。
4. 专项 race 重复3轮、完整普通/race 以及原有核心门禁复验，见最终记录。原始失败不被改写为通过。

未发现需要本轮新增产品功能的确定业务缺陷。长任务规划质量、真实账号/权限、厂商协议变化、Windows 后台、原生内部模型/子进程不可精确计数、固定角色参数缺失等限制见 [版本对照](VERSION_COMPARISON.md) 和 [手册](USER_MANUAL.md)。

## 未执行或受限项目

- **NOT_TESTED**：本轮 Codex/Cursor/agy 真实推理、实际额度、登录状态、原生工具权限、真实取消/超时、跨 CLI 协作收益、图片再生成/视觉重验、其他设备运行。
- **BLOCKED（历史不变）**：R4 原真实启动/外部控制器计数 UNKNOWN、NEEDS_FIXES；不从 Fake PASS 或新本地窗口推导旧余额。11-06 Cursor 新临时工作区 Trust 受阻，不声称现在已解除。
- **NOT_TESTED（范围禁止）**：tests/r0/smoke.py/native_media.py --execute、R1/R2 native、R3 compatibility.py/experiment --execute 等真实验收入口；tests/model_gateway/lifecycle.py、native_agent_gateway/control_smoke.py 等显式 opt-in 服务 smoke；因此 verify-offline.sh/旧 phase1 全流程未整段执行。其允许的 Go/Python/构建部分已分别执行。
- **NOT_TESTED**：真实历史 Task 私有副本 --study 检查和 GoReleaser snapshot。没有读取用户运行数据或发布安装包/Tag；只生成临时验包资产。

## 本地体验判断

离线基础、CLI 交接/恢复和安装路径已验证，可以由用户按 [QUICK_START](QUICK_START.md) 从独立小工作区开始。原生账号/权限/额度未重验，需在各工具自管；执行前需真实声明本次启动窗口。本轮不自动发起第一条模型请求，不自动开启 R5。

## 最终核验记录

原有核心门禁最终退出0，终结标记 `R4_CORE_REGRESSION_PASS`，R0 11项、R3 25项及安装12检查均通过。完整普通/race 最终均退出0；工作树仅一处测试同步修复，产品 Go 源码和依赖未改。文档本地链接、JSON、命令帮助及 diff 检查通过，Git 引用/历史完整性在整合提交后再次核对。最终提交与本机二进制摘要见 本机忽略文件 `FINAL_STATE.json`（不上传；clone 后用 `git rev-parse HEAD` 查询提交）。

用户后续授权推送后的提交仅更新发布、分支状态和获取源码说明；产品/测试源码与上述通过的整理提交 `2ca75c4` 完全一致，因此未重复运行完整 Go/race 套件。发布时复核文档链接、shell 示例语法、JSON 和 diff，未启动原生模型。
