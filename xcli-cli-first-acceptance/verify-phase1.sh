#!/usr/bin/env bash
# xcli CLI-first Phase 1 verification. No model calls unless --native-smoke.
set -uo pipefail
MODE_SMOKE=0
MODE_RACE=0
for arg in "$@"; do
  case "$arg" in
    --native-smoke) MODE_SMOKE=1 ;;
    --race) MODE_RACE=1 ;;
    -h|--help)
      cat <<'EOF'
Usage: bash verify-phase1.sh [--race] [--native-smoke]
  Default: Go tests, vet, build, CLI version discovery, xcli agents/doctor and gateway lifecycle.
  --race: additionally run go test -race ./...
  --native-smoke: opt in to existing tests/r0/smoke.py --execute (up to 6 native CLI launches; may consume account quotas). No retries.
No Ollama, no external provider API keys, no login, no model Gateway native CLI bridge is assumed.
EOF
      exit 0;;
    *) echo "Unknown option: $arg" >&2; exit 2;;
  esac
done
command -v git >/dev/null || { echo 'git missing'; exit 2; }
command -v go >/dev/null || { echo 'go missing'; exit 2; }
ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo 'Run in the xcli repository'; exit 2; }
cd "$ROOT" || exit 2
if [[ ! -f go.mod ]] || ! grep -q 'module github.com/Garden12138/xcli' go.mod; then
  echo 'Not Garden12138/xcli'; exit 2
fi
umask 077
REPORT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/xcli-cli-phase1.XXXXXXXX")" || exit 2
# correct accidental trailing slash in parent parameterization is handled by mktemp host.
BINARY="$REPORT_DIR/xcli"
SUMMARY="$REPORT_DIR/summary.txt"
FAIL=0
say() { printf '%s\n' "$*" | tee -a "$SUMMARY"; }
run_check() {
  local label="$1"; shift
  say "--- $label ---"
  if "$@"; then say "PASS $label"; else say "FAIL $label"; FAIL=1; fi
}
say "xcli CLI-first phase1: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
say "repo: $ROOT"
say "evidence directory: $REPORT_DIR (private, not automatically uploaded)"
say "HEAD: $(git rev-parse --short HEAD)"
say "Git modified/tracked status count: $(git status --porcelain | wc -l | tr -d ' ')"
say "Go version: $(go version)"
say '--- Native CLI installation discovery (no model requests) ---'
for bin in codex agy cursor-agent agent; do
  if command -v "$bin" >/dev/null 2>&1; then
    printf '%s: installed\n' "$bin" | tee -a "$SUMMARY"
    # Avoid logging paths, account names or untrusted verbose help text.
    if "$bin" --version >/dev/null 2>&1; then say "PASS $bin --version"; else say "WARN $bin --version failed"; fi
  else
    say "NOT_TESTED $bin: not found"
  fi
done
run_check 'go test ./...' go test ./...
run_check 'go vet ./...' go vet ./...
run_check 'go build' go build -trimpath -o "$BINARY" .
if [[ $MODE_RACE -eq 1 ]]; then run_check 'go test -race ./...' go test -race ./...; fi
if [[ -x "$BINARY" ]]; then
  run_check 'xcli --version' "$BINARY" --version
  run_check 'xcli agents --json' "$BINARY" agents --json
  for name in codex cursor antigravity; do
    # Doctor diagnoses installation/help; failure does not mean model execution is impossible.
    if "$BINARY" doctor "$name" --json >/dev/null 2>&1; then
      say "PASS doctor $name"
    else
      say "WARN doctor $name (possibly absent/configured differently; inspect locally)"
    fi
  done
  if [[ -f tests/model_gateway/lifecycle.py ]]; then
    run_check 'Gateway lifecycle (fake upstream only)' python3 tests/model_gateway/lifecycle.py --binary "$BINARY"
  fi
fi
if [[ $MODE_SMOKE -eq 1 ]]; then
  say '--- Real native CLI smoke: opted in, up to 6 CLI invocations, no retries ---'
  say 'Native agent commands may consume quota or invoke internal tools. Never paste credentials into the report.'
  if [[ -x "$BINARY" ]] && [[ -f tests/r0/smoke.py ]]; then
    if python3 tests/r0/smoke.py --binary "$BINARY" --execute --out "$REPORT_DIR/native-smoke.json"; then
      say 'PASS R0 native smoke (review individual scenario statuses)'
    else
      say 'BLOCKED_OR_FAIL R0 native smoke (review native-smoke.json)'
      FAIL=1
    fi
  else
    say 'NOT_TESTED R0 smoke: binary or script missing'; FAIL=1
  fi
else
  say 'NOT_TESTED Real native CLI execution (use --native-smoke only after approving quota).'
fi
# Static capability distinction, not a live bridge assessment.
if [[ -f internal/modelstore/config.go ]] && grep -q 'native CLI bridge is disabled' internal/modelstore/config.go; then
  say 'GATE_B_BLOCKED: Current Model Gateway config explicitly disables native CLI backend. CLI run success does NOT prove /v1 model API bridge.'
else
  say 'GATE_B_NOT_TESTED: Inspect currently implemented Gateway native adapters and test via a real authorized client.'
fi
say "END report: $REPORT_DIR"
if [[ $FAIL -ne 0 ]]; then exit 1; fi
