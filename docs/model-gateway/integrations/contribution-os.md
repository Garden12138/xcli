# Contribution OS 接入修改清单（未联调）

只读复核 `app/model_settings.py`、`app/providers/{studio,gateway_http}.py`。PublicHTTPSClient和validate_base_url当前仍拒绝本机/私网；StudioGateway仍验证冻结Profile和短期作用域Token。**保持这些约束**。

获写权限后的实现清单：

1. 独立`xcli_local` connection类型，管理端显式选择受信任安装实例。普通openai_compatible仍使用原有公共HTTPS校验。xcli_local端点只能取管理端注册的loopback IP/端口和/v1；禁止hostname、userinfo、query、redirect和任意路径；密钥来自独立credential_ref，不写入冻结快照。
2. `ProfileUpstream`仅在provider=xcli_local时选择受控本地transport，其余仍PublicHTTPSClient。目录/请求遵守冻结model alias、max_tokens、JSONSchema/Tools能力验证和超时。保留StudioGateway Token验签、任务/配置绑定、限次、失效检查、审计与模型输出验证。
3. Worker在容器时127.0.0.1不等于宿主；禁止为兼容而把xcli绑定LAN或对sandbox开放网络。需另行获批、固定目的地址并验证身份的UDS/Host Bridge。当前没有实现或验证容器Bridge，容器端接入阻断。
4. 参考`examples/model-gateway/client.py::local_base_url`和smoke的Contribution OS画像；只证明Provider HTTP面，不替代原项目签名Token/Profile冻结/沙箱网络测试。

可执行离线协议：`go test -race ./tests/model_gateway -run TestDesktopConsumerProfiles/contribution-os -count=1`。严格JSON参数、工具schema/id/arguments和finish_reason由Fake Backend验证。`TestProviderSwitchRequiresExplicitAliasAndNoFailureFallback`验证不在失败后自动换供应商。

原仓库验收待执行：Plan/Review结构化输出、冻结Profile/历史记录兼容、短期Token过期/重复/越权、故障回滚、日志脱敏、非受信任localhost/私网URL仍拒绝、沙箱无法接触xcli、Host Bridge目的地址固定。没有修改线上资源、发布、迁移数据库或替换原有Gateway。
