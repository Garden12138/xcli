# xcli Model Gateway（开发候选）

Model Gateway 与 Agent Runtime 同时存在：`task/run/use/resume` 保持原用途；`model` 提供受控模型HTTP API，不启动Coding Agent或复用会员登录。

**当前仅离线工程验收完成。真实模型、四个桌面应用与异平台实机Gate仍开放，不能作为已正式发布/完整集成的版本。** 状态见[PROGRESS.md](PROGRESS.md)，合同见[api-contract.md](api-contract.md)，每阶段证据见acceptance目录。

后续实际操作从[真实应用与实机验收交接](REAL_ACCEPTANCE.md)开始，覆盖四应用UI、真实上游、Docker桥接、目标实机和升级回退；使用[验收记录模板](acceptance/real-acceptance-record.template.md)记录结果。提交/推送不代表这些待验项完成。

## 构建和配置

Go 1.26：`go build -trimpath -o xcli .`（Windows使用`xcli.exe`）。复制 `examples/model-gateway.yaml` 为用户自己的私有 `model-gateway.yaml`，填写实际endpoint、model和显式API Key引用。示例.invalid不可访问；不要把密钥写到YAML或argv。上游支持兼容协议并不等于其所有模型都支持全部功能。

```yaml
version: 1
server:
  host: 127.0.0.1
  port: 18741 # 0由OS分配，客户端读取model status
  max_concurrent_requests: 3
  timeout_seconds: 180
backends:
  local:
    type: openai-compatible
    endpoint: http://127.0.0.1:11434/v1
    inference_location: unknown # 核实该服务确实本机推理后才设local
models:
  xcli/text-default:
    backend: local
    upstream_model: user-selected-model
    kind: text
    capabilities: [text.generate, text.stream]
```

后端类型：openai-compatible、openai-images、openai-compatible-embeddings。凭据引用为`env:NAME`或`file:PATH`（相对配置目录解析）。只读取显式引用，不读原生CLI登录。远程必须HTTPS；HTTP只接受loopback IP。上游不继承HTTP_PROXY，不重定向/重试。

图片模型设置kind=image，按验证结果声明image.generate/edit/mask/multi_reference；Vision在kind=text模型声明vision.analyze。强能力要附verification=probe_verified/runtime_observed、RFC3339 verified_at及不含秘密的evidence版本；它是配置者的验收声明，不能复制Fake证据冒充真实通过。Embedding设置kind=embedding、capabilities=[embeddings]、dimensions和index_version，指纹变化要求应用显式重建索引。

## 启动、身份与停止

以下是用户配置后自行执行的操作，不是本轮对真实服务的调用记录。

```bash
xcli model doctor --config /private/path/model-gateway.yaml --no-billable-requests
xcli model list --config /private/path/model-gateway.yaml
xcli model start --config /private/path/model-gateway.yaml
xcli model status
xcli model client issue worklog --config /private/path/model-gateway.yaml \
  --model xcli/text-default --token-file /private/path/worklog.token --allow-remote
xcli model client list
xcli model client rotate worklog --token-file /private/path/worklog-new.token
xcli model client revoke worklog
xcli model logs
xcli model stop
```

`--allow-remote`明确允许该App访问remote/unknown推理，不需要时省略。Chat Insight身份强制逐请求`X-Xcli-Remote-Consent: allow`，并且必须先完成其应用内隐私修复；拒绝授权不带头，网关在上游调用前拒绝。其他App也可以--require-consent。Token不打印；文件交付后应转入App原有安全存储，按用户自身流程清理交付文件。不能用上游API Key作为本地App Token。

`model serve`为前台，Ctrl-C取消所有请求。`start`后台启动，同一state-dir幂等；`status`依据OS文件锁显示active/实际address/健康reachable，不信任旧PID；`stop`写绑定随机instance_id的私有控制请求并等待退出，重复停止安全。配置变化需stop→start，不偷偷换路由。端口占用明确失败；port=0可避免冲突。每个安装实例用同一个明确`--state-dir`，不同用户/实例不要共用Token。

私有目录默认`${XDG_DATA_HOME:-~/.local/share}/xcli/model-gateway`。Unix目录0700/文件0600；Windows新建状态根使用当前用户+SYSTEM受保护DACL，已有不安全目录/凭据文件会拒绝。Windows交付Token文件建议位于该私有状态目录，再转入客户端Credential Store。未知存储schema拒绝，升级不会自动迁移或删除旧Task/Run。

## 安装、升级、卸载

当前从源码构建，未发布本升级安装包。构建成功后将二进制放到用户PATH目录；macOS/Linux赋予执行权限，Windows使用.exe。无需管理员/系统服务。交叉构建例：`GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o xcli.exe .`。CI新增Windows x64与Darwin/Linux双架构编译；native网关检查只在CI实际执行后才可算目标机证据。

升级：保存旧二进制→用旧版`model stop`→替换二进制→doctor→start→status→授权models目录检查。需要回退时停止新版并恢复旧二进制；未改配置/凭据schema，不自动迁移数据。卸载：model stop后删除自己安装的二进制；默认保留独立状态和Task/Run数据，Token撤销/私有数据删除由用户显式决定。不会自动清除原生CLI安装或登录。

可选开机启动：让用户自己的launchd/systemd用户单元或Windows任务计划执行绝对路径`xcli model serve --config ... --state-dir ...`；只运行当前用户、loopback、无密钥argv。不自动注册启动项；不要同时使用OS守护重启策略和model start。实际重启/休眠恢复尚未验收。

## 诊断和验证

`requests.jsonl`仅含随机request_id、白名单route、client_id、状态/错误类别/耗时；2MiB轮转+一份备份，无prompt/图像/Token/上游正文。service.log记录启动/停止、实例ID、版本和配置摘要，用于追踪路由配置变化，按2MiB保留预算；不保存实际密钥或配置正文。日志写入失败会取消服务，避免静默失去诊断。没有遥测。/healthz只证明进程；/readyz/doctor不证明真实上游额度或模型可用。

```bash
go test -race ./...
go vet ./...
go mod tidy -diff
python3 -m unittest discover -s tests/model_gateway -v
go build -o /tmp/xcli-gateway .
python3 tests/model_gateway/lifecycle.py --binary /tmp/xcli-gateway
```

测试使用Fake/合成材料，依赖Python3执行消费画像。真实Provider smoke入口`examples/model-gateway/smoke.py`默认只读catalog；--execute与--max-model-requests N才发请求，可能计费，必须先获授权。四应用的接入源码均已写回；原生客户端离线联通、构建及剩余部署/真实验收见[integration-matrix.md](integration-matrix.md)。

Windows构建不代表完整Agent Runtime支持：foreground进程实现使用Job Objects，实际CLI/取消语义未验收；原有基于Unix描述符传递的Agent后台Task/jobs在Windows明确拒绝，Model Gateway后台管理独立可编译但尚未实机验证。macOS/Linux原有运行路径保持回归。CLI会员文本桥接、URL图片下载、Responses/音视频和容器Host Bridge均没有作为已支持能力发布。


四个原生客户端的离线HTTP联通（先复制源码，Rust探针仅注入临时副本）：

```bash
python3 tests/model_gateway/native_consumers.py \
  --binary /tmp/xcli-gateway --sources-root /path/to/checkouts \
  --contribution-python /path/to/contribution-os/.venv/bin/python \
  --worklog-target /tmp/xcli-worklog-target --chat-insight-target /tmp/xcli-chat-target \
  --output /tmp/native-consumers.json
python3 tests/model_gateway/performance.py --binary /tmp/xcli-gateway --output /tmp/gateway-performance.json
```

这些入口只启动临时loopback网关和假上游，使用合成文本、PNG、向量及临时Token；不读取应用数据库、真实Keyring或原生CLI登录。需要已安装的Rust/Node/Python依赖，Rust使用offline模式。性能结果包含本机客户端/假服务开销和采样误差，不能推断真实模型速度或其他机器表现。

Contribution OS另有默认关闭的认证宿主桥接隔离实现，真实mTLS本机/OrbStack合成链路通过，尚未写回或启用生产配置；见[桥接补丁与验收](integrations/contribution-os-host-bridge.md)。这不改变原生transport拒绝容器的行为或公共HTTPS边界。
