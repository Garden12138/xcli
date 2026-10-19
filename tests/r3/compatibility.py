#!/usr/bin/env python3
"""R3 native resume/cancel and detached stop/material recovery, shared budget."""
import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

from common import Ledger, locked, read_json, write_json
from experiment import COMPATIBILITY, environment, validate, xcli


def records(root):
    folder = root / "data/xcli/runs"
    return [read_json(p) for p in sorted(folder.glob("*.json"))]


def wait_for(check, timeout=90):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = check()
        if value:
            return value
        time.sleep(.1)
    raise TimeoutError("expected native activity/state was not observed")


def resume(root, study, name):
    adapter = name.split("-")[0]
    candidates = [r for r in records(root) if r.get("agent") == adapter and r.get("status") == "success" and r.get("session_id")]
    if not candidates:
        return {"status": "not_verified", "reason": "no successful recorded native session"}
    original = candidates[0]
    argv = ["resume", original["id"], "Return exactly R3_RESUME_OK. Do not use tools or change files.", "--json"]
    if adapter == "codex":
        argv += ["--", "--skip-git-repo-check"]
    invocation, result = xcli(root, study, name, argv, timeout=140, label="native-resume")
    saved = [r for r in records(root) if r.get("resumed_from") == original["id"]]
    same = bool(saved) and saved[-1].get("session_id") == original["session_id"]
    passed = invocation["exit_code"] == 0 and same and "R3_RESUME_OK" in result.get("output", "")
    return {"status": "passed" if passed else "failed", "same_native_session": same,
            "exit_code": invocation["exit_code"], "parse_status": result.get("parse_status")}


def cancel(root, study, name):
    adapter = name.split("-")[0]
    folder = root / "compatibility" / name
    folder.mkdir(parents=True, mode=0o700)
    before = {r["id"] for r in records(root)}
    argv = [study["binary"], "--config", str(root / "config.json"), "run",
            "For cancellation testing, run a shell command sleep 30, then return R3_UNEXPECTED_COMPLETION. Do not change files or use other tools.",
            "--agent", adapter, "--cwd", str(folder), "--json", "--"]
    argv += ["--skip-git-repo-check", "--sandbox", "read-only"] if adapter == "codex" else ["--mode", "ask"]
    with subprocess.Popen(argv, env=environment(root, name, "native-cancel"), stdin=subprocess.DEVNULL,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, start_new_session=True) as child:
        try:
            def native_started():
                new = [r for r in records(root) if r["id"] not in before]
                for r in new:
                    for raw in (root / "data/xcli/runs" / r["id"]).glob("*.log"):
                        text = raw.read_text(errors="replace")
                        if "thread.started" in text or '"session_id"' in text or '"sessionId"' in text:
                            return r
                if child.poll() is not None:
                    raise RuntimeError("native command ended before cancellation")
            started = wait_for(native_started, 90)
            child.send_signal(signal.SIGINT)
            stdout, stderr = child.communicate(timeout=20)
            write_json(root / "private" / (name + ".json"), {"stdout": stdout, "stderr": stderr, "exit_code": child.returncode})
            result = json.loads(stdout)
            saved = read_json(root / "data/xcli/runs" / (started["id"] + ".json"))
            passed = child.returncode == 130 and saved["status"] == "canceled"
            return {"status": "passed" if passed else "failed", "native_activity_observed": True,
                    "exit_code": child.returncode, "run_status": saved["status"], "parse_status": result.get("parse_status")}
        finally:
            if child.poll() is None:
                child.terminate()
                try:
                    child.communicate(timeout=10)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.communicate()


def recovery(root, study, name):
    single = name == "single-recovery"
    folder = root / "compatibility" / name
    folder.mkdir(parents=True, mode=0o700)
    facts = folder / "facts.txt"
    facts.write_text("North: 12\nSouth: 9\nTotal: 21\n")
    goal = ("Write report.md containing literal Total: 21, North 12 and South 9, citing the facts.txt input. "
            "Before writing report.md, use a shell command to write started into progress.txt in the output directory, "
            "then sleep 15 seconds. If progress.txt already exists skip this pause. The marker is not an acceptance artifact. "
            "Runtime validates files; preserve all input facts. ")
    if not single:
        goal += "Require Codex worker and a different CLI, Cursor, for review."
    argv = ["task", goal, "--input", str(facts), "--timeout", "10m", "--max-calls", "4", "--detach", "--json"]
    if single:
        argv += ["--agent", "codex"]
    invocation, initial = xcli(root, study, name, argv, timeout=30, label="background-start")
    if invocation["exit_code"]:
        raise RuntimeError("detached start failed")
    task_id = initial["id"]
    task_path = root / "data/xcli/tasks" / task_id / "task.json"
    paused = None
    try:
        def marker():
            task = read_json(task_path)
            if task["status"] in ("failed", "waiting_user", "completed", "paused"):
                raise RuntimeError("Task ended before intended stop: " + task["status"])
            path = Path(task["output"]) / "progress.txt"
            return task if path.exists() else None
        live = wait_for(marker, 150)
        _, paused = xcli(root, study, name, ["task", "stop", task_id, "--json"], timeout=30, label="stop")
        if paused["status"] != "paused":
            raise RuntimeError("Task did not pause")
        # Explicitly recover from durable material, without trusting the old
        # native session. The interrupted attempt remains in Task history.
        invocation, continued = xcli(root, study, name, ["task", "continue", task_id, "--fresh-session", "--detach", "--json"], timeout=30, label="material-continue")
        if invocation["exit_code"]:
            raise RuntimeError("detached continuation failed")
        def finished():
            _, current = xcli(root, study, name, ["task", "status", task_id, "--json"], timeout=20, label="status")
            return current if not current.get("owner_active") else None
        final = wait_for(finished, 600)
        prior_calls = final["calls"]
        _, again = xcli(root, study, name, ["task", "continue", task_id, "--json"], timeout=30, label="repeat-continue")
        events = [json.loads(line) for line in (task_path.parent / "events.jsonl").read_text().splitlines()]
        mode = final["mode"] == ("single" if single else "auto") and (not single or final["agent"] == "codex")
        roles = [a["role"] for a in final.get("attempts", [])]
        reviews = final.get("reviews", [])
        review_ok = bool(reviews) and reviews[-1]["passed"] and reviews[-1]["snapshot"] == final.get("snapshot")
        artifact = Path(final["output"]) / "report.md"
        correct = artifact.exists() and "Total: 21" in artifact.read_text()
        passed = (final["status"] == "completed" and final["id"] == task_id and mode and correct and review_ok
                  and again["calls"] == prior_calls and all(e["sequence"] == i + 1 for i, e in enumerate(events)))
        return {"status": "passed" if passed else "failed", "task_id": task_id, "background": initial.get("background"),
                "stop_status": paused["status"], "final_status": final["status"], "same_task": final["id"] == task_id,
                "mode_preserved": mode, "repeat_continue_no_calls": again["calls"] == prior_calls,
                "roles": roles, "review_kinds": [r.get("kind") for r in reviews], "current_review_bound": review_ok,
                "material_recovery_requested": True, "calls_before_stop": live["calls"], "calls": final["calls"]}
    finally:
        current = read_json(task_path)
        if current.get("owner_active"):
            xcli(root, study, name, ["task", "stop", task_id, "--json"], timeout=30, label="cleanup-stop")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--study", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    parser.add_argument("--cases", nargs="+", choices=COMPATIBILITY)
    parser.add_argument("--execute", action="store_true")
    args = parser.parse_args()
    if not args.execute or args.out.exists():
        parser.error("requires --execute and a fresh --out")
    os.umask(0o077)
    root = args.study.resolve()
    with locked(root / "runner.lock", nonblocking=True):
        study = validate(root)
        for name in COMPATIBILITY:
            if args.cases and name not in args.cases:
                continue
            destination = root / "compatibility" / (name + ".json")
            if destination.exists():
                continue
            write_json(destination, {"case": name, "status": "running"})
            start = time.monotonic()
            try:
                result = resume(root, study, name) if name.endswith("resume") else cancel(root, study, name) if name.endswith("cancel") else recovery(root, study, name)
            except Exception as error:
                result = {"status": "failed", "error": str(error)}
            result.update(case=name, seconds=round(time.monotonic() - start, 3))
            result["ledger_calls"] = sum(e["case"] == name and e.get("binary_sha256") == study["binary_sha256"] for e in Ledger(root).entries())
            write_json(destination, result)
            print(json.dumps(result), flush=True)
    results = [read_json(root / "compatibility" / (name + ".json")) if (root / "compatibility" / (name + ".json")).exists() else {"case": name, "status": "not_run"} for name in COMPATIBILITY]
    write_json(args.out, {"kind": "r3_native_compatibility", "binary_sha256": study["binary_sha256"],
                          "native_versions": {a: n["version"] for a, n in study["native"].items()},
                          "platform": study["platform"], "results": results, "total_campaign_calls": len(Ledger(root).entries()),
                          "status": "passed" if all(r["status"] == "passed" for r in results) else "incomplete_or_failed"})


if __name__ == "__main__":
    main()
