# G7 最终决策：PARTIAL_HOST_ISOLATION_ONLY

> R4 / 2026-11-10：此页是冻结实验的历史记录，不是当前实施路线。xcli 已收敛为 CLI/Task 编排，不继续 Gateway、隔离探针或 OAuth；参见 [R4 模块处置](../product-realignment/MODULE_DISPOSITION.md)。历史证据和未完成状态保留，不作为核心可用性门禁。

2026-11-10。**真实独立执行环境已运行、宿主文件隔离通过；凭据和不可信工具未完成可靠分离。真实 Profile 继续关闭。** G7 到此结束，不自动进入 G8，也不继续给当前同域 app-server 架构叠加验证器。

## 决策证据

- 真实 Linux arm64 Codex 0.167.0 在现有 OrbStack 的断网、只读、UID 65532 容器中完成 initialize/initialized、指定合成材料读取及正常退出。宿主假凭据、绝对/相对链接、宿主映射候选、容器 socket 均不可读；写入/目录替换拒绝。见[运行时实验](G7_ISOLATED_RUNTIME_EVALUATION.md)。
- 一次有限反例：app-server 直接 fs 和同 UID 代表子进程能读模拟 Access Token 的环境载体。官方认证有文档，但完整凭据/全工具执行分离尚无可验证路径。路线 B 为 NOT_FEASIBLE_UNDER_CURRENT_CONSTRAINTS；见[凭据边界](G7_CREDENTIAL_BOUNDARY.md)。
- HTTP→NAG→容器内**假 peer** 的成功、销毁、持久事件、幂等和重启收敛已通过；生产没有启用隔离 transport，Native 拒绝保持有效。假结果不代表真实模型调用成功。
- G6 macOS SIGABRT 的底层根因仍未知：取栈受 OS debugger attach 权限阻断；Linux 可用不代表修复 Seatbelt。未放宽系统安全设置。

这符合 PARTIAL 的定义，而不是 GO 或模糊 READY。首个真实 HTTP→Codex 文本请求仍不具备工程条件；单次调用授权不能跨越该缺口。Codex/Cursor/Antigravity 的真实 Gate B 均未 PASS。

## 产品路线比较（仅评估）

| 路线 | 适用目标 | 剩余条件/限制 | 建议 |
| --- | --- | --- | --- |
| 1 保留 NAG，使用官方受控 Agent 集成 | 需要原生 Agent 任务、工具和事件 | 先确认全工具执行边界、应用授权；remote Code Mode 只按官方范围评估，当前证据不足以直接部署；不能复用 CLI 私有凭据作共享推理服务 | 保留协议和已有实现，暂停当前同域认证的补丁开发；需要新的具体受控能力证据再立项 |
| 2 独立 SIWC/Responses 适配器 | 符合条件的本地开源应用需要订阅授权文本能力 | 应用自己的 OAuth/plan usage 同意、资格/账号权益、私有凭据生命周期；不是 CLI Adapter，不继承 CLI 图片或未支持工具；付费/远程应用需另核实 | 若主要目标是四应用的订阅文本能力，优先作下一次产品选择；本轮不实施 |
| 3 保持个人 CLI Task/Run | 本人直接管理本机原生 Agent | 保留原生审批与个人使用边界，不转为无人值守共享服务；历史 CLI 验收不扩大为 NAG 真实 Gate | **现在推荐**：保留已验证的 xcli 用途，停止为首次 Gateway 调用继续堆叠安全补丁 |

官方 [SIWC 适用条件](https://developers.openai.com/siwc/token-sharing-open-source)区分开源/本地与付费/远程服务；[官方 app-server 认证说明](https://learn.chatgpt.com/docs/app-server#auth-endpoints)区分原生本地授权与商业/托管使用。[SIWC app-server](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server)是另一种显式应用授权，不能借既有 CLI 登录替代。以上比较不引入普通计费 API 来替代订阅目标。

本结论不是“再做若干天便可真实调用”的承诺；非 GO 状态不提供完成日期。若以后选择路线 1，最少先解决正式支持的认证/全部执行路径分离、受控模型出口、应用独立授权及资格、版本匹配、真实 Profile 生命周期与原生 Thread 持久化。必须另立有界范围，不自动进入实现。

## 当前准确操作

在仓库根目录执行已验证的离线门禁：

```bash
bash tests/native_agent_gateway/verify-offline.sh
```

外部系统 A5 的复验仅在本轮合成资产尚在时执行，**只运行假 peer**，并需本机 socket/loopback 权限：

```bash
GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off \
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath \
  -o /private/tmp/xcli-g7-runtime-frr74red/workspace/g7-fake-peer \
  ./tests/native_agent_gateway/g7_peer
GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off \
  XCLI_G7_RUN_CONTAINER_TEST=1 \
  XCLI_G7_WORKSPACE=/private/tmp/xcli-g7-runtime-frr74red/workspace \
  XCLI_G7_SOCKET=unix:///Users/cengjiada/.orbstack/run/docker.sock \
  XCLI_G7_IMAGE=sha256:49340b346798875b5592e5d420241f2f9cfba5f47e62ebb1446a1b34734e898f \
  go test -race ./internal/nativeadapter \
  -run '^TestG7ContainerHTTPFakePeerAndRuntimeLoss$' -count=1 -v
```

生产就绪/关闭仍使用既有控制命令；不新增容器 HTTP 控制 API。安全 dry-run：

```bash
GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off \
  go build -trimpath -o /tmp/xcli-g7 .
/tmp/xcli-g7 agent-gateway --state-dir "$NAG_STATE" \
  --gateway-config "$NAG_CONFIG" app-server readiness
python3 tests/native_agent_gateway/codex_first_http.py \
  --binary /tmp/xcli-g7 --adapter codex-app-server-v1 \
  --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  --out /private/tmp/g7-codex-first-dry-run.json
```

`NAG_STATE/NAG_CONFIG` 为本人选择的既有独立私有根/配置，`--out` 需不存在。默认 dry-run 不读客户端 Token、不发送 Task。**当前没有可成功进行首次真实调用的准确命令**；旧 `--execute` 形态仍在 readiness 阻断，不应通过手改 Profile、Native=false 或附加引用强行运行。未来须产品路线明确、工程门禁补齐、本人账号/用途/云发送/费用确认、Profile 单独启用及新的 1 Task/1 process/120s 授权。

未来真实 PASS 条件继续要求 HTTP/持久状态/原生终态/归属/固定文本标记一致、final=true、唯一完成事件、只扣一次预算、停止确认、重启不重执行；云停止与费用仍各自记录，不由本地退出推断。没有回退、自动重试、换账号或额外安全测试预算。

## 交接

三份 G7 文档、源码清单、完整离线门禁和 A0–A5 证据均在仓库。最终实现 SHA 见 [IMPLEMENTATION_PROGRESS](IMPLEMENTATION_PROGRESS.md) 的 G7 交接段。没有生产部署、Push、Tag、Release、外部应用改动、Cursor Trust 或 Antigravity 启用。真实模型请求仍为 **0**。
