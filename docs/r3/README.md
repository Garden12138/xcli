# R3 experiments and candidate installation

See the [quota-restored recovery follow-up](cursor-restored-20261019.zh-CN.md) for the latest native evidence: cross-CLI worker recovery passed, completing both background recovery checks alongside the earlier single-Codex result. The supplemental budget now uses 10/12 invocations, preserving prior ledgers and failures. Independent user observation and collaboration benefit remain pending.

The [committed-candidate installation acceptance](acceptance-20261019.zh-CN.md) adds 30 configuration, error-reporting and historical-query checks without model calls. The [support matrix](compatibility.zh-CN.md) separates measured roles and platforms from quota failures and unverified combinations.

The [participant kit](field-study-20261019.zh-CN.md) is ready, with no real observations collected. The [new value comparison](value-results-20261019.zh-CN.md) used 8/16 native calls: all three methods found all six supported issues, establishing no collaboration benefit. Completing the experiment does not pass the original benefit gate.

This is an unpublished `0.5.0-r3-dev` candidate. Snapshot versions append the base commit. Published v0.4.0 archives do not contain Task. See the [measured results](results.zh-CN.md), [migration notes](release-notes.md), and [Chinese guide](README.zh-CN.md).

## Start and recover a task

Install and sign in to Codex or Cursor through its native tools. Build with Go 1.26:

```bash
go build -trimpath -o xcli .
./xcli --version
./xcli doctor codex
./xcli task "Read the inputs and write report.md with source citations" \
  --input ./materials --agent codex --detach --json
./xcli task status <task-id> --json
./xcli task stop <task-id> --json
./xcli task continue <task-id> --detach --json

# Project: clean local Git repository, isolated worktree, actual tests
./xcli task "Fix the bug and have another CLI review the result" --project . \
  --verify '["go","test","-count=1","./..."]'
```

Inspect retained materials before acknowledging `needs_reconcile` with `continue --reconcile`. Use `--fresh-session` to restore from materials. Continuing preserves the execution mode, selected CLI and cumulative budget. The [R2 guide](../r2/README.zh-CN.md) documents lifecycle and storage details.

A source list may identify a selected input by its unique filename; ambiguous names require explicit paths. Relative source paths resolve from the report's directory. Acceptance still checks the original input hash, so a same-named workspace copy cannot conceal input changes.

For a locally generated snapshot, verify the archive against `checksums.txt`, extract the archive for your OS/architecture into a temporary directory, then copy `xcli` to a directory on your PATH. This phase creates local snapshots only. A successful doctor check establishes installation/help detection; login, quota and actual execution are separate.

## Reproduce the comparison

```bash
python3 tests/r3/experiment.py prepare --study /tmp/xcli-r3-study --binary ./xcli --budget 48
python3 tests/r3/experiment.py run --study /tmp/xcli-r3-study \
  --out /tmp/r3-comparison.json --execute
python3 tests/r3/compatibility.py --study /tmp/xcli-r3-study \
  --out /tmp/r3-compatibility.json --execute
python3 tests/r3/experiment.py summarize --study /tmp/xcli-r3-study \
  --out /tmp/r3-summary.json
```

The study freezes inputs, baselines, prompts, scoring, order, settings and the candidate binary before native execution. Work checks conflicting revisions, totals and missing approval. Project checks TTL boundaries, clock rollback and int64 overflow. Independent evaluation is outside Agent workspaces; hidden tests are never repair feedback.

Each scenario compares direct Codex, a Task locked to Codex, and a Task using Codex execution with Cursor review. Each arm has a six-invocation/ten-minute cap. Direct Codex receives an implementation turn and one scripted self-review/correction; Tasks use their existing bounded orchestration and repair behavior. Actual counts and available usage are reported. Native models use configured defaults; Codex reasoning effort is fixed to low across arms.

The shared durable ledger reserves before dispatch. Comparisons use at most 36 calls, compatibility at most 12, and the campaign at most 48. Failures and cancellations count. Version/help probes do not. New output paths and restarts cannot replay recorded cases. A new candidate study must use `prepare --campaign <original-study>` to retain the original campaign budget. Never delete a ledger to rerun a study.

Raw streams and diagnostics remain in the private study directory. Public evidence contains summaries and hashes. Cursor trust applies only to generated temporary workspaces. No login settings, persistent native rules or credentials are changed.

Export only a selected summary, with paths and private payload fields removed:

```bash
python3 tests/r3/evidence.py --study /tmp/xcli-r3-study \
  --report /tmp/r3-summary.json --out /tmp/r3-public.json
```

Reported findings must be reproducible. Scripted coordination, automatic handoffs and human intervention are distinct. If collaboration shows no benefit, retain that outcome; one small run cannot establish general superiority or reduced cost. Independent user observations remain a separate gate, with a [participant script and blank record](observation.zh-CN.md).

## Checks and packaging

For a later recovery-only follow-up, use `tests/r3/revalidate.py prepare --study <new-study> --binary ./xcli --previous-campaign <old-study> --budget <new-limit>`, then `run --study <new-study> --out <fresh-report> --execute`. The new limit must be explicitly established first. The prior ledger remains unchanged and is linked by hash. A failed Cursor readiness check suppresses cross-CLI dispatch. Passing requires an actual canceled worker, a different native worker session after material recovery, preserved mode/budget/bindings, and a review bound to current output. Operator clarification is reported separately. `inspect` reads receipts without spending another native invocation.

After the user confirms quota recovery, use `prepare --study <fresh-study> --binary ./xcli --previous-campaign <recovery-study> --cross-only --carry-budget` to spend the remaining recovery allowance. This forbids a new `--budget`, preserves the original total cap, copies prior ledger bytes as an immutable prefix, and skips single-CLI recovery. Each new dispatch checks prior ledger hashes and the inherited prefix. Global accounting includes inherited calls; reports separate new calls from the carried total. Old failures and ledgers remain intact.

```bash
go test -race ./...
go vet ./...
go mod tidy -diff
XCLI_R3_BINARY=./xcli python3 -m unittest discover -s tests/r3 -v
goreleaser release --snapshot --clean
python3 tests/r3/package.py --dist dist --out /tmp/r3-installation.json
python3 tests/r3/install_acceptance.py --binary /path/to/extracted/xcli \
  --study /tmp/old-study --out /tmp/r3-installed-acceptance.json
```

CI pins GoReleaser v2.16.0 and runs deterministic protocol fixtures without model credentials. Archive checks cover Darwin/Linux × amd64/arm64. Only the host artifact receives native installation execution; cross-builds do not verify native Agent compatibility on other platforms. No release tag is created by these steps.
