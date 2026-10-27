# 桌面应用接入与验收

2026-10-26：四个应用的接入源码已写回原仓库，保留既有用户改动。四种实际原生客户端代码均已通过本机xcli与假HTTP上游联通；没有真实模型调用，也未替换安装或启动用户桌面程序。原生代码联通、实际UI验收和真实模型验收分别记录。

| 应用 | 已实现与验证 | 仍开放的真实Gate |
| --- | --- | --- |
| Worklog | xcli provider、独立Keyring Token、本机端点/目录检查；Rust40项、前端与可执行构建；原生报告/模板/日报JSON通过网关 | 真实日报/周报/模板与UI、异常/Keyring实际操作 |
| GardenFlow | 独立Source/目录位置/Pi能力前置检查；严格单次图片请求；780项/两类类型检查/生产构建；原生生成与参考图编辑通过网关 | 实际Vision/封面落库/Pi工具流、模型效果；未提供mask控件 |
| Chat Insight | 逐模型位置与云确认、后台限制、索引身份；Rust96项/React56项/build/lint/可执行构建；原生SSE/取消/向量与拒绝授权通过网关 | 实际UI确认、真实索引/检索；桌面隐私修复部署前不启用正式云路由 |
| Contribution OS | 注册的native xcli_local、公共HTTPS边界、冻结Profile与短期Token；原生JSON Schema联通/篡改Profile拒绝；全套1403通过/45跳过，更新模块504通过/28跳过 | 六服务后续ENV-R08已恢复完整运行时；认证桥接完整1460/46跳过、最新增量162/1跳过；本机/OrbStack合成验收通过，尚未写回/启用；真实Plan/Review和物理Linux Docker仍开放 |

交付入口：[四应用说明与补丁](integrations/README.md)、[2026-10-26验收](acceptance/closeout-20261026.md)、`tests/model_gateway/native_consumers.py`。旧Python消费画像仍保留，不能与实际应用UI验收混同。

真实smoke入口`examples/model-gateway/smoke.py`默认只读目录；显式`--execute --max-model-requests N`可能计费，需要已选定的上游、模型、凭据引用与预算。当前全部推理证据来自合成Fake服务。

后续由用户按[真实应用与实机验收交接](REAL_ACCEPTANCE.md)执行；[记录模板](acceptance/real-acceptance-record.template.md)按用例ID记录实际UI、模型与目标机证据，未测项继续保持开放。
