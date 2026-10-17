#!/usr/bin/env python3
"""Opt-in R1 native Task acceptance: four tasks, <=6 calls/task, <=24 total.

Uses default native login environments, Codex and Cursor only. No credentials,
global configuration, accounts, native permission rules or source repositories
are changed. Cursor trust applies only to this harness's generated directories.
Raw tool output and native session IDs are never written into public evidence.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import tempfile
import time


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--cases", nargs="+", choices=["work-single", "work-auto", "project-single", "project-auto"],
                        default=["work-single", "work-auto", "project-single", "project-auto"])
    parser.add_argument("--budget", type=int, default=24, help="total invocation ceiling for this execution")
    parser.add_argument("--max-calls", type=int, default=6, help="per-task invocation ceiling, at most six")
    parser.add_argument("--codex-reasoning-effort", choices=["low", "medium", "high", "xhigh"],
                        help="per-invocation override; native persistent settings stay unchanged")
    parser.add_argument("--record-native-output", action="store_true",
                        help="retain private native streams for diagnosis; never copy them into public evidence")
    args = parser.parse_args()
    if not args.execute:
        parser.error("native model calls require --execute")
    if not 1 <= args.budget <= 24 or not 1 <= args.max_calls <= 6:
        parser.error("budget must be 1..24 and max-calls 1..6")
    binary = str(Path(args.binary).resolve())
    output = Path(args.out).resolve()
    if output.exists():
        parser.error("evidence path already exists; choose a new path")
    # Retain failed workspaces and private Task evidence for diagnosis.
    scratch = Path(tempfile.mkdtemp(prefix="xcli-r1-native-")).resolve()
    report = {"kind": "real", "date": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
              "binary_sha256": sha(binary), "platform": platform.platform(),
              "max_calls": args.budget, "calls": 0, "tasks": [], "native_versions": {}, "cases": args.cases}

    def save():
        output.parent.mkdir(parents=True, exist_ok=True)
        temporary = output.with_suffix(".tmp")
        temporary.write_text(json.dumps(report, indent=2) + "\n")
        temporary.replace(output)

    env = dict(os.environ)
    env["XDG_DATA_HOME"] = str(scratch / "data")
    # A private cache keeps project verification independent of host cache
    # write permissions. Authentication roots and credentials are unchanged.
    env["GOCACHE"] = str(scratch / "go-cache")
    config = scratch / "config.json"
    agents = {name: {"command": str(scratch / "unavailable")} for name in
              ["claude", "gemini", "opencode", "antigravity"]}
    agents.update({"codex": {"adapter": "codex", "command": "codex"},
                   "cursor": {"adapter": "cursor", "command": "cursor-agent", "args": ["--trust"]}})
    if args.codex_reasoning_effort:
        agents["codex"]["args"] = ["-c", 'model_reasoning_effort="' + args.codex_reasoning_effort + '"']
    report["invocation_overrides"] = {"codex_reasoning_effort": args.codex_reasoning_effort,
                                      "private_native_output": args.record_native_output}
    config.write_text(json.dumps({"version": 1, "default_agent": "codex", "agents": agents,
                                  "recording": {"output": args.record_native_output}}))
    for name, command in [("codex", "codex"), ("cursor", "cursor-agent")]:
        probe = subprocess.run([command, "--version"], capture_output=True, text=True, timeout=10)
        report["native_versions"][name] = probe.stdout.strip() if probe.returncode == 0 else "unavailable"
    save()
    for kind, mode in [("work", "single"), ("work", "auto"), ("project", "single"), ("project", "auto")]:
        label = kind + "-" + mode
        if label not in args.cases:
            continue
        task_limit = min(args.max_calls, args.budget - report["calls"])
        minimum_calls = 5 if kind == "project" else 3
        if task_limit < minimum_calls:
            break
        base = scratch / label
        base.mkdir(mode=0o700)
        entry = {"case": label, "status": "running"}
        report["tasks"].append(entry)
        save()
        argv = [binary, "--config", str(config), "task"]
        original = None
        if kind == "work":
            material = base / "facts.txt"
            material.write_text("Apples: 3\nPears: 2\nTotal: 5\n")
            goal = "Read the declared input facts. Write report.md with exact literals Apples: 3, Pears: 2, Total: 5 and a source reference to the input file. Check the arithmetic and requirement coverage."
            argv.extend([goal, "--input", str(material), "--output", str(base / "deliverables")])
        else:
            project = base / "source"
            project.mkdir()
            (project / "go.mod").write_text("module r1fixture\n\ngo 1.26\n")
            (project / "calc.go").write_text("package r1fixture\nfunc Value() int { return 0 }\n")
            (project / ".gitignore").write_text(".r1-first-verification\n")
            (project / "calc_test.go").write_text('''package r1fixture
import ("os"; "testing")
func TestValue(t *testing.T) {
 if _,err:=os.Stat(".r1-first-verification"); os.IsNotExist(err) {
  if err:=os.WriteFile(".r1-first-verification",[]byte("injected"),0600);err!=nil{t.Fatal(err)}
  t.Fatal("injected initial verification failure; preserve tests and use the bounded repair/reverification path")
 }
 if Value()!=1 { t.Fatal("Value must equal 1") }
}
''')
            for command in [["git", "init", "-q"], ["git", "add", "."],
                            ["git", "-c", "user.name=R1", "-c", "user.email=r1@example.invalid", "commit", "-qm", "fixture"]]:
                subprocess.run(command, cwd=project, check=True, capture_output=True)
            original = {p.name: sha(p) for p in project.iterdir() if p.is_file()}
            goal = "Fix calc.go so Value returns 1. Do not modify calc_test.go or other existing files. Use the declared go test command for verification; do not run tests in Agent roles. The fixture deliberately fails its first verification once. Preserve that failure evidence, perform the one bounded repair and verify again."
            argv.extend([goal, "--project", str(project), "--verify", '["go","test","-count=1","./..."]'])
        if mode == "single":
            argv.extend(["--agent", "codex"])
        else:
            argv[argv.index("task") + 1] += " Use another CLI for independent review: worker codex, reviewer cursor."
        argv.extend(["--json", "--max-calls", str(task_limit), "--timeout", "10m"])
        started = time.monotonic()
        try:
            completed = subprocess.run(argv, env=env, capture_output=True, text=True, timeout=620)
            result = json.loads(completed.stdout)
        except (subprocess.TimeoutExpired, json.JSONDecodeError) as exc:
            entry.update(status="failed_no_result", diagnostic=type(exc).__name__)
            report["calls"] += task_limit  # reserve the entire budget when state is unknown
            save()
            continue
        report["calls"] += result.get("calls", task_limit)
        selected = base / "task-result.json"
        descriptor = os.open(selected, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as stream:
            json.dump(result, stream)
        entry.update({key: result.get(key) for key in ["id", "status", "exit_code", "calls", "repairs", "mode", "kind"]})
        entry["elapsed_seconds"] = round(time.monotonic() - started, 3)
        entry["cli_exit_code"] = completed.returncode
        entry["reason"] = result.get("reason", "")
        entry["roles"] = [{"role": a["role"], "agent": a.get("agent"), "run_id": a["run_id"], "snapshot": a.get("snapshot"),
                           "retry_notices": a.get("retry_notices", 0)}
                          for a in result.get("attempts", [])]
        entry["checks"] = [{key: c.get(key) for key in ["kind", "argv", "status", "exit_code", "snapshot"]}
                           for c in result.get("checks", [])]
        entry["reviews"] = [{key: r.get(key) for key in ["kind", "passed", "snapshot", "findings"]}
                            for r in result.get("reviews", [])]
        entry["artifacts"] = [{"name": Path(a["path"]).name, "sha256": a["sha256"]} for a in result.get("artifacts", [])]
        valid = completed.returncode == 0 and result.get("status") == "completed"
        participants = {a.get("agent") for a in result.get("attempts", []) if a.get("agent")}
        entry["mode_constraint_passed"] = participants == {"codex"} if mode == "single" else {"codex", "cursor"}.issubset(participants)
        valid &= entry["mode_constraint_passed"]
        reviews = result.get("reviews", [])
        entry["current_review_passed"] = bool(reviews and reviews[-1].get("passed") is True and
                                              not reviews[-1].get("findings") and
                                              reviews[-1].get("snapshot") == result.get("snapshot"))
        valid &= entry["current_review_passed"]
        saved_path = scratch / "data/xcli/tasks" / result["id"] / "task.json"
        saved = json.loads(saved_path.read_text())
        entry["saved_result_consistent"] = all(saved.get(key) == result.get(key)
                                                for key in ["id", "status", "calls", "snapshot", "sequence"])
        valid &= entry["saved_result_consistent"]
        handoffs = result.get("handoffs", [])
        last_review = next((a for a in reversed(result.get("attempts", [])) if a["role"] == "reviewer"), {})
        entry["handoff_consumed"] = bool(handoffs and handoffs[-1]["run_id"] in last_review.get("inputs", []))
        valid &= entry["handoff_consumed"]
        if original is not None:
            entry["original_unchanged"] = original == {p.name: sha(p) for p in project.iterdir() if p.is_file()}
            command_checks = [c for c in result.get("checks", []) if c["kind"] == "command"]
            entry["failure_then_repair"] = result.get("repairs") == 1 and len(command_checks) == 2 and command_checks[0]["status"] == "failed" and command_checks[1]["status"] == "passed"
            workspace = Path(result["workspace"]["path"])
            entry["protected_files_unchanged"] = all(sha(workspace / name) == digest for name, digest in original.items()
                                                       if name != "calc.go")
            entry["current_verification_bound"] = bool(command_checks and
                                                         command_checks[-1]["snapshot"] == result.get("snapshot") and
                                                         command_checks[-1]["exit_code"] == 0)
            valid &= (entry["original_unchanged"] and entry["failure_then_repair"] and
                      entry["protected_files_unchanged"] and entry["current_verification_bound"])
        else:
            artifact = Path(result["output"]) / "report.md"
            body = artifact.read_text() if artifact.is_file() else ""
            entry["deliverable_content_passed"] = all(text in body for text in ["Apples: 3", "Pears: 2", "Total: 5"])
            reference = os.path.relpath(material, artifact.parent)
            entry["source_reference_present"] = str(material) in body or reference in body
            entry["input_unchanged"] = material.read_text() == "Apples: 3\nPears: 2\nTotal: 5\n"
            valid &= entry["deliverable_content_passed"] and entry["source_reference_present"] and entry["input_unchanged"]
        entry["acceptance"] = "passed" if valid else "failed"
        save()
        print(label, entry["status"], entry["acceptance"], "calls", entry["calls"], flush=True)
        if report["calls"] > args.budget:
            raise RuntimeError("native acceptance budget exceeded")
    report["acceptance"] = "passed" if len(report["tasks"]) == len(set(args.cases)) and all(e.get("acceptance") == "passed" for e in report["tasks"]) else "failed"
    save()
    print("Private evidence:", scratch, flush=True)
    return 0 if report["acceptance"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
