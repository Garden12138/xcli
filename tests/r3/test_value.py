import copy
import json
from pathlib import Path
import tempfile
import unittest

import observe
import value
import value_fixtures


class ValueTests(unittest.TestCase):
    def test_independent_score_rejects_omissions_wrong_decisions_and_unbacked_citations(self):
        with tempfile.TemporaryDirectory() as temporary:
            folder = Path(temporary)
            reference = value_fixtures.reference_report()
            markdown = "\n".join(row["component"] + ": " + row["decision"] for row in reference["targets"])
            (folder / "report.md").write_text(markdown + "\n" + " ".join(reference["sources"]))
            (folder / "report.json").write_text(json.dumps(reference))
            self.assertTrue(value_fixtures.score(folder)["passed"])
            for mutate in (lambda d: d["findings"].pop(),
                           lambda d: d["findings"][0].update(record_ids=["invented-record"]),
                           lambda d: d["targets"][0].update(decision="blocked"),
                           lambda d: d["findings"].append(copy.deepcopy(d["findings"][0]))):
                changed = copy.deepcopy(reference)
                mutate(changed)
                (folder / "report.json").write_text(json.dumps(changed))
                self.assertFalse(value_fixtures.score(folder)["passed"], changed)

    def test_infrastructure_failure_never_becomes_quality_benefit(self):
        cross = {"arm": "cross", "status": "passed", "runtime_status": "completed",
                 "score": {"passed": True, "findings": ["a", "b"]}}
        base = {"arm": "direct", "status": "failed", "runtime_status": "failed", "inputs_unchanged": True,
                "score": {"passed": False, "findings": []}}
        single = {**copy.deepcopy(base), "arm": "single", "runtime_status": "completed"}
        self.assertFalse(value.assess([cross, base, single])["benefit_established_in_this_sample"])
        base["runtime_status"] = "completed"
        self.assertTrue(value.assess([cross, base, single])["benefit_established_in_this_sample"])
        base.update(status="passed", score={"passed": True, "findings": ["a", "b"]})
        self.assertFalse(value.assess([cross, base, single])["benefit_established_in_this_sample"])


class ObservationTests(unittest.TestCase):
    def test_filled_forms_still_require_manual_independence_review(self):
        with tempfile.TemporaryDirectory() as temporary:
            kit = Path(temporary)
            observe.write(kit / "candidate-manifest.json", {"targets": [{"os": "darwin", "arch": "arm64", "binary_sha256": "simulation"}]})
            for identifier in "ABC":
                record = {k: "simulated observation" if v is None else v for k, v in observe.template(identifier).items()}
                record.update(participant_attested=True, platform="darwin/arm64", binary_sha256="simulation",
                              cli_versions={"codex": "simulation"}, day1_date="2026-10-19", day2_date="2026-10-20",
                              installation_minutes=3, first_success_minutes=5, help_count=0, observed_days=2,
                              evidence_refs=["simulated evidence"], publication_consent=False)
                observe.write(kit / (identifier + ".json"), record)
            result = observe.summarize(kit)
            self.assertEqual(result["complete_records"], 3)
            self.assertEqual(result["status"], "needs_manual_review")
            self.assertFalse(result["independent_observation_verified"])

    def test_blank_kit_is_not_evidence_and_tampered_candidates_are_rejected(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            dist = root / "dist"
            dist.mkdir()
            report = {"status": "passed", "targets": []}
            for system in ("darwin", "linux"):
                for arch in ("amd64", "arm64"):
                    path = dist / (system + "-" + arch + ".tar.gz")
                    path.write_bytes(b"simulated archive, not executable")
                    report["targets"].append({"os": system, "arch": arch, "archive": path.name,
                                              "archive_sha256": observe.sha(path), "binary_sha256": "simulation"})
            (dist / "checksums.txt").write_text("simulation\n")
            report["checksums_sha256"] = observe.sha(dist / "checksums.txt")
            installation = root / "installation.json"
            observe.write(installation, report)
            kit = root / "kit"
            self.assertEqual(observe.prepare(kit, dist, installation)["participants"], 0)
            summary = observe.summarize(kit)
            self.assertEqual(summary["complete_records"], 0)
            self.assertFalse(summary["independent_observation_verified"])
            (dist / report["targets"][0]["archive"]).write_bytes(b"tampered")
            with self.assertRaisesRegex(ValueError, "checksum"):
                observe.prepare(root / "tampered", dist, installation)


if __name__ == "__main__":
    unittest.main()
