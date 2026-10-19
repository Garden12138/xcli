#!/usr/bin/env python3
"""Explicitly budgeted follow-up for R3's two unfinished worker recovery cases.

The prior ledger stays immutable. Native readiness is not inferred from login.
Only a running worker can satisfy stop/recovery; an idle clarification cannot.
"""
import argparse
import json
import os
from pathlib import Path
import sys
import time

from common import Ledger, digest, locked, read_json, write_json
from compatibility import wait_for
from experiment import prepare, validate, xcli
from fixtures import manifest


def check_study(root):
    study = validate(root)
    if study.get("profile") != "recovery-followup":
        raise ValueError("use a separately prepared recovery follow-up")
    prior = study["previous_campaign"]
    if digest(Path(prior["path"]) / "calls.jsonl") != prior["ledger_sha256"]:
        raise ValueError("previous campaign ledger changed")
    for name, expected in study["followup_inputs"].items():
        if manifest(root / "followup" / name / "inputs") != expected:
            raise ValueError("frozen recovery input changed")
    return study


def run_record(root, attempt):
    return read_json(root / "data/xcli/runs" / (attempt["run_id"] + ".json"))


def native_session(root, record):
    if record.get("session_id"):
        return record["session_id"]
    # A canceled stream can retain thread.started without a complete result.
    for path in (root / "data/xcli/runs" / record["id"]).glob("*.log"):
        with path.open() as stream:
            for line in stream:
                try:
                    event = json.loads(line)
                    if event.get("type") == "thread.started":
                        return event.get("thread_id")
                except ValueError:
                    continue
    return None


def readiness(root, study):
    folder = root / "followup/cursor-readiness"
    folder.mkdir(mode=0o700, exist_ok=True)
    invocation, result = xcli(root, study, "cursor-readiness", ["run", "Return exactly R3_READY. Do not use tools or modify files.",
                        "--agent", "cursor", "--cwd", str(folder), "--json", "--", "--mode", "ask"], timeout=140, label="readiness")
    return {"status": "passed" if invocation["exit_code"] == 0 and "R3_READY" in result.get("output", "") else "failed",
            "exit_code": invocation["exit_code"], "parse_status": result.get("parse_status"),
            "meaning": "one real read-only invocation; not a guarantee of future quota"}


def recovery(root, study, name):
    single = name == "single-recovery"
    facts = root / "followup" / name / "inputs/facts.txt"
    reviewer = "codex" if single else "cursor"
    goal = ("Create report.md from the selected facts.txt. Include exact literals North: 12, South: 9 and Total: 21, "
            "and cite facts.txt using its absolute path. Preserve the input. "
            "Before writing report.md, the WORKER must use a shell command to write started into progress.txt in the output directory "
            "and then sleep 20 seconds. If progress.txt exists, skip the pause and complete the report. "
            "The coordinator and reviewer must not write this marker; it is not an acceptance artifact. "
            f"The coordinator must return schema version 1 with acceptance for report.md, a delegate worker codex action, "
            f"then a delegate reviewer {reviewer} action and complete. Even in single mode both roles are required. "
            "Runtime checks the artifact; no command verification is necessary. ")
    if not single:
        goal += "Require Cursor as a different CLI reviewer; set collaboration_required=true."
    argv = ["task", goal, "--input", str(facts), "--max-calls", "5", "--timeout", "10m", "--detach", "--json"]
    if single:
        argv += ["--agent", "codex"]
    invocation, initial = xcli(root, study, name, argv, timeout=30, label="start")
    if invocation["exit_code"]:
        raise RuntimeError("background submission rejected")
    task_id = initial["id"]
    task_path = root / "data/xcli/tasks" / task_id / "task.json"
    result = {"task_id": task_id, "mode": initial["mode"], "operator_clarifications": 0,
              "worker_interruption_observed": False, "background": initial.get("background", False)}
    try:
        def active_worker():
            task = read_json(task_path)
            if task["status"] == "waiting_user" and task.get("reason") == "complete requires work and review" and not result["operator_clarifications"]:
                result["initial_plan_rejection"] = task["reason"]
                result["operator_clarifications"] = 1
                message = (f"Preserve the original goal. The previous plan was rejected. Return protocol version 1 with report.md acceptance, "
                           f"delegate worker codex, delegate reviewer {reviewer}, then complete. The worker alone creates progress.txt and pauses. "
                           "Do not omit either role. This is an explicit test-operator clarification, not an independent user observation.")
                call, _ = xcli(root, study, name, ["task", "continue", task_id, message, "--fresh-session", "--detach", "--json"], timeout=30, label="plan-clarification")
                if call["exit_code"]:
                    raise RuntimeError("protocol clarification rejected")
                return None
            if task["status"] in ("failed", "waiting_user", "completed", "paused", "needs_reconcile"):
                raise RuntimeError("Task ended before worker stop: " + task["status"])
            workers = [a for a in task.get("attempts", []) if a["role"] == "worker"]
            if workers and (Path(task["output"]) / "progress.txt").exists():
                record = run_record(root, workers[-1])
                if record["status"] == "running":
                    return task, workers[-1]
        live, interrupted = wait_for(active_worker, 240)
        _, paused = xcli(root, study, name, ["task", "stop", task_id, "--json"], timeout=30, label="active-worker-stop")
        stopped = run_record(root, interrupted)
        result.update(stop_status=paused["status"], stopped_run_status=stopped["status"], calls_before_stop=live["calls"])
        result["worker_interruption_observed"] = paused["status"] == "paused" and not paused.get("owner_active") and stopped["status"] == "canceled"
        if not result["worker_interruption_observed"]:
            raise RuntimeError("active worker cancellation/paused state was not established")
        invocation, _ = xcli(root, study, name, ["task", "continue", task_id, "--fresh-session", "--detach", "--json"], timeout=30, label="fresh-material-continue")
        if invocation["exit_code"]:
            raise RuntimeError("material continuation rejected")
        def finished():
            _, task = xcli(root, study, name, ["task", "status", task_id, "--json"], timeout=20, label="status")
            return task if not task.get("owner_active") else None
        final = wait_for(finished, 600)
        workers = [a for a in final.get("attempts", []) if a["role"] == "worker"]
        restored = run_record(root, workers[-1])
        old_session, new_session = native_session(root, stopped), native_session(root, restored)
        result.update(final_status=final["status"], calls=final["calls"], same_task=final["id"] == task_id,
                      mode_preserved=final["mode"] == initial["mode"] and final.get("agent") == initial.get("agent"),
                      candidates_preserved=final["candidates"] == initial["candidates"],
                      bindings_preserved=final.get("bindings") == initial.get("bindings"),
                      budget_preserved=final["max_calls"] == 5 and final["elapsed_ns"] >= paused["elapsed_ns"],
                      interrupted_attempt_retained=any(a["run_id"] == interrupted["run_id"] for a in workers),
                      fresh_worker_attempt=workers[-1]["run_id"] != interrupted["run_id"] and restored["status"] == "success",
                      fresh_native_session=bool(old_session and new_session and old_session != new_session))
        reviews = final.get("reviews", [])
        expected_review = "self_check" if single else "cross_cli"
        result["current_review_bound"] = bool(reviews) and reviews[-1]["passed"] and reviews[-1]["snapshot"] == final.get("snapshot") and reviews[-1]["kind"] == expected_review
        result["review_kinds"] = [r["kind"] for r in reviews]
        events = [json.loads(line) for line in (task_path.parent / "events.jsonl").read_text().splitlines()]
        result["events_monotonic"] = all(e["sequence"] == i + 1 for i, e in enumerate(events))
        report = Path(final["output"]) / "report.md"
        result["artifact_correct"] = report.is_file() and all(text in report.read_text() for text in ("North: 12", "South: 9", "Total: 21"))
        result["input_unchanged"] = manifest(facts.parent) == study["followup_inputs"][name]
        result["single_cli_locked"] = not single or all(a.get("agent") in (None, "codex") for a in final.get("attempts", []))
        required = ("worker_interruption_observed", "same_task", "mode_preserved", "candidates_preserved", "bindings_preserved", "budget_preserved",
                    "interrupted_attempt_retained", "fresh_worker_attempt", "fresh_native_session", "current_review_bound", "events_monotonic", "artifact_correct", "input_unchanged", "single_cli_locked")
        if final["status"] == "completed":
            calls_before = len(Ledger(root).entries())
            _, again = xcli(root, study, name, ["task", "continue", task_id, "--json"], timeout=30, label="completed-continue")
            result["repeat_continue_no_calls"] = again["status"] == "completed" and again["calls"] == final["calls"] and len(Ledger(root).entries()) == calls_before
        else:
            result["repeat_continue_no_calls"] = False
        result["status"] = "passed" if final["status"] == "completed" and result["repeat_continue_no_calls"] and all(result[key] for key in required) else "failed"
    except Exception as error:
        result.update(status="failed", error=str(error))
    finally:
        _, current = xcli(root, study, name, ["task", "status", task_id, "--json"], timeout=20, label="cleanup-status")
        if current.get("owner_active"):
            xcli(root, study, name, ["task", "stop", task_id, "--json"], timeout=30, label="cleanup-stop")
    return result


def summarize(root, study):
    entries = Ledger(root).entries()
    results = [read_json(root / "followup" / (name + ".json")) if (root / "followup" / (name + ".json")).exists() else {"case": name, "status": "not_run"} for name in study["compatibility_cases"]]
    prior = study["previous_campaign"]
    return {"kind": "r3_recovery_followup", "source_commit": study["source_commit"], "binary_sha256": study["binary_sha256"],
            "platform": study["platform"], "native_versions": {a: n["version"] for a, n in study["native"].items()},
            "prior_campaign": {k: prior[k] for k in ("calls", "budget", "ledger_sha256")},
            "prior_ledger_unchanged": digest(Path(prior["path"]) / "calls.jsonl") == prior["ledger_sha256"],
            "new_budget": study["budget"], "new_calls": len(entries), "historical_plus_new_calls": prior["calls"] + len(entries),
            "ledger": [{k: e[k] for k in ("sequence", "case", "adapter", "phase", "binary_sha256")} for e in entries],
            "results": results, "status": "passed" if all(r["status"] == "passed" for r in results) else "incomplete_or_failed"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("prepare", "run", "inspect"))
    parser.add_argument("--study", type=Path, required=True)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--previous-campaign", type=Path)
    parser.add_argument("--budget", type=int, help="explicit separately authorized supplemental limit; prepare only")
    parser.add_argument("--out", type=Path)
    parser.add_argument("--execute", action="store_true")
    args = parser.parse_args()
    os.umask(0o077)
    root = args.study.resolve()
    if args.action == "prepare":
        if not args.binary or not args.previous_campaign or args.budget is None:
            parser.error("prepare requires --binary, --previous-campaign and explicit --budget")
        prepare(root, args.binary, args.budget, previous_campaign=args.previous_campaign.resolve())
        print(json.dumps({"status": "prepared", "supplemental_budget": args.budget, "native_calls": 0}))
        return
    if not args.out or args.out.exists() or args.budget is not None:
        parser.error("a fresh --out is required; --budget is prepare-only")
    study = check_study(root)
    if args.action == "run":
        if not args.execute:
            parser.error("real native execution requires --execute")
        with locked(root / "runner.lock", nonblocking=True):
            for name in study["compatibility_cases"]:
                receipt = root / "followup" / (name + ".json")
                if receipt.exists():
                    continue
                if name == "cross-recovery" and read_json(root / "followup/cursor-readiness.json")["status"] != "passed":
                    write_json(receipt, {"case": name, "status": "not_verified", "reason": "Cursor readiness failed; no retry or CLI substitution"})
                    continue
                write_json(receipt, {"case": name, "status": "running"})
                start = time.monotonic()
                try:
                    result = readiness(root, study) if name == "cursor-readiness" else recovery(root, study, name)
                except Exception as error:
                    result = {"status": "failed", "error": str(error)}
                result.update(case=name, seconds=round(time.monotonic() - start, 3), calls=sum(e["case"] == name for e in Ledger(root).entries()))
                write_json(receipt, result)
                print(json.dumps(result), flush=True)
    write_json(args.out, summarize(root, study))


if __name__ == "__main__":
    main()
