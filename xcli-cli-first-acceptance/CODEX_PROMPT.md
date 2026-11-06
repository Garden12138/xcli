# Codex 主任务：xcli CLI-first 真实验证、缺陷修复与交付文档

你是 `Garden12138/xcli` 的验收负责人兼研发工程师。**核心目标不是使用 Ollama、vLLM、云模型 API Key 或 Fake Backend 证明 xcli 有模型服务能力**；目标是检验用户已登录的 **Codex CLI、Cursor CLI、Antigravity CLI 原生能力**，以及这些能力能否合规、稳定地提供给用户本人控制的桌面应用。

## 一、先做事实核查

1. 检查 `git status --short`、当前 HEAD、README.zh-CN.md、`internal/agent/*`、`cmd/model*.go`、`internal/modelbackend/*`、`internal/modelstore/config.go`、`docs/r0/media-acceptance.zh-CN.md`、`docs/model-gateway/*`、`tests/r0/smoke.py`。不要覆盖用户修改或假定旧文档与当前代码一致。
2. 严格区分：
   - A：`xcli run` 调用原生 CLI 完成 Agent Task（已有历史证据）；
   - B：`xcli model` 通过 `/v1/chat/completions`、`/v1/images/generations` 等接口调用原生 CLI（是否存在必须据代码核实）；
   - C：四个消费应用实际使用该能力（本轮不验收 UI）。
   不能把 A 的成功写成 B 的成功，也不能把 Fake 或普通 API 上游写成“Codex/Cursor/Antigravity 会员模型桥接成功”。
3. 阅读并核对各原生 CLI 的官方当前非交互、结构化输出、媒体工具、SDK/app-server、授权和账号使用规则；记录链接、核查日期和适用边界。优先选正式支持的集成方式。不得读取、打印、复制或导出登录 Cookie/Token，不得调用私有接口、伪装账号或绕过权限/配额。官方不支持通用模型转发时，改为面向该账号持有者的受控 Agent 任务接口，并明确不能兼容哪些 Chat Completions 语义。

## 二、可执行验收

在仓库根目录运行解压后的 `bash xcli-cli-first-acceptance/verify-phase1.sh --race`（离线，不发模型请求）。也可使用现有脚本等效执行。记录编译、Go test、race、vet、Model Gateway 生命周期、CLI 安装和 doctor 检查的真实结果，不得把未运行标记 PASS。

在用户明确授权本轮原生 CLI 调用额度后，运行 `bash xcli-cli-first-acceptance/verify-phase1.sh --native-smoke`；这将调用仓库已有 `tests/r0/smoke.py`，最多 6 次原生 CLI 进程启动，不能重试造成额外调用。若该脚本过时或 CLI 版本变化，先做只读修复及自测再执行。内部模型轮数/计费不完全可观测，**6 次 CLI 进程不等于 6 次模型请求**。

逐个验证三个 CLI：
- 已安装版本、原有登录可用性（不执行登录/注销/切换账号）、headless 输出、会话恢复、结构化事件、超时/取消；
- 文本仅使用无敏感信息的合成材料；输出必须有可独立判定的标记；
- 图片生成、参考图编辑：历史通过不等于现在通过。分别在明确授权额外媒体调用额度后操作，每个 CLI 最多一次生图和一次编辑；保留原始格式，验证真实文件字节、MIME、尺寸、摘要、原图未覆盖、实质视觉差异，不把模型写出的文件路径或 exit 0 视为媒体成功；
- 记录能力差异：仅输入图片、真正文生图、带参考图编辑、原生流增量、工具调用、JSON Schema、并发、进程启动延迟和本地权限限制，分别标 PASS/FAIL/BLOCKED/NOT_TESTED。
- 所有验收在临时可信工作区执行；不自动修改 CLI 全局配置、trust、approval、MCP、代理设置；不使用 `--yolo`、`--force`、`--dangerously-skip-permissions` 等全权限参数。记录输出必须脱敏，不保留凭据和原始聊天。

## 三、关键 Gate：CLI 到应用接口

如果现有 `xcli model` 不支持原生 CLI backend，必须指出 **Gate B BLOCKED（尚未实现）**。不得用“把模型服务 endpoint 指向 OpenAI API/Ollama”代替用户目标。

针对每个 CLI，评估可行的合法集成形态：
1. Codex 官方 SDK/app-server 或 CLI exec；
2. Cursor 官方 headless CLI / 支持的集成方式；
3. Antigravity 官方 headless / stream-json 输入模式。

评估用户本人本地运行、权限、隐私、费用与服务条款边界。为支持的场景提出最小 `NativeAgentBackend`/`Agent Task API` 设计；区分 **可真实映射到 Chat Completion 的文本结果** 和 **只能以 Agent 任务/文件产物交付的图片能力**。不能伪造 token usage、原生增量流、图片 b64_json、模型身份或严格 JSON Schema 语义。原生执行可读/写文件或运行工具时，明确隔离和审批边界；默认单用户 loopback、独立客户端授权、合成数据、限流/取消、真实文件校验。

本轮以验收和文档为主，不要未经授权发起大规模新框架重构。如无法合法/可靠映射为通用 Chat API，记录明确的技术阻断、替代 Agent API 及改造验收计划；**不以虚构 Gateway 已支持为成功标准**。仅允许修复验收中明确复现的最小缺陷；新增 CLI 桥接实现单独列为需要确认的工程任务。

## 四、验证结束立即生成交付文档

在 `docs/cli-first/` 下创建（文件路径可按仓库惯例微调）：
- `ACCEPTANCE_REPORT.md`：机器/版本/命令/Git SHA/真实证据/各 Gate 状态、已知缺陷、权限及能力限制。
- `XCLI_USER_MANUAL.md`：从零安装或源码构建、原生 CLI 登录前置检查（不导出凭据）、`xcli agents/doctor/run/resume/task` 的已验收启动方式；若 B 真实通过，再写 `xcli model start/status/stop` 与客户端 Token、真实可用模型 alias、完整请求和错误处理；若 B 未通过，必须显著注明 Gateway CLI 模式尚不可用并写清替代方案与未来接入步骤。包含 Mac 重启后如何启动、日常检查、故障排查和升级回退。
- `NATIVE_CLI_CAPABILITY_MATRIX.md`：Codex、Cursor、Antigravity 文本、图片输入、生成、编辑、流、会话、工具、结构化 JSON、真实与未验证矩阵；附历史验证与本轮实测区分。
- `INTEGRATION_ARCHITECTURE.md`：根据真实验收确定推荐接口（Agent API / 特定受支持模型接口）、JSON/SSE/图片字节契约、认证、路由、错误/取消、隐私、模型能力发现和示例客户端。
- `integrations/WORKLOG.md`：日报、周报、模板、结构化 JSON，何种入口可用、需要改哪些模块、断连与错误、Token/Keyring、安全与验证清单。
- `integrations/GARDEN_FLOW.md`：文本与生成/编辑图片、文件/媒体库导入、Vision、工具审批、能力门禁、实际文件与引用；不假定原生 CLI 图像等价于 `/images/generations`。
- `integrations/CHAT_INSIGHT.md`：对话/SSE、真实云推理位置和同意、批量 Embedding **不等于原生 Coding CLI 能力**（须有独立合法接口或标缺失）、重建索引和隐私失败关闭。
- `integrations/CONTRIBUTION_OS.md`：冻结 Profile、JSON Schema/Tool Calling、审批/审计、Run/Plan/Review、Docker 原生 CLI 与宿主桥接限制、不放宽网络/SSRF/权限。

四应用本轮**只做接入设计，不修改源仓库、不运行桌面 UI**。如果真正的 Gateway CLI 后端尚不可用，文档必须明确 `Planned/Blocked`，并按优先级给出所需 xcli 代码改动，不得提供会失败却声称可运行的配置。

## 五、最后交付

报告整体 PASS/FAIL/BLOCKED/NOT_TESTED 与下一步；先看是否有真实能力，然后才发布使用手册中的对应操作。所有文档、修复和证据都落在仓库，检查链接有效，保留 Git 工作区已有改动；不自行 push、release、安装服务或上线。修复有测试，未执行的测试不宣称通过。若遇阻塞，写出可以复现的最小命令与下一步，不要反复索取已知信息。

现在开始：首先核查当前实现和运行离线门禁，随后进行获授权的原生能力验证，最后生成上述文档。
