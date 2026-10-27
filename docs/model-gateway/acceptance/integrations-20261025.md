# 四应用接入补丁续轮验收

日期：2026-10-25；环境：macOS arm64、Node 22.23.2。本轮真实模型调用0次；没有登录、读取真实凭据、迁移真实数据库、部署、Push或发布。经受保护apply写回Chat Insight的15个文件；其他原仓库本轮未写。

## 交付与边界

`integrations/`下四个`*-provider-full.patch`及对应JSON侧车共同交付，记录源码HEAD、补丁SHA-256、每个修改路径的原内容摘要。Worklog8文件、Chat Insight15文件、GardenFlow14文件、Contribution OS9文件。旧的`worklog-provider.patch`仅是早期最小provider补丁，不应与完整补丁叠加。

新增`tests/model_gateway/integration_workspace.py`：源码隔离快照、补丁生成、独立受保护应用；不复制数据库/凭据/环境文件/媒体/依赖/构建产物。既有脏源码和已被引用的未跟踪源码依赖保留。拒绝HEAD/原内容漂移、补丁/侧车路径集合不一致、父目录符号链接越界及基线被改写。一次读取的源码同时写入base/work，摘要绑定实际复制内容。

四补丁均在独立本地Git副本中执行完整guarded apply，并将所有改动文件与测试副本逐字节核对成功。原仓库最终只读检查：Worklog正好已包含补丁结果，`git apply --reverse --check`通过；Chat Insight随后经权限审核通过并完成受保护apply，写回15个文件均与测试副本一致、反向检查通过；另外两项`git apply --check`通过。GardenFlow/Contribution OS在续轮期间有新增用户改动，已刷新快照并三方合并集成代码，无冲突；没有覆盖原文件。

## 原生检查

| 应用/检查 | 命令与结果 | 证据限制 |
| --- | --- | --- |
| Worklog Rust | `CARGO_TARGET_DIR=/tmp/xcli-gateway-worklog-target cargo test --offline --manifest-path <work>/src-tauri/Cargo.toml --lib`：40通过 | HTTP测试监听临时loopback并返回合成报告；沙箱EPERM后同命令获准在沙箱外复验通过 |
| Worklog前端 | `<work>/frontend`中`npm run build`通过 | 未启动实际应用或使用Keyring真实账户 |
| Chat Insight Rust | `CARGO_TARGET_DIR=/tmp/xcli-gateway-chat-insight-target cargo test --offline --manifest-path <work>/src-tauri/Cargo.toml --lib`：96通过、2忽略 | 忽略项是需要真实模型的抽取评估；HTTP测试仅本机假服务，获准沙箱外执行 |
| Chat Insight前端 | `npm test -- --reporter=dot`：13文件/56通过；`npm run build`、`npm run lint`通过 | 合成UI测试；build仅有现有大chunk提示 |
| GardenFlow早期快照 | `node --test tests/*.test.ts`：763通过；两类tsc检查通过 | 这是合并最新用户改版前的快照，不能代表当前原仓库 |
| GardenFlow最新合并副本 | 全套762通过/1失败；xcli/图片/路由26项全部通过 | 原用户基线也失败，详见下方；没有标为全套通过 |
| Contribution OS | `python -m pytest tests/test_xcli_local.py tests/test_model_settings.py tests/test_provider_gateway.py tests/test_task_model_selection.py`：56通过/1跳过 | 跳过`test_real_gateway_network_reaches_only_internal_service`，需`CONTRIBOS_RUN_DOCKER_ACCEPTANCE=1`；未运行真实Docker验收 |
| Contribution OS静态 | `python -m compileall -q app`、`node --check app/static/model-settings.js`通过 | 未部署、未变更真实运行环境 |
| xcli补丁工具与参考客户端 | `python3 -m unittest discover -s tests/model_gateway -v`：8通过 | 临时Git夹具验证保留用户编辑、漂移拒绝、路径清单与符号链接边界 |

GardenFlow最新副本及未加网关改动的原用户基线均复现：

- `tests/workbenchNavigation.test.ts`仍期望compose=gardenflow/ideate=wander，而当前用户改版为works/opportunities。
- `tsc --noEmit --pretty false`：`electron/appMain.ts:12449`缺少`z`名称，`:12469`的`ProductVideoProjectMetadata`缺少`creationTarget`。
- `tsc -p tsconfig.renderer.json --pretty false`：`src/components/Layout.tsx:130`为TS1128语法错误。

这些路径未被集成补丁修改，现有用户改动完整保留；需对应改版修复后重跑。集成涉及的Settings.tsx与用户新布局修改已无冲突合并。

## 实现内容

Worklog保留既有业务prompt/Keyring，增加xcli provider、独立Token切换检查、严格loopback地址、无代理/重定向transport、受认证目录检查与脱敏错误；目录检查不调用模型。

Chat Insight移植逐模型目录位置识别及逐请求同意到Rust/React，remote/unknown/无效目录不能在确认前上传聊天；xcli禁用后台自动扫描。Embedding以endpoint/alias/dimensions/version/fingerprint识别，首次接入或身份变化要求显式重建，响应身份变化丢弃结果。直连provider保留原行为；敏感配置沿用该应用原有私有文件存储。

GardenFlow新增xcli Source和目录检查/位置提示，保留目录能力元数据，Pi要求经验证的Tool Calling/流式能力。图片走独立单次HTTP路径，保留size/n/参考图，不自动重试、删参、改尺寸或从编辑降级生成；拒绝返回URL下载，沿用媒体入库路径。暂未提供mask控件；真实媒体/工具流未验收。

Contribution OS新增显式操作员注册的`xcli_local`，仅允许`CONTRIBOS_XCLI_LOCAL_ENDPOINT`指定的loopback IP/端口及/v1。普通供应商仍走原公共HTTPS校验；独立加密Token引用、冻结Profile、短期任务Token和审计不变。目录确认JSON Schema/Tools后才发送任务内容；无代理、重定向或自动重试。容器执行明确拒绝，Host Bridge未实现。

## 恢复与剩余Gate

本轮未改动Go实现，因此沿用M5已记录的Go全套Race/Vet结果，没有将其记为本轮重跑。源码副本原生回归与网关协议画像是两个层面的离线证据，均不代表真实模型能力、应用数据流程、跨平台或部署验收完成。

恢复时读取PROGRESS.md和四个JSON侧车，核对原仓库HEAD/摘要，再应用尚未写回的补丁。Worklog与Chat Insight不要重复应用。Chat Insight源码已写回，但部署隐私修复前不得正式启用云路由。GardenFlow受测基线存在回归，且最终核对Settings.tsx又有新用户修改：普通apply检查通过，但摘要门禁将拒绝写入，需再次刷新快照、合并和测试。Contribution OS最终摘要匹配，仅native接入草稿，容器Bridge/真实Plan/Review仍阻断。真实上游、模型、凭据引用与合成请求预算仍需显式选定。

最终源码状态、补丁摘要及检查计数见[integrations-20261025.json](integrations-20261025.json)。Chat Insight首次写回请求的自动权限审核曾因审核用量限额未能完成，操作没有执行；用户指示继续后通过同一权限审核流程重试成功，没有绕过审核。
