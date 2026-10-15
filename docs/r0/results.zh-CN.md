# R0 实施与验收记录 · 2026-10-15

R0 代码已落地，自动化回归与四平台构建通过。真实 CLI 验收中 Codex、Antigravity 的 headless 和同 session 继续通过，Codex 取消通过；首次 Cursor 实际执行因工作区信任检查失败；用户后续显式 --trust 复测及真实项目体验中，Cursor 在线运行、resume 和后台成功。体验另发现 Antigravity headless 权限拒绝误报成功，已修复判定；随后授权四个读取命令，真实 README 读取复验成功。完整兼容仍未验收。以下记录形成于本地验收阶段；未打 tag 或发布安装包。

基线为 main `0a880f9`，实施在现有工作目录进行；开工时已有的中文 README、两份旧计划及产品调整方案改动保留。worktree、内部 command/preflight 和相应回归从本地开发快照 `origin/v.0.5.0` 按模块接入，随后修复目录映射；没有整分支合并，也没有新增独立 worktree 管理入口。

## 交付

- F03：import 深拷贝及 targets 扩展保留环境、参数、目录；输入不变，重复导入幂等。
- F02：过渡保护替代 ownership v3 建设。原生条目更新、删除、强制接管拒绝；新增前核查权威快照及同名条目，源声明变化使计划失效。状态锁、陈旧保存检测、替换前重检及导入回滚保护避免覆盖外部变更。
- F04/F01：步骤保存实际 cwd；旧 workflow 回退明确提示。前台和后台 worktree 将原仓库内绝对路径映射到隔离目录，越界和目录替换在启动前拒绝；隔离模式只允许一个写入角色。
- F05：增量解析及统一状态合成覆盖 run、resume、workflow、后台和 jobs wait。坏流、截断、原生协议错误及缺失完成事件不再被退出 0 覆盖；保留原生退出证据。录制或事件输出失败同样不能报告完整成功。
- F06/F07：示例 cwd 修正，doctor 区分安装、版本与能力来源；版本失败、超时、缺失和空版本可见。
- Cursor/Antigravity 独立适配、session/resume、用量映射；新增 --events，保持 JSON 单对象与后台初始 Job 契约。交互透传和显式输出记录保留。
- 退役说明、launcher 兼容、R0 使用文档；release 通过同 tag 提交的可复用 CI 门禁后才运行 GoReleaser。

Task 入口、自动协作、目标验收与检查点属于 R1/R2，本批没有实现或宣传这些能力。

## 自动化与构建

| 检查 | 结果 |
| --- | --- |
| `go test -race ./...` | 全部通过；含新旧结果契约、六种执行入口坏流、后台取消、MCP 字段保护及前后台 worktree |
| 最后增加的交互记录和事件计时回归 | 定向 race 检查通过；事件计时以夹具发出事件为起点，启动另设 5 秒限时 |
| `go vet ./...`、`go mod tidy -diff` | 通过；未改变依赖 |
| gofmt、`git diff --check` | 通过 |
| Darwin/Linux × amd64/arm64，`CGO_ENABLED=0 go build -trimpath` | 全部通过 |
| 四平台临时 tar.gz 的独立 SHA-256 校验、文件清单、二进制一致性及执行权限 | 通过；本机 arm64 二进制 `--version` 通过 |
| GoReleaser 本机 snapshot | 未运行：本机没有该工具；本地采用独立归档检查，正式 snapshot 仍由已配置的 CI 执行 |

检查汇总、Go 版本及生产源码摘要见 [validation.json](evidence/validation.json)。最终构建与归档证据见 [build-and-archive-final.json](evidence/build-and-archive-final.json)。跨平台构建只证明可编译，不能代替目标机器上的原生 CLI 验收。

`BenchmarkDecoderOutput` 单轮测试，Darwin/arm64、Apple M4 Pro：

| 事件总量 | 时间 | 采样堆内存增量最大值 |
| --- | --- | --- |
| 100 MiB | 0.413 秒 | 3,664,792 字节 |
| 1 GiB | 4.243 秒 | 3,909,368 字节 |

该测量使用固定 4 KiB 未知合法事件和终结事件，每约 16 MiB 采样一次 HeapAlloc，关闭完整记录。它支持“内存不随总流量线性增长”的结论；不是操作系统 RSS 峰值或所有协议载荷的性能承诺。超大单事件和最终回答截断另有回归。

## 真实 CLI

证据见 [native-smoke.json](evidence/native-smoke.json)。仅使用已有默认原生登录环境；没有登录、退出、切换账号、复制凭据或外部发布。测试材料和项目在临时目录，原始输出默认不保留，证据不含账号标识、会话 ID 或原始工具载荷。

| CLI / 版本 | 实测 | 边界 |
| --- | --- | --- |
| Codex 0.162.1 | headless、同 session 继续、取消通过 | 固定材料和上下文标记吻合；取消时 xcli 返回 130，保存原生 -1 和 canceled |
| Antigravity 1.3.5 | headless、同 conversation 继续通过 | 独立协议结束、session、回答和用量提取通过；真实取消未测 |
| Cursor 2026.10.14-0f9edde | 首次 headless 被信任检查阻断；后续显式 --trust 的 headless 通过 | 新建临时目录、一次原生复测；当前 xcli 对捕获日志的离线回放通过；真实 resume/取消未测 |
| Claude/Gemini/OpenCode | 本地模拟回归 | 本批真实环境兼容未验证 |

本轮计数为 **6 次原生 CLI 进程调用**，没有失败重试。厂商内部模型/工具往返不可完整观测，因此不声称内部模型请求总数也不超过 6；原方案按所有模型轮次计数的目标仍有观测缺口。到达调用上限后没有为 Cursor 继续发起模型请求。

真实 smoke 使用的候选二进制 SHA-256 已保存在证据中；之后补充了 I/O 失败处理、显式交互记录兼容、探测等待上限等保护，最终源码经过回归和重新构建，未重复消耗真实调用预算。真实证据不能冒充最终二进制的完整端到端复验。

## 发布边界

当前可确认的真实路径为上述 Codex、Antigravity 场景，以及后续[用户体验](user-experience.zh-CN.md)中的 Cursor 在线运行、JSON、events、resume 和后台，另有 Antigravity 授权后读取真实项目 README 的复验。Cursor/Antigravity 真实取消、Antigravity 修复后遇到权限拒绝时的原生失败复验、其他原生 CLI 和非 macOS 实机仍是发布兼容声明的限制。完整 Task 场景尚未开始；没有将 R0 实现完成标作 R1/R2 或正式发布完成。

MCP 原生覆盖/删除被拒绝是有意的兼容变化，--force 不能恢复旧行为；已有 `mcp serve` 引用继续可用，用户数据不自动删除。迁移说明见 [R0 使用说明](README.zh-CN.md)。

## Cursor 失败追查

用户另行授权直接复测后，使用相同 headless 参数在新建临时 Git 项目中执行一次，保留私有 stdout/stderr。执行环境登录检查通过，但 Cursor 在约 0.395 秒后返回 1；stdout 为 0 字节，stderr 明确报 `Workspace Trust Required`，要求用户先信任该目录。脱敏证据见 [cursor-diagnostic.json](evidence/cursor-diagnostic.json)。

这解释了同参数复现中的无 session、无结构化事件现象；首次验收的原始 stderr 仍无法追溯恢复。本次没有添加 --trust、修改全局信任或使用权限跳过参数，也没有继续发起模型任务。后续对已确认安全的目录可由调用者显式传入 --trust，随后再验收真实执行。

## Cursor 显式信任复测

用户进一步授权后，仅对刚才由测试程序创建并校验过的新临时项目添加 --trust，执行一次相同 headless 任务。约 25.959 秒完成，原生退出 0、stderr 为空，结构化流包含 system/user/tool_call/assistant/result，终结事件为 result/success、is_error=false；有效 session 存在，返回标记匹配，测试文件未改变。没有添加 --force、--yolo 或自动批准 MCP 参数。

对捕获流使用当前源码构建的 xcli 离线回放，得到 status=success、parse_status=complete，session 和最终回答提取正确；回放没有调用原生 Cursor 或模型。证据见 [cursor-trusted-smoke.json](evidence/cursor-trusted-smoke.json)。此处验证了原生 headless 和适配器离线解析，尚未把当前 xcli 的在线 headless/resume/取消全部登记为通过。原始日志保留于私有临时目录。

## 用户真实项目体验与后续修复

2026-10-15 用户在 chat-insight 项目中验证了 Cursor 普通运行、JSON、events、resume 和后台读取；Codex 新增文档任务也成功。完整命令、运行 ID、耗时、模型回答摘要、账号入口及待改进事项见[用户体验记录](user-experience.zh-CN.md)。这些是用户发起的真实调用，独立于前述自动 smoke 的调用预算。

Antigravity 的 README 读取实际因 headless command 权限自动拒绝而无输出，旧记录却是 success、complete、exit_code=0。本次增加有界 stderr 诊断识别，明确权限拒绝且无回答时保留原生 0，但最终 failed、process_failed、xcli 退出 1；不改历史、不自动批准原生工具。完整协议的 parse_status 仍为 complete。回归使用模拟输出，未额外调用模型或验证授权后的实际执行。此前 Antigravity 固定材料 smoke 成功不代表工具读取权限普遍可用。

六入口新增回归发现并修复前台 run/resume 的原生 stderr 与活动提示并发写入问题。最终完整 `go test -race ./...`、`go vet ./...`、差异格式检查通过，并已重新构建本地 `./xcli`；未为本次小修复重复四平台归档或发起原生模型任务。

用户进一步要求实际可用后，经授权备份并合并原生 CLI 的 cat、ls、pwd、head 四个命令 allow 规则，真实读取 chat-insight README 一次，34.095 秒成功，无权限拒绝，README 内容与项目 Git 状态未改变。该次调用独立于前述修复的模拟回归；任务明确要求只用 cat README.md 读取，不包含任意命令执行。证据见 [antigravity-authorized-read.json](evidence/antigravity-authorized-read.json)，配置、备份和复用命令见[体验文档](user-experience.zh-CN.md#后续授权-antigravity-并真实重试)。

## 媒体能力真实验收

用户随后要求真实验收图片、视频、音乐的生成与编辑。首轮完成 10 个基础场景的调用，另有 4 次定向探查，合计 14 次 headless 原生进程启动；无自动重试，内部模型轮次未完整观测。授权后追加 4 次媒体调用与 1 次交付恢复，累计 19 次。最终三个 CLI 的图片生成/编辑，以及 Antigravity 的 FFmpeg 视频/音频基础剪辑均产出了有效文件；视频和音乐模型生成仍缺当前环境可调用的接口。Cursor 原生输出 JPEG，按用户澄清判为通过；历史 PNG 转换被拒记录保留。

本轮也真实确认了 Antigravity 权限自动拒绝时原生退出 0、xcli 返回失败的修复行为；含 BLOCKED 文本且正常结束的对话仍可能记 success，需结合产物验收。完整结果、原图与编辑对照、产物链接及限制见[媒体真实验收](media-acceptance.zh-CN.md)，证据见 [media-acceptance-20261015.json](evidence/media-acceptance-20261015.json)。临时加入的两条精确 FFmpeg 授权已移除，原有四个读取命令授权保留。未修改用户体验项目。

用户明确授权媒体子代理后，追加图片生成/编辑、视频生成、音乐生成四次调用，以及一次同会话图片交付恢复，累计 19 次 headless 启动。Antigravity 的 image-generator 原生日志确认调用 generate_image，生成和编辑 JPEG 均验证通过。首次编辑交付 result 含 denied_actions 但 response 非空，已补充 xcli 失败判定并通过真实日志离线回放；单条精确复制授权后 resume 成功返回产物路径，没有重复生图，复制授权已移除。视频子代理返回 not found or not allowed，音乐会话没有生成接口，该能力排查现按用户决定暂缓。

该生产代码修复已通过完整 go test -race ./...、go vet ./... 并重新构建 ./xcli。Cursor 按用户澄清的原生格式验收要求判为生图通过，历史 PNG 转换拒绝不被改写。图片生成、编辑产物与新证据均已保存。

继续排查确认 agy mcp list 返回 No MCP servers configured，当前环境也无相应生成服务凭据变量入口。未安装外部服务、未启用付费额度，也未把其他产品的媒体生成功能记为 Antigravity 通过。此前询问的服务或凭据入口不再作为待办；用户查阅资料后明确暂缓视频、音乐研究。

## 阶段交接

2026-10-15 用户明确暂缓 Antigravity 视频和音乐能力研究，要求提交推送当前成果。现有图片验收、权限失败修复及基础剪辑证据保留；本阶段不再等待媒体服务凭据。下一阶段为 R1：统一 Task 入口、指定单 CLI 与自动协作两种执行方式、交接包、真实目标验证及结果摘要。R2 再扩展长任务后台、检查点、停止与继续以及失活恢复。

提交前最终检查：`go test -race ./...`、`go vet ./...`、`go mod tidy -diff`、Go 格式和 `git diff --check` 均通过；最新代码的 Darwin/Linux × amd64/arm64 构建、临时归档及校验和复验通过，见 [提交前构建证据](evidence/build-and-archive-precommit.json)。未重复调用原生模型。
