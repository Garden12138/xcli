#!/usr/bin/env bash
# Product gate: native CLI/Task fixtures only. No Gateway readiness or login.
set -euo pipefail
repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repo_root"
export GOCACHE="${GOCACHE:-/tmp/xcli-nag-go-cache}"
export GOPROXY=off GOSUMDB=off
export GORACE="${GORACE:-atexit_sleep_ms=0}"
core_packages=(. ./cmd ./internal/agent ./internal/config ./internal/jobs
  ./internal/mcp ./internal/runstore ./internal/runtime ./internal/task
  ./internal/workflow ./internal/worktree ./tests/phase0)
# These three archived CLI tests require experimental opt-in. R4 default-off
# and cleanup tests remain part of cmd's core product contract.
archived='^(TestModelClientCLIAndConfigurationIsolation|TestAgentGatewayClientCLIIsolation|TestRevokedAgentClientNeedsExplicitReauthorization)$'
go test -race -p 1 -count=1 -timeout 240s -skip "$archived" "${core_packages[@]}"
go vet "${core_packages[@]}"
go mod tidy -diff
artifact_dir="$(mktemp -d /tmp/xcli-r4-core.XXXXXX)"
trap 'rm -rf "$artifact_dir"' EXIT
go build -trimpath -o "$artifact_dir/xcli" .
python3 -m unittest discover -s tests/r0 -p 'test_*.py'
XCLI_R3_BINARY="$artifact_dir/xcli" python3 -m unittest discover -s tests/r3 -p 'test_*.py'
python3 tests/r3/install_acceptance.py --binary "$artifact_dir/xcli" --out "$artifact_dir/installation.json"
git diff --check
printf 'R4_CORE_REGRESSION_PASS\n'
