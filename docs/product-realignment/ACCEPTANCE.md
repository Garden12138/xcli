# R4 验收

基线 `a3e258e`，开始时工作区干净。当前 `0.5.0-r4-dev` 是未发布源码候选。

## 已完成调整

- 主 README/使用手册/总体架构以统一 CLI 调度与 Task 编排为中心；新增八份产品定位文档。
- `model`、`agent-gateway` 从主帮助隐藏；服务、探针、授权创建/轮换等默认拒绝，保留查询/停止/撤销/disable。实验 flag 只用于兼容历史工程，未启用任何真实 Profile 或 SIWC。
- 没有删除已有包/Adapter、配置 schema、Task/Run/Jobs、Session、工作区、媒体文件或历史证据。后台 model start 显式传递实验选项，避免隐式环境开关。
- 核心 CI/打包转向 CLI/Task 回归；冻结实验另设手动 workflow。仍有源码构建耦合，未假装已经分库。
- 四应用当前方案统一为明确授权的 Agent Task，保留旧设计/补丁为历史参考，不修改业务源码。

## 本轮验证

定向 `TestR4*` 与原 Gateway 客户端契约通过：默认关闭在 I/O 前生效、主帮助聚焦核心、收尾入口保留、显式实验选择不等于真实授权、Doctor 不执行 legacy login、旧配置字节未变。

冻结源码后，以下两条检查都已实际完成，均为 **exit 0 / PASS**：

```bash
bash tests/product_realignment/verify-offline.sh
bash tests/product_realignment/verify-core.sh
```

完整回归保留全量 Go race/vet/build、R0/R3/配置/历史查询、冻结模型/HTTP 契约、Python、假上游生命周期及跨平台构建；核心回归独立验证不要求 Gateway 配置/真实就绪。跨平台构建和合成 fixture 不等于所有系统/厂商 CLI 实机可用。

| 检查 | 本轮结果 |
| --- | --- |
| 全量 Go | `go test -race -p 1 -count=1 -timeout 240s ./...`、`go vet ./...` 全通过，含新增 R4 默认关闭/恢复/认证职责测试。 |
| 核心独立 Go | CLI/Task 核心包 race/vet 全通过；三个历史 Gateway CLI 测试单列到全量回归，新 R4 边界测试仍在核心门禁。 |
| 配置与安装 | 旧配置字节保护、legacy auth_args 不执行、现有 schema/Run/Task/Jobs 回归通过；两轮安装兼容各 12 项通过。 |
| 完整 Python | R0 11、Native Gateway 9、R3 25、Model Gateway 9（54 项）全通过。 |
| 核心 Python | R0 11、R3 25（36 项）独立通过。 |
| 既有实验保留 | 假进程控制面、真实 loopback 假模型生命周期和 Native Profile 拒绝通过；未削弱旧隔离/资格门禁。 |
| 构建 | 本机及 Darwin amd64/arm64、Linux amd64/arm64、Windows amd64 构建通过。 |
| 其他 | go mod tidy -diff、gofmt、diff check、R4 活跃文档本地链接检查通过；go.mod/go.sum 无变化。 |

证据：[全量日志](evidence/full-regression.log)、[核心日志](evidence/core-regression.log)、[机器汇总](evidence/validation.json)、[模块依赖审查](evidence/module-audit.json)、[源码清单](evidence/source-manifest.json)。CI workflow 已分离配置，本轮没有 Push，因此不宣称 GitHub 远程任务已运行。

安装检查使用隔离临时目录，`historical_tasks=0`；旧数据兼容由源码未改、配置/存储/续接夹具和查询测试支持，没有扫描本人实际历史任务或原生登录目录。旧 R3 harness 的 “real read-only invocation” 文案在此为注入 PATH 的 fake Agent，workspace 测试的 Worklog 也为临时假仓库，不是实际应用操作。没有把这些计数当作真实供应商调用。

## 最终六项回答

1. 产品定位是统一 AI Coding Agent CLI 调度与编排，原生 CLI 自管认证、模型与额度。
2. 核心是 Discovery/Doctor、Adapter、Task/Workflow、Session/Run/Jobs、监督、真实产物、配置日志审计；Gateway HTTP、SIWC/OAuth/Responses、Credential Manager 和通用媒体服务冻结。
3. R0–R3 核心实现全部保留，并在本轮做离线回归；真实可用受既有支持矩阵、原生安装/授权/权限约束。不能声称全部 Adapter/角色/平台已真实通过，Windows 后台仍不支持。
4. 保留 journal/CAS/幂等/预算、OS 文件/进程、事件/崩溃测试、安全 FD 导入和图片字节校验供审查复用；OAuth/模型 HTTP 仅归档，不推进。
5. 使用 `run --agent codex|cursor|antigravity`、`use`、`resume` 和有界 `task`；详见[当前手册](../cli-first/XCLI_USER_MANUAL.md)。不要启用 Gateway 来运行 CLI。
6. 下一步优先可审查阶段计划/显式角色绑定、统一任务时间线/恢复预览、统一真实产物验收；见[路线图](TASK_ORCHESTRATION_ROADMAP.md)。

本轮真实模型调用、原生登录/配置修改、四应用修改、部署、Push 均为 0。M0–M5/G1–G8 原始证据不覆盖、不重写；不重启 G8.1 或 G7 实验。最终本地提交以包含本文与验收清单的提交定位，避免自引用 SHA。
