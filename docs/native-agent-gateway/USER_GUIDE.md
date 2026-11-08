# Native Agent Gateway V1 使用与接入

2026-11-08（G5A）。**已交付独立服务、无模型预检与可复现的 HTTP→假 CLI 离线链路；三家真实 native profile 均关闭。** 当前不能把本服务作为已验收的原生推理入口。真实 HTTP→CLI、真实取消/媒体以及四应用 UI 仍待验收。没有 OpenAI 兼容路由、Embedding、自动 fallback、账号池或工具批准接口。

## 安装、启动和停止

在本仓库根目录，使用 Go 1.26 与已有依赖构建：

```bash
GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off \
  go build -trimpath -o /tmp/xcli-nag .
/tmp/xcli-nag agent-gateway --help
```

这会生成本地可执行文件，不安装系统启动项。需要长期保存时可将构建目标设为本人选择的私有工具目录；本轮没有写系统 bin 或安装后台服务。

```bash
NAG_CONFIG="$PWD/examples/native-agent-gateway.yaml"
NAG_STATE="${XDG_DATA_HOME:-$HOME/.local/share}/xcli/agent-gateway"
/tmp/xcli-nag agent-gateway --gateway-config "$NAG_CONFIG" --state-dir "$NAG_STATE" serve
```

`serve` 在前台持续运行。另一个终端检查/停止：

```bash
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" status
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" stop
```

Ctrl-C/TERM 也会停止接收并清理当前 worker。`stop` 通过实例+进程代次的私有请求停止服务，不对历史 PID 发信号；不表示云任务一定撤销。V1 没有系统 daemon 或 `start` 子命令。每次重启 instance_id 保持、generation_id 改变；更换状态根则改变实例，不能跨实例续接。

独立默认配置：`$XDG_CONFIG_HOME/xcli/agent-gateway.yaml`，否则 `~/.config/xcli/agent-gateway.yaml`；默认数据根：`$XDG_DATA_HOME/xcli/agent-gateway`，否则 `~/.local/share/xcli/agent-gateway`。使用 `--gateway-config`，不复用全局 `--config` 的旧配置语义。默认日预算 0；改预算也不能绕过关闭的真实 profile。不要把 Model Gateway 根、运行记录根或业务目录当作新数据根，namespace 检查会拒绝非空未知根。

## 每个应用独立 Token

Token 为 256-bit 随机值，只在新建私有交付文件中出现；服务只存摘要。不使用 native 登录 Token，也不接受 Model Gateway Token。

```bash
NAG_SECRET_DIR="$HOME/.local/share/xcli/agent-client-secrets"
mkdir -p -m 700 "$NAG_SECRET_DIR"
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" client issue worklog \
  --agent codex --policy synthetic-text-readonly-v1 --operation text_task \
  --scope session:create,session:read,session:close,input:write,task:create,task:read,task:cancel \
  --allow-remote --daily-dispatches 2 --daily-process-starts 2 \
  --daily-execution-seconds 300 --token-file "$NAG_SECRET_DIR/worklog.token"
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" client list
```

已有公开权限目录会被拒绝，不回退写公开目录。示例授予的是客户端上限；服务全局预算仍为 0、真实 profile 仍关闭。四应用应由后台安全存储 Token，renderer/网页不接触凭据。GardenFlow 可单独授予 image_generate/image_edit 与 task:review/artifact:read；普通应用不授予 `artifact:attest-origin`。

```bash
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" client rotate worklog \
  --token-file "$NAG_SECRET_DIR/worklog-rotated.token"
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" client revoke worklog
```

轮换保留 client_id/对象归属并使旧 Token 失效。撤销阻止读写，并由监督器取消活跃 turn；已排队任务不能因再次授权而复活。API 不提供 Token 管理路由。已撤销的 grant 不能静默轮换恢复。本人审查后，可在 rotate 时另加 `--reauthorize-revoked` 恢复原授权并交付新 Token；预算累计和撤销 epoch 保留，不恢复旧队列。

可选非模型 refresh 只能在停服时由本人显式执行。G5A 中 Codex 0.167.0、Antigravity 1.5.2 的真实 `--version` 已通过；Cursor 独立版本可返回，但 Gateway refresh 为 `version_probe_stop_unconfirmed`，没有写入缓存：

```bash
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" refresh \
  --agent codex --binary /absolute/path/to/official/codex
```

它只运行固定 `--version`、有界超时/输出、最小环境并缓存二进制摘要，不登录、不读凭据、不启动任务，也不能启用 profile。当前网页上的新参数不能当作本机版本支持证明。

停服控制面的安全预检不启动 CLI，不发送任何 prompt：

```bash
/tmp/xcli-nag agent-gateway --gateway-config "$NAG_CONFIG" --state-dir "$NAG_STATE" \
  preflight --agent codex
```

`--agent` 也可为 `cursor` 或 `antigravity`。报告分别列出配置、缓存、选中文件 hash、策略、认证环境、账号／应用授权及真实验收；Cursor 另列历史 Trust 阻塞。退出 0 表示报告生成，当前 `can_execute` 始终为 false。缓存／hash PASS 不证明 runtime 依赖、账号权益或沙箱有效；不支持通过 JSON/YAML 修改报告启用真实 Profile。

三家的无 turn stdio 初始化已验证；Codex 的本机 schema 已生成。ACP/app-server 尚无生产运行 adapter，不能作为 serve 的可选开关；Cursor ACP 的 EOF 退出还有阻塞。详见 [G5A 启用前状态](G5A_PROFILE_READINESS.md)和[官方授权审查](OFFICIAL_INTEGRATION_REVIEW.md)。这些探针不是 HTTP 推理验收。

## 实际 HTTP 路由

所有业务路由前缀 `/agent/v1`，固定 literal loopback endpoint；仅 `/healthz` 可未认证。只接受一个 Bearer header，拒 Origin/代理转发/查询参数，禁止浏览器直连与容器宿主桥接。

| 方法与路径 | 行为 |
| --- | --- |
| GET `/healthz` | 进程存活；不证明 native 可用 |
| GET `/agent/v1/capabilities` | 当前客户端可见的缓存、权限与阻断；0 native 探测 |
| POST `/agent/v1/sessions` | 创建逻辑 Session；0 native 启动 |
| GET `/agent/v1/sessions/{id}` | 查询 owner 的 revision/context/budget，不公开 native ID |
| POST `/agent/v1/sessions/{id}/close` | 停止接受并取消活跃任务；未知副作用保留封存 |
| POST `/agent/v1/inputs` | 原始 UTF-8/PNG/JPEG/WebP 字节上传与哈希验证 |
| POST `/agent/v1/tasks` | 接受一个 turn；202 不是成功 |
| GET `/agent/v1/tasks/{id}` | 持久状态、执行/协议/验证、结果与 nullable usage |
| GET `/agent/v1/tasks/{id}/events` | SSE，Last-Event-ID 重放，不创建 turn |
| POST `/agent/v1/tasks/{id}/cancel` | 取消；不会将本地停止写作云撤销 |
| POST `/agent/v1/tasks/{id}/review` | 字节/来源/视觉接收；0 模型调用 |
| GET `/agent/v1/artifacts/{id}` | owner 的产物元数据与交付状态 |
| GET `/agent/v1/artifacts/{id}/content` | 认证预览/ready 原始字节与强 ETag |

修改请求都带稳定 `Idempotency-Key`。跨客户端对象 404，无 Token 401，缺 scope/同意 403，CAS/并发冲突 409，本地预算/队列限制 429，未验证/缺失能力 422，策略不可实施 503。当前对真实 profile 创建 Session 会返回 `policy_unenforceable`；不是“模型失败”。不要用另一 CLI 或普通模型 API 代替。

## 调用示例与恢复规则

下面结构遵循实际路由，但真实 Session/Task 示例须先满足 [G5B 逐 CLI 计划](G5B_REAL_HTTP_PLAN.md)的工程和新授权条件。首次文本 1 次与原 L01 文本＋续接 2 次分别授权。当前默认配置不会完成真实推理；现在不应执行下面 Task 示例来尝试启用。

不要把 Token 放进 curl argv/env。先在上述 0700 目录创建 header 文件（不会打印秘密）：

```bash
python3 - "$NAG_SECRET_DIR/worklog.token" "$NAG_SECRET_DIR/worklog.header" <<'PY'
import os, sys
from pathlib import Path
value = Path(sys.argv[1]).read_text().strip()
fd = os.open(sys.argv[2], os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as f:
    f.write('Authorization: Bearer ' + value + '\n')
PY
curl --proxy '' --noproxy '*' --max-redirs 0 --max-time 5 \
  --header @"$NAG_SECRET_DIR/worklog.header" \
  http://127.0.0.1:18742/agent/v1/capabilities
```

客户端应先检查目录/实例/权限和本次云同意，再读取私有材料。以下 JSON 分别保存为请求文件，占位符必须来自当前目录和 owner Session；不能填写历史 native ID：

```json
{
  "agent_id": "codex",
  "adapter_id": "codex-exec-v1",
  "capability_revision": "<current directory revision>",
  "policy_id": "synthetic-text-readonly-v1",
  "budget": {"max_dispatches": 2, "max_process_starts": 2, "max_execution_seconds": 300},
  "context": {"app": "worklog", "workflow": "report-draft", "opaque_ref": "synthetic-period"}
}
```

```bash
curl --proxy '' --noproxy '*' --max-redirs 0 --max-time 10 \
  --header @"$NAG_SECRET_DIR/worklog.header" --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: session-example-001' --data-binary @session.json \
  http://127.0.0.1:18742/agent/v1/sessions
```

```json
{
  "session_id": "<returned logical Session id>",
  "expected_session_revision": 1,
  "operation": "text_task",
  "input": [{"type": "text", "text": "总结这份合成材料，不执行外部动作。"}],
  "budget": {"max_dispatches": 1, "max_process_starts": 1, "timeout_seconds": 120},
  "consent": {"allow_remote": true, "context_sha256": "<current Session context_sha256>"}
}
```

```bash
curl --proxy '' --noproxy '*' --max-redirs 0 --max-time 10 \
  --header @"$NAG_SECRET_DIR/worklog.header" --header 'Content-Type: application/json' \
  --header 'Idempotency-Key: task-example-001' --data-binary @task.json \
  http://127.0.0.1:18742/agent/v1/tasks
```

保存 task_id。后续 GET Task/SSE 不提交新请求。SSE 断线先查同一 Task，以 `Last-Event-ID` 续读；过窗 410 时用响应的 `X-Xcli-Latest-Event-ID` 与 GET Task 恢复。没有原生 delta 时只有聚合活动和最终引用，不能拼 final 冒充流。慢 SSE 观察者会断开，worker 不依赖网络 sink。

若 POST 遇到 EOF/超时（包含不确定 fsync 的主动断连），只允许重放**原键、原路由、原正文**来查原对象，不生成新键重做。幂等保留 30 天，结果/事件/产物 7 天；过期不是重新执行授权。取消用 `{}` POST `tasks/{id}/cancel`，新续问是 GET 最新 Session 后 POST 新 Task，携带新 revision/context 摘要与新明确预算/同意。

媒体上传用 `--data-binary @selected.png`，header 为 `Content-Type:image/png`、`X-Xcli-Session-ID`、`X-Xcli-Content-SHA256` 与幂等键；API 不接本机路径。image_edit 仅一个已上传参考，结果必须另存。review 请求示例：

```json
{
  "decision": "accept",
  "artifact_id": "<review_only artifact>",
  "expected_sha256": "<downloaded original bytes hash>",
  "semantic_check": "edit_matches_requested_change"
}
```

来源不明时普通 Token 返回 `provenance_unverified`。本人检查本任务官方原生工具痕迹后，才可用另授予的 attest scope 和 `provenance_attestation:{source:"native_task_trace",evidence_sha256:"..."}` 补证；图片看起来正确不等于来源已证明。只有 Task succeeded + Artifact ready 后，应用才可认证下载、复算 MIME/尺寸/hash 并经本人确认入库。预览也要认证，取消/过期/隔离产物不可下载。

## 离线复现与故障恢复

```bash
bash tests/native_agent_gateway/verify-offline.sh
GORACE=atexit_sleep_ms=0 GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off \
  go test -race -run '^TestLoopbackHTTPToCLIToPersistedResult$' ./internal/nativeadapter -v
```

这些只运行假原生协议子进程、假上游和明确标记的图片夹具；需要本机 loopback 测试权限。不要运行旧 `tests/r0/smoke.py --execute`、native_media 或 phase0 的真实 smoke 来“补验”。

崩溃后重新 `serve`：未 dispatch 队列封存为 service_restarted，已 dispatch 且无完整终态为 needs_reconcile；不会按旧 PID kill 或补发。未知本地 worker 会使整根隔离，关闭服务不能证明云端撤销。检查脱敏 Task/事件与官方本任务记录，不读取认证缓存；V1 无自动 reconcile/继续接口，不手改 journal/预算或用清空根恢复额度。

正常到期会删除选材/工作区和产物，过期正文从 journal/checkpoint 压缩移除。无法确认仍在运行的 worker 的工作区保留隔离，需本人先解决执行事实；不把清理目录当作取消。厂商/系统备份/云端历史不受本地清理承诺控制。journal/watermark 损坏时保留根并停止，不能删除日志重置预算；尚未发布的旧预览根没有自动迁移。

## 四应用后续入口

四应用本轮均未修改。按 [APP-NAG-1 与模块入口](MIGRATION_AND_COMPATIBILITY.md)另立实现：Worklog 的 `src-tauri/src/llm.rs` 附近加入草稿 transport/本地 Schema；GardenFlow 的 `xcliGateway.ts` 旁接 Task→review→Artifact 两阶段入库；Chat Insight 的 `src-tauri/src/ai/client.rs` 旁接选段总结，Embedding/索引独立；Contribution OS 新增 `xcli_agent` provider，只读 Plan/Review，Run/发布/Docker 桥接另立范围。
