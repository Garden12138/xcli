import hashlib
import json
import unittest
from codex_first_http import execute_gate, one_task, EXPECTED, PROMPT


class FirstCodexHTTPTest(unittest.TestCase):
    def test_execute_requires_separate_authorization(self):
        state = {"state": "READY_FOR_AUTHORIZATION", "enabled": True, "client_ready": True,
                 "call_authorized": True, "authorization_ref_sha256": hashlib.sha256(b"new-ref").hexdigest()}
        self.assertTrue(execute_gate(state, "new-ref"))
        for field in ("enabled", "client_ready", "call_authorized"):
            self.assertFalse(execute_gate({**state, field: False}, "new-ref"))
        self.assertFalse(execute_gate({**state, "state": "BLOCKED"}, "new-ref"))
        self.assertFalse(execute_gate(state, "old-ref"))
        self.assertFalse(execute_gate(state, ""))

    def test_one_post_success_and_failure_never_retry(self):
        for failure in (False, True):
            calls = []
            def request(method, path, body=None, key=None, raw=False):
                calls.append((method, path, body, key))
                if path.endswith("/sessions"):
                    return 201, {"id": "ses", "profile_sha256": "profile", "revision": 1, "context_sha256": "context"}
                if path.endswith("/tasks"):
                    self.assertEqual(body["input"], [{"type": "text", "text": PROMPT}])
                    if failure:
                        raise OSError("ambiguous connection loss")
                    return 202, {"id": "tsk", "status": "succeeded", "profile_sha256": "profile",
                        "execution": {"native_exit_code": 0, "native_terminal": "success", "local_stop_confirmed": True,
                                      "upstream_cancel_confirmed": None},
                        "protocol": {"status": "complete", "native_session_observed": True},
                        "result": {"text": EXPECTED, "final": True},
                        "budget": {"dispatches_consumed": 1, "process_starts_consumed": 1}}
                return 200, "data: " + json.dumps({"seq": 1, "type": "task.completed", "task_id": "tsk"}) + "\n\n"
            caps = {"revision": "cap", "agents": [{"agent_id": "codex", "adapter_id": "codex-exec-v1"}]}
            if failure:
                with self.assertRaises(OSError): one_task(request, caps, "unique-key", lambda _: None)
            else:
                one_task(request, caps, "unique-key", lambda _: None)
            self.assertEqual(sum(method == "POST" and path == "/agent/v1/tasks" for method, path, _, _ in calls), 1)
            self.assertFalse(any("cursor" in path or "antigravity" in path for _, path, _, _ in calls))
