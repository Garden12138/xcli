#!/usr/bin/env python3
"""Deterministic protocol fixture, explicitly gated; never a native result."""
import json
import os
from pathlib import Path
import sys
import time

if os.environ.get("XCLI_R3_SIMULATION") != "1":
    raise SystemExit("simulation fixture requires XCLI_R3_SIMULATION=1")
adapter = os.environ.get("XCLI_R3_FAKE_ADAPTER", "codex")
if sys.argv[-1] in ("--version", "-v"):
    print("r3-simulated-" + adapter)
    raise SystemExit(0)
if sys.argv[-1] in ("--help", "-h"):
    print("exec resume --print --output-format stream-json --resume --mode --sandbox")
    raise SystemExit(0)
if adapter == "cursor" and os.environ.get("XCLI_R3_FAKE_CURSOR_QUOTA") == "1":
    print("ActionRequiredError: usage limit reached (simulation)", file=sys.stderr)
    raise SystemExit(1)
prompt = sys.argv[-1]
session = next((arg for arg in sys.argv[1:-1] if arg.startswith("r3-simulation-")), "r3-simulation-" + str(os.getpid()))
if "For cancellation testing" in prompt:
    if adapter == "codex":
        print(json.dumps({"type": "thread.started", "thread_id": "r3-simulation-session"}), flush=True)
    else:
        print(json.dumps({"type": "system", "subtype": "init", "session_id": "r3-simulation-session"}), flush=True)
    time.sleep(30)
    raise SystemExit(0)
if adapter == "codex":
    print(json.dumps({"type": "thread.started", "thread_id": session}), flush=True)
brief = None
if "Task brief:\n" in prompt:
    brief = json.JSONDecoder().raw_decode(prompt.split("Task brief:\n", 1)[1])[0]
if "Return exactly R3_READY" in prompt:
    output = "R3_READY"
elif "Return exactly R3_RESUME_OK" in prompt:
    output = "R3_RESUME_OK"
elif prompt.startswith("You are the xcli Task coordinator"):
    project = brief["kind"] == "project"
    cross = brief["mode"] != "single"
    output = {"version": 1, "collaboration_required": cross, "reason": "simulated protocol fixture",
              "acceptance": [{"path": "cache.go" if project else "report.md", "contains": [], "sources": []}],
              "actions": [{"type": "delegate", "role": "worker", "agent": "codex", "objective": brief["goal"]},
                          *([{"type": "verify", "argv": ["go", "test", "-count=1", "./..."], "cwd": "."}] if project else []),
                          {"type": "delegate", "role": "reviewer", "agent": "cursor" if cross else "codex", "objective": "check current materials"},
                          {"type": "complete"}]}
    if os.environ.get("XCLI_R3_FAKE_BAD_PLAN") == "1":
        output["actions"] = [{"type": "complete"}]
elif prompt.startswith("You are the xcli Task reviewer"):
    output = {"passed": True, "findings": [], "coverage": ["simulated fixture requirements"], "sources": ["cache.go" if Path("cache.go").exists() else "report.md"]}
else:
    folder = Path(brief["output"]) if brief else Path.cwd()
    if brief and "Before writing report.md" in brief["goal"] and not (folder / "progress.txt").exists():
        (folder / "progress.txt").write_text("started")
        time.sleep(5)
    if "Release evidence audit v1" in (brief["goal"] if brief else prompt):
        import runpy
        data = runpy.run_path(str(Path(__file__).with_name("value_fixtures.py")))["reference_report"]()
        (folder / "report.json").write_text(json.dumps(data))
        (folder / "report.md").write_text("\n".join(row["component"] + ": " + row["decision"] for row in data["targets"])
                                         + "\nSources: " + " ".join(data["sources"]) + "\n")
    elif (folder / "cache.go").exists():
        (folder / "cache.go").write_text("package r3fixture\nfunc Alive(inserted, now, ttl int64) bool { return ttl>0 && now>=inserted && uint64(now)-uint64(inserted)<uint64(ttl) }\n")
    else:
        data = {"north": 12, "south": 9, "total": 21, "release_date": "2026-10-25", "pilot_approved": False,
                "sources": ["draft.txt", "revision.txt", "approval.txt"], "findings": [
                    {"kind": "total_conflict", "evidence": "20 versus 21", "sources": ["draft.txt", "revision.txt"]},
                    {"kind": "date_conflict", "evidence": "2026-10-20 versus 2026-10-25", "sources": ["draft.txt", "revision.txt"]},
                    {"kind": "approval_missing", "evidence": "pending unsigned", "sources": ["approval.txt"]}]}
        (folder / "report.json").write_text(json.dumps(data))
        (folder / "report.md").write_text("North 12, South 9, total 21; release 2026-10-25; pilot approval pending. Sources: draft.txt revision.txt approval.txt\n")
        if brief and "Before writing report.md" in brief["goal"]:
            (folder / "report.md").write_text("North: 12 South: 9 Total: 21. Source: facts.txt\n")
    output = {"summary": "simulated worker handoff", "facts": ["fixture written"], "decisions": [], "sources": [], "unresolved": []}
if os.environ.get("XCLI_R3_FAKE_BROKEN") == "1":
    print("broken stream")
elif adapter == "codex":
    print(json.dumps({"type": "item.completed", "item": {"type": "agent_message", "text": output if isinstance(output, str) else json.dumps(output)}}))
    print(json.dumps({"type": "turn.completed", "usage": {"input_tokens": 10, "output_tokens": 5}}))
else:
    print(json.dumps({"type": "result", "subtype": "success", "result": output if isinstance(output, str) else json.dumps(output), "is_error": False, "session_id": session}))
