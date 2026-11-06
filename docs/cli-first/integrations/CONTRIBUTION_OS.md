# Contribution OS 接入适配（Planned / Blocked）

2026-11-06，只读核对 HEAD `8aa905151329f03e32397c3d1e057bf2a0ac2950`；已有 7 项用户修改，本轮保留。不修改源码、不启动 Docker/业务服务、不部署、不运行 UI。

**xcli 原生 CLI Agent API 接入 BLOCKED，Docker → 宿主 CLI 桥接也未在本轮实现/启用。** 当前 `xcli_local` 只接已有 HTTP Model Gateway，不能通过把 model 改成 Codex 就获得原生 Agent 能力。

## 当前源码边界

已核对 `app/model_settings.py`、`app/providers/xcli_local.py`，定位 `contracts.py`、`gateway.py`、`gateway_http.py`、`runtime.py`、`workflow.py`、`codex_cli.py` 等相关模块。

`xcli_local` 要求独立登记的 loopback endpoint、显式端口和 `/v1` 路径；只允许 `/models` 与 `/chat/completions`，限制请求/响应、禁代理继承/重定向/自动重试，发现容器会拒绝。其 `require_model` 要求冻结 alias 与所需能力，对未经验证的 Schema/Tools 和 stream 请求拒绝。它不是 NativeAgentBackend。

应用已有 `codex_cli.py` 直接执行原生任务的代码；其存在也不证明经由 xcli 的 Agent API 或 Model Gateway 已通过。本轮未执行该 provider、Plan、Review、发布或真实仓库任务。

本仓库历史认证宿主桥接补丁针对其他 HEAD/HTTP 模型服务，不能重放到当前有用户修改的源码，也不证明它能安全承载原生 CLI 的文件/工具权限。见[历史桥接边界](../../model-gateway/integrations/contribution-os-host-bridge.md)。

## 冻结 Profile 与 Run/Plan/Review

拟议新增独立 Agent provider，不复用 `xcli_local` 的协议名。任务创建时冻结 Profile：Agent ID、版本与已验证模型身份（未知为空）、推理位置、能力证据版本、客户端授权引用、工作区/工具策略、进程与时间预算、合同版本。Profile 不存 Token 明文或原生登录资料。

| 阶段 | 拟议边界 |
| --- | --- |
| Plan | 只读合成仓库/固定材料；输出本地 Schema 验证过的计划，不能执行计划中的外部写入 |
| Run | 只在专属隔离工作区做已批准的动作；真实命令验证与原生任务结果分开记录 |
| Review | 使用固定输入与 diff、独立只读权限，不能让审查者通过工具修改被审内容 |
| 接收/发布 | 由应用已有审批执行；xcli 成功不自动授予 merge、push、issue/PR 发布或凭据访问 |

恢复使用原冻结 Profile 和剩余累计预算；版本/模型/权限变化时阻止无声继续，并要求独立确认新的执行条件。不自动换供应商、账号、Schema 版本或私网 endpoint。

## Schema 与 Tool Calling

原生 JSONL 事件与业务 JSON 是两个合同。Plan/Review 的 JSON 必须解析、Schema 校验和语义验证；原生 `--output-schema`/`--json-schema` 是否严格生效需逐 CLI 实测。严格条件无法满足时返回 capability_missing/validation_failed，不能修饰能力目录使其通过。

Agent 内部 shell/MCP 调用不等于应用可执行的 `tool_calls`。应用工具由现有白名单、参数 Schema、审批与审计约束；模型不能设置新的 host mount、网络权限、发布凭据或任意 cwd。工具请求与执行结果绑定任务/步骤/审批记录，拒绝仍是拒绝，不重试为强制模式。

审计只保存任务/步骤 ID、Profile 摘要、版本、动作类别、审批结果、退出/错误、产物/验证摘要、原生上报的用量及其范围。原生用量未知为 null，不伪造 token 或成本。原始仓库文件、秘密、完整原始聊天/工具载荷不进入公共日志。

## Docker 与宿主桥接

容器内 `127.0.0.1` 指向容器自身；把 endpoint 改成 `host.docker.internal`、LAN、`0.0.0.0` 或放宽 SSRF/网络策略不是解决方案。不得挂载宿主原生登录目录、Keychain、Docker socket 或全盘到应用容器。

若未来选择宿主 Agent 服务，必须独立实现与验收认证桥接：服务只监听本人控制的本机边界，固定实例/客户端证书或等价强认证、专属客户端授权、短期任务权限、步骤/预算限制、安全 Artifact 通道。不把 shell 字符串、任意环境、路径/URL 或原生 Session ID 直接从容器转交宿主。

宿主代理和容器各自执行既有授权检查；Model Gateway/API/Sandbox Worker/Publisher 权限不合并。已有公网 HTTPS 与 SSRF 校验保持原样，单独的桥接类型不能扩成“允许任何私网 URL”。宿主取消需核实实际进程/任务归属，禁止按历史 PID 任意 kill。身份、证书、实例、挂载或副作用状态不明时失败关闭。

该桥接本轮没有实现、安装、信任登记或生产证书操作；不能根据旧补丁和 OrbStack 历史合成记录称当前 Docker 原生 CLI 可用。

## 待确认工程优先级

| 优先级 | 变更 |
| --- | --- |
| P0（xcli） | 实测三 CLI 原生文本/媒体，确认 Agent 服务的审批/隔离/取消与文件合同 |
| P1（xcli） | [独立 Agent API](../INTEGRATION_ARCHITECTURE.md)、客户端认证/会话归属与真实 HTTP→CLI 合成验收 |
| P2（应用） | `model_settings.py` 增加冻结 Agent Profile/能力来源，`providers` 新增独立 Agent adapter，不把旧 HTTP transport 扩权 |
| P2（应用） | `runtime.py`/`workflow.py`/`contracts.py` 接入任务生命周期、Schema 验证、审批和预算审计；保留既有原生 provider 的独立边界 |
| P3（另立任务） | 按当前 HEAD 设计认证宿主桥接、挂载/网络/证书方案，隔离验证后再审批部署 |

这些只是待确认改造，不修改业务仓库或给出声称可运行的 CLI 配置。工程前重新核对用户修改，绝不强制 apply 历史补丁。

## 后续验收（未执行）

- [ ] 冻结 Profile 后修改 alias/实例/能力/模型身份/授权或预算会拒绝继续。
- [ ] JSON 无效/Schema 不符/未知工具/未批准工具/伪造完成事件不会变成成功 Plan/Review。
- [ ] 跨任务会话、越界文件/URL、过期授权、重放、并发与取消均保持原有隔离。
- [ ] Docker 原生 transport 明确拒绝；宿主桥接无登记/错误证书/失效实例/私网改写/SSRF 失败关闭。
- [ ] 真实小额调用只使用合成仓库与非发布任务；真实 Run/Plan/Review、Docker、部署和 UI 需单独授权。
