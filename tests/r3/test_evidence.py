import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

from evidence import main, redact


class EvidenceTests(unittest.TestCase):
    def test_paths_and_private_payloads_do_not_enter_public_reports(self):
        source = {"kind": "r3_comparison", "results": [{"error": "/private/study/file unavailable", "session_id": "secret",
                    "native_calls": [{"usage": None, "session_present": True}], "stdout": "raw source", "stderr": "raw diagnostics"}]}
        public = redact(source, [("/private/study", "<private-study>")])
        self.assertEqual(public["results"][0]["error"], "<private-study>/file unavailable")
        self.assertEqual(set(public["results"][0]), {"error", "native_calls"})
        self.assertEqual(source["results"][0]["session_id"], "secret")

    def test_persona_summary_redacts_symlink_and_resolved_study_paths(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            study = root / "study"
            study.mkdir()
            alias = root / "alias"
            alias.symlink_to(study, target_is_directory=True)
            report, out = root / "report.json", root / "public.json"
            report.write_text(json.dumps({"kind": "r3_persona_review", "status": "completed",
                "paths": [str(alias / "feedback.json"), str(study.resolve() / "feedback.json")],
                "roles": [{"real_human": False, "session_id": "private", "stdout": "raw"}]}))
            with patch.object(sys, "argv", ["evidence.py", "--study", str(alias), "--report", str(report), "--out", str(out)]):
                main()
            public = json.loads(out.read_text())
            self.assertEqual(public["paths"], ["<private-study>/feedback.json"] * 2)
            self.assertEqual(public["roles"], [{"real_human": False}])


if __name__ == "__main__":
    unittest.main()
