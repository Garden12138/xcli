# CLI-first 验收报告

日期：2026-11-06（Asia/Shanghai）。**整体 BLOCKED，未完成真实能力验收，不能宣称三个 CLI 或四应用整体通过。**

## 基线与工作区

| 项目 | 本轮事实 |
| --- | --- |
| xcli HEAD | `f529e047ca03ef749e3ac49abe146f2ef4b46eab` |
| 机器 | macOS 26.5.1 / arm64 |
| Go | 1.26.4，模块要求 1.26.0 |
| 本轮源码构建 | `xcli version 0.5.0-r3-dev`，不是仓库已有二进制或发布版的验收 |
| 初始 Git 状态 | 仅 `?? xcli-cli-first-acceptance/`；该用户提供目录保留原样 |
| 范围 | 本仓库验收脚本修复、证据与文档；四消费仓库仅只读核对 |

已核对 README.zh-CN、`internal/agent` 的适配、探测、解码、用量及相关测试、`cmd/model*.go`、`internal/modelbackend`、`internal/modelstore/config.go`、历史媒体报告、Model Gateway 文档及 R0 smoke。历史计划中的“尚无 HTTP 网关”已不适用于当前源码；当前已有 HTTP 上游网关，但仍无原生 CLI 后端。

## Gates

| Gate | 状态 | 能证明的范围 |
| --- | --- | --- |
| Phase 1 离线门禁 | PASS | 源码 test/race/vet/build、安装与 doctor、假上游生命周期 |
| A：原生 CLI → xcli run/resume 文本 | NOT_TESTED | 预算确认仍待回复，真实推理启动 0 次 |
| A：原生图片输入/生成/编辑 | NOT_TESTED | 额外媒体预算待确认，真实媒体启动 0 次 |
| B：CLI → `/v1/*` | BLOCKED | 配置明确拒绝 native CLI backend，命令构建只创建 HTTP backend |
| C：四应用使用 CLI 能力 | BLOCKED | 依赖尚未实现的 Agent 接口；本轮仅设计 |
| 四应用 UI、部署、异机、Mac 真实重启 | NOT_TESTED | 本轮没有运行或授权这些操作 |

PASS 表示在注明环境真实执行且满足检查；FAIL 表示执行后不满足；BLOCKED 表示缺实现或前置条件；NOT_TESTED 表示未执行。登录状态、帮助标志及历史记录不提升为本轮推理 PASS。

## 实际执行与证据

先运行原始要求的命令：

```bash
bash xcli-cli-first-acceptance/verify-phase1.sh --race
```

第一次退出 1，受默认 Go 缓存权限限制；改用临时 `GOCACHE` 后仍退出 1，HTTP 测试夹具因沙箱禁止监听 loopback 而失败。分别保留[初始日志](evidence/phase1-sandbox-initial.log)与[临时缓存日志](evidence/phase1-sandbox-cache.log)，没有把它们计为通过。

获得执行环境的精确命令权限后，在宿主权限下运行：

```bash
env GOCACHE=/tmp/xcli-cli-first-go-cache GOPROXY=off GOSUMDB=off \
  bash xcli-cli-first-acceptance/verify-phase1.sh --race
```

退出 0，[完整脱敏日志](evidence/phase1-offline.log)。`GOPROXY=off/GOSUMDB=off` 使用本机已有依赖；不发模型请求。

| 检查 | 状态 | 限定 |
| --- | --- | --- |
| `go test ./...` | PASS | 包含确定性原生协议/进程夹具；部分包使用 Go 测试缓存 |
| `go vet ./...` | PASS | 静态检查 |
| `go build -trimpath -o "$BINARY" .` | PASS | 临时新二进制，未覆盖仓库 `./xcli` |
| `go test -race ./...` | PASS | 全包命令；Task 包 165.147 秒，其余部分缓存 |
| `xcli --version`、`agents --json` | PASS | 发现安装与声明能力，不验证推理 |
| `doctor codex/cursor/antigravity --json` | PASS | 帮助探测，不验证额度/权限执行 |
| `tests/model_gateway/lifecycle.py` | PASS | 1 次本机假上游 HTTP 请求，0 次原生 CLI 推理 |
| 修复后的 smoke 离线回归 | PASS | 4 项，子进程/探测均为测试替身，0 次模型调用 |
| `--native-smoke` | NOT_TESTED | 未收到明确的本轮调用额度回复，尚未执行 |

生命周期覆盖并发 start 单实例、端口 0、真实本机 HTTP、独立 Token 轮换/撤销、改配置须重启、stop/重复 stop/restart/陈旧停止隔离与端口冲突。其 HTTP 服务会自动清理，没有安装常驻服务。

[预检查](evidence/preflight.json)确认 Codex 0.167.0、Cursor 2026.10.22-27dcf5f、Antigravity 1.5.2。Codex `login status` 报告已有 ChatGPT 登录；Cursor 在沙箱内报告未认证，宿主权限下 `status --format json` 为已认证，因此不能将沙箱结果写成账号失效。没有调用登录、注销、切换账号，也没有打开凭据文件。Antigravity 登录有效性未验证。

帮助调用耗时仅记录本地启动与帮助解析；不作为模型首 token、冷启动延迟或速度比较。

## Gate B 最小复现

`internal/modelstore/config.go` 只允许 `openai-compatible`、`openai-images`、`openai-compatible-embeddings`。`cmd/model.go` 只实例化 `modelbackend.NewOpenAI`；没有调用 Agent Runtime。`internal/modelbackend/fake` 只是测试夹具。

```yaml
# 保存到临时 native-cli.yaml，离线即可复现
version: 1
backends:
  native:
    type: native-cli
models: {}
```

```bash
"$BINARY" --config "$TMPDIR/native-cli.yaml" model list
```

实际退出 1：`unsupported backend type (native CLI bridge is disabled)`。见[机器可读证据](evidence/gate-b.json)。这是配置拒绝测试成功，不是原生模型 API 成功。将 endpoint 改为 OpenAI/Ollama 或把 `upstream_model` 命名为 codex 都不能补上这一缺口。

## 已复现并修复的最小缺陷

原 `tests/r0/smoke.py` 的整体判断使用 `any(tool passed)`。离线回归复现：仅 Codex 安装并成功时，即使另外两种缺失，脚本仍返回 0。现改为三种都通过才返回成功；缺失/未登录/失败保留 partial。

同时固定每个 CLI 仅 headless/resume 各一次，不再把失败 CLI 剩余额度借给其他 CLI 的 cancel 探查。合计最多 6 次，失败不重试。以每进程只读/plan/ask 与 sandbox 参数约束文本测试，不修改全局配置或 trust。

回归命令：

```bash
env PYTHONDONTWRITEBYTECODE=1 \
  python3 -m unittest discover -s tests/r0 -p 'test_smoke.py' -v
```

修复前 4 项中 1 项失败；修复后 4/4 通过。[日志](evidence/smoke-harness-tests.log)。覆盖缺 CLI 不通过、完整三对最多六次、带正确标记但非零退出不通过且不重试、全部缺失不启动。未修改 xcli 产品业务代码或新增桥接框架。

## 调用预算与后续执行

已提出一次明确授权请求：[预算账本](evidence/native-budget.json)。待选额度为文本最多 6 次进程启动、媒体额外最多 6 次（各 CLI 生图/编辑各 1 次）；进程内部模型轮数和费用并不完全可观测。权限扩大只批准了离线门禁和只读认证探测，不等于批准模型额度。

收到文本授权后，执行 `bash xcli-cli-first-acceptance/verify-phase1.sh --native-smoke`，每次最长 120 秒，不重试、不自动 trust。原生工作区审批若阻止临时项目运行，记 BLOCKED，不追加全权限参数。成功路径六次已用尽；真实超时/取消需要另列预算，不能偷偷追加启动。

收到媒体授权后，各 CLI 在独立临时合成项目中最多生成一次、参考编辑一次。优先编辑本轮生成图；生成失败时只能使用明确标记的合成参考夹具，不能把夹具算作生图成功。保留原生 PNG/JPEG/WebP 字节，核对魔数、MIME、完整解码、尺寸、SHA-256、输入未覆盖及视觉差异；确认实际使用原生图片工具，禁止改用代码绘图、普通 API、Ollama 或助手的图片工具补成功。没有合法工具、图片读取/交付被拒或无有效字节时如实 BLOCKED/FAIL，并停止该场景。

## 消费仓库只读基线

| 仓库 | HEAD | 初始脏项数量 |
| --- | --- | --- |
| Worklog | `e38f2a4901d871777989ffee6684e836370e70aa` | 0 |
| GardenFlow | `33b59295888f5cd34abab902182c08cf6f5442b7` | 0 |
| Chat Insight | `920c900a2c8685cc92e9ba4b065cc77216e1b99d` | 11 |
| Contribution OS | `8aa905151329f03e32397c3d1e057bf2a0ac2950` | 7 |

仅核对源码路径和现有接入边界，不读取业务数据库、聊天内容或 secrets。后两者已有用户改动，后续工程必须重新核对 HEAD/diff，不能重放旧补丁覆盖。本轮不修改四仓库、不运行 UI、不部署、不 push/release。

下一步先补获授权的 A 真实证据，再按[接入架构](INTEGRATION_ARCHITECTURE.md)单独确认最小 Agent API 工程范围。即使 A 全通过，B 和 C 也保持 BLOCKED，直到实现与各自真实验收完成。

## 交付检查

10 份 Markdown 文档的 42 处本地链接、5 份 JSON、两个 Python 文件语法、证据/源码摘要和四应用所选源码/工作区状态核对通过；未发现未脱敏 home/email 标记。`bash -n xcli-cli-first-acceptance/verify-phase1.sh` 与 `git diff --check` 通过。检查的是本地链接存在性；官方资料已在当日实际打开核对，不据此保证网页今后不变。

最终工作区仅新增本组文档/证据、离线回归测试，修改 `tests/r0/smoke.py`；用户原有验收目录保留。无提交、push、release、服务安装或消费应用写入。
