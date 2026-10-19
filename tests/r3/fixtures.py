"""Frozen inputs and independent scoring. Oracles never enter Agent workspaces."""
import json
from pathlib import Path
import re
import shutil
import tempfile

from common import digest, process

WORK_FILES = {
    "draft.txt": "Revision 1, 2026-10-01. North: 12. South: 8. Total: 20. Release: 2026-10-20. Pilot ready: yes. This is an obsolete draft.\n",
    "revision.txt": "Revision 2, approved 2026-10-16, supersedes revision 1. North: 12. South: 9. Release: 2026-10-25. Recompute the total from this revision. Pilot requires a signed security approval.\n",
    "approval.txt": "Security approval register, 2026-10-17: the pilot approval is pending, unsigned. No approval has been granted.\n",
}
PROJECT_FILES = {
    "go.mod": "module r3fixture\n\ngo 1.26\n",
    "cache.go": "package r3fixture\n\n// Alive reports whether an entry inserted at inserted is live at now.\nfunc Alive(inserted, now, ttl int64) bool { return now-inserted <= ttl }\n",
    "cache_test.go": "package r3fixture\nimport \"testing\"\nfunc TestAlive(t *testing.T) { if !Alive(100, 105, 10) { t.Fatal(\"live entry\") }; if Alive(100, 110, 10) { t.Fatal(\"expiry is exclusive\") } }\n",
    "requirements.txt": "Alive uses signed int64 timestamps and TTL in the same unit. An entry is live exactly when TTL is positive, now is not before inserted, and mathematical elapsed time is strictly less than TTL. Handle the entire int64 range without overflow. Only cache.go may change.\n",
}
WORK_GOAL = '''Read all three input documents and produce report.md and report.json. Prefer the latest approved revision, recompute totals, distinguish pilot readiness from a planned release, and identify contradictions in the obsolete draft. Cite the input filenames in report.md. report.json must be a single JSON object with north, south, total (integers), release_date (YYYY-MM-DD), pilot_approved (boolean), sources (input filenames), and findings (objects with kind, evidence and sources). Use finding kinds total_conflict, date_conflict, approval_missing where supported, and do not invent other findings. Explain each finding with concrete evidence. The two reports must agree. Do not modify inputs or invoke other Agent CLIs.'''
PROJECT_GOAL = '''Fix Alive in cache.go to meet requirements.txt across the entire int64 range. Preserve go.mod, cache_test.go and requirements.txt byte-for-byte. Only cache.go may change. Produce a correct implementation, check ordinary and boundary cases including nonpositive TTL, clock rollback and overflow, and review the result. Do not install tools or invoke other Agent CLIs.'''
HIDDEN_TEST = '''package r3fixture
import ("math"; "testing")
func TestR3IndependentBoundaries(t *testing.T) {
 for _, c := range []struct{name string; inserted, now, ttl int64; want bool}{
 {"ordinary",100,105,10,true}, {"equality",100,110,10,false},
 {"zero",100,100,0,false}, {"negative",100,100,-1,false},
 {"rollback",100,99,10,false}, {"overflow",math.MinInt64,math.MaxInt64,math.MaxInt64,false},
 {"negative timestamp",-20,-15,10,true}, {"maximum live",0,math.MaxInt64-1,math.MaxInt64,true},
 {"maximum expiry",0,math.MaxInt64,math.MaxInt64,false},
 } { t.Run(c.name,func(t *testing.T){if got:=Alive(c.inserted,c.now,c.ttl);got!=c.want{t.Fatalf("Alive=%v want %v",got,c.want)}}) }
}
'''


def put_files(folder, files):
    folder.mkdir(parents=True, mode=0o700)
    for name, text in files.items():
        (folder / name).write_text(text)
        (folder / name).chmod(0o600)


def manifest(folder):
    return {p.name: digest(p) for p in sorted(folder.iterdir()) if p.is_file()}


def score_work(output):
    expected = {"north": 12, "south": 9, "total": 21, "release_date": "2026-10-25", "pilot_approved": False}
    result = {"passed": False, "correct_facts": 0, "findings": [], "omissions": [], "false_positives": [], "errors": []}
    try:
        data = json.loads((output / "report.json").read_text())
        text = (output / "report.md").read_text()
        for key, value in expected.items():
            if type(data.get(key)) is type(value) and data[key] == value:
                result["correct_facts"] += 1
            else:
                result["errors"].append("incorrect fact: " + key)
        sources = set(WORK_FILES)
        if not sources.issubset(set(data["sources"])) or not all(name in text for name in sources):
            result["errors"].append("missing source citations")
        if not all(re.search(pattern, text, re.I) for pattern in (r"\b12\b", r"\b9\b", r"\b21\b", "2026-10-25", r"pending|unsigned|not approved|未批准|待审批")):
            result["errors"].append("markdown lacks current facts or approval qualification")
        evidence = {"total_conflict": ({"draft.txt", "revision.txt"}, ("20", "21")),
                    "date_conflict": ({"draft.txt", "revision.txt"}, ("2026-10-20", "2026-10-25")),
                    "approval_missing": ({"approval.txt"}, ("pending", "unsigned", "未批准", "待审批"))}
        seen = set()
        for finding in data["findings"]:
            kind = finding["kind"]
            if kind not in evidence or kind in seen:
                result["false_positives"].append(kind)
                continue
            seen.add(kind)
            required, terms = evidence[kind]
            detail = finding["evidence"].lower()
            supported = any(t in detail for t in terms) if kind == "approval_missing" else all(t in detail for t in terms)
            if required.issubset(set(finding["sources"])) and supported:
                result["findings"].append(kind)
            else:
                result["errors"].append("unsupported finding: " + kind)
        result["omissions"] = sorted(set(evidence) - set(result["findings"]))
        result["passed"] = result["correct_facts"] == 5 and not any(result[k] for k in ("errors", "omissions", "false_positives"))
    except (OSError, ValueError, KeyError, TypeError, AttributeError) as error:
        result["errors"].append("missing or malformed reports: " + type(error).__name__)
    return result


def score_project(output, private):
    result = {"passed": False, "errors": [], "independent_tests": "not_run"}
    try:
        if not output.is_dir() or (output / "cache.go").is_symlink():
            raise ValueError("invalid implementation path")
        for name, text in PROJECT_FILES.items():
            if name != "cache.go" and (output / name).read_text() != text:
                result["errors"].append("protected file changed: " + name)
        visible = set(PROJECT_FILES)
        extra = [p.name for p in output.iterdir() if p.name not in visible and p.name != ".git"]
        if extra:
            result["errors"].append("unexpected workspace files")
        # Copy only permitted source into a fresh evaluation project. Hidden
        # tests are never fed back to a worker or used as runtime repair hints.
        with tempfile.TemporaryDirectory(prefix="xcli-r3-oracle-") as temporary:
            target = Path(temporary)
            (target / "go.mod").write_text(PROJECT_FILES["go.mod"])
            shutil.copyfile(output / "cache.go", target / "cache.go")
            (target / "oracle_test.go").write_text(HIDDEN_TEST)
            check = process(["go", "test", "-count=1", "./..."], cwd=target, timeout=45, private=private)
            result["independent_tests"] = "passed" if check["exit_code"] == 0 else "failed"
        result["passed"] = not result["errors"] and result["independent_tests"] == "passed"
    except (OSError, ValueError) as error:
        result["errors"].append(str(error))
    return result
