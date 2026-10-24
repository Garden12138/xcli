# GardenFlow 接入修改清单（未联调）

本轮只读核对 `desktop/electron/core/{aiModelRouteResolver,imageProviderAdapters}.ts` 与 Pi Agent。**发现方案之外的实际差异**：当前 openai-images 有最多4轮重试、移除不支持参数、调整size、edit→generation回退，以及SDK错误日志中记录body。xcli接入不能直接套用这些路径。

需在获写权限后实施：

1. 增加明确provider身份`xcli`，独立text与openai-images Source；endpoint来自用户安装/状态发现（默认示例18741可编辑），Token用原有安全存储。不要将原生Agent订阅当API。
2. `aiModelRouteResolver.ts`：从授权models目录解析能力/验证来源/推理位置，文本、Vision、图像、Embedding独立路由。Pi Agent需要verified tool_calls，写作模型不足时禁用Agent入口。UI区分本机传输和真实推理位置。
3. `imageProviderAdapters.ts::openAiAdapter.generate` 增加xcli专用分支。先检查目录的validated/capabilities；最多4图且不截断输入；edit缺参考图/缺image.edit/multi_reference/mask时拒绝，不切文生图。response_format固定b64_json或上游原生base64，禁用URL优先、参数删减、尺寸自动变化、SDK自动重试与edit回退。SDK client对xcli设置maxRetries=0；失败只显示本地稳定错误，不记录prompt/body/API Key。
4. 基本调用保持model/prompt/n/size与multipart image[]；xcli的4MiB Vision JSON、20MiB单图/32MiB总量/16M像素约束必须在发送前提示。返回图片经原有媒体库保存为实际资产，不能将普通文本当图片。
5. 图像/数据确认与原有媒体权限保持；流取消关闭HTTP响应；structured JSON与Tools按所选模型能力启用。旧provider行为和配置继续回归。

可执行协议验收：`go test -race ./tests/model_gateway -run TestDesktopConsumerProfiles/garden-flow -count=1`，启动真实本地HTTP网关并使用Python客户端消费文本SSE、图像base64和参考图编辑；**没有启动GardenFlow**。

真实验收清单：文章/续写、中文JSON、长文取消、Vision、封面入媒体库、参考图/mask、Pi tool流、禁用不具备工具能力模型、Token拒绝、无重试/降级/敏感日志。发布Gate仍未通过。
