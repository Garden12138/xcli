# GardenFlow接入状态

完整补丁`garden-flow-provider-full.patch`及JSON侧车共14文件。已将最新用户修改无冲突合并并受保护写回，保留既有界面改版；780项测试、主进程/前端类型检查与Vite生产构建全部通过，前轮基线回归已消除。后续用户HEAD推进至7539d51（文档提交），不重复应用旧基线补丁。

独立xcli Source、应用Token、授权目录检查和上游位置提示已实现，文本/图片alias分别选择。目录能力元数据贯穿设置；Vision不从未知别名推断，Pi在发送上下文前要求经验证的Tool Calling及流式文本能力。

xcli图片走独立单次HTTP分支，不重试、删参、调整size/n、截断参考图或从编辑降级生成。拒绝返回URL下载，不记录上游错误正文；仅PNG/JPEG/WebP、最多4参考图/结果、20MiB单图与32MiB总预算。结果沿用媒体入库路径，暂无mask控件。要求逐请求同意头的Token会明确拒绝，客户端不自动补授权。

2026-10-26原生Node客户端代码经真实本机xcli/假HTTP上游完成目录、生成、参考图multipart编辑联通，每次图片操作仅提交一次。没有安装/启动用户桌面程序；真实Vision、封面入库、Pi工具UI和模型效果仍开放。[最新验收](../acceptance/closeout-20261026.md)，[补丁入口](README.md)。
