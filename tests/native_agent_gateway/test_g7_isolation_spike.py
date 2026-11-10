import contextlib
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("g7_spike", Path(__file__).with_name("g7_isolation_spike.py"))
spike = importlib.util.module_from_spec(spec)
spec.loader.exec_module(spike)


class G7SpikeSafetyTests(unittest.TestCase):
    def test_dry_run_never_loads_assets_or_starts_process(self):
        with tempfile.TemporaryDirectory() as root:
            output = Path(root) / "result.json"
            with patch("sys.argv", ["g7", "--metadata", "/does-not-exist", "--socket", "not-a-socket", "--out", str(output)]), \
                 patch.object(spike.subprocess, "Popen", side_effect=AssertionError("process started")), \
                 patch.object(spike.subprocess, "run", side_effect=AssertionError("control started")):
                spike.main()
            report = json.loads(output.read_text())
            self.assertEqual(report["result"], "DRY_RUN")
            self.assertEqual(report["sent_methods"], [])
            self.assertEqual(report["real_model_requests"], 0)

    def test_no_task_auth_or_command_rpc(self):
        for method in ("turn/start", "turn/interrupt", "thread/start", "thread/resume", "account/read",
                       "account/login/start", "config/write", "command/exec", "fs/writeFile"):
            with self.subTest(method=method), self.assertRaises(ValueError):
                spike.rpc_message(method, 1, {})
        self.assertEqual(json.loads(spike.rpc_message("initialize", 1, {}))["method"], "initialize")

    def test_plan_is_pinned_readonly_and_has_no_host_credentials(self):
        with tempfile.TemporaryDirectory() as root:
            ws = Path(root).resolve()
            (ws / "material.txt").write_bytes(spike.MATERIAL)
            (ws / "codex").write_bytes(b"synthetic-executable")
            meta = {"workspace": str(ws), "image": "sha256:" + "a" * 64,
                    "linux_binaries": {"codex": {"sha256": hashlib.sha256(b"synthetic-executable").hexdigest()}}}
            args = spike.docker_args(meta, "test-only")
            for item in ("--network=none", "--read-only", "--user=65532:65532", "--cap-drop=ALL",
                         "--security-opt=no-new-privileges", "--pull=never"):
                self.assertIn(item, args)
            self.assertEqual(args.count("--mount"), 1)
            self.assertIn("type=bind,src=" + str(ws) + ",dst=/workspace,readonly", args)
            self.assertNotIn("--privileged", args)
            self.assertNotIn("--pid=host", args)
            meta["image"] = "alpine:latest"
            with self.assertRaises(ValueError): spike.docker_args(meta, "test-only")
