# xcli

[English](README.md) | [简体中文](README.zh-CN.md)

Give a Work or Project goal to native Agent CLIs, with bounded execution, explicit handoffs and verifiable results.

The current source is **R1 / `0.5.0-r1-dev`**, an unreleased development version. Published v0.4.0 packages do not contain Task. Read the [R1 behavior and migration guide](docs/r1/README.zh-CN.md) and [acceptance evidence](docs/r1/results.zh-CN.md) for actual validation limits.

## Start a task

Install and authenticate your preferred CLI using its native instructions. xcli reuses existing authentication and tool configuration; it does not require a model API account.

```bash
go build -o xcli .
./xcli agents
./xcli doctor codex

# Work: produce a document from selected files, with Codex self-check
./xcli task "Read the materials and write report.md with conclusions and sources" \
  --input ./materials --output ./deliverables --agent codex

# Automatic collaboration; a request for another CLI requires another adapter
./xcli task "Prepare a report and have another CLI check the conclusions" \
  --input ./materials --output ./deliverables

# Project: isolated worktree from a clean local Git repository
./xcli task "Fix the cache invalidation bug and pass project tests" \
  --project . --agent codex

# Explicit argv overrides discovered validation commands; repeat as needed
./xcli task "Fix the bug and have another CLI review it" --project . \
  --verify '["go","test","-count=1","./..."]' --json

./xcli task status <task-id> --json
```

`--agent` locks all xcli Agent invocations to the configured CLI, including planning, repair and self-check. It never silently falls back to another CLI. Without it, available structured adapters are selected using the configured default preference and deterministic candidate ordering; the coordinator assigns roles. Different aliases of the same adapter do not count as cross-CLI review.

Default bounds are 10 minutes total, 6 Agent CLI invocations, 120 seconds per invocation, and one repair followed by revalidation. Change total limits with `--timeout` and `--max-calls`. Native internal model calls and subagents may not be observable; invocation counts are not model request counts.

## Results and verification

A Task has its own ID, linked Runs and native Sessions, plan versions, handoffs, artifact hashes, actual command checks and reviews. Runtime acceptance, Agent execution and user acceptance are separate. A successful native process or an Agent saying “done” cannot bypass output checks.

Project tasks preserve a managed worktree, baseline diff, untracked files and validation evidence. Command verification runs argv directly. Known local test families can be discovered automatically; unsupported entrypoints need explicit `--verify`. Missing Project validation produces `waiting_user`, not completion. Tests and reviewers are associated with a content snapshot; changed source invalidates old evidence. Reviewers receive fixed material copies.

Tests execute trusted project code. Argument checks, Git worktrees and native permission flags are not a general OS sandbox. xcli does not automatically install tools, change persistent permissions or publish results. Failures retain material for inspection. Review the worktree and complete diff, then commit or copy changes using your normal Git process.

Progress goes to stderr and a human result summary to stdout. `--json` emits one Task object; `--events` emits sequenced JSONL events with the final result. The flags are mutually exclusive. `task status` reads saved state.

Task background execution, stop/continue, checkpoints and stale-owner recovery are **R2**, not implemented in R1. Ctrl-C cancels the current Task. `waiting_user` preserves missing requirements; supply them in a new Task with explicitly selected old materials for now.

## Configuration and adapters

No xcli configuration is required for built-in defaults. Optional configuration uses `~/.config/xcli/config.yaml`, `XDG_CONFIG_HOME`, or `--config`:

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

Adapters exist for Codex, Cursor, Antigravity, Claude Code, Gemini and OpenCode. Generic aliases require an explicitly configured structured output format for Task participation. Availability, authentication, quota and native permission enforcement are separate; see the recorded validation matrix instead of assuming every adapter was tested live. Task native role policies currently map Codex read-only/workspace-write, Cursor ask/sandbox enabled and Antigravity plan/accept-edits. xcli does not automatically trust new workspaces.

`config path/init/validate`, `agents` and `doctor` remain available. Agent `args`, `env`, network configuration and native credentials remain under user control. `doctor` reports observed installation/version/help capabilities and leaves unknown authentication or enforcement unverified.

## Data and retained execution commands

Task goals, selected inputs and artifact hashes, plans, handoffs, fixed review material and bounded validation diagnostics persist in the private XDG data directory (`~/.local/share/xcli` by default). Directories use 0700 and files 0600. Native raw output is off by default and controlled by `recording.output`; verification diagnostics are necessary private repair material. Credentials and arbitrary tool payloads are not automatically added to handoffs.

```bash
./xcli run "Explain this project" --agent codex --cwd . --json
./xcli use codex
./xcli resume <run-id-or-native-session> "Continue checking compatibility"
./xcli runs list
./xcli runs show <run-id>
./xcli jobs list
```

`run/use/resume`, `jobs`, `runs`, Agent discovery and configuration remain lower-level tools. Run selection uses an explicit Agent or `default_agent`; legacy prompt-routing rules are ignored.

R1 removes `install`, `auth`, `default`, `acp`, `route`, standalone `workflow`, `usage` and MCP import/plan/sync. R0 commit `5a2a676` was the last source milestone retaining them. Use native installation/login, Task, configuration and Task/Run usage instead. Existing configurations, MCP ownership, historical Runs and native files are not deleted. **`xcli mcp serve` remains compatible** for existing native launcher references; follow the [migration notes](docs/r0/README.zh-CN.md) before replacing those launchers.

## Development and validation

Go 1.26 is required. Release checks include formatting, `go mod tidy -diff`, `go vet ./...`, `go test -race ./...`, and Darwin/Linux amd64/arm64 builds. CI also runs a GoReleaser snapshot before release.

```bash
go test -race ./...
go vet ./...
go mod tidy -diff

# Explicit live acceptance: four temporary tasks, at most 24 Agent invocations
python3 tests/r1/native.py --binary ./xcli --out /tmp/r1-native.json --execute
```

The live harness uses default Codex/Cursor login environments, temporary project/data directories, and Cursor trust only for generated test directories. It preserves private failure evidence and emits sanitized public results. Historical R0 and phase-0 records retain their original validation scope; [R1 results](docs/r1/results.zh-CN.md) describe this development milestone.
