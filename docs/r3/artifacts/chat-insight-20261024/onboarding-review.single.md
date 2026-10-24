> 导出说明：experience-root 是本机体验目录的占位符；源码引用对应所选输入快照。原始文档及本机可打开的引用路径保持不变。

# Onboarding 静态核对

“已确认”仅指选定输入的静态源码证据。本次未执行测试、验证命令、应用或真实微信读取，未修改输入。下文命令均由 [lib.rs](../../materials/src-tauri/src/lib.rs) `run → generate_handler!` 注册；封装见 [api.ts](../../materials/src/api.ts) `api`，绕行分支另列。

## 已确认行为

### 首次使用

| 用户路径 | 前端入口与处理 | 命令及实现／边界 |
| --- | --- | --- |
| 打开应用 | [App.tsx](../../materials/src/App.tsx) `App` 初始化，调用 `api.capabilities/accounts`；无数据集显示 [Onboarding.tsx](../../materials/src/views/Onboarding.tsx) `Onboarding` | `get_capabilities →` [bridge.rs](../../materials/src-tauri/src/bridge.rs) `capabilities`；`list_accounts →` [database.rs](../../materials/src-tauri/src/database.rs) `Database::list_accounts`。lib.rs `run` 初始化应用目录内的 SQLite。 |
| 先看／重看指引 | `Onboarding.onShowGuide → App.guideMode=preview`；侧栏 `showGuide`；[GettingStarted.tsx](../../materials/src/components/GettingStarted.tsx) `GettingStarted/advance` | [guide.ts](../../materials/src/guide.ts) `stepsFor/guideCompleted/markGuideCompleted`；直接用 DOM/localStorage，无 api.ts／Rust。有数据且未完成指引、空闲时自动 tour；跳过也记完成，preview 不记。 |
| 浏览器／桌面 | [README.md](../../materials/README.md)“本地开发”；[package.json](../../materials/package.json) `scripts.dev/tauri` | `npm run dev` 浏览器：`inTauri=false`，browser/import_only、空账号、默认 AI、事件空订阅，其余 `call` 拒绝。可预览欢迎页／指引，导入按钮的原生 dialog 也需桌面。`npm run tauri dev` 才走桌面命令；启动效果未验证。 |

### 连接与同步

| 用户路径 | 前端 → api.ts 命令 → Rust 实现 |
| --- | --- |
| 欢迎页连接 | `Onboarding.onBootstrap → App.bootstrapAndSync → api.bootstrap → bootstrap_bridge → lib.rs::connect_bridge → bridge.rs::connect/run_platform_bootstrap`；成功后 `runSync`。 |
| 同步／取消 | 顶栏刷新、[DataManagement.tsx](../../materials/src/views/DataManagement.tsx) `onSync → App.runSync → api.sync → start_sync → bridge.rs::sync_wechat/sync_chat`；`api.onSyncProgress` 订阅 `sync-progress`。主界面进度非 preparing 时可 `api.cancelSync → cancel_sync` 设置原子标记；按聊天／页检查取消，已写数据保留；欢迎页进度没有取消按钮。 |
| 切换账号／数据集 | `DataManagement.onSwitchAccount → App.switchAccount → api.switchAccount → switch_wechat_account → connect_bridge(require_switch=true) → bridge.rs::connect/switch_account`；[wechat_account.rs](../../materials/src-tauri/src/wechat_account.rs) `detect_active_account/AccountStore::save/load` 按最近写入目录识别、备份恢复配置。同号不再同步，新号成功后同步；顶栏数据集选择仅改 `accountId`。 |

| 桌面环境 | 直接连接、同步 | JSONL |
| --- | --- | --- |
| Apple Silicon Mac（aarch64） | 需数据桥；连接还检查 wxkey，执行 `run_wxkey_bootstrap → wxkey bootstrap`。提示完全磁盘访问、可能重启微信和管理员授权。 | 可导入导出，不依赖桥。 |
| Intel Mac（x86_64） | `capabilities.direct_wechat_read=false`，欢迎页隐藏连接，数据管理禁用同步、不显示换号。不能据此说整个应用不支持。 | 支持导入分析和导出。 |
| Windows x64 | 需 `wechat-cli.exe`；`run_platform_bootstrap` 执行 `cache refresh --force`，提示微信 4.x 登录并打开普通聊天；同步走公共实现。 | 可导入导出。 |
| 支持架构但缺桥 | `bridge_missing`；连接隐藏、同步禁用。Apple Silicon 的能力探测只查 cli，wxkey 缺失到连接时才报错。 | 仍可用。 |

依据：README.md“平台能力”、bridge.rs `capabilities/BridgePaths::resolve/connect`、Onboarding `canConnect`、DataManagement `canReadWechat`、portable.rs（下节）。`start_sync → sync_wechat` 只查 cli，未复核架构；UI 禁用不等于命令均有平台拒绝。开发缺桥为 README 声明，实际包内容待验证。

### JSONL 导入导出

| 路径 | 实际链路与实现依据 |
| --- | --- |
| 欢迎页／数据管理导入 | `onImport → App.importData → @tauri-apps/plugin-dialog::open`（不经 api.ts；lib.rs `run` 安装 dialog 插件）→ `api.importJsonl → import_jsonl →` [portable.rs](../../materials/src-tauri/src/portable.rs) `import_jsonl`。取消选文件不调用导入；成功刷新账号和页面。 |
| 数据管理导出 | `onExport → App.exportData → plugin-dialog::save`（同上）→ `api.exportJsonl → export_jsonl → portable.rs::export_jsonl`；导出当前数据集全部消息，不受顶部日期范围限制。 |
| 格式与失败边界 | `PortableRecord` 仅 Manifest/Chat/Message，`FORMAT/SCHEMA_VERSION=chat-insight-jsonl/v1`；导入集为 `import:{dataset_id}`。事务内校验；未知版本、坏记录、无效时间或新集未声明聊天会失败回滚；消息 `INSERT OR IGNORE` 去重。聊天存在性查数据库，可引用已有聊天。 |
| 携带范围 | **JSONL 不包含约定和群决策**：枚举无对应变体，仅查询 accounts/chats/messages，导入不写约定／决策表。新集不迁入这些项，重导也不清除已有项。无密钥、原库、媒体文件／专用媒体路径字段；`text` 原样输出，不代表正文脱敏。 |

### 关键词搜索

| 操作 | 实际链路与行为 |
| --- | --- |
| 搜索与问答 → 关键词搜索 | [Ask.tsx](../../materials/src/views/Ask.tsx) `submit/runSearch → api.searchMessages(limit=200) → commands.rs::search_messages → Database::search_messages`。AI 关闭且无会话选择时默认搜索；账户、聊天与日期过滤，最多前六个不同空白分隔词全部匹配；长词用 trigram FTS5，短词用 LIKE；新到旧，UI 最多取 200 条。 |
| 打开结果前后文 | `Ask.context →` [MessageContext.tsx](../../materials/src/components/MessageContext.tsx) `MessageContext → api.messageContext → commands.rs::message_context → Database::message_context`；无 AI 门禁，不传日期，按 local_id 邻接取上下文，可能超出搜索日期。转聊天详情经 `App.openDetail`，不保证滚到原消息。 |

本节 `commands.rs` 专指 [ai/commands.rs](../../materials/src-tauri/src/ai/commands.rs)，虽位于 AI 模块，以上两命令仅查本机数据库。

### AI 开关与云端确认

| 分支 | 入口、映射及边界 |
| --- | --- |
| AI 默认关闭 | [aiDefaults.ts](../../materials/src/aiDefaults.ts) `defaultAiSettings` 与 [settings.rs](../../materials/src-tauri/src/ai/settings.rs) `AiSettings::default` 一致：enabled/allowCloud/autoScanCommitments 均 false，默认 127.0.0.1:11434/v1、qwen3:8b、bge-m3。既有配置由 `SettingsStore::load` 恢复，并非每次重置。 |
| 保存／加载 | 数据管理 → [AiSettings.tsx](../../materials/src/components/AiSettings.tsx) `save/removeKey → api.saveAiSettings → ai_save_settings → SettingsStore::save`；[AiProvider.tsx](../../materials/src/components/AiProvider.tsx) 初始化 `api.aiSettings → ai_get_settings → SettingsStore::view`。设置与独立 Key 位于应用目录 ai/。 |
| 无需 AI 的功能 | 同步、JSONL、关键词搜索及上下文不需模型；[Overview.tsx](../../materials/src/views/Overview.tsx) `Overview → api.dashboard → query_dashboard → Database::dashboard`；[ChatDetail.tsx](../../materials/src/views/ChatDetail.tsx) `ChatDetail/loadMore → api.chatDetail → query_chat_detail → Database::chat_detail →` [analysis.rs](../../materials/src-tauri/src/analysis.rs) `top_keywords`；[Report.tsx](../../materials/src/views/Report.tsx) `Report → api.yearInsights → query_year_insights →` [insights.rs](../../materials/src-tauri/src/insights.rs) `Database::year_insights`。图表、时间线、词频及年度关系统计均本地计算。 |
| 已存约定／决策 | [Commitments.tsx](../../materials/src/views/Commitments.tsx) `update`、[ChatMemoryPanel.tsx](../../materials/src/components/ChatMemoryPanel.tsx) `updateDecision` 及加载 → `api.commitments/updateCommitment/decisions/updateDecision → list_commitments/update_commitment/list_decisions/update_decision →` [本地 commitments.rs](../../materials/src-tauri/src/commitments.rs)、[本地 decisions.rs](../../materials/src-tauri/src/decisions.rs) 同名 `Database` 方法。关闭 AI 仍可管理，扫描需 AI。 |
| 确认门禁 | settings.rs `is_local_endpoint` 仅认 localhost/127.0.0.1/::1，其余含局域网均算云端。`ensure_allowed` 要 enabled，云端再要 allowCloud，正文再要 cloudConsent=true。AiProvider `confirmCloud` 本机返回 false，远端弹 [CloudConsent.tsx](../../materials/src/components/CloudConsent.tsx) `CloudConsent` 显示 host／范围；确认 true，取消／Esc／遮罩返回 null，中止调用。弹窗无 api.ts／Rust。 |

下表每行 api 方法映射到所列命令，由 lib.rs 注册；正文类命令调用 `ensure_allowed(true, cloud_consent)`。一次操作确认可覆盖多次模型 HTTP 请求，并非每个分段都弹窗。

| 内容发送路径 | 前端处理 → api 方法／命令 → Rust 实现；覆盖范围 |
| --- | --- |
| 语义索引 | `AiSettings.buildIndex → buildAiIndex/ai_build_index →` [indexer.rs](../../materials/src-tauri/src/ai/indexer.rs) `build_index/index_chat`；当前数据集未索引文本，重建则全量，约 12 条／600 字分段，另含聊天名、日期。 |
| 问答 | `Ask.runAsk → ask/ai_ask →` [features.rs](../../materials/src-tauri/src/ai/features.rs) `ask/ask_prompt` 与 [retrieval.rs](../../materials/src-tauri/src/ai/retrieval.rs) `retrieve`；问题及最多 12 段，含聊天名／时间／发言人。有索引时还把问题发给向量模型；无索引仍能关键词检索。 |
| 摘要 | [AiSummaryPanel.tsx](../../materials/src/components/AiSummaryPanel.tsx) `generate → summarizeChat/ai_summarize_chat → features.rs::summarize_chat`；当前聊天／日期文本，长文分段；`aiEstimate/ai_estimate →` [store.rs](../../materials/src-tauri/src/ai/store.rs) `Database::ai_estimate` 本地估算。 |
| 话题 | [TopicsPanel.tsx](../../materials/src/components/TopicsPanel.tsx) `analyze → chatTopics/ai_chat_topics → indexer::index_chat + features::chat_topics`；先索引该聊天**全部日期**的待处理消息，再按所选日期聚类，发送每话题最多 5 段代表片段。估算使用空日期范围。 |
| 约定／群决策 | `Commitments.startScan/ChatMemoryPanel.scan → commitmentEstimate/decisionEstimate → ai_commitment_estimate/ai_decision_estimate` 本地规划；确认后 `scanCommitments/scanDecisions → ai_scan_commitments/ai_scan_decisions →` [ai/commitments.rs](../../materials/src-tauri/src/ai/commitments.rs) `scan_commitments`／[ai/decisions.rs](../../materials/src-tauri/src/ai/decisions.rs) `scan_decisions`；[extraction.rs](../../materials/src-tauri/src/ai/extraction.rs) `build_windows/scan/OPEN_ITEMS`：当前范围有线索待扫窗口、前最多 6 条上下文、最多 8 条有效项标题等。约定群窗口需我发言，决策无需。 |
| 自动扫描 | `App.runSync` 仅新增消息且 AI 开启、本机、autoScanCommitments 时调用 `api.scanCommitments` 扫近 30 天，无弹窗；云端设置保存时强制取消自动扫描。群决策无此自动入口。 |
| 确认例外 | `Report.generate → api.yearReport → ai_year_report → features::year_report/report_payload`：`ensure_allowed(false,false)`，仅统计及名称，云端默认代号、可关闭匿名。`AiSettings.test → api.testAi → ai_test_connection` 仅查云端 allowCloud，enabled=false 也能发送固定“连接测试”并获取模型列表。摘要／话题／年报 cached 只读本机。 |

## 旧导览与 README 对照

[chat-insight-experience.md](../../materials/history/chat-insight-experience.md) 只作线索；本范围未发现可确证为旧版本行为的过时项，不强行归类。

| 原说法 | 判定／当前依据 |
| --- | --- |
| 浏览器空数据、桌面导入；引导、默认关闭 AI、本地搜索；JSONL 无约定决策 | 一致：api.ts `inTauri`、App、guide.ts、settings.rs `Default`、portable.rs `PortableRecord`。 |
| api.ts 为前端统一入口 | 概括需收窄：App 原生 open/save、指引 localStorage 均绕过它；不能将此概括当作完整调用链。 |
| Tauri dev 自动启动 Vite，构建载入 dist | 缺乏选定配置证据：package.json 仅定义 `tauri`，未选入 tauri.conf.json；不据历史文字确认。 |
| README“同步过程中可以取消” | UI 表述过宽：App 仅主界面非 preparing 阶段有取消；Onboarding 无按钮，后端取消也非即时中断。 |
| README“已读消息不再发给模型” | 需限定：extraction.rs `build_windows` 的前 6 条上下文可能重带已扫描消息；不重复的是主扫描窗口。 |
| README 年报发送“统计数字和聊天对象名称” | 一致：features.rs `report_payload/year_report` 包含统计及名称；启用匿名时以代号替换，不发送聊天正文。 |
| README 云端逐次确认 | 适用范围是发送聊天正文的操作；年报与连接测试不经正文确认。对应 commands.rs `ai_year_report/ai_test_connection`；不能扩成“所有网络请求都确认”。 |
| Ask 弹窗“每段约 600 字” | 与实现有偏差：retrieval.rs `retrieve` 的无索引回退取前后各 3 条，每条截 300 字，没有整段 600 字上限。 |

## 改进建议

| 建议 | 对应证据 |
| --- | --- |
| 浏览器导入按钮提示桌面要求；补欢迎页取消入口，统一同步架构检查 | App `importData`、Onboarding、bridge.rs `sync_wechat`。 |
| 明示话题全日期索引、确认按操作批次、年报名称和测试例外；修订约 600 字提示与已读不再发送说法 | TopicsPanel `analyze`、commands.rs、retrieval.rs `retrieve`、extraction.rs `build_windows`。 |

## 需实际运行验证

| 路径 | 未验证 |
| --- | --- |
| 首次使用／连接同步 | 引导与错误呈现；三平台包、权限、微信兼容、换号、取消与保留数据。 |
| JSONL／搜索 | 合成样例往返、回滚去重；中文检索、日期／上下文与性能。 |
| AI | 模型、取消零发送、载荷、缓存、批次、匿名还原及质量。待执行步骤见 [acceptance-checklist.md](acceptance-checklist.md)，均非通过记录。 |
