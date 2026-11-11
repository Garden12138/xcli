# 本地安装与启动

适用：未发布源码版 `0.5.0-r4.1-dev`，优先 macOS Apple Silicon。xcli 是终端程序，安装后直接执行命令即可，不需要启动守护服务、Gateway、OAuth 或模型 API。

## 环境

| 组件 | 要求 |
| --- | --- |
| 系统 | 本轮真实构建/离线运行 macOS arm64；macOS Intel、Linux amd64/arm64 仅交叉构建通过 |
| Go | go.mod 要求 1.26.0；本轮验证 1.26.4。使用可运行 `go version` 的 Go 1.26 工具链 |
| Git | 构建信息、Project Task/worktree 需要 Git；Project 仓库必须已有提交且干净 |
| 原生 CLI | 可选择任意一个；默认 Codex=`codex`，Cursor=`cursor-agent`，Antigravity=`agy`；由各工具自行安装和登录 |
| Python | 仅跑 Python 验收时需要，本机 3.10.9；正常使用 xcli 不需要 Python |
| 图片检查 | `file`、macOS `sips`/Linux 图片查看器；Pillow 是历史媒体验收的可选依赖 |

无需 Node/Python 作为 xcli 自身运行时，但具体原生 CLI 可能需要它们。Go 依赖首次下载需要网络；本轮使用已有缓存离线验证，不要求用户设置 GOPROXY=off。

## 使用当前本地完整源码（推荐）

在本次交付的仓库根目录执行：

```bash
git switch feature/upgrade-chatgpt
git status --short
go version
go build -trimpath -o bin/xcli .
./bin/xcli --version
./bin/xcli --help
```

应显示 `xcli version 0.5.0-r4.1-dev`。本轮已生成可直接运行的 `./bin/xcli`，该文件忽略于 Git；源码重建不依赖旧根目录二进制。

本轮没有 push。**从 GitHub 新 clone 不能获得本地未推送的 21 个升级提交和本次交付提交。** 在另一目录体验可克隆当前本地仓库（将 `/path/to/existing/xcli` 替换为原仓库）：

```bash
git clone --branch feature/upgrade-chatgpt /path/to/existing/xcli xcli-local
cd xcli-local
go build -trimpath -o bin/xcli .
```

远程源码的获取命令如下，仅用于后续分支已实际分享的情况；本次不要把旧远程 main 当作相同版本：

```bash
git clone https://github.com/Garden12138/xcli.git xcli-remote
cd xcli-remote
git branch -a
# 确认含本次提交的分支/补丁已交付，再选择对应源码构建。
```

## 安装到个人 PATH

确认构建成功后，在仓库根执行（由用户执行，本轮未写入个人 bin）：

```bash
mkdir -p "$HOME/.local/bin"
install -m 755 ./bin/xcli "$HOME/.local/bin/xcli"
export PATH="$HOME/.local/bin:$PATH"
command -v xcli
xcli --version
```

macOS 默认 zsh：需要永久生效时，将 `export PATH="$HOME/.local/bin:$PATH"` 加入自己的 `~/.zshrc`，再打开新终端。Linux bash 可加到 `~/.bashrc` 或登录 profile；如果该目录已在 PATH，无需重复添加。已有 xcli 会被 install 替换，需保留旧版本时先自行备份；也可以始终用本仓库 `./bin/xcli`，不覆盖个人 PATH。

Apple Silicon 通常为 `arm64`，Intel Mac 为 `x86_64`/`amd64`，用 `uname -m` 检查。原机 `go build` 自动选择正确架构。Linux 用同样构建/安装命令；Linux 无 Keychain/sips，原生登录和工具依赖按 Linux 版本处理。Windows 仅编译验证，后台 Jobs/Task 当前不支持，不作为本手册的本地体验目标。

## 原生 CLI 的安装发现与原生登录

以下仅查询 PATH，不启动工具、不推理：

```bash
command -v codex
command -v cursor-agent
command -v agy
```

本机三者已发现，文件摘要见 [native-discovery.json](native-discovery.json)。缺少某个命令时，用该工具的官方安装渠道安装对应平台的 CLI；`cursor` 编辑器命令不等于本仓库默认的 `cursor-agent`，其他同名程序也不能替代 `agy`。不要使用旧 xcli install/auth 命令，它们已退役。

分别从你自己的终端进入所选原生工具，按它本身的登录或设置界面完成认证：Codex 自管 Codex 账号，Cursor 自管 Cursor 账号和 Workspace Trust，Antigravity 自管其登录与原生工具权限。原生帮助/版本仅是探针，不证明额度或工具可用；交互界面也可能执行任务，应由用户控制。本轮没有替用户执行任何原生登录、切换账号或修改原生配置。无需把 Token、API Key 或账号路径写进 xcli。

## xcli 配置

默认配置可直接使用，不强制写文件：

```bash
xcli config path
xcli config validate
# 仅在你确实想创建配置且该文件不存在时：
xcli config init
```

默认路径 `~/.config/xcli/config.yaml`，设置 XDG_CONFIG_HOME 后为 `$XDG_CONFIG_HOME/xcli/config.yaml`；`--config /path/to/config.yaml` 优先。init 默认拒绝覆盖已有文件；不要为体验盲目加 --force。

可选最小配置（保存到 `xcli config path` 显示的路径，不包含账号）：

```yaml
version: 1
default_agent: codex
agents:
  codex:
    adapter: codex
    command: codex
  cursor:
    adapter: cursor
    command: cursor-agent
  antigravity:
    adapter: antigravity
    command: agy
recording:
  output: false
```

如果可执行文件使用其他名称/绝对路径，修改你自己的 `command`，保留正确 `adapter`；未知配置会由 validate 报错。JSON 配置也可用。普通原生网络/代理环境默认继承；必要时通过已有 networks/agents.network 配置选择网络，不应把代理凭据提交到 Git。

## R4.1 首次执行前：建立一次启动窗口

R4.1 已有启动预算要求，必须完成；不会自动生成无限窗口。计数单位是启动原生 CLI 入口一次，**版本/help 也计数**，与模型请求数/费用不同。

下面只适用于**用户从独立终端开始的新本地体验，窗口内没有其他控制器/未记账原生启动**。`none/0` 是用户对该事实的确认；存在控制器或无法确认次数时，不能照填，应查 [预算语义](../product-realignment/R4_1_BUDGET_SEMANTICS.md)，补齐真实来源和计数，未知就停止启动。历史 R4 UNKNOWN 余额不能沿用。

```bash
export XCLI_LAUNCH_WINDOW="local-$(date +%Y%m%d-%H%M%S)"
xcli launches window create "$XCLI_LAUNCH_WINDOW" \
  --limit 30 --concurrency 1 \
  --external-agent none --external-count 0 --external-scope included \
  --external-source '本人独立终端的新本地体验，无其他作用域内原生启动' \
  --confirm-external
xcli launches budget
```

30 是本示例中用户可自行选择的进程启动上限，不保证 30 次模型请求，也不保证覆盖复杂任务。此命令只创建本机私有窗口，不执行模型。窗口不可改写，继续同一 Run/Task 必须沿用其原窗口和 store；开新终端时重新 export 原来的 ID，不要给原任务换窗口。

## 安装后的验证与第一个任务

以下会执行版本/help 探针，但不要求模型回答：

```bash
xcli agents --json
xcli doctor codex --json
```

agents 检查所有已配置工具；doctor 无参数会因任意未安装 Agent 失败，初次建议只检查你要用的一个。安装/版本/help 通过并不证明账号或额度可用。

随后由用户主动发起自己的首个模型任务，按 [QUICK_START.md](QUICK_START.md) 操作。本轮验收没有执行该步骤。单次 run/Task 直接启动，无需另起服务。运行数据默认在 `${XDG_DATA_HOME:-$HOME/.local/share}/xcli`，保留配置、launches、tasks、runs；卸载或排障不要直接删除这些数据。
