# R3 演示：复看真实任务与验收证据

演示使用已完成的真实验收数据，不重新调用模型。私有 study 仅留在验收机器；公开读者可以阅读 [脱敏结果](results.zh-CN.md)，或按 [说明](README.zh-CN.md)创建自己的实验。

建议在 60–90 秒内依次展示：

1. **0–10 秒：候选身份。** 运行归档中提取的二进制 `--version`，将 SHA-256 与安装证据中的 Darwin arm64 条目核对。说明当前为候选版。
2. **10–25 秒：固定输入。** 展示 Work 的三份短材料，指出旧版、修订版和未签署审批。展示三组相同的输入摘要与预算。
3. **25–45 秒：Task 与产物。** 从修复后候选私有 `cases/work-single/result.json` 取 Task ID，以 study 的 XDG 数据目录执行 `task status <id> --json`。展示所选 CLI、completed 状态、调用数、report.md 与 report.json，核对总数 21、发布日期和审批状态。
4. **45–65 秒：真实验证与交接。** 展示 Project 跨 CLI 组的实际角色、验证结果和内容摘要；查阅保存的 worktree diff，确认原仓库保持干净。明确区分运行时验证、独立评分和用户接收。
5. **65–80 秒：失败与恢复。** 展示兼容记录中的 stop 状态、同 Task 材料恢复及保留的中断 attempt。若该项失败，展示失败结论与原因，不替换成模拟成功。
6. **80–90 秒：结论边界。** 展示三组实际调用、耗时、评分与观察到的收益/无收益，指出独立用户观察仍待完成。

本机复看命令示例：

```bash
export R3_STUDY=/tmp/xcli-r3-20261017-b
/tmp/xcli-r3-fixed --version
# 从 cases/<case>/result.json 读取对应 Task ID
XDG_DATA_HOME="$R3_STUDY/data" /tmp/xcli-r3-fixed \
  --config "$R3_STUDY/config.json" task status <task-id> --json
python3 tests/r3/experiment.py summarize --study "$R3_STUDY" \
  --out /tmp/r3-demo-summary.json
```

`--out` 必须是尚不存在的新路径；复看和 summarize 不消耗模型调用。录屏时隐藏个人目录、原始流、原生 session、账号和凭据；公开材料使用脱敏报告。
