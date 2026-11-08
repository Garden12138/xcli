#!/usr/bin/env python3
"""Offline executable lifecycle smoke. Never invokes an installed Agent CLI."""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True)
    args = parser.parse_args()
    binary = str(Path(args.binary).resolve())
    with tempfile.TemporaryDirectory(prefix="xcli-nag-control-") as tmp:
        base = Path(tmp)
        config = base / "gateway.yaml"
        config.write_text("version: 1\nhost: 127.0.0.1\nport: 0\nqueue_limit: 4\n")
        state = base / "state"
        token_file = base / "token"
        command = [binary, "agent-gateway", "--state-dir", str(state), "--gateway-config", str(config)]
        def run(*tail):
            return subprocess.run(command + list(tail), check=True, text=True,
                                  capture_output=True, timeout=12).stdout
        run("client", "issue", "worklog", "--agent", "codex,cursor,antigravity",
            "--policy", "synthetic-text-readonly-v1", "--allow-remote",
            "--daily-dispatches", "2", "--daily-process-starts", "2",
            "--daily-execution-seconds", "300", "--token-file", str(token_file))
        token = token_file.read_text().strip()
        for agent in ("codex", "cursor", "antigravity"):
            preflight = json.loads(run("preflight", "--agent", agent))
            assert preflight["can_execute"] is False
            assert preflight["process_starts"] == preflight["model_requests"] == 0
            checks = {c["id"]: c for c in preflight["checks"]}
            assert checks["execution_policy"]["evidence"] == "policy_unenforceable"
            assert checks["account_entitlement"]["status"] == "NOT_TESTED"
        identities = []
        generations = []
        for iteration in range(2):
            proc = subprocess.Popen(command + ["serve"], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            try:
                until = time.monotonic() + 6
                status = None
                while time.monotonic() < until:
                    if proc.poll() is not None:
                        _, diagnostics = proc.communicate(timeout=1)
                        raise AssertionError("gateway exited during startup: " + diagnostics.decode("utf-8", "replace"))
                    try:
                        status = json.loads(run("status"))
                    except subprocess.CalledProcessError:
                        time.sleep(0.03)
                        continue
                    if status["active"] and (iteration == 0 or status["generation_id"] != generations[-1]):
                        break
                    time.sleep(0.03)
                assert status and status["active"]
                identities.append(status["instance_id"])
                generations.append(status["generation_id"])
                host, port = status["address"].rsplit(":", 1)
                conn = http.client.HTTPConnection(host, int(port), timeout=3)
                conn.request("GET", "/healthz")
                response = conn.getresponse()
                assert response.status == 200
                assert json.loads(response.read())["status"] == "ok"
                conn.request("GET", "/agent/v1/capabilities", headers={"Authorization": "Bearer " + token})
                response = conn.getresponse()
                assert response.status == 200
                capabilities = json.loads(response.read())
                assert capabilities["instance_id"] == identities[-1]
                assert capabilities["agents"][0]["operations"]["text_task"]["availability"] == "blocked"
                for agent, adapter in (("codex", "codex-exec-v1"), ("cursor", "cursor-headless-v1"), ("antigravity", "antigravity-print-v1")):
                    payload = {"agent_id": agent, "adapter_id": adapter,
                           "policy_id": "synthetic-text-readonly-v1",
                           "capability_revision": capabilities["revision"],
                           "budget": {"max_dispatches": 2, "max_process_starts": 2, "max_execution_seconds": 300},
                           "context": {"app": "worklog", "workflow": "synthetic", "opaque_ref": "offline"}}
                    conn.request("POST", "/agent/v1/sessions", json.dumps(payload),
                             headers={"Authorization": "Bearer " + token, "Content-Type": "application/json",
                                      "Idempotency-Key": "offline-session"})
                    response = conn.getresponse()
                    assert response.status == 503
                    assert json.loads(response.read())["error"]["code"] == "policy_unenforceable"
                conn.close()
                run("stop")
                proc.wait(timeout=7)
                assert proc.returncode == 0
                assert not json.loads(run("status"))["active"]
            finally:
                if proc.poll() is None:
                    proc.terminate()
                    proc.wait(timeout=7)
                proc.communicate(timeout=1)
        assert identities[0] == identities[1]
        assert generations[0] != generations[1]
        assert not (base / "model-gateway").exists()
        print(json.dumps({"test": "offline_control_smoke", "result": "PASS",
                          "binary_sha256": hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                          "native_model_requests": 0, "server_starts": 2,
                          "persistent_instance": True, "distinct_generations": True,
                          "native_profile_rejection": "policy_unenforceable"}, indent=2))

if __name__ == "__main__":
    main()
