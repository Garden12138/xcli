#!/usr/bin/env python3
"""Small desktop integration reference. No model requests run on import.

Only synthetic fixtures are used by tests. Real requests require an explicit caller.
"""
from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass


def local_base_url(value: str) -> str:
    parsed = urllib.parse.urlsplit(value)
    if (parsed.scheme != "http" or parsed.hostname not in ("127.0.0.1", "::1")
            or parsed.username or parsed.password or parsed.query or parsed.fragment
            or parsed.path.rstrip("/") != "/v1"):
        raise ValueError("xcli endpoint must be an explicit loopback /v1 URL")
    if not parsed.port:
        raise ValueError("use the discovered explicit gateway port")
    return value.rstrip("/")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def effective_cloud(descriptor: dict | None) -> bool:
    # A loopback HTTP URL never grants local-inference status.
    return not isinstance(descriptor, dict) or descriptor.get("xcli", {}).get("inference_location") != "local"


@dataclass(frozen=True)
class IndexIdentity:
    model: str
    dimensions: int
    version: str
    fingerprint: str

    @classmethod
    def from_model(cls, descriptor: dict) -> "IndexIdentity":
        meta = descriptor.get("xcli", {})
        if ("embeddings" not in meta.get("capabilities", [])
                or not isinstance(meta.get("dimensions"), int) or meta["dimensions"] < 1
                or not meta.get("index_version") or not meta.get("index_fingerprint")):
            raise ValueError("embedding identity is incomplete; do not reuse an existing index")
        return cls(descriptor["id"], meta["dimensions"], meta["index_version"], meta["index_fingerprint"])

    def requires_rebuild(self, previous: "IndexIdentity | None") -> bool:
        return self != previous


class GatewayClient:
    def __init__(self, base_url: str, token: str, timeout: float = 180):
        self.base_url = local_base_url(base_url)
        if not token or "\n" in token or "\r" in token:
            raise ValueError("a local app token is required")
        self.token, self.timeout = token, timeout
        # Do not leak localhost content through ambient proxy settings or redirects.
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())

    def _request(self, path: str, payload=None, consent=False):
        headers = {"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"}
        if consent:
            headers["X-Xcli-Remote-Consent"] = "allow"
        request = urllib.request.Request(self.base_url + path,
                                        data=None if payload is None else json.dumps(payload).encode(),
                                        headers=headers)
        return self.opener.open(request, timeout=self.timeout)

    def models(self):
        with self._request("/models") as response:
            data = json.load(response)
        if data.get("object") != "list" or not isinstance(data.get("data"), list):
            raise ValueError("untrusted model catalog")
        return data["data"]

    def authorize(self, alias: str, capability: str, allow_cloud: bool):
        # Refresh before every content-bearing request; failure is fail-closed.
        descriptor = next((d for d in self.models() if d.get("id") == alias), None)
        if descriptor is None or capability not in descriptor.get("xcli", {}).get("capabilities", []):
            raise PermissionError("configured model/capability is unavailable")
        cloud = effective_cloud(descriptor)
        if cloud and not allow_cloud:
            raise PermissionError("cloud or unknown inference requires the application's explicit consent")
        return descriptor, cloud

    def chat(self, alias: str, messages: list, *, allow_cloud=False, **options):
        _, cloud = self.authorize(alias, "text.generate", allow_cloud)
        if options.get("stream"):
            raise ValueError("use stream_chat for streaming and cancellation")
        with self._request("/chat/completions", {**options, "model": alias, "messages": messages, "stream": False}, cloud) as response:
            return json.load(response)

    def stream_chat(self, alias: str, messages: list, *, allow_cloud=False, **options):
        self.authorize(alias, "text.stream", allow_cloud)
        _, cloud = self.authorize(alias, "text.generate", allow_cloud)
        # Closing this generator closes the HTTP response and cancels the upstream.
        with self._request("/chat/completions", {**options, "model": alias, "messages": messages, "stream": True}, cloud) as response:
            for line in response:
                if not line.startswith(b"data: "):
                    continue
                data = line[6:].strip()
                if data == b"[DONE]":
                    return
                event = json.loads(data)
                if "error" in event:
                    raise RuntimeError("gateway stream failed")
                yield event
        raise RuntimeError("incomplete gateway stream")

    def embeddings(self, alias: str, inputs, *, allow_cloud=False, previous: IndexIdentity | None = None):
        descriptor, cloud = self.authorize(alias, "embeddings", allow_cloud)
        identity = IndexIdentity.from_model(descriptor)
        if previous is not None and identity.requires_rebuild(previous):
            raise ValueError("embedding identity changed; explicit index rebuild is required")
        with self._request("/embeddings", {"model": alias, "input": inputs, "encoding_format": "float"}, cloud) as response:
            result = json.load(response)
        # Catch a configuration change between the catalog check and inference response.
        meta = result.get("xcli", {})
        if (meta.get("index_fingerprint") != identity.fingerprint or meta.get("dimensions") != identity.dimensions):
            raise ValueError("embedding route changed during inference; discard result and rebuild explicitly")
        return result, identity
