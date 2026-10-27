import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("integration_workspace", Path(__file__).with_name("integration_workspace.py"))
workspace = importlib.util.module_from_spec(spec)
spec.loader.exec_module(workspace)


class IntegrationWorkspaceTests(unittest.TestCase):
    def fixture(self, root):
        repository = root / "worklog"
        repository.mkdir()
        (repository / "app").mkdir()
        (repository / "app/source.py").write_text("baseline = True\n")
        (repository / ".env").write_text("PRIVATE_CANARY=never-copy\n")
        for arguments in (["init", "-q"], ["add", "app/source.py", ".env"], ["-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-qm", "fixture"]):
            subprocess.run(["git", "-C", str(repository), *arguments], check=True)
        # An existing user edit and untracked source dependency must survive.
        (repository / "app/source.py").write_text("baseline = True\nuser_edit = True\n")
        (repository / "app/active.py").write_text("active_user_module = True\n")
        (repository / "private.sqlite").write_text("private data")
        snapshot = root / "snapshot"
        workspace.snapshot(repository, snapshot)
        return repository, snapshot

    def test_patch_preserves_dirty_source_and_excludes_secrets_and_generated_output(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            repository, snapshot = self.fixture(root)
            self.assertFalse((snapshot / "work/.env").exists())
            self.assertFalse((snapshot / "work/private.sqlite").exists())
            self.assertTrue((snapshot / "work/app/active.py").exists())
            (snapshot / "work/app/source.py").write_text("baseline = True\nuser_edit = True\ngateway = True\n")
            generated = snapshot / "work/src-tauri/gen/metadata.json"
            generated.parent.mkdir(parents=True)
            generated.write_text("not for a patch")
            patch = root / "integration.patch"
            workspace.prepare(snapshot, patch)
            report = json.loads(patch.with_suffix(".patch.json").read_text())
            self.assertEqual(set(report["preimages"]), {"app/source.py"})
            workspace.apply(patch, repository)
            self.assertEqual((repository / "app/source.py").read_text(), "baseline = True\nuser_edit = True\ngateway = True\n")
            self.assertEqual((repository / "app/active.py").read_text(), "active_user_module = True\n")

    def test_source_drift_and_patch_tampering_cannot_overwrite_user_changes(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            repository, snapshot = self.fixture(root)
            (snapshot / "work/app/source.py").write_text("gateway = True\n")
            patch = root / "integration.patch"
            workspace.prepare(snapshot, patch)
            (repository / "app/source.py").write_text("new_user_edit = True\n")
            with self.assertRaisesRegex(ValueError, "source changed"):
                workspace.apply(patch, repository)
            self.assertEqual((repository / "app/source.py").read_text(), "new_user_edit = True\n")
            patch.write_text(patch.read_text() + "tampered\n")
            with self.assertRaisesRegex(ValueError, "invalid prepared patch"):
                workspace.apply(patch, repository)
            (snapshot / "base/app/source.py").write_text("altered_baseline = True\n")
            with self.assertRaisesRegex(ValueError, "snapshot baseline changed"):
                workspace.prepare(snapshot, root / "altered-baseline.patch")

    def test_report_cannot_omit_a_source_path_in_the_patch(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            repository, snapshot = self.fixture(root)
            (snapshot / "work/app/source.py").write_text("gateway = True\n")
            (snapshot / "work/app/second.py").write_text("new_file = True\n")
            patch = root / "integration.patch"
            workspace.prepare(snapshot, patch)
            report_path = patch.with_suffix(".patch.json")
            report = json.loads(report_path.read_text())
            del report["preimages"]["app/second.py"]
            report_path.write_text(json.dumps(report))
            with self.assertRaisesRegex(ValueError, "patch source paths"):
                workspace.apply(patch, repository)
            self.assertFalse((repository / "app/second.py").exists())
            self.assertIn("user_edit", (repository / "app/source.py").read_text())

    def test_snapshot_and_prepare_do_not_follow_parent_symlinks(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            repository, snapshot = self.fixture(root)
            outside = root / "private"
            outside.mkdir()
            (outside / "source.py").write_text("PRIVATE_CANARY\n")
            (repository / "app/source.py").unlink()
            (repository / "app/active.py").unlink()
            (repository / "app").rmdir()
            (repository / "app").symlink_to(outside, target_is_directory=True)
            fresh = root / "fresh"
            workspace.snapshot(repository, fresh)
            self.assertFalse((fresh / "work/app/source.py").exists())
            (snapshot / "work/app/source.py").unlink()
            (snapshot / "work/app/active.py").unlink()
            (snapshot / "work/app").rmdir()
            (snapshot / "work/app").symlink_to(outside, target_is_directory=True)
            with self.assertRaisesRegex(ValueError, "source parent symlink"):
                workspace.prepare(snapshot, root / "unsafe.patch")

    def test_runtime_inventory_detects_missing_lazy_dependency_before_prepare(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            repository, snapshot = self.fixture(root)
            (snapshot / "work/app/source.py").write_text("def later():\n    from app.active import run\n")
            workspace.validate_python_runtime(snapshot / "work")
            (snapshot / "work/app/active.py").unlink()
            with self.assertRaisesRegex(ValueError, "runtime imports missing"):
                workspace.validate_python_runtime(snapshot / "work")


if __name__ == "__main__":
    unittest.main()
