#!/usr/bin/env python3
"""Transparent native-process observer: forward stdout unchanged, save metadata only."""
import collections
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time


def observed_tools(event):
    # Initialization catalogs are declarations, never evidence of tool use.
    if event.get("event", event.get("type")) in {"init", "system", "thread.started"}:
        return set()
    found = set()

    def visit(value):
        if isinstance(value, dict):
            for key, item in value.items():
                if re.search(r"image|subagent|command|toolcall", key, re.I) and isinstance(item, dict):
                    if re.fullmatch(r"[A-Za-z][A-Za-z0-9_.-]{0,79}", key):
                        found.add(key)
                if key in {"tool", "tool_name", "name", "type", "step_type"} and isinstance(item, str):
                    if re.fullmatch(r"[A-Za-z][A-Za-z0-9_.-]{0,79}", item) and re.search(r"image|subagent|command|tool.call", item, re.I):
                        found.add(item)
                if isinstance(item, (dict, list)):
                    visit(item)
        elif isinstance(value, list):
            for item in value:
                visit(item)
    visit(event)
    return found


def main():
    destination = Path(os.environ["XCLI_NATIVE_AUDIT"])
    started = time.monotonic()
    audit = {"native_started": False, "stdout_sha256": None, "event_types": {},
             "observed_tool_names": [], "image_tool_events": 0, "parse_errors": 0}
    counts = collections.Counter()
    tools = set()
    digest = hashlib.sha256()
    process = subprocess.Popen(sys.argv[1:], stdout=subprocess.PIPE)
    audit["native_started"] = True
    def stop(sig, frame):
        if process.poll() is None:
            process.send_signal(sig)
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    for line in process.stdout:
        if "first_stdout_seconds" not in audit:
            audit["first_stdout_seconds"] = round(time.monotonic() - started, 3)
        digest.update(line)
        sys.stdout.buffer.write(line)
        sys.stdout.buffer.flush()
        try:
            event = json.loads(line)
            if not isinstance(event, dict):
                raise ValueError()
            kind = event.get("event", event.get("type", "unknown"))
            if isinstance(kind, str) and re.fullmatch(r"[A-Za-z][A-Za-z0-9_.-]{0,79}", kind):
                counts[kind] += 1
            names = observed_tools(event)
            tools.update(names)
            if any(re.search("image", name, re.I) for name in names):
                audit["image_tool_events"] += 1
        except (ValueError, UnicodeError):
            audit["parse_errors"] += 1
    code = process.wait()
    audit.update({"native_exit_code": code, "duration_seconds": round(time.monotonic() - started, 3),
                  "stdout_sha256": digest.hexdigest(), "event_types": dict(counts),
                  "observed_tool_names": sorted(tools)})
    destination.write_text(json.dumps(audit, indent=2) + "\n")
    return code if code >= 0 else 128 - code


if __name__ == "__main__":
    raise SystemExit(main())
