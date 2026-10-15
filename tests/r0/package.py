#!/usr/bin/env python3
"""Build and inspect temporary release-like archives without publishing."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    destination = Path(args.out)
    if destination.exists():
        parser.error("evidence path already exists")
    report = {"kind": "cross_build_and_archive", "publishes": False, "targets": []}
    with tempfile.TemporaryDirectory(prefix="xcli-r0-package-") as temporary:
        root = Path(temporary)
        for target_os in ("darwin", "linux"):
            for target_arch in ("amd64", "arm64"):
                environment = dict(os.environ, GOOS=target_os, GOARCH=target_arch, CGO_ENABLED="0")
                folder = root / (target_os + "_" + target_arch)
                folder.mkdir()
                binary = folder / "xcli"
                subprocess.run(["go", "build", "-trimpath", "-o", str(binary), "."], env=environment, check=True)
                archive = root / (folder.name + ".tar.gz")
                with tarfile.open(archive, "w:gz") as stream:
                    stream.add(binary, arcname="xcli")
                    for document in ("LICENSE", "README.md", "README.zh-CN.md"):
                        stream.add(document, arcname=document)
                checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
                checksum_tool = shutil.which("shasum") or shutil.which("sha256sum")
                if not checksum_tool:
                    raise RuntimeError("checksum verifier is unavailable")
                check_argv = [checksum_tool, "-a", "256", "--check"] if Path(checksum_tool).name == "shasum" else [checksum_tool, "--check"]
                subprocess.run(check_argv, input=checksum + "  " + str(archive) + "\n", text=True, check=True, capture_output=True)
                with tarfile.open(archive) as stream:
                    members = stream.getnames()
                    if sorted(members) != sorted(["xcli", "LICENSE", "README.md", "README.zh-CN.md"]):
                        raise RuntimeError("archive contents mismatch")
                    archived_binary = stream.extractfile("xcli").read()
                    if archived_binary != binary.read_bytes() or stream.getmember("xcli").mode & 0o111 == 0:
                        raise RuntimeError("archive binary mismatch or not executable")
                entry = {"os": target_os, "arch": target_arch, "status": "passed", "archive_sha256": checksum,
                         "binary_sha256": hashlib.sha256(archived_binary).hexdigest(), "members": members}
                native_os = "darwin" if platform.system() == "Darwin" else "linux"
                native_arch = "arm64" if platform.machine() in ("arm64", "aarch64") else "amd64"
                if (target_os, target_arch) == (native_os, native_arch):
                    result = subprocess.run([str(binary), "--version"], text=True, capture_output=True, check=True)
                    if not result.stdout.startswith("xcli version "):
                        raise RuntimeError("unexpected native version output")
                    entry["native_version"] = result.stdout.strip()
                report["targets"].append(entry)
                print(json.dumps(entry), flush=True)
        report["status"] = "passed"
        report["goreleaser"] = "not run locally; CI uses the pinned snapshot workflow"
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(json.dumps(report, indent=2) + "\n")


if __name__ == "__main__":
    main()
