import concurrent.futures
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from common import Ledger, digest, process, read_json, write_json
import experiment
import fixtures
import compatibility
import clarify


class LedgerTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.study = {"budget": 7, "case_limits": {"a": 6, "b": 6}, "order": ["a", "b"], "compatibility_cases": []}
        write_json(self.root / "study.json", self.study)
        write_json(self.root / "frozen.json", {"study_sha256": digest(self.root / "study.json")})
        (self.root / "calls.jsonl").touch()

    def test_concurrent_reservations_and_restart_cannot_exceed_total(self):
        code = "from common import Ledger; import sys; Ledger(sys.argv[1]).reserve('codex',sys.argv[2],'test')"
        def reserve(index):
            return subprocess.run([sys.executable, "-c", code, str(self.root), "a" if index % 2 else "b"],
                                  cwd=experiment.HERE, capture_output=True).returncode
        with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:
            self.assertEqual(sum(code == 0 for code in pool.map(reserve, range(16))), 7)
        self.assertEqual(len(Ledger(self.root).entries()), 7)
        with self.assertRaisesRegex(ValueError, "total"):
            Ledger(self.root).reserve("codex", "a", "restart")

    def test_corrupt_missing_and_changed_budget_fail_closed(self):
        (self.root / "calls.jsonl").write_text('{"sequence":1}')
        with self.assertRaisesRegex(ValueError, "truncated"):
            Ledger(self.root).reserve("codex", "a", "test")
        (self.root / "calls.jsonl").unlink()
        with self.assertRaisesRegex(ValueError, "missing"):
            Ledger(self.root).reserve("codex", "a", "test")
        (self.root / "calls.jsonl").touch()
        self.study["budget"] = 48
        write_json(self.root / "study.json", self.study)
        with self.assertRaisesRegex(ValueError, "policy changed"):
            Ledger(self.root).reserve("codex", "a", "test")

    def test_per_case_limit_and_new_candidate_share_campaign(self):
        for _ in range(6):
            Ledger(self.root).reserve("codex", "a", "test")
        with self.assertRaisesRegex(ValueError, "case"):
            Ledger(self.root).reserve("codex", "a", "restart")
        candidate = self.root / "candidate"
        candidate.mkdir()
        write_json(candidate / "study.json", {"ledger_root": str(self.root), "binary_sha256": "next"})
        Ledger(candidate).reserve("codex", "a", "fixed-candidate")
        with self.assertRaisesRegex(ValueError, "total"):
            Ledger(candidate).reserve("codex", "b", "test")
        self.assertEqual(Ledger(self.root).entries()[-1]["binary_sha256"], "next")

    def test_comparison_and_compatibility_partitions_cannot_borrow(self):
        self.study = {"budget": 48, "case_limits": {"a": 48, "c": 48}, "order": ["a"], "compatibility_cases": ["c"]}
        write_json(self.root / "study.json", self.study)
        write_json(self.root / "frozen.json", {"study_sha256": digest(self.root / "study.json")})
        ledger = Ledger(self.root)
        for _ in range(12):
            ledger.reserve("codex", "c", "compatibility")
        with self.assertRaisesRegex(ValueError, "phase"):
            ledger.reserve("cursor", "c", "compatibility")
        for _ in range(36):
            ledger.reserve("codex", "a", "comparison")
        self.assertEqual(len(ledger.entries()), 48)
        with self.assertRaisesRegex(ValueError, "total"):
            ledger.reserve("cursor", "a", "comparison")


class ScoringTests(unittest.TestCase):
    def test_work_missing_false_finding_and_wrong_facts(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            self.assertFalse(fixtures.score_work(folder)["passed"])
            (folder / "report.json").write_text(json.dumps({"north": 12, "south": 9, "total": 20, "release_date": "2026-10-25", "pilot_approved": False,
                "sources": list(fixtures.WORK_FILES), "findings": [{"kind": "invented", "evidence": "x", "sources": []}]}))
            (folder / "report.md").write_text("12 9 21 2026-10-25 pending draft.txt revision.txt approval.txt")
            score = fixtures.score_work(folder)
            self.assertFalse(score["passed"])
            self.assertEqual(score["correct_facts"], 4)
            self.assertEqual(score["false_positives"], ["invented"])
            self.assertEqual(len(score["omissions"]), 3)

    def test_project_oracle_rejects_original_and_tampered_tests(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            folder = root / "workspace"
            fixtures.put_files(folder, fixtures.PROJECT_FILES)
            self.assertFalse(fixtures.score_project(folder, root / "original.json")["passed"])
            (folder / "cache.go").write_text("package r3fixture\nfunc Alive(i,n,t int64) bool{return t>0 && n>=i && uint64(n)-uint64(i)<uint64(t)}\n")
            self.assertTrue(fixtures.score_project(folder, root / "correct.json")["passed"])
            (folder / "cache_test.go").write_text("package r3fixture\n")
            self.assertFalse(fixtures.score_project(folder, root / "tampered.json")["passed"])

    def test_timeout_terminates_children_and_preserves_diagnostics(self):
        result = process([sys.executable, "-c", "import subprocess,time; subprocess.Popen(['sleep','30']); time.sleep(30)"], timeout=.1)
        self.assertTrue(result["timed_out"])
        self.assertEqual(result["exit_code"], 124)
        self.assertLess(result["seconds"], 7)


@unittest.skipUnless(os.environ.get("XCLI_R3_BINARY"), "set XCLI_R3_BINARY for process integration")
class IntegrationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="r3-simulation-")
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name)
        commands = self.base / "bin"
        commands.mkdir()
        for name in ("codex", "cursor-agent"):
            target = commands / name
            adapter = "cursor" if "cursor" in name else "codex"
            target.write_text(f"#!{sys.executable}\nimport os,runpy\nos.environ['XCLI_R3_FAKE_ADAPTER']={adapter!r}\nrunpy.run_path({str(experiment.HERE / 'fake_agent.py')!r},run_name='__main__')\n")
            target.chmod(0o700)
        self.patch = patch.dict(os.environ, {"PATH": str(commands) + os.pathsep + os.environ["PATH"], "XCLI_R3_SIMULATION": "1"})
        self.patch.start()
        self.addCleanup(self.patch.stop)
        self.root = self.base / "study"
        self.study = experiment.prepare(self.root, Path(os.environ["XCLI_R3_BINARY"]), 48)

    def test_six_arms_preservation_scoring_and_no_replay(self):
        experiment.validate(self.root)
        for name in experiment.ORDER:
            result = experiment.run_case(self.root, self.study, name)
            self.assertEqual(result["status"], "passed", result)
        before = len(Ledger(self.root).entries())
        for name in experiment.ORDER:
            experiment.run_case(self.root, self.study, name)
        self.assertEqual(len(Ledger(self.root).entries()), before)
        report = experiment.summarize(self.root)
        self.assertEqual(report["status"], "passed")
        self.assertTrue(all(not c["observed_benefits"] for c in report["comparisons"]))
        for name in ("codex-resume", "cursor-resume"):
            self.assertEqual(compatibility.resume(self.root, self.study, name)["status"], "passed")
        for name in ("codex-cancel", "cursor-cancel"):
            self.assertEqual(compatibility.cancel(self.root, self.study, name)["status"], "passed")
        for name in ("single-recovery", "cross-recovery"):
            result = compatibility.recovery(self.root, self.study, name)
            self.assertEqual(result["status"], "passed", result)

    def test_bad_stream_retained_without_retry(self):
        with patch.dict(os.environ, {"XCLI_R3_FAKE_BROKEN": "1"}):
            result = experiment.run_case(self.root, self.study, "work-direct")
        self.assertEqual(result["status"], "failed")
        self.assertEqual(result["calls"], 1)
        self.assertTrue(list((self.root / "private").glob("work-direct-*.json")))
        self.assertEqual(experiment.run_case(self.root, self.study, "work-direct")["calls"], 1)

    def test_interrupted_case_receipt_cannot_be_replayed(self):
        receipt = self.root / "cases/work-direct/result.json"
        write_json(receipt, {"case": "work-direct", "status": "running"})
        result = experiment.run_case(self.root, self.study, "work-direct")
        self.assertEqual(result["status"], "running")
        self.assertEqual(Ledger(self.root).entries(), [])

    def test_assisted_clarification_keeps_initial_failure_and_budget(self):
        with patch.dict(os.environ, {"XCLI_R3_FAKE_BAD_PLAN": "1"}):
            with self.assertRaisesRegex(RuntimeError, "waiting_user"):
                compatibility.recovery(self.root, self.study, "single-recovery")
        destination = self.base / "clarification.json"
        argv = ["clarify.py", "--study", str(self.root), "--out", str(destination), "--execute"]
        with patch.object(sys, "argv", argv):
            clarify.main()
        report = read_json(destination)
        self.assertEqual(report["status"], "passed", report)
        self.assertTrue(report["operator_assisted"])
        self.assertFalse(report["native_worker_interruption_verified"])
        self.assertTrue(report["stop_was_idle_noop"])
        self.assertFalse(report["active_stop_verified"])
        self.assertEqual(report["calls_after"], 4)
        self.assertEqual(len(Ledger(self.root).entries()), 4)


if __name__ == "__main__":
    unittest.main()
