#!/usr/bin/env python3
"""Opt-in R3 comparison. Prepare once, then retain every success and failure.

Examples:
  python3 tests/r3/experiment.py prepare --study /tmp/r3-study --binary ./xcli
  python3 tests/r3/experiment.py run --study /tmp/r3-study --out /tmp/r3.json --execute
  python3 tests/r3/experiment.py summarize --study /tmp/r3-study --out /tmp/r3-summary.json
"""
import argparse
import json
import os
from pathlib import Path
import platform
import shutil
import sys
import time

from common import Ledger, digest, locked, process, read_json, write_json
import fixtures

HERE = Path(__file__).resolve().parent
REPOSITORY = HERE.parent.parent
ORDER = ["work-direct", "work-single", "work-cross", "project-cross", "project-single", "project-direct"]
COMPATIBILITY = ["codex-resume", "cursor-resume", "codex-cancel", "cursor-cancel", "single-recovery", "cross-recovery"]


def checked(argv, **kwargs):
    result = process(argv, **kwargs)
    if result["exit_code"]:
        raise RuntimeError(f"command failed: {argv[0]} (exit {result['exit_code']})")
    return result["stdout"].strip()


def prepare(root, binary, budget, campaign=None):
    if not 1 <= budget <= 48:
        raise ValueError("R3 budget must be 1..48")
    binary = binary.resolve(strict=True)
    root.mkdir(mode=0o700, parents=True, exist_ok=False)
    (root / "private").mkdir(mode=0o700)
    (root / "tools").mkdir(mode=0o700)
    for name in ("common.py", "native_wrapper.py", "fixtures.py", "experiment.py", "compatibility.py"):
        shutil.copyfile(HERE / name, root / "tools" / name)
    native = {}
    for adapter, command in (("codex", "codex"), ("cursor", "cursor-agent")):
        actual = shutil.which(command)
        if not actual:
            raise ValueError("missing native CLI: " + command)
        native[adapter] = {"command": actual, "version": checked([actual, "--version"]), "sha256": digest(actual)}
        wrapper = root / "tools" / adapter
        wrapper.write_text(f"#!{sys.executable}\nimport os,runpy\nos.environ['XCLI_R3_ADAPTER']={adapter!r}\nrunpy.run_path({str(root / 'tools/native_wrapper.py')!r},run_name='__main__')\n")
        wrapper.chmod(0o700)
    cfg = {"version": 1, "default_agent": "codex", "recording": {"output": True}, "agents": {
        "codex": {"adapter": "codex", "command": str(root / "tools/codex"), "args": ["-c", 'model_reasoning_effort="low"']},
        "cursor": {"adapter": "cursor", "command": str(root / "tools/cursor"), "args": ["--trust"]}}}
    # Config.Load merges built-ins. Disable the other adapters without changing
    # user's native configuration or enabling a hidden automatic fallback.
    for name in ("antigravity", "claude", "gemini", "opencode"):
        cfg["agents"][name] = {"adapter": name, "command": str(root / "not-enabled")}
    write_json(root / "config.json", cfg)
    cases = {}
    for name in ORDER:
        kind, arm = name.split("-")
        folder = root / "cases" / name
        folder.mkdir(parents=True, mode=0o700)
        source = folder / "source"
        files = fixtures.WORK_FILES if kind == "work" else fixtures.PROJECT_FILES
        fixtures.put_files(source, files)
        if kind == "project":
            commit_env = dict(os.environ, GIT_AUTHOR_DATE="2026-10-17T00:00:00Z", GIT_COMMITTER_DATE="2026-10-17T00:00:00Z")
            for argv in (["init", "-q"], ["add", "."], ["-c", "user.name=R3 Fixture", "-c", "user.email=r3@example.invalid", "commit", "-qm", "frozen fixture"]):
                checked(["git", "-C", str(source), *argv], env=commit_env)
            baseline = checked(["git", "-C", str(source), "rev-parse", "HEAD"])
        else:
            baseline = None
        cases[name] = {"kind": kind, "arm": arm, "inputs": fixtures.manifest(source), "baseline": baseline,
                       "goal": fixtures.WORK_GOAL if kind == "work" else fixtures.PROJECT_GOAL}
    if campaign:
        original = read_json(campaign / "study.json")
        if original["budget"] != budget or original.get("ledger_root", str(campaign)) != str(campaign):
            raise ValueError("campaign must name the original study with the same budget")
        Ledger(campaign).entries()
    study = {"schema": 1, "prepared_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
             "platform": platform.platform(), "binary": str(binary), "binary_sha256": digest(binary),
             "source_commit": checked(["git", "rev-parse", "HEAD"], cwd=REPOSITORY),
             "budget": budget, "ledger_root": str(campaign or root), "native": native, "order": ORDER, "cases": cases,
             "compatibility_cases": COMPATIBILITY,
             "case_limits": {**{name: 6 for name in ORDER}, **{name: 1 for name in COMPATIBILITY[:4]}, **{name: 4 for name in COMPATIBILITY[4:]}},
             "limits": {"comparison_calls": 36, "compatibility_calls": 12, "case_seconds": 600, "call_seconds": 120, "repairs": 1},
             "settings": {"codex_reasoning_effort": "low", "models": "native configured defaults", "cursor_trust": "generated temporary cwd only"},
             "benefit_rule": "Cross arm independently passes with fewer omissions, more reproducible findings, or fewer recorded human handoffs than a baseline; report extra cost. Scripted handoffs are not human observations.",
             "tools": {name: digest(HERE / name) for name in ("fixtures.py", "common.py", "native_wrapper.py", "experiment.py", "compatibility.py")}}
    write_json(root / "study.json", study)
    write_json(root / "frozen.json", {"study_sha256": digest(root / "study.json"),
                                      "config_sha256": digest(root / "config.json"),
                                      "tools": {p.name: digest(p) for p in (root / "tools").iterdir()}})
    if not campaign:
        (root / "calls.jsonl").touch(mode=0o600, exist_ok=False)
    return study


def validate(root):
    frozen = read_json(root / "frozen.json")
    if digest(root / "study.json") != frozen["study_sha256"] or digest(root / "config.json") != frozen["config_sha256"]:
        raise ValueError("prepared study/config changed")
    study = read_json(root / "study.json")
    if digest(study["binary"]) != study["binary_sha256"]:
        raise ValueError("candidate binary changed; preserve this study and prepare a new one")
    for name, expected in study["tools"].items():
        if digest(HERE / name) != expected:
            raise ValueError("experiment/scoring implementation changed; prepare a new study")
    for name, expected in frozen["tools"].items():
        if digest(root / "tools" / name) != expected:
            raise ValueError("prepared launcher changed")
    Ledger(root).entries()
    return study


def environment(root, name, phase="task"):
    return dict(os.environ, XDG_DATA_HOME=str(root / "data"), XCLI_R3_CASE=name, XCLI_R3_PHASE=phase)


def xcli(root, study, name, argv, timeout=620, label="task"):
    result = process([study["binary"], "--config", str(root / "config.json"), *argv],
                     env=environment(root, name, label), timeout=timeout,
                     private=root / "private" / f"{name}-{label}-{time.time_ns()}.json")
    try:
        value = json.loads(result["stdout"])
    except ValueError:
        raise RuntimeError(f"invalid xcli JSON (exit {result['exit_code']}); private diagnostics retained")
    return result, value


def direct_call(root, name, cwd, prompt, phase, remaining):
    result = process([str(root / "tools/codex"), "-c", 'model_reasoning_effort="low"', "exec", "--json",
                      "--skip-git-repo-check", "--sandbox", "workspace-write", "--", prompt],
                     cwd=cwd, env=environment(root, name, phase), timeout=min(120, remaining),
                     private=root / "private" / f"{name}-{phase}.json")
    events, output, usage, session = [], [], None, None
    try:
        for line in result["stdout"].splitlines():
            event = json.loads(line)
            events.append(event)
            if event.get("type") == "thread.started":
                session = event.get("thread_id")
            if event.get("type") == "item.completed" and event.get("item", {}).get("type") == "agent_message":
                output.append(event["item"].get("text", ""))
            if event.get("type") == "turn.completed":
                usage = event.get("usage")
        valid = bool(output) and any(e.get("type") == "turn.completed" for e in events)
    except (ValueError, AttributeError):
        valid = False
    return {"exit_code": result["exit_code"], "parse_complete": valid, "session_present": bool(session), "usage": usage}


def run_direct(root, name, spec, result):
    folder = root / "cases" / name
    output = folder / "output"
    if spec["kind"] == "project":
        shutil.copytree(folder / "source", output, ignore=shutil.ignore_patterns(".git"))
    else:
        output.mkdir(mode=0o700)
    goal = spec["goal"] + f"\nInput directory: {folder / 'source'}\nWrite deliverables only in: {output}\n"
    if spec["kind"] == "project":
        goal += "Run go test -count=1 ./... in the output project."
    result["native_calls"] = []
    start = time.monotonic()
    first = direct_call(root, name, output, goal, "implementation", 600)
    result["native_calls"].append(first)
    if first["exit_code"] or not first["parse_complete"]:
        result["runtime_status"] = "failed"
        return output
    # Match the Task's review/one-repair opportunity without giving the baseline
    # hidden oracle answers. This is scripted coordination, never a user study.
    feedback = "Read the current deliverables, inputs and original goal below. Review all requirements yourself. Correct any omissions or defects once, rerun the visible project tests if applicable, and leave final deliverables. Do not change protected files.\n" + goal
    review = direct_call(root, name, output, feedback, "self-review", max(1, 600 - (time.monotonic() - start)))
    result["native_calls"].append(review)
    result["scripted_external_handoffs"] = 1
    result["runtime_status"] = "completed" if not review["exit_code"] and review["parse_complete"] else "failed"
    return output


def run_task(root, study, name, spec, result):
    source = root / "cases" / name / "source"
    goal = spec["goal"]
    if spec["arm"] == "cross":
        goal += "\nRequire Codex as worker and a different CLI, Cursor, as reviewer. The coordinator must set collaboration_required=true."
    argv = ["task", goal, "--timeout", "10m", "--max-calls", "6", "--json"]
    if spec["arm"] == "single":
        argv += ["--agent", "codex"]
    if spec["kind"] == "project":
        argv += ["--project", str(source), "--verify", '["go","test","-count=1","./..."]']
    else:
        argv += ["--input", str(source)]
    invocation, task = xcli(root, study, name, argv)
    result.update(task_id=task.get("id"), runtime_status=task.get("status"), exit_code=invocation["exit_code"],
                  repairs=task.get("repairs", 0), usage=task.get("usage"), usage_incomplete=task.get("usage_incomplete", True),
                  roles=[a["role"] for a in task.get("attempts", [])],
                  adapters=[a["agent"] for a in task.get("attempts", []) if a.get("agent")],
                  review_kinds=[r.get("kind") for r in task.get("reviews", [])],
                  automatic_handoffs=len(task.get("handoffs", [])))
    runs = [read_json(root / "data/xcli/runs" / (a["run_id"] + ".json")) for a in task.get("attempts", []) if a.get("agent")]
    result["native_calls"] = [{"adapter": r.get("agent"), "status": r.get("status"), "parse_status": r.get("parse_status"),
                               "session_present": bool(r.get("session_id")), "usage": r.get("usage")} for r in runs]
    result["usage_incomplete"] = task.get("usage_incomplete", False) or any(r.get("usage") is None for r in runs)
    result["mode_preserved"] = task.get("mode") == ("single" if spec["arm"] == "single" else "auto")
    if spec["arm"] == "single":
        result["mode_preserved"] &= all(a == "codex" for a in result["adapters"])
    else:
        result["mode_preserved"] &= "cross_cli" in result["review_kinds"]
    result["current_review_bound"] = bool(task.get("reviews")) and task["reviews"][-1].get("snapshot") == task.get("snapshot")
    return Path(task["output"]) if task.get("output") else root / "missing-output"


def run_case(root, study, name):
    destination = root / "cases" / name / "result.json"
    if destination.exists():
        return read_json(destination)  # Never spend again by changing --out.
    spec = study["cases"][name]
    result = {"case": name, "kind": spec["kind"], "arm": spec["arm"], "status": "running",
              "human_interventions": 0, "human_handoffs": 0, "scripted_external_handoffs": 0,
              "automatic_handoffs": 0, "estimated_cost_usd": None}
    write_json(destination, result)  # An interrupted case cannot silently replay.
    started = time.monotonic()
    source = destination.parent / "source"
    try:
        if fixtures.manifest(source) != spec["inputs"]:
            raise ValueError("frozen inputs changed before execution")
        output = run_direct(root, name, spec, result) if spec["arm"] == "direct" else run_task(root, study, name, spec, result)
        score = fixtures.score_work(output) if spec["kind"] == "work" else fixtures.score_project(output, root / "private" / f"{name}-oracle.json")
        result["score"] = score
        result["inputs_unchanged"] = fixtures.manifest(source) == spec["inputs"]
        if spec["kind"] == "project":
            result["original_git_clean"] = not checked(["git", "-C", str(source), "status", "--porcelain"])
        result["status"] = "passed" if (score["passed"] and result["inputs_unchanged"] and result.get("original_git_clean", True)
                                             and result.get("mode_preserved", True) and result.get("current_review_bound", True)
                                             and result.get("runtime_status") == "completed") else "failed"
        result["deliverable_sha256"] = fixtures.manifest(output) if output.is_dir() else {}
    except Exception as error:
        result.update(status="failed", error=str(error))
    finally:
        result["seconds"] = round(time.monotonic() - started, 3)
        result["calls"] = sum(e["case"] == name and e.get("binary_sha256") == study["binary_sha256"] for e in Ledger(root).entries())
        write_json(destination, result)
    return result


def summarize(root):
    study = read_json(root / "study.json")
    calls = Ledger(root).entries()
    results = []
    for name in study["order"]:
        path = root / "cases" / name / "result.json"
        results.append(read_json(path) if path.exists() else {"case": name, "status": "not_run"})
    comparisons = []
    for kind in ("work", "project"):
        group = {r.get("arm"): r for r in results if r.get("kind") == kind}
        cross = group.get("cross", {})
        benefits = []
        for arm in ("direct", "single"):
            base = group.get(arm, {})
            if cross.get("status") == "passed" and base.get("status") in ("passed", "failed") and "score" in base:
                a, b = cross.get("score", {}), base.get("score", {})
                if a.get("passed") and not b.get("passed"):
                    benefits.append("independent acceptance versus " + arm)
                elif len(a.get("findings", [])) > len(b.get("findings", [])):
                    benefits.append("more supported findings versus " + arm)
        comparisons.append({"kind": kind, "observed_benefits": benefits,
                            "claim": "small predefined sample only" if benefits else "no collaboration benefit established"})
    return {"kind": "r3_comparison", "schema": 1, "prepared_at": study["prepared_at"], "platform": study["platform"],
            "source_commit": study["source_commit"], "binary_sha256": study["binary_sha256"],
            "frozen_study_sha256": read_json(root / "frozen.json")["study_sha256"],
            "native_versions": {a: n["version"] for a, n in study["native"].items()},
            "settings": study["settings"], "limits": study["limits"], "call_limit": study["budget"],
            "calls": len(calls), "calls_by_adapter": {a: sum(e["adapter"] == a for e in calls) for a in study["native"]},
            "candidate_calls": sum(e.get("binary_sha256") == study["binary_sha256"] for e in calls),
            "ledger": [{k: e[k] for k in ("sequence", "case", "adapter", "phase", "time", "binary_sha256")} for e in calls],
            "results": results, "comparisons": comparisons, "independent_user_observation": "pending",
            "status": "passed" if all(r["status"] == "passed" for r in results) else "incomplete_or_failed"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("prepare", "run", "summarize"))
    parser.add_argument("--study", type=Path, required=True)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--out", type=Path)
    parser.add_argument("--budget", type=int, default=48)
    parser.add_argument("--campaign", type=Path, help="original study whose cumulative budget this candidate shares")
    parser.add_argument("--cases", nargs="+", choices=ORDER)
    parser.add_argument("--execute", action="store_true")
    args = parser.parse_args()
    os.umask(0o077)
    root = args.study.resolve()
    if args.action == "prepare":
        if not args.binary:
            parser.error("prepare requires --binary")
        study = prepare(root, args.binary, args.budget, args.campaign.resolve() if args.campaign else None)
        print(json.dumps({"status": "prepared", "budget": study["budget"], "order": study["order"]}))
        return
    if args.out is None or args.out.exists():
        parser.error("a fresh --out report path is required")
    if args.action == "run":
        if not args.execute:
            parser.error("native execution requires --execute")
        with locked(root / "runner.lock", nonblocking=True):
            study = validate(root)
            for name in study["order"]:
                if not args.cases or name in args.cases:
                    result = run_case(root, study, name)
                    print(json.dumps({k: result.get(k) for k in ("case", "status", "calls", "seconds")}), flush=True)
    report = summarize(root)
    write_json(args.out, report)
    print(json.dumps({"status": report["status"], "calls": report["calls"]}))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
