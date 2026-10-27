#!/usr/bin/env python3
"""Exercise native desktop client code through a built gateway and local fixtures.

All sources are copied first. Rust probes are appended only inside temporary
source copies. No real application state, account, model or credential is used.
This proves native client HTTP compatibility, not application UI acceptance.
"""
from __future__ import annotations

import argparse
from collections import Counter
from email.parser import BytesParser
from email.policy import default
import hashlib
import http.server
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import time

from integration_workspace import snapshot


PROBES = Path(__file__).with_name("probes")
PNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="


class Fixture(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", "0")))
        if self.path == "/v1/images/edits":
            message = BytesParser(policy=default).parsebytes(
                ("Content-Type: " + self.headers["Content-Type"] + "\r\n\r\n").encode() + body)
            parts = list(message.iter_parts())
            fields = {part.get_param("name", header="content-disposition"): part.get_payload(decode=True)
                      for part in parts if not part.get_filename()}
            assert fields["model"] == b"fixture-image"
            assert any(part.get_filename() and part.get_content_type() == "image/png" for part in parts)
            request = {"model": "fixture-image"}
        else:
            request = json.loads(body)
        model = request["model"]
        with self.server.calls_lock:
            self.server.calls[(self.path, model)] += 1
        if self.path.startswith("/v1/images/"):
            response = {"created": 1, "data": [{"b64_json": PNG}]}
        elif self.path == "/v1/embeddings":
            inputs = request["input"]
            inputs = inputs if isinstance(inputs, list) else [inputs]
            response = {"object": "list", "data": [
                {"object": "embedding", "index": i, "embedding": [0.1, 0.2, 0.3]} for i, _ in enumerate(inputs)
            ]}
        elif self.path == "/v1/chat/completions":
            if request.get("stream"):
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.end_headers()
                for delta, finish in [({"role": "assistant", "content": "synthetic-"}, None),
                                      ({"content": "native-result"}, None), ({}, "stop")]:
                    chunk = {"id": "native-fixture", "object": "chat.completion.chunk", "created": 1,
                             "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}
                    try:
                        self.wfile.write(("data: " + json.dumps(chunk) + "\n\n").encode())
                        self.wfile.flush()
                        time.sleep(0.01)
                    except (BrokenPipeError, ConnectionResetError):
                        return
                try:
                    self.wfile.write(b"data: [DONE]\n\n")
                except (BrokenPipeError, ConnectionResetError):
                    pass
                return
            content = "synthetic-native-result"
            if model == "fixture-json":
                content = json.dumps({"task": "synthetic milestone", "progress": "validated", "result": "synthetic result", "blockers": None})
            elif request.get("response_format"):
                content = '{"ok":true}'
            message = {"role": "assistant", "content": content}
            finish = "stop"
            if request.get("tools"):
                message = {"role": "assistant", "content": None, "tool_calls": [{"id": "call_native", "type": "function", "function": {
                    "name": request["tools"][0]["function"]["name"], "arguments": '{"ok":true}',
                }}]}
                finish = "tool_calls"
            response = {"id": "native-fixture", "object": "chat.completion", "created": 1,
                        "choices": [{"index": 0, "message": message, "finish_reason": finish}],
                        "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}}
        else:
            self.send_error(404)
            return
        encoded = json.dumps(response).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, *args):
        pass


def execute(arguments, *, cwd=None, environment=None, timeout=240):
    result = subprocess.run([str(argument) for argument in arguments], cwd=cwd, env=environment,
                            capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f"{Path(arguments[0]).name} failed ({result.returncode})\n" + (result.stdout + result.stderr)[-6000:])
    return result.stdout


def node_dependencies(original: Path, copied: Path):
    destination = copied / "node_modules"
    destination.mkdir()
    for entry in (original / "node_modules").iterdir():
        if not entry.name.startswith(".") or entry.name in {".bin", ".pnpm"}:
            (destination / entry.name).symlink_to(entry, target_is_directory=entry.is_dir())
    for name in (".tmp", ".vite-temp", ".vite", ".cache"):
        (destination / name).mkdir()


def run(args):
    binary = args.binary.resolve()
    sources = args.sources_root.resolve()
    report = {"evidence": "native-client-code-through-built-gateway", "application_ui_e2e": "unverified",
              "real_model_calls": 0, "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(), "checks": {}}
    with tempfile.TemporaryDirectory(prefix="xcli-native-consumers-") as temporary:
        directory = Path(temporary)
        upstream = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fixture)
        upstream.calls, upstream.calls_lock = Counter(), threading.Lock()
        threading.Thread(target=upstream.serve_forever, daemon=True).start()
        state = directory / "state"
        config = directory / "gateway.yaml"
        metadata = {"verification": "probe_verified", "verified_at": "2026-10-25T00:00:00Z", "evidence": "native-client-synthetic-offline-only"}
        models = {}
        for alias, backend, name, kind, caps in [
            ("text-default", "local", "fixture-text", "text", ["text.generate", "text.stream", "tool_calls", "json_schema", "json_object", "vision.analyze"]),
            ("text-json", "local", "fixture-json", "text", ["text.generate", "json_object"]),
            ("text-cloud", "remote", "fixture-text-cloud", "text", ["text.generate", "text.stream"]),
            ("image-default", "local", "fixture-image", "image", ["image.generate", "image.edit", "image.multi_reference"]),
            ("embed-default", "local", "fixture-embed", "embedding", ["embeddings"]),
        ]:
            models["xcli/" + alias] = {"backend": backend, "upstream_model": name, "kind": kind, "capabilities": caps, **metadata}
        models["xcli/embed-default"].update(dimensions=3, index_version="native-v1")
        configuration = {"version": 1, "server": {"host": "127.0.0.1", "port": 0, "timeout_seconds": 30},
                         "backends": {location: {"type": "openai-compatible", "endpoint": f"http://127.0.0.1:{upstream.server_port}/v1", "inference_location": location} for location in ("local", "remote")},
                         "models": models}
        config.write_text(json.dumps(configuration))  # JSON is valid YAML.
        config.chmod(0o600)
        def cli(*arguments):
            return execute([binary, "--config", config, "model", "--state-dir", state, *arguments], timeout=20)
        started = False
        try:
            base_environment = {key: value for key, value in os.environ.items()
                                if not key.startswith("CONTRIBOS_RUN_") and key not in {"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"}}
            tokens = {}
            scopes = {"worklog": ["text-default", "text-json"], "chat-insight": ["text-default", "text-cloud", "embed-default"],
                      "garden-flow": ["text-default", "image-default"], "contribution-os": ["text-default"]}
            for name, aliases in scopes.items():
                token_file = directory / (name + ".token")
                command = ["client", "issue", name, "--token-file", str(token_file)]
                for alias in aliases:
                    command += ["--model", "xcli/" + alias]
                if name == "chat-insight":
                    command += ["--allow-remote"]
                cli(*command)
                tokens[name] = token_file
            status = json.loads(cli("start"))
            started = True
            base_url = "http://" + status["address"] + "/v1"
            for name in ("worklog", "chat-insight", "garden-flow", "ai-open-source-contribution-os-summary"):
                print("Running native client: " + name, flush=True)
                destination = directory / ("source-" + name)
                snapshot(sources / name, destination)
                work = destination / "work"
                identity = "contribution-os" if name == "ai-open-source-contribution-os-summary" else name
                environment = {**base_environment, "XCLI_NATIVE_BASE_URL": base_url,
                               "XCLI_NATIVE_TOKEN_FILE": str(tokens[identity]), "XCLI_NATIVE_SOURCE": str(work)}
                if name in {"worklog", "chat-insight"}:
                    relative = "src-tauri/src/xcli.rs" if name == "worklog" else "src-tauri/src/ai/client.rs"
                    source = work / relative
                    probe = PROBES / ("worklog.rs" if name == "worklog" else "chat_insight.rs")
                    source.write_text(source.read_text() + "\n" + probe.read_text())
                    target = args.worklog_target if name == "worklog" else args.chat_insight_target
                    environment["CARGO_TARGET_DIR"] = str(target.resolve())
                    output = execute(["cargo", "test", "--offline", "--manifest-path", work / "src-tauri/Cargo.toml",
                                      "--lib", "native_gateway_contract_probe"], environment=environment)
                    assert "1 passed; 0 failed" in output
                    report["checks"][identity] = {"native_rust_probe": "passed", "source_head": json.loads((destination / "manifest.json").read_text())["head"]}
                elif name == "garden-flow":
                    node_dependencies(sources / name / "desktop", work / "desktop")
                    output = execute(["node", PROBES / "garden_flow.mjs"], cwd=work, environment=environment)
                    report["checks"][identity] = json.loads(output)
                else:
                    environment["PYTHONPATH"] = str(work)
                    output = execute([args.contribution_python, PROBES / "contribution_os.py"], cwd=work, environment=environment)
                    report["checks"][identity] = json.loads(output)
            with upstream.calls_lock:
                counts = dict(upstream.calls)
            assert counts[("/v1/chat/completions", "fixture-text-cloud")] == 1, "Unconsented cloud request reached the fixture"
            assert counts[("/v1/embeddings", "fixture-embed")] == 1, "Unapproved index reached the fixture"
            assert counts[("/v1/images/generations", "fixture-image")] == 1
            assert counts[("/v1/images/edits", "fixture-image")] == 1
            logs = cli("logs")
            assert all(token.read_text().strip() not in logs for token in tokens.values())
            report["fixture_model_requests"] = sum(counts.values())
            report["fixture_requests_by_route"] = [{"path": path, "model": model, "count": count} for (path, model), count in sorted(counts.items())]
            report["unconsented_cloud_uploads"] = 0
            report["unapproved_embedding_uploads"] = 0
            report["token_audit_leaks"] = 0
        finally:
            if started:
                cli("stop")
            upstream.shutdown()
            upstream.server_close()
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--sources-root", type=Path, required=True)
    parser.add_argument("--contribution-python", type=Path, required=True)
    parser.add_argument("--worklog-target", type=Path, required=True)
    parser.add_argument("--chat-insight-target", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    arguments = parser.parse_args()
    result = run(arguments)
    arguments.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(result, ensure_ascii=False))
