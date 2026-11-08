# Codex 首次文本的隔离边界收敛（G5A.2）

2026-11-08；起点 `3494756`。结论：**ENGINEERING_BLOCKED**。本轮实际运行原生 feature resolver 和 sandbox helper，但没有运行 `codex exec` turn，没有发出推理 prompt，也没有读取、输出或复制真实认证材料。三家真实 Gate B 均未通过。历史 G5A.1 的登录状态不是本轮账号、权益或用途确认。

## 最小能力与官方开关

只允许已登记 Codex 0.167.0／二进制 hash、一个客户端、loopback、一个新建合成 Git 目录、固定内联文本、并发 1、累计顶层启动 1、120s；失败不重试、不 fallback、不续接、不自动升级权限。标记在 prompt 中直接提供，**首测不需要读文件工具**。prompt 的“不使用工具”只描述任务，不承担安全保证。

官方提供 `features.shell_tool=false`；`unified_exec` 的禁用另有 managed requirements 条件。文档中的常用 feature 表不是全部工具清单。[官方配置](https://learn.chatgpt.com/docs/config-file/config-basic)。本机 resolver 的基线和关闭六项后的差异见 [features.json](evidence/g5a2/features.json)：六项 false 后，browser/computer/code mode/remote plugin 等仍可为 true。因此候选固定参数额外关闭这些能力、view_image 和 plugins，保留 MCP 空表／web_search disabled；**不声称这些键已覆盖所有内置能力**。

固定候选参数还加入 `shell_environment_policy.inherit="none"`、`ignore_default_excludes=false`。官方文档说明 shell 子进程环境有独立过滤规则；默认值不能当作自动过滤所有秘密的保证。[官方环境策略](https://learn.chatgpt.com/docs/config-file/config-advanced)。本轮 helper 不是 exec 的实际工具启动器，未把配置静态检查晋升为运行时证明。

权限 profile 约束本地沙箱命令；浏览器、Computer Use、MCP、连接器和模型／认证服务流量另有边界。[官方权限说明](https://learn.chatgpt.com/docs/permissions)。故禁止把命令 curl 被拒绝推断为所有 builtin 都不能联网，或把父进程连通 mock 推断为真实授权模型端点可用。

## 身份、环境和认证

- Gateway、Codex 父进程及本轮 sandbox 命令均为同一个 OS UID；UID 仅比较、不在证据公开数值。同 UID 未沙箱子进程可读 0600 假 auth.json，证实仅改 HOME／隐藏路径不是权限隔离。
- 现有 exec 路线由官方 CLI 父进程在登记 CODEX_HOME 使用既有登录；xcli 只保存目录引用，不解析或复制文件。Gateway→CLI 环境只允许 HOME、固定系统 PATH、LANG 和专用 CODEX_HOME，无代理、loader、API Key 或 Gateway Token 继承。
- CLI 父进程必须能使用其认证，不等于每个工具都获准读取认证。**尚未证明所有内置直接 IO 和工具环境都不能接触真实认证目录**；没有真实凭据测试，不能回答“全部不可读”。本轮使用完全合成 HOME/CODEX_HOME、假凭据、无害文件及 mock endpoint。
- 没有新增用户、挂载、认证副本或真实登录，也未修改原生配置、Trust、账号或 MCP。环境 allowlist 与原生代码隔离是不同的证据。

## 实际证据分层

[初次 probe](evidence/g5a2/boundary-probe.json)保留原历史登记摘要；最终 probe 使用新登记与目录身份检查，见 [final-boundary-probe.json](evidence/g5a2/final-boundary-probe.json)。这两个均为真实安装二进制的**无推理**命令，不是 Fake CLI 推理证据。

| 边界 | 机制与实际范围 | 结果 |
| --- | --- | --- |
| 同 UID 非沙箱读取假凭据 | 实际 /bin/cat、程序构造 0600 文件，阳性对照 | 可读，证明风险基线 |
| 登记 material.txt 可读 | 原生 sandbox + OS 控制，严格比对固定字节 | PASS |
| 假 CODEX_HOME 绝对／相对路径 | 原生 sandbox 启动 cat，实际 OS 拒绝 | PASS，仅该命令 |
| 未登记文件／指向假凭据的链接 | 原生 sandbox + Canary | PASS，仅该命令 |
| 工作区写入 | touch 被拒，文件不存在；材料和目录身份复核 | PASS，仅该命令 |
| 命令网络 | 父 curl 命中 mock；sandbox curl 失败且命中数不增加 | PASS，仅 loopback mock 对照 |
| 10 项 feature=false | 实际官方 resolver 输出 false | PASS，非最终工具目录 |
| Gateway→CLI 环境 | 传入假 KEY/proxy/loader/app 变量，allowlist 排除 | PASS，Go 层验证 |
| 字节相同的目录替换 | dev/inode 与 no-follow 内容摘要联合校验 | PASS，离线攻击回归；不是持续 OS 锁 |
| HTTP prompt/argv/env/path、所有权、撤销、预算 | 既有与增量 Fake CLI／契约／race 测试 | PASS，仅离线范围 |
| exec 实际暴露的完整工具目录 | 未运行真实 turn，无版本绑定的有效目录证明 | NOT_PROVEN |
| builtin 直接 IO／工具读取真实凭据 | 没有覆盖所有执行通道的独立 broker 或 OS 身份边界 | NOT_PROVEN；未测试真实材料 |
| exec 工具实际环境过滤 | 固定配置；没有 exec 工具运行证据 | NOT_PROVEN |
| 任意 builtin 网络、真实模型／认证端点 | 只测 mock；未向供应商发送请求 | NOT_TESTED |
| check→spawn 间目录替换竞争 | 启动前／监督校验不是 fd 固定的 OS 工作区边界 | 未证明；不得泛化目录攻击测试 |

真实 exec 才可观测的工具行为不能借本轮授权运行。即使未来进行一次文本 turn，也只能证明该次所观测行为；缺少工具调用不证明其他工具不存在。

## Fail closed 与兼容性

`limited_boundary_probe.go` 生成范围明确的证据，含二进制／登记／策略摘要、时间、真实凭据使用 false、模型请求 0。`limited_readiness.go` 验证新鲜性，同时保留三个不可由 JSON 声明补足的工程缺口：

1. `exec_effective_tool_catalog_not_attested`
2. `direct_builtin_io_has_no_independent_credential_boundary`
3. `exec_tool_environment_filter_not_runtime_verified`

伪造 receipt 把所有项写 PASS、填 YAML 用途引用、导入用户声明或编辑启用布尔值，均不能改变这些机制缺失。原生 enable、authorize、Session/Task、dispatch、spawn 都继续拒绝。缺少当前登录检查／客户端／工作区等另列，不用一个 BLOCKED 掩盖原因。

新增 workspace dev/inode 登记：G5A.1 仅有字节摘要的旧登记继续可读，但执行检查拒绝；不自动给旧目录补身份，不覆盖历史证据。未来需在独立私有根重新 prepare，不把重新登记当作重置一次调用预算的手段。没有改变旧 Model Gateway 或 task/run/resume/jobs 的语义。

## 增量路线与估算（未实施）

当前 exec 路线不能满足首次真实任务的完整安全条件；**需要补一个可独立验证的执行边界**。不等于 app-server 天然安全，也不要求未经评估就迁移。

| 路线 | 必须补的工作／验收 | 估算 |
| --- | --- | --- |
| 保留 exec，独立用户或 VM／受控环境 | 专用非个人 HOME；由用户通过官方流程自行登录；无个人目录共享；父进程模型流量与工具 egress 分离；不可读认证的独立工具身份／broker；用全部能力目录 + Canary 验收。仅新 UID 自己同时跑父与工具仍不足 | 3–6 人日，可行性探针后重估；用户／环境创建与登录另需授权 |
| 官方 app-server Adapter | 增量接入 typed request/response、受控工具／审批 deny、thread/turn 所有权映射、interrupt/重启、有效能力目录和直接 IO 审查；复用现有持久预算、worker、HTTP。仍需认证与工具边界 | 5–9 人日；不是本轮切换开关 |
| app-server + SIWC | 应用正式资格／注册、用户 OAuth + plan usage 同意、独立 token 生命周期与防泄漏；不得读取 CLI 私有凭据。模型目录不是权益证明 | 上项外约 3–5 人日，不含供应商审核等待 |

[官方 SIWC app-server 文档](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server)描述应用自己的 OAuth 授权和对话协议，不是导出既有 CLI 登录的依据。这些估算是工程评估，不能证明账号或商业／共享用途已获许可。没有在本轮创建系统用户、VM、OAuth、Adapter 或调用模型。

## 门禁结论

`engineering_state=ENGINEERING_BLOCKED`，`user_confirmation_state=USER_CONFIRMATION_REQUIRED`，总体 `state=ENGINEERING_BLOCKED`。工程方案和本人账号／用途声明均未齐备；脚本有 `--execute` 也不能执行。下一任务先选定并独立验收边界，再按 [本人确认](CODEX_ACCOUNT_AND_USAGE_CONFIRMATION.md)和[首次文本入口](CODEX_FIRST_REAL_HTTP_TEST.md)处理，不能以一次推理授权覆盖工程缺口。
