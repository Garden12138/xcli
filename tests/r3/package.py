#!/usr/bin/env python3
"""Inspect actual GoReleaser snapshot archives and install the native artifact.

Does not publish, invoke models, install native CLIs, or alter login settings.
Run goreleaser release --snapshot --clean first; --dist consumes its artifacts.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import tarfile
import tempfile

from common import digest, process, write_json


def inspect(dist, native_copy=None):
    sums = dist / "checksums.txt"
    checksums = {}
    for line in sums.read_text().splitlines():
        value, name = line.split(maxsplit=1)
        checksums[name.lstrip("*")] = value
    report = {"kind": "r3_snapshot_installation", "publishes": False, "targets": [],
              "checksums_sha256": digest(sums), "goreleaser_artifacts_sha256": digest(dist / "artifacts.json")}
    native_os = "darwin" if platform.system() == "Darwin" else "linux"
    native_arch = "arm64" if platform.machine() in ("arm64", "aarch64") else "amd64"
    for system in ("darwin", "linux"):
        for arch in ("amd64", "arm64"):
            archives = list(dist.glob(f"xcli_*_{system}_{arch}.tar.gz"))
            if len(archives) != 1:
                raise ValueError("expected exactly one archive per target")
            archive = archives[0]
            if digest(archive) != checksums[archive.name]:
                raise ValueError("archive checksum mismatch")
            with tarfile.open(archive) as stream:
                entries = stream.getmembers()
                if sorted(m.name for m in entries) != ["LICENSE", "README.md", "README.zh-CN.md", "xcli"]:
                    raise ValueError("unexpected archive contents")
                if not all(m.isfile() for m in entries):
                    raise ValueError("archive contains nonregular files")
                member = stream.getmember("xcli")
                if not member.mode & 0o111:
                    raise ValueError("binary lacks executable permission")
                binary_bytes = stream.extractfile(member).read()
                target = {"os": system, "arch": arch, "archive": archive.name, "archive_sha256": digest(archive),
                          "binary_sha256": hashlib.sha256(binary_bytes).hexdigest(), "archive_check": "passed",
                          "native_execution": "not_verified"}
                if (system, arch) == (native_os, native_arch):
                    with tempfile.TemporaryDirectory(prefix="xcli-r3-install-") as temporary:
                        root = Path(temporary)
                        bindir = root / "bin"
                        bindir.mkdir()
                        executable = bindir / "xcli"
                        executable.write_bytes(binary_bytes)
                        executable.chmod(0o700)
                        env = dict(os.environ, PATH=str(bindir) + os.pathsep + os.environ["PATH"],
                                   XDG_CONFIG_HOME=str(root / "config"), XDG_DATA_HOME=str(root / "data"))
                        calls = {}
                        for label, argv in (("version", ["xcli", "--version"]), ("help", ["xcli", "task", "--help"]),
                                            ("config", ["xcli", "config", "init"]), ("validate", ["xcli", "config", "validate"]),
                                            ("doctor", ["xcli", "doctor", "codex", "--json"])):
                            result = process(argv, cwd=root, env=env, timeout=15)
                            calls[label] = {"exit_code": result["exit_code"]}
                            if label == "version":
                                target["native_version"] = result["stdout"].strip()
                            if label == "doctor":
                                # Read-only installation/help detection is not an
                                # account or model execution claim.
                                calls[label]["observed"] = json.loads(result["stdout"])["agents"]["codex"]["installed"]
                        if any(c["exit_code"] != (1 if name == "doctor" and not c.get("observed") else 0)
                               for name, c in calls.items()):
                            raise ValueError("native clean-XDG installation check failed")
                        target["installation_checks"] = calls
                        target["native_execution"] = "passed"
                        if native_copy:
                            if native_copy.exists():
                                raise ValueError("native candidate destination exists")
                            shutil.copyfile(executable, native_copy)
                            native_copy.chmod(0o700)
                report["targets"].append(target)
    report["status"] = "passed"
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dist", type=Path, default=Path("dist"))
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--native-copy", type=Path)
    args = parser.parse_args()
    if args.out.exists():
        parser.error("fresh --out required")
    os.umask(0o077)
    report = inspect(args.dist, args.native_copy)
    write_json(args.out, report)
    print(json.dumps(report))


if __name__ == "__main__":
    main()
