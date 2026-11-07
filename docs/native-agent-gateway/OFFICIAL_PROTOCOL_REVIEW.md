# 官方接口、权限和条款核查（2026-11-07）

本次实际访问公开官方页面；未启动 native `--help`/`--version`/登录/推理，不读取任何凭据。网站当前版本不能证明本机旧二进制具有新参数；本仓库历史原生 PASS 不能证明 Gateway 权限边界成立。

| 入口 | 核查与实现决策 | 当前阻断 |
| --- | --- | --- |
| Codex exec | [非交互官方文档](https://learn.chatgpt.com/docs/non-interactive-mode)描述 JSON、恢复与只读沙箱；优先复用现有 Run/Resume | 只读不约束全盘读取/所有继承工具；当日网页新增忽略用户配置等参数未在本机版本验证，禁止默加。真实 profile 关闭 |
| Codex SDK/app-server | [官方 app-server](https://learn.chatgpt.com/docs/app-server)描述 initialize/initialized、请求 ID、thread/turn、原生 delta、审批与 interrupt。`internal/nativeadapter/rpc_eval*` 只做静态有限状态评估：未初始化拒绝、关联 ID、控制面拒绝、审批 decline、interrupt ack 不能等同 terminal | 无本机同版本 schema/真实权限/取消 parity，G5 不晋升；不安装 SDK 或 sidecar |
| Cursor headless | [官方 headless](https://cursor.com/docs/cli/headless)支持 print、stream-json；[权限文档](https://cursor.com/docs/cli/reference/permissions)说明原生权限配置 | 历史 workspace trust 阻断，且无逐次全局工具隔离证据；不加 force/yolo/trust，不改 Trust |
| Antigravity print | [官方 headless](https://antigravity.google/docs/cli/headless/)支持单 turn 与 conversation；[权限文档](https://antigravity.google/docs/permissions/)需结合沙箱执行 | plan/只读和 prompt 均非全盘/网络隔离证明；继承插件与媒体输出关联未验，关闭 profile |

## 条款边界

- [OpenAI 使用条款](https://openai.com/policies/terms-of-use/)页面生效日期 2026-01-01，包含程序化提取和规避限制等限制。公开 Codex 程序化文档与通用条款需结合实际账号/产品适用范围确认；没有据此宣称订阅可转为通用 API 或多人代理服务。当前 native Gateway 路线不作账号许可已确认的声明，真实启用还需本人核查适用账号合同。
- [Cursor 条款](https://cursor.com/terms-of-service)页面更新日期 2026-09-03，约束服务使用、提取、转售及组织协议；headless 文档支持自动化，不等于任意数据与订阅共享获得许可。
- [Antigravity 附加条款](https://antigravity.google/terms)按个人、企业/Cloud 等访问方式区分适用合同，并要求用户负责代理的动作及数据权限；未查询实际账号合同。不能将另一 SDK 的 Key/企业认证路线替代已有 CLI 登录。

工程结论：只实现公开官方传输的适配代码和明确拒绝；不接私有推理端点、不导出登录缓存、不自动换账号/CLI/模型绕过额度。三家的真实业务 profile 均为 BLOCKED，不能因网页有 headless 示例就标记 Gateway available。网站证据是 Documented；本机权限、服务条款适用性、Gateway L01–L05 另行验收。

## 最小实现调整记录

新增 `internal/agentapi` 集中 wire DTO，供 store/service/HTTP/adapter 共用，避免循环依赖；不变更 HTTP 合同。Driver 的同步 `StartTurn(ctx, bound, observer)` 在任务独占 controller 内运行；context 承担只对活 worker 的取消，不暴露持久 PID。缓存发现由 server-owned policy 目录提供。未来 app-server 仍须单独实现双向 pump，不把静态评估器当运行传输。

G1 checkpoint 为可重建缓存，journal 为唯一恢复依据。记录以完整状态帧提交，带版本/序号/长度/头校验/内容校验/commit trailer；有限容量到限拒绝，不悄悄删除审计或已接受事务。后续 G4 检查压缩/保留策略与性能界限。
