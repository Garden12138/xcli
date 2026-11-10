#!/usr/bin/env bash
# G8 and all existing deterministic regression gates. No native login/inference.
set -euo pipefail
repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repo_root"
export GOCACHE="${GOCACHE:-/tmp/xcli-nag-go-cache}"
export GOPROXY=off GOSUMDB=off
export GORACE="${GORACE:-atexit_sleep_ms=0}"
# Do not enter the opt-in G7 container experiment during ordinary regression.
unset XCLI_G7_RUN_CONTAINER_TEST
go test -race -p 1 -count=1 -timeout 240s -v ./internal/siwc
bash tests/native_agent_gateway/verify-offline.sh
go mod tidy -diff
unformatted="$(gofmt -l internal/siwc internal/modelstore/config.go internal/modelstore/siwc_test.go internal/modelgateway/siwc_test.go)"
test -z "$unformatted"
artifact_dir="$(mktemp -d /tmp/xcli-g8-build.XXXXXX)"
trap 'rm -rf "$artifact_dir"' EXIT
go build -trimpath -o "$artifact_dir/xcli" .
XCLI_R3_BINARY="$artifact_dir/xcli" python3 -m unittest discover -s tests/r3 -p 'test_*.py'
python3 -m unittest discover -s tests/model_gateway -p 'test_*.py'
python3 tests/model_gateway/lifecycle.py --binary "$artifact_dir/xcli"
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64; do
  GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=0 \
    go build -trimpath -o "$artifact_dir/xcli-${target%/*}-${target#*/}" .
  printf 'BUILD_PASS %s\n' "$target"
done
git diff --check
printf 'G8_OFFLINE_REGRESSION_PASS\n'
