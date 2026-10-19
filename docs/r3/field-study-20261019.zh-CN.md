# R3 独立试用准备 · 2026-10-19

历史准备状态：**试用包和记录工具已就绪，实际参与者记录为零。** 用户随后要求改用三个不同经验的 AI 角色独立试用并反馈，[该模拟评审已完成](persona-results-20261019.zh-CN.md)。当前轮次不再等待招募，本页保留为可选真人研究方案；没有主动联系他人，也没有用模拟结果填写真人表单。

本页是参与者试用准备；另行执行的[协作收益对照](value-results-20261019.zh-CN.md)已经完成，不能用那三组自动化运行补足三位用户的记录。

从干净的已提交源码 `c209756751e29075a7abbbea3c96db5357803dc3` 构建四平台 GoReleaser 2.16.0 snapshot `0.5.0-r3-dev-c209756`；四组归档内容、权限和校验和通过，Darwin arm64 实际安装检查通过，见[安装证据](evidence/installation-c209756.json)。其他平台只说明构建/归档通过，不扩大原生 CLI 实测范围。

本机试用包为 `/tmp/xcli-r3-fieldstudy-20261019/trial-kit.zip`，包含四平台候选、校验和、候选摘要、[参与者说明](participant.zh-CN.md)、A/B/C 三份空白 JSON 和无需第三方依赖的记录检查脚本。未上传或发布安装包，不把本机文件路径视为公共下载地址。组织者可以自行分发，不需向参与者提供 Go 开发环境。

三位用户各自选择匿名编号并填写自己的记录。允许失败、缺账号、求助和提前退出；未知值保留 null。第一次任务、自行诊断失败、后台 stop/continue 和隔天找回均需真实操作，日期必须真实，未观察七天不能声称七天复用。原始记录默认私有，只保存愿意提供的脱敏证据；没有遥测、自动上传或自动发送。

```bash
python3 tests/r3/observe.py prepare --kit /tmp/r3-participant-kit \
  --dist /path/to/snapshot/dist --installation /tmp/r3-installation.json \
  --out /tmp/r3-kit-prepared.json
python3 tests/r3/observe.py summarize --kit /tmp/r3-participant-kit \
  --out /tmp/r3-observation-inventory.json
```

prepare 只验证/复制归档并创建空表，不调用模型。summarize 检查候选摘要、缺项、日期和求助记录，报告计划人数三人、完整记录数、未完成项和错误，不公开参与者自由文本或证据位置。即使表单全部填满，也只能标记“待人工复核”，不会自动认证独立性或通过 R3。正式结论还需核对真实参与者、操作覆盖、证据和发布同意。

新建试用包不是三个用户已成功安装，也不是维护者自身模拟观察。实际结果将在收回记录后另行形成报告；本批只保存[试用准备清单](evidence/observation-preparation-20261019.json)和[空白记录盘点](evidence/observation-inventory-20261019.json)。
