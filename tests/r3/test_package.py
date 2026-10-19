import hashlib
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

import package


class ArchiveTests(unittest.TestCase):
    def build(self, root, unsafe=False):
        sums = []
        for system in ("darwin", "linux"):
            for arch in ("amd64", "arm64"):
                archive = root / f"xcli_test_{system}_{arch}.tar.gz"
                with tarfile.open(archive, "w:gz") as stream:
                    for name in ("LICENSE", "README.md", "README.zh-CN.md", "xcli"):
                        member = tarfile.TarInfo(name)
                        member.mode = 0o755 if name == "xcli" else 0o644
                        data = b"fixture"
                        member.size = len(data)
                        if unsafe and name == "xcli":
                            member.type = tarfile.SYMTYPE
                            member.linkname = "/outside"
                            member.size = 0
                        stream.addfile(member, io.BytesIO(data))
                sums.append(hashlib.sha256(archive.read_bytes()).hexdigest() + "  " + archive.name)
        (root / "checksums.txt").write_text("\n".join(sums) + "\n")
        (root / "artifacts.json").write_text("[]")

    def test_checksum_and_nonregular_file_rejected_before_execution(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.build(root, unsafe=True)
            with self.assertRaisesRegex(ValueError, "nonregular"):
                package.inspect(root)
            self.build(root)
            (root / "xcli_test_darwin_amd64.tar.gz").write_bytes(b"tampered")
            with self.assertRaisesRegex(ValueError, "checksum"):
                package.inspect(root)

    def test_clean_ci_without_native_agent_reports_prerequisite(self):
        def fake_process(argv, **kwargs):
            if argv[1] == "doctor":
                return {"exit_code": 1, "stdout": json.dumps({"agents": {"codex": {"installed": False}}})}
            return {"exit_code": 0, "stdout": "xcli version fixture"}
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.build(root)
            with patch.object(package, "process", fake_process):
                result = package.inspect(root)
            self.assertEqual(result["status"], "passed")
            native = next(t for t in result["targets"] if t["native_execution"] == "passed")
            self.assertFalse(native["installation_checks"]["doctor"]["observed"])


if __name__ == "__main__":
    unittest.main()
