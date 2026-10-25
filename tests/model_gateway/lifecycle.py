#!/usr/bin/env python3
"""Run a built xcli against an offline HTTP fixture and private temporary state."""
import argparse
import concurrent.futures
import http.server
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request


class Backend(http.server.BaseHTTPRequestHandler):
    calls = 0
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        Backend.calls += 1
        assert body["model"] == "fixture-text"
        content = {"id": "fixture", "object": "chat.completion", "created": 1,
                   "choices": [{"index": 0, "message": {"role": "assistant", "content": "synthetic result"}, "finish_reason": "stop"}]}
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(content).encode())
    def log_message(self, *args):
        pass


def run(binary):
    checks = []
    with tempfile.TemporaryDirectory(prefix="xcli-gateway-") as temporary:
        private = Path(temporary) / "private"
        private.mkdir(mode=0o700)
        state = private / "state"
        config = private / "gateway.yaml"
        upstream = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Backend)
        thread = threading.Thread(target=upstream.serve_forever, daemon=True)
        thread.start()
        config.write_text(f'''version: 1
server:
  host: 127.0.0.1
  port: 0
backends:
  fixture:
    type: openai-compatible
    endpoint: http://127.0.0.1:{upstream.server_port}/v1
    inference_location: local
models:
  xcli/text-default:
    backend: fixture
    upstream_model: fixture-text
    kind: text
    capabilities: [text.generate]
''')
        config.chmod(0o600)
        def cli(*args, ok=True):
            result = subprocess.run([str(binary), "--config", str(config), "model", "--state-dir", str(state), *args],
                                    capture_output=True, text=True, timeout=20)
            if ok and result.returncode:
                raise RuntimeError("CLI fixture command failed: " + result.stderr)
            if not ok and not result.returncode:
                raise RuntimeError("CLI command unexpectedly succeeded")
            return result.stdout
        def request(address, token, body=None):
            data = None if body is None else json.dumps(body).encode()
            headers = {"Authorization": "Bearer " + token, "Content-Type": "application/json"}
            req = urllib.request.Request("http://" + address + ("/v1/models" if body is None else "/v1/chat/completions"), data=data, headers=headers)
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with opener.open(req, timeout=3) as response:
                return json.load(response)
        try:
            token_file = state / "app-token"
            cli("client", "issue", "fixture-app", "--model", "xcli/text-default", "--token-file", str(token_file))
            token = token_file.read_text().strip()
            assert token not in cli("client", "list")
            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                states = list(pool.map(lambda _: json.loads(cli("start")), range(2)))
            first = states[0]
            assert first["active"] and first["reachable"] and states[1]["instance_id"] == first["instance_id"]
            checks.append("concurrent start is a single healthy instance; port 0 discovered")
            assert json.loads(cli("status"))["instance_id"] == first["instance_id"]
            assert request(first["address"], token)["data"][0]["id"] == "xcli/text-default"
            body = {"model": "xcli/text-default", "messages": [{"role": "user", "content": "PRIVATE_SYNTHETIC_SENTINEL"}]}
            assert request(first["address"], token, body)["choices"][0]["message"]["content"] == "synthetic result"
            logs = cli("logs")
            assert token not in logs and "PRIVATE_SYNTHETIC_SENTINEL" not in logs
            assert '"gateway_started"' in logs and first["config_digest"] in logs
            checks.append("real HTTP model request with private token; safe audit")
            new_token_file = state / "rotated-token"
            cli("client", "rotate", "fixture-app", "--token-file", str(new_token_file))
            try:
                request(first["address"], token)
                raise AssertionError("old token accepted")
            except urllib.error.HTTPError as error:
                assert error.code == 401
            new_token = new_token_file.read_text().strip()
            assert request(first["address"], new_token)["object"] == "list"
            cli("client", "revoke", "fixture-app")
            try:
                request(first["address"], new_token)
                raise AssertionError("revoked token accepted")
            except urllib.error.HTTPError as error:
                assert error.code == 401
            checks.append("token rotation and live revocation")
            original = config.read_text()
            config.write_text(original.replace("fixture-text", "changed-fixture"))
            cli("start", ok=False)
            config.write_text(original)
            checks.append("changed routes require an explicit stop/restart")
            cli("stop")
            assert not json.loads(cli("status"))["active"]
            cli("stop")
            second = json.loads(cli("start"))
            assert second["instance_id"] != first["instance_id"]
            time.sleep(0.2)  # An old stop request must not stop the replacement instance.
            assert json.loads(cli("status"))["active"]
            cli("stop")
            checks.append("stop/idempotent stop/restart/stale stop isolation")
            # Bind a foreign service to prove port conflicts cannot hijack state.
            config.write_text(original.replace("port: 0", f"port: {upstream.server_port}"))
            cli("start", ok=False)
            assert not json.loads(cli("status"))["active"]
            checks.append("foreign port conflict is explicit; no new active instance")
        finally:
            try:
                cli("stop")
            finally:
                upstream.shutdown()
                upstream.server_close()
    return {"evidence": "offline-built-binary", "application_e2e": "unverified", "checks": checks, "upstream_fixture_calls": Backend.calls}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    print(json.dumps(run(parser.parse_args().binary.resolve()), ensure_ascii=False))
