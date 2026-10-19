import unittest

from evidence import redact


class EvidenceTests(unittest.TestCase):
    def test_paths_and_private_payloads_do_not_enter_public_reports(self):
        source = {"kind": "r3_comparison", "results": [{"error": "/private/study/file unavailable", "session_id": "secret",
                    "native_calls": [{"usage": None, "session_present": True}], "stdout": "raw source", "stderr": "raw diagnostics"}]}
        public = redact(source, [("/private/study", "<private-study>")])
        self.assertEqual(public["results"][0]["error"], "<private-study>/file unavailable")
        self.assertEqual(set(public["results"][0]), {"error", "native_calls"})
        self.assertEqual(source["results"][0]["session_id"], "secret")


if __name__ == "__main__":
    unittest.main()
