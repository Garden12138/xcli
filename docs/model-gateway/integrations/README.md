# 四应用接入补丁

完整补丁与同名`.patch.json`侧车共同交付。侧车记录源码HEAD、补丁摘要和每个源文件的前置摘要。历史验收见[续轮记录](../acceptance/integrations-20261025.md)，最新验收见[2026-10-26记录](../acceptance/closeout-20261026.md)，整体状态见[PROGRESS](../PROGRESS.md)。

2026-10-27交接：后续由用户按[真实验收步骤](../REAL_ACCEPTANCE.md)完成应用安装、实际流程、真实模型与目标机验收；本仓库提交补丁不等于替外部应用仓库提交、安装或部署。

| 应用 | 文件 | 当前状态 |
| --- | --- | --- |
| Worklog | `worklog-provider-full.patch`，8文件 | 源码已具备；原生业务HTTP联通、Rust/前端/可执行构建通过 |
| Chat Insight | `chat-insight-provider-full.patch`，15文件 | 源码已写回；原生SSE/向量/隐私门禁联通及构建通过；未安装桌面程序 |
| GardenFlow | `garden-flow-provider-full.patch`，14文件 | 源码已写回；780项、类型检查/生产构建、原生生成/编辑联通通过；旧基线回归已消除 |
| Contribution OS | `ai-open-source-contribution-os-summary-provider-full.patch`，9文件 | 源码已写回；1403/45跳过与增量504/28跳过；已获明确批准并完成六服务部署/健康检查；后续ENV-R08已修复运行依赖；认证桥接另有13文件隔离补丁/合成验收，尚未写回/启用，真实验收仍开放 |


旧的`worklog-provider.patch`仅保留为M4早期最小补丁证据；完整补丁包含它的功能，不应叠加使用。

## 复核与应用

当前四仓库均已应用完整接入，不要重复apply。以下命令用于精确基线的恢复/复现：先确认目标仓库的现有修改与HEAD，阅读应用说明和补丁。写入已授权后，从xcli仓库运行独立apply步骤，例如：

```sh
python3 tests/model_gateway/integration_workspace.py apply \
  --patch docs/model-gateway/integrations/chat-insight-provider-full.patch \
  --repository /path/to/chat-insight
```

该步骤核对HEAD、补丁与侧车摘要、实际修改路径集合和原文件摘要，先执行`git apply --check`，再执行本地apply；不提交、不推送、不启动服务、不调用模型。任何相关文件漂移都会拒绝写入，不能覆盖现有工作。发生漂移时应重新创建源码隔离快照、合并集成改动、运行原生检查并prepare新补丁。

生成新补丁的两个独立步骤：

```sh
python3 tests/model_gateway/integration_workspace.py snapshot \
  --repository /path/to/chat-insight --destination /tmp/my-chat-insight-integration
# 在destination/work中完成修改与原生测试；base保持不变。
python3 tests/model_gateway/integration_workspace.py prepare \
  --destination /tmp/my-chat-insight-integration \
  --output docs/model-gateway/integrations/chat-insight-provider-full.patch
```

源码快照保留脏文件及已引用的未跟踪源码依赖，排除数据库、环境文件、凭据、无关媒体、依赖目录和构建输出。原生测试只使用合成内容；真实上游/模型/凭据引用及预算应另行选定。补丁可应用不代表实际应用已完成联调。

认证桥接新增补丁及防漂移侧车见[contribution-os-host-bridge.md](contribution-os-host-bridge.md)，不要把它与已写回的native provider完整补丁混用。
