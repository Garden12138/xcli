# xcli v2

xcli orchestrates real Codex, Cursor and Antigravity CLI processes to deliver text, coding, reviews and images to local desktop apps through a JSON subprocess protocol.

Version: `2.1.0`. Supports macOS and Linux (amd64 / arm64). Download binaries from [Releases](https://github.com/Garden12138/xcli/releases).

## Build

Install Go 1.26, then run:

```sh
go build -o bin/xcli .
export PATH="$PWD/bin:$PATH"
```

Install and log in to the native CLIs (`codex`, `cursor-agent`, `agy`) to use task capabilities with default settings.

## Usage

```sh
xcli ask "Summarize notes.md" -i notes.md --json
xcli image "A green plant on a cream background, square" -o ./covers --json
xcli code "Fix the TTL expiry boundary" -C ~/repo --verify "go test ./..." --json
xcli review "Check concurrency and boundary conditions" -C ~/repo --json
xcli do "Write a README introduction and generate a cover image" -C ~/repo
```

Apps can send a Request JSON to `xcli submit --request - --json`, or read JSONL progress with `--events`. Use `--detach` for background tasks; `xcli serve` provides an optional local HTTP task API.

See the [user guide (Chinese)](docs/v2/USER_GUIDE.md), [protocol](docs/v2/03-interfaces.md), and [design](https://github.com/Garden12138/xcli/tree/v2.1.0/docs/v2). Run `scripts/smoke.sh` for a manual smoke test; it asks before about 10 real CLI calls that consume quota. Previous v0.x work is preserved in tag `archive/r4.1-final`.
