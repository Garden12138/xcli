# 十分钟开始本地体验

版本：`0.5.0-r4.1-dev`，未发布开发版。本轮离线验收通过；本轮没有进行模型调用。下面最后一条是**你主动执行的真实任务**，会使用你自己的原生 CLI 账号和额度。

## 1. 在当前交付仓库安装

需要 Git、Go 1.26.0+，本机验证 Go 1.26.4；原生 CLI 由用户先自行安装/登录。当前目录应为本次交付的 xcli 仓库根：

```bash
git switch feature/upgrade-chatgpt
go build -trimpath -o bin/xcli .
mkdir -p "$HOME/.local/bin"
install -m 755 ./bin/xcli "$HOME/.local/bin/xcli"
export PATH="$HOME/.local/bin:$PATH"
xcli --version
xcli config validate
command -v codex
```

应看到 `xcli version 0.5.0-r4.1-dev`。配置默认可用，无需初始化或启动后台服务。若不想覆盖 PATH 中的 xcli，可将下面命令的 xcli 替换为本仓库二进制的绝对路径。

本次成果已在用户后续授权后推送；GitHub 默认 main 不等于此版本。尚无本地源码时先执行 `git clone --branch feature/upgrade-chatgpt https://github.com/Garden12138/xcli.git`，再进入 xcli 目录构建。源码获取、PATH 持久化和其他平台见 [安装说明](LOCAL_INSTALLATION.md)。

## 2. 从你自己的独立终端建立新体验窗口

本例 30 次启动、串行。**只有本次窗口没有其他控制器或未记账原生启动时，才确认下面 none/0。** 不确定时先看 [安装说明的窗口条件](LOCAL_INSTALLATION.md#r41-首次执行前建立一次启动窗口)，不要复用未知历史或删除账本。

```bash
export XCLI_LAUNCH_WINDOW="local-$(date +%Y%m%d-%H%M%S)"
xcli launches window create "$XCLI_LAUNCH_WINDOW" \
  --limit 30 --concurrency 1 \
  --external-agent none --external-count 0 --external-scope included \
  --external-source '本人独立终端的新本地体验，无其他作用域内原生启动' \
  --confirm-external
xcli launches budget
xcli agents --json
xcli doctor codex --json
```

启动窗口还会计版本/help 探针；它不是费用/Token 预算。doctor 只验证安装/可观察参数，原生登录和额度需用户在原生工具中确认。

## 3. 准备一个独立小项目并发起第一个任务

以下不会使用其他业务仓库。模型任务要求只读分析；本机文件访问仍由原生工具的权限规则决定：

```bash
XCLI_DEMO_DIR="$(mktemp -d "${TMPDIR:-/tmp}/xcli-first.XXXXXX")"
git -C "$XCLI_DEMO_DIR" init -q
printf '%s\n' 'package demo' '' 'func Add(a, b int) int { return a + b }' > "$XCLI_DEMO_DIR/add.go"
git -C "$XCLI_DEMO_DIR" add add.go
git -C "$XCLI_DEMO_DIR" -c user.name='Local Demo' -c user.email='demo@example.invalid' commit -qm 'demo input'
xcli run --agent codex --cwd "$XCLI_DEMO_DIR" \
  '请只读分析 add.go，说明函数用途和建议的边界测试；不要修改文件。' --json
```

返回对象应包含 `run_id`、`status`、`parse_status`、`session_id` 和 `output`（是否有 Session 以实际结果为准）。原生拒绝读取/未登录/额度不足时，先按 [故障排查](USER_MANUAL.md#故障排查) 处理，不自动扩大权限。

Cursor 或 Antigravity 可替换最后一条命令（同样由你主动调用）：

```bash
xcli run --agent cursor --cwd "$XCLI_DEMO_DIR" '只读复核 add.go，列出边界测试，不修改文件。' --json
xcli run --agent antigravity --cwd "$XCLI_DEMO_DIR" '只读说明 add.go 的用途，不修改文件。' --json
```

Cursor 若报告 Workspace Trust，先在原生工具针对这个目录做你自己的信任决定。xcli 不自动添加 --trust/force 或修改授权。使用其他 CLI 前可先执行 `xcli doctor cursor --json` 或 `xcli doctor antigravity --json`。

## 4. 查看与继续

```bash
xcli runs list --json
# 用上一步实际返回的 ID 替换下面值：
XCLI_RUN_ID='run-请替换为实际ID'
xcli runs show "$XCLI_RUN_ID"
xcli resume "$XCLI_RUN_ID" '继续补充一个溢出场景的说明，不修改文件。' --json
xcli launches budget
```

resume 是新的真实执行，继续计启动且需原窗口；没有有效 Session 时不要伪造 ID。Run 成功表示原生执行/协议成功，不等于完成文件或用户接收。需要修 Bug、交接审查、后台长任务和文件产物时，继续看 [USER_MANUAL.md](USER_MANUAL.md)。
