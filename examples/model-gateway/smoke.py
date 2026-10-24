#!/usr/bin/env python3
"""Opt-in synthetic provider-contract checks; never reads application databases.

Without --execute only the authorized model catalog is fetched. --execute may be
billable on a real configured upstream and requires an explicit request budget.
These are API consumer profiles, not end-to-end application acceptance.
"""
import argparse
import json
import os
import struct
import zlib
from client import GatewayClient, IndexIdentity


def synthetic_png():
    def chunk(tag, data):
        return struct.pack("!I", len(data)) + tag + data + struct.pack("!I", zlib.crc32(tag + data))
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack("!2I5B", 2, 2, 8, 6, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(b"\0" + b"\0" * 8 + b"\0" + b"\0" * 8)) + chunk(b"IEND", b""))


def run(args):
    token = os.environ.get("XCLI_APP_TOKEN", "")
    api = GatewayClient(args.base_url, token)
    catalog = api.models()
    by_id = {d["id"]: d for d in catalog}
    report = {"profile": args.profile, "evidence": "synthetic-api-contract-only",
              "application_e2e": "unverified", "model_requests": 0, "checks": []}
    if not args.execute:
        report["checks"].append("authorized catalog parsed; no inference requested")
        return report
    if args.max_model_requests is None or args.max_model_requests < 1:
        raise ValueError("--execute requires a positive --max-model-requests budget")

    def call(label, operation):
        if report["model_requests"] >= args.max_model_requests:
            raise RuntimeError("explicit model request budget exhausted; no retries")
        report["model_requests"] += 1
        result = operation()
        report["checks"].append(label)
        return result

    messages = [{"role": "user", "content": 'Synthetic acceptance only. Return exactly {"ok":true}.'}]
    profile = args.profile
    if profile == "worklog":
        result = call("nonstream text/JSON content", lambda: api.chat(args.text_model, messages, allow_cloud=args.allow_cloud))
        assert json.loads(result["choices"][0]["message"]["content"])["ok"] is True
    elif profile == "garden-flow":
        chunks = call("SSE delta and termination", lambda: list(api.stream_chat(args.text_model, messages, allow_cloud=args.allow_cloud)))
        assert len(chunks) >= 2
        image = call("image generation base64", lambda: api.generate_image(args.image_model, "Synthetic blue square", size=args.image_size, n=1, allow_cloud=args.allow_cloud))
        assert image["data"][0]["b64_json"]
        call("reference image edit", lambda: api.edit_image(args.image_model, "Synthetic edit", synthetic_png(), size=args.image_size, allow_cloud=args.allow_cloud))
    elif profile == "chat-insight":
        result, identity = call("batch embedding identity", lambda: api.embeddings(args.embed_model, ["synthetic one", "synthetic two"], allow_cloud=args.allow_cloud))
        assert len(result["data"]) == 2 and isinstance(identity, IndexIdentity)
        call("chat SSE", lambda: list(api.stream_chat(args.text_model, messages, allow_cloud=args.allow_cloud)))
    elif profile == "contribution-os":
        # This exercises the Provider surface only. Internal scopes/frozen Profiles
        # must still be tested inside Contribution OS before enabling the adapter.
        caps = by_id[args.text_model].get("xcli", {}).get("capabilities", [])
        if "json_schema" not in caps or "tool_calls" not in caps:
            raise PermissionError("Contribution OS requires verified json_schema and tool_calls")
        result = call("strict JSON parameter passthrough", lambda: api.chat(args.text_model, messages,
                      allow_cloud=args.allow_cloud, response_format={"type": "json_schema", "json_schema": {
                          "name": "synthetic_decision", "strict": True, "schema": {"type": "object",
                          "properties": {"ok": {"type": "boolean"}}, "required": ["ok"], "additionalProperties": False}}}))
        assert json.loads(result["choices"][0]["message"]["content"])["ok"] is True
        result = call("tool function schema/id/arguments", lambda: api.chat(args.text_model, messages,
                      allow_cloud=args.allow_cloud, tools=[{"type": "function", "function": {"name": "synthetic_decision",
                      "parameters": {"type": "object", "properties": {"ok": {"type": "boolean"}}, "required": ["ok"]}}}],
                      tool_choice={"type": "function", "function": {"name": "synthetic_decision"}}))
        choice = result["choices"][0]
        assert choice["finish_reason"] == "tool_calls" and choice["message"]["tool_calls"][0]["id"]
    return report


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--base-url", required=True)
    p.add_argument("--profile", choices=["worklog", "garden-flow", "chat-insight", "contribution-os"], required=True)
    p.add_argument("--execute", action="store_true")
    p.add_argument("--max-model-requests", type=int)
    p.add_argument("--allow-cloud", action="store_true")
    p.add_argument("--text-model", default="xcli/text-default")
    p.add_argument("--image-model", default="xcli/image-default")
    p.add_argument("--embed-model", default="xcli/embed-default")
    p.add_argument("--image-size", default="1024x1024")
    args = p.parse_args()
    try:
        print(json.dumps(run(args), ensure_ascii=False))
    except Exception as error:
        # Do not print upstream payloads, prompts, token-bearing URLs or credentials.
        print(json.dumps({"profile": args.profile, "status": "failed", "error_type": type(error).__name__}))
        raise SystemExit(1) from None


if __name__ == "__main__":
    main()
