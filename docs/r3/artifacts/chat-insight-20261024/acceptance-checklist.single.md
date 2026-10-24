> 导出说明：experience-root 是本机体验目录的占位符；源码引用对应所选输入快照。原始文档及本机可打开的引用路径保持不变。

# 用户路径验收清单（待执行）

以下是**待执行步骤，不是运行通过记录**。仅有静态源码证据；本次未执行测试、验证命令、应用或真实微信读取，未修改输入。后续用合成 JSONL／受控模型，连接需另备授权环境。

公共链路：[api.ts](../../materials/src/api.ts) `api/call/inTauri` 封装命令；[lib.rs](../../materials/src-tauri/src/lib.rs) `run → generate_handler!` 注册全部命令，AI／搜索实现在 [ai/commands.rs](../../materials/src-tauri/src/ai/commands.rs)。表内 `方法/命令` 表示映射。

## 首次使用

| 操作 | 预期结果 | 证据源码位置 | 未验证项 |
| --- | --- | --- | --- |
| 浏览器打开欢迎页、预览指引并关闭／重开 | 账号为空、无直连；指引可看，导入／本地查询需桌面。AI 为默认值、事件空订阅，其余 call 拒绝。 | [README.md](../../materials/README.md)“本地开发”；api.ts `browserCapabilities/accounts/aiSettings/call/onSyncProgress`；[App.tsx](../../materials/src/App.tsx) `App`；[Onboarding.tsx](../../materials/src/views/Onboarding.tsx) `Onboarding` | 渲染及原生 dialog 错误。 |
| 干净桌面打开，导入合成样例 | 无数据先欢迎页；有数据、空闲且指引未完成才自动 tour。 | `App` 初始化 `capabilities/get_capabilities`、`accounts/list_accounts`；lib.rs `run/list_accounts`；[bridge.rs](../../materials/src-tauri/src/bridge.rs) `capabilities`；[database.rs](../../materials/src-tauri/src/database.rs) `Database::list_accounts` | 启动、建库与引导时序。 |
| 预览指引退出；tour 跳过／完成；重开应用，再点侧栏指引 | preview 不写完成标记，tour 关闭写标记；仍可手动重看并返回原页面。 | `App.closeGuide/showGuide`；[GettingStarted.tsx](../../materials/src/components/GettingStarted.tsx) `GettingStarted/advance`；[guide.ts](../../materials/src/guide.ts) `stepsFor/guideCompleted/markGuideCompleted`；直接 React/DOM/localStorage，不经 api.ts、无 Rust | 高亮定位、键盘、存储不可用时的表现。 |

## 连接与同步

本节平台预期取自 README.md“平台能力”、bridge.rs `capabilities/connect/run_platform_bootstrap`。下列 JSONL 能力是桌面便携路径，不依赖直连数据桥。

| 操作 | 预期结果 | 证据源码位置 | 未验证项 |
| --- | --- | --- | --- |
| Apple Silicon Mac，微信 4.x 登录，完整桥，点连接 | 提示完全磁盘访问、可能重启微信和管理员授权；执行 wxkey bootstrap 后同步。 | `Onboarding.onBootstrap → App.bootstrapAndSync → bootstrap/bootstrap_bridge → lib.rs::connect_bridge → bridge.rs::connect/run_wxkey_bootstrap` | arm64 包、wxkey、权限窗口及上游兼容。 |
| Intel Mac 导入合成 JSONL，查看数据管理 | import_only；无连接／换号，UI 同步禁用；图表、搜索、导出仍可用。 | bridge.rs `capabilities`；Onboarding `canConnect`；[DataManagement.tsx](../../materials/src/views/DataManagement.tsx) `canReadWechat`；[portable.rs](../../materials/src-tauri/src/portable.rs) `import_jsonl/export_jsonl` | x64 包、JSONL 往返；不代表全应用不支持。 |
| Windows x64，微信 4.x 已登录并打开普通聊天，点连接 | 使用 wechat-cli.exe，初始化执行 cache refresh --force，再同步；JSONL 同样可用。 | bridge.rs `BridgePaths::resolve/run_platform_bootstrap/sync_wechat`；`App.bootstrapAndSync`；portable.rs | Windows 包、密钥初始化和同步兼容。 |
| 支持平台使用缺桥环境；Apple Silicon 另模拟仅缺 wxkey | 缺 cli 时连接隐藏、同步禁用、提示 bridge_missing，JSONL 仍可用；仅缺 wxkey 可能仍见连接，点后报错。 | bridge.rs `capabilities/connect`；Onboarding `canConnect`；DataManagement `canReadWechat` | 开发环境／安装包真实资源；能力探测与实际错误一致性。 |
| 顶栏刷新／“立即同步”，取消后重试 | 独立账号增量写入、sync-progress 回传；按边界取消、保留已写数据。仅主界面非 preparing 有取消按钮，欢迎页无。 | `App.runSync → sync/start_sync → bridge.rs::sync_wechat/sync_chat`；`onSyncProgress`；`cancelSync/cancel_sync`；lib.rs `start_sync/cancel_sync` | 数量、警告与取消时机；后端只查 cli，不复核架构。 |
| 微信切新号并打开聊天后换号；同号重试；切回旧号；选顶栏数据集 | 新号授权后同步；同号不再同步；旧配置恢复。顶栏选择只换展示。 | `DataManagement.onSwitchAccount → App.switchAccount → switchAccount/switch_wechat_account → bridge.rs::connect/switch_account`；[wechat_account.rs](../../materials/src-tauri/src/wechat_account.rs) `detect_active_account/AccountStore::save/load`；App `accountId` | 最近目录识别、失败恢复与账号隔离。 |

## JSONL 导入导出

| 操作 | 预期结果 | 证据源码位置 | 未验证项 |
| --- | --- | --- | --- |
| 在桌面欢迎页／数据管理选择合成 v1 文件；先取消再重选 | 取消不导入；成功创建／更新 import: 数据集并刷新页面。选文件直接用 Tauri dialog，不经 api.ts。 | `Onboarding.onImport/DataManagement.onImport → App.importData → plugin-dialog::open → importJsonl/import_jsonl → portable.rs::import_jsonl`；lib.rs `run` 注册 dialog 插件 | 三平台文件选择、错误反馈、导入数量。 |
| 当前数据集设置窄日期范围后导出，再在另一桌面导入 | 导出整数据集，首行 manifest、其后 chat/message；不只导出当前日期；两种对话与正文可迁移。 | `DataManagement.onExport → App.exportData → plugin-dialog::save`（绕过 api.ts）→ `exportJsonl/export_jsonl → portable.rs::export_jsonl/PortableRecord/FORMAT/SCHEMA_VERSION` | 跨 Apple Silicon、Intel Mac、Windows 往返、文件编码及计数。 |
| 重导；导入未知 schema、坏 JSON／时间、新集未声明聊天消息 | 消息去重；坏文件失败回滚。可引用数据库已有聊天，不强制本次声明。 | portable.rs `import_jsonl` 的 transaction、INSERT OR IGNORE、EXISTS、commit；README.md“便携数据格式” | 回滚状态与既有数据保留。 |
| 有约定／群决策的源集导出至新集；已有本机约定的导入集重导 | **JSONL 不包含约定和群决策**；不迁入也不清除已有项。无密钥、原库、媒体文件／专用路径字段，text 原样携带。 | portable.rs `PortableRecord/export_jsonl/import_jsonl`：仅 manifest/chat/message，查询 accounts/chats/messages；README.md“约定与待办／便携数据格式” | 记录与保留情况；正文不保证脱敏。 |

## 关键词搜索

| 操作 | 预期结果 | 证据源码位置 | 未验证项 |
| --- | --- | --- | --- |
| AI 关闭进入搜索页，输入单词、空白分隔多词、中文长短词，切聊天和日期 | 无会话选择时默认关键词模式；无需 AI／语义索引。按账户、聊天、日期过滤，前六个不同词全部匹配；≥3 字走 trigram FTS5，短词 LIKE；新到旧，UI 上限 200。 | [Ask.tsx](../../materials/src/views/Ask.tsx) `Ask/submit/runSearch → searchMessages/search_messages → Database::search_messages` | 中文、引号／通配符、无结果、大数据性能和空白输入 UI。 |
| 点搜索结果、关闭弹窗，再“在聊天详情中打开” | 本机读取邻接消息，无模型；上下文不带日期参数，可超出搜索范围；打开聊天详情不保证定位原条。 | [MessageContext.tsx](../../materials/src/components/MessageContext.tsx) `MessageContext → messageContext/message_context → Database::message_context`；`App.openDetail` | local_id 邻接顺序、焦点与滚动；搜索范围与上下文的差别。 |

## AI 开关与云端确认

| 操作 | 预期结果 | 证据源码位置 | 未验证项 |
| --- | --- | --- | --- |
| 使用无 AI 配置的桌面打开设置，再保存并重开 | **AI 默认关闭**，allowCloud、自动扫描也关闭；默认本机地址、qwen3:8b/bge-m3；既有设置应恢复。 | [aiDefaults.ts](../../materials/src/aiDefaults.ts) `defaultAiSettings`；[settings.rs](../../materials/src-tauri/src/ai/settings.rs) `AiSettings::default/SettingsStore::load/save`；[AiProvider.tsx](../../materials/src/components/AiProvider.tsx) `AiProvider → aiSettings/ai_get_settings`；[AiSettings.tsx](../../materials/src/components/AiSettings.tsx) `save → saveAiSettings/ai_save_settings` | 持久化、配置不可读回退、独立 Key 文件及权限。 |
| AI 关闭，导入后看总览、时间线、词频、年度数字 | **无需 AI 的功能**可用；扫描、问答、摘要、话题和年报生成需 AI。 | [Overview.tsx](../../materials/src/views/Overview.tsx) `Overview → dashboard/query_dashboard → Database::dashboard`；[ChatDetail.tsx](../../materials/src/views/ChatDetail.tsx) `ChatDetail/loadMore → chatDetail/query_chat_detail → Database::chat_detail →` [analysis.rs](../../materials/src-tauri/src/analysis.rs) `top_keywords`；[Report.tsx](../../materials/src/views/Report.tsx) `Report → yearInsights/query_year_insights →` [insights.rs](../../materials/src-tauri/src/insights.rs) `Database::year_insights` | 图表、筛选与模型未运行时的可用性。 |
| 关闭 AI 查看／编辑已存约定和群决策 | 本机列表和更新不检查 AI 开关；不生成新建议。 | [Commitments.tsx](../../materials/src/views/Commitments.tsx) `update`、[ChatMemoryPanel.tsx](../../materials/src/components/ChatMemoryPanel.tsx) `updateDecision` → `api.commitments/updateCommitment/decisions/updateDecision → list_commitments/update_commitment/list_decisions/update_decision →` [本地 commitments.rs](../../materials/src-tauri/src/commitments.rs)、[本地 decisions.rs](../../materials/src-tauri/src/decisions.rs) 同名 Database 方法 | 保存、确认和编辑后的 UI 状态。 |
| 保存三个回环地址与局域网／公网地址；未允许云端尝试生成 | localhost/127.0.0.1/::1 算本机，其余算云端；正文需 enabled，云端还需 allowCloud+cloudConsent，否则拒绝。 | settings.rs `is_local_endpoint/AiSettings::ensure_allowed`；AiSettings `save`；commands.rs 各正文命令 | 服务路由、拒绝错误；地址分类不代表抓包结论。 |
| 本机／云端建、更新、重建索引；云端取消／Esc／遮罩／确认 | **云端内容发送确认**显示 host、数量与范围；取消中止，本机不弹。一次确认覆盖多批次；重建全量、增量待处理文本，含聊天名／日期。 | `AiSettings.buildIndex → AiProvider.confirmCloud →` [CloudConsent.tsx](../../materials/src/components/CloudConsent.tsx) `CloudConsent`（弹窗无 api.ts／Rust）→ `buildAiIndex/ai_build_index →` [indexer.rs](../../materials/src-tauri/src/ai/indexer.rs) `build_index/index_chat/embedding_input` | 取消零发送、批次、约 12 条／600 字分段与进度。 |
| 云端问答，有索引／无索引各试一次并查看来源 | 确认后发问题及最多 12 段，含聊天名／时间／发言人；有索引另做问题向量化，无索引可关键词回退。不要把弹窗约 600 字当硬上限。 | `Ask.runAsk → ask/ai_ask →` [features.rs](../../materials/src-tauri/src/ai/features.rs) `ask/ask_prompt`；[retrieval.rs](../../materials/src-tauri/src/ai/retrieval.rs) `retrieve` 回退最多 7 条上下文、各截 300 字 | 载荷上限提示偏差、引用对应、无匹配及质量。 |
| 云端生成／重生摘要话题；打开缓存 | 摘要发当前范围文本；话题先索引聊天全部日期待处理消息，再按范围聚类、每话题最多 5 段；cached 只读本机。 | [AiSummaryPanel.tsx](../../materials/src/components/AiSummaryPanel.tsx) `generate → summarizeChat/ai_summarize_chat`；[TopicsPanel.tsx](../../materials/src/components/TopicsPanel.tsx) `analyze → chatTopics/ai_chat_topics`；`aiEstimate/ai_estimate →` [store.rs](../../materials/src-tauri/src/ai/store.rs) `Database::ai_estimate`；commands.rs `cache_mode`、indexer `index_chat`、features `summarize_chat/chat_topics` | 估算、全日期索引、缓存失效与分段请求。 |
| 云端扫约定／群决策，取消／确认；对比私聊群聊 | 发当前范围有线索待扫窗口、前最多 6 条上下文、最多 8 条有效项标题等；约定群窗口需我发言，决策无需。私聊无决策扫描；上下文可重带已读消息。 | `Commitments.startScan/ChatMemoryPanel.scan → commitmentEstimate/decisionEstimate → ai_commitment_estimate/ai_decision_estimate` 本地估算；`scanCommitments/scanDecisions → ai_scan_commitments/ai_scan_decisions →` [ai/commitments.rs](../../materials/src-tauri/src/ai/commitments.rs) `scan_commitments/CommitmentExtractor`、[ai/decisions.rs](../../materials/src-tauri/src/ai/decisions.rs) `scan_decisions/DecisionExtractor`；[extraction.rs](../../materials/src-tauri/src/ai/extraction.rs) `build_windows/scan/OPEN_ITEMS` | 线索、续扫与效果；README 已读不再发送仅指主窗口。 |
| 本机开启同步后自动扫描；再保存云端地址；同步有／无新增 | 仅 enabled、本机、自动扫描开关且有新增时扫近 30 天约定；云端保存强制关闭自动扫描；群决策只手动。 | `App.runSync → scanCommitments/ai_scan_commitments`；settings.rs `SettingsStore::save`；ChatMemoryPanel `scan` | 同步后触发、零新增分支、窗口与中断。 |
| 云端年报保留／取消代号；AI 开／关时测试连接 | 年报需 enabled+allowCloud，不弹正文确认，仅统计／名称，默认代号可取消；测试仅需云端 allowCloud，不要求 enabled，发固定“连接测试”。 | `Report.generate → yearReport/ai_year_report → features::year_report/report_payload/restore_names`；`AiSettings.test → testAi/ai_test_connection`，后者不调用 ensure_allowed | 名称还原、载荷、模型列表／向量；README 逐次确认有这些例外。 |

上述未验证项均留待授权环境执行；静态证据不足以确认安装包可用、实际权限、数据完整性或模型质量。
