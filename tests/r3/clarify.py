#!/usr/bin/env python3
"""Operator-assisted recovery of the preserved R3 single-CLI invalid-plan case.

This supplemental observation never replaces the original failed acceptance.
It acknowledges stop on an idle waiting Task, supplies a protocol clarification, and
continues from materials within the original campaign/case budget. It does not
claim an interrupted native worker or an independent user observation.
"""
import argparse
import json
import os
from pathlib import Path
import time

from common import Ledger, locked, read_json, write_json
from compatibility import wait_for
from experiment import validate, xcli


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--study", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--inspect", action="store_true", help="recheck an existing receipt without another model invocation")
    args = parser.parse_args()
    if args.execute == args.inspect or args.out.exists():
        parser.error("requires exactly one of --execute/--inspect and a fresh --out")
    os.umask(0o077)
    root = args.study.resolve()
    name = "single-recovery"
    receipt = root / "compatibility/single-clarification.json"
    with locked(root / "runner.lock", nonblocking=True):
        study = validate(root)
        if args.inspect:
            result = dict(read_json(receipt))
            result["original_harness_verdict"] = result["status"]
            _, final = xcli(root, study, name, ["task", "status", result["task_id"], "--json"], timeout=30, label="clarification-inspect")
            result["stop_was_idle_noop"] = result["stop_status"] == "waiting_user"
            result["active_stop_verified"] = False
            result["assessment"] = "operator-assisted continuation after an invalid plan; idle stop is idempotent"
            result["status"] = "passed" if (final["status"] == "completed" and final["mode"] == "single" and final["agent"] == "codex"
                                                and final["calls"] == result["calls_after"] and result["repeat_continue_no_calls"]
                                                and result["stop_status"] in ("paused", "waiting_user")) else "failed"
            result["total_campaign_calls"] = len(Ledger(root).entries())
            write_json(args.out, result)
            print(json.dumps(result))
            return
        if receipt.exists():
            raise ValueError("clarification already attempted; preserve its evidence")
        candidates = [read_json(p) for p in (root / "data/xcli/tasks").glob("*/task.json")]
        candidates = [t for t in candidates if t["mode"] == "single" and t["status"] == "waiting_user" and t.get("reason") == "complete requires work and review"]
        if len(candidates) != 1:
            raise ValueError("expected one preserved single-CLI invalid-plan Task")
        original = candidates[0]
        task_id = original["id"]
        result = {"kind": "r3_failure_recovery", "status": "running", "binary_sha256": study["binary_sha256"],
                  "task_id": task_id, "original_failure_retained": True, "operator_assisted": True,
                  "native_worker_interruption_verified": False, "calls_before": original["calls"]}
        write_json(receipt, result)
        start = time.monotonic()
        try:
            _, paused = xcli(root, study, name, ["task", "stop", task_id, "--json"], timeout=30, label="waiting-stop")
            result["stop_status"] = paused["status"]
            result["stop_was_idle_noop"] = paused["status"] == "waiting_user"
            result["active_stop_verified"] = False
            message = ("Your previous coordinator plan was rejected: complete requires work and review. Preserve the original task goal and inputs. "
                       "Return protocol schema version 1 with collaboration_required=false, acceptance for report.md containing literal Total: 21 "
                       "and citing the selected facts.txt input by its absolute path, then these actions in order: delegate worker codex "
                       "to perform the original goal; delegate reviewer codex to check current output and facts; complete. "
                       "Use the original timing marker instructions. Do not request another CLI. Do not omit the worker or reviewer. "
                       "The operator stopped a waiting Task, not an active worker; this continuation checks material-based recovery after clarification.")
            invocation, continued = xcli(root, study, name, ["task", "continue", task_id, message, "--fresh-session", "--detach", "--json"], timeout=30, label="clarified-continue")
            if invocation["exit_code"]:
                raise RuntimeError("clarified continuation rejected")
            def finished():
                _, current = xcli(root, study, name, ["task", "status", task_id, "--json"], timeout=20, label="clarified-status")
                return current if not current.get("owner_active") else None
            final = wait_for(finished, 600)
            _, repeated = xcli(root, study, name, ["task", "continue", task_id, "--json"], timeout=30, label="clarified-repeat")
            mode_ok = final["mode"] == "single" and final["agent"] == "codex" and all(a.get("agent") in (None, "codex") for a in final.get("attempts", []))
            result.update(final_status=final["status"], calls_after=final["calls"], mode_preserved=mode_ok,
                          same_task=final["id"] == task_id, repeat_continue_no_calls=repeated["calls"] == final["calls"],
                          review_kinds=[r.get("kind") for r in final.get("reviews", [])])
            result["status"] = "passed" if final["status"] == "completed" and mode_ok and result["repeat_continue_no_calls"] and paused["status"] in ("paused", "waiting_user") else "failed"
        except Exception as error:
            result.update(status="failed", error=str(error))
        finally:
            result["seconds"] = round(time.monotonic() - start, 3)
            result["total_campaign_calls"] = len(Ledger(root).entries())
            write_json(receipt, result)
            write_json(args.out, result)
    print(json.dumps(result))


if __name__ == "__main__":
    main()
