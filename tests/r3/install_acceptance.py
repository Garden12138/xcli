#!/usr/bin/env python3
"""Exercise an installed candidate without invoking an Agent or editing old studies.

Checks paths containing spaces/Unicode, configuration preservation, truthful
missing-prerequisite errors, and read-only queries against copies of old Tasks.
This is operator acceptance, not an independent participant observation.
"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from common import digest, process, read_json, write_json


def manifest(root):
    return {str(p.relative_to(root)): digest(p) for p in root.rglob("*") if p.is_file()}


def inspect(binary, studies):
    report = {"kind": "r3_installed_candidate_acceptance", "binary_sha256": digest(binary),
              "source_commit": subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip(),
              "native_model_invocations": 0, "independent_participant": False,
              "checks": [], "historical_tasks": [], "status": "running"}
    with tempfile.TemporaryDirectory(prefix="xcli-r3-安装 acceptance-") as temporary:
        root = Path(temporary)
        bindir = root / "bin space"
        bindir.mkdir()
        installed = bindir / "xcli"
        shutil.copyfile(binary, installed)
        installed.chmod(0o700)
        env = dict(os.environ, PATH=str(bindir), XDG_CONFIG_HOME=str(root / "配置 config"),
                   XDG_DATA_HOME=str(root / "数据 data"))
        cwd = root / "工作 input"
        cwd.mkdir()
        facts = cwd / "facts.txt"
        facts.write_text("North: 12\nSouth: 9\nTotal: 21\n")
        inputs = manifest(cwd)

        def call(*argv, environment=None):
            return process([str(installed), *argv], cwd=cwd, env=environment or env, timeout=20)

        def require(condition, label):
            if not condition:
                raise ValueError("installed candidate acceptance failed: " + label)
            report["checks"].append({"check": label, "status": "passed"})

        version = call("--version")
        require(version["exit_code"] == 0, "version_from_installed_binary")
        report["version"] = version["stdout"].strip()
        config = Path(env["XDG_CONFIG_HOME"]) / "xcli/config.yaml"
        located = call("config", "path")
        require(located["exit_code"] == 0 and Path(located["stdout"].strip()) == config,
                "unicode_and_space_xdg_path")
        defaults = call("config", "validate")
        require(defaults["exit_code"] == 0 and not config.exists(), "defaults_do_not_create_config")
        require(call("config", "init")["exit_code"] == 0 and config.is_file(), "isolated_config_creation")
        config.write_text(config.read_text() + "\n# acceptance: preserve user customizations\n")
        original = digest(config)
        rejected = call("config", "init")
        require(rejected["exit_code"] != 0 and digest(config) == original, "existing_config_not_overwritten")
        require(call("config", "validate")["exit_code"] == 0 and digest(config) == original,
                "config_validation_preserves_bytes")

        explicit = root / "显式 explicit.json"
        explicit.write_text(json.dumps({"version": 1, "default_agent": "codex", "agents": {
            "codex": {"command": str(root / "absent-native-codex")}}}))
        explicit_hash = digest(explicit)
        selected = call("--config", str(explicit), "config", "path")
        require(selected["exit_code"] == 0 and Path(selected["stdout"].strip()) == explicit,
                "explicit_config_overrides_xdg")
        doctor = call("--config", str(explicit), "doctor", "codex", "--json")
        diagnosis = json.loads(doctor["stdout"])
        require(doctor["exit_code"] != 0 and not diagnosis["agents"]["codex"]["installed"],
                "missing_cli_doctor_fails_truthfully")
        submitted = call("--config", str(explicit), "task", "Summarize facts.txt", "--agent", "codex",
                         "--input", str(facts), "--max-calls", "1", "--json")
        failed = json.loads(submitted["stdout"])
        require(submitted["exit_code"] != 0 and failed["status"] == "failed" and failed["exit_code"] != 0
                and failed["calls"] == 0 and not failed.get("attempts") and bool(failed.get("reason")),
                "missing_cli_task_failed_without_dispatch")
        persisted = call("task", "status", failed["id"], "--json")
        stored = json.loads(persisted["stdout"])
        require(persisted["exit_code"] == 0 and stored["status"] == "failed" and stored["calls"] == 0
                and stored.get("reason") == failed["reason"], "missing_cli_failure_remains_inspectable")
        invalid = root / "invalid.json"
        invalid.write_text('{"version":')
        invalid_hash = digest(invalid)
        require(call("--config", str(invalid), "config", "validate")["exit_code"] != 0
                and digest(invalid) == invalid_hash, "malformed_config_rejected_and_preserved")
        require(digest(config) == original and digest(explicit) == explicit_hash and manifest(cwd) == inputs,
                "config_and_inputs_unchanged_after_errors")

        for index, study in enumerate(studies):
            source = study / "data/xcli/tasks"
            before = manifest(source)
            require(bool(before), "historical_task_data_present_" + str(index))
            copied = root / ("historical-" + str(index))
            shutil.copytree(source, copied / "xcli/tasks")
            historical_env = dict(env, XDG_DATA_HOME=str(copied))
            for task_path in sorted((copied / "xcli/tasks").glob("*/task.json")):
                task = read_json(task_path)
                journal = task_path.parent / "checkpoints.jsonl"
                if journal.exists():
                    lines = journal.read_text().splitlines()
                    if lines:
                        task = json.loads(lines[-1])
                if task["status"] not in ("completed", "failed", "paused", "needs_reconcile", "waiting_user", "canceled"):
                    raise ValueError("only inactive historical Tasks may be inspected")
                queried = call("task", "status", task["id"], "--json", environment=historical_env)
                restored = json.loads(queried["stdout"])
                fields = ("id", "version", "status", "mode", "agent", "calls", "max_calls", "snapshot", "attempts", "reviews")
                require(queried["exit_code"] == 0 and not restored.get("owner_active")
                        and all(restored.get(k) == task.get(k) for k in fields), "historical_task_" + task["id"])
                queried_events = call("task", "status", task["id"], "--events", environment=historical_env)
                events = [json.loads(line) for line in queried_events["stdout"].splitlines()]
                require(queried_events["exit_code"] == 0 and bool(events)
                        and all(e["sequence"] == i + 1 for i, e in enumerate(events)), "historical_events_" + task["id"])
                report["historical_tasks"].append({"id": task["id"], "status": restored["status"],
                                                   "calls": restored["calls"], "events": len(events)})
            require(manifest(source) == before, "historical_source_unchanged_" + str(index))
        report["status"] = "passed"
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--study", action="append", type=Path, default=[])
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    if args.out.exists():
        parser.error("fresh --out required")
    report = inspect(args.binary.resolve(strict=True), [s.resolve(strict=True) for s in args.study])
    write_json(args.out, report)
    print(json.dumps({"status": report["status"], "checks": len(report["checks"]),
                      "historical_tasks": len(report["historical_tasks"]), "native_model_invocations": 0}))


if __name__ == "__main__":
    main()
