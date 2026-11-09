# G6：离线与非推理验收

2026-11-09。**Fake app-server 纵向链路通过；真实 stdio 握手通过；真实执行隔离未通过。** 未发送真实 turn/start，真实 Gate B 非 PASS。

## 离线门禁

恢复和复验命令：

```bash
bash tests/native_agent_gateway/verify-offline.sh
```

最终 [offline.log](evidence/g6/offline.log)：全量 `go test -race -p 1 -count=1 -timeout 240s ./...`、`go vet ./...`、build、两次本地服务 lifecycle、旧 Python 11 项、新 Python 6 项、diff 检查。旧 Runtime／Model Gateway／Run Store／CLI 测试全部保留，不跑真实 native smoke。[targeted.log](evidence/g6/targeted.log)记录 app-server 专项。

| 测试范围 | 内容 | 证据性质 |
| --- | --- | --- |
| HTTP → 假 app-server | Session 201、Task 202、20 并发同键、唯一 dispatch、最终结果、SSE、存储重开与续接 | 真 HTTP／真假进程，非模型 |
| 事件顺序与关联 | 回复前通知、native Thread/Turn 匹配、错 ID／缺终态／坏 JSON／超长流 | 构造协议，不证明真实模型行为 |
| 审批 | command/file decline、空 permissions grant、其他请求拒绝、认证刷新拒绝 | 假 peer；无自动 approve |
| 生命周期 | cancel、timeout、client revoke、profile disable、进程崩溃、晚到 success、interrupt ACK | 假 peer；云停止仍未知 |
| 身份与请求 | Session 跨 client 拒绝、native ID 不公开；native_args/env/cwd/sandboxPolicy/Thread 参数及 RPC/config/account 路由拒绝 | HTTP 合同与服务层 |
| Feature Flag | 默认关闭；Native=true 即使清空 Blocker 也在 spawn 前拒绝 | 0 进程负向测试 |
| 资源 | 有界帧、流、写入；原 worker 并发／持久预算／幂等／恢复机制 | 离线 race，不是性能或费用保证 |
| 外层环境构造 | 私有 sibling 根、链接／公开目录／根目录拒绝，无网络／Keychain grant | 静态／单元测试，不是实际隔离 PASS |

新增测试没有通过删例、放宽权限或增加生产超时过关。初次测试方法名错误见 targeted-initial.log；夹具错误地用 rotate 试图改变旧 scope，随后改成独立新 grant。HTTP admission 429 按既有合同计零 dispatch；重启按原登记配置重建，不把运行时 capability 快照当配置。失败日志 retained：[targeted-fixture-failures.log](evidence/g6/targeted-fixture-failures.log)。外层单元夹具初次不是私有目录，修正测试目录为 0700，生产条件未放宽。

## 真实 CLI 非推理证据

- 本机版本/schema：2 次启动；[schema 索引](evidence/g6/schema-capabilities.json)。
- 正常 stdio 初始化、三个合成 fs/readFile、两个严格 command/exec 候选：[最终 receipt](evidence/g6/local-app-server-final.json)。进程 exit 0、停止已确认；命令候选均被拒，**不是命令执行隔离通过**。
- 第一次 probe 遇到正式通知新增 emittedAtMs 字段，保留 [初次结果](evidence/g6/local-app-server.json)；只补类型校验，没有忽略任意 envelope 字段。
- 全进程 OS 原型初始化失败及单次诊断保留，没有把失败算 PASS。
- [非模型账本](evidence/g6/non-model-ledger.json)：7 次顶层 metadata／probe／launcher 尝试；不等于全部 OS 子进程，不是模型调用或费用。没有真实 Thread／Turn／account/login 请求。

完整结论见 [G6_SECURITY_BOUNDARY](G6_SECURITY_BOUNDARY.md)。所有 true 的模型事件、Session 续接和批准拒绝成功证据都来自明确命名的 Fake peer；不能拼接“假 HTTP PASS＋真实握手 PASS”为真实 HTTP 推理 PASS。

## 真实 Gateway 的阻断证据

[http-native-blocked.json](evidence/g6/http-native-blocked.json)：使用本轮真实安装版本／二进制 hash 的元数据缓存，Flag=true、专用 Token、1/1/120 预算；app-server Session 仍 503 policy_unenforceable，原生启动 0。撤销 Token 后 GET 401；profile disable 标记保存，最终 Flag=false、服务停止。缓存只含已实际测量的安装字段，没有 native 认证信息，也没有额外 --version 启动。

指定 `--adapter codex-app-server-v1` 的[默认 dry-run](evidence/g6/script-dry-run.json)和[显式 execute 前置阻断](evidence/g6/script-execute-blocked.json)都记录 0 Task POST。源码清单及最终门禁索引见 [validation.json](evidence/g6/validation.json)；真实 HTTP 推理仍 NOT_TESTED。
