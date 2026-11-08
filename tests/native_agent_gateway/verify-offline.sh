#!/usr/bin/env bash
# Entirely offline. Go fixtures are the only CLI processes in this gate.
set -euo pipefail
repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repo_root"
export GOCACHE="${GOCACHE:-/tmp/xcli-nag-go-cache}"
export GOPROXY=off GOSUMDB=off
export GORACE="${GORACE:-atexit_sleep_ms=0}"
# Serialize packages so OS process startup under a full race build does not
# consume the production probe's fixed 3s deadline. In-package race/concurrency
# tests still run unchanged, and no test result is reused from the cache.
go test -race -p 1 -count=1 -timeout 240s ./...
go vet ./...
artifact_dir="$(mktemp -d /tmp/xcli-nag-build.XXXXXX)"
trap 'rm -rf "$artifact_dir"' EXIT
go build -trimpath -o "$artifact_dir/xcli" .
python3 tests/native_agent_gateway/control_smoke.py --binary "$artifact_dir/xcli"
python3 -m unittest discover -s tests/r0 -p 'test_*.py'
python3 -m unittest discover -s tests/native_agent_gateway -p 'test_*.py'
git diff --check
