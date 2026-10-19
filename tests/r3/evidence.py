#!/usr/bin/env python3
"""Export selected summary reports, with paths redacted. Never export raw study data."""
import argparse
import json
from pathlib import Path

from common import read_json, write_json


ALLOWED_KINDS = {"r3_comparison", "r3_native_compatibility", "r3_snapshot_installation", "r3_engineering_validation", "r3_failure_recovery", "r3_recovery_followup", "r3_installed_candidate_acceptance", "r3_value_comparison", "r3_observation_inventory", "r3_observation_preparation", "r3_persona_review"}


def redact(value, replacements):
    if isinstance(value, dict):
        # These fields do not belong to a public summary, even if supplied by
        # an accidentally selected private invocation/Run/Task record.
        forbidden = {"stdout", "stderr", "session_id", "output", "command", "env"}
        return {key: redact(item, replacements) for key, item in value.items() if key not in forbidden}
    if isinstance(value, list):
        return [redact(item, replacements) for item in value]
    if isinstance(value, str):
        for before, after in replacements:
            value = value.replace(before, after)
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--study", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    if args.out.exists():
        parser.error("fresh --out required")
    report = read_json(args.report)
    if report.get("kind") not in ALLOWED_KINDS:
        parser.error("only explicit R3 summary report kinds may be exported")
    study_paths = {str(args.study.absolute()), str(args.study.resolve())}
    replacements = [(path, "<private-study>") for path in sorted(study_paths, key=len, reverse=True)]
    public = redact(report, replacements + [(str(Path.home()), "<home>")])
    write_json(args.out, public)
    print(json.dumps({"kind": public["kind"], "status": public["status"]}))


if __name__ == "__main__":
    main()
