#!/usr/bin/env python3
"""Study-owned native launcher. No credentials or raw argv enter the ledger."""
import os
from pathlib import Path
import sys

from common import Ledger, digest, read_json


def main():
    root = Path(__file__).resolve().parent.parent
    study = read_json(root / "study.json")
    adapter = os.environ["XCLI_R3_ADAPTER"]
    native = study["native"][adapter]
    probe = sys.argv[1:] and sys.argv[-1] in ("--help", "--version", "-h", "-v")
    if not probe:
        if digest(native["command"]) != native["sha256"]:
            raise ValueError("native executable changed since study preparation")
        version = __import__("subprocess").check_output(
            [native["command"], "--version"], text=True, stderr=__import__("subprocess").DEVNULL).strip()
        if version != native["version"]:
            raise ValueError("native version changed since study preparation")
        Ledger(root).reserve(adapter, os.environ["XCLI_R3_CASE"], os.environ.get("XCLI_R3_PHASE", "task"))
    os.execv(native["command"], [native["command"], *sys.argv[1:]])


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(str(error), file=sys.stderr)
        sys.exit(90)
