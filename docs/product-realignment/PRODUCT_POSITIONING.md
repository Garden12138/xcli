# 产品定位

## xcli 做什么

xcli 面向已使用原生 AI Coding Agent CLI、需要统一运行和交付任务的个人开发者及本机自动化用户。明确支持 Codex、Cursor 和 Antigravity；保留 Claude、Gemini、OpenCode、generic Adapter。支持声明与实测角色/平台分开，不把 Adapter 存在理解为所有组合已真实验收。

原生 CLI 维护安装、登录、模型选择、工作区信任、工具审批与额度；xcli 维护任务选择、计划、材料、执行进程、预算、状态、取消、恢复、验证与交付。用户给定目标和边界，最终检查真实产物，不靠 Agent 自述完成。

核心价值是减少跨 CLI 手工启动、复制上下文、追踪会话、检查后台执行和复核产物的成本。运行模式为 `run/use/resume` 的直接执行和 `task` 的有界编排；不以 Gateway 启用、模型订阅接入或四业务应用改造为前提。

## 七项核心

1. CLI Discovery / Doctor：发现安装、版本和能力来源。
2. CLI Adapter：稳定 argv/结构化事件/Session/错误映射。
3. Task / Workflow Orchestrator：计划、阶段、交接、验证、复核与有界修复。
4. Session / Run / Jobs：保留身份、历史与后台工作。
5. Execution Supervision：预算、超时、取消、进程收尾与异常恢复。
6. Result / Artifact Management：真实文件、摘要、测试、复核与接收。
7. 本地配置、日志和任务审计：可诊断、兼容、不泄露凭据。

## 明确排除

不实现模型供应商 OAuth、登录或 Token 刷新；不读取、复制、导出原生 CLI 凭据；不共享订阅额度或提供通用模型 API；不把原生 CLI 包装成 OpenAI Chat/Responses/Images 服务。冻结已有这类实验，而非继续补齐其真实验收。

不通过换账号、换 CLI、自动 trust、force/yolo、修改长期权限或隐式增加预算绕过失败。原生工具/云端副作用不由 xcli 进程退出推断撤销。跨 CLI 是计划中的分工，不是 quota fallback。

不承诺协作一定更准确、更便宜，也不宣称通用 OS 沙箱、Windows 后台支持、所有模型/媒体能力或正式版本已发布。R3 的收益未证实、未测组合和历史失败继续可查。

## 接口与发布边界

当前产品入口是 CLI。未来本地应用可使用明确授权的 Task API/受限 IPC，围绕 Agent、工作区、材料、产物、预算和结果设计。它不是模型供应商授权委托，更不是不限量调用用户订阅。

核心 CI、候选安装和产品验收只要求核心调度能力。保留实验代码的编译/离线回归作为源码维护检查独立存在；不要求 Gateway 真实 Profile、OAuth 资格或模型服务就绪。此轮没有发布、部署或改动用户原生环境。
