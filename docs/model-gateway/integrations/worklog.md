# Worklog 接入状态

完整补丁：[worklog-provider-full.patch](worklog-provider-full.patch)及同名JSON侧车，共8文件。默认`xcli` provider为`http://127.0.0.1:18741/v1`、`xcli/text-default`、180秒，实际端口和别名可配置。

已实现独立应用Token沿用Keyring、切换provider时禁止复用旧凭据、严格loopback IP/端口路径、禁用环境代理和重定向、有界JSON与脱敏错误。保留日报/周报/模板prompt和无配置fallback。设置页增加授权目录检查，只GET /models，不触发推理，并显示local/remote/unknown。

隔离副本Rust40项和前端build通过。最终只读复核发现原仓库8项改动已与补丁逐字节一致，反向`git apply --check`通过；本轮没有重复应用。旧最小provider补丁不应再叠加。

真实日报、周报、模板、结构化JSON、异常、Token撤销和Keyring操作仍未验收；未调用付费模型。操作步骤见[补丁入口](README.md)，证据见[续轮验收](../acceptance/integrations-20261025.md)。

2026-10-26：可执行文件编译与Rust原生报告/模板/日报JSON经xcli的假HTTP上游联通通过；未安装/启动用户桌面程序或操作真实Keyring。见[最新记录](../acceptance/closeout-20261026.md)。
