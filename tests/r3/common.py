"""Private, durable study state shared by foreground and detached processes."""
import contextlib
import fcntl
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import time


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def read_json(path):
    return json.loads(Path(path).read_text())


def write_json(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    temporary = path.with_name(path.name + f".{os.getpid()}.tmp")
    with temporary.open("x") as stream:
        os.chmod(temporary, 0o600)
        json.dump(value, stream, indent=2)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    temporary.replace(path)


@contextlib.contextmanager
def locked(path, nonblocking=False):
    with Path(path).open("a+") as stream:
        os.chmod(path, 0o600)
        fcntl.flock(stream, fcntl.LOCK_EX | (fcntl.LOCK_NB if nonblocking else 0))
        try:
            yield
        finally:
            fcntl.flock(stream, fcntl.LOCK_UN)


class Ledger:
    """Reserve before dispatch; even a crashed/failed invocation consumes a slot.

    A damaged ledger fails closed. Limits live in the prepared study, never in
    per-process environment variables or a new output filename.
    """
    def __init__(self, root):
        self.study_root = Path(root)
        study = read_json(self.study_root / "study.json")
        self.root = Path(study.get("ledger_root", root))
        self.binary_sha256 = study.get("binary_sha256")

    def entries(self):
        path = self.root / "calls.jsonl"
        if not path.exists():
            raise ValueError("invocation ledger missing; refusing to reset the budget")
        data = path.read_text()
        if data and not data.endswith("\n"):
            raise ValueError("truncated invocation ledger; reconcile privately before proceeding")
        entries = [json.loads(line) for line in data.splitlines()]
        if any(e["sequence"] != i + 1 for i, e in enumerate(entries)):
            raise ValueError("invalid invocation sequence")
        return entries

    def reserve(self, adapter, case, phase):
        with locked(self.root / "ledger.lock"):
            study = read_json(self.root / "study.json")
            frozen = read_json(self.root / "frozen.json")
            if digest(self.root / "study.json") != frozen["study_sha256"]:
                raise ValueError("invocation budget policy changed")
            inherited = study.get("inherited_calls", 0)
            for prior in study.get("prior_ledgers", []):
                if digest(Path(prior["path"]) / "calls.jsonl") != prior["ledger_sha256"]:
                    raise ValueError("previous campaign ledger changed")
            if inherited:
                prefix = (Path(study["previous_campaign"]["path"]) / "calls.jsonl").read_bytes()
                if not (self.root / "calls.jsonl").read_bytes().startswith(prefix):
                    raise ValueError("inherited ledger prefix changed")
            if case not in study["case_limits"]:
                raise ValueError("unregistered study case")
            entries = self.entries()
            if len(entries) >= study["budget"]:
                raise ValueError("R3 total invocation budget exhausted")
            if sum(e["case"] == case and e.get("binary_sha256") == self.binary_sha256 for e in entries[inherited:]) >= study["case_limits"][case]:
                raise ValueError("R3 case invocation budget exhausted")
            is_comparison = case in study["order"]
            partition = study["order"] if is_comparison else study["compatibility_cases"]
            key, default = ("comparison_calls", 36) if is_comparison else ("compatibility_calls", 12)
            limit = study.get("limits", {}).get(key, default)
            if sum(e["case"] in partition for e in entries[inherited:]) >= limit:
                raise ValueError("R3 phase invocation budget exhausted")
            entry = {"sequence": len(entries) + 1, "adapter": adapter, "case": case,
                     "phase": phase, "time": time.time(), "pid": os.getpid(), "binary_sha256": self.binary_sha256}
            with (self.root / "calls.jsonl").open("a") as stream:
                os.chmod(stream.name, 0o600)
                stream.write(json.dumps(entry) + "\n")
                stream.flush()
                os.fsync(stream.fileno())
            return entry


def process(argv, *, cwd=None, env=None, timeout=120, private=None):
    """Bound the whole native process group, including timed-out tool children."""
    start = time.monotonic()
    with subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          text=True, start_new_session=True) as child:
        timed_out = False
        try:
            stdout, stderr = child.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            os.killpg(child.pid, signal.SIGTERM)
            try:
                stdout, stderr = child.communicate(timeout=5)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                stdout, stderr = child.communicate()
        except BaseException:
            os.killpg(child.pid, signal.SIGKILL)
            child.communicate()
            raise
    result = {"exit_code": 124 if timed_out else child.returncode,
              "timed_out": timed_out, "seconds": round(time.monotonic() - start, 3),
              "stdout": stdout, "stderr": stderr}
    if private:
        write_json(private, result)
    return result
