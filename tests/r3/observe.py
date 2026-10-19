#!/usr/bin/env python3
"""Prepare a private participant kit or summarize participant-written records.

Never invokes models, contacts participants, or certifies human independence.
"""
import argparse
from datetime import date
import hashlib
import json
import os
from pathlib import Path
import shutil
import zipfile


def read(path):
    return json.loads(path.read_text())


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def template(identifier):
    return {"schema": 1, "participant_id": identifier, "participant_attested": None,
            "native_cli_experience": None, "platform": None, "binary_sha256": None, "cli_versions": {},
            "day1_date": None, "day2_date": None, "goal": None, "mode": None, "task_id": None,
            "acceptance": None, "installation_minutes": None, "first_success_minutes": None,
            "help_count": None, "help_events": [], "failure_diagnosis": None,
            "background_stop_and_continue": None, "next_day_task_materials_and_budget": None,
            "observed_days": None, "spontaneous_reuse": None, "exit_or_reuse_reason": None,
            "evidence_refs": [], "publication_consent": None}


def prepare(kit, dist, installation):
    report = read(installation)
    if report.get("status") != "passed" or len(report.get("targets", [])) != 4:
        raise ValueError("require a successful four-target installation report")
    if {(t["os"], t["arch"]) for t in report["targets"]} != {(s, a) for s in ("darwin", "linux") for a in ("amd64", "arm64")}:
        raise ValueError("candidate target matrix mismatch")
    if sha(dist / "checksums.txt") != report["checksums_sha256"]:
        raise ValueError("candidate checksum manifest changed")
    kit.mkdir(mode=0o700, parents=True, exist_ok=False)
    (kit / "candidates").mkdir()
    for target in report["targets"]:
        archive = dist / target["archive"]
        if Path(target["archive"]).name != target["archive"] or archive.parent != dist or sha(archive) != target["archive_sha256"]:
            raise ValueError("candidate archive name/checksum mismatch")
        shutil.copyfile(archive, kit / "candidates" / archive.name)
    shutil.copyfile(dist / "checksums.txt", kit / "candidates/checksums.txt")
    write(kit / "candidate-manifest.json", report)
    for identifier in "ABC":
        write(kit / (identifier + ".json"), template(identifier))
    shutil.copyfile(Path(__file__), kit / "observe.py")
    guide = Path(__file__).resolve().parents[2] / "docs/r3/participant.zh-CN.md"
    shutil.copyfile(guide, kit / "请先阅读.md")
    archive = kit.with_suffix(".zip")
    if archive.exists():
        raise ValueError("participant zip destination already exists")
    with zipfile.ZipFile(archive, "x", zipfile.ZIP_DEFLATED) as stream:
        for path in sorted(kit.rglob("*")):
            if path.is_file():
                stream.write(path, str(path.relative_to(kit)))
    return {"status": "prepared", "participants": 0, "blank_records": 3, "native_model_invocations": 0,
            "zip_sha256": sha(archive), "targets": [{k: t[k] for k in ("os", "arch", "archive_sha256", "binary_sha256")}
                                                   for t in report["targets"]]}


def summarize(kit):
    manifest = read(kit / "candidate-manifest.json")
    candidates = {t["os"] + "/" + t["arch"]: t["binary_sha256"] for t in manifest["targets"]}
    rows = []
    for identifier in "ABC":
        record = read(kit / (identifier + ".json"))
        if record.get("participant_id") != identifier or record.get("schema") != 1:
            raise ValueError("duplicate/mismatched participant identity or schema")
        missing = [k for k in template(identifier) if k != "schema" and (record.get(k) is None or record.get(k) == "" or record.get(k) == [] or record.get(k) == {})]
        if record.get("help_count") == 0 and "help_events" in missing:
            missing.remove("help_events")
        errors = []
        if record.get("participant_attested") not in (None, True):
            errors.append("participant must attest to performing the operations personally")
        if record.get("binary_sha256") is not None and candidates.get(record.get("platform")) != record["binary_sha256"]:
            errors.append("candidate platform/digest mismatch")
        for name in ("installation_minutes", "first_success_minutes", "help_count", "observed_days"):
            value = record.get(name)
            if value is not None and (type(value) not in (int, float) or value < 0):
                errors.append("invalid numeric observation: " + name)
            elif name in ("help_count", "observed_days") and value is not None and type(value) is not int:
                errors.append("count must be an integer: " + name)
        if record.get("help_count") is not None and (not isinstance(record.get("help_events"), list)
                                                     or len(record["help_events"]) != record["help_count"]):
            errors.append("help count and recorded help events disagree")
        if record.get("day1_date") and record.get("day2_date"):
            try:
                if date.fromisoformat(record["day2_date"]) <= date.fromisoformat(record["day1_date"]):
                    errors.append("next-day observation must occur on a later date")
            except (ValueError, TypeError):
                errors.append("invalid observation date")
        complete = not missing and not errors and record.get("participant_attested") is True
        rows.append({"participant_id": identifier, "status": "needs_manual_review" if complete else "incomplete",
                     "missing_fields": missing, "errors": errors})
    return {"kind": "r3_observation_inventory", "status": "needs_manual_review" if all(r["status"] == "needs_manual_review" for r in rows) else "pending_participant_records",
            "planned_participants": 3, "complete_records": sum(r["status"] == "needs_manual_review" for r in rows),
            "independent_observation_verified": False, "rows": rows,
            "note": "Records require human review of independence, coverage, evidence and consent; template completion alone never passes R3."}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("prepare", "summarize"))
    parser.add_argument("--kit", type=Path, required=True)
    parser.add_argument("--dist", type=Path)
    parser.add_argument("--installation", type=Path)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    if args.out.exists():
        parser.error("fresh --out required")
    os.umask(0o077)
    if args.action == "prepare":
        if not args.dist or not args.installation:
            parser.error("prepare requires --dist and --installation")
        result = prepare(args.kit.resolve(), args.dist.resolve(), args.installation.resolve())
    else:
        result = summarize(args.kit.resolve())
    write(args.out, result)
    print(json.dumps({k: result[k] for k in ("status", "complete_records") if k in result}))


if __name__ == "__main__":
    main()
