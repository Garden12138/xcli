# R3 参与者操作说明

这是未发布候选的真实试用。请由三位已有原生 CLI 使用经验的用户各自独立操作；观察者只记录，遇到求助时记录帮助内容。脚本演练、维护者代操作和机器人不能算作独立参与者。我们不会自动收集数据。

## 开始前

1. 选择 A、B 或 C 中尚未分配的匿名编号，只填写自己的 JSON。保留其他两份为空。三位用户分别保留自己的副本，由组织者合并。
2. 从 `candidate-manifest.json` 选择对应系统/架构的归档，核对 `candidates/checksums.txt`。Darwin arm64 有本机原生验收记录，其他平台不能因构建通过而视为 CLI 实测通过。当前支持范围见仓库 `docs/r3/compatibility.zh-CN.md`。
3. 解压归档，用其中的 xcli 操作。记录 `./xcli --version` 以及二进制 SHA-256；macOS 可用 `shasum -a 256 ./xcli`，Linux 可用 `sha256sum ./xcli`。JSON 的 platform 写 `darwin/arm64`、`darwin/amd64`、`linux/arm64` 或 `linux/amd64`。
4. 已有原生 CLI 的安装、登录、权限和额度由你确认。先确定自己允许的调用预算。使用不含秘密的材料和可丢弃项目；不用为了试用切换账号、购买额度或修改真实项目。

## 自行完成并记录

先运行 `./xcli doctor codex`，选择自己的一项 Work 或 Project 小任务。以下命令只是入口示例，材料路径和目标由你选择：

```bash
./xcli task "你的任务目标与完成标准" --input ./materials --agent codex --max-calls 5 --timeout 10m --json
./xcli task status <task-id> --json
```

记录从开始安装到首次成功的实际耗时，包括原生安装、登录或额度阻力。自行找到产物、验证结果与未解决项；若没有成功，保留未完成状态和原因，不填零耗时。

在可丢弃材料中尝试一个可解释的失败，例如缺少输入文件或错误的验证 argv；自行查看错误并尝试修正。记录你认为的原因、实际修正、结果及求助，不以“必须成功”为目标。

再启动一个自己的后台任务，观察状态并停止，从另一终端找回和继续：

```bash
./xcli task "另一项可丢弃任务" --input ./materials --agent codex --detach --max-calls 5 --timeout 10m --json
./xcli task status <task-id> --events
./xcli task stop <task-id> --json
./xcli task continue <task-id> --detach --json
```

如果任务已经完成，记录 stop 是闲置操作，不能填写“中断运行中的 worker”。continue 继承已用调用数；不要为了填表反复提高预算。若出现 needs_reconcile，先检查材料与状态，记录是否能理解原因，不盲目确认未知副作用。

**隔天**再次找回同一 Task，说明它的状态、工作材料、模式和累计预算。必须写实际日期，不能在同一天补填隔天结果。第二个 CLI 可用时可尝试跨 CLI 复核；否则写未测。接下来是否自发复用由你决定，只填写实际观察天数，未满七天不能声称七天留存。

## 填写及交回

所有未知项保留 `null`。没有求助时亲自确认 `help_count: 0`；有求助时在 `help_events` 中写时间、问题与帮助内容。失败诊断、后台继续、隔天找回字段写真实过程和结果；失败、受阻和提前退出也应保留。

`participant_attested: true` 表示你本人完成了记录中的操作；`publication_consent` 由你决定。`evidence_refs` 指向你愿意提供的脱敏记录，不放原生 session、账号、密钥、完整原始流或个人目录。三位用户的 JSON 含个人操作描述，默认作为私有材料，仅交给组织者；试用包不提供自动上传或遥测。

组织者可运行 `python3 observe.py summarize --kit <合并后的目录> --out <新摘要.json>` 检查缺项。摘要不会自动认定独立性或通过 R3，仍需人工核对参与者、证据、失败样本和发布同意。
