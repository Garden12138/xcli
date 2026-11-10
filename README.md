# xcli

R4.1 requires an explicit process launch budget window before native CLI execution or probes; see [launch ledger](docs/product-realignment/R4_1_LAUNCH_LEDGER.md) and [user manual](docs/cli-first/XCLI_USER_MANUAL.md).

[English](README.md) | [简体中文](README.zh-CN.md)

xcli schedules and orchestrates native AI Coding Agent CLIs. Each native CLI owns installation, sign-in, accounts, permissions and model access. xcli discovers and selects the CLI, executes bounded tasks, supervises processes, preserves context, recovers interrupted work and delivers verifiable results.

**R4 — CLI-First Product Realignment** is an unpublished `0.5.0-r4-dev` source candidate. Codex CLI, Cursor CLI and Antigravity CLI are explicit entrypoints; the existing Claude, Gemini, OpenCode and generic adapters remain. Published v0.4.0 archives do not contain Task. See the [product decision](docs/product-realignment/README.md), [user manual](docs/cli-first/XCLI_USER_MANUAL.md), and [measured support limits](docs/r3/compatibility.zh-CN.md).

## Discover and run

Install and sign in through each vendor's own CLI first. Build with Go 1.26. The `xcli` examples below refer to this built binary; use its full path or put it on your PATH yourself.

```bash
go build -trimpath -o /tmp/xcli-r4 .
/tmp/xcli-r4 --version
/tmp/xcli-r4 agents --json
/tmp/xcli-r4 doctor codex --json

xcli run --agent codex --cwd ./project "Explain this project" --json
xcli run --agent cursor --cwd ./project "Review this change" --json
xcli run --agent antigravity --cwd ./project "Analyze these materials" --json
```

`agents` discovers installations and versions. `doctor` checks versions/help and declared capabilities; it does not prove authentication, quota or permission enforcement. Native trust or permission failures remain failures. xcli never signs in, switches vendor accounts, exports credentials or changes persistent native permissions.

## Sessions and bounded tasks

```bash
xcli use codex --cwd ./project
xcli resume <run-id> "Continue the review" --json
xcli resume <native-session-id> "Continue" --agent cursor --cwd ./project --json

# Work: selected inputs, output files and review
xcli task "Read the materials, write report.md and check its sources" \
  --agent codex --input ./materials --output ./deliverables

# Project: clean committed local repository, managed worktree and actual tests
xcli task "Fix the bug and have another CLI review the result" --project ./project \
  --verify '["go","test","-count=1","./..."]'

xcli task "Review the selected materials and produce report.md" \
  --agent antigravity --input ./materials --output ./deliverables --detach --json
xcli task status <task-id> --json
xcli task status <task-id> --events --follow
xcli task stop <task-id> --json
xcli task continue <task-id> --detach --json

xcli runs list
xcli runs show <run-id>
xcli jobs list
xcli jobs logs <run-id> --follow
```

Only records with a valid native Session ID can be resumed by Run ID; interactive `use` records may not contain one.

`--agent` locks every Task role to that configured CLI. Without it, the planner assigns work among installed structured adapters; requesting cross-CLI review requires different adapters, not aliases of one adapter. Inspect the recorded plan and review identities. Explicit role-binding flags are a future improvement. Failure never triggers an account or CLI switch to evade quota or permissions.

Default Task limits are 6 Agent invocations, 10 minutes of cumulative execution and 120 seconds per invocation, with one bounded repair. `--max-calls`, `--timeout` and `--call-timeout` adjust those limits. Continuation preserves the candidate pool, bindings and cumulative budget. A missing native session can recover from retained materials; uncertain side effects require inspection and explicit `--reconcile`. See the [manual](docs/cli-first/XCLI_USER_MANUAL.md) and [R2 lifecycle](docs/r2/README.zh-CN.md).

Agent execution, output validation and user acceptance are separate. Task retains real files, hashes, tests, handoffs and reviews; Project retains its worktree, diff and untracked files. Images produced by a native CLI remain native files and need format/source/visual acceptance. xcli does not supply an image-generation API. No automatic commit, merge, deployment or publication occurs.

## Configuration and retained data

Built-in defaults need no configuration. Optional configuration uses `~/.config/xcli/config.yaml`, `XDG_CONFIG_HOME`, or `--config`:

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

`config path/init/validate` manages xcli configuration. Legacy fields remain readable; retired login/routing/workflow-management commands do not return. `mcp serve` remains a compatibility launcher for existing native configurations. R4 does not delete or migrate configuration, Task/Run/Jobs, worktrees, native sessions or artifacts.

Task and Run records live under `${XDG_DATA_HOME:-$HOME/.local/share}/xcli`, with private directories/files. Raw output is off by default; selected inputs, final results and necessary validation diagnostics are still private retained material. Verification runs trusted project commands as argv. Worktrees and native permission flags are not a general OS sandbox.

## Development

```bash
# Core product: CLI/Task/configuration/recovery fixtures, zero model calls
bash tests/product_realignment/verify-core.sh

# Full retained-source regression, including frozen experiment fixtures
bash tests/product_realignment/verify-offline.sh
```

Core CI and packaging use the CLI/Task gate. Frozen experiment regression is separately available by manual workflow. Cross-builds do not establish native runtime support on another platform; Windows background Agent Task/jobs remain unsupported. Historical native evidence and collaboration-benefit limitations remain unchanged.

Model Gateway, Native Agent Gateway HTTP, SIWC/OAuth/Responses and generic Media Gateway are **frozen non-core experiments, disabled by default**. They are not requirements for using or accepting xcli. Source, configurations and evidence remain for compatibility review; see [module disposition](docs/product-realignment/MODULE_DISPOSITION.md). xcli's product goal excludes subscription sharing and a general OpenAI-compatible proxy around CLIs. Application integration is limited to [explicit Agent Tasks](docs/product-realignment/APPLICATION_INTEGRATION_GUIDE.md).
