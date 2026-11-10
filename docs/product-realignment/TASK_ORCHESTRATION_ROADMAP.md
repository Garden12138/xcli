# Task 编排能力与路线

## R4 保留的 A–G 主链路

| 场景 | 当前入口和事实 | 验收边界 |
| --- | --- | --- |
| A 发现安装/版本/能力 | `agents --json`、`doctor <agent> --json` | 实现及离线覆盖保留；不是登录/额度证明。 |
| B 选择 Codex/Cursor/Antigravity | `run --agent ...` 或 `task --agent ...` | 三个 Adapter 保留；真实可运行取决于原生安装、授权、工作区信任及工具权限。 |
| C 创建/续接 Session | `run --json` 获得 Session/Run ID，`resume`；交互入口 `use` | 只有含有效 Session 的记录可按 Run ID 续接；交互记录未必有可解析 Session ID，原生 Session 必须匹配 CLI。 |
| D 分阶段、交接与复核 | 自动 Task 计划、固定材料、不同 Adapter 复核 | 现有基础保留；`--agent` 锁定单 CLI。没有公开可编辑 DAG/固定 worker/reviewer flags。 |
| E 后台、取消、恢复、日志 | Task detach/status/events/stop/continue；旧 jobs/runs | owner/attempt、预算与 reconcile 保留；Windows 后台 Task/jobs 仍不支持，开机自启不在范围。 |
| F 文本/代码/图片真实产物 | Work 输出目录、Project worktree、文件 hash、检查与复核 | 图片能作为文件保留，但尚无统一 MIME/来源/视觉接收界面。文本路径不是图片，进程成功不是验收成功。 |
| G 原生独立授权与失败 | 无登录/凭据提取、无 quota/账号/CLI 逃逸 fallback | 原生条件失败如实停止；不把 Gateway/SIWC 未完成门禁算到核心 Task 上。 |

R0 可靠解析/argv/退出码、R1 有界 Task/材料/验证、R2 后台/继续/恢复、R3 当前实测与观察材料均保留。R4 验证的是这些路径未因冻结实验而退化，不新增真实模型效果、跨平台执行或所有 Adapter/角色组合验收。

## 最值得推进的三个功能

| 优先级 | 功能 | 解决的问题与通过标准 |
| --- | --- | --- |
| 1 | 可审查的阶段计划与显式角色绑定 | 增加用户可确认的计划/阶段选择、worker/reviewer Adapter 绑定；运行前可看每阶段输入/权限/预算，跨角色材料最小化。失败/继续不得暗换绑定，单 CLI 不冒充独立复核。 |
| 2 | 统一任务列表、时间线与恢复预览 | 补 `task list`、易读日志/当前阶段、waiting/reconcile 原因、下一次 continue 将执行什么/消耗多少预算的预览。只读预览不启动 Agent，避免排查时花额度。 |
| 3 | 统一真实产物清单与验收流水线 | 文件路径/字节数/MIME/hash/来源/测试/复核/用户接收分别展示；图片做真实解码与来源/视觉确认，代码关联当前 diff/test；文件变化使旧验收失效。复用已有 FD/图片校验组件，不新增媒体 Provider。 |

先完善当前 CLI 用户的任务体验，再讨论明确授权的本地 Task IPC。没有 Gateway/OAuth/统一订阅代理里程碑；没有以“再补几次隔离探针”代替产品取舍。

新功能进入实现前单独冻结范围与验收，不把本路线图当作本轮已经实现的命令。当前不开发跨机器调度、任意循环、自动提交/部署、账户切换或通用 OS 沙箱。
