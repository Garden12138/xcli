#!/usr/bin/env python3
"""One Codex HTTP text acceptance. Default dry-run. Never probes native login.

The owner prepares/probes/enables and separately authorizes the profile first.
Current unverified native tool boundary intentionally prevents execution.
"""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import stat
import subprocess
import time
import uuid

PROMPT = "Return exactly XCLI_G5B_CODEX_SYNTHETIC_20261108 and nothing else. Do not use tools."
EXPECTED = "XCLI_G5B_CODEX_SYNTHETIC_20261108"


def execute_gate(readiness, authorization_ref):
    return (readiness.get("state") == "READY_FOR_AUTHORIZATION"
            and readiness.get("engineering_state") in ("ENGINEERING_READY", "APP_SERVER_ENGINEERING_READY")
            and readiness.get("user_confirmation_state") == "CONFIRMED"
            and bool(readiness.get("user_confirmation_sha256"))
            and readiness.get("enabled") is True
            and readiness.get("call_authorized") is True
            and readiness.get("client_ready") is True
            and bool(authorization_ref)
            and readiness.get("authorization_ref_sha256") == hashlib.sha256(authorization_ref.encode()).hexdigest())


def new_service_ready(state, previous_generation):
    # The process lock can be held before the new endpoint is published.
    # An old status file must not send an acceptance request to a stale port.
    return (state.get("active") is True and bool(state.get("generation_id"))
            and state["generation_id"] != previous_generation and bool(state.get("address")))


def read_token(path):
    path = Path(path)
    if path.parent.stat().st_mode & 0o077:
        raise ValueError("token_directory_not_private")
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd) as f:
        info = os.fstat(f.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077:
            raise ValueError("token_file_not_private")
        token = f.read(65).strip()
    if len(token) != 48 or not token.startswith("nag1_"):
        raise ValueError("invalid_gateway_token_format")
    return token


def validate_result(task, events):
    assert task["status"] == "succeeded"
    assert task["execution"]["native_exit_code"] == 0
    assert task["execution"]["native_terminal"] == "success"
    assert task["execution"]["local_stop_confirmed"] is True
    assert task["protocol"]["status"] == "complete" and task["protocol"]["native_session_observed"] is True
    assert task["result"]["final"] is True and task["result"]["text"].strip() == EXPECTED
    assert task["budget"]["dispatches_consumed"] == task["budget"]["process_starts_consumed"] == 1
    assert events and [e["seq"] for e in events] == sorted(set(e["seq"] for e in events))
    assert sum(e["type"] == "task.completed" for e in events) == 1
    assert all(e["task_id"] == task["id"] for e in events)


def one_task(request, caps, key, save, adapter="codex-exec-v1"):
    policy = {"codex-exec-v1": "codex-limited-readonly-v1",
              "codex-app-server-v1": "codex-app-server-readonly-v1"}[adapter]
    agent = next(a for a in caps["agents"] if a["agent_id"] == "codex" and a["adapter_id"] == adapter)
    status, ses = request("POST", "/agent/v1/sessions", {
        "agent_id": "codex", "adapter_id": adapter, "capability_revision": caps["revision"],
        "policy_id": policy,
        "budget": {"max_dispatches": 1, "max_process_starts": 1, "max_execution_seconds": 120},
        "context": {"app": "worklog", "workflow": "codex-first-http", "opaque_ref": "synthetic"}}, key + "-session")
    assert status == 201 and ses["profile_sha256"]
    save({"session_id": ses["id"], "task_post_attempted": True})
    # The only inference POST in this program. Never retried, even after EOF.
    status, task = request("POST", "/agent/v1/tasks", {
        "session_id": ses["id"], "expected_session_revision": ses["revision"], "operation": "text_task",
        "input": [{"type": "text", "text": PROMPT}],
        "budget": {"max_dispatches": 1, "max_process_starts": 1, "timeout_seconds": 120},
        "consent": {"allow_remote": True, "context_sha256": ses["context_sha256"]}}, key + "-task")
    assert status == 202
    save({"task_id": task["id"], "profile_sha256": task["profile_sha256"]})
    deadline = time.monotonic() + 130
    while task["status"] in ("queued", "running", "validating", "cancel_requested"):
        if time.monotonic() >= deadline:
            request("POST", "/agent/v1/tasks/" + task["id"] + "/cancel", {}, key + "-cancel")
            raise ValueError("acceptance_deadline_exceeded")
        time.sleep(0.1)
        status, task = request("GET", "/agent/v1/tasks/" + task["id"])
        assert status == 200
    status, stream = request("GET", "/agent/v1/tasks/" + task["id"] + "/events", raw=True)
    assert status == 200
    events = [json.loads(line[6:]) for line in stream.splitlines() if line.startswith("data: ")]
    save({"task_status": task["status"], "error_code": (task.get("error") or {}).get("code"),
          "event_types": [e["type"] for e in events], "event_seqs": [e["seq"] for e in events],
          "local_stop_confirmed": task["execution"]["local_stop_confirmed"],
          "upstream_cancel_confirmed": task["execution"]["upstream_cancel_confirmed"]})
    validate_result(task, events)
    return task


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--adapter", choices=("codex-exec-v1", "codex-app-server-v1"), default="codex-exec-v1")
    parser.add_argument("--state-dir", required=True)
    parser.add_argument("--gateway-config", required=True)
    parser.add_argument("--token-file")
    parser.add_argument("--authorization-ref", default="")
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    if not __debug__:
        parser.error("Python optimization is forbidden: acceptance assertions must be enabled")
    command = [str(Path(args.binary).resolve(strict=True)), "agent-gateway", "--state-dir", args.state_dir,
               "--gateway-config", args.gateway_config]
    def run(*tail):
        result = subprocess.run(command + list(tail), capture_output=True, timeout=12, text=True)
        if result.returncode:
            raise ValueError("gateway_control_failed_no_raw_diagnostics")
        return result.stdout
    evidence = {"dry_run": not args.execute, "model_task_posts": 0, "result": "BLOCKED",
                "idempotency_key": "g5b-codex-" + uuid.uuid4().hex}
    # Reserve the evidence file before any possible network mutation. Reusing
    # --out fails instead of silently creating a second acceptance attempt.
    fd = os.open(args.out, os.O_RDWR | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w+") as output:
        def save(update):
            evidence.update(update)
            output.seek(0); json.dump(evidence, output, indent=2); output.truncate(); output.flush(); os.fsync(output.fileno())
        proc = None
        try:
            readiness = json.loads(run("app-server" if args.adapter == "codex-app-server-v1" else "codex-limited", "readiness"))
            save({"readiness": readiness})
            if not args.execute:
                save({"result": "DRY_RUN", "can_execute": execute_gate(readiness, args.authorization_ref)})
                return
            if not execute_gate(readiness, args.authorization_ref) or not args.token_file:
                raise ValueError("profile_or_call_authorization_not_ready")
            token = read_token(args.token_file)
            def start():
                nonlocal proc
                previous = json.loads(run("status"))
                if previous["active"]:
                    raise ValueError("gateway_already_active")
                proc = subprocess.Popen(command + ["serve"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                deadline = time.monotonic() + 7
                while time.monotonic() < deadline:
                    if proc.poll() is not None:
                        raise ValueError("gateway_exited")
                    try:
                        state = json.loads(run("status"))
                    except ValueError:
                        # Only read-only startup discovery is polled; Task
                        # submission still has exactly one POST attempt.
                        time.sleep(0.05)
                        continue
                    if new_service_ready(state, previous["generation_id"]):
                        return state["address"]
                    time.sleep(0.05)
                raise ValueError("gateway_start_timeout")
            address = start()
            def request(method, path, body=None, key=None, raw=False):
                # http.client has no proxy, redirect or POST retry behavior.
                conn = http.client.HTTPConnection(address, timeout=7)
                try:
                    headers = {"Authorization": "Bearer " + token}
                    if key: headers["Idempotency-Key"] = key
                    if body is not None: headers["Content-Type"] = "application/json"
                    conn.request(method, path, None if body is None else json.dumps(body), headers)
                    res = conn.getresponse(); data = res.read(8 * 1024 * 1024 + 1)
                    if len(data) > 8 * 1024 * 1024: raise ValueError("response_limit")
                    return res.status, data.decode() if raw else json.loads(data)
                finally:
                    conn.close()
            status, caps = request("GET", "/agent/v1/capabilities"); assert status == 200
            def record(update):
                if update.get("task_post_attempted"): update["model_task_posts"] = 1
                save(update)
            task = one_task(request, caps, evidence["idempotency_key"], record, args.adapter)
            run("stop"); proc.wait(timeout=7); proc = None
            address = start()
            status, restored = request("GET", "/agent/v1/tasks/" + task["id"])
            assert status == 200 and restored["status"] == "succeeded" and restored["budget"]["process_starts_consumed"] == 1
            assert restored["result"] == task["result"]
            save({"result": "PASS_SINGLE_TEXT_ONLY", "restart_persistence": True,
                  "result_sha256": hashlib.sha256(task["result"]["text"].encode()).hexdigest()})
        except (ValueError, AssertionError, OSError, subprocess.SubprocessError, KeyError, StopIteration):
            save({"result": "FAILED_OR_BLOCKED_NO_RETRY"})
            raise SystemExit(1)
        finally:
            if proc is not None and proc.poll() is None:
                try: run("stop"); proc.wait(timeout=7)
                except (ValueError, OSError, subprocess.SubprocessError):
                    proc.terminate()
                    try: proc.wait(timeout=7)
                    except subprocess.TimeoutExpired: save({"gateway_stop": "UNKNOWN_OWNER_RECONCILE_REQUIRED"})


if __name__ == "__main__":
    main()
