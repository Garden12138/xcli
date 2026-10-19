# R3：对照实验与候选验收

最新真实补验见 [2026-10-19 后台恢复记录](followup-20261019.zh-CN.md)：指定 Codex 中断恢复通过，Cursor 当前额度耗尽，跨 CLI 后台恢复仍未补齐；独立新增预算使用 5/12，历史账本保持不变。

后续[已提交候选安装验收](acceptance-20261019.zh-CN.md)新增 30 项配置、失败诊断和历史查询检查，无新增模型调用。[实测支持矩阵](compatibility.zh-CN.md)区分已测角色、当前额度失败与未测范围。

本阶段围绕可重复实验、原生兼容、安装和发布材料收敛。候选版本为 `0.5.0-r3-dev`；snapshot 附带基线短提交号。它不是已发布的 v0.5.0。最终结果见 [验收记录](results.zh-CN.md)。

## 安装与开始

要求已安装、登录且有可用额度的原生 Codex 或 Cursor。源码构建要求 Go 1.26：

```bash
go build -trimpath -o xcli .
./xcli --version
./xcli doctor codex
./xcli task "根据材料生成带来源的 report.md" --input ./materials --agent codex
./xcli task "修复缺陷，通过测试并由另一 CLI 复核" --project . \
  --verify '["go","test","-count=1","./..."]'
```

发布归档的安装方法是先核对官方提供的 `checksums.txt`，将对应 OS/架构归档解压到临时目录，再把 `xcli` 复制到自己的 PATH 目录。本次只有本地 snapshot；不要用历史 v0.4.0 下载链接安装并期待 Task 功能。Work 输入可以是文本或目录；Project 要求干净、有提交的本地 Git 仓库，修改位于受管理 worktree。

后台与隔天继续：

```bash
./xcli task "根据材料形成报告" --input ./materials --agent codex --detach --json
./xcli task status <task-id> --json
./xcli task status <task-id> --events --follow
./xcli task stop <task-id> --json
./xcli task continue <task-id> --detach --json
# 原生 session 不可用时，显式使用已保存材料
./xcli task continue <task-id> --fresh-session --detach --json
```

`needs_reconcile` 表示存在未知执行结果或材料变化。先检查 worktree、diff 和记录，再通过 `continue --reconcile` 确认当前材料。追加 `--max-calls`、`--timeout` 是提高原 Task 的累计上限，不会清零已用预算。登录或额度失败由原生工具处理，不自动切换指定 CLI。

报告可用唯一的输入文件名引用所选材料，运行时仍检查原输入的内容摘要；同名输入需显式路径。相对来源路径从报告文件所在目录解析。输出目录的同名副本不能掩盖原输入被修改。

## 重复对照实验

```bash
python3 tests/r3/experiment.py prepare --study /tmp/xcli-r3-study --binary ./xcli --budget 48
python3 tests/r3/experiment.py run --study /tmp/xcli-r3-study \
  --out /tmp/r3-comparison.json --execute
python3 tests/r3/compatibility.py --study /tmp/xcli-r3-study \
  --out /tmp/r3-compatibility.json --execute
python3 tests/r3/experiment.py summarize --study /tmp/xcli-r3-study \
  --out /tmp/r3-final.json
```

prepare 只创建私有夹具并读取版本，不执行模型任务。run 的 `--cases` 可选择冻结的子集，仍按预先固定顺序运行。更换 `--out` 不会重跑已记录案例；被中断的 `running` 案例保留，不自动重放。输入、评分器、配置、二进制或原生版本变化均拒绝沿用旧实验。

新的候选必须新建 study，并通过 `prepare --campaign <原 study>` 共用原始账本和累计上限，不能借新目录清零预算。每组最多六次，对照总计最多 36 次，兼容总计最多 12 次，整批最多 48 次。失败、取消和启动失败后的原生分派都计数；版本/help 不计数；厂商内部模型请求不完全可见。

公开报告只保留指标、内容摘要和非秘密版本信息。study 目录、原始输出、验证诊断和 task 数据按私有文件保存，不应直接上传整个 study。工具只对生成的临时目录传入 Cursor `--trust`，不更改原生长期规则、账号或凭据。

发布前只导出指定的摘要报告，删除私有载荷字段并替换个人目录：

```bash
python3 tests/r3/evidence.py --study /tmp/xcli-r3-study \
  --report /tmp/r3-final.json --out /tmp/r3-public.json
```

## 实验口径

后续只补两个后台恢复缺口时，可使用单独的补验入口。它要求显式的新预算，保存旧账本摘要，先做一次 Cursor 真实可用性检查；失败时不重试或换 CLI。运行中的 worker 必须确实被取消，之后新 worker 的原生 session 必须与中断前不同，才判材料恢复通过。单纯澄清 waiting_user 不会计为 worker 中断成功。

```bash
# <本轮新增上限> 必须先确定，不沿用已耗尽的旧兼容预算
python3 tests/r3/revalidate.py prepare --study /tmp/r3-followup \
  --binary ./xcli --previous-campaign /tmp/xcli-r3-study --budget <本轮新增上限>
python3 tests/r3/revalidate.py run --study /tmp/r3-followup \
  --out /tmp/r3-followup.json --execute
python3 tests/r3/revalidate.py inspect --study /tmp/r3-followup \
  --out /tmp/r3-followup-inspected.json
```

补验最多为每个恢复场景分派五次调用，可记录一次协调计划澄清；有澄清时标记操作员介入。旧 campaign 的调用数和账本不变，新调用单独记账，公开结果同时显示历史与本轮消耗。失败或中断的补验不因更换输出文件重放。

- Work：三份材料包含旧总数、旧发布日期和缺失审批；生成 `report.md` 与机器可核对的 `report.json`，检查事实、来源、遗漏和误报。
- Project：修复缓存 TTL 的开区间、非正 TTL、时钟回退及 int64 溢出；可见测试作为运行时反馈，独立测试只用于最终评分。
- 三组分别为直接 Codex、Task 指定 Codex、Task 的 Codex 执行与 Cursor 复核。相同输入、完成标准、十分钟时限及最多六次调用；直接组采用实现和一次自查/修正，Task 按原生协调、执行、验证、复核和一次修复运行。报告实际用量差异。
- 自动交接、脚本安排的交接与真实人工介入分别计数。本轮自动化实验不能证明减少真实用户求助或交接。
- 跨 CLI 组通过独立验收，且相对基线有更少遗漏、更多可复现发现或更少真实人工交接，才形成对应场景收益证据。基础设施失败单列；无收益照实保留。单轮小样本不能推出普遍质量或成本优势。

## 开发检查与发布边界

```bash
go test -race ./...
go vet ./...
go mod tidy -diff
XCLI_R3_BINARY=./xcli python3 -m unittest discover -s tests/r3 -v
goreleaser release --snapshot --clean
python3 tests/r3/package.py --dist dist --out /tmp/r3-installation.json
# 验收实际归档中的本机二进制；可选 study 只复制旧 Task 数据用于查询
python3 tests/r3/install_acceptance.py --binary /path/to/extracted/xcli \
  --study /tmp/old-study --out /tmp/r3-installed-acceptance.json
```

CI 固定 GoReleaser v2.16.0。package 检查真实 snapshot 的四份归档、SHA-256、内容和权限，并在本机用干净 XDG 和临时 PATH 验证版本、配置及 doctor；非本机归档仅声明构建/归档通过。原生最小 Task 由同一归档二进制的对照实验覆盖。

[独立观察脚本](observation.zh-CN.md)、[迁移与候选发布说明](release-notes.md)和[演示](demo.zh-CN.md)属于本阶段交付。真实用户观察、其他原生平台、实际重启和持续满三十分钟运行没有被模拟测试替代。未达到退出条件时保持候选状态，不创建 tag。
