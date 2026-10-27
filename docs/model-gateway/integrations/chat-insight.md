# Chat Insight 接入状态

完整补丁：[chat-insight-provider-full.patch](chat-insight-provider-full.patch)及同名JSON侧车，共15文件。Rust/React实现已在隔离副本验证，随后经权限审核通过并受保护写回15个文件，逐字节核对与测试副本一致。隔离副本可执行文件已构建，但未替换安装或重启用户桌面程序，正式云路由仍需在实际修复部署后启用。

已移植xcli provider、严格本机端点和独立Token切换检查。每次发送前授权读取/models，分别核对聊天/向量模型的真实推理位置；remote/unknown/缺失或非法目录不能绕过云端授权及本次确认，已确认请求才携带网关同意头。xcli关闭后台自动扫描，UI不以loopback声称本机推理。保留原SSE/取消及直连provider，xcli截断流不报成功。

Embedding身份包括endpoint、alias、dimensions、index_version和index_fingerprint。首次接入或身份变化要求显式重建，旧索引不会自动迁移；响应身份不符丢弃结果。敏感配置沿用Chat Insight原有私有文件存储，没有改读其他应用Keyring或登录信息。

Rust96项通过、2项真实模型评估忽略；React56项、build、lint通过。测试覆盖拒绝授权不上传正文、目录失败、显式同意、索引身份切换及UI。真实连接、批量索引、检索/问答、确认/拒绝和后台流程仍需部署后验收。

操作步骤见[补丁入口](README.md)，证据见[续轮验收](../acceptance/integrations-20261025.md)。

2026-10-26：Rust原生客户端经已构建xcli和假上游完成SSE、取消、向量身份及云同意门禁联通。真实模型请求0次，应用UI/真实索引仍未验收；见[最新记录](../acceptance/closeout-20261026.md)。
