#!/usr/bin/env bash
# Full retained-source regression; not a live Gateway/product-readiness gate.
set -euo pipefail
repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$repo_root"
export GOCACHE="${GOCACHE:-/tmp/xcli-nag-go-cache}"
export GOPROXY=off GOSUMDB=off
export GORACE="${GORACE:-atexit_sleep_ms=0}"
unset XCLI_G7_RUN_CONTAINER_TEST
bash tests/native_agent_gateway/verify-offline.sh
go mod tidy -diff
test -z "$(gofmt -l cmd internal tests/model_gateway)"
artifact_dir="$(mktemp -d /tmp/xcli-r4-full.XXXXXX)"
trap 'rm -rf "$artifact_dir"' EXIT
go build -trimpath -o "$artifact_dir/xcli" .
XCLI_R3_BINARY="$artifact_dir/xcli" python3 -m unittest discover -s tests/r3 -p 'test_*.py'
python3 -m unittest discover -s tests/model_gateway -p 'test_*.py'
python3 tests/model_gateway/lifecycle.py --binary "$artifact_dir/xcli"
python3 tests/r3/install_acceptance.py --binary "$artifact_dir/xcli" --out "$artifact_dir/installation.json"
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64; do
  GOOS="${target%/*}" GOARCH="${target#*/}" CGO_ENABLED=0 \
    go build -trimpath -o "$artifact_dir/xcli-${target%/*}-${target#*/}" .
  printf 'BUILD_PASS %s\n' "$target"
done
git diff --check
printf 'R4_FULL_REGRESSION_PASS\n'
