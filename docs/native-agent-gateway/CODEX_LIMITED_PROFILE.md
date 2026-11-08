# Codex 最小受限 Profile（G5A.1）

2026-11-08，基线 `6e2ecd7`。**控制路径、合成工作区、无模型原生 canary、离线集成已实现；真实启用 Gate 仍 BLOCKED，不能标 READY_FOR_AUTHORIZATION。** 真实模型调用 0。Cursor/Agy 的生产 Adapter、Trust、账号和配置不变。

## 三种集成模式

| 模式 | 本轮处理及边界 |
| --- | --- |
| 本人本机使用既有 Codex CLI 登录做个人 Agent 任务 | 本轮候选：官方 `codex exec`，只注册一个 auth-home 和合成 Git 项目。仅官方进程读取其认证；xcli 不打开、复制或导出凭据。只读 login status 报告 ChatGPT 登录；账号所有权、当前计划／目标模型权益及具体用途仍需确认。不是“登录成功就获得全部应用授权” |
| 应用 SIWC → Codex app-server | 应用须自行取得官方 OAuth、plan usage 同意、注册与续期。不能挪用 CLI 登录凭据替代应用授权；本轮未实现或使用该路线。[官方 SIWC 概览](https://developers.openai.com/siwc/token-sharing-open-source)、[app-server 接法](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server)、[官方教程](https://developers.openai.com/cookbook/articles/sign-in-with-chatgpt) |
| 对其他应用提供通用模型推理服务 | 不属于本 Profile；不提供通用 prompt、模型选择、账号池或 `/v1/*` 兼容接口。公开 headless／SIWC 文档不能视为任意共享服务许可 |

## 已实现的控制面

- 默认关闭；YAML `codex_limited.enabled` 与本地 `enable` 是启用意向，**不是推理授权**。只有所有工程／用途条件可验证时才允许启用；当前 `enable` 明确拒绝。
- `authorize --authorization-ref` 是另一步骤，最多 15 分钟，当前未通过启用 Gate 时也拒绝；不从旧额度或 YAML 自动继承调用许可。
- `disable` 可在 serve 运行时原子撤销开关及调用许可；worker 每 25ms 检查并取消，有界清理最多另 5s。它不是云端已停止的证明。修改 YAML 需重启，实时撤销使用 disable／client revoke。
- 控制文件写入 fsync、原子替换、目录 fsync；本地管理锁串行化。未知／损坏／不匹配状态失败关闭，重新启用不退款或重置已用预算。
- 单个登记 workspace、固定材料和固定内联 prompt，text_task-only；Session 预算固定 1/1/120，Task timeout=120。每个登记 Profile 的 lifetime 启动数由原 journal 的 `limited/<registration SHA>` 计数约束为 1，跨 Session、重启、换幂等键、跨日不恢复。并发仍为全局 1。
- 指定客户端 ID、原有 Bearer scope/撤销/owner/CAS/幂等与 dispatch 前持久预扣；不允许恢复原生 Session、传文件或任意 prompt，更不接受 HTTP argv/env/binary/cwd/model。
- HTTP 合同和错误枚举不变；新的 policy_id 为 `codex-limited-readonly-v1`。未验收 Profile 不在目录冒充 available。旧二进制不识别新 policy 时拒绝续接，不迁移旧 Session。

## 工作区与真实隔离证据

`prepare` 只在独立状态根创建新项目：`codex-limited/workspace`。`git init --template=` 禁用全局 Git 配置与 hooks 模板；不使用 xcli 源码、已有个人项目或公司项目。固定材料 `material.txt` 内容为 `XCLI_G5B_CODEX_SYNTHETIC_20261108` 加换行。项目只允许材料和 `.git/HEAD`、`.git/config`，每次检查全量路径／字节摘要；额外配置、hooks、符号链接、硬链接、材料变化均拒绝。文件读取复用 no-follow FD 校验。

实际项目：`/private/tmp/xcli-g5a1-state-9a6vyye3/codex-limited/workspace`。它是私有临时合成项目，没有登录文件副本，没有运行中的 Gateway；临时目录未来可能被系统清理，不能把恢复临时项目当自动重发授权。

[最终原生探针](evidence/g5a1/final-probe.json)使用 Codex 0.167.0、已登记 executable hash，直接运行官方 `codex sandbox`，**没有启动 exec turn**：

| 检查 | 实际结果 |
| --- | --- |
| login status | AUTHENTICATED_CHATGPT；不记录账户名、账号 ID、Token、余额或原始诊断 |
| exec 参数格式帮助 | ignore-user-config / ignore-rules 存在；不是完整配置生效证明 |
| 登记材料读取（阳性对照） | exit 0，字节严格相同 |
| 未登记合成文件读取 | exit 1，拒绝 |
| 工作区创建文件 | exit 1，拒绝，文件未出现 |
| 符号链接指向外部合成文件 | exit 1，拒绝 |
| 工具请求临时 loopback canary | curl exit 7，接收计数 0 |

使用 `:root=deny`、`:minimal=read`、登记目录 read、tmp deny、工具 network=false；阳性对照失败时所有“拒绝”均不能计 PASS。[官方权限文档](https://learn.chatgpt.com/docs/permissions)区分命令边界与独立工具／服务流量，[sandbox helper 文档](https://learn.chatgpt.com/docs/developer-commands)说明其验证范围。`:minimal` 包含系统运行时例外，不声称只能读一个文件。

**这些 PASS 仅覆盖原生沙箱中的本地命令。** 当前 exec 是单向事件适配器，不能在执行前仲裁所有工具；`--ignore-user-config` 与 MCP/apps/plugins/hooks 的固定候选配置，尚无同版本、零推理的完整有效工具清单／认证根访问隔离证明。不能把关闭配置字段、shell canary 或退出 0 当成所有工具已隔离。

因此源码 `nativeToolBoundaryVerified()` 明确返回 false；编辑启用布尔值、控制文件或伪造探针 JSON 均不能晋升真实 Profile。该验证器未完成是**真实工程阻塞**，不是“只差用户批准一次调用”。下一工程需要证实当前 exec 所有暴露工具的执行边界，或在独立审查后增加能提供所需证据的官方 transport；不在本轮自动切换 app-server。

## 配置和命令

完整配置见 [examples/codex-limited-gateway.yaml](../../examples/codex-limited-gateway.yaml)。默认如下，true 也不会绕过未满足的工程 Gate：

```yaml
version: 1
host: 127.0.0.1
port: 0
queue_limit: 1
daily_dispatches: 1
daily_process_starts: 1
daily_execution_seconds: 120
codex_limited:
  enabled: false
  client_id: codex-acceptance
  purpose: personal_local_synthetic
  provider_authorization_ref: ""
```

```bash
GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off go build -trimpath -o /tmp/xcli-g5a1 .
NAG_STATE=/private/tmp/xcli-g5a1-state-9a6vyye3
NAG_CONFIG="$PWD/examples/codex-limited-gateway.yaml"
/tmp/xcli-g5a1 agent-gateway --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" codex-limited readiness
```

为新的独立根登记时，`prepare --binary /精确官方/codex --auth-home /本人选择的规范CODEX_HOME`，随后显式 `probe`。prepare 只执行一次 --version 和 git init；probe 为 7 次非推理原生调用。必须先审查精确路径，不自动寻找登录目录。新根不能重复导入已有项目；现有登记不能就地覆盖。当前只验证 0.167.0，版本变化关闭并另行审查。

新独立根的本人专用 Token 创建示例（只有此身份可使用该 Profile；仍不能使 native Gate 通过）。本轮实际临时测试根已经有 `codex-acceptance`，Token 仅存 `$NAG_STATE/client-secrets/codex.token`，不重复 issue 或覆盖：

```bash
/tmp/xcli-g5a1 agent-gateway --state-dir "$NAG_STATE" client issue codex-acceptance \
  --agent codex --policy codex-limited-readonly-v1 --operation text_task \
  --scope session:create,session:read,session:close,task:create,task:read,task:cancel \
  --allow-remote --daily-dispatches 1 --daily-process-starts 1 \
  --daily-execution-seconds 120 --token-file /本人私有0700目录/codex.token
```

未来证明所有工程前置且用途确认后，显式设置 enabled=true、记录 provider_authorization_ref，再依次调用 `codex-limited enable` 和另行获批的 `codex-limited authorize --authorization-ref <本轮新授权引用>`。**当前两条命令都会失败关闭，不能用手改 JSON、Native=false 或删除检查代替。** 一次进程可有多次内部推理／重连，费用与模型内部调用数不是启动计数可精确约束的对象。

关闭：`agent-gateway --state-dir "$NAG_STATE" codex-limited disable`；撤销客户端：`client revoke codex-acceptance`；停止服务：`stop`。崩溃恢复只查状态，不补发；本地／云执行事实未知时保留 needs_reconcile，不清空根或预算来重做。

## 交付与未通过的门禁

[完整离线日志](evidence/g5a1/offline.log)及[源码／测试摘要](evidence/g5a1/validation.json)已通过；[真实安装的 HTTP 拒绝测试](evidence/g5a1/native-gateway-rejection.json)确认配置意向不会绕过验证器。实际仅本地验收客户端已准备，无真实调用许可。179 个源码文件摘要固定为 `ac62544bea11f79f9f5a5309f59dfa35a9225729078692d6ae1c05f465d0d417`；最终实现 Git SHA 见 [进度交接](IMPLEMENTATION_PROGRESS.md) 末尾。

剩余项分别为：内置工具／认证根隔离验证器（工程 BLOCKED）、账号／计划／用途适用性确认（尚未完整证实）、本轮 1 次真实调用授权（未授予）、真实 HTTP L01-a（NOT_TESTED）。前两项没有解决前不能仅凭第三项运行；原 Gate B 始终 BLOCKED。
