# xcli v2 使用手册

xcli 在本机启动 Codex、Cursor、Antigravity，完成文本、编码、审查、生图和多步骤协作。支持 macOS / Linux；安装并登录原生 `codex`、`cursor-agent`、`agy` 后，直接提交任务即可使用默认配置。

## 安装与第一次运行

从 [Releases](https://github.com/Garden12138/xcli/releases) 下载对应系统和架构的 `.tar.gz`，解压后将 `xcli` 放入 PATH。发布包附带 `checksums.txt`，可用 `shasum -a 256 <压缩包>` 核对下载文件。

从源码安装（Go 1.26）：

```sh
go build -o bin/xcli .
mkdir -p "$HOME/.local/bin"
install -m 755 bin/xcli "$HOME/.local/bin/xcli"
export PATH="$HOME/.local/bin:$PATH"
xcli ask "用一句话介绍你自己" --json
```

也可以把对应系统和架构的发布包中的 `xcli` 放进 PATH。需要检查环境时运行 `xcli agents`：显示安装、登录、冷却状态；Antigravity 的登录状态显示 `unknown`，实际调用时使用它已有的登录会话。

## 日常五种能力

```sh
xcli ask "总结 notes.md 的要点" -i notes.md --json
xcli code "修复 TTL 到期边界" -C ~/repo --verify "go test ./..." --json
xcli review "检查边界条件与并发问题" -C ~/repo --json
xcli image "米白背景上的绿色植物插画，方形" -o ./covers --json
xcli do "给 README 写一段介绍并配一张封面图" -C ~/repo --json
```

`ask --schema schema.json` 返回符合 JSON Schema 的 `output`；`image --image cover.png "改成蓝色"` 生成修改后的新图片。`code` 默认由另一个 CLI 审查，按验证和审查结果最多修复 2 轮。`do` 自动规划并行/串行步骤，`plan "需求" -C ~/repo --json` 只看计划。`-a codex|cursor|antigravity` 锁定一个 CLI；默认按能力选择并在不可用时切换。

`--json` 的 stdout 是完整 Result，图片路径在 `artifacts`，改动在 `changes`，验证和审查在 `verify` / `review`。`--events` 输出 JSONL 进度，最后一条 `task.finished` 携带 Result。应用可以一次子进程调用接入：

```sh
echo '{"capability":"ask","goal":"给新项目起三个名字"}' | xcli submit --request - --json
```

## 后台、续接和隔离

```sh
xcli code "补齐边界测试" -C ~/repo --verify "go test ./..." --detach
xcli status <task-id>
xcli logs <task-id> -f
xcli wait <task-id> --timeout 30m --json
xcli cancel <task-id>
xcli resume <已结束的task-id> "再加一个负数用例" --json
xcli code "重构 Add" -C ~/repo --isolate --json
xcli apply <隔离任务id> --remove-worktree
```

Ctrl-C 取消前台任务；`wait` 超时后任务继续运行。`resume` 复用最后一步的 CLI 会话，图片任务会自动带上上次图片。`--isolate` 基于 Git HEAD 创建 worktree；`apply` 把改动应用回原仓库。用 `xcli result <id>` 重读结果，`xcli gc --older-than 7d` 清理旧任务。

## 查看接入应用统计

`xcli stats` 显示最近 30 天的任务量、成功率、耗时、失败原因、Token 和每日趋势。
`xcli stats --since 7d --app worklog --json` 可按应用筛选并输出 JSON。
接入应用在请求中填写 `metadata.app`。终端默认归为 `terminal`，可用 `--app` 指定名称。
统计会只读补录历史任务，并标出 CLI 调用和耗时的估算记录。

## 可选配置与 HTTP

配置放在 `~/.xcli/config.yaml`，只填写需要覆盖的字段；`xcli config show` 查看全部生效值，`XCLI_HOME` 可修改存储目录。

```yaml
defaults:
  permission: write       # read / write / full；ask、review 固定只读
agents:
  cursor:
    enabled: false        # 示例：关闭 Cursor
  codex:
    model: ""             # 填原生 CLI 支持的模型名；空值沿用原生默认值
path: ""                  # 空值自动探测；GUI 找不到命令时填完整 PATH
```

本机 HTTP 用 `xcli serve` 启动，默认 `127.0.0.1:7788`。客户端读取 `~/.xcli/server.token`，以 `Authorization: Bearer <token>` 调用接口。HTTP 创建的任务通过同一服务的 `POST /v1/tasks/{id}/cancel` 取消；`xcli cancel` 会给出这个接口的指引。完整请求、SSE 和下载接口见 [接口规范](03-interfaces.md)。

## 常见问题与冒烟

| 遇到的问题 | 下一步 |
| --- | --- |
| 未登录 | 在终端运行 `codex login`、`cursor-agent login` 或 `agy` 完成对应 CLI 登录。 |
| 额度用尽 | 默认自动切换其他可用 CLI；额度恢复后用 `xcli agents --reset` 清除冷却。 |
| GUI 找不到命令或 Node | 在配置的 `path` 填写终端的完整 PATH；桌面应用用 xcli 绝对路径启动子进程。 |
| Cursor 工作区信任 | xcli 自动传 `--trust`；发生 trust 错误时确认原生 CLI 版本及工作目录可访问。 |
| 任务失败或需要处理 | 查看 Result 的 `error`、`verify`、`review`；按任务 ID 用 `logs` 和 `result` 继续排查。 |

仓库中运行 `go build -o bin/xcli .` 后执行 `scripts/smoke.sh`，按回车开始约 10 次真实 CLI 调用。脚本逐项打印 PASS / FAIL、任务 ID 和汇总，临时工作目录保留 JSON、日志和产物，便于定位失败。
