# Chat Insight 首次使用与数据路径审阅

本报告只依据本次选定输入的静态源码和 README。下文“已确认行为”仅指可以从源码分支、参数和 SQL 推导出的行为，不代表已经在浏览器、桌面应用、模型服务或微信上执行成功。未安装工具、未执行测试或验证命令、未启动应用、未读取真实微信数据，输入源码未作修改。配套的 [acceptance-checklist.md](acceptance-checklist.md) 是待执行清单。

当前行为以源码为准；[README 的“开始使用／平台能力／隐私边界／本地开发／便携数据格式”][README] 是对照材料，[历史导览][HISTORY] 只提供定位线索。所有源码引用均指向本次选定输入的绝对路径；链接旁标出具体符号。

## 已确认行为

### 1. 首次使用

#### 入口、运行时与导览

| 用户动作或状态 | 真实前端路径 | api.ts、Rust 注册和具体实现 |
| --- | --- | --- |
| 打开应用，读取平台与数据集 | [main.tsx 的 createRoot][MAIN] 挂载 `AiProvider → App`；[App 初始化 useEffect][APP] 并行请求平台和账号，没有 `activeAccount` 时渲染 `Onboarding` | [api.capabilities / api.accounts、inTauri、call][API] → 桌面命令 `get_capabilities / list_accounts`；均在 [lib.rs::run 的 generate_handler!][LIB] 注册 → `bridge::capabilities` / `Database::list_accounts`。桌面 [main.rs::main][RMAIN] 调用 `chat_insight_lib::run`；`run::setup` 初始化应用数据目录内 `chat-insight.db`、`wechat-accounts/`、`ai/`。 |
| 无数据时选择入口 | [Onboarding::Onboarding、canConnect][ONBOARD]：同时满足 `directWechatRead && bridgeBundled` 才显示“连接本机微信”；导入和“先看使用指引”仍显示 | 连接、导入回调由 [App 的 bootstrapAndSync / importData][APP] 提供，分别见后文；组件本身不直接 invoke。浏览器导入按钮也存在，不表示浏览器支持导入。 |
| “先看使用指引” | [App 的 onShowGuide][APP] 设置 `guideMode="preview"`；[GettingStarted、advance、readRect][GETTING] 使用 [guide.ts::previewSteps / stepsFor][GUIDE] | 此按钮、翻页、高亮定位本身不经过 `api.ts`，没有对应 Rust 命令；属于 React 状态、DOM 和浏览器存储操作。预览结束不记录 tour 完成。 |
| 数据集出现后自动 tour；跳过后重新打开 | [App 的自动导览 useEffect / closeGuide / showGuide][APP]；[GettingStarted][GETTING] 按 [tourSteps][GUIDE] 切换 `view` 并定位 `data-guide` | 自动 tour 条件是存在 `activeAccount`、当前启动未自动展示过、`guideCompleted()` 为假、没有其他导览，且 `busy` 和同步均不在运行。不是按消息条数判断“数据就绪”。`closeGuide` 只对 tour 调用 [markGuideCompleted][GUIDE] 写 `localStorage["chat-insight.guide.completed"]="1"` 并恢复进入前页面；侧栏可随时重开。导览操作本身无 Rust 命令，但 tour 切换页面会间接触发该页正常数据查询。 |

[README“本地开发”][README] 区分 `npm run dev` 的浏览器界面预览与 `npm run tauri dev` 的完整桌面应用；[package.json::scripts.dev / scripts.tauri][PACKAGE] 分别是 `vite / tauri`，[vite.config.ts::server][VITE] 固定端口 1420。这些是脚本定义，不是本次执行记录。

浏览器中，[api.ts::browserCapabilities][API] 返回 `os=browser`、`arch=unknown`、`directWechatRead=false`、`bridgeBundled=false`、`setupState=import_only`；`accounts()` 返回空数组、`aiSettings()` 返回默认设置，事件订阅返回空清理函数。因此通常只能看到欢迎页和 preview 指引，没有演示聊天数据或完整主界面数据路径。其余 `call()` 在无 `__TAURI_INTERNALS__` 时拒绝操作。另一个更早的边界是 [App::importData][APP] 先直接调用 `@tauri-apps/plugin-dialog::open`，不经过 `api.ts`，浏览器可能先在该插件调用处失败并被 `catch` 显示错误，不能保证一定出现 `call()` 的桌面限定文案。文件对话框在 Rust 侧由 [lib.rs::run 的 tauri_plugin_dialog::init][LIB] 接入，没有本项目自定义的“打开对话框”命令。

### 2. 连接与同步

#### 平台边界

以下区分桌面应用、直接连接能力、实际桥文件和 JSONL，不把直连限制扩大为整个应用限制。[README“平台能力／本地开发”][README] 与 [bridge.rs::capabilities / BridgePaths::resolve][BRIDGE] 给出边界：

| 环境 | 直接连接与同步入口 | JSONL 路径及限制 |
| --- | --- | --- |
| macOS Apple Silicon，桌面 `aarch64` | `directWechatRead=true`；`bridgeBundled` 还要求桥目录的 `wechat-cli` 是文件。连接时 `bridge::connect` 另外检查 `wxkey`；只有 CLI 存在而 wxkey 缺失时，按钮仍可能显示，点击后报错。初始化走 `run_platform_bootstrap → run_wxkey_bootstrap`，启动 `wxkey bootstrap`；完全磁盘访问、可能重开微信及管理员授权已在欢迎页提示 | 导入导出没有该架构条件，也不要求桥存在；本机微信兼容性与权限成败未验证。 |
| macOS Intel，桌面 `x86_64` | `directWechatRead=false`、`setupState=import_only`，正常 UI 不提供连接／切换和顶部同步，数据管理同步禁用；`bridge::connect` 明确拒绝 | 支持桌面 JSONL 导入分析和导出，仍可查询统计、文本时间线和搜索；不能据此说应用不支持 Intel Mac。 |
| Windows x64，桌面 `x86_64` | `directWechatRead=true`；要求 `wechat-cli.exe`。初始化走 `run_platform_bootstrap → run_status(["cache","refresh","--force"])`，提示微信 4.x 已登录且打开过普通聊天；没有 macOS 的 wxkey 授权分支 | 支持 JSONL；不能把 Apple Silicon 初始化要求套到 Windows，也不由源码推出所有 Windows 架构都支持直连。 |
| 支持架构但缺少桥的桌面开发环境／安装 | 能力标记可为 `directWechatRead=true`，但 `bridgeBundled=false`、`setupState=bridge_missing`；[Onboarding::canConnect][ONBOARD]、[DataManagement::canReadWechat][DATA] 和 [App 顶栏按钮][APP] 都以两者同时成立为准 | JSONL 命令和本地数据库查询仍可用。[README][README] 说明开发环境默认不含数据桥；本次未准备数据桥。 |
| 普通浏览器 | 不提供桌面直连／同步，返回预览能力；无 Rust 运行时 | 导入入口可见，但插件和 invoke 没有浏览器实现，不能实际导入或分析本地数据。见 [api.ts::call / browserCapabilities][API]、[App::importData][APP]。 |

**后端保护范围的区别：**[bridge.rs::connect][BRIDGE] 检查 `directWechatRead`；`sync_wechat` 自身只先检查 CLI 文件、再调用桥查询状态，没有重复相同的 OS/arch 拒绝条件。[lib.rs::start_sync][LIB] 也不补该平台校验。因此能确认的是正常 UI 的平台支持边界，不能声称“所有非支持平台的同步命令都会因平台判断被拒绝”，也不能把自备非标准桥可能运行的情形提升为官方支持。

#### 从按钮到落库

下表命令均由 [lib.rs::run 的 generate_handler!][LIB] 注册，事件不是 invoke 命令。

| 用户路径 | 前端 → api.ts 命令映射 → Rust 实现 | 可由源码确认的结果 |
| --- | --- | --- |
| 欢迎页连接 | [Onboarding::onBootstrap][ONBOARD] → [App::bootstrapAndSync][APP] → [api.bootstrap][API] → `bootstrap_bridge` → [lib.rs::connect_bridge(require_switch=false)][LIB] → [bridge::connect / run_platform_bootstrap][BRIDGE] | 授权成功后重读 capabilities，再调用 `runSync`；失败设置 `setupError`，不进入同步。连接与同步共用 `sync_running` 互斥锁。 |
| 数据管理“立即同步”或顶部“同步新消息” | [DataManagement::onSync][DATA] / [App 顶栏 onClick][APP] → `App::runSync` → [api.sync][API] → `start_sync` → [lib.rs::start_sync][LIB] → [bridge::sync_wechat / read_all_sessions / sync_chat][BRIDGE] | 从桥 `status --include-debug` 确定账号，分页读取 `sessions` 和 `timeline`，用 [Database::upsert_account / upsert_chat / insert_message / save_cursor][DB] 写应用 SQLite；使用已有最大 local_id 停止向旧消息继续翻页，插入去重。不是 watcher。 |
| 观察进度与取消 | [App 初始化订阅、sync-strip 取消按钮][APP] → [api.onSyncProgress / cancelSync][API] → 监听 `sync-progress` / invoke `cancel_sync`；[lib.rs::cancel_sync][LIB] 设置原子标记，[bridge::emit_progress / sync_wechat / read_all_sessions / sync_chat][BRIDGE] 发事件并在循环检查 | 顶部条仅在同步 running 且 phase 非 `preparing` 时给取消按钮；无数据欢迎页 [Onboarding][ONBOARD] 只展示进度，没有取消控件。已写入数据没有整次回滚。取消是协作式，不能承诺立即中断正在执行的桥子进程。 |
| 切换微信账号 | [DataManagement::onSwitchAccount][DATA] → [App::switchAccount][APP] → [api.switchAccount][API] → `switch_wechat_account` → [lib.rs::connect_bridge(require_switch=true)][LIB] → [bridge::connect / switch_account][BRIDGE] | 先在微信登录目标号并打开聊天。检测同账号则 UI 提示不变且不继续同步；成功换号后 `runSync`。配置通过 [wechat_account.rs::AccountStore、detect_active_account / most_recent_account][WECHAT] 备份恢复；按最近数据库写入时间推断活跃账号，不是微信登录状态 API。 |
| 在右上角选择已保存数据集 | [App 的 account-select::onChange][APP] 更新 `accountId` | 此动作不经过 api.ts，也不调用桥切换微信。各页面随后以新的 `accountId` 查询自己的数据；同步目标仍由 `sync_wechat` 的当前桥身份决定，并不接收当前下拉框的 accountId。 |

同步后的 `refreshAccounts(result.accountId)` 与 `revision++` 刷新页面。只有新增消息数大于零，且 `enabled && isLocal && autoScanCommitments` 均成立时，[App::runSync][APP] 才调用 [api.scanCommitments][API] 扫描近 30 天，`cloudConsent=false`；不会自动扫描群决策、建立语义索引或处理 JSONL 导入。后端 [SettingsStore::save][SETTINGS] 还会把云端配置的 `auto_scan_commitments` 强制关闭。

**README 与实现差异：**[README“开始使用”][README] 泛称“同步可以取消”，[Onboarding][ONBOARD] 也如此介绍，但首次无数据的欢迎页没有取消按钮；主界面才有上述控件。此外 [bridge::sync_wechat][BRIDGE] 主要在遍历下一聊天前返回 `cancelled=true`，而 `read_all_sessions / sync_chat` 可直接因取消 `break`。若取消发生在空列表或最后聊天等时点，最后仍可能落到 `cancelled=false` 的完成返回；这是静态可见的边缘分支，实际 UI 表现待验证。

### 3. JSONL 导入导出

| 用户动作 | 实际调用链和注册位置 | 已确认边界 |
| --- | --- | --- |
| 欢迎页“导入 Chat Insight 数据”或数据管理“选择文件” | [Onboarding::onImport][ONBOARD] / [DataManagement::onImport][DATA] → [App::importData][APP] → **直接调用插件** `open({multiple:false, filters:[jsonl]})` → [api.importJsonl(path)][API] → `import_jsonl`，由 [lib.rs::run][LIB] 注册、`lib.rs::import_jsonl` 在 blocking 工作中调用 [portable::import_jsonl][PORTABLE] | 取消选文件不 invoke；成功导入后选择返回的 accountId、刷新 revision。无需 AI、桥或直连能力。浏览器缺插件运行时，不能据按钮存在推断可导入。 |
| 数据管理“导出 JSONL” | [DataManagement::onExport][DATA] → [App::exportData][APP] → **直接调用插件** `save`（默认 `<数据集显示名>.chat-insight.jsonl`）→ [api.exportJsonl(accountId,path)][API] → `export_jsonl`；[lib.rs::run / export_jsonl][LIB] → [portable::export_jsonl][PORTABLE] | 导出当前数据集全部 chats/messages，不传顶部 range；不是当前页面或当前日期范围导出。取消保存不 invoke。可导出导入数据集。 |
| 清空当前数据集 | [App::clearData][APP] 先直接 `window.confirm`；确认后 [api.clearAccount][API] → `clear_account`；[lib.rs::run / clear_account][LIB] → [Database::clear_account][DB] | 浏览器确认本身无 api/Rust；真正删除是应用 `accounts` 行，外键级联删除聊天、消息、约定、群决策等。`Database::connection` 开启外键；不删除微信文件。 |

[portable.rs::PortableRecord、FORMAT、SCHEMA_VERSION、import_jsonl、export_jsonl][PORTABLE] 是便携格式的实现依据：

- 首行必须是 `manifest`，格式为 `chat-insight-jsonl`、schema 为 1；其余类型只有 `chat`、`message`。校验空文件、错误版本、损坏记录、重复 manifest、空聊天 ID、RFC3339 时间与消息引用的聊天是否已在目标事务数据库存在；空白后续行会跳过。账号 ID 写为 `import:<dataset_id>`，source_platform 为 `jsonl-import`。
- 数据写入在单个 SQLite transaction 内，全部通过才 `commit`；错误返回会使未提交事务回滚，manifest 格式错误则在建事务前退出。`INSERT OR IGNORE` 和 [Database::initialize 的 messages 主键／server_id 唯一索引][DB] 支持重复导入去重；聊天 upsert 仍可更新名称、类型，不能把“去重”等同于“文件完全不产生任何更新”。
- 导出只查询 accounts 显示名、chats 和 messages 的声明字段；没有密钥、数据库、媒体二进制、专门的绝对媒体路径字段，也没有 AI 结果字段。消息 `text` 原样序列化，不能把没有媒体路径字段解释成会净化正文内所有路径或个人信息。
- **JSONL 不包含约定和群决策。**枚举没有相应记录类型，导出没有读取 commitments/decisions 表，导入没有写入这些表。约定与群决策实际上由 [database.rs::SCHEMA_V3 / SCHEMA_V4][DB] 建表，由 [commitments.rs::Database::insert_commitment][COMMITMENTDB] 和 [decisions.rs::Database::insert_decision][DECISIONDB] 保存。新目标数据集不会从文件恢复这些记录；已有同名导入数据集重导时，`portable::import_jsonl` 没有删除已有约定／决策，导入后也可主动开启 AI 重新扫描生成。因此 [README“导入的数据集也没有约定”][README] 只适合解释新建导入结果，不能当作永久禁用或每次重导都会清空。

[README“便携数据格式”][README] 关于未知 schema／损坏记录回滚、消息去重与以上实现一致。“缺少聊天声明”应精确理解为目标数据库不存在该聊天：同一 dataset_id 若此前已有聊天行，消息可能无需在本文件重新声明也被接受（`import_jsonl` 的 `SELECT EXISTS` 不是只查本文件声明集合）。

平台的“导入分析支持”也不等于全部媒体可以离线搬走：[portable::PortableRecord][PORTABLE] 没有媒体内容，而 [media.rs::MediaResolver::resolve / ensure_current_account][MEDIA] 需当前桥账号匹配并解析本机媒体；导入账号通常不匹配，不能承诺其图片、视频、表情预览。该限制不妨碍文本查询和图表。此处仅确认代码路径，未实际读取媒体。

### 4. 关键词搜索

| 路径 | 前端入口、api 映射、Rust 注册与实现 | 已确认行为 |
| --- | --- | --- |
| 搜索消息正文 | [App 的 ask 导航][APP] → [Ask::submit → runSearch][ASK] → [api.searchMessages(accountId,text,range,chatId,200)][API] → `search_messages`，在 [lib.rs::run][LIB] 注册 → [ai/commands.rs::search_messages][COMMANDS] → [Database::search_messages][DB] | 未选择过模式时，AI 关闭默认关键词搜索；已选模式保留在前端 session Map，因此不承诺关闭 AI 后覆盖用户原有选择。可限全部／单个聊天和顶部日期范围，输入会 trim；空输入不提交。命令虽然在 ai 目录，实际无 `AiState`、模型客户端或 AI 开关校验。 |
| 匹配、排序与空结果 | [Ask::runSearch / highlight][ASK]；[Database::search_messages、SCHEMA_V2、escape_like][DB] | 空白分词，最多取前 6 个不同词，所有被采用的词都需匹配；至少 3 字符用 trigram FTS，更短用转义 LIKE，参数绑定；按 occurred_at、local_id 降序。UI 请求 200 条，后端 limit clamp 为 1–500；UI 显示“200+”不代表精确总数或有继续翻页。无结果显示调整关键词／日期提示。 |
| 点结果看前后文，再打开聊天 | [Ask 结果 onClick][ASK] → [MessageContext 的 useEffect][CONTEXT] → [api.messageContext][API] → `message_context`，由 [lib.rs::run][LIB] 注册 → [ai/commands.rs::message_context][COMMANDS] → [Database::message_context][DB]；再点“在聊天详情中打开” → `App::openDetail` → [ChatDetail 的 api.chatDetail][DETAIL] → `query_chat_detail` → [lib.rs::query_chat_detail][LIB] → `Database::chat_detail` | 普通命中请求半径 8，后端 clamp 到 0–50、按 local_id 排序；上下文命令没有日期参数，因此可越过搜索的日期边界。打开详情传 chatId，没有 localId，不保证把时间线滚到命中消息。 |

不要把聊天详情左侧“搜索好友或群聊”混同消息全文搜索：[ChatDetail 的 search useEffect][DETAIL] 延时 180ms → [api.chats][API] → `list_chats`（[lib.rs::run / list_chats][LIB]）→ [Database::list_chats][DB]，匹配 display_name。全文搜索无需语义索引；其 FTS 插入／更新／删除触发器由 `SCHEMA_V2` 建立，与可选的向量索引不同。

### 5. AI 开关与云端确认

#### 默认值、开启与不需要 AI 的功能

[aiDefaults.ts::defaultAiSettings][DEFAULTS] 和 [ai/settings.rs::AiSettings::default][SETTINGS] 都是 `enabled=false`、`allowCloud=false`、`autoScanCommitments=false`，默认服务 `http://127.0.0.1:11434/v1`、对话 `qwen3:8b`、向量 `bge-m3`。这是**新设置或读取失败时的默认值**，不是每次启动强行关闭：桌面 [AiProvider 初始化 useEffect][PROVIDER] → [api.aiSettings][API] → `ai_get_settings`（[lib.rs::run][LIB] 注册）→ [ai/commands.rs::ai_get_settings][COMMANDS] → `SettingsStore::view/load` 读取已保存设置。

在 [DataManagement::AiSettings][DATA] 中，[AiSettings::save][AISET] 调用 [api.saveAiSettings(input)][API] → `ai_save_settings`（[lib.rs::run][LIB] 注册）→ [ai/commands.rs::ai_save_settings][COMMANDS] → [SettingsStore::save][SETTINGS]。勾选开关只更新表单，保存后才更新全局设置。存储路径为 app_data/ai，Key 单独存储；未传 Key 保留，空字符串删除。这里只确认保存逻辑，未验证各系统实际文件权限。

无需 AI 的功能有明确非模型调用链：

| 功能 | 源码证据与实际路径 |
| --- | --- |
| 连接、同步、JSONL、清空数据集 | 上述 [api.ts][API] → [lib.rs 命令][LIB] → [bridge][BRIDGE]／[portable][PORTABLE]／[Database][DB]，这些命令不检查 AI 设置。 |
| 总览图表、日期筛选、聊天时间线 | [Overview 的 useEffect][OVERVIEW] → `api.dashboard → query_dashboard → Database::dashboard`；[ChatDetail / loadMore][DETAIL] → `api.chatDetail → query_chat_detail → Database::chat_detail`，命令在 [lib.rs::run][LIB] 注册。 |
| 常聊关键词 | [TopicsPanel 的 KeywordCloud][TOPICS] 在 AI 关闭时显示传入 keywords；来自 [Database::chat_detail][DB] → [analysis.rs::top_keywords][ANALYSIS]，用本机分词／词频，不请求模型。 |
| 消息关键词搜索、原话上下文 | [Ask::runSearch][ASK] → [api.searchMessages / messageContext][API] → [ai/commands.rs::search_messages / message_context][COMMANDS] → [Database][DB]。 |
| 年度统计及关系变化 | [Report 的统计 useEffect][REPORT] → [api.yearInsights][API] → `query_year_insights`（[lib.rs::run][LIB] 注册）→ [ai/commands.rs::query_year_insights][COMMANDS] → [insights.rs::Database::year_insights][INSIGHTS]；只有年度文字回顾另走模型。 |
| 已保存约定／群决策的查看、用户更新 | [Commitments::reload / update][COMMITMENTS]、[ChatMemoryPanel::reload / updateDecision][MEMORY] → [api.commitments / updateCommitment / decisions / updateDecision][API] → [lib.rs 同名 list/update 命令与 run 注册][LIB] → 对应 [Database 约定方法][COMMITMENTDB]／[群决策方法][DECISIONDB]；AI 关闭时仍能管理已有记录，扫描按钮受 enabled 控制。 |

#### 云端内容发送确认：触发条件与覆盖范围

[SettingsStore::view、is_local_endpoint、endpoint_host][SETTINGS] 仅把 HTTP(S) 服务的 `localhost`、`127.0.0.1`、`::1` 视为本机，局域网 IP 也按云端处理。[AiProvider::confirmCloud][PROVIDER] 只按保存设置的 `isLocal` 分支：本机立即返回 `false`；非本机弹 [CloudConsent][CONSENT]，确认返回 `true`，取消返回 `null`，调用者收到 null 就不调用生成命令。弹窗展示主机和操作提供的内容说明；默认焦点在取消，Escape、遮罩和取消按钮均拒绝。弹窗本身无 api/Rust 调用。

这是两层门槛：云端 `allowCloud` 是服务许可；每次包含聊天正文的操作还需 `cloudConsent=true`。[AiSettings::save][AISET] 可以保存 enabled 的云端地址但不勾 allowCloud，不能把“设置可保存”误当作“请求可执行”。[AiSettings::ensure_allowed(sends_content,cloud_consent)][SETTINGS] 顺序检查 enabled、本机／云端、allow_cloud，以及聊天正文标记。前端 `confirmCloud` 本身不检查 enabled/allowCloud，后端生成门禁负责拒绝不满足条件的调用；本机生成同样要求 enabled。

下面所有 `ai_*` 命令由 [lib.rs::run 的 generate_handler!][LIB] 注册，实现入口在 [ai/commands.rs][COMMANDS]；模型 HTTP 实现在 [ai/client.rs::OpenAiCompatClient::complete / embed][CLIENT]，分别请求 `/chat/completions`、`/embeddings`。

| 内容操作与前端处理函数 | api 映射及 Rust 实现 | 一次确认实际覆盖的内容 |
| --- | --- | --- |
| [AiSettings::buildIndex(rebuild)][AISET] | [api.buildAiIndex][API] → `ai_build_index` → `ensure_allowed(true,consent)` → [indexer::build_index / index_chat / embedding_input][INDEXER] | 当前账号待处理或重建的文本消息分段送向量模型，含聊天名、日期及说话者等上下文；消息数量来自 status，未限定顶部日期范围。一次确认覆盖这次任务的多个向量批次，不逐 HTTP 请求弹窗。 |
| [Ask::runAsk][ASK] | [api.ask][API] → `ai_ask` → `ensure_allowed(true,consent)` → [features::ask / ask_prompt][FEATURES]、[retrieval::retrieve][RETRIEVAL] | 问题、最多 12 段来源，包含聊天名、类型、时间、说话者和文本；有现存向量索引时还把问题送向量模型。无索引仍能关键词检索后用对话模型回答；没有来源时返回固定“未找到”，但有索引时此前仍可能已发送问题向量请求。前端“每段不超过约 600 字”不是严格上限：关键词回退会取半径 3 的上下文、每条正文截 300 字，合计可大于 600；上下文还可能越过日期边界。 |
| [AiSummaryPanel::generate][SUMMARY] | `api.aiEstimate → ai_estimate → Database::ai_estimate` 本地估算；[api.summarizeChat][API] → `ai_summarize_chat` → 生成前门禁 → [features::summarize_chat / segment_lines][FEATURES] | 所选聊天当前日期范围的文本、聊天名与说话者／时间；每条正文最多取 800 字，长历史分段并合并要点，一次确认覆盖整个任务。 |
| [TopicsPanel::analyze][TOPICS] | 本地 `api.aiEstimate(accountId,{},chatId)`；[api.chatTopics][API] → `ai_chat_topics` → 生成前门禁 → [indexer::index_chat][INDEXER]，再 [features::chat_topics][FEATURES] | **索引补建覆盖这个聊天所有日期的尚未索引文本**（index_chat 使用 `DateRange::default()`）；之后话题聚类按选定 range 取向量，每个话题最多 5 个样本、每个样本文本取 300 字给对话模型。不能说本次所有发送都受顶部日期范围限制。 |
| [Commitments::startScan][COMMITMENTS]，或 [ChatMemoryPanel::scan("commitment")][MEMORY] | 本地 `api.commitmentEstimate → ai_commitment_estimate → ai::commitments::plan_scan/estimate`；[api.scanCommitments][API] → `ai_scan_commitments` → 门禁 → [ai::commitments::scan_commitments / extraction_prompt][AICOMMIT] → [extraction::scan][EXTRACTION] | 当前范围未扫描且有约定线索的窗口，附前最多 6 条上下文；窗口正文预算 3000 字，单条正文截 300 字，附加上下文／元信息／最多 8 条未完成约定（标题、方向／人名、截止日期）另计。群约定要求窗口中有“我”的消息；无提示窗口在本机跳过并记录扫描进度。估算并非完整 HTTP payload 字节数。 |
| [ChatMemoryPanel::scan("decision")][MEMORY]，按钮只在群聊显示 | 本地 `api.decisionEstimate → ai_decision_estimate → ai::decisions::plan_scan/estimate`；[api.scanDecisions][API] → `ai_scan_decisions` → 门禁 → [ai::decisions::scan_decisions / extraction_prompt][AIDECISION] → [extraction::scan][EXTRACTION] | 群决策线索窗口、前最多 6 条上下文、最多 8 条仍有效的决定／待定记录（还可含负责人、截止日期）。`DecisionExtractor::group_needs_me=false`，不能套用群约定必须有我发言的过滤。只在手动扫描时触发，非同步后自动任务。 |

**确认不覆盖的分支及 README 的措辞边界：**

1. 缓存：`ai_summarize_chat / ai_chat_topics / ai_year_report` 的 `cache_mode("cached")` 只查本地缓存，命中或禁止生成时在门禁前返回；`generate` 也可直接复用有效缓存。前端进入页面的 cached 读取不弹确认。不能把读取本地 AI 结果描述成再次上传。[AiSummaryPanel 的 useEffect][SUMMARY]、[TopicsPanel 的 useEffect][TOPICS]、[Report 的缓存 useEffect][REPORT]、[ai/commands.rs::cache_mode / cached][COMMANDS]。
2. 年度 AI 回顾：[Report::generate][REPORT] → [api.yearReport][API] → `ai_year_report`，无 cloudConsent 参数；后端 `ensure_allowed(false,false)`，云端仍要求 enabled 与 allow_cloud，但无逐次聊天内容弹窗。[features::report_payload / year_report / alias_names / restore_names][FEATURES] 只构造统计、日期时段与对象名称，不发送正文；云端默认匿名化名称，可取消匿名化而发送名称。统计并非只有数字。[README“AI 功能”][README] 已列出名称与默认代号，数据管理或指引中“每次发送前确认”的宽泛措辞须按“聊天正文操作”理解。
3. 测试连接：[AiSettings::test][AISET] 先 save → [api.testAi][API] → `ai_test_connection`；[ai/commands.rs::ai_test_connection][COMMANDS] 只对非本机检查 allow_cloud，不检查 enabled，也不调用 ensure_allowed。它会请求模型列表，并发送固定“连接测试”给向量模型，没有聊天正文且不弹 CloudConsent。因此 **AI 默认关闭并不意味着用户主动测试连接时绝不会联网**；不是后台自动上传聊天。
4. 停止与进度：[ai.ts::runAiTask / stopAiTask][AI] → `api.onAiStream / cancelAi`，分别是 `ai-stream` 事件与 `ai_cancel`（[lib.rs::run][LIB] 注册，[ai/commands.rs::ai_cancel][COMMANDS] 实现）；索引另走 `ai-index-progress / ai_cancel_index`。这些操作本身不发送聊天正文，不要求重新确认。

侧栏文案由 [ai.ts::usesCloud][AI] 的 `enabled && !isLocal && allowCloud` 决定，[App 侧栏 privacy 分支][APP] 才显示“已启用云端 AI”。它是保存设置状态标识，不是网络审计结果，不能证明全部历史请求均留在本机。源码只按地址主机分类，也没有在本次验证实际网络路由。

## 历史导览核对

核对对象：[history/chat-insight-experience.md 的“项目入口／页面与体验路径／模块关系／AI 调用链／测试与验证命令”][HISTORY]。没有为了填充“过时”类别而把仍一致的文字判成过时。

| 历史内容 | 判断 | 当前依据与维护解释 |
| --- | --- | --- |
| React 入口挂 AiProvider 和 App；App 用 view 状态切页；无账号欢迎页，有数据集自动 tour | 一致，但“数据就绪”需按条件解释 | [main.tsx::createRoot][MAIN]、[App 的初始化／自动导览 useEffect][APP]、[guide.ts::guideCompleted][GUIDE]；不是检查 messageCount。 |
| 浏览器只给空数据集和预览能力，导入／同步／本地查询需桌面 | 一致 | [api.ts::inTauri / browserCapabilities / call][API]、[App::importData][APP]；还需补充插件先失败这一条实际路径。 |
| 开发默认无桥，桌面可通过 JSONL 入门；便携格式事务、去重、不带约定／决策 | 与 README 和当前实现一致 | [README“本地开发”][README]、[portable::PortableRecord / import_jsonl / export_jsonl][PORTABLE]；未据此确认实际安装包已携带桥。 |
| 六个页面及关键词、统计、年度洞察无需 AI；约定／决策已保存记录可本地管理 | 一致 | [App::navItems][APP]、[Ask::runSearch][ASK]、[Database::chat_detail][DB]、[insights::Database::year_insights][INSIGHTS]、[Commitments::reload][COMMITMENTS]、[ChatMemoryPanel::reload][MEMORY]。 |
| api.ts 是“前端统一调用入口”，模块图把 App 全部接到 API | 表述需更新／过于概括，不是所有入口的真实链 | [App::importData / exportData][APP] 直接使用 dialog 插件；`clearData` 先用 window.confirm，导览与数据集选择本身没有自定义 Rust 命令。应把统一业务 invoke 层与插件／UI 状态分开写。 |
| 云端服务启用及聊天内容确认由前后端共同校验 | 核心一致，覆盖范围需补充 | [AiProvider::confirmCloud][PROVIDER]、[CloudConsent][CONSENT]、[AiSettings::ensure_allowed][SETTINGS]；年度统计生成、连接测试、缓存读取的分支见上文。历史没有提供这些例外，不能据旧图外推所有 AI 操作逐请求弹窗。 |
| 总览、问答、同步及 AI 事件的例子 | 一致但简化 | [Overview 的 useEffect][OVERVIEW] → `api.dashboard → query_dashboard → Database::dashboard`；[Ask::runAsk][ASK] → `api.ask → ai_ask → features::ask`；[lib.rs::run][LIB] 注册，事件由 [bridge::emit_progress][BRIDGE] 发 `sync-progress`，前端 [api.onAiStream / onAiIndexProgress][API] 分发。 |
| Vite 1420、脚本和 CI 检查列表 | 定义一致，不能当成通过记录 | [vite.config.ts::server][VITE]、[package.json::scripts][PACKAGE]、[ci.yml::jobs.verify.steps][CI] 提供静态定义；本次未执行任一命令。 |
| HTML、Tauri 配置、合成 testdata、发布桥构建／上游固定包等外部或未选文件的描述，以及全部测试覆盖／运行成功 | 本次证据不足，不判定过时或成功 | 历史引用的 index.html、tauri.conf.json、testdata 与发布准备脚本不在本次选定输入中，未读取；本次也没有测试报告、安装包或第三方桥实现，不能凭 [历史导览][HISTORY]／[README][README] 的文字确认运行结果。 |

## 改进建议

这些是建议，未修改实现或 README。

1. 首次欢迎页提供同步取消入口，统一 `read_all_sessions / sync_chat / sync_wechat` 的取消返回语义，并说明取消是协作式；目前文案比控件可达性更宽。[Onboarding][ONBOARD]、[App 的 sync-strip][APP]、[bridge::sync_wechat][BRIDGE]。
2. 以实际运行时能力显示导入入口；浏览器可以提示“请使用桌面应用”，避免直接触发 dialog 插件错误。数据管理的“运行模式”当前只看 directWechatRead，建议同时反映桥缺失及数据集来源，避免把 JSONL 数据集显示成正在直读微信。[App::importData][APP]、[DataManagement 的 source-card / canReadWechat][DATA]、[api.ts::browserCapabilities][API]。
3. 让发送说明与真实 payload 对齐：问答的 600 字不是硬上限；话题索引可能跨整个聊天；扫描估算不含所有元信息与已有条目；年度回顾含名称、日期与统计且无内容确认弹窗。明确“一次操作确认覆盖其多个模型请求”，再考虑是否需要单独的年度统计发送提示。[Ask::runAsk][ASK]、[TopicsPanel::analyze][TOPICS]、[retrieval::retrieve][RETRIEVAL]、[extraction::estimate][EXTRACTION]、[features::report_payload][FEATURES]。
4. 把 README 的“导入的数据集也没有约定”限定为不会从 JSONL 恢复；写清重新扫描和重导已有数据集的区别，并解释媒体不随文件迁移。[portable::import_jsonl / PortableRecord][PORTABLE]、[README][README]。
5. 更新旧导览的“统一 API”图，补充插件调用与纯 UI 路径；描述平台时沿用能力矩阵，不将 Intel 直连限制扩大为整个应用不可用。[历史导览][HISTORY]、[App][APP]、[bridge::capabilities][BRIDGE]。

## 需实际运行验证

以下均未执行，后续由维护者在授权环境用合成数据／受控服务验证，细项见配套清单：

- 浏览器欢迎页、指引高亮／键盘交互和导入失败展示；桌面首次引导的触发、完成标记持久化与已有零消息账号场景。[App][APP]、[GettingStarted][GETTING]、[guide.ts][GUIDE]。
- Apple Silicon 的 wxkey、权限与微信兼容性；Windows x64 桥初始化、密钥获取；Intel／桥缺失时 JSONL 可用性；取消边缘时点、子进程等待、部分聊天警告、账号时间戳识别与备份恢复。这些不能由 [bridge][BRIDGE] 和 [wechat_account][WECHAT] 静态证明成功。
- JSONL 文件对话框、事务回滚、重复导入、已有聊天声明／已有约定保留、导出全数据集与媒体占位，依据 [App][APP]、[portable][PORTABLE]、[Database][DB] 的预期仍待运行确认。
- 中文短词／多词查询、边界日期、200 条截断、前后文可跨日期、进入详情不定位原消息等体验；源码为 [Ask][ASK]、[Database::search_messages / message_context][DB]。
- 新设置与已保存 AI 设置、local/cloud 判断、未许可拒绝、取消／确认、缓存不生成、年度匿名开关、AI 关闭时主动测试连接、本机自动扫描、话题跨日期索引及实际请求 payload。弹窗展示与后端门禁由 [CloudConsent][CONSENT]、[AiSettings::ensure_allowed][SETTINGS] 和 [ai/commands.rs][COMMANDS] 支撑，未进行网络抓取或模型调用；更不宣称模型结果准确。

## 引用来源（均为选定输入）

正文引用采用下列绝对路径，历史文档的相对链接未被当作本报告引用路径使用。

[README]: <体验目录>/materials/README.md
[HISTORY]: <体验目录>/materials/history/chat-insight-experience.md
[PACKAGE]: <体验目录>/materials/package.json
[VITE]: <体验目录>/materials/vite.config.ts
[CI]: <体验目录>/materials/.github/workflows/ci.yml
[MAIN]: <体验目录>/materials/src/main.tsx
[RMAIN]: <体验目录>/materials/src-tauri/src/main.rs
[APP]: <体验目录>/materials/src/App.tsx
[API]: <体验目录>/materials/src/api.ts
[ONBOARD]: <体验目录>/materials/src/views/Onboarding.tsx
[DATA]: <体验目录>/materials/src/views/DataManagement.tsx
[GETTING]: <体验目录>/materials/src/components/GettingStarted.tsx
[GUIDE]: <体验目录>/materials/src/guide.ts
[LIB]: <体验目录>/materials/src-tauri/src/lib.rs
[BRIDGE]: <体验目录>/materials/src-tauri/src/bridge.rs
[WECHAT]: <体验目录>/materials/src-tauri/src/wechat_account.rs
[DB]: <体验目录>/materials/src-tauri/src/database.rs
[PORTABLE]: <体验目录>/materials/src-tauri/src/portable.rs
[MEDIA]: <体验目录>/materials/src-tauri/src/media.rs
[ASK]: <体验目录>/materials/src/views/Ask.tsx
[CONTEXT]: <体验目录>/materials/src/components/MessageContext.tsx
[DETAIL]: <体验目录>/materials/src/views/ChatDetail.tsx
[OVERVIEW]: <体验目录>/materials/src/views/Overview.tsx
[REPORT]: <体验目录>/materials/src/views/Report.tsx
[ANALYSIS]: <体验目录>/materials/src-tauri/src/analysis.rs
[INSIGHTS]: <体验目录>/materials/src-tauri/src/insights.rs
[DEFAULTS]: <体验目录>/materials/src/aiDefaults.ts
[AI]: <体验目录>/materials/src/ai.ts
[PROVIDER]: <体验目录>/materials/src/components/AiProvider.tsx
[CONSENT]: <体验目录>/materials/src/components/CloudConsent.tsx
[AISET]: <体验目录>/materials/src/components/AiSettings.tsx
[SETTINGS]: <体验目录>/materials/src-tauri/src/ai/settings.rs
[COMMANDS]: <体验目录>/materials/src-tauri/src/ai/commands.rs
[CLIENT]: <体验目录>/materials/src-tauri/src/ai/client.rs
[INDEXER]: <体验目录>/materials/src-tauri/src/ai/indexer.rs
[FEATURES]: <体验目录>/materials/src-tauri/src/ai/features.rs
[RETRIEVAL]: <体验目录>/materials/src-tauri/src/ai/retrieval.rs
[SUMMARY]: <体验目录>/materials/src/components/AiSummaryPanel.tsx
[TOPICS]: <体验目录>/materials/src/components/TopicsPanel.tsx
[COMMITMENTS]: <体验目录>/materials/src/views/Commitments.tsx
[MEMORY]: <体验目录>/materials/src/components/ChatMemoryPanel.tsx
[EXTRACTION]: <体验目录>/materials/src-tauri/src/ai/extraction.rs
[AICOMMIT]: <体验目录>/materials/src-tauri/src/ai/commitments.rs
[AIDECISION]: <体验目录>/materials/src-tauri/src/ai/decisions.rs
[COMMITMENTDB]: <体验目录>/materials/src-tauri/src/commitments.rs
[DECISIONDB]: <体验目录>/materials/src-tauri/src/decisions.rs
