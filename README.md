# xcli

[简体中文](README.zh-CN.md)

**A unified scheduler and orchestrator for native AI coding agent CLIs.**

Current source version: **`0.5.0-r4.1-dev`**, an unpublished development build. Codex CLI, Cursor CLI and Antigravity CLI keep their own installation, login, accounts, permissions, models and quotas. xcli discovers and executes CLIs, orchestrates Tasks, retains Sessions, monitors background work and delivers inspectable results.

Start with the **[release handoff](docs/release-handoff/README.md)** for the current version, offline acceptance, local installation, first task and Git state. Published v0.4.0 and the unpushed remote main snapshot do not contain this complete candidate.

| Need | Current documentation |
| --- | --- |
| Install and run a first task | [Quick start](docs/release-handoff/QUICK_START.md) |
| macOS Apple Silicon / Intel / Linux setup | [Local installation](docs/release-handoff/LOCAL_INSTALLATION.md) |
| Codex / Cursor / Antigravity, Tasks, Sessions, Jobs, media | [User manual](docs/release-handoff/USER_MANUAL.md) |
| Main / stable Tag changes and migration | [Version comparison](docs/release-handoff/VERSION_COMPARISON.md) |
| Test results and limits | [Automated acceptance](docs/release-handoff/AUTOMATED_ACCEPTANCE.md) |
| Repository and branch audit | [Cleanup](docs/release-handoff/CLEANUP_REPORT.md), [branches](docs/release-handoff/BRANCH_CONSOLIDATION.md) |

Build from this local checkout:

```bash
go build -trimpath -o bin/xcli .
./bin/xcli --version
./bin/xcli --help
```

R4.1 requires an explicitly confirmed native process launch window before execution or version/help probes. Follow the quick start to create one for your own scope; old UNKNOWN histories do not supply a usable balance. No background service is needed. Do not reuse historical examples without checking the current help and window requirements.

Core entries are `agents`, `doctor`, `run`, `use`, `resume`, `task`, `runs`, `jobs`, `config`, `launches` and the compatibility launcher `mcp serve`. Task supports Work/Project, validation, handoffs, bounded repair, background execution and checkpoint recovery. `--agent` locks all roles to one CLI; automatic cross-CLI planning has no fixed worker/reviewer flags. Worktrees are not an OS sandbox. xcli does not automatically commit, merge, deploy or publish.

Offline developer gates:

```bash
bash tests/product_realignment/verify-core.sh
go test ./...
go test -race ./...
go vet ./...
```

Tests use fake native processes; HTTP unit fixtures need local loopback access. Cross-builds do not prove runtime support on other devices; Windows background Tasks/Jobs remain unsupported. Native model availability and historical real acceptance are reported separately.

Model Gateway, Native Agent Gateway HTTP, SIWC/OAuth/Responses and generic Media Gateway are **frozen experiments, disabled by default**. They are not installation requirements or promoted product features. Source, configuration compatibility and historical evidence remain; see [module disposition](docs/product-realignment/MODULE_DISPOSITION.md). The old full offline scripts also explicitly start synthetic experimental services and are not part of this handoff's permitted runtime checks.

Historical R0–R4 material remains under `docs/`; current user instructions are centralized in `docs/release-handoff/`.
