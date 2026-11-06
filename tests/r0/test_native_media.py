"""Offline checks for image bytes and transparent native observation; no models."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from PIL import Image
import native_audit
import native_media


class NativeMediaTests(unittest.TestCase):
    def test_missing_nested_tool_event_requires_review_without_failing_valid_bytes(self):
        self.assertEqual(native_media.bytes_status(True, 0), "PASS_BYTES_PENDING_REVIEW")
        self.assertEqual(native_media.bytes_status(True, 1), "PASS_BYTES_PENDING_VISUAL")
        self.assertIsNone(native_media.bytes_status(False, 3))

    def test_billing_or_permission_signal_blocks_even_with_valid_image(self):
        self.assertEqual(native_media.media_status(True, 1, "extra_billing"), "BLOCKED")
        self.assertEqual(native_media.media_status(True, 1, "permissions"), "BLOCKED")

    def test_initial_tool_catalog_is_not_tool_execution(self):
        self.assertEqual(native_audit.observed_tools({"event": "init", "tools": [{"name": "generate_image"}]}), set())
        self.assertIn("generate_image", native_audit.observed_tools({"event": "step_update", "tool_name": "generate_image"}))

    def test_audit_forwards_exact_bytes_without_retaining_content(self):
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            child = root / "fixture.py"
            payload = b'{"type":"item.completed","item":{"type":"image_generation_call"},"text":"PRIVATE_SYNTHETIC_CONTENT"}\n'
            child.write_text("import sys\nsys.stdout.buffer.write(" + repr(payload) + ")\n")
            target = root / "audit.json"
            env = dict(os.environ, XCLI_NATIVE_AUDIT=str(target))
            result = subprocess.run([sys.executable, str(Path(native_audit.__file__)), sys.executable, str(child)], capture_output=True, env=env)
            self.assertEqual(result.returncode, 0)
            self.assertEqual(result.stdout, payload)
            audit = json.loads(target.read_text())
            self.assertEqual(audit["stdout_sha256"], hashlib.sha256(payload).hexdigest())
            self.assertEqual(audit["image_tool_events"], 1)
            self.assertNotIn("PRIVATE_SYNTHETIC_CONTENT", target.read_text())

    def test_real_format_and_symlink_rejection(self):
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            path = root / "misnamed.jpg"
            Image.new("RGB", (24, 32), "blue").save(path, format="PNG")
            info = native_media.image_info(path)
            self.assertEqual(info["mime"], "image/png")
            self.assertEqual((info["width"], info["height"]), (24, 32))
            link = root / "link.png"
            link.symlink_to(path)
            self.assertFalse(native_media.allowed_image(link, root))
            with self.assertRaises(ValueError):
                native_media.image_info(link)


if __name__ == "__main__":
    unittest.main()
