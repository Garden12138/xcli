# G6 最终决策：ISOLATED_RUNTIME_REQUIRED

2026-11-09。最小 app-server stdio Adapter 已实现，Fake HTTP 全链路及离线回归通过，真实非推理 initialize 通过。**首个真实 HTTP 文本请求尚不具备工程条件**，不标 APP_SERVER_ENGINEERING_READY、READY_FOR_AUTHORIZATION 或真实 Gate B PASS。

## 作出 B 决策的独立原因

1. 安装版本 0.167.0 的生成 schema 不具备文档中的 restricted read access 结构，严格 command/exec 候选实际被拒；不可用全目录读取替代。
2. app-server 直接 fs 管理面可以读取工作区外假凭据和链接目标。它不是所有 builtin 越权的证明，却明确不能充当整个进程的隔离保证。真实 builtin 读取、环境及网络仍 NOT_TESTED；不能推断安全。
3. 已优先实现并实际尝试外层、无认证、断网 OS 原型；app-server 在 initialize 前 SIGABRT，未获可用阳性对照。没有开个人目录／Keychain／网络来绕过。
4. 应用认证、凭据保留在工具环境外的 broker、账号／用途及供应商适用性尚未完成。直接继承 CLI 私有凭据不能填补此缺口。

具体证据与最小环境工作量见 [安全边界](G6_SECURITY_BOUNDARY.md)。本轮停止这条真实启用路径；不继续叠加同类 Canary，也不自动升级 CLI、开系统用户、装 VM、发起 OAuth 或使用普通 API 代替 Native app-server。

## 当前可准确执行的命令

在仓库根目录构建，选择新的私有 NAG_STATE、独立 NAG_CONFIG：

```bash
GOCACHE=/tmp/xcli-nag-go-cache GOPROXY=off GOSUMDB=off \
  go build -trimpath -o /tmp/xcli-g6 .
/tmp/xcli-g6 agent-gateway --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  app-server readiness
python3 tests/native_agent_gateway/codex_first_http.py \
  --binary /tmp/xcli-g6 --adapter codex-app-server-v1 \
  --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  --out /private/tmp/g6-app-server-dry-run.json
```

默认 dry-run 不读客户端 Token、不启动服务或 CLI、不提交 Task；输出 B。`--out` 必须新文件。指定 app-server 时没有 exec／其他 CLI fallback。

需要立即关闭：`/tmp/xcli-g6 agent-gateway --state-dir "$NAG_STATE" app-server disable`。客户端撤销仍用 `client revoke <本人客户端ID>`，停服仍用 stop；原生 interrupted 不是云停止确认，不删除预算／journal重做。

## 将来首次真实文本入口（当前不可执行成功）

先完成独立环境和认证 broker、安装版本／策略验证、专用客户端／工作区及一次任务授权绑定，才可能提供新的本地 enable/authorize 实现；**当前没有这样的真实 app-server 激活命令**。必须由本人重新确认账号、计划、个人本地用途、无共享／商业托管场景、云发送与费用风险，不能复用 exec 的旧声明／授权记录；供应商模式适用性未确认则拒绝。

未来所有前置实现并通过后，已实现的验收脚本调用形态为：

```bash
python3 tests/native_agent_gateway/codex_first_http.py \
  --binary /tmp/xcli-g6 --adapter codex-app-server-v1 \
  --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  --token-file "$NAG_SECRET_DIR/codex.token" \
  --authorization-ref "$NAG_AUTHORIZATION_REF" --execute \
  --out /private/tmp/g6-codex-first-real-evidence.json
```

目前此命令在 readiness 阶段拒绝，0 Task POST；execute 和引用字符串不构成授权。未来门禁须返回工程 APP_SERVER_ENGINEERING_READY、总体 READY_FOR_AUTHORIZATION、本人确认与专用客户端就绪、独立 enable、有效的新调用引用摘要，脚本才会提交。

单次新授权单位：固定纯文本、只读合成项目、并发 1、1 Task／最多 1 顶层 app-server 进程、120s、失败不重试／不退款／不 fallback。原生内部请求与额度费用不可由启动次数精确上限约束。必要的真实 builtin 安全试验须另审、另批，不能挪用首次文本预算。

PASS 要求 HTTP 201/202/200、匹配固定标记、完整且归属正确的原生 Thread/Turn 终态、正常退出／停止已确认、final=true、事件递增且唯一完成、只扣 1 次、服务重启后结果／预算不变。首次单文本 PASS 也不能泛化 Session、真实取消、媒体或旧 Model Gateway Gate B。

## 交接

实现 SHA、最终源码摘要和回归证据见 IMPLEMENTATION_PROGRESS 的 G6 交接定位。四应用、Cursor/Agy、生产部署、Push／Tag／Release 都未执行。新真实推理请求始终为 0。
