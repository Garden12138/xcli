# 原生 CLI 认证职责

原生 CLI 自行完成安装、登录、账号/订阅/模型授权、Token 刷新和工作区信任。用户在各厂商工具中操作；xcli 不实现供应商 OAuth 或登录命令，不主动切换账号，不读取、复制、导出原生凭据。

## 当前执行行为

`agents` 读取配置/PATH 并执行版本查询；`doctor` 再执行有限时的 `--help`，只报告安装/版本/能力来源。它不调用 login，不遍历认证文件，也不据 help 推断登录、额度或 sandbox 已生效。xcli 可报告原生执行明确返回的认证、权限、额度错误；不为判断状态额外发小模型请求。

Task/Run 启动用户选择的原生 CLI，使用其现有登录入口和用户选择的环境。原生 CLI 自己读取本人的认证资料，不等于 xcli 从登录文件提取 Token。用户配置中的 env/network 仍由用户控制；R4 不修改它们，也不能从当前 argv/环境绑定推断用户未在原生应用中手动换过账号。

旧 `auth_args` 字段仅为兼容保留；R1 已去掉 `xcli auth`。R4 不调用这些参数，不重新添加登录 UI、Token Manager、Keychain 或订阅路由。冻结的 SIWC 契约没有生产网络/凭据存储实现，也不进入核心路径。

## 失败与使用限制

原生工作区 trust、工具审批、地区/模型/额度限制依旧有效。失败后保留原生退出、解析和本地错误类别；不自动 trust、不放宽规则、不 force/yolo、不切账号或换 CLI 绕过限制。自动模式的计划分工在 dispatch 前确定，和失败后的逃避式 fallback 分开。

认证/额度/网络错误不触发材料恢复重试。仅明确的原生 Session 失效可以按现有 Task 策略在同一 CLI/角色上从材料恢复，保留失败尝试和累计预算。用户恢复前应自己在原生 CLI 处理前置条件。

原生模型/工具可能上云并留存会话；localhost、worktree、readonly prompt 和关闭 xcli 原始录制都不能承诺厂商侧不留存。应用必须只导出用户选定、获准的材料，不通过 xcli 获得无限订阅调用资格。

## xcli 自己的应用授权

未来本机 Task API/受限 IPC 可以有 xcli 自己的客户端 Token、OS peer identity、任务/工作区 scope、预算与撤销。这只表示“该本地应用可以请求这个受限任务”，不表示供应商登录或模型使用授权。

已有 `modelstore.Clients` / `agentstore.Clients` 及其轮换/撤销机制保留冻结，用于兼容收尾或将来抽取中性组件。不能把这类 Token 放进供应商 Authorization，也不能把供应商 Token 返回给本地应用。当前没有新 Task API 或代管认证服务。
