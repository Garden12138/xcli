#!/usr/bin/env python3
"""Owner-only HTTP rejection preflight against real cached installations.

Exactly three official --version launches via xcli refresh, zero native tasks.
No Session is accepted and no inference prompt is submitted. Uses temporary
config/state/token, leaves existing Gateway and native settings untouched.
"""
import argparse
import http.client
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    for agent in ("codex", "cursor", "antigravity"):
        parser.add_argument("--" + agent, required=True)
    parser.add_argument("--execute-non-model", action="store_true")
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    if not args.execute_non_model:
        parser.error("explicit --execute-non-model required")
    binary = str(Path(args.binary).resolve(strict=True))
    report = {"test": "real_installation_http_rejection_preflight", "native_process_starts": 0,
              "model_requests_sent": 0, "native_tasks_submitted": 0, "installations": [],
              "preflight": [], "http": []}
    with tempfile.TemporaryDirectory(prefix="xcli-g5a-http-") as tmp:
        base = Path(tmp)
        state, token = base / "state", base / "client.token"
        config = base / "gateway.yaml"
        # Nonzero budgets still cannot promote a real profile.
        config.write_text("version: 1\nhost: 127.0.0.1\nport: 0\nqueue_limit: 4\n"
                          "daily_dispatches: 1\ndaily_process_starts: 1\ndaily_execution_seconds: 120\n")
        command = [binary, "agent-gateway", "--state-dir", str(state), "--gateway-config", str(config)]
        env = {"HOME": tmp, "PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "LANG": "C.UTF-8"}
        def run(*tail):
            return subprocess.run(command + list(tail), env=env, check=True, capture_output=True,
                                  timeout=12, text=True).stdout
        for agent in ("codex", "cursor", "antigravity"):
            path = getattr(args, agent)
            if not Path(path).is_absolute():
                raise ValueError("absolute native executable required")
            report["native_process_starts"] += 1
            try:
                installation = json.loads(run("refresh", "--agent", agent, "--binary", path))
                installation["probe_status"] = "PASS"
            except subprocess.CalledProcessError as err:
                # Only xcli's fixed failure code, never native diagnostics.
                code = next((s for s in ("version_probe_start_failed", "version_probe_stop_unconfirmed",
                    "version_probe_timeout_or_canceled", "version_probe_exit_nonzero", "version_probe_output_limit",
                    "version_probe_io_failed") if s in err.stderr), "refresh_failed")
                installation = {"agent_id": agent, "probe_status": "FAIL", "reason": code}
            report["installations"].append(installation)
            result = json.loads(run("preflight", "--agent", agent))
            assert not result["can_execute"] and result["process_starts"] == 0
            report["preflight"].append(result)
        run("client", "issue", "worklog", "--agent", "codex,cursor,antigravity",
            "--policy", "synthetic-text-readonly-v1", "--allow-remote",
            "--daily-dispatches", "1", "--daily-process-starts", "1",
            "--daily-execution-seconds", "120", "--token-file", str(token))
        secret = token.read_text().strip()
        proc = subprocess.Popen(command + ["serve"], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            deadline = time.monotonic() + 6
            status = None
            while time.monotonic() < deadline:
                status = json.loads(run("status"))
                if status["active"]:
                    break
                time.sleep(0.05)
            assert status and status["active"]
            host, port = status["address"].rsplit(":", 1)
            conn = http.client.HTTPConnection(host, int(port), timeout=3)
            headers = {"Authorization": "Bearer " + secret}
            conn.request("GET", "/agent/v1/capabilities", headers=headers)
            response = conn.getresponse()
            caps = json.loads(response.read())
            assert response.status == 200 and len(caps["agents"]) == 3
            report["capabilities"] = caps
            for agent in caps["agents"]:
                assert agent["operations"]["text_task"]["availability"] == "blocked"
                body = {"agent_id": agent["agent_id"], "adapter_id": agent["adapter_id"],
                        "policy_id": "synthetic-text-readonly-v1", "capability_revision": caps["revision"],
                        "budget": {"max_dispatches": 1, "max_process_starts": 1, "max_execution_seconds": 120},
                        "context": {"app": "worklog", "workflow": "preflight", "opaque_ref": "synthetic"}}
                conn.request("POST", "/agent/v1/sessions", json.dumps(body), headers={**headers,
                    "Content-Type": "application/json", "Idempotency-Key": "g5a-session-" + agent["agent_id"]})
                response = conn.getresponse()
                result = json.loads(response.read())
                assert response.status == 503 and result["error"]["code"] == "policy_unenforceable"
                report["http"].append({"agent": agent["agent_id"], "status": response.status, "error": result["error"]})
            conn.close()
            run("stop")
            proc.wait(timeout=7)
            assert proc.returncode == 0
            report["result"] = "PASS_REJECTION_ONLY"
        finally:
            if proc.poll() is None:
                proc.terminate()
                proc.wait(timeout=7)
    with open(args.out, "x") as f:
        json.dump(report, f, indent=2)
        f.write("\n")
    print(json.dumps({"result": report["result"], "native_version_starts": report["native_process_starts"],
                      "model_requests_sent": 0}))


if __name__ == "__main__":
    main()
