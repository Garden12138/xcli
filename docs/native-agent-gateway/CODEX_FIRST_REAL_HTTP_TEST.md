# Codex 首次真实 HTTP 文本验收入口

2026-11-08。脚本：[codex_first_http.py](../../tests/native_agent_gateway/codex_first_http.py)。**已实现并离线测试；本轮未执行真实模型。当前 Profile ENGINEERING_BLOCKED，脚本不能执行成功。** 先读 [CODEX_LIMITED_PROFILE](CODEX_LIMITED_PROFILE.md)，不能把一次用户调用授权替代尚缺的原生隔离工程。

默认 dry-run，读取停服控制面的就绪报告，不读 Token、不启动服务、不执行原生探针、不提交 Task：

```bash
python3 tests/native_agent_gateway/codex_first_http.py \
  --binary /tmp/xcli-g5a2 \
  --state-dir /private/tmp/xcli-g5a2-pgnzz8as/state \
  --gateway-config "$PWD/examples/codex-limited-gateway.yaml" \
  --out /private/tmp/codex-first-dry-run.json
```

`--out` 必须是新文件；已有文件拒绝覆盖。G5A.2 实际 dry-run [证据](evidence/g5a2/script-dry-run.json)显示 ENGINEERING_BLOCKED／0 Task POST，显式 execute 的[负向证据](evidence/g5a2/script-execute-blocked.json)也为零 Task。该临时根只登记空假 auth-home，不用于真实调用。输出 DRY_RUN 不是 READY。不要使用 Python `-O` 或 PYTHONOPTIMIZE，脚本会拒绝执行，防止跳过验收断言。

## 真实执行前的独立条件

1. 按 [CODEX_ISOLATION_BOUNDARY](CODEX_ISOLATION_BOUNDARY.md)先解决三个独立机制缺口，engineering_state 必须为 ENGINEERING_READY；当前 ENGINEERING_BLOCKED。工程就绪但缺本人声明时总体为 USER_CONFIRMATION_REQUIRED，两者齐备才可为 READY_FOR_AUTHORIZATION。当前源码拒绝不能由文件声明绕过。
2. 本人按 [CODEX_ACCOUNT_AND_USAGE_CONFIRMATION](CODEX_ACCOUNT_AND_USAGE_CONFIRMATION.md)完成六项声明，以 private JSON 显式导入；readiness 必须给出 user_confirmation_state=CONFIRMED 及当前摘要。仅 login status 或旧 YAML 字符串不能证明所有权、用途或费用同意。这不是 SIWC 应用授权或共享推理服务许可。
3. 登记 workspace、binary/version/固定策略摘要不变；原生无模型 probe 证据新鲜。Token 必须来自专用客户端，scope 和预算符合受限配置；非法 Token 由 Gateway 拒绝。
4. 本人显式启用 Profile，然后**另行**批准本轮授权引用：只一次原生顶层进程、最大并发 1、Task 120s；同意合成材料发送到云端及可能的订阅／credits 消耗。启动次数不等于内部请求、Token 或费用精确上限。硬金额无法强制时拒绝执行。

未来工程门禁、本人六项声明、有效登录／客户端／新鲜证据全部满足，且获得本轮一次明确调用授权后，准确调用形态如下（**当前仍会拒绝，一次调用授权不能填补隔离缺口**）。先按 USER_GUIDE 构建 /tmp/xcli-nag，变量须指向经独立验收的新根和配置；不要使用本轮空假 auth-home：

```bash
NAG_AUTHORIZATION_REF='G5B-CODEX-ONE-REPLACE_WITH_NEW_APPROVAL_REFERENCE'
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  codex-limited enable
/tmp/xcli-nag agent-gateway --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  codex-limited authorize --authorization-ref "$NAG_AUTHORIZATION_REF"
python3 tests/native_agent_gateway/codex_first_http.py \
  --binary /tmp/xcli-nag --state-dir "$NAG_STATE" --gateway-config "$NAG_CONFIG" \
  --token-file "$NAG_SECRET_DIR/codex.token" \
  --authorization-ref "$NAG_AUTHORIZATION_REF" --execute \
  --out /private/tmp/codex-first-real-evidence.json
```

占位引用须先替换为本人本轮实际批准引用。`NAG_STATE`、`NAG_CONFIG` 和私有 Token 目录按受限 Profile 手册设置；NAG_SECRET_DIR 是本人登记的私有 Token 目录。G5A.2 负向测试的交付目录为 `/private/tmp/xcli-g5a2-pgnzz8as/client-secrets`，仅含独立 Gateway 验收 Token，不是 Codex 登录凭据，也不赋予真实调用许可。脚本要求 Gateway 先停止，自己启动/停止 foreground 服务并验证新 generation 后才使用端口，不安装后台项。授权引用摘要须与控制面一致；ENGINEERING_READY、CONFIRMED 摘要、配置 enabled、运行开关、调用许可、客户端就绪缺一均拒绝，execute 不是补授权的开关。

## 执行与 PASS

脚本先占用新的证据文件并持久化唯一幂等键，创建 Session（零原生启动），仅 POST 一次固定 text Task。不读取真实业务材料；固定合成标记同时位于只读登记材料和内联指令，首测不宣称模型已通过工具读了文件。202 仅为接受，之后 GET 原 Task／SSE，无自动重试、fallback 或第二个 Task。

PASS 要求：HTTP 201/202/200、正确固定标记、实际 native Session ID 观察、完整 terminal+exit 0、protocol complete、final=true、1 dispatch/1 process_start、持久事件递增且唯一 task.completed。停止并重新启动 Gateway，原 Task/结果/已用次数必须保留，不能自动重新执行。只保存摘要、事件种类／seq、状态、预算和本地／云停止字段，不保存 Token、账号资料或原始工具载荷。

`PASS_SINGLE_TEXT_ONLY` 只代表首次文本场景；Session 续接、真实取消、原生 delta、媒体和完整 L01–L05 仍分别验收，原 Gate B 不变。

## 失败停止和恢复

任何前置失败、权限／Trust／额度／支付拒绝、网络歧义、内容不符或缺终态均停止，不重发 Task。等待越过 130s 时仅请求取消原 Task，然后停止自己启动的服务；不保证云任务已停止。未知本地／云副作用保持 needs_reconcile，不按历史 PID kill，不删除 journal/预算。

EOF/超时后保留证据中的幂等键和 task_id（若已取得），本人查询原对象；**不得换新输出文件／幂等键重做来绕过一次性预算**。必要时先 `codex-limited disable`、`client revoke codex-acceptance`、`stop`。服务停止失败会记录 UNKNOWN_OWNER_RECONCILE_REQUIRED；不自动扩大沙箱或批准工具。
