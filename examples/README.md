# 配置与历史示例分类

当前使用入口：[本地交付手册](../docs/release-handoff/USER_MANUAL.md)。默认配置无需本目录文件。

| 文件 | 处置与适用范围 |
| --- | --- |
| config.yaml | 保留 version 1 完整字段兼容示例；包含旧 routing/ACP 和示例代理，不宜直接覆盖个人配置；最小当前配置见安装手册 |
| project-mcp.yaml / native-mcp-claude.json | 既有 MCP launcher/原生配置参考，不自动写入原生配置 |
| implement-and-review.yaml / parallel-review.yaml | 历史 YAML Workflow runner 夹具/示例；当前公开 workflow 命令已退役，使用 task；保留内部兼容与测试引用 |
| model-gateway.yaml / model-gateway/ | 冻结 Model Gateway 历史示例，当前不启用、不配置真实密钥 |
| native-agent-gateway.yaml / codex-limited-gateway.yaml | 冻结 NAG/limited 实验示例，不是正式 CLI 使用步骤 |

示例与历史补丁不因目录陈旧而删除；不按旧命令自动恢复实验。原生账号、权限、模型与额度由对应 CLI 自己维护。
