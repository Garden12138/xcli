# R3 candidate notes / R3 候选说明

This is an unpublished `0.5.0-r3-dev` source/snapshot candidate. No release tag or public package was created. Read the [native comparison record](results.zh-CN.md), [worker recovery follow-up](followup-20261019.zh-CN.md), and [committed-candidate installation acceptance](acceptance-20261019.zh-CN.md) for measured scope and pending gates.

R3 adds repeatable Work/Project comparisons of direct Codex, single-CLI Tasks and Codex/Cursor collaboration, a durable shared invocation budget, independent scoring, native compatibility checks, and installation checks against actual GoReleaser archives. CI runs deterministic protocol fixtures without model credentials. Snapshot archives contain the license and both READMEs.

The first native comparison exposed a source-citation acceptance defect. A unique selected input filename now resolves to the original input and verifies its saved content hash; duplicate names require an explicit path. Relative source paths resolve from the report's directory. An unrelated workspace copy cannot hide a changed input. Task/Run wire formats remain unchanged.

Build from source with Go 1.26, or use a locally generated snapshot archive after verifying `checksums.txt`. Native CLI installation, login and quota remain prerequisites. Runtime commands and persisted Task/Run formats retain their R2 contracts.

本候选新增开发与验收工具，未扩展 Task 主命令、团队模式或自动合并。支持声明按本次真实记录限定；其他 Adapter 与平台的既有实现继续保留，但没有因跨平台构建而获得原生实测结论。

The two original predefined Work/Project comparisons established no collaboration benefit. This candidate makes no claim of higher quality, fewer human handoffs, faster completion, or lower cost. Both single-Codex and cross-CLI background worker recovery passed; the latter was verified after the user confirmed Cursor quota recovery, retaining earlier failures and budget accounting. See the [recovery evidence](cursor-restored-20261019.zh-CN.md) and [version, role and platform matrix](compatibility.zh-CN.md). Installation detection and `doctor` do not establish account quota or model readiness.

原先两个预定义场景均未建立协作收益，当前候选不宣传协作更优、更快、更省或减少人工交接。指定 Codex 及跨 CLI 的实际 worker 中断恢复均已通过，旧额度失败保留；独立参与者观察与协作收益门槛仍待完成。这些原始门槛没有因可靠性、收窄主张或安装通过而自动关闭。

A third preregistered synthetic release-evidence comparison is now complete: all three methods found all six supported issues with no omissions or false positives. It also established no collaboration benefit; cross-CLI execution took longer in this local sample. A participant kit is ready, but no independent user records have been collected. See the [new comparison](value-results-20261019.zh-CN.md) and [field study preparation](field-study-20261019.zh-CN.md).

## Migration / 迁移

- Published v0.4.0 does not include Task. R1/R2/R3 refer to source milestones, not previously published v0.5.0 packages.
- `task --agent <name>` locks all xcli-assigned Agent roles to that CLI. Deterministic verification commands may still run. Automatic mode can use different CLIs for execution and review.
- R1 retired `install`, `auth`, `default`, `acp`, `route`, independent `workflow`, `usage` and MCP import/plan/sync. Use native installation/login, `task`, configuration and Task/Run records. Old YAML workflows require a compatible historical binary or translation to a Task goal.
- `xcli mcp serve` remains compatible with existing launchers. Historical config, ownership, native files and run records are not removed. See [R0 migration](../r0/README.zh-CN.md) before replacing a launcher.
- Stop active tasks before changing executables. Keep private Task data and worktrees. Do not assume older binaries can interpret newer lifecycle state; rollback does not delete or rewrite saved data.

## Remaining gates / 待完成门槛

The user subsequently requested three independent experience-level AI personas for this round's usability review. That [simulation review is complete](persona-results-20261019.zh-CN.md); participant recruitment is no longer a prerequisite for this requested task. Human observation, next-day use and retention remain unmeasured. The earlier participant kit remains available for optional research, without simulated attestations.

A small comparison does not establish general superiority, lower cost or retention; collaboration benefit remains unestablished. Actual host reboot, sustained thirty-minute execution, other OS/architectures and other Adapter combinations remain outside the measured native scope. Formal release requires evidence review against the final candidate, with directory/configuration integrity and truthful success reporting intact.
