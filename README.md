# xcli

[English](README.md) | [简体中文](README.zh-CN.md)

[Manual testing guide (简体中文)](docs/manual-testing.zh-CN.md)

`xcli` is a unified entry point for installing, running, switching between, and orchestrating mainstream AI coding agent CLI tools.

It manages the real CLI processes rather than replacing them with model API calls. Native authentication, permissions, plugins, MCP configuration, and agent-specific flags remain available.

## Supported agents

| Agent | Interactive | Non-interactive | Structured workflow output | npm | Homebrew |
| --- | --- | --- | --- | --- | --- |
| [Claude Code](https://code.claude.com/docs/en/cli-usage) | `claude` | `claude -p` | stream JSON | `@anthropic-ai/claude-code` | — |
| [Codex CLI](https://developers.openai.com/codex/cli) | `codex` | `codex exec` | JSONL | `@openai/codex` | `--cask codex` |
| [Gemini CLI](https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/headless.md) | `gemini` | `gemini -p` | stream JSON | `@google/gemini-cli` | `gemini-cli` |
| [OpenCode](https://opencode.ai/docs/cli/) | `opencode` | `opencode run` | JSON events | `opencode-ai` | `anomalyco/tap/opencode` |

Declarative aliases and custom agents can be added without rebuilding xcli.

## Install

Download a prebuilt archive from the [GitHub Releases](https://github.com/Garden12138/xcli/releases) page. Releases are available for macOS and Linux on amd64 and arm64.

Choose one of `darwin_amd64`, `darwin_arm64`, `linux_amd64`, or `linux_arm64`, then verify and install the archive:

```bash
VERSION=0.1.0
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

# Run one task; agent-specific arguments follow --
xcli run codex "Review the current changes"
xcli run "Fix the failing tests" -- --sandbox workspace-write

# Run a serial workflow
xcli workflow validate examples/implement-and-review.yaml
xcli workflow run examples/implement-and-review.yaml \
  --var requirement="Implement the cache invalidation fix"
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
```

The prompt remains one process argument even when it contains spaces, semicolons, or shell metacharacters. Supported output modes are `text`, `json`, and `jsonl`.

### Network profiles

A child process inherits the current environment, then xcli applies the selected network profile and agent-specific environment variables. A direct profile can remove both upper- and lower-case proxy variables:

```yaml
networks:
  direct:
    unset: [HTTP_PROXY, HTTPS_PROXY, ALL_PROXY, http_proxy, https_proxy, all_proxy]
```

xcli does not store API keys. Authentication stays with each native CLI, and environment values are never printed by xcli.

## Workflows

Workflows run one step at a time in declaration order. Code changes pass naturally through the shared working directory; textual results are injected only by explicit references:

```yaml
version: 1
name: implement-and-review
cwd: .
steps:
  - id: implement
    agent: codex
    prompt: Implement the requested feature.

  - id: review
    agent: claude
    depends_on: [implement]
    prompt: "Review the implementation: {{ steps.implement.output }}"
```

Supported references are:

- `{{ vars.name }}`
- `{{ steps.id.output }}`
- `{{ steps.id.output_file }}`
- `{{ steps.id.session_id }}`

Inline output is limited to 128 KiB. Larger results must be passed through `output_file`. Steps default to a 30-minute timeout, no retries, and fail-fast behavior. Use `continue_on_error: true` only when later independent steps should still run.

## Run records and privacy

xcli stores private (`0600`) metadata under `$XDG_DATA_HOME/xcli/runs` or `~/.local/share/xcli/runs`. Metadata includes the agent, working directory, timestamps, status, exit code, and a native session ID when the structured output exposes one.

Full output is disabled by default because it may contain source code or secrets. Enable it explicitly with workflow `--record-output` or `recording.output: true`.

```bash
xcli runs list
xcli runs show <run-id>
```

## Safety boundaries

- Installers invoke npm or Homebrew directly without `sudo`, a shell, or hidden `curl | sh` pipelines.
- Install commands are shown before execution and require confirmation; automation must pass `--yes`.
- Unknown YAML fields, invalid templates, missing networks, and forward workflow dependencies fail validation.
- xcli does not add telemetry or automatically trust repository configuration.

Parallel workflows, automatic routing, cost aggregation, ACP/CAP, MCP synchronization, daemons, process control, session resume, Windows ConPTY, and a web UI are intentionally deferred beyond v0.1.
