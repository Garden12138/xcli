# Contribution OS 认证宿主桥接

状态：**隔离副本中已实现并通过 macOS arm64/OrbStack 合成验收，尚未写回应用仓库或启用现有部署**。独立 provider 为 `xcli_bridge`；原来的 `xcli_local` 继续拒绝容器。完整补丁和前置摘要分别见 [13 文件补丁](contribution-os-host-bridge.patch)及[防漂移清单](contribution-os-host-bridge.patch.json)。

## 已实现的通道

Model Gateway 使用专用客户端证书，通过双向 TLS 连接只监听宿主 loopback 的 relay。客户端只信任登记的自签服务端叶证书；relay 同时验证客户端 CA 和精确叶证书指纹。普通供应商的公共 HTTPS/DNS/SSRF 校验不变。

冻结连接记录包含登记地址、安装实例 ID 和服务端证书 SHA256。更换实例或证书后，旧 Profile 失效，必须保存新的连接/模型版本。模型正文不能修改登记；密钥路径与私钥不进入 API、业务记录、审计或冻结 Profile。原有短期任务 Token 的 Profile/hash 绑定、过期与限次继续生效。

relay 仅转发 `GET /v1/models` 和 `POST /v1/chat/completions` 到一个固定的 HTTP loopback xcli 实例，保留独立应用 Token 鉴权。模型白名单、目录能力检查、4 MiB 请求、4/32 MiB 响应、最多 180 秒绝对期限、最多 8 并发与每分钟请求预算均有边界。无代理继承、重定向、自动重试或供应商 fallback；不执行工具、不读取原生 CLI 登录。

客户端配置与证书仅允许进程 UID 拥有的 Unix 私有目录/普通文件（0700/0600），不跟随证书文件 symlink。Windows ACL 检查尚未实现，会拒绝启用。Sandbox Worker/Runner 和 Publisher 不获得证书、Token、宿主 socket 或宿主网络。

## 已验证

[机器报告](../acceptance/host-bridge-20261026.json)记录了真实本机 mTLS 和临时 OrbStack 容器的完整链路：

`StudioGateway / 冻结 Profile / 短期 Token → mTLS relay → 已构建 xcli → 假 HTTP 上游`

- 本机与容器均完成 JSON Schema 调用；该次完整验收 4 次合成推理、0 次真实模型调用。
- 错误/缺失/过期证书、同 CA 但未登记客户端、错误实例/指纹、越权模型/路径、浏览器 Origin、超大请求/响应、速率/并发/绝对期限受测。
- Profile 篡改、过期任务 Token、超额重放被拒绝；实例/证书轮换令旧 Profile 失效；错误应用 Token 无法穿透 xcli 鉴权。
- 临时容器无已发布端口、host networking 或 Docker socket；只读挂载专用源码与合成客户端材料，限 CPU/内存/PID，测试后删除。204 个应用文件包含完整的 202 文件基线和两个桥接模块；静态内部导入检查通过。

复现（需要 Contribution OS 开发 Python 和预先存在、受信任的依赖镜像 ID）：

```sh
/path/to/contribution-os/.venv/bin/python tests/model_gateway/host_bridge.py \
  --source /path/to/isolated-bridge-source --binary /path/to/built-xcli \
  --docker-image sha256:<existing-local-image-id> --output /tmp/host-bridge.json
```

不传 `--docker-image` 只运行本机链路。该脚本创建自己的临时状态和证书，不接触现有应用数据库或凭据。OrbStack 官方文档只说明宿主连通机制；实际成功由本轮合成测试证明。[OrbStack 容器网络](https://docs.orbstack.dev/docker/network)

完整离线回归 **1460 通过 / 46 跳过**。后续独立的用户草案恢复改动已保留，最新审查副本的桥接/模型/Token/运行完整性/草案恢复回归 **162 通过 / 1 跳过**；桥接运行文件与完整回归版本一致。[回归证据](../acceptance/host-bridge-regression-20261026.json)

该最新源码还单独通过[本机链路复验](../acceptance/host-bridge-latest-native-20261026.json)，该次额外 2 次合成推理、0 次真实调用。完整容器验收所用本地依赖镜像随后被其他工作替换删除；未把后续无法启动镜像的复跑记为成功，也未把旧摘要冒充最新源码摘要。

补丁已在独立临时 Git 副本实际应用，13 个结果文件逐字节一致；原仓库前置摘要无漂移，dry-run 通过。尚未写回原仓库或修改部署。[补丁复核](../acceptance/host-bridge-patch-review-20261026.json)

## 尚待启用与真实验收

补丁包含应用侧部署说明 `docs/deployment/xcli-host-bridge.md` 和 CLI `contribos xcli-host-bridge --config …`。生产启用仍需完成可审查的实际证书登记、专用 Gateway 挂载/UID、实例、端口、模型白名单/预算、镜像与回退清单，并取得对应部署授权。此前六服务旧镜像上线的确认不覆盖新的证书/网络信任关系。

Linux amd64 物理 Docker、真实 Plan/Review 模型、目标异机与系统休眠/重启验收仍开放；本轮不替代它们。真实上游、模型、密钥引用和计费预算尚未提供，未发起真实请求。

## 已修复部署的保护

此前 200 文件快照遗漏了运行依赖，后续应用仓库 ENV-R08 已恢复完整 202 文件运行时并增加构建时导入检查。本轮以该完整源码为基线；快照工具也新增静态导入检查。旧 `xcli-integration-75d9cea7fbde` 镜像及旧 rollout/rollback 文件仅保留历史记录，不能用于覆盖当前修复。
