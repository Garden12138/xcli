"""Release evidence scenario. Scoring/reference data never enter Agent inputs."""
import csv
import io
import json
import re


WORK_FILES = {
    "policy.md": """Release evidence audit v1
Evaluate production releases as of 2026-10-19T09:00:00Z, inclusive.
Every target needs a passing test and an approved, signed, unexpired approval.
Match component, environment, semantic version and artifact digest exactly.
Version 1.9.0 and version 1.10.0 are different versions; never select by text sorting.
Ignore future records and other environments. Within matching records, use latest recorded_at.
A matching failed test blocks; no matching test is test_missing.
Approval decision revoked blocks even if an earlier approval was approved.
An approved decision with signed=false is approval_unsigned; valid_until must be strictly after cutoff.
No matching approval is approval_missing. Use approval_expired for expired approvals.
Every dependency must itself be ready. Otherwise add dependency_not_ready.
Report all independently supported blockers, not just the first. Do not block a ready target.
Allowed rule codes: test_missing, test_failed, approval_missing, approval_unsigned,
approval_revoked, approval_expired, dependency_not_ready.
""",
    "targets.csv": """record_id,component,environment,version,digest,depends_on
T-alpha,alpha,production,1.10.0,sha-alpha,
T-beta,beta,production,2.0.0,sha-beta,
T-gamma,gamma,production,2.2.0,sha-gamma,
T-delta,delta,production,3.0.0,sha-delta,
T-epsilon,epsilon,production,1.0.0,sha-epsilon,alpha
T-zeta,zeta,production,1.0.0,sha-zeta,beta
""",
    "checks.csv": """record_id,component,environment,version,digest,recorded_at,result
C-alpha-old,alpha,production,1.9.0,sha-old,2026-10-19T08:55:00Z,failed
C-alpha-fail,alpha,production,1.10.0,sha-alpha,2026-10-19T08:00:00Z,failed
C-alpha-pass,alpha,production,1.10.0,sha-alpha,2026-10-19T08:30:00Z,passed
C-beta,beta,production,2.0.0,sha-beta,2026-10-19T08:30:00Z,passed
C-gamma-old,gamma,production,2.1.9,sha-gamma,2026-10-19T08:30:00Z,passed
C-gamma-digest,gamma,production,2.2.0,sha-wrong,2026-10-19T08:40:00Z,passed
C-delta,delta,production,3.0.0,sha-delta,2026-10-19T09:00:00Z,failed
C-delta-future,delta,production,3.0.0,sha-delta,2026-10-19T09:01:00Z,passed
C-epsilon,epsilon,production,1.0.0,sha-epsilon,2026-10-19T08:30:00Z,passed
C-zeta,zeta,production,1.0.0,sha-zeta,2026-10-19T08:30:00Z,passed
""",
    "approvals.csv": """record_id,component,environment,version,digest,recorded_at,decision,signed,valid_until
A-alpha,alpha,production,1.10.0,sha-alpha,2026-10-19T08:00:00Z,approved,true,2026-10-20T00:00:00Z
A-beta-old,beta,production,2.0.0,sha-beta,2026-10-19T08:00:00Z,approved,true,2026-10-20T00:00:00Z
A-beta-revoked,beta,production,2.0.0,sha-beta,2026-10-19T08:50:00Z,revoked,true,2026-10-20T00:00:00Z
A-beta-future,beta,production,2.0.0,sha-beta,2026-10-19T09:01:00Z,approved,true,2026-10-20T00:00:00Z
A-gamma,gamma,production,2.2.0,sha-gamma,2026-10-19T08:00:00Z,approved,false,2026-10-20T00:00:00Z
A-delta,delta,production,3.0.0,sha-delta,2026-10-19T08:00:00Z,approved,true,2026-10-20T00:00:00Z
A-delta-staging,delta,staging,3.0.0,sha-delta,2026-10-19T08:55:00Z,revoked,true,2026-10-20T00:00:00Z
A-epsilon,epsilon,production,1.0.0,sha-epsilon,2026-10-19T08:00:00Z,approved,true,2026-10-19T09:00:00Z
A-zeta,zeta,production,1.0.0,sha-zeta,2026-10-19T08:00:00Z,approved,true,2026-10-20T00:00:00Z
""",
}
WORK_GOAL = """Release evidence audit v1. Read all four selected files and evaluate every release target using policy.md.
Produce report.json and report.md without modifying inputs or invoking other Agent CLIs.
JSON schema: {"cutoff":"2026-10-19T09:00:00Z","ready_count":integer,"blocked_count":integer,
"targets":[{"component":string,"version":string,"digest":string,"decision":"ready"|"blocked"}],
"findings":[{"component":string,"rule":one allowed policy rule code,"record_ids":[supporting CSV record IDs],
"sources":[input filenames],"evidence":nonempty explanation}],"sources":[all input filenames]}.
Report every supported blocker once, and cite target plus concrete supporting evidence record IDs.
For missing checks, cite candidate records rejected for version/digest mismatches. For blocked dependencies, cite the dependency target and its blocking evidence.
Markdown must agree with JSON, cite all input filenames, and include one literal line per target formatted 'component: ready' or 'component: blocked'.
Review all rules and correct omissions before finishing. No command verification is required.
"""

# These expected outcomes were derived from the policy before model execution.
EXPECTED = {
    ("beta", "approval_revoked"): ({"T-beta", "A-beta-revoked"}, {"targets.csv", "approvals.csv"}),
    ("gamma", "test_missing"): ({"T-gamma", "C-gamma-old", "C-gamma-digest"}, {"targets.csv", "checks.csv"}),
    ("gamma", "approval_unsigned"): ({"T-gamma", "A-gamma"}, {"targets.csv", "approvals.csv"}),
    ("delta", "test_failed"): ({"T-delta", "C-delta"}, {"targets.csv", "checks.csv"}),
    ("epsilon", "approval_expired"): ({"T-epsilon", "A-epsilon"}, {"targets.csv", "approvals.csv"}),
    ("zeta", "dependency_not_ready"): ({"T-zeta", "T-beta", "A-beta-revoked"}, {"targets.csv", "approvals.csv"}),
}


def reference_report():
    targets = list(csv.DictReader(io.StringIO(WORK_FILES["targets.csv"])))
    return {"cutoff": "2026-10-19T09:00:00Z", "ready_count": 1, "blocked_count": 5,
            "targets": [{**{k: row[k] for k in ("component", "version", "digest")},
                         "decision": "ready" if row["component"] == "alpha" else "blocked"} for row in targets],
            "findings": [{"component": c, "rule": rule, "record_ids": sorted(ids), "sources": sorted(sources),
                          "evidence": "reference evidence " + ", ".join(sorted(ids))} for (c, rule), (ids, sources) in EXPECTED.items()],
            "sources": sorted(WORK_FILES)}


def score(output):
    result = {"passed": False, "findings": [], "omissions": [], "false_positives": [], "errors": []}
    try:
        data = json.loads((output / "report.json").read_text())
        text = (output / "report.md").read_text()
        expected = reference_report()
        for key in ("cutoff", "ready_count", "blocked_count"):
            if type(data.get(key)) is not type(expected[key]) or data[key] != expected[key]:
                result["errors"].append("incorrect summary: " + key)
        if sorted(data["targets"], key=lambda r: r["component"]) != sorted(expected["targets"], key=lambda r: r["component"]):
            result["errors"].append("incorrect or duplicated target decision/version/digest")
        if set(data["sources"]) != set(WORK_FILES) or not all(name in text for name in WORK_FILES):
            result["errors"].append("missing input citations")
        if not all(re.findall(r"(?m)^\s*(?:[-*]\s*)?" + re.escape(row["component"]) + r":\s*(ready|blocked)\s*$", text)
                   == [row["decision"]] for row in data["targets"]):
            result["errors"].append("markdown and JSON disagree")
        known = {row["record_id"] for name, content in WORK_FILES.items() if name.endswith(".csv")
                 for row in csv.DictReader(io.StringIO(content))}
        seen = set()
        for finding in data["findings"]:
            key = finding["component"], finding["rule"]
            label = "/".join(key)
            if key not in EXPECTED or key in seen:
                result["false_positives"].append(label)
                continue
            seen.add(key)
            ids, sources = EXPECTED[key]
            if (ids.issubset(set(finding["record_ids"])) and set(finding["record_ids"]).issubset(known)
                    and sources.issubset(set(finding["sources"])) and set(finding["sources"]).issubset(WORK_FILES)
                    and isinstance(finding["evidence"], str) and finding["evidence"].strip()):
                result["findings"].append(label)
            else:
                result["errors"].append("unsupported citation: " + label)
        result["omissions"] = sorted("/".join(k) for k in EXPECTED if "/".join(k) not in result["findings"])
        result["passed"] = not any(result[k] for k in ("omissions", "false_positives", "errors"))
    except (OSError, ValueError, KeyError, TypeError, AttributeError) as error:
        result["errors"].append("missing or malformed reports: " + type(error).__name__)
    return result
