# Native Agent Gateway V1 — Tasks & Milestones

日期：2026-11-06。只有 G0 为本轮设计交付；其余全部 Planned，未执行实现、真实模型或应用测试。每个阶段建议独立本地提交；不自动 Push/部署。验收编号见 [ACCEPTANCE.md](ACCEPTANCE.md)。

## G0 — 固定证据与设计（本轮）

- [x] 读取五份指定文档及 Agent/Runtime/Runstore/Model Gateway 关键源码，核对官方集成路线。
- [x] 核验旧工作区验收证据与 Smoke 修复：11/11 离线回归、源码/证据哈希、9 次账本、四张有效图片。
- [x] 创建独立提交 `864a6b1`，33 文件；不 Push、不改历史验收结论。
- [x] 输出七份 SPEC、接口/应用协议、安全和里程碑评估。

完成标准：D01–D04 通过设计交付检查；所有未来 endpoint/命令明确 Proposed，真实 HTTP→CLI 和四应用仍未验收。设计交付检查不把任何 NAG 实现 Gate 提升为 PASS。

## G1 — 合同、策略、事务骨架与官方协议评估（4–6 人日）

依赖 G0，默认服务 dispatch 预算 0。此阶段先证明安全和 crash 不重放，再接真实 native worker。

- [ ] 固定 JSON DTO/闭合字段、状态/error/SSE 合同，增加 fake-only contract suite。
- [ ] 独立配置/数据根/loopback server/Host-Origin 检查、客户端 issue/rotate/revoke 与权限 scope；复用私有存储工具，不共用 Model Token。
- [ ] 实现单写者事务 journal、checkpoint、幂等/预算原子扣减、归属与 Session CAS。
- [ ] 建 profile/二进制摘要/适配 revision/能力证据目录；GET 仅缓存，显式非模型 refresh 与执行分开。
- [ ] 逐 CLI 建立可执行的最小策略清单；无法约束全局配置的标为 policy_unenforceable，先只允许合成数据。
- [ ] 以官方 app-server 文档/schema 静态夹具 timebox 1–2 人日评估 Go stdio RPC：初始化、请求关联、delta、拒绝审批、interrupt、未知方法；不登录、不 turn/start 真模型。

明确验收：N01–N06、N09、N14 的 fake/存储部分通过；100 个并发同幂等键提交只创建 1 个 Task；任意 crash 点不产生第 2 个 dispatch。旧 Model Gateway native-cli 拒绝仍通过。测试只用受控临时目录/假 worker/假上游，不启动已安装的 native 程序。

退出条件：可演示 HTTP→fake Driver 的完整状态；未知策略/云拒绝/无 Token/零日预算均为 0 native 启动。若某 CLI 权限不可实现，记录关闭该 profile 的决策，不通过降低权限要求解决。

## G2 — 单 turn 文本、Session、活动流和本地停止（7–11 人日）

依赖 G1。实现 `codex-exec-v1`、`antigravity-print-v1` 和 Cursor 的解析/阻断路径，不在本阶段自动执行真实调用。

- [ ] 给现有 agent decoder 增加向后兼容 typed observer：原生会话来源、terminal/denial/usage/activity；不把 fallback ID 当 observed。
- [ ] 受控固定 argv + 最小环境 + task 工作区；进程组监督、总 deadline、无界 wait/后代 pipe 收尾问题修复。
- [ ] Session 首次绑定与续接；strict owner/profile/capability checks；无任意原生 Session import。
- [ ] Task 异步调度、有限队列、安装域/Session 锁、累积预算、取消/超时/撤销。
- [ ] 本地 journal 与 SSE 观察者解耦，终态先持久化；缺 native delta 时仅活动+最终引用。
- [ ] 版本化错误分类，trust/auth/quota/denied_actions 不再被纯 protocol_error 掩盖；保留旧 CLI 状态兼容。
- [ ] token/usage 字段缺失语义保真，新 API 不推断精确费用或 native model。

明确验收：N01–N10、N14、N15 文本部分全通过；拆分/超长/未知/截断事件、Agy soft-deny、Cursor trust、Codex reconnect/quota、Session 缺 ID/错 ID 均有固定夹具。fake 子进程主动拒绝 TERM/持有 pipe/派生后代，收敛在 deadline+5 秒清理硬上限内（含 2 秒 TERM grace）；无法确认时正确进入 needs_reconcile 并隔离，不声称 worker 已停止。

退出条件：文本 HTTP→fake worker、续接、并发/取消/断连/重启可靠；真实三 CLI 的取消和 HTTP 链路仍为 NOT_TESTED。若剩余 worker 无法确认停止，正确进入 needs_reconcile 也必须阻止后续执行。

## G3 — 输入与真实 Artifact 交付、来源/视觉复核（6–9 人日）

依赖 G2，新增媒体 profile 单独注册，不把文本默认沙箱升级为写权限。

- [ ] input 字节上传、不可变哈希与 Session 归属；只接受文本/单参考图片。
- [ ] 提取现有纯图片校验器，安全根/目录 FD/no-follow/有界复制/atomic publish；拒绝全局原生目录泛读。
- [ ] ArtifactCandidate、真实 MIME/尺寸/完整解码/哈希、原图不变、来源观察与人工复核。
- [ ] authenticated preview/review_only → provenance+semantics → ready；review 不调用模型，不覆盖权限或账单阻断。
- [ ] 下载、过期、取消后晚到文件隔离、存储额度/磁盘不足/清理。
- [ ] GardenFlow 风格 fake 客户端按两阶段接收，但只放在 xcli 测试目录，不改应用。

明确验收：N07、N11、N12 图片部分；恶意路径/目录分量 symlink/硬链接/替换竞态/超大解码/伪 MIME/输入覆盖/URL/仅文案完成均失败关闭。嵌套工具不可见进入复核，不能自动 PASS；无码来源证明不能 ready。过期或取消后文件不自动入库。

退出条件：可在假 Agent 生成的测试图片上完成 API 字节/摘要闭环；重用历史真实图片只能测解码和传输，不能记作新的真实模型成功。

## G4 — 故障收敛与获批的真实 HTTP→CLI 验收（4–7 人日，等待授权另计）

依赖 G1–G3 全部离线检查。先运行 N13–N15 故障/兼容回归；只有用户另立的 CLI/场景/进程预算与数据/权限范围获批后，才执行 L01–L05。

- [ ] go test/race/vet/build、旧 CLI/HTTP Gateway 确定性回归；不引入 native-cli backend。
- [ ] journal crash 点、预算 race、对象越权、重启、Token 撤销、断网/SSE 重连、取消与磁盘错误。
- [ ] 逐 profile 检查实际可执行权限边界及全局工具继承；无足够约束只保留合成 profile。
- [ ] 新授权后：先 Codex HTTP 新文本+同 Session，再 Antigravity；Cursor 先由本人审查 trust，不加 --trust/force 或变更全局权限。
- [ ] 文本通过后，另按明确预算验证媒体生成/编辑与官方工具来源、真实字节/视觉，不自动重复失败。
- [ ] 主动取消、超时、并发分别安排独立场景和调用槽；不借已通过文本/图片证据补齐。

明确验收：每个启用 profile 的 NAG Gate 都有 CLI 版本/二进制/策略/适配 revision/调用账本/错误与产物证据。至少一条官方 native 路径完成 HTTP→CLI 的文本、续接、事件、取消/超时及媒体闭环，其他 CLI 逐项保留 BLOCKED/NOT_TESTED。宣称“三 CLI 全可用”必须三者各自全部过门禁。

退出条件：发布的是逐能力可用目录，非全局“一切通过”；如果三 CLI 全支持为业务硬要求，Cursor 尚阻断则 V1 整体验收保持部分完成。**原 Gate B 仍 BLOCKED**，Agent 独立门禁不能替代 Model API 验收。此阶段也不包含四应用 UI、部署或正式发布。

## G5 — 官方 app-server 晋升（可选，另估 5–9 人日）

G1 调研先做，晋升不阻塞 exec V1。只有本地官方 schema、权限、会话、取消、媒体、用量和错误 parity 都通过，才能注册新 adapter_id 并由本人选择新 Session。

- [ ] Go stdio JSON-RPC pump、初始化/请求超时、通知解析、未知方法拒绝；不用 SDK sidecar 也可遵循公开协议。
- [ ] 所有工具审批 deny-only；按任务独占 worker，不透传控制面或开放新的权限。
- [ ] turn 与 process 两种计数分别扣预算；interrupt 等真实 terminal，超时再关闭 worker。
- [ ] 原生 message delta 去重/最终结果一致，不能继承 exec 的虚构 delta 能力。
- [ ] 以 N01–N15 和新的 L 场景单独验收，不将 exec Session 无声迁移。

完成标准：显式选用 app-server 的用户可观察到与 exec 同等安全/结果合同，且新增 delta/审批观察能力分别有证据。SDK 若以后采用，须额外管理 sidecar/runtime 包版本、认证路径及进程监督，不默认启用。

## G6 — 四应用采用与宿主桥接（独立后续任务，本轮不执行）

| 推荐顺序 | 未来工作 | 粗估人日（不含真实数据/部署审批等待） | 单独完成门槛 |
| --- | --- | --- | --- |
| 1 Worklog | 文本草稿/本地 Schema/预览接收，独立 Agent transport/Keyring | 3–5 | A01 合成流程，再另批 UI；零重复模型请求 |
| 2 GardenFlow | task→preview→review→认证下载→媒体入库，编辑原图保护 | 5–8 | A02、媒体入库事务与重启持久引用 |
| 3 Chat Insight | 选段对话/续问、隐私同意；Embedding 独立禁用 | 4–6 | A03；不以文本成功宣称完整 RAG |
| 4 Contribution OS | 只读 Plan/Review、冻结 Profile、业务校验，独立 provider | 5–8 | A04；受控 Run 的仓库写入另立范围 |
| 另立项目 | Docker→宿主强认证/实例固定/短期授权/安全 Artifact 通道 | 5–10+ | BHOST 独立安全/容器/生产环境验收，不能沿用旧 HTTP 补丁直接启用 |

每次从最新应用 HEAD/diff 重新评估，保护用户修改，不回放历史整包补丁。文本、图片、向量和发布分别授权；代码改造、合成 native 联通、真实模型、UI 和生产发布分别留证。

## 持续开发规则

每阶段收尾更新本文件与独立进度记录：实现范围、代码提交、离线检查、真实启动数、未解决 blocker、下阶段入口。失败保留原始脱敏证据，不能以重跑/人工编辑状态覆盖；新执行需要明确原因和剩余/新增预算。核心 G1–G4 21–33 人日，建议预留 25–40 人日；四应用/桥接/app-server 不混入核心估算。
