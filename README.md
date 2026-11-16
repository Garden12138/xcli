# xcli v2

xcli orchestrates real Codex, Cursor and Antigravity CLI processes to deliver text, coding, reviews and images to local desktop apps through a JSON subprocess protocol.

Version: `2.0.0-dev`. The v2 rebuild is in progress on the `v2` branch.

## Build

Install Go 1.26, then run:

```sh
go build -o bin/xcli .
```

Install and log in to the native CLIs (`codex`, `cursor-agent`, `agy`) to use task capabilities with default settings.

## Planned usage

```sh
xcli ask "Summarize notes.md" -i notes.md --json
xcli image "A green plant on a cream background, square" -o ./covers --json
xcli code "Fix the TTL expiry boundary" -C ~/repo --verify "go test ./..." --json
xcli do "Write a README introduction and generate a cover image" -C ~/repo
```

See [the v2 design and implementation plan](docs/v2/README.md). Previous v0.x work is preserved in tag `archive/r4.1-final`.
