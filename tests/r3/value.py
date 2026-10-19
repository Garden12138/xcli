#!/usr/bin/env python3
"""Preregistered R3 value comparison; never count infrastructure failures as benefit."""
import argparse
import json
import os
from pathlib import Path

from common import Ledger, digest, locked, read_json, write_json
from experiment import prepare, run_case, validate
import value_fixtures


def assess(results):
    group = {r.get("arm", r.get("case", "").split("-")[-1]): r for r in results}
    cross = group.get("cross", {})
    comparisons = []
    for arm in ("direct", "single"):
        base = group.get(arm, {})
        eligible = (cross.get("status") == "passed" and cross.get("runtime_status") == "completed"
                    and base.get("runtime_status") == "completed" and "score" in base
                    and base.get("inputs_unchanged") is True and base.get("exit_code", 0) == 0
                    and base.get("mode_preserved", True) and base.get("current_review_bound", True)
                    and base.get("expected_roles_preserved", True))
        benefits = []
        if eligible and not base["score"]["passed"]:
            benefits.append("independent acceptance")
        if eligible and len(cross["score"]["findings"]) > len(base["score"]["findings"]):
            benefits.append("more independently supported findings")
        comparisons.append({"baseline": arm, "eligible": eligible, "observed_benefits": benefits})
    established = all(c["observed_benefits"] for c in comparisons)
    return {"comparisons": comparisons, "benefit_established_in_this_sample": established,
            "improvement_vs_any_baseline": any(c["observed_benefits"] for c in comparisons),
            "claim": "one preregistered synthetic sample only" if established else "no collaboration benefit established",
            "human_handoff_benefit": "not_measured", "cost_advantage": "not_established"}


def check(root):
    study = validate(root)
    if study.get("profile") != "value-comparison":
        raise ValueError("prepare a value comparison first")
    for prior in study["prior_ledgers"]:
        if digest(Path(prior["path"]) / "calls.jsonl") != prior["ledger_sha256"]:
            raise ValueError("prior campaign ledger changed")
    return study


def summarize(root, study):
    results = [read_json(root / "cases" / name / "result.json") if (root / "cases" / name / "result.json").exists()
               else {"case": name, "status": "not_run"} for name in study["order"]]
    entries = Ledger(root).entries()
    return {"kind": "r3_value_comparison", "scenario": study["scenario"], "source_commit": study["source_commit"],
            "binary_sha256": study["binary_sha256"], "frozen_study_sha256": digest(root / "study.json"),
            "native_versions": {k: v["version"] for k, v in study["native"].items()},
            "order": study["order"], "call_limit": study["budget"], "new_calls": len(entries),
            "historical_calls_before": study["historical_calls_before"],
            "historical_plus_new_calls": study["historical_calls_before"] + len(entries),
            "prior_ledgers_unchanged": all(digest(Path(p["path"]) / "calls.jsonl") == p["ledger_sha256"] for p in study["prior_ledgers"]),
            "ledger": [{k: row[k] for k in ("sequence", "case", "adapter", "phase", "binary_sha256")} for row in entries],
            "results": results, **assess(results), "independent_user_observation": "pending",
            "status": "passed" if all(r["status"] == "passed" for r in results) else "incomplete_or_failed"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("prepare", "run", "inspect"))
    parser.add_argument("--study", required=True, type=Path)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--previous-campaign", type=Path)
    parser.add_argument("--budget", type=int)
    parser.add_argument("--out", type=Path)
    parser.add_argument("--execute", action="store_true")
    args = parser.parse_args()
    os.umask(0o077)
    root = args.study.resolve()
    if args.action == "prepare":
        if not args.binary or not args.previous_campaign or args.budget is None:
            parser.error("prepare requires --binary, --previous-campaign and an explicit --budget")
        if not 1 <= args.budget <= 16:
            parser.error("value pilot budget must be 1..16; authorize before executing")
        prepare(root, args.binary, args.budget, value_campaign=args.previous_campaign.resolve())
        print(json.dumps({"status": "prepared", "native_calls": 0, "call_limit": args.budget}))
        return
    if not args.out or args.out.exists() or args.budget is not None:
        parser.error("fresh --out required; budget is prepare-only")
    study = check(root)
    if args.action == "run":
        if not args.execute:
            parser.error("real execution requires --execute and authorized budget")
        with locked(root / "runner.lock", nonblocking=True):
            for name in study["order"]:
                result = run_case(root, study, name, work_scorer=value_fixtures.score)
                print(json.dumps({k: result.get(k) for k in ("case", "status", "calls", "seconds")}), flush=True)
    write_json(args.out, summarize(root, study))


if __name__ == "__main__":
    main()
