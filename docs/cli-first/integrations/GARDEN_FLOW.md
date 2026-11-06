# GardenFlow 接入适配（Planned / Blocked）

2026-11-06，只读核对 HEAD `33b59295888f5cd34abab902182c08cf6f5442b7`，初始干净。没有改源码、运行 UI、导入媒体或调用模型。

**CLI 文本、Vision、生成/编辑图片接入均 BLOCKED。** 现有 xcli 图片 provider 调用 `/v1/images/*` 的 HTTP 上游路径；不能用三个 Agent 的名字填 model 参数，宣称已用会员原生图片能力。

## 当前边界

已核对 `desktop/electron/core/aiModelRouteResolver.ts`、`xcliGateway.ts`、`imageProviderAdapters.ts`，并定位 `imageGenerationService.ts` 与 `desktop/electron/pi/agent.ts`。当前 xcli transport 只允许 loopback `/v1` 和有限 routes，校验能力目录；图片 Adapter 期待有效 `b64_json` 并限制大小。它不理解 Agent 会话、工作区审批或原生文件路径。

[历史三 CLI 媒体验收](../../r0/media-acceptance.zh-CN.md)有原生工具、文件与视觉证据，本轮三种 CLI 的生成/编辑仍为 NOT_TESTED。不能把历史 JPEG/PNG 复制进本轮目录再记成功。

## 拟议用户流程

1. 用户选择独立的“原生 Agent 任务”入口，显示 CLI 版本、推理位置、已验证能力及预算。未知能力默认关闭，不复用普通 HTTP 图片配置。
2. 文本或 Vision 仅提交本人批准的合成文本/图片；Vision 是识别图片输入，不能因此开放图片生成。
3. 生图/编辑任务在独立工作区执行。编辑只导出明确选中的参考图；原图只读，不允许 Agent 扫描媒体库或读取未选文件。
4. 原生工具完成后，xcli 验证真实文件并通过认证 Artifact 下载交付。应用检查字节、MIME、尺寸、摘要及允许格式，再预览并由用户决定入库。
5. 保存成功后创建应用自己的媒体 ID 与文件引用；临时原生路径不是持久媒体引用。失败、权限拒绝、文件校验不通过均不创建成功资产。

以上流程依赖[尚未实现的 Agent/Artifact API](../INTEGRATION_ARCHITECTURE.md)。当前没有可运行的 endpoint 配置或自动入库命令。

## 各能力的门禁

| 能力 | 必须单独证明 | 不可推导 |
| --- | --- | --- |
| 文本 | headless/会话、独立标记、权限/取消 | 有 Agent 安装即能写入应用数据 |
| Vision | 同意后的真实图片输入、正确识别合成视觉标记 | 能生成图片即能理解任意图片 |
| 文生图 | 使用原生图片工具，得到可解码真实字节 | exit 0、回答路径、代码绘图即成功 |
| 参考图编辑 | 原图真实送入工具，原图摘要不变，目标视觉修改成立 | 只参考文件名或重新独立生图即编辑成功 |
| mask/多参考图/尺寸/n | 参数真实传递并验证产物 | 单张图片成功即兼容完整 Images API |
| 工具调用 | Pi/应用明确批准的工具、权限、审计 | Agent 内部工具等于应用 tool_calls |

保留原生 JPEG/PNG/WebP，文件扩展名取真实格式；不强制转码来满足虚构合同。Artifact 下载有字节/像素/数量上限、摘要核对与认证。禁止按模型返回路径任意读宿主文件，禁止未登记 URL 下载或放宽 SSRF。参考文件不覆盖，导入失败不删除用户原图。

视觉验收既看差异也看目标，例如“只把花盆改蓝，植物/背景保留”；仅 SHA 不同或全图噪声变化不算编辑语义通过。并行生成、批量任务、mask、媒体取消/晚到产物均需另行预算，不从本轮每 CLI 1+1 推导。

## 待确认模块变更

| 优先级 | 变更 |
| --- | --- |
| P0（xcli） | 原生媒体真实验收、独立 Agent API、安全 Artifact 服务及进程/云任务取消边界 |
| P1（GardenFlow） | `aiModelRouteResolver.ts` 区分 HTTP model 与 Agent task；新增能力门禁，不虚构 model alias |
| P1（GardenFlow） | `xcliGateway.ts` 旁新增独立 Agent transport；保留原来的 `/v1` URL/route/SSRF 限制，不直接扩大白名单 |
| P1（GardenFlow） | `imageProviderAdapters.ts`/`imageGenerationService.ts` 增加任务→认证文件→媒体导入的状态机；不将路径塞进 b64_json |
| P2（GardenFlow） | 媒体库存储/IPC/预览接入内容摘要、来源任务、入库确认及失败清理；实际模块改造前重新定位 |
| P2（GardenFlow） | `pi/agent.ts` 接入审批、客户端权限与预算；原生工具不得继承 Pi 的全部读写/网络权限 |

客户端 Token 由 Electron 主进程的安全存储持有，renderer 只得到能力和任务状态；不在渲染层保存原生凭据、Token 或任意产物绝对路径。Agent 产物下载授权只作用于本应用/本任务。

断连只查询/重放，不自动重复生图。用户取消后晚到图片保持隔离，待确认才入库；不能显示已取消却自动提交文件。工具审批拒绝保持 BLOCKED，不添加 force/yolo 或修改全局 trust。

## 后续验证（未执行）

- [ ] 未验证/无 Token/拒绝上云/越界输入时无原生启动与文件读取。
- [ ] 原生文件、伪图、错误 MIME、超大像素、symlink、路径越界和重复 Artifact 的处理。
- [ ] 每个 CLI 各一张生成图和一张编辑图的真实工具与字节/视觉证明；不重试补成功。
- [ ] 媒体导入失败、重启后引用、取消/断连/晚到图片均不会覆盖或重复入库。
- [ ] 在另行批准后才运行 GardenFlow 桌面 UI 和真实业务材料。
