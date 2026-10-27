#!/usr/bin/env python3
"""Snapshot desktop source for isolated integration work and prepare guarded patches.

Snapshot and diff never write the original repository or copy databases, credentials,
untracked media, node_modules or build output. Applying a prepared patch is a
separate, explicit action that rechecks every changed source file's digest.
"""
from __future__ import annotations

import argparse
import ast
import difflib
import hashlib
import json
from pathlib import Path
import shutil
import subprocess


REPOSITORIES = {"worklog", "chat-insight", "garden-flow", "ai-open-source-contribution-os-summary"}
EXCLUDED = {".git", "node_modules", "target", "dist", "dist-electron", "gen", ".tmp", ".vite-temp", ".venv", "venv", "__pycache__", ".pytest_cache", ".ruff_cache", ".coverage", "htmlcov", ".DS_Store", ".aws", ".codex", ".local-development"}
PRIVATE_SUFFIXES = {".db", ".sqlite", ".sqlite3", ".pem", ".key", ".p12", ".token"}
SOURCE_ROOTS = {
    "worklog": ("app/", "tests/", "frontend/src/", "src-tauri/src/"),
    "chat-insight": ("app/", "tests/", "src/", "src-tauri/src/"),
    "garden-flow": ("desktop/src/", "desktop/shared/", "desktop/electron/", "desktop/tests/", "Plugin/src/"),
    "ai-open-source-contribution-os-summary": ("app/", "tests/", "scripts/"),
}


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def permitted(name: str) -> bool:
    path = Path(name)
    return (bool(name) and name not in {".", ".."} and not path.is_absolute() and ".." not in path.parts
            and not EXCLUDED.intersection(path.parts)
            and not any(part.startswith(".env") for part in path.parts)
            and path.suffix.lower() not in PRIVATE_SUFFIXES)


def git(root: Path, *args: str) -> bytes:
    return subprocess.check_output(["git", "-C", str(root), *args])


def validate_python_runtime(root: Path) -> None:
    """Inspect internal imports without executing the copied application's code."""
    package = root / "app"
    missing = set()
    if not package.is_dir():
        return
    for source in package.rglob("*.py"):
        if source.is_symlink() or not source.resolve().is_relative_to(package.resolve()):
            raise ValueError("runtime source symlink escapes the snapshot")
        tree = ast.parse(source.read_text(), filename=str(source.relative_to(root)))
        for node in ast.walk(tree):
            names = ([node.module] if isinstance(node, ast.ImportFrom) and not node.level
                     else [alias.name for alias in node.names] if isinstance(node, ast.Import) else [])
            for name in names:
                if name and name.startswith("app."):
                    target = package.joinpath(*name.split(".")[1:])
                    if not target.with_suffix(".py").is_file() and not (target / "__init__.py").is_file():
                        missing.add((source.relative_to(package).as_posix(), name))
    if missing:
        raise ValueError("snapshot runtime imports missing: " + repr(sorted(missing)[:20]))


def snapshot(root: Path, destination: Path) -> None:
    root = root.resolve()
    if root.name not in REPOSITORIES:
        raise ValueError("snapshot only supports the four named integration repositories")
    destination.mkdir(mode=0o700, parents=True, exist_ok=False)
    names = git(root, "ls-files", "-z").decode().split("\0")
    # Active source modules may depend on untracked user code. Copy only source
    # extensions under known source roots, excluding unrelated user artifacts.
    untracked = git(root, "ls-files", "--others", "--exclude-standard", "-z").decode().split("\0")
    names += [name for name in untracked if name.startswith(SOURCE_ROOTS[root.name])
              and Path(name).suffix in {".py", ".js", ".ts", ".tsx", ".jsx", ".css", ".rs"}]
    files = {}
    for name in sorted(set(names)):
        if not name or not permitted(name):
            continue
        source = root / name
        if not source.parent.resolve().is_relative_to(root) or source.is_symlink() or not source.is_file():
            continue
        contents = source.read_bytes()
        for tree in ("base", "work"):
            target = destination / tree / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(contents)
            shutil.copystat(source, target)
        files[name] = hashlib.sha256(contents).hexdigest()
    if root.name == "ai-open-source-contribution-os-summary":
        validate_python_runtime(destination / "work")
    manifest = {"repository": str(root), "head": git(root, "rev-parse", "HEAD").decode().strip(),
                "status": git(root, "status", "--short").decode(), "files": files}
    (destination / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps({"repository": root.name, "files": len(files), "head": manifest["head"]}))


def prepare(destination: Path, output: Path) -> None:
    manifest = json.loads((destination / "manifest.json").read_text())
    root = Path(manifest["repository"])
    base_root, work_root = (destination / "base").resolve(), (destination / "work").resolve()
    names = set(manifest["files"])
    names.update(str(p.relative_to(destination / "work")) for p in (destination / "work").rglob("*")
                 if p.is_file() and not p.is_symlink() and permitted(str(p.relative_to(destination / "work"))))
    chunks = []
    changed = {}
    for name in sorted(names):
        if not permitted(name):
            continue
        before_path, after_path = destination / "base" / name, destination / "work" / name
        if (not before_path.parent.resolve().is_relative_to(base_root)
                or not after_path.parent.resolve().is_relative_to(work_root)
                or before_path.is_symlink()):
            raise ValueError("source parent symlink escapes the integration snapshot")
        if not after_path.is_file() or after_path.is_symlink():
            continue  # File removals are not part of this integration workflow.
        before = before_path.read_bytes() if before_path.exists() else b""
        if name in manifest["files"] and hashlib.sha256(before).hexdigest() != manifest["files"][name]:
            raise ValueError("snapshot baseline changed; recreate the integration workspace")
        after = after_path.read_bytes()
        if before == after:
            continue
        try:
            old, new = before.decode(), after.decode()
        except UnicodeDecodeError as error:
            raise ValueError("binary source mutations are not supported") from error
        if not after.endswith(b"\n"):
            raise ValueError("integration text files must end with a newline")
        chunks.extend(difflib.unified_diff(old.splitlines(keepends=True), new.splitlines(keepends=True),
                                         fromfile="a/" + name if before_path.exists() else "/dev/null", tofile="b/" + name))
        changed[name] = manifest["files"].get(name)
    if root.name == "ai-open-source-contribution-os-summary":
        validate_python_runtime(destination / "work")
    if not changed:
        raise ValueError("there are no integration source changes")
    output.parent.mkdir(parents=True, exist_ok=True)
    patch = "".join(chunks).encode()
    output.write_bytes(patch)
    report = {"repository": root.name, "source_head": manifest["head"], "patch_sha256": digest(output), "preimages": changed}
    output.with_suffix(output.suffix + ".json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"repository": root.name, "changed_files": len(changed), "patch_sha256": report["patch_sha256"]}))


def apply(output: Path, root: Path) -> None:
    report = json.loads(output.with_suffix(output.suffix + ".json").read_text())
    root = root.resolve()
    if root.name not in REPOSITORIES or root.name != report["repository"] or digest(output) != report["patch_sha256"]:
        raise ValueError("invalid prepared patch or repository")
    if git(root, "rev-parse", "HEAD").decode().strip() != report["source_head"]:
        raise ValueError("source HEAD changed; rebuild and review the integration patch")
    # The digest pairs the files, but does not prove that the patch only edits
    # files listed in the report. Check the parsed patch before applying it.
    paths = []
    for record in git(root, "apply", "--numstat", "-z", str(output.resolve())).split(b"\0"):
        if not record:
            continue
        fields = record.split(b"\t", 2)
        if len(fields) != 3 or not fields[0].isdigit() or not fields[1].isdigit():
            raise ValueError("only ordinary text patches are supported")
        paths.append(fields[2].decode())
    if not paths or len(paths) != len(set(paths)) or set(paths) != set(report["preimages"]):
        raise ValueError("patch source paths do not match the prepared report")
    for name, expected in report["preimages"].items():
        if not permitted(name):
            raise ValueError("unsafe source path")
        path = root / name
        if not path.parent.resolve().is_relative_to(root):
            raise ValueError("source parent symlink escapes the repository")
        if path.is_symlink() or (expected is None and path.exists()) or (expected is not None and (not path.is_file() or digest(path) != expected)):
            raise ValueError("source changed after snapshot; refusing to overwrite existing work")
    subprocess.run(["git", "-C", str(root), "apply", "--check", str(output.resolve())], check=True)
    subprocess.run(["git", "-C", str(root), "apply", str(output.resolve())], check=True)
    print(json.dumps({"repository": root.name, "applied_files": len(report["preimages"]), "commit": False, "publish": False}))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    snap = sub.add_parser("snapshot")
    snap.add_argument("--repository", type=Path, required=True)
    snap.add_argument("--destination", type=Path, required=True)
    diff = sub.add_parser("prepare")
    diff.add_argument("--destination", type=Path, required=True)
    diff.add_argument("--output", type=Path, required=True)
    patch = sub.add_parser("apply")
    patch.add_argument("--patch", type=Path, required=True)
    patch.add_argument("--repository", type=Path, required=True)
    args = parser.parse_args()
    if args.command == "snapshot":
        snapshot(args.repository, args.destination)
    elif args.command == "prepare":
        prepare(args.destination, args.output)
    else:
        apply(args.patch, args.repository)


if __name__ == "__main__":
    main()
