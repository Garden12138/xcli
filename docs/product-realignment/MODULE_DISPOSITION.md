# 模块审查与处置

审查基线 `a3e258e`。读取双语 README、R0–R3、cli-first、model-gateway、native-agent-gateway、model-access-gateway 的入口、架构、验收和对应代码/配置/测试。没有将历史未完成门禁当作核心产品阻塞，也没有凭目录名直接删除代码。

## M0–M5 引入的范围

| 阶段 | 非 CLI 编排扩展 | 现状与处置 |
| --- | --- | --- |
| M0 | model registry、能力元数据、独立 YAML、HTTP health/catalog、Host/Origin 边界 | 保留 `model/modelgateway/modelstore` 与配置；隐藏主帮助，默认拒绝服务启动。 |
| M1 | Chat/SSE、OpenAI-compatible HTTP backend、App Token、云位置授权、速率/并发 | 不再作为产品目标；保留原协议与客户端状态，不迁移或导出 Token。 |
| M2 | Vision、images generate/edit、multipart、PNG/JPEG/WebP 校验 | 通用媒体服务冻结；低层字节/MIME/像素校验可用于原生文件验收。 |
| M3 | Embedding passthrough、向量维度/索引指纹、云同意 | 与 Coding CLI 编排无直接关系，冻结；不把 Agent 文本假装成向量。 |
| M4 | 四应用协议画像、接入补丁和严格参数/工具透传 | 保留历史补丁和实测范围，不回放到当前四应用；当前指南只设计 Task。 |
| M5 | model start/status/stop/logs、OS 锁、私有文件、日志审计、Windows 支持 | 不删除；状态/停止/日志继续可用，启动需实验 opt-in。核心共享进程/锁实现保留。 |

参考：[M0–M5 原始验收](../model-gateway/acceptance/M0.md)、[完整阶段状态](../model-gateway/PROGRESS.md)、[历史交付总结](../model-gateway/SUMMARY.md)。旧应用改造/部署记录保持当时事实；R4 未操作外部仓库或线上服务。

## G1–G8 成果与冻结原因

| 阶段 | 已形成成果 | 冻结原因与可复用部分 |
| --- | --- | --- |
| G1 | 独立 Agent API、策略、journal、客户端/Session/预算合同 | HTTP Gateway 不是当前主产品；journal/CAS/幂等/预算检查可借鉴。 |
| G2 | HTTP→固定 argv 假 CLI、Session、事件、取消与持久任务 | 另造单 turn 调度器并未调用核心 Task Runner；不以此替代现有 Task 编排。可复用进程监督经验和协议拒绝测试。 |
| G3 | Artifact 输入/文件 FD 导入、格式/摘要、preview/review/下载 | 通用媒体 HTTP 冻结；安全真实文件交付和来源/接收分离有复用价值。 |
| G4 | 故障注入、并发、崩溃收敛、容量/事件/存储硬化 | 保留测试和实现；不把假进程 PASS 当真实原生授权或安全隔离。 |
| G5 / G5A.1 / G5A.2 | 官方协议审查、受限控制、授权回执、只读探针与凭据边界证据 | 本路线扩展到供应商使用资格和工具/凭据隔离；真实 Profile 未就绪，不继续堆探针。 |
| G6 | Codex app-server stdio adapter、deny-only RPC、取消/生命周期合同 | 独立原生 stdio 实验，macOS 故障与全工具认证隔离未解决；不推进至核心运行入口。 |
| G7 | 独立容器 transport、宿主文件隔离、假 peer 生命周期 | `PARTIAL_HOST_ISOLATION_ONLY`，凭据/全工具分离未成立；保持默认关闭，停止实验。 |
| G8 | 独立 SIWC PKCE/ID/scope/轮换、Responses/SSE/目录/历史纯契约及 Mock | `OFFLINE_CONTRACTS_PASS / PRODUCTION_BLOCKED`；OAuth/统一模型订阅代理已退出产品目标，不进入 G8.1。无生产 Token/Keychain/Provider。 |

原证据入口：[G1–G4 报告](../native-agent-gateway/IMPLEMENTATION_REPORT.md)、[G5–G7 进度](../native-agent-gateway/IMPLEMENTATION_PROGRESS.md)、[G7 决策](../native-agent-gateway/G7_DECISION.md)、[G8 验收](../model-access-gateway/ACCEPTANCE.md)。这些目录中的 source-manifest 属于各历史提交，R4 修改文档后不应拿当前文件重新宣称旧摘要匹配。

## 调用关系与依赖

核心调用图：`cmd → task/workflow → agent/runtime/runstore/worktree/jobs/config`。核心任务不调用 `modelgateway`、`agentgateway`、`siwc` 或 app-server。命令装配时 `cmd/root.go` 仍注册两个隐藏实验命令，因此当前单二进制仍编译/链接保留包，不是物理分库。

实验调用图：`cmd/model* → modelgateway/modelbackend/modelstore/model`；`cmd/agent_gateway* → agentservice/agentstore/agentpolicy/nativeadapter/agentartifact`，其中多个包依赖 `modelstore` 的私有文件/锁、`media` 的图片检查。`modelstore` 引用 `siwc` 只为保留 Provider 的强拒绝。不能删 `modelstore` 后期望 NAG 仍构建。

| 依赖 | 当前用途 | R4 处理 |
| --- | --- | --- |
| cobra / pflag / mousetrap | 核心命令/跨平台 argv | 保留。 |
| yaml.v3 | 核心与实验各自配置 | 保留 schema，不自动改写。 |
| x/sys | 核心 Windows process/jobs/task lock，以及实验 DACL/锁/no-follow | 保留，不能作为 Gateway 专属删除。 |
| x/image | M2 引入 WebP 解码，现供 `media` 及实验文件验收 | 因保留源码及未来真实产物校验而保留；没有新增媒体供应商依赖。 |
| SIWC SDK/Keychain/Ollama | 当前无这类新增外部依赖 | 不引入、不安装。 |

## 配置、命令与数据兼容

核心 `config.yaml`、Task v1/v2、Run/Jobs、旧 workflow worker、MCP launcher 和 worktree 不迁移不删除。`auth_args`、旧 ACP/routing 字段兼容读取，但不恢复已退役的登录/路由命令；Doctor 测试证明 auth_args 不执行。

独立 `model-gateway.yaml`、`agent-gateway.yaml` 及 `model-gateway`/`agent-gateway` 私有数据根保留。R4 的 `--experimental-gateways` 是显式单次 argv，不写新 YAML 字段或默认环境变量。没有 flag 时，服务 start/serve、创建/轮换客户端授权、probe/prepare/enable 等默认失败，且不读取配置/创建状态。查询 list/doctor/status/logs/preflight/readiness 和 stop/revoke/disable 保留原行为；它们不代表生产就绪，也不保证完全无元数据写入。

已有正在运行的旧二进制不会被 R4 自动杀死。用户可用 `xcli model status/stop`、`xcli agent-gateway status/stop` 收尾，不必先打开实验服务。若本人确需重现旧离线工程，在原命令前显式加 `--experimental-gateways`；这仅恢复旧入口，不绕过 Native 或 SIWC 的原门禁。后台 model start 会把该显式选择传给自己的 serve 子进程；不靠继承环境自动启用。

## 是否拆分

建议后续将 Model/媒体 HTTP、NAG HTTP/app-server/隔离实验，以及 SIWC 分别放入明确的实验项目或开发分支；核心只保留确认需要的中性文件/日志/进程库。**本轮不创建远程仓库、不切分历史、不新建 OAuth 产品。**

拆分前先抽取共享存储与媒体校验，再保留数据导入/只读查询/停止工具，明确旧二进制与状态版本，验证旧配置/客户端撤销和测试归属。实际迁移用户数据或删除代码需另行审查，不能因产品冻结立即删除。
