# 能力与路由

模型 ID 是稳定 alias；上游名称和密钥引用不暴露给 API。独立模型注册中心与 Agent 注册中心互不替代。模型通过 backend/upstream_model 固定路由，不自动换供应商或重试计费请求。

能力：`text.generate`、`text.stream`、`vision.analyze`、`image.generate`、`image.edit`、`image.mask`、`image.multi_reference`、`embeddings`、`tool_calls`、`json_object`、`json_schema`。Vision 与图片生成分开，mask/多参考图必须单独声明。

`xcli` 元数据公开 kind、capabilities、supports_stream、inference_location（local/remote/unknown）、provider_family、verification、validated、verified_at、evidence、dimensions、index_version。验证来源为 declared/probe_verified/runtime_observed。后两类必须提供 RFC3339 时间与不含秘密的证据版本；它们是配置者的明确证据声明，网关不会把配置自动标为实测成功。

只有 text、stream、embedding 可按 declared 路由；图片、Vision、JSON、工具等强能力同时要求 validated=true。应在受控验证后填写证据。测试夹具证据仅对 Fake Backend 有效，不能复制为真实模型验收。

Chat Insight 应以每个所选模型的真实 inference_location 判断授权；unknown 与 remote 都视为上云。即使 localhost 网关也不能视为本地推理。Embedding dimensions/index_version 变化必须触发显式索引重建，禁止混用。
