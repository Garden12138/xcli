# xcli v2

xcli 是本机多 CLI 编排引擎，通过真实 Codex、Cursor、Antigravity 进程，将文本、编码、审查和图片能力以子进程 JSON 协议提供给桌面应用。

版本：`2.0.0`。支持 macOS / Linux（amd64 / arm64），可从 [Releases](https://github.com/Garden12138/xcli/releases) 下载可执行文件。

## 构建

安装 Go 1.26 后运行：

```sh
go build -o bin/xcli .
export PATH="$PWD/bin:$PATH"
```

安装并登录原生 CLI（`codex`、`cursor-agent`、`agy`），即可使用任务能力的默认配置。

## 用法

```sh
xcli ask "总结 notes.md 的要点" -i notes.md --json
xcli image "米白背景上的绿植插画，方形" -o ./covers --json
xcli code "修复 TTL 到期判断的 Bug" -C ~/repo --verify "go test ./..." --json
xcli review "检查并发与边界条件" -C ~/repo --json
xcli do "给 README 写一段介绍并配一张封面图" -C ~/repo
```

应用通过 `xcli submit --request - --json` 提交 Request JSON，或用 `--events` 读取 JSONL 进度。后台任务用 `--detach`，本机 HTTP 任务接口用 `xcli serve`。

参见 [一页用户手册](docs/v2/USER_GUIDE.md)、[接口规范](docs/v2/03-interfaces.md)、[设计与实施计划](https://github.com/Garden12138/xcli/tree/v2.0.0/docs/v2)。手动运行 `scripts/smoke.sh`，按回车开始约 10 次消耗额度的真实 CLI 调用。v0.x 的历史成果保存在 tag `archive/r4.1-final`。
