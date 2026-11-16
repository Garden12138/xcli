# xcli v2

xcli 是本机多 CLI 编排引擎，通过真实 Codex、Cursor、Antigravity 进程，将文本、编码、审查和图片能力以子进程 JSON 协议提供给桌面应用。

版本：`2.0.0-dev`。正在 `v2` 分支按设计重建。

## 构建

安装 Go 1.26 后运行：

```sh
go build -o bin/xcli .
```

安装并登录原生 CLI（`codex`、`cursor-agent`、`agy`），即可使用任务能力的默认配置。

## 规划中的用法

```sh
xcli ask "总结 notes.md 的要点" -i notes.md --json
xcli image "米白背景上的绿植插画，方形" -o ./covers --json
xcli code "修复 TTL 到期判断的 Bug" -C ~/repo --verify "go test ./..." --json
xcli do "给 README 写一段介绍并配一张封面图" -C ~/repo
```

参见 [v2 设计与实施计划](docs/v2/README.md)。v0.x 的历史成果保存在 tag `archive/r4.1-final`。
