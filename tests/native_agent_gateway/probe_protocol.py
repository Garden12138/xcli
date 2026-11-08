#!/usr/bin/env python3
"""Explicit non-inference stdio probe. Never authenticates or creates a session.

Requires the owner's prior review of the exact official executable's --help.
Uses a disposable HOME, no parent environment and no network listener. This is
readiness evidence, not an adapter or an account/permission/entitlement test.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import signal
import subprocess
import tempfile
import time


def plan(agent):
    if agent == "codex":
        return ["app-server", "--listen", "stdio://"], {
            "id": 1, "method": "initialize", "params": {
                "clientInfo": {"name": "xcli_g5a_probe", "title": "xcli G5A probe", "version": "1"},
                "capabilities": {"experimentalApi": False}}}
    if agent in ("cursor", "antigravity"):
        return ["acp"], {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
            "protocolVersion": 1,
            "clientCapabilities": {"fs": {"readTextFile": False, "writeTextFile": False}, "terminal": False},
            "clientInfo": {"name": "xcli_g5a_probe", "version": "1"}}}
    raise ValueError("registered agent required")


def shape(value, depth=0):
    """Keep only structure and booleans, never arbitrary native string values."""
    if depth > 5:
        return "depth_limit"
    if isinstance(value, dict):
        return {k: shape(v, depth + 1) for k, v in value.items() if len(k) < 80}
    if isinstance(value, list):
        return [shape(v, depth + 1) for v in value[:16]]
    if isinstance(value, bool) or value is None:
        return value
    return type(value).__name__


def probe(agent, binary):
    args, request = plan(agent)
    binary = Path(binary)
    if not binary.is_absolute():
        raise ValueError("absolute owner-selected executable required")
    binary = binary.resolve(strict=True)
    before = hashlib.sha256(binary.read_bytes()).hexdigest()
    report = {"agent": agent, "binary": str(binary), "binary_sha256": before,
              "argv": args, "sent_methods": ["initialize"], "native_process_starts": 1,
              "model_requests_sent": 0, "environment": "disposable_home_no_parent_env",
              "account_entitlement": "NOT_TESTED", "turn_semantics": "NOT_TESTED"}
    with tempfile.TemporaryDirectory(prefix="xcli-g5a-rpc-") as tmp:
        proc = subprocess.Popen([str(binary), *args], cwd=tmp, start_new_session=True,
                                env={"HOME": tmp, "PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "LANG": "C.UTF-8"},
                                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        sel = selectors.DefaultSelector()
        for stream in (proc.stdout, proc.stderr):
            os.set_blocking(stream.fileno(), False)
            sel.register(stream, selectors.EVENT_READ)
        proc.stdin.write(json.dumps(request).encode() + b"\n")
        proc.stdin.flush()
        pending = b""
        total = 0
        initialized = False
        unexpected = 0
        deadline = time.monotonic() + 5
        report["status"] = "NOT_TESTED"
        try:
            while sel.get_map() and time.monotonic() < deadline:
                for key, _ in sel.select(timeout=0.1):
                    data = os.read(key.fileobj.fileno(), 65536)
                    if not data:
                        sel.unregister(key.fileobj)
                        continue
                    total += len(data)
                    if total > 1 << 20:
                        raise ValueError("bounded output exceeded")
                    if key.fileobj is proc.stderr:
                        continue  # Never persist startup diagnostics or credentials.
                    pending += data
                    if len(pending) > 256 << 10:
                        raise ValueError("bounded frame exceeded")
                    while b"\n" in pending:
                        line, pending = pending.split(b"\n", 1)
                        msg = json.loads(line)
                        if "method" in msg:
                            if "id" in msg:
                                unexpected += 1
                                reply = {"jsonrpc": "2.0", "id": msg["id"], "error": {
                                    "code": -32601, "message": "probe method not permitted"}}
                                proc.stdin.write(json.dumps(reply).encode() + b"\n")
                                proc.stdin.flush()
                            continue
                        if msg.get("id") != 1 or initialized:
                            raise ValueError("unmatched response")
                        report["reply_shape"] = shape(msg.get("result", msg.get("error")))
                        report["status"] = "PASS" if isinstance(msg.get("result"), dict) else "FAIL"
                        initialized = True
                        if agent == "codex" and report["status"] == "PASS":
                            proc.stdin.write(b'{"method":"initialized","params":{}}\n')
                            proc.stdin.flush()
                            report["sent_methods"].append("initialized")
                        proc.stdin.close()
            report["timed_out"] = proc.poll() is None and time.monotonic() >= deadline
            report["unexpected_server_requests"] = unexpected
        except (ValueError, OSError, BrokenPipeError):
            report["status"] = "FAIL"
            report["reason"] = "protocol_or_io_error_no_raw_payload_retained"
        finally:
            sel.close()
            if not proc.stdin.closed:
                proc.stdin.close()
            if proc.poll() is None:
                os.killpg(proc.pid, signal.SIGTERM)
                try:
                    proc.wait(timeout=1)
                except subprocess.TimeoutExpired:
                    os.killpg(proc.pid, signal.SIGKILL)
            proc.wait(timeout=2)
            proc.stdout.close()
            proc.stderr.close()
        report["exit_code"] = proc.returncode
        report["initialized"] = initialized
        report["captured_bytes"] = total
        report["binary_unchanged"] = before == hashlib.sha256(binary.read_bytes()).hexdigest()
        if not initialized or unexpected or not report["binary_unchanged"]:
            report["status"] = "FAIL"
        # PASS is only an initialize reply. EOF/cleanup is recorded independently.
        return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--agent", choices=["codex", "cursor", "antigravity"], required=True)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--execute-non-model", action="store_true")
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    if not args.execute_non_model:
        parser.error("explicit --execute-non-model required; no process started")
    report = probe(args.agent, args.binary)
    with open(args.out, "x") as f:
        json.dump(report, f, indent=2)
        f.write("\n")
    print(json.dumps({"agent": args.agent, "status": report["status"], "model_requests_sent": 0}))


if __name__ == "__main__":
    main()
