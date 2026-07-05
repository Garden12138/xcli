# xcli

[English](README.md) | [简体中文](README.zh-CN.md)

[Manual testing guide (简体中文)](docs/manual-testing.zh-CN.md)

`xcli` is a unified entry point for installing, running, switching between, and orchestrating mainstream AI coding agent CLI tools.

It manages the real CLI processes rather than replacing them with model API calls. Native authentication, permissions, plugins, MCP configuration, and agent-specific flags remain available.

## Supported agents

| Agent | Interactive | Non-interactive | Structured workflow output | ACP | npm | Homebrew |
| --- | --- | --- | --- | --- | --- | --- |
| [Claude Code](https://code.claude.com/docs/en/cli-usage) | `claude` | `claude -p` | stream JSON | `claude-agent-acp` bridge | `@anthropic-ai/claude-code` | — |
| [Codex CLI](https://developers.openai.com/codex/cli) | `codex` | `codex exec` | JSONL | `codex-acp` bridge | `@openai/codex` | `--cask codex` |
| [Gemini CLI](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/headless.md) | `gemini` | `gemini -p` | stream JSON | native `--acp` | `@google/gemini-cli` | `gemini-cli` |
| [OpenCode](https://opencode.ai/docs/cli/) | `opencode` | `opencode run` | JSON events | native `acp` | `opencode-ai` | `anomalyco/tap/opencode` |

Declarative aliases and custom agents can be added without rebuilding xcli.

## Install

Download a prebuilt archive from the [GitHub Releases](https://github.com/Garden12138/xcli/releases) page. Releases are available for macOS and Linux on amd64 and arm64.

Choose one of `darwin_amd64`, `darwin_arm64`, `linux_amd64`, or `linux_arm64`, then verify and install the archive:

```bash
VERSION=0.2.0
PLATFORM=darwin_arm64
ARCHIVE="xcli_${VERSION}_${PLATFORM}.tar.gz"

curl -fLO "https://github.com/Garden12138/xcli/releases/download/v${VERSION}/${ARCHIVE}"
curl -fLO "https://github.com/Garden12138/xcli/releases/download/v${VERSION}/checksums.txt"

# Linux: replace `shasum -a 256` with `sha256sum`
grep -F "  ${ARCHIVE}" checksums.txt | shasum -a 256 --check

tar -xzf "${ARCHIVE}"
mkdir -p "$HOME/.local/bin"
mv xcli "$HOME/.local/bin/xcli"
xcli --version
```

The macOS archives are not code-signed or notarized. Always verify the published SHA-256 checksum before running a downloaded binary.

### Build from source

xcli targets Go 1.26 and produces a standalone binary:

```bash
go build -o xcli .
./xcli --help
```

## Quick start

```bash
# Create ~/.config/xcli/config.yaml
xcli config init

# Inspect configured and installed agents
xcli agents
xcli doctor

# Install and authenticate with native tooling
xcli install codex
xcli auth login codex

# Select and use a default agent
xcli default codex
xcli use

# Expose the selected agent to an ACP client over stdio
xcli acp codex

# Preview and synchronize declared MCP servers
xcli mcp plan
xcli mcp sync
xcli mcp plan --scope project --project .

# Safely import existing native MCP entries
xcli mcp import plan
xcli mcp import apply

# Run one task; agent-specific arguments follow --
xcli run codex "Review the current changes"
xcli run "Fix the failing tests" -- --sandbox workspace-write
xcli route "Review the authentication changes"

# Resume a recorded session, interactively or with a follow-up prompt
xcli resume <run-id>
xcli resume <run-id> "Continue with the next step" --json

# Run a non-interactive task in the background
xcli run --detach "Run the full test suite"
xcli jobs list

# Run a workflow (parallelism is opt-in)
xcli workflow validate examples/implement-and-review.yaml
xcli workflow run examples/implement-and-review.yaml \
  --var requirement="Implement the cache invalidation fix"

# Summarize recorded token usage and native cost estimates
xcli usage --days 7
```

`--var` overrides variables declared by the workflow; undeclared keys are rejected.

`xcli run` treats the first positional argument as an agent only when it exactly matches a configured agent. `--agent` can always be used to remove ambiguity.

## Configuration

xcli loads only the user-level configuration by default:

```text
$XDG_CONFIG_HOME/xcli/config.yaml
~/.config/xcli/config.yaml
```

Repository configuration is never loaded implicitly. Pass `--config path/to/config.yaml` when a project configuration is trusted.

See [`examples/config.yaml`](examples/config.yaml) for proxy routing, direct connections, agent aliases, and native default arguments.

### Custom agents

Custom commands are argv templates, not shell snippets:

```yaml
agents:
  custom:
    adapter: generic
    command: my-agent
    interactive_args: []
    run_args: ["run", "{{ prompt }}"]
    auth_args: ["auth", "login"]
    output: text
    acp:
      command: my-agent-acp
      args: ["--stdio"]
```

The prompt remains one process argument even when it contains spaces, semicolons, or shell metacharacters. Supported output modes are `text`, `json`, and `jsonl`.

### ACP stdio entry point

`xcli acp [agent] [-- native-args...]` starts the selected agent's [Agent Client Protocol](https://agentclientprotocol.com/) server with stdin, stdout, and stderr connected directly to the ACP client. A positional agent overrides `default_agent`; prompt routing is not involved because xcli does not inspect the protocol stream. `--cwd` sets the child process directory, and the usual agent environment and network profile still apply.

Gemini and OpenCode use their native ACP commands. Claude and Codex require the maintained bridges to be installed explicitly; xcli never downloads them while starting a protocol server:

```bash
npm install -g @agentclientprotocol/claude-agent-acp
npm install -g @agentclientprotocol/codex-acp
```

The optional `agents.<name>.acp` block is a complete command override: its arguments replace ordinary agent `args`, while arguments after `--` are appended. Generic agents must define this block before they can be used with `xcli acp`.

xcli does not parse, capture, or persist ACP messages. One connection may contain multiple sessions and tasks, so ACP processes do not create run records and do not contribute to `xcli usage`. Protocol version negotiation, authentication, permissions, and capability handling remain the responsibility of the downstream ACP server and client.

### MCP configuration synchronization

Declare MCP servers once and synchronize them to installed Claude, Codex, Gemini, and OpenCode clients at user or project scope:

```yaml
mcp:
  servers:
    local-tools:
      transport: stdio
      command: npx
      args: ["-y", "@example/mcp-server"]
      cwd: ./tools
      env:
        LOG_LEVEL: info
      env_vars: [SERVICE_TOKEN]
      targets: [claude, codex, gemini, opencode]

    docs:
      transport: http
      url: https://example.com/mcp
```

`targets` defaults to all four built-in adapters. Relative `cwd` values resolve from the xcli configuration directory. `env_vars` names are copied only when `xcli mcp serve` starts the local server and must exist at that time; use them instead of putting secrets in `env`.

For stdio servers, native clients launch xcli, which then applies the configured command, working directory, and minimal environment. HTTP URLs are written directly and each native client owns its OAuth login. The first release does not synchronize SSE, static authentication headers, tool policies, or vendor-specific OAuth fields.

```bash
xcli mcp plan
xcli mcp plan --target codex --json
xcli mcp sync
xcli mcp sync --yes
```

User scope is the default and keeps the existing machine-local behavior. Its stdio launcher and source configuration paths are absolute. If xcli was built under a temporary directory, pass a stable installed path with `--launcher`; moving either path requires another sync. User-level OpenCode JSONC edits preserve comments and unrelated settings, create an `.xcli.bak` backup, and are written atomically.

Project scope writes portable shared configuration and requires the xcli source configuration to live inside the selected project:

```bash
xcli --config .xcli/config.yaml mcp plan --scope project --project .
xcli --config .xcli/config.yaml mcp sync --scope project --project . --yes
```

The project launcher defaults to the literal PATH command `xcli`. `--launcher` may select another PATH command name, but absolute paths and path separators are rejected. Project stdio entries store only that command name, `mcp serve --project-config <relative-path> <server>`, and environment variable names—never the current machine's launcher/config paths or variable values. Every project member must install the launcher in PATH and keep the source configuration at the same relative location.

Project files are `.mcp.json` (Claude), `.codex/config.toml` (Codex), `.gemini/settings.json` (Gemini), and `opencode.json` (OpenCode). JSONC/TOML comments and unrelated settings are preserved; new files use `0644`, existing modes are retained, and atomic writes leave no backup file in the repository. Codex loads project configuration only for trusted projects; Claude project approval and native OAuth flows remain client responsibilities.

See [`examples/project-mcp.yaml`](examples/project-mcp.yaml) for a shareable source configuration.

`sync` always shows the sorted add/update/remove plan before asking for confirmation; automation must pass `--yes`. xcli tracks only entries it owns in the scope/project namespaces of `$XDG_DATA_HOME/xcli/mcp-sync.json`; version 1 user ownership is migrated automatically. Same-name native entries, ownership from another xcli source/scope/project, and external edits are conflicts; inspect them before using `--force`. Removing a server or target from xcli schedules removal only for an unchanged managed entry.

### Importing native MCP configuration

Use the import workflow to pull existing common MCP definitions into xcli without modifying the native files:

```bash
xcli mcp import plan
xcli mcp import apply

xcli --config .xcli/config.yaml mcp import plan --scope project --project .
xcli --config .xcli/config.yaml mcp import apply --scope project --project . --yes
```

Import reads native configuration files directly and does not require the corresponding CLI to be installed. Without `--target`, it scans the files that exist in the selected scope; an explicitly selected target with no file is treated as empty. Equivalent same-name definitions are merged into one server with explicit target coverage. Different native definitions conflict, while `--force` can replace a different xcli definition, accept native drift, or take over ownership. Import is additive and never deletes an xcli server merely because it is absent natively.

Only definitions that map losslessly to xcli's stdio or Streamable HTTP schema are imported. Static environment values, headers, OAuth options, SSE/WebSocket, disabled entries, timeout/tool policy fields, variable-expanded commands, and other vendor fields are reported as `unsupported` and skipped without exposing their values. Project-relative working directories are rewritten relative to the xcli source; machine-specific project paths and ambiguous user-relative directories are skipped.

`import apply` preserves YAML comments, ordering, unrelated settings, and file permissions. A missing user source is created as a minimal private (`0600`) version 1 configuration. Applied entries are explicitly claimed in the ownership state, so HTTP entries can immediately be a sync noop and direct stdio entries can be converted to the xcli launcher by the next `mcp sync`. Existing xcli launcher wrappers are recognized and never recursively imported. See [`examples/native-mcp-claude.json`](examples/native-mcp-claude.json) for an importable native fixture.

### Prompt routing

When `xcli run` has no explicit agent, ordered routing rules can select one by matching the complete prompt with Go regular expressions:

```yaml
default_agent: codex

routing:
  rules:
    - name: review
      prompt_regex: '(?i)(review|audit|审查)'
      agent: claude
```

The first matching rule wins. If none match, xcli uses `default_agent`; if neither produces an agent, the command fails without starting a process. Case sensitivity follows the regular expression, so use `(?i)` when matching should be case-insensitive.

Selection precedence is `--agent`, a configured agent in the first positional argument, the first matching routing rule, then `default_agent`. Explicit selections never evaluate routing rules. Use `xcli route <prompt>` or `xcli route --json <prompt>` to inspect the rule decision without starting an agent or creating a run record. Routing applies only to `run`; interactive sessions and workflow steps retain their explicit agent behavior.

### Session resume

`xcli resume <target> [prompt...]` continues a native session using Claude's `--resume`, Codex's `resume`/`exec resume`, Gemini's `--resume`, or OpenCode's `--session`. With no prompt, xcli connects the terminal directly to the restored interactive session. With a prompt, it uses the same structured output, normalized result, usage capture, and `--json` behavior as `run`:

```bash
# Restore the agent, session ID, and working directory from an xcli record
xcli resume run-20260705T010203Z-ab12cd34
xcli resume run-20260705T010203Z-ab12cd34 "Implement the remaining tests" --json

# A workflow record needs an explicit step
xcli resume workflow-20260705T020304Z-ef56ab78 --step review "Address the findings"

# Sessions created outside xcli require an explicit configured agent
xcli resume 7f9f9a2e-1b3c-4c7a-9b0e-000000000000 --agent codex
```

Record lookup always happens first. If no matching record exists, xcli treats the target as a native session identifier only when `--agent` is present; this prevents a misspelled run ID from silently using `default_agent`. A record supplies its original agent and `cwd`; a matching `--agent` is accepted, a conflicting one fails, and `--cwd` can replace a directory that moved or no longer exists. Resume never evaluates prompt routing or `default_agent`, `--json` requires a follow-up prompt, and generic agents are not supported in this first release.

Interactive resumes create `use` records and remain excluded from `xcli usage`. Non-interactive resumes create ordinary `run` tasks with `selection_source: resume`, plus `resumed_from` and optional `resumed_step` metadata. If the native client reports a new session ID, the new record uses it; otherwise it retains the requested ID.

### Background jobs

Add `--detach` to a non-interactive `run` to start it in an independent process session and return immediately. Agent selection, prompt routing, cwd, environment profiles, structured parsing, usage capture, and native arguments stay the same as a foreground run:

```bash
xcli run --detach "Run the slow integration tests"
xcli run codex --detach --json "Review the repository" -- --sandbox workspace-write

xcli jobs list
xcli jobs show <run-id>
xcli jobs logs <run-id>
xcli jobs logs <run-id> --follow
xcli jobs stop <run-id>
```

`run --detach --json` returns the initial Job object rather than a final `RunResult`. Job JSON contains the ID, agent, status, worker PID, cwd, timestamps, and private log path, then adds exit code, session ID, and usage when available. `jobs list` includes only background records; `jobs show` always returns one Job as JSON.

Each job keeps a private `0600` normalized log. Native stderr is appended live. Plain generic stdout is appended live, while structured agents append only their normalized final text when they finish, so `jobs logs` does not expose JSONL events. Raw structured stdout is still saved only when `recording.output` is enabled. These logs may contain source code, diagnostics, or secrets and have no automatic retention policy.

`jobs stop` first sends TERM to the complete job process group, waits five seconds by default, then escalates to KILL. Use `--timeout` to change the grace period or `--force` to kill immediately. Repeating stop on a completed job is safe. xcli verifies the job's held lock before signaling its PID; a nonterminal record without a live lock becomes `orphaned` instead of risking a signal to an unrelated reused PID.

Background jobs survive the launching terminal but not a machine restart. This first release has no daemon, restart, attach, deletion, background workflow, background resume, or interactive background mode.

### Network profiles

A child process inherits the current environment, then xcli applies the selected network profile and agent-specific environment variables. A direct profile can remove both upper- and lower-case proxy variables:

```yaml
networks:
  direct:
    unset: [HTTP_PROXY, HTTPS_PROXY, ALL_PROXY, http_proxy, https_proxy, all_proxy]
```

xcli does not store API keys. Authentication stays with each native CLI, and environment values are never printed by xcli.

## Workflows

Workflows default to one step at a time for backward compatibility. Set `max_parallel` to run independent steps concurrently; `--max-parallel` can override the file for one invocation. Explicit `depends_on` entries and step-result template references both delay a step until its prerequisites succeed:

```yaml
version: 1
name: parallel-review
cwd: .
max_parallel: 2
steps:
  - id: correctness
    agent: codex
    prompt: Review the current changes for correctness.

  - id: security
    agent: claude
    prompt: Review the current changes for security issues.

  - id: summarize
    agent: codex
    depends_on: [correctness, security]
    prompt: |
      Combine these reviews:
      {{ steps.correctness.output }}
      {{ steps.security.output }}
```

Dependencies and template references must still point to earlier declarations, and execution summaries remain in declaration order even when completion order differs. See [`examples/parallel-review.yaml`](examples/parallel-review.yaml) for a complete fan-out/fan-in workflow.

Supported references are:

- `{{ vars.name }}`
- `{{ steps.id.output }}`
- `{{ steps.id.output_file }}`
- `{{ steps.id.session_id }}`

Inline output is limited to 128 KiB. Larger results must be passed through `output_file`. Steps default to a 30-minute timeout, no retries, and fail-fast behavior. A fatal failure or timeout cancels running sibling steps and skips pending steps. Use `continue_on_error: true` only when independent branches should continue; dependents are still skipped and the workflow still fails.

Parallel steps keep the existing shared `cwd` behavior. xcli does not create isolated worktrees, so steps that can write files should use separate `cwd` values or otherwise coordinate access. Concurrent stderr is streamed live and may be interleaved.

## Run records, usage, and privacy

xcli stores private (`0600`) metadata under `$XDG_DATA_HOME/xcli/runs` or `~/.local/share/xcli/runs`. Metadata includes the agent, working directory, timestamps, status, exit code, native session ID, resume parent/step, background PID/log information when applicable, and normalized token usage when the structured output exposes them.

Full output is disabled by default because it may contain source code or secrets. Enable it explicitly with workflow `--record-output` or `recording.output: true`.

```bash
xcli runs list
xcli runs show <run-id>
```

Built-in agents use their machine-readable mode for non-interactive runs. xcli captures those events and prints only the normalized final message to stdout; native progress and diagnostics continue on stderr. Generic agents keep their existing output behavior and do not have a usage contract.

Use `xcli usage` to aggregate one-off runs and attempted workflow steps. Workflow retries are accumulated into their logical step. Interactive `use` sessions are excluded, while legacy and generic tasks appear as untracked so coverage stays visible:

```bash
xcli usage
xcli usage --days 30 --agent claude
xcli usage --json
```

Codex and Gemini report tokens but not a dollar estimate. Claude and OpenCode may report a native `estimated_cost_usd`; it is a client-side estimate rather than authoritative billing. The `TRACKED` and `COSTED` columns show coverage, and a missing estimate remains distinct from an explicitly reported zero. Usage metadata never includes the prompt or model output.

## Safety boundaries

- Installers invoke npm or Homebrew directly without `sudo`, a shell, or hidden `curl | sh` pipelines.
- Install commands are shown before execution and require confirmation; automation must pass `--yes`.
- Unknown YAML fields, invalid templates, missing networks, and forward workflow dependencies fail validation.
- xcli does not add telemetry or automatically trust repository configuration.

CAP, automatic/continuous MCP reconciliation, project auto-discovery, vendor-specific advanced MCP fields, daemons, background workflows and interactive sessions, job restart/attach, Windows ConPTY, and a web UI are intentionally deferred beyond v0.2.
