#!/usr/bin/env python3
"""Record final source identity and required engineering checks; no native calls."""
import argparse
import os
from pathlib import Path
import subprocess
import time

from common import digest, process, write_json


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, type=Path)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    if args.out.exists():
        parser.error("fresh --out required")
    os.umask(0o077)
    repository = Path(__file__).resolve().parents[2]
    files = subprocess.check_output(["rg", "--files", "--hidden", "-g", "!.git", "-g", "!dist", "-g", "!xcli", "-g", "!__pycache__", "-g", "!docs/r3/evidence/**"], cwd=repository, text=True).splitlines()
    files = [name for name in files if name.endswith((".go", ".py")) or name in ("go.mod", "go.sum", ".goreleaser.yaml") or name.startswith(".github/workflows/")]
    source = {name: digest(repository / name) for name in sorted(files) if (repository / name).is_file()}
    report = {"kind": "r3_engineering_validation", "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
              "base_commit": subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=repository, text=True).strip(),
              "worktree_dirty": bool(subprocess.check_output(["git", "status", "--porcelain"], cwd=repository)),
              "go_version": subprocess.check_output(["go", "version"], text=True).strip(),
              "binary_sha256": digest(args.binary), "source_files": source, "checks": [], "status": "running"}
    checks = [("format", ["gofmt", "-l", *[name for name in files if name.endswith(".go")]]),
              ("modules", ["go", "mod", "tidy", "-diff"]), ("vet", ["go", "vet", "./..."]),
              ("race", ["go", "test", "-race", "./..."]),
              ("r3_simulation", ["python3", "-m", "unittest", "discover", "-s", "tests/r3", "-v"])]
    for name, argv in checks:
        result = process(argv, cwd=repository, env=dict(os.environ, XCLI_R3_BINARY=str(args.binary.resolve())), timeout=600)
        passed = result["exit_code"] == 0 and (name != "format" or not result["stdout"].strip())
        entry = {"check": name, "status": "passed" if passed else "failed", "seconds": result["seconds"], "exit_code": result["exit_code"]}
        if not passed:
            entry["diagnostics"] = (result["stdout"] + result["stderr"])[-6000:]
        report["checks"].append(entry)
        report["status"] = ("failed" if any(c["status"] == "failed" for c in report["checks"])
                            else "passed" if len(report["checks"]) == len(checks) else "running")
        write_json(args.out, report)
        print(name + ": " + entry["status"], flush=True)
    if any(digest(repository / name) != expected for name, expected in source.items()):
        report["status"] = "failed"
        report["source_changed_during_validation"] = True
        write_json(args.out, report)
    raise SystemExit(0 if report["status"] == "passed" else 1)


if __name__ == "__main__":
    main()
