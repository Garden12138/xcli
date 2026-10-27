# Model Gateway升级交付总结

2026-10-26：本仓库M0–M5工程实现与四消费端接入源码均已交付；四应用实际原生客户端代码的离线HTTP联通通过。**整体真实验收仍未完成**，不能作为正式发布、模型效果或实际应用UI验收声明。恢复入口为[PROGRESS.md](PROGRESS.md)，最新证据为[closeout-20261026.md](acceptance/closeout-20261026.md)。

2026-10-27：用户接手后续真实验收。[操作步骤与通过标准](REAL_ACCEPTANCE.md)、[验收记录模板](acceptance/real-acceptance-record.template.md)已整理；本仓库实现、接入补丁和证据按用户授权集中提交推送。实际应用安装、桥接启用和真实调用不在本次提交操作中执行。

## 已实现

独立Model Gateway，不启动Agent/Task或继承执行工具权限。支持本机受认证的Chat/SSE、经能力门控的JSON/Tools、Vision内联图像、图片生成/编辑及Embedding透传；固定alias路由、无隐式fallback、无自动收费重试。具有独立App Token、模型/位置权限、Chat Insight逐请求云同意、边界检查、图片/向量校验、预算/取消/脱敏审计、跨平台编译与本机生命周期管理。CLI会员桥接、图片URL下载、Responses/音视频和容器Host Bridge不作为已支持能力。

## 消费端交付与检查

| 应用 | 源码与离线验收 | 尚未完成 |
| --- | --- | --- |
| Worklog | 8文件已在原仓库；Rust40项、前端/可执行构建；原生报告/模板/日报JSON经xcli联通 | 实际UI、Keyring操作与真实模型流程 |
| Chat Insight | 15文件已写回；Rust96项/React56项/build/lint/可执行构建；原生SSE/取消/向量/云确认与索引门禁联通 | 桌面修复安装与真实UI/索引检索；未部署前不启用正式云路由 |
| GardenFlow | 14文件已写回并保留用户改版；780项、两类类型检查、生产构建；原生目录/生成/编辑联通 | 实际Vision/媒体入库/Pi工具流与模型效果；暂无mask控件 |
| Contribution OS | 9文件已写回；冻结Profile/短期Token/JSON Schema原生联通；全套1403通过/45跳过，更新模块504通过/28跳过 | 六服务已部署且健康；认证桥接已在隔离副本合成验收，尚未写回/启用；真实Plan/Review仍待验收 |

四应用原生联通共10次假上游模型请求，未经确认的云上传/未批准的向量上传均0次，日志Token泄露0项；没有真实模型请求，没有安装或启动用户桌面程序。旧Python协议画像继续保留为另一层证据。

macOS arm64本机生命周期、独立Linux arm64禁网容器六类生命周期、延迟/RSS基线通过。假上游小文本单客户端p95为3.915ms、三客户端8.003ms；合成PNG负载采样峰值RSS约102.5MiB。这不是实际模型性能或目标物理机保证。Go完整Race/Vet/跨平台构建沿用M0–M5历史记录，本轮没有Go实现变更。

## 部署历史与待验项

用户明确批准后，Contribution OS旧镜像`xcli-integration-75d9cea7fbde`曾部署至六服务，当时的健康、200文件摘要和原数据卷检查通过。该快照后来发现遗漏运行依赖，应用仓库ENV-R08已恢复完整202文件运行时。旧镜像及其rollout/rollback只作历史记录，不能作为当前部署或回退方案。

认证Host Bridge已在隔离副本实现并通过本机/OrbStack合成验收，4次假推理、0次真实调用；生产证书/挂载与启用仍待独立审查。另待真实上游/模型/密钥引用与预算，以及Windows/Linux x64/macOS x64目标实机。Docker内native接入继续明确拒绝，未放宽公共HTTPS、LAN或Sandbox Worker边界。实际UI/模型、异机、OS休眠/重启与真实升级回退Gate保持开放。

本次由用户授权提交推送xcli交付材料；外部四仓库未代为提交/推送，没有tag或Release。此前未提取真实凭据或原生CLI登录、未读取聊天/工作日志数据库内容；使用既有Compose完成过经授权的服务切换，未引入新数据库迁移。

后续ENV-R08已修复上述200文件部署遗漏，恢复完整202文件运行时；旧镜像及其rollout/rollback记录不得作为当前回退方案。桥接补丁基于完整源码，见[桥接实现与验收](integrations/contribution-os-host-bridge.md)。
