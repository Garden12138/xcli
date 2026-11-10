# G8 验收与最终决策

2026-11-10，起点 `e8cf6b4`。阶段交付为离线研究与契约；生产状态 **BLOCKED_ELIGIBILITY_AND_ARCHITECTURE**，不是可用性 GO。

## 最终八项决策

| 项目 | 决策 |
| --- | --- |
| 1. xcli 当前是否符合资格？ | 作为用户描述的本地 MIT 开源项目，一般形态与官方 OSS 文档相符；**当前多应用 Gateway 的完整用途资格无法确认，BLOCKED**。没有把公开仓库或个人使用当批准。 |
| 2. 多个本地应用共用 xcli 是否符合要求？ | **未确认，BLOCKED**。同 client 多 host 的说明不能证明多独立产品共用授权。 |
| 3. OAuth 能否安全统一管理？ | 工程上可由专用 broker 和 OS 安全存储设计；统一注册/代管的授权归属仍待官方确认，真实 Keychain 与专用 broker 尚未实现，当前不能启用。 |
| 4. 可否不用 app-server 直接 Responses？ | **YES，官方明确支持**。本轮只模拟，不使用 app-server 或原生登录。 |
| 5. 文本与 Vision 是否可以实现？ | 协议可行，取决于账号/模型接受的输入；离线文本/图片合同通过不等于真实模型已验收。Vision 只做图片理解。 |
| 6. 图片生成缺口？ | 独立 Media Provider；GardenFlow 可继续走已有 Qwen-Image 2.1 API。没有调用、迁移或伪造 SIWC 图片生成。 |
| 7. 还需官方确认什么？ | 多应用 broker 资格、用户同意覆盖、统一/分应用 client、凭据代管与 host 定义，以及收费/托管/企业分发边界。详见 [资格审查](SIWC_ELIGIBILITY.md)。 |
| 8. 下一阶段可否真实 OAuth/推理？ | **当前不具备**。需先确认资格，补生产 OS 存储/签名 broker/OIDC/刷新与 HTTP transport 并离线验收，再取得单独真实调用授权。 |

官方依据：[Overview](https://developers.openai.com/siwc/token-sharing-open-source)、[Models and inference](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)、[Preview limitations](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)。这些来源支持协议和一般适用范围，没有对本项目作专门批准。

## 离线验收矩阵

| 范围 | 证据/边界 |
| --- | --- |
| 动态注册、PKCE、回调、重放与到期 | `TestOAuthPKCECallbackAndReplay`，验证精确 redirect/client 与重复参数拒绝。 |
| ID Token 签名/issuer/audience/nonce/sub、账号替换和 granted scope | `TestIDTokenAndAccountScopeBinding`，使用测试 RSA key；不是生产 JWKS 拉取验收。 |
| 刷新轮换、scope 降级、保留 ID、CAS、删除与脱敏 | `TestCredentialRotationRedactionAndFailClosed`；内存库仅测试，生产 OS 库仍不可用。 |
| Mock OAuth → 模型目录 → Responses → refresh → revoke | `TestOfflineOAuthResponsesLifecycle`；8 个 loopback 合成 HTTP 请求，真实请求为 0。 |
| 时限、取消、redirect、增量和网络边界 | `TestOfflineTimeoutCancellationAndRedirect` / `TestOfflineDeltaArrivesBeforeCompletion`，上游 stall 后取消，外部 Dial/redirect 拒绝，完成前交付真实合成增量。 |
| 文本/Vision 与参数/工具拒绝 | `TestStrictResponsesRequestContract`，真实 PNG 解码、MIME/URL 拒绝；无模型图像理解。 |
| SSE 终态、失败、断流、序号与错误脱敏 | `TestStreamCompletionAndSanitizedErrors`；不将部分输出算成功。 |
| 上下文 ownership/注册/TTL/容量与失败不提交 | `TestHistoryIsolationExpiryAndCapacity`；仅显式消息内存合同。 |
| 生产关闭与凭据隔离入口 | modelstore 拒绝 `siwc-responses`；modelgateway 无 OAuth 或 Responses 路由、OAuth Token 不能当本地客户端 Token。 |
| 保留既有功能 | 全量 Go race、vet、module、Python fixture、lifecycle 与跨平台构建，详细运行结果见下。 |

## 回归记录

完整命令：`bash tests/model_access_gateway/verify-offline.sh`。冻结源码后的最终执行 **exit 0 / PASS**，日志以 `G8_OFFLINE_REGRESSION_PASS` 结束。记录见 [完整日志](evidence/offline.log)、[机器可读汇总](evidence/validation.json)、[源码清单](evidence/source-manifest.json)、[官方阅读记录](evidence/official-review.json)和[初次失败说明](evidence/failure-notes.json)。

| 检查 | 最终结果 |
| --- | --- |
| G8 定向 race | 10 个顶层契约/安全测试及其子场景通过。 |
| 全量 Go | `go test -race -p 1 -count=1 -timeout 240s ./...` 全通过。 |
| 静态与模块 | `go vet ./...`、`go mod tidy -diff`、gofmt、`git diff --check` 通过。 |
| Python | R0 11、Native Gateway 9、R3 25、Model Gateway 9 项通过（共 54）。 |
| 现有 Gateway 进程/HTTP 回归 | control_smoke 与 built-binary lifecycle 通过，仅连接合成上游。 |
| 构建 | 本机及 Darwin amd64/arm64、Linux amd64/arm64、Windows amd64 全通过；交叉构建不等于异平台运行验收。 |

初次定向执行被沙箱 loopback bind 限制中断（EPERM），包括原有 Gateway 测试；获准在沙箱外运行后，SIWC/modelstore/modelgateway race 全通过。首轮完整回归期间收紧了回调/验证边界，源码编辑与编译并行导致构建输入变化，该轮不能作为最终证据；最终需冻结源码后重跑。

R3 回归使用其测试中注入 PATH 的假 Agent 和 `XCLI_R3_SIMULATION=1`。旧 harness 的输出可能含 “one real read-only invocation”等沿用文案；在本日志中它们是合成 fixture 子进程，不是启动真实 Cursor/Codex 或模型请求。G7 容器 opt-in 环境变量由本轮脚本显式清除。

Model Gateway workspace 测试中的 `repository: worklog` / `applied_files` 指测试创建的临时假仓库，不是本人的 Worklog 工程；没有修改四个真实业务应用。

## 未执行事项

真实 OAuth 登录、真实 Token 获取、真实推理、真实 Keychain 读写、G7 容器 opt-in 实验、四应用修改、部署、Push、Tag/Release 均为 **0**。本轮无新的订阅/API 推理费用验收。保留 G1–G7 证据与所有既有 Provider；Native 真实 Profile 继续默认关闭。
