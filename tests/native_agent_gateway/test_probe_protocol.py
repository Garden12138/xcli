"""No native process: assert the entire permitted probe request surface."""
import json
import unittest
from probe_protocol import plan, shape


class NonInferenceProbeTest(unittest.TestCase):
    def test_only_initialize_and_stdio(self):
        for agent in ("codex", "cursor", "antigravity"):
            argv, request = plan(agent)
            self.assertEqual(request["method"], "initialize")
            self.assertEqual(argv, ["app-server", "--listen", "stdio://"] if agent == "codex" else ["acp"])
            self.assertNotIn("prompt", json.dumps(request))
            self.assertNotIn("token", json.dumps(request))
        with self.assertRaises(ValueError):
            plan("unregistered")

    def test_native_values_are_never_retained(self):
        result = shape({"email": "secret@example.invalid", "token": "sensitive", "tools": ["private"]})
        self.assertNotIn("secret", json.dumps(result))
        self.assertNotIn("sensitive", json.dumps(result))
        self.assertNotIn("private", json.dumps(result))
