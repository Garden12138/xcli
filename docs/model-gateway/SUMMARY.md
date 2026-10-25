# Model Gateway 升级交付总结

M0→M5本仓库实现与离线工程验收已完成；**整体升级仍未通过真实验收**，不作为正式发布或四App已集成的声明。进度/恢复见PROGRESS.md，使用见README.md，API见api-contract.md，逐阶段证据在acceptance/。

## 已实现

- 独立model注册中心、能力/证据/位置/向量身份元数据、固定alias路由与最小App权限；不启动Task、无隐式fallback。
- 受认证本机HTTP：healthz、readyz、models、Chat Completions/SSE、Vision内联图像、图片生成/编辑、Embedding透传。Tools/JSONSchema按能力透传，不执行客户端工具。
- PNG/JPEG/WebP完整解码与像素/字节预算；mask/多图门控；错误输出/尺寸不符/不支持能力明确失败；不伪造图像或向量。
- 256-bit独立客户端Token、私有摘要存储/撤销/轮换、模型/位置/逐请求云同意、Origin/Host边界、并发/速率/超时/取消与脱敏诊断。
- start/serve/status/stop/logs、文件锁单实例、端口0发现/冲突、实例绑定停止、配置摘要审计、配置版本拒绝自动迁移、跨平台构建和部署说明。
- 默认全离线Fake Backend；CLI会员桥接关闭，原生CLI登录未读取/导出；既有Task/Run/Use/Resume通过回归。

## 验证与消费端

Go完整Race/Vet/格式/依赖/构建通过；Darwin arm64/amd64、Linux arm64/amd64、Windows amd64均可编译。当前macOS arm64实际二进制通过6类临时服务生命周期检查；R3确定性25项、安装兼容12项通过。4类Python HTTP消费画像、4项隐私策略与图像/工具/流/向量契约通过。没有真实模型额度消耗。

| 应用 | 本轮交付 | 仍阻断的真实验收 |
| --- | --- | --- |
| Worklog | 精确改点、可应用provider补丁（git apply --check通过）、非流/JSON画像 | 外部补丁应用、Rust/前端原生测试、日报/周报/模板/异常/Keyring真实流程 |
| GardenFlow | 双Source/能力UI/Pi/图像严格分支清单，SSE/图像/编辑画像 | 禁用原重试/删参/edit回退与敏感日志，实际Vision/封面入库/编辑/Pi工具流 |
| Chat Insight | 显式推理位置/授权/索引身份参考适配器、拒绝上传/批量向量/流测试 | 旧is_local仍需应用内修复；真实连接/索引/问答/确认UI/后台权限；未修复前不启用正式云路由 |
| Contribution OS | xcli_local受控Provider/冻结Profile/短期Token/URL边界清单，JSON/Tools画像 | 原安全套件/Plan/Review/Token/沙箱测试；容器Host Bridge未实现且不能开放LAN替代 |

外部仓库只读，没有修改其源文件、用户数据库、密钥或线上资源。协议画像不等于App端到端成功。

## 剩余限制与恢复条件

1. 真实API账号/模型和合成请求预算未提供/授权。文本、JSONSchema、Tools、Vision、生成/编辑/mask与Embedding均需独立真实验收；配置者的validated声明不能替代真实证据。
2. 外部四仓库写权限及其原生回归仍需具备。先检查Git状态/最新源码，再应用清单/补丁；不复制真实聊天数据库。
3. Windows/Linux/macOS x64实机、Windows ACL/进程、OS休眠/重启/启动项、真实升级/回退与性能基线未验证。Windows Agent后台Task/jobs明确不支持；Gateway后台独立，Windows也仅构建。
4. 图像仅验证base64输出；URL下载、Responses/音视频、容器Bridge和CLI会员登录桥接没有发布为已支持能力。完整JSON Schema语义仍由真实上游及消费端校验。

上述Gate全部补齐后才能将整体状态改为完成。当前没有Push、tag或Release；每阶段保留本地Git检查点。下一步从PROGRESS.md末尾的真实/外部验收项恢复，无需重建网关实现。
