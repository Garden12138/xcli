#!/usr/bin/env python3
"""Opt-in native smoke: at most six invocations, 120 seconds per invocation.

Only default native login environments are used. No login, credential copy,
account rotation, native configuration changes, or retries are performed.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import subprocess
import tempfile
import time


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--execute", action="store_true", help="opt in to model calls")
    args = parser.parse_args()
    if not args.execute:
        parser.error("real model calls require --execute")
    binary = str(Path(args.binary).resolve())
    output = Path(args.out).resolve()
    if output.exists():
        parser.error("use a fresh evidence path; existing evidence is never replaced")
    report = {"kind": "real", "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
              "binary_sha256": digest(binary), "max_invocations": 6,
              "timeout_seconds": 120, "invocations": 0, "tools": [], "calls": []}

    def write_report():
        output.parent.mkdir(parents=True, exist_ok=True)
        temporary = output.with_suffix(".tmp")
        with temporary.open("w") as stream:
            json.dump(report, stream, indent=2)
            stream.write("\n")
        temporary.replace(output)

    def safe_probe(command):
        try:
            return subprocess.run(command, capture_output=True, text=True, timeout=10)
        except (OSError, subprocess.TimeoutExpired):
            return None

    try:
        with tempfile.TemporaryDirectory(prefix="xcli-r0-smoke-") as scratch:
            root = Path(scratch)
            work = root / "project"
            work.mkdir()
            marker = "R0-" + secrets.token_hex(8)
            material = work / "r0-context.txt"
            material.write_text(marker + "\n")
            for argv in [["git", "init", "-q", str(work)], ["git", "-C", str(work), "add", "."],
                         ["git", "-C", str(work), "-c", "user.name=R0", "-c", "user.email=r0@example.invalid", "commit", "-qm", "fixture"]]:
                subprocess.run(argv, check=True, capture_output=True)
            environment = dict(os.environ)
            environment["XDG_DATA_HOME"] = str(root / "data")
            # Explicit xcli config; native credentials and native config roots
            # remain in the existing environment.
            config = root / "xcli.yaml"
            config.write_text("version: 1\nrecording:\n  output: false\n")
            prefix = [binary, "--config", str(config)]

            def call(tool, kind, argv, expected=None):
                if report["invocations"] >= 6:
                    return None
                report["invocations"] += 1
                entry = {"agent": tool, "kind": kind, "status": "running"}
                report["calls"].append(entry)
                write_report()
                started = time.monotonic()
                process = subprocess.Popen(prefix + argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                           text=True, env=environment, start_new_session=True)
                try:
                    if kind == "cancel":
                        time.sleep(3)
                        if process.poll() is None:
                            process.send_signal(signal.SIGTERM)
                        else:
                            entry["finished_before_cancel"] = True
                    stdout, stderr = process.communicate(timeout=120)
                    if process.returncode != 0:
                        # Categories only; never persist raw stderr, account identifiers or tool output.
                        lower = stderr.lower()
                        for category, needles in [("workspace_trust", ("not trusted", "untrusted", "workspace trust")), ("authentication", ("unauthorized", "not authenticated", "not logged in")), ("quota", ("quota", "rate limit", "usage limit")), ("permissions", ("permission denied", "not permitted")), ("network", ("connection", "network", "dns"))]:
                            if any(needle in lower for needle in needles):
                                entry["diagnostic_category"] = category
                                break
                    entry["cli_exit_code"] = process.returncode
                    try:
                        result = json.loads(stdout)
                    except json.JSONDecodeError:
                        entry["status"] = "failed_no_result"
                        return None
                    entry.update({key: result[key] for key in ["status", "exit_code", "parse_status", "error_kind", "usage"] if key in result})
                    entry["session_present"] = bool(result.get("session_id"))
                    if expected is not None:
                        entry["expected_marker_present"] = expected in result.get("output", "")
                        if not entry["expected_marker_present"] or process.returncode != 0:
                            entry["status"] = "failed"
                    if kind == "cancel" and process.returncode == 130 and result.get("status") == "canceled":
                        entry["status"] = "passed"
                    elif kind == "cancel":
                        entry["status"] = "unverified_cancel"
                    if digest(material) != hashlib.sha256((marker + "\n").encode()).hexdigest():
                        entry["status"] = "failed_fixture_modified"
                    return result if entry["status"] in ("success", "passed") else None
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGTERM)
                    try:
                        process.communicate(timeout=5)
                    except subprocess.TimeoutExpired:
                        os.killpg(process.pid, signal.SIGKILL)
                        process.communicate()
                    entry["status"] = "timeout"
                    return None
                finally:
                    entry["duration_seconds"] = round(time.monotonic() - started, 3)
                    write_report()
                    print(json.dumps({key: entry[key] for key in ["agent", "kind", "status", "duration_seconds"]}), flush=True)

            successful = []
            for tool, command in [("codex", "codex"), ("antigravity", "agy"), ("cursor", "cursor-agent")]:
                metadata = {"agent": tool, "installed": bool(shutil.which(command)), "authentication": "unknown"}
                report["tools"].append(metadata)
                if not metadata["installed"]:
                    metadata["status"] = "unverified_missing_cli"
                    continue
                version = safe_probe([command, "--version"])
                if version is not None and version.returncode == 0:
                    metadata["version"] = version.stdout.strip()[:200]
                if tool == "codex":
                    status = safe_probe([command, "login", "status"])
                    metadata["authentication"] = "logged_in" if status is not None and status.returncode == 0 else "unavailable"
                if tool == "cursor":
                    status = safe_probe([command, "status", "--format", "json"])
                    try:
                        logged_in = status is not None and json.loads(status.stdout).get("isAuthenticated") is True
                    except json.JSONDecodeError:
                        logged_in = False
                    metadata["authentication"] = "logged_in" if logged_in else "unavailable"
                if metadata["authentication"] == "unavailable":
                    metadata["status"] = "unverified_login"
                    continue
                prompt = "Read only r0-context.txt in the current directory. Remember its exact marker for the next turn and reply with that marker only. Do not modify files or call other AI CLIs."
                result = call(tool, "headless", ["run", "--agent", tool, "--cwd", str(work), "--json", prompt], marker)
                if result is None:
                    metadata["status"] = "failed_headless"
                    continue
                session = result.get("session_id")
                if not session:
                    metadata["status"] = "unverified_resume_no_session"
                    continue
                resumed = call(tool, "resume", ["resume", session, "--agent", tool, "--cwd", str(work), "--json",
                                               "Without reading any file or using tools, repeat the exact marker you remembered from the preceding turn. Reply with the marker only."], marker)
                metadata["status"] = "passed" if resumed is not None else "failed_resume"
                if resumed is not None:
                    successful.append(tool)
            for tool in successful:
                if report["invocations"] >= 6:
                    break
                call(tool, "cancel", ["run", "--agent", tool, "--cwd", str(work), "--json",
                                      "Read r0-context.txt, then explain its content in detail. Do not modify any file or call other AI CLIs."])
            report["unverified"] = ["Claude/Gemini/OpenCode native compatibility", "non-macOS native behavior"]
            report["status"] = "passed_with_unverified" if any(t.get("status") == "passed" for t in report["tools"]) and all(c["status"] in ("success", "passed") for c in report["calls"]) else "partial_or_failed"
    finally:
        report["ended_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        write_report()
    return 0 if report.get("status") == "passed_with_unverified" else 1


if __name__ == "__main__":
    raise SystemExit(main())
