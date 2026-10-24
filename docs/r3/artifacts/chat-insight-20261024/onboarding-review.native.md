> 导出说明：experience-root 是本机体验目录的占位符；源码引用对应所选输入快照。原始文档及本机可打开的引用路径保持不变。

# Chat Insight 维护者上手复核

依据当前输入源码及 [README.md][readme] 静态核对，范围仅为下列五条用户路径。输入根目录为 `experience-root/materials`；文件链接指向该目录，紧随其后的符号用于定位。[旧导览][history]仅作线索，不能代替当前实现。**本次未启动浏览器或桌面应用、未读取真实微信数据、未运行测试。**

## 已确认行为

### 1. 首次使用

| 用户路径 | 前端入口与行为 | api.ts 映射 → Rust 实现 |
| --- | --- | --- |
| 启动、等待加载 | [App.tsx][app] `App` 获取能力和数据集；无数据集显示 [Onboarding.tsx][onboarding] `Onboarding`，有数据集显示六页主界面。 | [api.ts][api] `api.capabilities/accounts` → `get_capabilities/list_accounts` → [lib.rs][lib] 同名命令、[bridge.rs][bridge] `capabilities`、[database.rs][database] `Database::list_accounts`。 |
| 先了解再连接 | 欢迎页「先看使用指引」进入 preview；有数据且不忙、不在同步、未完成指引时自动进入 tour；跳过或关闭 tour 会记完成，侧栏可重开。 | [App.tsx][app] `guideMode/autoGuide/closeGuide/showGuide`；[guide.ts][guide] `previewSteps/tourSteps/guideCompleted/markGuideCompleted`；无 Rust 命令，完成标记在浏览器 localStorage。 |
| 数据就绪后的路线 | 先选时间，再总览→聊天详情→搜索→年度报告；另有约定与待办、数据管理。总览/详情/搜索使用顶部范围，年度报告另选年份。 | [App.tsx][app] `navItems/range/openDetail`；[api.ts][api] `dashboard/chats/chatDetail/yearInsights` → `query_dashboard/list_chats/query_chat_detail/query_year_insights` → [lib.rs][lib] 对应命令、[database.rs][database] `Database::dashboard/list_chats/chat_detail`、[ai/commands.rs][commands] `query_year_insights` → [insights.rs][insights] `Database::year_insights`。 |

| 运行边界 | 源码核对后的维护口径 |
| --- | --- |
| 浏览器预览 | [package.json][package] `scripts.dev` 为 Vite；[vite.config.ts][vite] `server.port=1420`。[api.ts][api] `inTauri/browserCapabilities/call` 返回 browser、无直读/桥、空账号、默认 AI 设置；一般数据操作抛桌面专用错误。只能预览欢迎页和指引，不能据此验收导入、同步或真实查询。文件选择器也依赖桌面插件（[App.tsx][app] `importData/exportData`）。 |
| 完整桌面 | [README.md][readme]「本地开发」要求 Node.js 22+、Rust stable、Tauri 2 系统依赖，使用 `npm run tauri dev`。[lib.rs][lib] `run` 初始化应用目录下 `chat-insight.db`、注册命令及文件对话框。开发环境默认无桥是 [README.md][readme] 的说明，实际就绪情况以能力与连接结果为准。 |
| Apple Silicon Mac | [bridge.rs][bridge] `capabilities` 仅 macOS/aarch64 支持直读；需桥 CLI，`connect` 另查 wxkey；`run_wxkey_bootstrap` 执行 bootstrap。欢迎页提前说明完全磁盘访问、可能重开微信及管理员授权（`capabilities.setup_hint`）。[README.md][readme]「开始使用」前提为微信 4.x 已安装并登录。 |
| Intel Mac | macOS/x86_64 为 import_only；不能直接读取微信，但桌面可导入 JSONL 后分析。[bridge.rs][bridge] `capabilities/connect`；[README.md][readme]「平台能力」列 Intel DMG。 |
| Windows | 直读边界为 Windows/x86_64；需桥与已登录微信 4.x，并先打开普通聊天。`run_platform_bootstrap` 调用 `cache refresh --force`，不能套用 Mac 的 wxkey 授权流程。[bridge.rs][bridge] `capabilities/run_platform_bootstrap`；[README.md][readme]「平台能力」列 x64 NSIS。 |
| 缺桥 | 受支持架构仍可能返回 bridge_missing；连接入口要求直读能力与桥同时成立；可走桌面 JSONL。能力只检查 CLI 文件，不证明所有桥依赖可用。[bridge.rs][bridge] `capabilities/connect`；[Onboarding.tsx][onboarding] `canConnect`。 |

### 2. 连接与同步

| 操作 | 已确认路径与结果 | 证据及命令映射 |
| --- | --- | --- |
| 欢迎页连接 | 先授权，成功后同步；授权失败保留原因供重试。只把标准化消息写入应用 SQLite。 | [App.tsx][app] `bootstrapAndSync/runSync`；[api.ts][api] `bootstrap/sync` → [lib.rs][lib] `bootstrap_bridge/start_sync` → [bridge.rs][bridge] `connect/sync_wechat/sync_chat`。 |
| 更新与取消 | 数据管理「立即同步」或右上刷新；按账号/聊天最大 local_id 增量读取、消息去重；`sync-progress` 回传。取消标记由工作循环检查，已写入数据保留。**主界面仅非 preparing 阶段显示取消；无数据首次同步的欢迎页只有进度，没有取消按钮。** | [DataManagement.tsx][data] `canReadWechat`；[App.tsx][app] `runSync/syncProgress`；[api.ts][api] `sync/cancelSync/onSyncProgress` → [lib.rs][lib] `start_sync/cancel_sync` → [bridge.rs][bridge] `sync_chat/sync_wechat/emit_progress`、[database.rs][database] `Database::insert_message/max_local_id`。 |
| 换号与查看旧数据 | 先在微信登录目标号并点开聊天，再在数据管理切换；识别最近写入账号目录，备份旧桥配置，已连接账号恢复配置，新账号重新初始化。顶部数据集下拉只切换查看；同步对象由桥身份决定。 | [App.tsx][app] `switchAccount/accountId/runSync`；[api.ts][api] `switchAccount` → [lib.rs][lib] `switch_wechat_account` → [bridge.rs][bridge] `connect/switch_account/sync_wechat`、[wechat_account.rs][account] `detect_active_account/most_recent_account/AccountStore::save/load`。 |

### 3. JSONL 导入导出

| 操作 | 已确认行为 | 入口、命令与实现 |
| --- | --- | --- |
| 导入 | 欢迎页或数据管理选单个 `.jsonl`；首行 manifest，format=`chat-insight-jsonl`、schema_version=1，后续 chat/message。导入 ID 为 `import:{dataset_id}`；成功刷新并选中数据集。不是任意第三方 JSONL 通道。 | [App.tsx][app] `importData`；[api.ts][api] `importJsonl` → [lib.rs][lib] `import_jsonl` → [portable.rs][portable] `PortableRecord/import_jsonl/FORMAT/SCHEMA_VERSION`。 |
| 校验与重复导入 | 单事务提交；未知版本不写入，坏记录、坏时间、重复 manifest 或无法找到所引用聊天时失败回滚。新库须先有 chat；已有同数据集聊天也可满足引用检查。重复消息忽略，聊天名称等仍可更新。 | [portable.rs][portable] `import_jsonl`；[database.rs][database] `Database::initialize` 的消息主键与 server_id 唯一索引。 |
| 导出与迁移边界 | 导出当前数据集全部聊天及消息，不受顶部日期范围限制。含正文和身份字段，需妥善保管；无密钥、原微信库、媒体文件/媒体路径字段、约定或群决策。新库导入不会带入这些 AI 记录；重复导入也不清除目标数据集已有约定/决策。 | [App.tsx][app] `exportData`；[api.ts][api] `exportJsonl` → [lib.rs][lib] `export_jsonl` → [portable.rs][portable] `export_jsonl/PortableRecord/import_jsonl`。媒体另受 [media.rs][media] `ensure_current_account` 限制，导入数据集不能凭 JSONL 预览原媒体。 |
| 清空 | 二次确认后删除当前应用数据集，并级联删除消息、约定、决策等数据库记录；不删除微信原始文件。不等于擦除全部应用设置或账号配置。 | [App.tsx][app] `clearData`；[api.ts][api] `clearAccount` → [lib.rs][lib] `clear_account` → [database.rs][database] `Database::clear_account/connection/initialize/SCHEMA_V3/SCHEMA_V4`。 |

### 4. 关键词搜索

| 用户路径 | 已确认行为与调用链 |
| --- | --- |
| 搜索原话 | [Ask.tsx][ask] `Ask/runSearch`：AI 关闭且未手选模式时默认关键词；可选聊天并按顶部日期筛选。UI 请求最多 200 条、时间从新到旧、高亮词；[api.ts][api] `searchMessages` → [ai/commands.rs][commands] `search_messages` → [database.rs][database] `Database::search_messages`：取前六个去重空白分词，词间 AND；≥3 字符用 trigram FTS，短词用转义 LIKE，后端上限 500。全程无需模型或语义索引。 |
| 核对上下文 | 点命中打开 [MessageContext.tsx][context] `MessageContext`；[api.ts][api] `messageContext` → [ai/commands.rs][commands] `message_context` → [database.rs][database] `Database::message_context`。关键词命中默认取前后各 8 条，按 local_id 排序；此查询不带日期范围。跳详情只传 chatId，不能宣称精确定位命中消息（[App.tsx][app] `openDetail`、[ChatDetail.tsx][detail] `ChatDetail`）。 |

### 5. AI 开关与云端确认

| 边界 | 已确认行为及证据 |
| --- | --- |
| 默认与设置 | AI、allowCloud、自动扫描均默认 false；默认 `http://127.0.0.1:11434/v1`、`qwen3:8b`、`bge-m3`。[aiDefaults.ts][defaults] `defaultAiSettings` 与 [ai/settings.rs][settings] `AiSettings::default` 一致。数据管理内 [AiSettings.tsx][ai-settings] `save` → [api.ts][api] `saveAiSettings` → [ai/commands.rs][commands] `ai_save_settings` → `SettingsStore::save`；设置与 Key 分存于应用 `ai/`。 |
| 无需 AI | 同步、JSONL、总览图表、聊天时间线、关键词搜索/词云、年度统计和关系洞察均无需模型。[api.ts][api] 对应非 AI 映射；[database.rs][database] `Database::dashboard/chat_detail/search_messages`、[analysis.rs][analysis] `top_keywords`、[insights.rs][insights] `Database::year_insights`。摘要、语义话题、自然语言回答、扫描约定/群决策及年度文字回顾才需要 AI。 |
| 本机/云端 | 仅 localhost、127.0.0.1、::1 被视为本机，局域网地址也按云端处理；非本机须允许云端，发聊天内容还须逐次确认。[ai/settings.rs][settings] `is_local_endpoint/AiSettings::ensure_allowed`；[AiProvider.tsx][provider] `confirmCloud` 本机返回 false、云端确认返回 true、取消返回 null；[CloudConsent.tsx][consent] `CloudConsent` 展示 host/内容，取消、Esc、点遮罩均拒绝。 |
| 双重门禁与标识 | 下表内容请求都带 cloudConsent，后端 `ensure_allowed(true, cloud_consent)` 拒绝未确认请求；读已有缓存不发起模型生成。[ai/commands.rs][commands] 对应命令及 `cache_mode`。侧栏仅在 enabled、非本机且 allowCloud 时显示云端状态（[ai.ts][ai] `usesCloud`、[App.tsx][app] `App`），标识不是网络实测。 |

| 前端动作／发送内容 | api.ts 方法 → Rust 命令／实现 |
| --- | --- |
| [AiSettings.tsx][ai-settings] `buildIndex`：待索引正文分段 | [api.ts][api] `buildAiIndex` → [ai/commands.rs][commands] `ai_build_index` → [ai/indexer.rs][indexer] `build_index`。 |
| [Ask.tsx][ask] `runAsk`：问题及最多 12 段相关聊天 | [api.ts][api] `ask` → [ai/commands.rs][commands] `ai_ask` → [ai/features.rs][features] `ask/ASK_SOURCES`。无向量索引仍可用关键词检索，回答仍需 AI。 |
| [AiSummaryPanel.tsx][summary] `generate`：当前聊天日期范围内文本 | [api.ts][api] `summarizeChat/aiEstimate` → [ai/commands.rs][commands] `ai_summarize_chat/ai_estimate` → [ai/features.rs][features] `summarize_chat`。 |
| [TopicsPanel.tsx][topics] `analyze`：该聊天未索引正文及每话题最多 5 段代表片段；索引不限顶部日期 | [api.ts][api] `chatTopics/aiEstimate` → [ai/commands.rs][commands] `ai_chat_topics/ai_estimate` → [ai/indexer.rs][indexer] `index_chat`、[ai/features.rs][features] `chat_topics`。 |
| [Commitments.tsx][commitments] `startScan`、[ChatMemoryPanel.tsx][memory-panel] `scan`：线索片段和已有未完成/有效条目标题，云端先估算片段/字数 | [api.ts][api] `commitmentEstimate/scanCommitments/decisionEstimate/scanDecisions` → [ai/commands.rs][commands] `ai_commitment_estimate/ai_scan_commitments/ai_decision_estimate/ai_scan_decisions` → [ai/commitments.rs][ai-commitments] `scan_commitments`、[ai/decisions.rs][ai-decisions] `scan_decisions`。 |
| 例外：年度文字回顾只发统计及对象名称，云端默认用代号，可取消匿名化；不弹正文确认 | [Report.tsx][report] `anonymize/generate`；[api.ts][api] `yearReport` → [ai/commands.rs][commands] `ai_year_report` 用 `ensure_allowed(false,false)` → [ai/features.rs][features] `report_payload/year_report/alias_names`，生成后本机还原名称。 |
| 例外：保存并测试连接请求模型列表及固定「连接测试」向量，不发聊天；即使 AI 未开启也可测试 | [AiSettings.tsx][ai-settings] `test`；[api.ts][api] `testAi` → [ai/commands.rs][commands] `ai_test_connection`，仍要求云端 opt-in。不能把 AI 关闭解释为所有网络操作被禁止。 |

自动找约定仅在同步有新增、AI 开启、本机模型及自动扫描开关开启时触发近 30 天扫描；云端保存设置会关闭自动扫描。[App.tsx][app] `runSync`、[ai/settings.rs][settings] `SettingsStore::save`。隐私也不能概括为绝对离线：[README.md][readme]「隐私边界」另述表情 CDN 下载。

## 旧导览核对与改进建议

| 分类 | 结论／建议（均非已实施改动） |
| --- | --- |
| 旧导览仍成立 | [旧导览][history]「启动方式」「页面与体验路径」的浏览器/桌面区分、六页入口、AI 默认关闭与 JSONL 不含约定/决策，已由上表当前符号核对；其测试介绍不是本次测试通过记录。 |
| 需补条件 | “数据就绪自动引导”须加完成标记/忙状态；“每次发送确认”限发送聊天内容的新请求，年度统计回顾与连接测试例外。依据 [App.tsx][app] `autoGuide`、[ai/commands.rs][commands] `ai_year_report/ai_test_connection`。 |
| 建议修正文案与入口 | 欢迎页补首同步取消入口；浏览器欢迎页避免显示“安装不含桥”的泛化标题；数据管理运行模式应同时考虑桥与数据集来源，当前只看 directWechatRead。依据 [Onboarding.tsx][onboarding] `Onboarding`、[App.tsx][app] `syncProgress`、[DataManagement.tsx][data] `DataManagement/canReadWechat`。 |

## 需要实际运行验证

平台安装/依赖、系统授权和微信重启、真实账号识别/换号、同步取消时机与部分失败、文件对话框/事务故障、搜索中文效果、模型联网与确认取消是否零内容发送，均待执行；不能从静态调用链推定通过。逐项操作和留证要求见 [acceptance-checklist.md](experience-root/outputs/r3-native/acceptance-checklist.md)。

[history]: experience-root/materials/history/chat-insight-experience.md
[app]: experience-root/materials/src/App.tsx
[onboarding]: experience-root/materials/src/views/Onboarding.tsx
[api]: experience-root/materials/src/api.ts
[lib]: experience-root/materials/src-tauri/src/lib.rs
[bridge]: experience-root/materials/src-tauri/src/bridge.rs
[database]: experience-root/materials/src-tauri/src/database.rs
[guide]: experience-root/materials/src/guide.ts
[package]: experience-root/materials/package.json
[vite]: experience-root/materials/vite.config.ts
[data]: experience-root/materials/src/views/DataManagement.tsx
[account]: experience-root/materials/src-tauri/src/wechat_account.rs
[portable]: experience-root/materials/src-tauri/src/portable.rs
[media]: experience-root/materials/src-tauri/src/media.rs
[ask]: experience-root/materials/src/views/Ask.tsx
[context]: experience-root/materials/src/components/MessageContext.tsx
[detail]: experience-root/materials/src/views/ChatDetail.tsx
[defaults]: experience-root/materials/src/aiDefaults.ts
[settings]: experience-root/materials/src-tauri/src/ai/settings.rs
[ai-settings]: experience-root/materials/src/components/AiSettings.tsx
[commands]: experience-root/materials/src-tauri/src/ai/commands.rs
[analysis]: experience-root/materials/src-tauri/src/analysis.rs
[insights]: experience-root/materials/src-tauri/src/insights.rs
[provider]: experience-root/materials/src/components/AiProvider.tsx
[consent]: experience-root/materials/src/components/CloudConsent.tsx
[ai]: experience-root/materials/src/ai.ts
[indexer]: experience-root/materials/src-tauri/src/ai/indexer.rs
[features]: experience-root/materials/src-tauri/src/ai/features.rs
[summary]: experience-root/materials/src/components/AiSummaryPanel.tsx
[topics]: experience-root/materials/src/components/TopicsPanel.tsx
[commitments]: experience-root/materials/src/views/Commitments.tsx
[memory-panel]: experience-root/materials/src/components/ChatMemoryPanel.tsx
[report]: experience-root/materials/src/views/Report.tsx

[readme]: experience-root/materials/README.md
[ai-commitments]: experience-root/materials/src-tauri/src/ai/commitments.rs
[ai-decisions]: experience-root/materials/src-tauri/src/ai/decisions.rs
