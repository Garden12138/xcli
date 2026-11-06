# CLI-first 真实验收报告

日期：2026-11-06。**整体 BLOCKED；Codex、Antigravity 的本轮原生 Agent 能力部分 PASS；Cursor 被 workspace trust 阻断。Gate B 始终 BLOCKED，未新增 CLI → HTTP backend。**

## 基线和预算

- 本轮 HEAD：`7e57c6774b4bcfd4735f0d4c35c7c7a3a58d2ac4`，开始时工作区干净。新构建 `0.5.0-r3-dev`；二进制 SHA-256：`0c431b9b6dd6ecd764fa5fb7f157dd58f9afa26605456c8e23f29b669ba1de35`。
- macOS 26.5.1 / arm64，Go 1.26.4，Python 3 与 Pillow 12.2.0。
- 用户明确批准文本最多 6、媒体额外最多 6 次原生 CLI 启动，失败不重试、不扩大原生权限、不改长期 trust、不 Push。
- **实际 9 次：文本 5、媒体 4；8 个场景 PASS，1 个已启动场景 BLOCKED。剩余文本 1、媒体 2，共 3 次未使用，不转给其他场景。**
- 本轮没有额外启动 native `--version`/登录状态探测；版本沿用当日先前的[只读预检查](evidence/preflight.json)，另记录[当前可执行文件摘要](evidence/native-20261106/bounded-preflight.json)。Codex 本轮会话元数据再次确认 0.167.0；Cursor 2026.10.22-27dcf5f、Antigravity 1.5.2 的版本来源如实保留为先前探测。

每个进程可能有多轮推理、工具或子代理请求。报告保留原生上报的 usage，但它可能含缓存或累计统计，**不能当精确请求次数或账单**。没有观察到显式额外支付提示，没有购买/启用付费额度；不能据此保证实际 credits/费用为零。常见 API Key/ADC 环境变量在测试子进程中屏蔽，保留已有原生账号根；未读取或复制登录凭据、登录/注销/切换账号。

[最终账本](evidence/native-budget.json)、[文本原始脱敏结果](evidence/native-20261106/text.json)、[媒体复核结果](evidence/native-20261106/media.json)、[阶段转换记录](evidence/native-20261106/phase-transition.json)。

## Gates

| Gate | 状态 | 证据范围 |
| --- | --- | --- |
| 第一阶段离线门禁 | PASS（前一阶段） | test/race/vet/build、doctor、假上游生命周期；不是真实 CLI 模型 API |
| A：Codex 文本/同 Session/生成/编辑 | PASS | 本轮 4 次，独立标记、真实图片及参考编辑 |
| A：Antigravity 同上 | PASS | 本轮 4 次；子代理内部请求数不完整可观测 |
| A：Cursor | BLOCKED | 唯一一次启动被工作区 trust 拒绝；没有改 trust 或重试 |
| B：原生 CLI → `/v1/*` | **BLOCKED** | 尚无 Native CLI Backend；没有实现或运行此链路 |
| C：四应用消费原生能力 | BLOCKED / Planned | 仅更新设计，未改业务代码或运行 UI |
| Schema、独立 Vision、并发、真实取消/超时、异机/重启 | NOT_TESTED | 未用剩余额度填补这些未授权场景 |

PASS 是本次具体场景通过；FAIL 是实际未满足验收；BLOCKED 是权限/网络/额度/实现等前置受阻；NOT_SUPPORTED 只用于明确不支持的能力；NOT_TESTED 是未执行。本轮未观察到可断言 NOT_SUPPORTED 的真实场景，不能把 Cursor trust 误写成“不支持文本/图片”。

## 逐次结果

退出码按“xcli/原生”列示；媒体原生退出还由透明观察器记录。顺序先完整结束文本组，再开始媒体；媒体生成两次后离线复核，再各编辑一次，没有重复任何生成或失败文本调用。

| CLI | 调用类型 | 最终状态 | 秒 | 退出码 | 判定 |
| --- | --- | --- | --- | --- | --- |
| codex | headless | PASS | 10.600 | 0/0 | 独立标记正确 |
| codex | resume | PASS | 5.747 | 0/0 | 独立标记正确；同 Session |
| antigravity | headless | PASS | 57.725 | 0/0 | 独立标记正确 |
| antigravity | resume | PASS | 66.153 | 0/0 | 独立标记正确；同 Session |
| cursor | headless | BLOCKED | 0.623 | 1/1 | workspace_trust |
| codex | image generate | PASS | 44.176 | 0/0 | PNG 1254×1254；文件/视觉核验 |
| antigravity | image generate | PASS | 58.094 | 0/0 | JPEG 1024×1024；文件/视觉核验 |
| codex | image edit | PASS | 49.039 | 0/0 | PNG 1254×1254；文件/视觉核验 |
| antigravity | image edit | PASS | 80.473 | 0/0 | JPEG 1024×1024；文件/视觉核验 |

Cursor 的 continue、generate、edit 均未启动：执行状态 NOT_TESTED，前置 Gate BLOCKED（workspace_trust）。首次失败 native=1、无输出/Session，xcli 归一化为 process_failed/parse_error；这不等于模型返回了坏 JSON。没有用另一个 CLI 替代 Cursor 的通过。最小调用形态如下（已经执行过，本轮不要重跑）：

```bash
"$BINARY" --config "$CONFIG" run --agent cursor --cwd "$ISOLATED_PROJECT" --json \
  'Read only r0-context.txt and reply with its exact marker.' \
  -- --mode ask --sandbox enabled
```

文本每次只读临时 Git 项目中的随机合成标记；继续调用不提供该标记、不允许读文件，标记相同且 Session ID 一致。原始输出不保存，仅留摘要和判定。Codex 使用 read-only/approval_policy=never，Cursor ask+sandbox，Antigravity plan+sandbox。为访问已有 Keychain/会话存储使用经审批的宿主执行环境；没有放松 CLI 自身沙箱或改变全局授权。

## 媒体字节与视觉

| 文件 | MIME | 尺寸 | 字节 | SHA-256 |
| --- | --- | --- | --- | --- |
| [codex-generate.png](evidence/native-20261106/artifacts/codex-generate.png) | image/png | 1254×1254 | 1959177 | `cf2638ab84bd063c83805ac5ff03d593ae5f6e81c31e7c36da511b39db969436` |
| [antigravity-generate.jpg](evidence/native-20261106/artifacts/antigravity-generate.jpg) | image/jpeg | 1024×1024 | 470653 | `a7ea7e3ae25038643d8b8f2e11fd1498242b58943dccc7e1d92dca3ebc81694e` |
| [codex-edit.png](evidence/native-20261106/artifacts/codex-edit.png) | image/png | 1254×1254 | 2073699 | `ea5568fa8213b14a75f21f05c34581252ae1290ea640c992de19cebb66515285` |
| [antigravity-edit.jpg](evidence/native-20261106/artifacts/antigravity-edit.jpg) | image/jpeg | 1024×1024 | 541008 | `9e852c568ed988cea9ee050a8c0d7d871aa07133ec568c6c5575e8c56a55fb41` |

完整解码与真实格式均通过，未强制转码或只改后缀。两次编辑的工作区参考文件哈希与证据原图均保持不变；Codex 全图归一化像素差约 0.070025，Antigravity 约 0.025542。人工视觉核对确认花盆变为鲜蓝色，植物、米白背景和主体构图保留；像素差不是单独的语义成功依据。

| CLI | 原图 | 编辑后 |
| --- | --- | --- |
| Codex | ![原图](evidence/native-20261106/artifacts/codex-generate.png) | ![编辑](evidence/native-20261106/artifacts/codex-edit.png) |
| Antigravity | ![原图](evidence/native-20261106/artifacts/antigravity-generate.jpg) | ![编辑](evidence/native-20261106/artifacts/antigravity-edit.jpg) |

### 工具来源与观察器修正

`native_audit.py` 只调用真实原生可执行文件，逐字节转发 stdout，不替换、制造或修饰模型回答；仅保存事件种类/工具标识/退出码/时间/输出摘要。离线测试验证转发字节完全一致且不保留聊天内容。

初版观察器把“未在外层 JSONL 中出现 image 工具名”当作生成失败；两张图其实已生成且有效。保留[最初观察记录](evidence/native-20261106/media-initial-observer.json)，在**零新增生成调用**下纠正了观察器判定：[生成复核](evidence/native-20261106/generation-review.json)。这不是把失败模型重试成功。

Codex 外层只显示 command_execution 等事件；在严格匹配本次隔离 cwd 的新会话记录中，仅提取工具标识，确认 generate/edit 都调用了 `image_gen__imagegen`，编辑还调用 `view_image`。[脱敏来源证据](evidence/native-20261106/codex-native-provenance.json)不含参数、输出、原始聊天或凭据。Antigravity 外层确认 invoke_subagent；新产生的 JPEG、提示目标及视觉结果一致，但子代理内部工具/模型调用数不透明，不声称精确 generate_image 次数。

两次编辑各仅一次，使用已复核生成图的精确字节副本，未覆盖原图。[编辑观察记录](evidence/native-20261106/media-edit-observer.json)与最终复核并列保存；最终 PASS 包含文件和人工视觉检查，不是单看 success/exit 0。

## 命令与复现边界

本轮新构建使用 `GOCACHE=/tmp/xcli-cli-first-go-cache GOPROXY=off GOSUMDB=off go build -trimpath -o "$BINARY" .`，退出 0。实际文本入口：

```bash
python3 tests/r0/smoke.py --binary "$BINARY" --execute \
  --preflight docs/cli-first/evidence/preflight.json --out "$PRIVATE/text.json"
```

先前离线门禁已完成，所以直接使用等效的原生 smoke，不重复执行包含假上游的整个 shell 包。媒体入口为 `tests/r0/native_media.py --execute`，固定最多各一次生成/编辑；观察器需要离线复核时使用 `--edit-only --verified-generations ... --prior-media-report ...`，只消费此前未使用的编辑槽，不重新生图。完整参数合同见脚本帮助与[手册](XCLI_USER_MANUAL.md)。**这些命令再次运行会消耗新额度，本轮已停止，不自动重跑。**

本轮在真实 Run 的私有 XDG 数据目录验证 `runs list --json`、`runs show ID` 均退出 0，原始 output_file 未保存；见[运行记录检查](evidence/native-20261106/run-inspection.json)。全部前台 CLI 已自然退出，未安装后台服务。没有强制取消/超时测试，不能宣称 Ctrl-C、后台 stop 或云端媒体撤销已真实验收。

## 离线门禁与修复回归

前一阶段基线 `f529e04` 的 `bash xcli-cli-first-acceptance/verify-phase1.sh --race` 最初受 Go 缓存/loopback 沙箱限制而失败；在临时缓存及获批宿主测试权限下退出 0。[通过日志](evidence/phase1-offline.log)、[缓存限制](evidence/phase1-sandbox-initial.log)、[loopback 限制](evidence/phase1-sandbox-cache.log)均保留。生命周期只产生 1 次假上游 HTTP 调用，不是 CLI 模型成功。

本轮没有改 Go 产品代码，因此不把旧 Go test/race 说成本轮重新运行。新构建通过；验收脚本离线回归 11/11 通过：[日志](evidence/native-20261106/harness-tests.log)。覆盖原有部分通过误判、无探测启动预算、API Key 屏蔽、同 Session 判定、透明转发、真实格式/symlink 拒绝，以及嵌套工具不可见时需复核而非虚假失败、有效图片不能盖过额外付费或权限阻断信号。

## Gate B 与四应用

`internal/modelstore/config.go` 仍只允许 `openai-compatible`、`openai-images`、`openai-compatible-embeddings`，`cmd/model.go` 仍仅创建 HTTP backend。此前离线复现 `type: native-cli` 被拒：`unsupported backend type (native CLI bridge is disabled)`，见[证据](evidence/gate-b.json)。这轮 Agent 成功没有改变此事实，也没有调用 `/v1/chat/completions` 或 `/v1/images/*` 验收原生 CLI。

四应用现有 HTTP provider 与未来 Agent 模式分开设计：[Worklog](integrations/WORKLOG.md)、[GardenFlow](integrations/GARDEN_FLOW.md)、[Chat Insight](integrations/CHAT_INSIGHT.md)、[Contribution OS](integrations/CONTRIBUTION_OS.md)。已有消费源码基线仅作只读参考，本轮没有修改四仓库、读取真实业务数据库或运行 UI，没有 Push、发布、生产部署或付费开通。[最终只读比较](evidence/native-20261106/consumer-preservation.json)中，前三仓库与先前基线一致；Contribution OS 的 HEAD/工作区元数据已变化，但所选接入源码摘要未变，本轮未写入或覆盖其独立改动。

下一步：若继续 Cursor，需要本人审查隔离项目并决定最小 trust 授权，再另立新的验收预算；本轮不自动处理。Native Agent Gateway 仍是[下一阶段方案](INTEGRATION_ARCHITECTURE.md)，需单独确认工程范围，先做认证/审批/预算/取消/Artifact 合同，再验收 HTTP→CLI；Embedding、严格 Schema、原生增量和应用 UI 分别保留独立门禁。
