"""Offline regression checks for the real-smoke harness; never start a CLI."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("r0_smoke", Path(__file__).with_name("smoke.py"))
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)


class SmokeHarnessTests(unittest.TestCase):
    def run_fixture(self, installed, failed=None, reuse_preflight=False):
        launches = []

        def probe(argv, **kwargs):
            if argv[-2:] == ["launches", "budget"]:
                return subprocess.CompletedProcess(argv, 0, '{"remaining":6,"status":"READY"}', "")
            if "launches" in argv and "probe" in argv:
                agent = argv[argv.index("probe")+1]
                if argv[-1] == "version":
                    return subprocess.CompletedProcess(argv, 0, '{"version":"fixture-version"}', "")
                return subprocess.CompletedProcess(argv, 0, '{"authenticated":true}', "")
            if argv[:2] == ["codex", "login"]:
                return subprocess.CompletedProcess(argv, 0, "", "Logged in using ChatGPT")
            if argv[:2] == ["cursor-agent", "status"]:
                return subprocess.CompletedProcess(argv, 0, '{"isAuthenticated":true}', "")
            return subprocess.CompletedProcess(argv, 0, "fixture-version", "")

        class Process:
            def __init__(self, argv, **kwargs):
                launches.append(argv)
                self.tool = argv[argv.index("--agent") + 1]
                self.work = Path(argv[argv.index("--cwd") + 1])
                self.returncode = 1 if self.tool == failed else 0

            def poll(self):
                return None

            def send_signal(self, sig):
                self.returncode = 130

            def communicate(self, **kwargs):
                result = {"status": "canceled" if self.returncode == 130 else "success",
                          "exit_code": self.returncode, "parse_status": "complete",
                          "session_id": "fixture-session",
                          "output": (self.work / "r0-context.txt").read_text().strip()}
                return json.dumps(result), ""

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            binary = root / "xcli"
            binary.write_bytes(b"offline fixture")
            out = root / "report.json"
            argv = ["smoke.py", "--binary", str(binary), "--out", str(out), "--execute"]
            if reuse_preflight:
                preflight = root / "preflight.json"
                preflight.write_text(json.dumps({"cli": {tool: {"version": "fixture-version"} for tool in ["codex", "cursor", "antigravity"]}}))
                argv += ["--preflight", str(preflight)]
                def no_probe(argv, **kwargs):
                    if argv[-2:] == ["launches", "budget"]:
                        return subprocess.CompletedProcess(argv, 0, '{"remaining":6,"status":"READY"}', "")
                    self.assertEqual(argv[0], "git", "reuse mode must not launch version/auth probes")
                    return subprocess.CompletedProcess(argv, 0, "", "")
                probe = no_probe
            with patch.dict(smoke.os.environ, {"XCLI_LAUNCH_STORE":str(root / "ledger"), "XCLI_LAUNCH_WINDOW":"offline"}), \
                 patch.object(sys, "argv", argv), \
                 patch.object(smoke.shutil, "which", side_effect=lambda command: command if command in installed else None), \
                 patch.object(smoke.subprocess, "run", side_effect=probe), \
                 patch.object(smoke.subprocess, "Popen", Process), \
                 patch.object(smoke.time, "sleep"), contextlib.redirect_stdout(io.StringIO()):
                code = smoke.main()
            return code, json.loads(out.read_text()), launches

    def test_one_passing_cli_does_not_pass_three_cli_acceptance(self):
        code, report, launches = self.run_fixture({"codex"})
        self.assertEqual(code, 1)
        self.assertEqual(report["status"], "partial_or_failed")
        self.assertEqual(len(launches), 2)

    def test_three_pairs_use_exactly_six_launches(self):
        code, report, launches = self.run_fixture({"codex", "agy", "cursor-agent"})
        self.assertEqual(code, 0)
        self.assertEqual(report["invocations"], 6)
        self.assertEqual([call["kind"] for call in report["calls"]], ["headless", "resume"] * 3)
        self.assertTrue(all(call["expected_marker_present"] for call in report["calls"]))
        for argv in launches:
            self.assertNotIn("--trust", argv)
            self.assertNotIn("--force", argv)
            self.assertNotIn("--yolo", argv)

    def test_nonzero_exit_with_marker_fails_without_retry(self):
        code, report, launches = self.run_fixture({"codex", "agy", "cursor-agent"}, failed="codex")
        self.assertEqual(code, 1)
        self.assertEqual(report["calls"][0]["status"], "failed")
        self.assertEqual(sum(argv[argv.index("--agent") + 1] == "codex" for argv in launches), 1)
        self.assertLessEqual(len(launches), 6)

    def test_absent_clis_never_pass_or_launch(self):
        code, report, launches = self.run_fixture(set())
        self.assertEqual(code, 1)
        self.assertEqual(report["invocations"], 0)
        self.assertEqual(launches, [])

    def test_reuse_metadata_starts_only_six_model_tasks(self):
        code, report, launches = self.run_fixture({"codex", "agy", "cursor-agent"}, reuse_preflight=True)
        self.assertEqual(code, 0)
        self.assertEqual(len(launches), 6)
        self.assertTrue(all(call["acceptance_status"] == "PASS" for call in report["calls"]))
        self.assertTrue(all(call["session_matches"] for call in report["calls"] if call["kind"] == "resume"))

    def test_native_environment_excludes_api_keys_without_changing_account_root(self):
        result = smoke.native_environment({"OPENAI_API_KEY": "fixture", "CURSOR_API_KEY": "fixture",
                                           "CODEX_HOME": "/fixture/account", "PATH": "/fixture/bin"})
        self.assertEqual(result, {"CODEX_HOME": "/fixture/account", "PATH": "/fixture/bin"})


if __name__ == "__main__":
    unittest.main()
