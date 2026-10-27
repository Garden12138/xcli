# Contribution OS接入状态

完整补丁`ai-open-source-contribution-os-summary-provider-full.patch`及JSON侧车共9文件。源码已受保护写回，保留用户修改和未跟踪依赖；未提交或推送。全套1403项通过/45项跳过，测试后更新的四个用户模块另经27个测试模块504项通过/28项跳过。

注册的native xcli_local只接受管理端登记的loopback IP/端口及/v1，设置页固定地址；普通供应商继续公共HTTPS校验。独立加密应用Token引用、冻结Profile、短期任务Token及审计保留。目录核验所需JSON/Tools能力后才发正文；无环境代理、重定向或自动重试。远程权限由应用Token显式授予，要求逐请求同意头的Token会明确拒绝。

原生Python客户端结合实际StudioGateway，经临时xcli和假HTTP上游完成JSON Schema联通；短期Token/冻结Profile正常，篡改Profile返回403，不发生legacy fallback。真实模型0次。

用户明确批准后，镜像xcli-integration-75d9cea7fbde已通过正常权限审核部署到六个现有服务。六服务均运行、重启计数0，API/Model Gateway/Publisher健康；实际HTTP健康和设置接口通过，200个部署源码文件与验收快照一致。原数据卷及127.0.0.1:8000绑定不变，Sandbox Worker网络仍为none，42个数据库结构源码一致。最新回退镜像ID已保存，docs/tasks.md已同步；之前缺少授权的审核拒绝已解决。

上述200文件部署后来发现遗漏运行依赖，应用仓库ENV-R08已恢复完整202文件并增加构建导入检查；旧部署覆盖与回退文件只作历史记录，不能覆盖修复。

Docker内native transport继续拒绝；新的独立`xcli_bridge`已在隔离副本实现并通过本机/OrbStack双向TLS合成验收，尚未写回或启用。[桥接补丁与验收](contribution-os-host-bridge.md)、[早前接入验收](../acceptance/closeout-20261026.md)。生产桥接、真实Plan/Review与异平台Gate保持开放。
