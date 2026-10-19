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
import revalidate


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

    def test_explicit_followup_partition_still_obeys_global_budget(self):
        self.study = {"budget": 15, "case_limits": {"a": 20}, "order": [], "compatibility_cases": ["a"], "limits": {"compatibility_calls": 24}}
        write_json(self.root / "study.json", self.study)
        write_json(self.root / "frozen.json", {"study_sha256": digest(self.root / "study.json")})
        ledger = Ledger(self.root)
        for _ in range(15):
            ledger.reserve("codex", "a", "followup")
        with self.assertRaisesRegex(ValueError, "total"):
            ledger.reserve("codex", "a", "followup")

    def test_carried_ledger_keeps_total_cap_and_rejects_changed_history(self):
        Ledger(self.root).reserve("codex", "a", "prior")
        parent_hash = digest(self.root / "calls.jsonl")
        child = self.root / "carry"
        child.mkdir()
        prior = {"path": str(self.root), "ledger_sha256": parent_hash}
        study = {"budget": 7, "case_limits": {"a": 20}, "order": [], "compatibility_cases": ["a"],
                 "inherited_calls": 1, "previous_campaign": prior, "prior_ledgers": [prior]}
        write_json(child / "study.json", study)
        write_json(child / "frozen.json", {"study_sha256": digest(child / "study.json")})
        (child / "calls.jsonl").write_bytes((self.root / "calls.jsonl").read_bytes())
        ledger = Ledger(child)
        for _ in range(6):
            ledger.reserve("codex", "a", "new")
        with self.assertRaisesRegex(ValueError, "total"):
            ledger.reserve("codex", "a", "over-budget")
        self.assertEqual(digest(self.root / "calls.jsonl"), parent_hash)
        Ledger(self.root).reserve("codex", "b", "history-changed")
        with self.assertRaisesRegex(ValueError, "previous campaign"):
            ledger.reserve("codex", "a", "changed-history")


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

    def test_followup_stops_active_workers_and_keeps_prior_ledger(self):
        followup = self.base / "followup"
        study = experiment.prepare(followup, Path(os.environ["XCLI_R3_BINARY"]), 12, previous_campaign=self.root)
        revalidate.check_study(followup)
        with self.assertRaisesRegex(ValueError, "follow-ups"):
            experiment.summarize(followup)
        ready = revalidate.readiness(followup, study)
        self.assertEqual(ready["status"], "passed", ready)
        for name in ("single-recovery", "cross-recovery"):
            result = revalidate.recovery(followup, study, name)
            self.assertEqual(result["status"], "passed", result)
            self.assertTrue(result["worker_interruption_observed"])
            self.assertTrue(result["fresh_native_session"])
            self.assertEqual(result["operator_clarifications"], 0)
        self.assertEqual(len(Ledger(followup).entries()), 9)
        self.assertEqual(Ledger(self.root).entries(), [])
        revalidate.check_study(followup)
        Ledger(self.root).reserve("codex", "work-direct", "changed-prior")
        with self.assertRaisesRegex(ValueError, "previous campaign"):
            revalidate.check_study(followup)

    def test_followup_quota_failure_skips_cross_without_retry(self):
        followup = self.base / "followup"
        experiment.prepare(followup, Path(os.environ["XCLI_R3_BINARY"]), 12, previous_campaign=self.root)
        output = self.base / "followup.json"
        argv = ["revalidate.py", "run", "--study", str(followup), "--out", str(output), "--execute"]
        with patch.dict(os.environ, {"XCLI_R3_FAKE_CURSOR_QUOTA": "1"}), patch.object(sys, "argv", argv):
            revalidate.main()
        result = read_json(output)
        self.assertEqual(result["status"], "incomplete_or_failed")
        self.assertEqual(result["new_calls"], 5)
        self.assertEqual(result["results"][-1]["status"], "not_verified")
        self.assertTrue(result["prior_ledger_unchanged"])
        self.assertEqual(sum(e["adapter"] == "cursor" for e in Ledger(followup).entries()), 1)

    def test_cross_only_carries_remaining_budget_without_replaying_single(self):
        prior = self.base / "prior"
        experiment.prepare(prior, Path(os.environ["XCLI_R3_BINARY"]), 12, previous_campaign=self.root)
        Ledger(prior).reserve("cursor", "cursor-readiness", "failed-readiness")
        for _ in range(4):
            Ledger(prior).reserve("codex", "single-recovery", "completed-single")
        prior_hash = digest(prior / "calls.jsonl")
        followup = self.base / "carry"
        study = experiment.prepare(followup, Path(os.environ["XCLI_R3_BINARY"]), 12,
                                   previous_campaign=prior, cross_only=True, carry_budget=True)
        self.assertEqual(study["inherited_calls"], 5)
        self.assertEqual(study["compatibility_cases"], ["cursor-readiness", "cross-recovery"])
        self.assertNotIn("single-recovery", study["case_limits"])
        self.assertFalse((followup / "followup/single-recovery").exists())
        output = self.base / "carry.json"
        with patch.object(sys, "argv", ["revalidate.py", "run", "--study", str(followup), "--out", str(output), "--execute"]):
            revalidate.main()
        result = read_json(output)
        self.assertEqual(result["status"], "passed", result)
        self.assertEqual(result["new_budget"], 7)
        self.assertEqual(result["new_calls"], 5)
        self.assertEqual(result["budget_total"], 12)
        self.assertEqual(result["budget_calls"], 10)
        self.assertEqual(result["historical_plus_new_calls"], 10)
        self.assertEqual([r["calls"] for r in result["results"]], [1, 4])
        self.assertEqual(digest(prior / "calls.jsonl"), prior_hash)
        revalidate.check_study(followup)
        data = (followup / "calls.jsonl").read_bytes()
        (followup / "calls.jsonl").write_bytes(b" " + data)
        with self.assertRaisesRegex(ValueError, "inherited ledger prefix"):
            revalidate.check_study(followup)
        changed = read_json(prior / "study.json")
        changed["budget"] = 13
        write_json(prior / "study.json", changed)
        with self.assertRaisesRegex(ValueError, "budget policy changed"):
            experiment.prepare(self.base / "tampered", Path(os.environ["XCLI_R3_BINARY"]), 13,
                               previous_campaign=prior, cross_only=True, carry_budget=True)


if __name__ == "__main__":
    unittest.main()
