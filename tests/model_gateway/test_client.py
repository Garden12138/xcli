import importlib.util
from pathlib import Path
import sys
import unittest

path = Path(__file__).resolve().parents[2] / "examples/model-gateway/client.py"
spec = importlib.util.spec_from_file_location("gateway_client", path)
client = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = client
spec.loader.exec_module(client)


def descriptor(location="remote", fingerprint="fixture-one", dimensions=3):
    return {"id": "embed", "xcli": {"inference_location": location,
            "capabilities": ["embeddings", "text.generate", "text.stream"],
            "dimensions": dimensions, "index_version": "fixture-v1", "index_fingerprint": fingerprint}}


class PrivacyContract(unittest.TestCase):
    def test_unknown_and_remote_are_cloud(self):
        for value in (None, {}, descriptor(), descriptor("unknown")):
            self.assertTrue(client.effective_cloud(value))
        self.assertFalse(client.effective_cloud(descriptor("local")))

    def test_catalog_failure_or_denial_never_sends_content(self):
        api = client.GatewayClient("http://127.0.0.1:18741/v1", "synthetic-token")
        sent = []
        api._request = lambda *args, **kwargs: sent.append((args, kwargs))
        api.models = lambda: [descriptor("remote")]
        with self.assertRaises(PermissionError):
            api.chat("embed", [{"role": "user", "content": "synthetic-chat-only"}])
        api.models = lambda: []
        with self.assertRaises(PermissionError):
            api.embeddings("embed", ["synthetic-chat-only"])
        def unavailable():
            raise OSError("catalog unavailable")
        api.models = unavailable
        with self.assertRaises(OSError):
            api.chat("embed", [{"role": "user", "content": "synthetic"}])
        self.assertEqual(sent, [])

    def test_model_route_or_dimension_change_requires_rebuild(self):
        old = client.IndexIdentity.from_model(descriptor())
        self.assertFalse(old.requires_rebuild(old))
        for updated in (descriptor(fingerprint="new-route"), descriptor(dimensions=4)):
            self.assertTrue(client.IndexIdentity.from_model(updated).requires_rebuild(old))

    def test_local_provider_url_cannot_expand_ssrf_scope(self):
        for url in ("http://localhost:18741/v1", "http://192.168.1.1:18741/v1", "http://127.0.0.1.evil:18741/v1", "http://127.0.0.1:18741/v1?secret=x", "http://user@127.0.0.1:18741/v1"):
            with self.assertRaises(ValueError):
                client.GatewayClient(url, "fixture")


if __name__ == "__main__":
    unittest.main()
