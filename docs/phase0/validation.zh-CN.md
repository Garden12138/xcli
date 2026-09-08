# A03：验证方式、矩阵与脱敏

## 重复运行模拟夹具

工具是 opt-in 的 Go main 包，不引入新依赖。默认 go test 只检查契约样例，不启动模拟套件或真实模型；Go 源码按 darwin/linux 构建，与项目支持范围一致。

在仓库根执行以下命令。`phase0_dir` 为本次专用临时目录，不覆盖 HOME、CODEX_HOME 或用户全局 Git 设置。源码导出不包含未提交工作，构建二进制时保存对应完整 commit。

```sh
phase0_dir=$(mktemp -d "${TMPDIR:-/tmp}/xcli-phase0.XXXXXX")
mkdir "$phase0_dir/stable" "$phase0_dir/development"
git archive main | tar -x -C "$phase0_dir/stable"
git archive origin/v.0.5.0 | tar -x -C "$phase0_dir/development"
(cd "$phase0_dir/stable" && go build -o "$phase0_dir/xcli-stable" .)
(cd "$phase0_dir/development" && go build -o "$phase0_dir/xcli-development" .)
go build -o "$phase0_dir/phase0" ./tests/phase0
"$phase0_dir/phase0" \
  -stable "$phase0_dir/xcli-stable" -source "$phase0_dir/stable" \
  -stable-commit "$(git rev-parse main)" \
  -development "$phase0_dir/xcli-development" -development-source "$phase0_dir/development" \
  -development-commit "$(git rev-parse origin/v.0.5.0)" \
  -out "$phase0_dir/simulation.json"
go test ./tests/phase0
```

为复验本轮固定基线，可把上述 main 和 origin/v.0.5.0 替换为 baseline 文档中的完整 SHA。参数中的 commit 是调用者声明，工具另记二进制 SHA-256 供核对；不据 `--version` 的 dev 字符串推断源码提交。F01 需要开发线二进制；省略 development 参数只执行稳定线六项。

每个场景新建临时目录、配置与 XDG 数据位置，用自身可执行文件模拟 Agent，模拟环境只包含夹具 bin、系统 PATH、XDG 目录及隔离 Git 设置。MCP 只操作临时项目原生配置，不连接真实服务，不依赖用户原生配置或凭据。F06 复制源树原始示例后从两个调用目录运行。F01 的 Git 配置仅在命令行指定测试身份，标记文件仅写临时源仓库。5 MiB 单行在执行时生成；临时工作目录结束后删除。

结果语义：

| status | 意义 | runner 退出 |
| --- | --- | --- |
| reproduced | 与已审查缺陷观察一致 | 无执行异常时 0；不是发布通过 |
| meets_fix_expectation | 满足本夹具检查的修复预期 | 无执行异常时 0；仍需阶段 1 扩展回归 |
| execution_error | 无法执行、夹具前置条件失败或观察落在已定义两类之外 | 2 |

不得把 reproduced 写成普通断言，要求修复后的二进制继续复现缺陷。命令退出、最终状态和关键文件实际内容共同判定，单纯“命令失败”不算修复完成。F03 顺带记录二次导入幂等；内存别名、全部未知字段、路径越界、前后台与重试等扩展回归留待 B01–B06。

## 真实 Codex smoke

用户已确认：使用本机现有登录和默认模型，最多两次 CLI 模型任务调用，每次 120 秒，失败不自动重试。这里是调用次数上限，不是货币金额上限或 provider 内部请求数上限。登录失效不自动登录，不索取凭据。其他 CLI/平台真实调用预算为 0，列为后续依赖。

```sh
"$phase0_dir/phase0" \
  -stable "$phase0_dir/xcli-stable" \
  -stable-commit "$(git rev-parse main)" \
  -real-codex "$(command -v codex)" \
  -out "$phase0_dir/codex-smoke.json"
```

这是**显式付费/配额任务入口**，不在默认测试或模拟模式执行。真实模式只有 passed 返回 0；failed/blocked 在写出证据后返回 1。输出路径必须不存在，防止意外重跑覆盖证据；改用新文件重跑意味着新增预算，需根据会话授权重新判断。

先本地探测版本和 login status；随后在临时 Git 仓库中运行一次 run --json，要求回复固定文本并记住一个无敏感内容的 token。第二次从 xcli 保存的 run ID 恢复，要求重复前一轮 token；不在第二个 prompt 中提供答案。两次均沿用默认模型，强制原生 read-only sandbox 与不请求执行审批，prompt 明确不使用工具、不修改文件。通过一个 exec 替换的透明入口独立记录原生实际 cwd，核对同一 session 与 resumed_from 链接。第一步不满足预期则不执行第二步。

只隔离 xcli 数据；原生 Codex 使用现有 HOME/CODEX_HOME 与认证，可能按其默认行为保存这两次原生 session。工具不复制、改写或删除原生配置与凭据。真实调用可能需要网络和原生 session 目录写权限；环境拒绝时应报告限制，不能用模拟通过替代。

## 证据与脱敏

- 模拟结果保存完整命令参数、退出码、输出、平台、Go 版本、commit、二进制摘要和关键观察。路径替换为 `<fixture>`，Agent 和 xcli 的可执行路径替换为占位符。较大输出仅存长度与 SHA-256。
- 真实结果只发布固定允许列表中的回复、退出码、耗时、usage、版本和布尔验收；任意原生 stderr、异常模型回复、账号身份、session ID 和真实路径不进入提交。不要以宽泛正则脱敏替代这个允许列表。
- 新真实事件样本必须先去除 token、headers、账号、项目路径、工具参数和源码片段；以来源 CLI 版本、平台、采集日期和脱敏说明登记。当前合成 JSONL 不冒充真实脱敏事件集。
- 检查记录使用具体命令与结果，不复用历史审查的通过记录。外部依赖写负责人、解除条件和发布影响。

## 后续测量清单

| 测量 | 脚本/入口计划 | 方法与目标 | 阶段 |
| --- | --- | --- | --- |
| 首事件反馈 | tests/phase0 下增量扩展延迟 Agent 与计时 reader | 记录 Agent flush 时间和 xcli 首个可展示字节；差值目标 500 ms 内 | C02/C03 |
| 启动开销 | 版本缓存命中、无模型延迟的多次进程启动夹具 | 固定机器预热后 30 次，记录原始值与 p95，目标 200 ms 内 | C06 |
| 大输出内存 | 产生 100 MiB/1 GiB JSONL；macOS time -l / Linux time -v | 关闭完整记录，记录峰值 RSS、事件大小和总量，验证不随总量线性增长 | C02 |
| 原生平台兼容 | 本页真实入口扩展 run/JSON/resume/取消/MCP 矩阵 | 每个 CLI 固定版本；独立记录账号预算和实际结果 | B07/E01 |

这些是测量入口与方法清单，不是已经完成的性能脚本或成绩。M0 只要求复现夹具可重复执行和测量方式可追踪。

## 验证矩阵模板

| 日期 / xcli commit | CLI / 版本 | OS / 架构 | 来源 | run | JSON | resume/cwd | 取消 | MCP | 账号/预算 | 证据 / 阻断负责人 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 待填写 | claude/codex/gemini/opencode + 固定版本 | darwin/linux + amd64/arm64 | simulated / real / unverified | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 账号只记别名与上限 | 路径、下一步、解除条件 |

构建验证单列 OS/架构、Go 版本、CGO 设置及结果，不能填入真实兼容格。本轮填写结果见 results 文档。
