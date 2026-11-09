# G6：Codex app-server 最小纵向集成

2026-11-09，基线 `15b608a`，G5A.1/G5A.2 证据不覆盖、不重做。本阶段按用户新定义使用 G6 名称，旧路线图中的四应用接入仍未实施。结论为 **ISOLATED_RUNTIME_REQUIRED**，不是 APP_SERVER_ENGINEERING_READY；真实推理 0。

## 已实现的路径

`HTTP /agent/v1 → 现有 Service/worker → Exec 按 adapter_id 分派 → AppServer → 有界 stdio JSON-RPC → 既有 Task/Session/journal/SSE`。

每个 Task 一个子进程，无常驻共享原生进程、自动重连、重发、换账号或 fallback。一个 Task 仍在 dispatch 前预扣一个进程启动并预留执行时间；原生内部请求、重试、Token 与费用不可据此精确计数。线程续接消耗新 Task／新进程预算。

| 方法／通知 | 实现及映射 | 实际证据 |
| --- | --- | --- |
| initialize → initialized | 固定 xcli clientInfo；experimentalApi=false，ID 关联 | 假 peer＋真实无推理 PASS |
| thread/start | 固定服务工作区、approvalPolicy=never；保存返回原生 ID | 假 peer PASS；真实 NOT_TESTED |
| thread/resume | ID 仅从 Session 私有记录读取；不同 ID 失败封存 | 假 peer 成功续接；真实 NOT_TESTED |
| turn/start | 固定 text input 形状；服务端覆盖 cwd、审批、候选只读策略 | 假 peer PASS；**真实禁止调用** |
| item/agentMessage/delta | 有界消费，映射 response 活动；不从 final 制造 delta | 假 peer PASS |
| item/completed / turn/completed | 合并最终消息，校验 Thread/Turn ID、成功状态和退出事实 | 假 peer PASS；真实 NOT_TESTED |
| turn/interrupt | 仅当前活跃 Thread/Turn；短 grace 后复用原进程组取消 | 假 peer ACK／interrupted／晚到 success 分别测试 |
| 原生 server request | command/fileChange decline，permissions 空 grant；其他 -32601 | 假 peer PASS；没有自动批准／凭据返回 |

[官方协议](https://learn.chatgpt.com/docs/app-server)使用逐行 JSON-RPC，stdio 默认可用；本实现不用 WebSocket。仅实现表中任务方法，未加入配置修改、账户操作、command/exec 透传或任意 RPC HTTP 路由。

## 协议和生命周期约束

- `internal/codexrpc`：Task 与 Probe 分开的请求白名单；严格 UTF-8、重复 key、JSON-RPC envelope、ID、未知 envelope 字段检查。本机观察到的可选 `emittedAtMs` 只作类型校验，不提供成功事实。单帧 1 MiB、总流 8 MiB；写入有时限，不无界等待。
- 接受交错通知；turn/start 回复前的相关事件有界暂存，得到权威 Turn ID 后才处理。未知通知不提供结果／会话／终态。异常退出、缺终态、错 ID、坏 JSON、超长输出、quota 与 auth 拒绝均有测试。
- 同一 Session 的原生 Thread 绑定仍在私有 Run Store；HTTP 不接受或返回原生 ID。跨客户端不能读 Session/Task，也不能传 Thread ID 接管。恢复使用原登记配置，不用变化后的 capability 快照当配置。
- SSE 继续发布 task.progress、text.completed 与唯一终止事件；本阶段 `native_text_delta=false`，没有新 wire DTO 或原始工具载荷。用量未提取时为 unknown/null，不估算。
- interrupted 只证明原生 app-server Turn 已结束；`upstream_cancel_confirmed` 保持 null，不能声称供应商云任务已停止。仅 ACK 不能提供原生终态；超时／撤销／取消优先于晚到 success。停止无法确认则沿用 needs_reconcile／隔离，不自动执行第二遍。
- 原 Model Gateway、CLI task/run/resume/jobs 和媒体实现不改。新 Adapter 仅 text_task，不提供图片或通用模型接口。

## 默认关闭的 Feature Flag

只在独立 NAG YAML 中增加可选字段，缺省 false：

```yaml
version: 1
host: 127.0.0.1
port: 0
queue_limit: 1
daily_dispatches: 1
daily_process_starts: 1
daily_execution_seconds: 120
codex_app_server_enabled: false
```

true 仅把 `codex-app-server-v1 / codex-app-server-readonly-v1` 候选加入目录；生产构造始终 Native=true 且 policy_unenforceable。清空 Blocker、伪造 receipt 或写配置均不能绕过 `AppServerBinding.Check(native)`。假配置不能从 YAML 构造 Native=false。exec 的受限 Profile 保留关闭；没有切换到它兜底。

`app-server disable` 写私有原子 tombstone；worker 25ms 监督并停止现有任务，随后创建／dispatch 失败。Feature Flag 修改需重启。当前没有真实 enable／authorize 命令，因为 [安全条件](G6_SECURITY_BOUNDARY.md)不成立，不能提供假启用开关。

## 本轮不是可运行的真实受限 Profile

本机 0.167.0 的 schema 缺少候选 restricted access 字段；Adapter 固定的严格候选策略不自动降级为全目录可读。当前无已验证的凭据隔离和认证云传输。Fake peer 对新策略形状的响应不能升级为本机支持证据。实现完成的是协议和离线纵向链路；首次真实调用仍须先完成 [G6_DECISION](G6_DECISION.md) 的最小缺口。
