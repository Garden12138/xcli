> 导出说明：experience-root 是本机体验目录的占位符；源码引用对应所选输入快照。原始文档及本机可打开的引用路径保持不变。

# 用户路径验收清单

本轮仅只读核对选定 `materials/` 快照（2026-10-24），未改输入。**静态确认不等于运行验收通过**；以下运行操作全部待执行。未运行测试、验证命令、桌面应用或真实微信读取，未重新计算输入哈希。详细调用链见 [onboarding-review.md](onboarding-review.md)。

命令公共证据：[api.ts][api] `api/call/inTauri` 负责包装；[lib.rs][lib] `run → generate_handler!` 注册，普通命令在 lib.rs，搜索／AI 命令在 [ai/commands.rs][commands]。表中 `包装/命令` 均按此注册；指引和原生 dialog 的例外另列。

## 已确认行为：静态源码核对

下表“操作”为本轮阅读核对；“未验证项”不视为通过。

| 编号／操作 | 预期结果（源码已确认） | 证据源码位置 | 未验证项 |
| --- | --- | --- | --- |
| S1 追踪启动及无数据分支 | 应用目录 SQLite；无账号进欢迎页。 | [main.rs][main] `main`；lib.rs `run/list_accounts`；[App.tsx][app] `App`；`capabilities/get_capabilities →` [bridge.rs][bridge] `capabilities`；`accounts/list_accounts →` [database.rs][db] `Database::list_accounts` | 建库、窗口、错误呈现。 |
| S2 对照浏览器／桌面入口 | 浏览器 browser/import_only、空账号、默认 AI、事件空订阅；其他 call 拒绝，原生 dialog 需桌面。 | [README.md「本地开发」][readme-dev]；[package.json][package] `scripts.dev/tauri`；api.ts `inTauri/browserCapabilities/call`；App `importData/exportData` | 两运行时的实际表现。 |
| S3 核对引导状态 | 无数据可 preview；有数据、空闲且未完成自动 tour；tour 完成／跳过记完成，preview 不记，侧栏可重看。 | [Onboarding.tsx][onboarding] `onShowGuide`；App `closeGuide/showGuide`；[GettingStarted.tsx][getting] `GettingStarted/advance`；[guide.ts][guide] `stepsFor/guideCompleted/markGuideCompleted`，DOM/localStorage，无命令 | 高亮、焦点、持久化。 |
| S4 核对平台及安装声明 | 直连只开放 Mac aarch64、Windows x86_64；Intel import_only；Mac DMG、Windows NSIS 是 README 声明。缺 cli 为 bridge_missing，仅缺 wxkey 在连接时报错。 | [README.md「平台能力」][readme-platform]；bridge.rs `capabilities/BridgePaths::resolve/connect`；Onboarding `canConnect`；[DataManagement.tsx][data] `canReadWechat` | 三平台包、桥完整性及上游兼容。 |
| S5 追踪连接／同步／取消 | bootstrap 成功才同步；sync-progress 回传；取消按边界检查并保留已写入数据。仅主界面非 preparing 可取消，欢迎页无按钮；同步后端只查 cli。 | App `bootstrapAndSync/runSync`；`bootstrap/bootstrap_bridge → lib.rs::connect_bridge → bridge::connect`；`sync/start_sync → bridge::sync_wechat/sync_chat`；`onSyncProgress/cancelSync/cancel_sync` | 权限、消息数量、取消时机。 |
| S6 核对换号与数据集选择 | 最近写入目录识别；备份恢复配置；同号不再同步，新号成功后同步。顶栏仅切展示数据集。 | App `switchAccount/accountId`；DataManagement `onSwitchAccount`；`switchAccount/switch_wechat_account → connect_bridge → bridge::connect/switch_account`；[wechat_account.rs][account] `detect_active_account/AccountStore::save/load` | 识别可靠性、失败恢复、隔离。 |
| S7 追踪 JSONL | dialog 取消不调用导入导出；v1 manifest/chat/message，导入集前缀 import:；事务、坏记录回滚、去重；允许引用已有聊天。 | [README.md「便携数据格式」][readme-jsonl]；App `importData/exportData → plugin-dialog::open/save`；`importJsonl/import_jsonl`、`exportJsonl/export_jsonl →` [portable.rs][portable] 同名函数、`PortableRecord/FORMAT/SCHEMA_VERSION` | 原生选文件、合成文件往返。 |
| S8 核对 JSONL 携带范围 | 导出当前数据集全量，不受日期筛选；**JSONL 不包含约定和群决策**、密钥、原库或媒体文件／专用路径字段。正文不脱敏；导入不清除已有约定决策。 | portable.rs `PortableRecord/import_jsonl/export_jsonl` 的枚举及 SQL；[README.md「开始使用 → 约定与待办」][readme-commitments] | 文件内容与重复导入后的数据库状态。 |
| S9 追踪关键词搜索及上下文 | 无需 AI；账号／聊天／日期筛选，最多六个不同词全匹配；长词 FTS、短词 LIKE，新到旧，页面最多 200。上下文无日期限制，转详情不保证定位消息。 | [Ask.tsx][ask] `submit/runSearch/context`；`searchMessages/search_messages → Database::search_messages`；[MessageContext.tsx][context] `MessageContext → messageContext/message_context → Database::message_context`；commands.rs 同名命令 | 中文命中、顺序、日期边界及性能。 |
| S10 核对 AI 默认与持久化 | enabled/allowCloud/autoScanCommitments=false；默认本机 127.0.0.1:11434/v1、qwen3:8b/bge-m3；已有设置恢复。 | [aiDefaults.ts][defaults] `defaultAiSettings`；[settings.rs][settings] `AiSettings::default/SettingsStore::load/save/view`；[AiProvider.tsx][provider] `AiProvider → aiSettings/ai_get_settings`；[AiSettings.tsx][ai-settings] `save/removeKey → saveAiSettings/ai_save_settings` | 重开恢复、Key 保存删除及权限。 |
| S11 核对无需 AI 的能力 | 连接同步、JSONL、关键词／上下文、图表筛选、时间线词频、年度统计及关系洞察不需模型；已存约定／决策可管理，扫描需 AI。 | [Overview.tsx][overview] `Overview → dashboard/query_dashboard → Database::dashboard`；[ChatDetail.tsx][detail] `ChatDetail/loadMore → chatDetail/query_chat_detail → Database::chat_detail`；[analysis.rs][analysis] `top_keywords`；[Report.tsx][report] `Report → yearInsights/query_year_insights →` [insights.rs][insights] `Database::year_insights`；`api.commitments/updateCommitment/decisions/updateDecision → lib.rs::list_commitments/update_commitment/list_decisions/update_decision → Database` 同名方法（[src-tauri/src/commitments.rs][local-commitments]、[src-tauri/src/decisions.rs][local-decisions]） | 关闭 AI 时逐项操作。 |
| S12 核对云端门禁 | 仅 localhost/127.0.0.1/::1 算本机，局域网算云端；生成需 enabled，云端需 allowCloud，正文需 cloudConsent。弹窗确认 true，取消／Esc／遮罩 null，中止正文调用；本机 false 放行。 | settings.rs `is_local_endpoint/ensure_allowed`；AiProvider `confirmCloud`；[CloudConsent.tsx][consent] `CloudConsent`；commands.rs 正文命令 | 实际网络零发送、主机及范围。 |
| S13 对照历史导览 | 主要路径一致；api 统一入口需排除 dialog/localStorage；JSONL 排除项与 AI 默认一致。自动启 Vite／加载 dist 缺选定 tauri.conf.json 依据。 | [chat-insight-experience.md「项目入口」][history-entry]、「[模块关系][history-modules]」；S1—S12 当前源码 | 桌面构建配置及历史陈述的运行结果。 |

## 需实际运行验证：待执行操作

建议后续使用合成 JSONL、受控模型服务；直连使用另备的授权环境。各行均**未执行**。

| 编号／操作 | 预期结果 | 证据源码位置 | 未验证项 |
| --- | --- | --- | --- |
| R1 分别打开浏览器预览、干净桌面；预览／跳过／重看引导 | 符合 S1—S3；浏览器不能实际导入查询，桌面数据就绪后引导。 | S1—S3 的 App/api/guide 链；README「本地开发」 | 所有 UI、存储及错误行为。 |
| R2 Apple Silicon 微信 4.x 登录后点连接 | 先提示完全磁盘访问、可能重启微信和管理员授权，再 `wxkey bootstrap`，成功后同步。 | Onboarding `onBootstrap`；App `bootstrapAndSync`；bridge.rs `run_platform_bootstrap/run_wxkey_bootstrap` | arm64 包、权限窗口、微信兼容。 |
| R3 Intel Mac 导入；Windows x64 登录并打开普通聊天后连接；另试缺桥 | Intel 无直连但可导入分析导出；Windows 用 wechat-cli.exe 执行 cache refresh --force；缺 cli 不给连接，JSONL 可用。 | S4/S7；bridge.rs `BridgePaths::resolve/run_platform_bootstrap` | 两平台包、Windows 初始化；Mac 仅缺 wxkey 的报错。 |
| R4 同步新增消息、取消重试；换号再切回；选顶栏数据集 | 增量写入，保留已完成数据；同号提示，新号同步，旧配置恢复；仅数据集选择不触发微信换号。 | S5/S6；App `runSync/switchAccount`；bridge.rs `sync_chat/switch_account` | 取消延迟、warnings、数据及配置隔离。 |
| R5 取消选文件；导入同一合成文件两次；导入坏版本、坏记录、无效时间、无聊天引用 | 取消无业务导入；重复无新增重复消息；失败不留下部分写入；已有聊天引用可成功。 | S7；portable.rs `import_jsonl` | 对话框、事务／去重实际结果。 |
| R6 改日期后导出并导入新集；目标另备已有约定决策 | 仍导出当前数据集全量正文；不迁入约定决策、媒体或密钥；既有约定决策不被重导清除。 | S8；portable.rs `export_jsonl/import_jsonl/PortableRecord` | 往返完整性、敏感字段及已有项保留。 |
| R7 AI 关闭时搜索多词、短词、指定聊天和日期，打开前后文 | 符合 S9；可跨搜索日期看上下文；另检查 200 条上限。 | Ask `runSearch`；commands.rs `search_messages/message_context`；Database 同名方法 | 中文检索和上下文边界、性能。 |
| R8 AI 关闭后查看图表、时间线词频、年度关系及已存约定决策；保存设置再重开 | 本地能力可用，模型扫描／生成不可用；已存设置恢复而非每次重置默认。 | S10/S11；[Commitments.tsx][commitments-view] `reload/update`；[ChatMemoryPanel.tsx][memory] `reload/updateDecision` | 功能与设置实际表现。 |
| R9 本机与远端分别生成；远端取消、Esc、遮罩，再确认；不允许云端时调用 | 本机无需云端弹窗；远端显示 host／范围，取消不启动正文操作，缺 allowCloud 或 cloudConsent 被后端拒绝；一次确认可覆盖多次 HTTP。 | S12；commands.rs `ai_build_index/ai_ask/ai_summarize_chat/ai_chat_topics/ai_scan_commitments/ai_scan_decisions` | 受控服务的请求记录及取消零发送。 |
| R10 建立、更新、重建语义索引 | 待索引文本发向量模型，重建全量；约 12 条／600 字分段，含聊天名、日期、发言人。 | `AiSettings.buildIndex → buildAiIndex/ai_build_index →` [indexer.rs][indexer] `build_index/index_chat/embedding_input` | 请求范围、分段、恢复；向量本地保存。 |
| R11 有／无索引问答 | 问题及最多 12 段发对话模型，含聊天名／时间／发言人；有索引另发问题向量；回退片段无总长 600 字硬限。 | `Ask.runAsk → ask/ai_ask →` [features.rs][features] `ask/ask_prompt`；[retrieval.rs][retrieval] `retrieve` | 载荷、引用、回答质量。 |
| R12 选聊天日期生成摘要；缩窄日期后归纳话题 | 摘要发当前范围文本，长文分段；话题先索引该聊天全部日期待处理文本，再按日期聚类，每话题最多 5 段。 | [AiSummaryPanel.tsx][summary] `generate → summarizeChat/ai_summarize_chat → features::summarize_chat`；[TopicsPanel.tsx][topics] `analyze → chatTopics/ai_chat_topics → indexer::index_chat/features::chat_topics`；`aiEstimate/ai_estimate` 本地估算 | 日期范围、估算、实际多次请求与缓存。 |
| R13 扫约定／群决策，改变群窗口是否有我发言，再增量续扫 | 先本地估算后确认；发送有线索待扫窗口、前最多 6 条上下文、最多 8 条有效项标题及负责人／截止日等；群约定需我发言、群决策不要求。已扫消息可能作为上下文重发。 | `Commitments.startScan/ChatMemoryPanel.scan → commitmentEstimate/ai_commitment_estimate、decisionEstimate/ai_decision_estimate`；`scanCommitments/ai_scan_commitments →` [ai/commitments.rs][ai-commitments] `scan_commitments`；`scanDecisions/ai_scan_decisions →` [ai/decisions.rs][ai-decisions] `scan_decisions`；[extraction.rs][extraction] `plan_chat/build_windows/scan/OPEN_ITEMS` | 筛选、增量与上下文载荷、结果质量。 |
| R14 开启本机自动约定扫描并同步新增消息；改用云端保存 | 仅新增且 enabled/isLocal/autoScanCommitments 才扫近 30 天；云端禁自动扫描，群决策无自动入口。 | App `runSync`；settings.rs `SettingsStore::save` | 触发条件及无新增时不调用。 |
| R15 生成年度叙述，切匿名；AI 关闭时保存并测试连接；打开已有缓存 | 年报发统计及名称、不发正文；云端默认代号可关闭，本机还原；无正文弹窗。连接测试仅需云端许可，取模型列表、发送固定“连接测试”。cached 摘要／话题／年报只读本机。 | `Report.generate → yearReport/ai_year_report → features::year_report/report_payload`，`ensure_allowed(false,false)`；`AiSettings.test → testAi/ai_test_connection`；commands.rs `cache_mode`；[client.rs][client] `list_models/complete/embed/request` | 匿名、缓存与真实网络载荷，含配置 Key 的鉴权。 |

## 改进建议（不作为现有行为验收）

| 操作 | 预期结果 | 证据源码位置 | 未验证项 |
| --- | --- | --- | --- |
| 补浏览器桌面提示、欢迎页取消及同步平台校验 | 减少入口与能力边界误解。 | App `importData`；Onboarding；bridge.rs `sync_wechat` | 尚未实现。 |
| 修订确认说明及旧导览概括 | 明示操作级确认、话题全日期索引、年报名称／测试例外，修正约 600 字和已读不再发送说法。 | [README.md「开始使用」][readme-start]、「[AI 功能][readme-ai]」；历史「项目入口」；Ask `runAsk`、TopicsPanel `analyze`、retrieval `retrieve`、extraction `build_windows` | 文案修改与后续运行复核。 |

本轮证据：已阅读并对照上述前端处理、api 映射、Rust 注册／实现、README 具体章节及历史导览；没有测试或应用通过日志。已有测试源码不代表执行成功。另一种 CLI 的独立复核仍待后续运行方执行，本轮未调用其他 Agent CLI。

[api]: ../../materials/src/api.ts
[lib]: ../../materials/src-tauri/src/lib.rs
[commands]: ../../materials/src-tauri/src/ai/commands.rs
[main]: ../../materials/src-tauri/src/main.rs
[app]: ../../materials/src/App.tsx
[bridge]: ../../materials/src-tauri/src/bridge.rs
[db]: ../../materials/src-tauri/src/database.rs
[package]: ../../materials/package.json
[onboarding]: ../../materials/src/views/Onboarding.tsx
[getting]: ../../materials/src/components/GettingStarted.tsx
[guide]: ../../materials/src/guide.ts
[data]: ../../materials/src/views/DataManagement.tsx
[account]: ../../materials/src-tauri/src/wechat_account.rs
[portable]: ../../materials/src-tauri/src/portable.rs
[ask]: ../../materials/src/views/Ask.tsx
[context]: ../../materials/src/components/MessageContext.tsx
[defaults]: ../../materials/src/aiDefaults.ts
[settings]: ../../materials/src-tauri/src/ai/settings.rs
[provider]: ../../materials/src/components/AiProvider.tsx
[ai-settings]: ../../materials/src/components/AiSettings.tsx
[overview]: ../../materials/src/views/Overview.tsx
[detail]: ../../materials/src/views/ChatDetail.tsx
[analysis]: ../../materials/src-tauri/src/analysis.rs
[report]: ../../materials/src/views/Report.tsx
[insights]: ../../materials/src-tauri/src/insights.rs
[consent]: ../../materials/src/components/CloudConsent.tsx
[commitments-view]: ../../materials/src/views/Commitments.tsx
[memory]: ../../materials/src/components/ChatMemoryPanel.tsx
[indexer]: ../../materials/src-tauri/src/ai/indexer.rs
[features]: ../../materials/src-tauri/src/ai/features.rs
[retrieval]: ../../materials/src-tauri/src/ai/retrieval.rs
[summary]: ../../materials/src/components/AiSummaryPanel.tsx
[topics]: ../../materials/src/components/TopicsPanel.tsx
[ai-commitments]: ../../materials/src-tauri/src/ai/commitments.rs
[ai-decisions]: ../../materials/src-tauri/src/ai/decisions.rs
[local-commitments]: ../../materials/src-tauri/src/commitments.rs
[local-decisions]: ../../materials/src-tauri/src/decisions.rs
[extraction]: ../../materials/src-tauri/src/ai/extraction.rs
[client]: ../../materials/src-tauri/src/ai/client.rs
[readme-dev]: ../../materials/README.md#本地开发
[readme-platform]: ../../materials/README.md#平台能力
[readme-jsonl]: ../../materials/README.md#便携数据格式
[readme-commitments]: ../../materials/README.md#约定与待办
[readme-start]: ../../materials/README.md#开始使用
[readme-ai]: ../../materials/README.md#ai-功能
[history-entry]: ../../materials/history/chat-insight-experience.md#项目入口
[history-modules]: ../../materials/history/chat-insight-experience.md#模块关系
