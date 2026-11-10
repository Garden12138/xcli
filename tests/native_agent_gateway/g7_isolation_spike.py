#!/usr/bin/env python3
"""Bounded G7 Linux host-isolation experiment. Default dry-run; no turns/auth.

Only an owner-created synthetic workspace and simulated credentials are used.
This tool never downloads assets, changes virtualization settings or enables a
real profile. Its fixed RPC allowlist excludes thread/turn/account operations.
"""
import argparse
import base64
import hashlib
import http.client
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import os
from pathlib import Path
import queue
import re
import subprocess
import tempfile
import threading
import time
import uuid

MATERIAL = b"XCLI_G7_SYNTHETIC_MATERIAL\n"
FAKE_ACCESS = "XCLI_G7_SIMULATED_ACCESS_NOT_A_REAL_TOKEN"
ALLOWED_RPC = {"initialize", "initialized", "fs/readFile"}


def rpc_message(method, ident, params):
    if method not in ALLOWED_RPC:
        raise ValueError("non_inference_method_not_allowed")
    value = {"method": method, "params": params}
    if ident is not None:
        value["id"] = ident
    return json.dumps(value).encode() + b"\n"


def docker_args(meta, name):
    if not re.fullmatch(r"sha256:[a-f0-9]{64}", meta["image"]):
        raise ValueError("pinned_image_required")
    ws = Path(meta["workspace"])
    if not ws.is_absolute() or ws.resolve(strict=True) != ws or "," in str(ws):
        raise ValueError("canonical_workspace_required")
    if (ws / "material.txt").read_bytes() != MATERIAL:
        raise ValueError("synthetic_material_required")
    if hashlib.sha256((ws / "codex").read_bytes()).hexdigest() != meta["linux_binaries"]["codex"]["sha256"]:
        raise ValueError("binary_changed")
    return ["run", "--rm", "--pull=never", "--platform=linux/arm64", "--name", name,
            "--label", "xcli.g7.spike=" + name, "--network=none", "--read-only", "--user=65532:65532",
            "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=512m",
            "--cpus=1", "--ipc=none", "--interactive", "--tmpfs",
            "/tmp:rw,noexec,nosuid,nodev,size=16m,mode=1777", "--tmpfs",
            "/home/xcli:rw,noexec,nosuid,nodev,size=32m,mode=0700,uid=65532,gid=65532",
            "--mount", "type=bind,src=" + str(ws) + ",dst=/workspace,readonly", "--workdir=/workspace",
            "--env", "HOME=/home/xcli", "--env", "CODEX_HOME=/home/xcli",
            "--env", "PATH=/usr/bin:/bin", "--env", "ACCESS_TOKEN=" + FAKE_ACCESS,
            "--entrypoint=/workspace/codex", meta["image"], "app-server", "--listen", "stdio://",
            "-c", 'mcp_servers={}', "-c", 'features.apps=false', "-c", 'features.plugins=false',
            "-c", 'features.hooks=false', "-c", 'cli_auth_credentials_store="ephemeral"']


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--metadata", required=True)
    parser.add_argument("--socket", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--execute-non-model", action="store_true")
    args = parser.parse_args()
    report = {"stage": "G7_A1_A4_AND_SINGLE_SIMULATED_B_CHECK", "result": "DRY_RUN",
              "real_model_requests": 0, "real_credentials_used": False, "sent_methods": []}
    with open(args.out, "x", encoding="utf-8") as output:
        os.chmod(args.out, 0o600)
        def save():
            output.seek(0); json.dump(report, output, indent=2); output.truncate(); output.flush(); os.fsync(output.fileno())
        if not args.execute_non_model:
            save(); return
        meta = json.loads(Path(args.metadata).read_text())
        ws = Path(meta["workspace"])
        if not args.socket.startswith("unix:///"):
            raise ValueError("local_socket_required")
        name = "xcli-g7-probe-" + uuid.uuid4().hex
        for label, target in (("escape-host", meta["control_file"]), ("escape-relative", "../trusted-control/auth.json")):
            link = ws / label
            if not link.exists() and not link.is_symlink(): link.symlink_to(target)
        with tempfile.TemporaryDirectory(prefix="xcli-g7-docker-", dir="/private/tmp") as tmp:
            env = {"HOME": tmp, "DOCKER_CONFIG": tmp, "DOCKER_HOST": args.socket,
                   "PATH": "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin", "LANG": "C.UTF-8"}
            command = ["/usr/local/bin/docker", "--host", args.socket, "--config", tmp]
            def control(*tail):
                return subprocess.run(command + list(tail), env=env, cwd=tmp, capture_output=True, timeout=5)
            request_queue = queue.Queue(maxsize=32)
            native = subprocess.Popen(command + docker_args(meta, name), env=env, cwd=tmp,
                                      stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            def read_frames():
                total = 0
                try:
                    while True:
                        line = native.stdout.readline((1 << 20) + 1)
                        if not line: break
                        total += len(line)
                        if len(line) > 1 << 20 or total > 8 << 20:
                            raise ValueError("bounded_output_exceeded")
                        request_queue.put(json.loads(line), timeout=1)
                except Exception:
                    request_queue.put({"probe_error": True}, timeout=1)
                finally:
                    request_queue.put({"probe_eof": True}, timeout=1)
            reader = threading.Thread(target=read_frames, daemon=True); reader.start()
            seq = 0
            def request(method, params):
                nonlocal seq
                seq += 1
                native.stdin.write(rpc_message(method, seq, params)); native.stdin.flush()
                report["sent_methods"].append(method)
                end = time.monotonic() + 5
                while time.monotonic() < end:
                    msg = request_queue.get(timeout=max(.01, end - time.monotonic()))
                    if msg.get("probe_error") or msg.get("probe_eof"): raise ValueError("rpc_stream_failed")
                    if "method" in msg:
                        if "id" in msg:
                            reply = {"id": msg["id"], "error": {"code": -32601, "message": "not permitted"}}
                            native.stdin.write(json.dumps(reply).encode() + b"\n"); native.stdin.flush()
                        continue
                    if msg.get("id") != seq: raise ValueError("unmatched_reply")
                    return msg
                raise ValueError("rpc_timeout")
            hits = [0]; revoked = [False]
            class MockControl(BaseHTTPRequestHandler):
                def do_GET(self):
                    hits[0] += 1
                    self.send_response(403 if revoked[0] else 204); self.end_headers()
                def log_message(self, *_): pass
            mock = HTTPServer(("127.0.0.1", 0), MockControl)
            thread = threading.Thread(target=mock.serve_forever, daemon=True); thread.start()
            try:
                init = request("initialize", {"clientInfo": {"name": "xcli_g7_offline_spike", "version": "1"},
                                              "capabilities": {"experimentalApi": False}})
                if not isinstance(init.get("result"), dict): raise ValueError("initialize_failed")
                native.stdin.write(rpc_message("initialized", None, {})); native.stdin.flush()
                report["sent_methods"].append("initialized"); report["A1_initialize"] = "PASS"
                checks = {}
                for label, path in [("material", "/workspace/material.txt"), ("host_control", meta["control_file"]),
                                    ("symlink_host", "/workspace/escape-host"), ("symlink_relative", "/workspace/escape-relative"),
                                    ("vm_host_alias", "/mnt/mac" + meta["control_file"]),
                                    ("daemon_socket", "/var/run/docker.sock")]:
                    reply = request("fs/readFile", {"path": path})
                    if label == "material":
                        data = base64.b64decode(reply.get("result", {}).get("dataBase64", ""))
                        checks[label] = "PASS" if data == MATERIAL else "FAIL"
                    else:
                        checks[label] = "PASS_DENIED_OR_ABSENT" if "error" in reply else "FAIL_READABLE"
                # One decisive fake-only credential-domain check; not a login.
                reply = request("fs/readFile", {"path": "/proc/self/environ"})
                data = base64.b64decode(reply.get("result", {}).get("dataBase64", ""))
                report["B_simulated_access_in_app_server_env_readable"] = FAKE_ACCESS.encode() in data
                child = control("exec", name, "/bin/sh", "-c",
                                "if grep -q XCLI_G7_SIMULATED_ACCESS_NOT_A_REAL_TOKEN /proc/1/environ; then echo VISIBLE; else echo DENIED; fi")
                report["B_same_uid_child_can_read_parent_simulated_access"] = child.returncode == 0 and child.stdout.strip() == b"VISIBLE"
                uid = control("exec", name, "/usr/bin/id", "-u")
                checks["untrusted_uid"] = "PASS" if uid.returncode == 0 and uid.stdout.strip() == b"65532" else "FAIL"
                attempt = control("exec", name, "/bin/sh", "-c", "touch /workspace/forbidden-write")
                checks["workspace_readonly"] = "PASS" if attempt.returncode != 0 and not (ws / "forbidden-write").exists() else "FAIL"
                rename = control("exec", name, "/bin/sh", "-c", "mv /workspace /tmp/replaced-workspace")
                checks["untrusted_directory_replace_denied"] = "PASS" if rename.returncode != 0 and (ws / "material.txt").read_bytes() == MATERIAL else "FAIL"
                conn = http.client.HTTPConnection("127.0.0.1", mock.server_port, timeout=1)
                conn.request("GET", "/mock-control"); response = conn.getresponse(); response.read(); conn.close()
                positive = response.status == 204 and hits[0] == 1
                net = control("exec", name, "/bin/sh", "-c", "nc -z -w 1 127.0.0.1 " + str(mock.server_port))
                checks["mock_control_network_denied"] = "PASS" if positive and net.returncode != 0 and hits[0] == 1 else "FAIL"
                revoked[0] = True
                conn = http.client.HTTPConnection("127.0.0.1", mock.server_port, timeout=1)
                conn.request("GET", "/mock-control"); response = conn.getresponse(); response.read(); conn.close()
                checks["mock_revocation"] = "PASS_SIMULATION_ONLY" if response.status == 403 else "FAIL"
                inspect = control("inspect", name, "--format", "{{json .HostConfig}}")
                config = json.loads(inspect.stdout)
                checks["kernel_container_constraints"] = "PASS" if config["ReadonlyRootfs"] and config["NetworkMode"] == "none" and not config["Privileged"] and "ALL" in config["CapDrop"] and "no-new-privileges" in config["SecurityOpt"] else "FAIL"
                report["checks"] = checks
                report["A3_A4"] = "PASS_HOST_ISOLATION_ONLY" if all(v.startswith("PASS") for v in checks.values()) else "FAIL"
                native.stdin.close(); native.wait(timeout=5)
                report["A2_exit_code"] = native.returncode
                report["native_stderr_sha256"] = hashlib.sha256(native.stderr.read(65536)).hexdigest()
                report["result"] = "PARTIAL_HOST_ISOLATION_ONLY" if report["A3_A4"].startswith("PASS") and native.returncode == 0 else "NO_GO_CURRENT_ARCHITECTURE"
            except Exception as error:
                report["result"] = "EXPERIMENT_FAILED"; report["failure_type"] = type(error).__name__
                report["failure_code"] = str(error) if str(error) in {"rpc_stream_failed", "initialize_failed", "unmatched_reply", "rpc_timeout"} else "unexpected_exception"
            finally:
                mock.shutdown(); mock.server_close()
                # Only our exact random name/label can be removed.
                remaining = control("container", "ls", "--all", "--filter", "name=^/" + name + "$",
                                    "--filter", "label=xcli.g7.spike=" + name, "--format", "{{.ID}}")
                if remaining.returncode == 0 and remaining.stdout.strip():
                    ident = remaining.stdout.decode().strip()
                    if re.fullmatch(r"[a-f0-9]{12,64}", ident): control("container", "rm", "--force", ident)
                if native.poll() is None: native.terminate(); native.wait(timeout=5)
                report["native_exit_code"] = native.returncode
                diagnostics = native.stderr.read(65536).decode("utf-8", "replace")
                report["native_stderr_excerpt"] = diagnostics[:3000].replace(str(ws), "<WORKSPACE>").replace(tmp, "<DOCKER_CONFIG>").replace(FAKE_ACCESS, "<SIMULATED_TOKEN>")
                remaining = control("container", "ls", "--all", "--filter", "label=xcli.g7.spike=" + name, "--format", "{{.ID}}")
                report["daemon_workload_stop_confirmed"] = remaining.returncode == 0 and not remaining.stdout.strip()
                save()


if __name__ == "__main__":
    main()
