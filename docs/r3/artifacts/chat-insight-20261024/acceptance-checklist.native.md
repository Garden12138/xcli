> 导出说明：experience-root 是本机体验目录的占位符；源码引用对应所选输入快照。原始文档及本机可打开的引用路径保持不变。

# Chat Insight 上手路径验收清单

**状态：以下预期均为源码静态核实，所有操作均待实际执行，不能记为通过。**本次未启动浏览器/桌面、未读取真实微信数据、未调用模型、未执行 npm/cargo 测试。输入根目录：`experience-root/materials`；链接及符号为源码证据，不是运行证据。[旧导览][history]只提供核对线索。

建议使用隔离的应用数据目录、合成 JSONL 和可检查请求体的模型测试服务；涉及微信授权/换号的项目由维护者在授权设备上执行。每行补记平台/架构、版本、时间、实际结果和脱敏截图或计数；云端项保留测试服务请求记录，不记录真实聊天内容。

## 首次使用与运行边界

| 编号／操作（均待执行） | 已确认的源码预期 | 证据位置与符号 | 未验证项／运行留证 |
| --- | --- | --- | --- |
| F1：运行 `npm run dev`，打开 1420 | browser/unknown、空数据集、AI 默认关闭；欢迎页与预览指引可见；无微信直读。一般数据 API 报桌面专用错误，文件对话框也依赖桌面。 | [package.json][package] `scripts.dev`；[vite.config.ts][vite] `server.port`；[api.ts][api] `inTauri/browserCapabilities/call/accounts/aiSettings`；[App.tsx][app] `importData`。 | 浏览器实际渲染、导入按钮的具体错误表现；截图不能作为桌面验收。 |
| F2：具备 [README.md][readme] 前置依赖后运行 `npm run tauri dev` | Rust 初始化应用 `chat-insight.db`、注册文件对话框与命令；Node.js 22+、Rust stable、Tauri 依赖见 [README.md][readme]「本地开发」。 | [lib.rs][lib] `run`；[package.json][package] `scripts.tauri`。 | 启动、系统依赖与插件行为；本次没有桌面启动记录。 |
| F3：空库点「先看使用指引」；导入后走完或跳过，再重开 | 空库为 preview；有数据、未完成且不忙/不在同步时自动 tour；关闭 tour 写完成标记，侧栏仍可重开。 | [App.tsx][app] `autoGuide/guideMode/closeGuide/showGuide`；[guide.ts][guide] `previewSteps/tourSteps/guideCompleted/markGuideCompleted`。 | 聚焦高亮、前后步骤、重启后标记；localStorage 不可用时不会自动 tour。 |
| F4：分别检查 Apple Silicon、Intel Mac、Windows x64 能力与入口 | 直读只限 macOS/aarch64、Windows/x86_64，且连接需桥 CLI；Mac 还需 wxkey。Intel 为导入模式；支持架构缺桥为 bridge_missing，仍可导入。[README.md][readme]「平台能力」列两种 Mac DMG、Windows NSIS；不代表安装包已验证。 | [bridge.rs][bridge] `capabilities/connect/BridgePaths::resolve`；[Onboarding.tsx][onboarding] `canConnect`。 | 三平台安装包、桥完整性、Intel 实际导入；不能用单台机器推定全部平台通过。 |

## 连接与同步

| 编号／操作（均待执行） | 已确认的源码预期 | 证据位置与符号 | 未验证项／运行留证 |
| --- | --- | --- | --- |
| S1：支持平台安装并登录微信 4.x，首次连接；再测试授权失败 | `bootstrap → bootstrap_bridge → connect` 成功后再 `sync → start_sync`；失败显示原因。Mac 先告知磁盘权限/可能重开微信/管理员授权；Windows 先打开普通聊天，以 `cache refresh --force` 初始化。 | [App.tsx][app] `bootstrapAndSync`；[api.ts][api] `bootstrap/sync`；[lib.rs][lib] `bootstrap_bridge/start_sync`；[bridge.rs][bridge] `capabilities/run_platform_bootstrap/run_wxkey_bootstrap`。 | 系统弹窗、权限失败重试、微信重启、真实密钥初始化；分别留证，不假定授权成功。 |
| S2：同步一次，增加消息再点立即同步/右上刷新，重复同步 | 标准化消息进入应用库；按账号/聊天增量读取并去重，刷新账号计数；`sync-progress` 通知进度。部分聊天失败可产生 warnings，完成不等于每个聊天成功。 | [App.tsx][app] `runSync`；[api.ts][api] `sync/onSyncProgress`；[bridge.rs][bridge] `sync_wechat/sync_chat/emit_progress`；[database.rs][database] `Database::insert_message/max_local_id`。 | 新增计数、重复为零、警告展示、原微信数据未改动；检查最后一个聊天失败情形。 |
| S3：已有数据主界面同步时取消；另观察空库首次同步 | `cancelSync → cancel_sync` 置标记，循环检查后停止，已写入保留。主界面非 preparing 才有取消；空库欢迎页无取消按钮，是当前入口缺口。 | [App.tsx][app] `syncProgress/runSync`；[Onboarding.tsx][onboarding] `Onboarding`；[api.ts][api] `cancelSync`；[lib.rs][lib] `cancel_sync`；[bridge.rs][bridge] `sync_chat/sync_wechat`。 | 取消延迟、末聊天取消的返回状态/提示、保留数量；不要将建议补按钮当成现有能力。 |
| S4：微信登录另一个号并点聊天，再切换；回切旧号；再选顶部旧数据集 | 最近写入目录用于识别账号；旧配置备份、已连接账号恢复、新号初始化；记录分数据集。未换号则提示；顶部下拉只切换查看，同步仍跟随桥当前身份。 | [App.tsx][app] `switchAccount/accountId/runSync`；[api.ts][api] `switchAccount` → [lib.rs][lib] `switch_wechat_account`；[bridge.rs][bridge] `switch_account/sync_wechat`；[wechat_account.rs][account] `detect_active_account/AccountStore::save/load`。 | 真实多账号识别、配置恢复与失败回滚、数据隔离；保留各数据集脱敏计数。 |

## JSONL 导入导出

| 编号／操作（均待执行） | 已确认的源码预期 | 证据位置与符号 | 未验证项／运行留证 |
| --- | --- | --- | --- |
| J1：桌面欢迎页/数据管理导入合成 v1 文件，再导入同一文件 | 首行 manifest，后续 chat/message；ID 为 `import:{dataset_id}`；刷新并选中导入数据集，重复消息不增加，chat 元信息可更新。 | [App.tsx][app] `importData`；[api.ts][api] `importJsonl` → [lib.rs][lib] `import_jsonl` → [portable.rs][portable] `import_jsonl/PortableRecord`。 | 文件选择器取消、首次/重复消息计数；`round_trip_is_idempotent` 是已有测试符号，本次未运行。 |
| J2：在新库依次导入未知 schema、坏 JSON、坏 RFC3339、未声明聊天、第二 manifest | 任一失败不提交；事务内已处理记录一并回滚。消息所需 chat 可来自当前文件或已有同数据集，故“未声明”用新库测试。 | [portable.rs][portable] `import_jsonl/SCHEMA_VERSION/rejects_unknown_schema_without_writing`。 | 比较失败前后数据集、聊天、消息数量均不变；必须含“前面有效、后面坏”的中途故障样本。 |
| J3：选窄日期范围后导出，再在另一空库导入 | 导出当前数据集全部记录，与日期范围无关；仅 manifest/chat/message，含正文及发送者/聊天标识；不带密钥、原库、媒体文件/媒体路径字段、约定和群决策。 | [App.tsx][app] `exportData`；[api.ts][api] `exportJsonl` → [lib.rs][lib] `export_jsonl` → [portable.rs][portable] `export_jsonl/PortableRecord`。 | 实际字段、全量计数与跨平台往返；新库无约定/群决策。JSONL 需按聊天正文保管。 |
| J4：在导入数据集生成约定/群决策后重复导入；尝试查看媒体；清空当前数据集 | 重导不会迁移或清除目标已有约定/决策；导入集不满足当前微信账号媒体身份。清空有二次确认，仅删应用数据集及级联记录，不删微信原文件，也不代表清除全部 AI 设置/桥配置。 | [portable.rs][portable] `import_jsonl`；[media.rs][media] `ensure_current_account`；[App.tsx][app] `clearData`；[api.ts][api] `clearAccount` → [lib.rs][lib] `clear_account`；[database.rs][database] `Database::clear_account/initialize/SCHEMA_V3/SCHEMA_V4`。 | 条目重导前后数量、媒体占位、清空级联与其他数据集保留；只在可丢弃测试库执行。 |

## 关键词搜索与无需 AI 的功能

| 编号／操作（均待执行） | 已确认的源码预期 | 证据位置与符号 | 未验证项／运行留证 |
| --- | --- | --- | --- |
| K1：保持 AI 关闭，查看总览、详情词云、年度数字与关系变化 | 图表、时间线、关键词提取、年度统计/洞察可用；导入导出与同步也不依赖 AI。年度文字回顾、摘要、语义话题、自然语言回答、扫描约定/群决策另需模型。 | [api.ts][api] `dashboard/chatDetail/yearInsights`；[database.rs][database] `Database::dashboard/chat_detail`；[analysis.rs][analysis] `top_keywords`；[insights.rs][insights] `Database::year_insights`。 | 离线且无模型服务时的实际渲染与计数；表情 CDN 是 [README.md][readme]「隐私边界」说明的网络例外，不能宣称全应用绝对离线。 |
| K2：搜索中文短词/长词、空格多词，限制聊天及日期，准备超过 200 条命中 | AI 关闭且未手选时默认关键词；前六个去重词 AND，≥3 字符 trigram、短词 LIKE；日期边界包含当日。UI 请求 200、降序、高亮，达到上限显示 200+，没有在此加载所有结果。 | [Ask.tsx][ask] `Ask/runSearch/highlight`；[api.ts][api] `searchMessages` → [ai/commands.rs][commands] `search_messages` → [database.rs][database] `Database::search_messages`。 | 中文命中、组合词与筛选正确性、上限/空态；后端上限 500 不等于 UI 请求 500。 |
| K3：点命中，关闭上下文，再在详情打开 | 关键词上下文前后各 8 条、local_id 升序；不受顶部日期限制。跳详情传 chatId、继续使用当前日期范围，不保证定位命中消息。 | [MessageContext.tsx][context] `MessageContext`；[api.ts][api] `messageContext` → [ai/commands.rs][commands] `message_context` → [database.rs][database] `Database::message_context`；[App.tsx][app] `openDetail`；[ChatDetail.tsx][detail] `ChatDetail`。 | 锚点滚动、Esc、聊天切换及日期外上下文；不能把“打开聊天”验成“精确跳转原消息”。 |

## AI 开关与云端确认

| 编号／操作（均待执行） | 已确认的源码预期 | 证据位置与符号 | 未验证项／运行留证 |
| --- | --- | --- | --- |
| A1：全新设置检查默认值；手动开启保存；重启；关闭后尝试生成 | enabled/allowCloud/autoScanCommitments 默认 false；本机地址 127.0.0.1:11434/v1，模型 qwen3:8b、bge-m3。生成门禁拒绝 AI 关闭；设置和 Key 分存，界面不回显 Key。 | [aiDefaults.ts][defaults] `defaultAiSettings`；[AiSettings.tsx][ai-settings] `save`；[api.ts][api] `aiSettings/saveAiSettings`；[ai/commands.rs][commands] `ai_get_settings/ai_save_settings`；[ai/settings.rs][settings] `AiSettings::default/ensure_allowed/SettingsStore::save/view`。 | 持久化、Key 保留/删除、错误反馈和磁盘权限；不从代码推定各 OS 权限已验证。 |
| A2：本机模型测试/生成；改为非本机或局域网地址，先不允许再允许云端 | 仅 localhost/127.0.0.1/::1 算本机；云端未 opt-in 时后端拒绝。保存并测试连接只发模型列表请求和固定「连接测试」向量，可在 AI 关闭时执行；本机内容请求不弹云确认。 | [AiSettings.tsx][ai-settings] `isLocalUrl/test`；[api.ts][api] `testAi`；[ai/commands.rs][commands] `ai_test_connection`；[ai/settings.rs][settings] `is_local_endpoint/ensure_allowed`。 | 实际服务可用性、向量维数、错误处理；固定探测与聊天发送必须分别记录。 |
| A3：云端建立索引、提问、摘要、话题、约定扫描、群决策扫描；各次先取消再确认 | 每个新内容请求均经确认；显示目标 host 与发送范围/估算。取消、Esc、遮罩返回拒绝，不调用生成；确认传 true。后端拒绝 cloudConsent=false；已有缓存读取不等于发送。 | [AiProvider.tsx][provider] `confirmCloud`；[CloudConsent.tsx][consent] `CloudConsent`；[api.ts][api] `buildAiIndex/ask/summarizeChat/chatTopics/scanCommitments/scanDecisions`；[ai/commands.rs][commands] `ai_build_index/ai_ask/ai_summarize_chat/ai_chat_topics/ai_scan_commitments/ai_scan_decisions`；[ai/settings.rs][settings] `ensure_allowed`。 | 测试服务记录取消时零正文请求、确认后目标/内容匹配；不能只验弹窗。摘要/扫描估算、话题全聊天未索引正文、问题最多 12 段来源都需核对。 |
| A4：云端生成年度回顾，分别保留/取消名称匿名化 | 只发送统计及对象名称，不发送正文；默认代号并在本机还原。无需聊天正文逐次确认，但仍要求 enabled 和 allowCloud；取消匿名化可发送真实名称。 | [Report.tsx][report] `anonymize/generate`；[api.ts][api] `yearReport`；[ai/commands.rs][commands] `ai_year_report` 的 `ensure_allowed(false,false)`；[ai/features.rs][features] `report_payload/alias_names/year_report`。 | 用测试服务核对实际 payload 无正文、代号/原名切换、还原效果；不能沿用“所有 AI 请求都弹窗”的旧理解。 |
| A5：分别设置本机/云端自动扫描；同步新增与零新增；观察侧栏 | 自动扫描默认关闭；开启后仅本机、AI 开启、同步有新增时扫近 30 天约定，云端保存会置 false。侧栏云标识条件是 enabled、非本机、allowCloud；标识不是上传检测器。 | [App.tsx][app] `runSync`；[ai/settings.rs][settings] `SettingsStore::save`；[ai.ts][ai] `usesCloud`；[AiSettings.tsx][ai-settings] `AiSettings`。 | 新增触发、无新增不触发、云端无自动请求、徽标切换；群决策仍由手动扫描入口触发。 |

## 建议与验收记录分开

| 类型 | 维护动作 |
| --- | --- |
| 改进建议，未实施 | 补欢迎页首同步取消；浏览器提示用“预览模式”；数据管理运行模式结合桥和数据集来源；指引说明统计回顾/连接测试例外。[Onboarding.tsx][onboarding] `Onboarding`、[DataManagement.tsx][data] `DataManagement`、[guide.ts][guide] `tourSteps`、[ai/commands.rs][commands] `ai_year_report/ai_test_connection`。 |
| 历史核对 | [旧导览][history]「启动方式」「页面与体验路径」「AI 调用链」的主线保留，自动引导/云确认须补上上述条件。其测试覆盖介绍不代表此次运行通过；不扩大到其他模块审计。 |
| 待补执行记录 | 每行填写“通过/失败/阻塞＋实际证据”，失败与已知入口缺口分别记。优先 F1–F4、J1–J3、K1–K3，再在合适设备/测试服务执行 S、A；只有真实执行后才能勾选通过。 |

[history]: experience-root/materials/history/chat-insight-experience.md
[package]: experience-root/materials/package.json
[vite]: experience-root/materials/vite.config.ts
[api]: experience-root/materials/src/api.ts
[app]: experience-root/materials/src/App.tsx
[lib]: experience-root/materials/src-tauri/src/lib.rs
[onboarding]: experience-root/materials/src/views/Onboarding.tsx
[guide]: experience-root/materials/src/guide.ts
[bridge]: experience-root/materials/src-tauri/src/bridge.rs
[database]: experience-root/materials/src-tauri/src/database.rs
[account]: experience-root/materials/src-tauri/src/wechat_account.rs
[portable]: experience-root/materials/src-tauri/src/portable.rs
[media]: experience-root/materials/src-tauri/src/media.rs
[analysis]: experience-root/materials/src-tauri/src/analysis.rs
[insights]: experience-root/materials/src-tauri/src/insights.rs
[ask]: experience-root/materials/src/views/Ask.tsx
[context]: experience-root/materials/src/components/MessageContext.tsx
[detail]: experience-root/materials/src/views/ChatDetail.tsx
[defaults]: experience-root/materials/src/aiDefaults.ts
[ai-settings]: experience-root/materials/src/components/AiSettings.tsx
[commands]: experience-root/materials/src-tauri/src/ai/commands.rs
[settings]: experience-root/materials/src-tauri/src/ai/settings.rs
[provider]: experience-root/materials/src/components/AiProvider.tsx
[consent]: experience-root/materials/src/components/CloudConsent.tsx
[report]: experience-root/materials/src/views/Report.tsx
[features]: experience-root/materials/src-tauri/src/ai/features.rs
[ai]: experience-root/materials/src/ai.ts
[data]: experience-root/materials/src/views/DataManagement.tsx

[readme]: experience-root/materials/README.md
