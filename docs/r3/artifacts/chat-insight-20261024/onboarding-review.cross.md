> 导出说明：experience-root 是本机体验目录的占位符；源码引用对应所选输入快照。原始文档及本机可打开的引用路径保持不变。

# 首次使用路径静态核对

本轮依据选定 `materials/` 快照（2026-10-24）只读追踪。**已确认行为仅指源码可见行为**，不代表应用运行通过。未运行测试、验证命令、桌面应用或真实微信读取，未修改输入；提供的 SHA-256 未重新计算。

公共入口：[main.rs][main] `main → chat_insight_lib::run`；[lib.rs][lib] `run` 初始化应用目录的 `chat-insight.db`、安装 dialog 插件，并以 `generate_handler!` 注册下表全部命令。[api.ts][api] `api` 是业务包装；原生文件对话框、指引另有入口。

## 已确认行为

### 首次使用

| 用户操作 | 前端 → api.ts 包装／命令 → Rust 实现及边界 |
| --- | --- |
| 打开无数据应用 | [App.tsx][app] `App` 初始化调用 `capabilities/get_capabilities → bridge.rs::capabilities`、`accounts/list_accounts → Database::list_accounts`；无数据集显示 [Onboarding.tsx][onboarding] `Onboarding`。实现见 [bridge.rs][bridge]、[database.rs][db]。 |
| 先看／重看指引 | `Onboarding.onShowGuide → App.guideMode=preview`；有数据、空闲且未完成时自动 tour；侧栏 `App.showGuide` 可重看。[GettingStarted.tsx][getting] `GettingStarted/advance`、[guide.ts][guide] `stepsFor/guideCompleted/markGuideCompleted` 直接操作 DOM/localStorage，无 api.ts 或 Rust 命令。tour 完成或跳过均记完成；preview 不记。 |
| 浏览器与桌面 | [README.md「本地开发」][readme-dev]、[package.json][package] `scripts.dev/tauri`：`npm run dev` 为浏览器预览，`npm run tauri dev` 为完整桌面入口。api.ts `inTauri/browserCapabilities/call`：浏览器返回 browser/import_only、空账号及默认 AI 设置，事件为空订阅，其余业务调用拒绝；导入的原生 dialog 也需桌面。不能把空欢迎页当真实数据体验。 |

### 连接与同步

| 用户操作 | 前端 → api.ts 包装／命令 → Rust 实现 |
| --- | --- |
| 连接后同步 | `Onboarding.onBootstrap → App.bootstrapAndSync → bootstrap/bootstrap_bridge → lib.rs::connect_bridge → bridge.rs::connect/run_platform_bootstrap`；成功后 `App.runSync → sync/start_sync → bridge.rs::sync_wechat/sync_chat`，写应用快照。 |
| 更新／取消 | 顶栏刷新或 [DataManagement.tsx][data] `onSync → App.runSync`；`onSyncProgress` 订阅 `sync-progress`。主界面非 preparing 才显示取消：`cancelSync/cancel_sync → lib.rs::cancel_sync` 设置标记，后端按聊天／页检查，已写数据保留；欢迎页无取消按钮。 |
| 换微信号／切数据集 | `DataManagement.onSwitchAccount → App.switchAccount → switchAccount/switch_wechat_account → connect_bridge → bridge.rs::connect/switch_account`；[wechat_account.rs][account] `detect_active_account` 按最近写入目录识别，`AccountStore::save/load` 备份恢复配置。同号不再同步，新号成功后同步。顶栏只改 `accountId`，不切微信登录。 |

| 桌面平台 | 直连边界；JSONL 均可作为桌面便携路径 |
| --- | --- |
| Apple Silicon Mac | `capabilities` 允许 aarch64；需 cli，连接另查 wxkey，`run_wxkey_bootstrap` 执行 `wxkey bootstrap`。事先提示完全磁盘访问、可能重启微信及管理员授权。 |
| Intel Mac | x86_64 返回 import_only；欢迎页不显示连接，数据管理禁用同步、隐藏换号。支持 JSONL 导入分析和导出，不能概括为整个应用不支持。 |
| Windows x64 | x86_64 允许直连；`BridgePaths::resolve` 取 `wechat-cli.exe`，初始化执行 `cache refresh --force`；提示微信 4.x 已登录并打开普通聊天。 |
| 缺桥 | 缺 cli 为 bridge_missing，连接隐藏、同步禁用；仅缺 wxkey 到连接时才失败。`sync_wechat` 只查 cli，未复核架构，UI 能力限制不等于所有命令都有平台拒绝。 |

依据：[README.md「平台能力」][readme-platform]、bridge.rs `capabilities/BridgePaths::resolve/connect`、Onboarding `canConnect`、DataManagement `canReadWechat`。README 声明两种 Mac DMG、Windows NSIS；包内桥、签名和实际兼容未核验。开发环境默认无桥见「本地开发」。

### JSONL 导入导出

| 用户操作 | 链路与携带范围 |
| --- | --- |
| 欢迎页／数据管理导入，数据管理导出 | `App.importData/exportData → plugin-dialog::open/save`（绕过 api.ts，lib.rs 安装插件）→ `api.importJsonl/import_jsonl`、`api.exportJsonl/export_jsonl → lib.rs` 同名命令 → [portable.rs][portable] `import_jsonl/export_jsonl`。取消选文件不调用业务导入导出；成功导入刷新账号及页面。 |
| 文件约定 | `PortableRecord/FORMAT/SCHEMA_VERSION`：首行 manifest，之后 chat/message，`chat-insight-jsonl` v1；导入到 `import:{dataset_id}`，事务提交，未知版本／坏记录／无效时间／找不到聊天时失败回滚；`INSERT OR IGNORE` 去重。聊天可已存在于同一数据集，不必每次重声明。 |
| 导出边界 | 当前数据集全部聊天消息，不受顶部日期范围限制。**JSONL 不包含约定和群决策**，也无密钥、原库、媒体文件或专用绝对媒体路径字段；`text` 原样输出，不是脱敏格式。导入不迁入约定／决策，也不清除目标中已有项。依据枚举及两函数 SQL；对照 [README.md「便携数据格式」][readme-jsonl]、「开始使用 → 约定与待办」。 |

### 关键词搜索

| 用户操作 | 前端 → api.ts 包装／命令 → Rust 实现及结果边界 |
| --- | --- |
| 搜索与问答 → 关键词搜索 | [Ask.tsx][ask] `submit/runSearch → searchMessages/search_messages →` [ai/commands.rs][commands] `search_messages → Database::search_messages`。不需 AI；按账号、可选聊天及日期过滤，最多六个不同空白分隔词全部匹配，长词走 FTS、短词 LIKE，新到旧，页面请求上限 200。AI 关闭且无已选模式时默认搜索。 |
| 打开结果原话 | Ask `context →` [MessageContext.tsx][context] `MessageContext → messageContext/message_context → commands.rs::message_context → Database::message_context`；按 local_id 取邻接记录，不传日期，可超出搜索日期。转聊天详情仅传 chatId，不保证定位原消息。 |

### AI 开关与云端确认

| 已确认行为 | 入口／映射与依据 |
| --- | --- |
| 默认关闭并恢复已存设置 | [aiDefaults.ts][defaults] `defaultAiSettings`、[settings.rs][settings] `AiSettings::default`：enabled、allowCloud、autoScanCommitments 均 false；默认 127.0.0.1:11434/v1、qwen3:8b、bge-m3。`SettingsStore::load` 恢复已有配置。数据管理 → [AiSettings.tsx][ai-settings] `save/removeKey → saveAiSettings/ai_save_settings → SettingsStore::save`；[AiProvider.tsx][provider] `AiProvider → aiSettings/ai_get_settings → SettingsStore::view`，命令在 commands.rs。 |
| 无需 AI | 连接同步、JSONL、关键词搜索及上下文；[Overview.tsx][overview] `Overview → dashboard/query_dashboard → Database::dashboard`；[ChatDetail.tsx][detail] `ChatDetail/loadMore → chatDetail/query_chat_detail → Database::chat_detail →` [analysis.rs][analysis] `top_keywords`；[Report.tsx][report] `Report → yearInsights/query_year_insights →` [insights.rs][insights] `Database::year_insights`。图表、筛选、时间线、词频、年度统计／关系洞察均本地。已存约定／决策可管理：`Commitments.reload/update`、`ChatMemoryPanel.reload/updateDecision → api.commitments/updateCommitment/decisions/updateDecision → lib.rs::list_commitments/update_commitment/list_decisions/update_decision → Database` 同名方法，位于 [src-tauri/src/commitments.rs][local-commitments]、[src-tauri/src/decisions.rs][local-decisions]；扫描才需模型。 |
| 确认流程 | settings.rs `is_local_endpoint` 只认 localhost/127.0.0.1/::1，局域网也算云端；`ensure_allowed` 要 enabled，云端另需 allowCloud，正文操作再需 cloudConsent。AiProvider `confirmCloud` 本机返回 false，远端弹 [CloudConsent.tsx][consent] `CloudConsent` 显示主机与范围；确认 true，取消／Esc／遮罩为 null，中止后续调用。一次操作可含多次模型请求。 |

下表均由 lib.rs 注册、commands.rs 实现；正文生成命令调用 `ensure_allowed(true, cloud_consent)`。估算命令只查本机。发送到所配置服务的实际载荷见 [client.rs][client] `OpenAiCompatClient::request/complete/embed`。

| 内容操作 | 前端 → api 方法／命令 → Rust；云端内容 |
| --- | --- |
| 语义索引 | `AiSettings.buildIndex → buildAiIndex/ai_build_index →` [indexer.rs][indexer] `build_index/index_chat/embedding_input`：数据集待索引文本，重建全量；约 12 条／600 字分段，含聊天名、日期、发言人。 |
| 问答 | `Ask.runAsk → ask/ai_ask →` [features.rs][features] `ask/ask_prompt`、[retrieval.rs][retrieval] `retrieve`：问题及最多 12 段，含聊天名、时间、发言人；有索引还将问题发给向量模型。无索引可回退关键词，但片段不保证总长 600 字。 |
| 摘要 | [AiSummaryPanel.tsx][summary] `generate → summarizeChat/ai_summarize_chat → features::summarize_chat`：当前聊天／日期文本，长文分段。`aiEstimate/ai_estimate` 为本地计数。 |
| 常聊话题 | [TopicsPanel.tsx][topics] `analyze → chatTopics/ai_chat_topics → indexer::index_chat + features::chat_topics`：先处理该聊天全部日期的待索引文本，再按所选日期聚类，每话题最多 5 段代表文本。 |
| 约定／群决策 | [Commitments.tsx][commitments-view] `startScan`／[ChatMemoryPanel.tsx][memory] `scan` 先本地 `commitmentEstimate/ai_commitment_estimate`、`decisionEstimate/ai_decision_estimate`；确认后 `scanCommitments/ai_scan_commitments →` [ai/commitments.rs][ai-commitments] `scan_commitments`、`scanDecisions/ai_scan_decisions →` [ai/decisions.rs][ai-decisions] `scan_decisions`，均入 [extraction.rs][extraction] `plan_chat/build_windows/scan`。发送当前范围有线索的待扫窗口、前最多 6 条上下文、最多 8 条有效项的标题及负责人／截止日等。群约定窗口需我发言，群决策不要求。 |
| 自动扫描 | `App.runSync` 仅新增消息且 enabled、isLocal、autoScanCommitments 时扫近 30 天；云端保存设置强制关闭自动扫描，群决策无同步后自动入口。 |
| 年度叙述／连接测试例外 | `Report.generate → yearReport/ai_year_report → features::year_report/report_payload` 用 `ensure_allowed(false,false)`，发送统计及对象名称，不发正文；云端默认代号，可取消匿名，结果本机还原。`AiSettings.test → testAi/ai_test_connection` 仅检查云端 allowCloud，AI 关闭也能取模型列表并发送固定“连接测试”。二者无正文弹窗。摘要／话题／年报 `mode=cached` 只读本机。 |

## 旧导览核对

历史文档仅作线索，以下按当前源码裁定；本范围未发现可确证为旧版本行为的过时项。

| 旧文具体章节／说法 | 当前核对 |
| --- | --- |
| [chat-insight-experience.md「项目入口」][history-entry]：启动方式、源码入口、页面与体验路径 | 浏览器空数据、欢迎页／指引、搜索与 AI 入口一致。统一 api 入口须排除原生 dialog 和 localStorage。Tauri 自动启 Vite、构建载入 dist 需 tauri.conf.json，未在选定输入中，不能据旧文确认。 |
| [「模块关系 → Rust 数据与业务模块／AI 调用链」][history-modules] | JSONL 排除约定／决策、AI 默认关闭及前后端正文确认一致。 |
| [README.md「开始使用」][readme-start]、[「隐私边界 → AI 功能」][readme-ai] | “同步可取消”应限定现有 UI；“已读不再发送”仍可能作为前 6 条上下文发送；逐次确认指正文操作，不能扩为所有网络请求。Ask 的约 600 字提示也不是回退检索的硬上限。 |

## 改进建议

| 建议 | 源码触发点 |
| --- | --- |
| 浏览器导入入口说明桌面要求；补欢迎页取消；同步命令统一检查平台能力。 | App `importData`、Onboarding、bridge.rs `sync_wechat`。 |
| 明示话题全日期索引、操作级确认、年报名称／连接测试例外；修订 600 字与已读消息说法。 | TopicsPanel `analyze`、commands.rs、retrieval `retrieve`、extraction `build_windows`。 |

## 需实际运行验证

| 范围 | 尚无运行证据 |
| --- | --- |
| 首次使用／连接同步 | 引导布局和持久化；三平台包、数据桥、权限、微信兼容、换号、取消及数据保留。 |
| JSONL／搜索／AI | 合成样例往返、回滚去重、中文检索、日期边界与性能；受控服务下取消零发送、实际载荷、缓存、匿名和模型质量。步骤见 [acceptance-checklist.md](acceptance-checklist.md)。 |

本轮证据为以上源码读取及符号链对照；既有测试仅是定义，无本轮通过记录。未启动其他 Agent CLI；另一种 CLI 的独立复核交由后续运行方完成。

[main]: ../../materials/src-tauri/src/main.rs
[lib]: ../../materials/src-tauri/src/lib.rs
[api]: ../../materials/src/api.ts
[app]: ../../materials/src/App.tsx
[onboarding]: ../../materials/src/views/Onboarding.tsx
[getting]: ../../materials/src/components/GettingStarted.tsx
[guide]: ../../materials/src/guide.ts
[package]: ../../materials/package.json
[bridge]: ../../materials/src-tauri/src/bridge.rs
[db]: ../../materials/src-tauri/src/database.rs
[data]: ../../materials/src/views/DataManagement.tsx
[account]: ../../materials/src-tauri/src/wechat_account.rs
[portable]: ../../materials/src-tauri/src/portable.rs
[ask]: ../../materials/src/views/Ask.tsx
[context]: ../../materials/src/components/MessageContext.tsx
[commands]: ../../materials/src-tauri/src/ai/commands.rs
[defaults]: ../../materials/src/aiDefaults.ts
[settings]: ../../materials/src-tauri/src/ai/settings.rs
[ai-settings]: ../../materials/src/components/AiSettings.tsx
[provider]: ../../materials/src/components/AiProvider.tsx
[consent]: ../../materials/src/components/CloudConsent.tsx
[overview]: ../../materials/src/views/Overview.tsx
[detail]: ../../materials/src/views/ChatDetail.tsx
[analysis]: ../../materials/src-tauri/src/analysis.rs
[report]: ../../materials/src/views/Report.tsx
[insights]: ../../materials/src-tauri/src/insights.rs
[client]: ../../materials/src-tauri/src/ai/client.rs
[indexer]: ../../materials/src-tauri/src/ai/indexer.rs
[features]: ../../materials/src-tauri/src/ai/features.rs
[retrieval]: ../../materials/src-tauri/src/ai/retrieval.rs
[summary]: ../../materials/src/components/AiSummaryPanel.tsx
[topics]: ../../materials/src/components/TopicsPanel.tsx
[commitments-view]: ../../materials/src/views/Commitments.tsx
[memory]: ../../materials/src/components/ChatMemoryPanel.tsx
[ai-commitments]: ../../materials/src-tauri/src/ai/commitments.rs
[ai-decisions]: ../../materials/src-tauri/src/ai/decisions.rs
[local-commitments]: ../../materials/src-tauri/src/commitments.rs
[local-decisions]: ../../materials/src-tauri/src/decisions.rs
[extraction]: ../../materials/src-tauri/src/ai/extraction.rs
[readme-dev]: ../../materials/README.md#本地开发
[readme-platform]: ../../materials/README.md#平台能力
[readme-jsonl]: ../../materials/README.md#便携数据格式
[readme-start]: ../../materials/README.md#开始使用
[readme-ai]: ../../materials/README.md#ai-功能
[history-entry]: ../../materials/history/chat-insight-experience.md#项目入口
[history-modules]: ../../materials/history/chat-insight-experience.md#模块关系
