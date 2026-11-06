#!/usr/bin/env python3
"""Opt-in, no-retry native image acceptance; one generation/edit pair per CLI."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time

from PIL import Image, ImageChops, ImageStat
from smoke import native_environment


def image_info(path):
    path = Path(path)
    if path.is_symlink() or not path.is_file() or path.stat().st_size > 25 * 1024 * 1024:
        raise ValueError("not_regular_or_oversized")
    with Image.open(path) as image:
        if image.format not in {"PNG", "JPEG", "WEBP"} or image.width * image.height > 16777216:
            raise ValueError("format_or_pixel_limit")
        image.verify()
    with Image.open(path) as image:
        image.load()
        return {"format": image.format, "mime": Image.MIME[image.format], "width": image.width,
                "height": image.height, "byte_length": path.stat().st_size,
                "sha256": hashlib.sha256(path.read_bytes()).hexdigest()}


def allowed_image(path, workspace):
    path = Path(path).expanduser()
    if not path.is_absolute() or path.is_symlink():
        return False
    real = path.resolve()
    roots = [workspace.resolve(), Path(os.environ.get("CODEX_HOME", str(Path.home() / ".codex"))).resolve(),
             (Path.home() / ".cursor/projects").resolve(), (Path.home() / ".gemini/antigravity").resolve(),
             (Path.home() / ".gemini/antigravity-cli").resolve()]
    if real.is_relative_to(roots[0]):
        return True
    return any(real.is_relative_to(root) for root in roots[1:]) and any(
        part.lower() in {"artifacts", "assets", "images", "generated_images", "generated-images", "brain"}
        for part in real.parts)


def classify(result, stderr):
    content = (result.get("output", "") + "\n" + stderr).lower()
    for category, expressions in [
        ("quota", ["quota", "usage limit", "rate limit", "credits exhausted"]),
        ("extra_billing", ["purchase credits", "enable paid", "additional payment"]),
        ("workspace_trust", ["not trusted", "untrusted", "workspace trust"]),
        ("authentication", ["unauthorized", "not authenticated", "authentication required", "not logged in"]),
        ("permissions", ["permission denied", "auto-denied", "denied_actions", "approval required", "permission_", "blocked"]),
        ("network", ["connection", "network error", "dns", "timed out connecting"]),
        ("native_tool_missing", ["not_supported", "no image", "image generation is not available", "cannot generate images", "don't have an image"]),
    ]:
        if any(expression in content for expression in expressions):
            return category
    if result.get("error_kind") == "quota_exceeded":
        return "quota"
    return "unclassified"


def bytes_status(valid_bytes, image_tool_events):
    if not valid_bytes:
        return None
    return "PASS_BYTES_PENDING_VISUAL" if image_tool_events else "PASS_BYTES_PENDING_REVIEW"


def media_status(valid_bytes, image_tool_events, category):
    if category in {"quota", "extra_billing", "workspace_trust", "authentication", "permissions", "network"}:
        return "BLOCKED"
    return bytes_status(valid_bytes, image_tool_events) or ("NOT_SUPPORTED" if category == "native_tool_missing" else "FAIL")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--text-report", required=True)
    parser.add_argument("--preflight", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--artifacts", required=True)
    parser.add_argument("--edit-only", action="store_true", help="perform only unused edit slots after offline generation review")
    parser.add_argument("--verified-generations", help="offline review with original artifact hashes")
    parser.add_argument("--prior-media-report", help="mandatory previous ledger for edit-only budget accounting")
    parser.add_argument("--execute", action="store_true")
    args = parser.parse_args()
    if not args.execute:
        parser.error("requires explicit --execute and authorized media budget")
    output = Path(args.out).resolve()
    if output.exists():
        parser.error("existing evidence must never be replaced or replayed")
    binary = str(Path(args.binary).resolve())
    artifacts = Path(args.artifacts).resolve()
    artifacts.mkdir(parents=True, exist_ok=True)
    preflight = json.loads(Path(args.preflight).read_text())
    text_report = json.loads(Path(args.text_report).read_text())
    reviewed = {}
    prior_starts = 0
    if args.edit_only:
        if not args.verified_generations or not args.prior_media_report:
            parser.error("edit-only requires reviewed generations and previous media ledger")
        reviewed = {item["agent"]: item for item in json.loads(Path(args.verified_generations).read_text())["generations"] if item["status"] == "PASS"}
        prior = json.loads(Path(args.prior_media_report).read_text())
        prior_starts = prior["invocations"]
        if any(item["kind"] == "edit" for item in prior["calls"]):
            parser.error("an edit was already attempted; no repeat is allowed")
    report = {"kind": "real-native-media", "max_process_starts": 6, "invocations": 0,
              "prior_media_process_starts": prior_starts,
              "started_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), "calls": [], "skipped": []}
    def save():
        temporary = output.with_suffix(".tmp")
        temporary.write_text(json.dumps(report, indent=2) + "\n")
        temporary.replace(output)
    text_tools = {item["agent"]: item for item in text_report["tools"]}
    for tool, command in [("codex", "codex"), ("cursor", "cursor-agent"), ("antigravity", "agy")]:
        if text_tools.get(tool, {}).get("status") != "passed":
            report["skipped"].append({"agent": tool, "operations": ["generate", "edit"], "status": "BLOCKED",
                                      "reason": "text_precondition_failed; no retry or permission expansion"})
            save()
            continue
        work = Path(tempfile.mkdtemp(prefix="xcli-native-media-" + tool + "-"))
        work.chmod(0o700)
        subprocess.run(["git", "init", "-q", str(work)], check=True, capture_output=True)
        (work / "README.txt").write_text("Isolated synthetic image acceptance. Do not read other projects or credentials.\n")
        native = shutil.which(command)
        config = work / "xcli.json"
        config.write_text(json.dumps({"version": 1, "recording": {"output": False}, "agents": {
            tool: {"adapter": tool, "command": sys.executable,
                   "args": [str(Path(__file__).with_name("native_audit.py").resolve()), native]}}}))
        env = native_environment(os.environ)
        env["XDG_DATA_HOME"] = str(work / "xcli-state")
        policy = {"codex": ["--sandbox", "workspace-write", "-c", 'approval_policy="never"'],
                  "cursor": ["--sandbox", "enabled"], "antigravity": ["--sandbox"]}[tool]
        reference = None
        reference_hash = None
        if args.edit_only:
            if tool not in reviewed:
                report["skipped"].append({"agent": tool, "operations": ["edit"], "status": "NOT_TESTED", "reason": "generation has not passed offline review"})
                save()
                continue
            original = artifacts / reviewed[tool]["evidence_file"]
            reference_hash = image_info(original)["sha256"]
            if reference_hash != reviewed[tool]["sha256"]:
                raise RuntimeError("reviewed original hash changed; no call allowed")
            reference = work / ("reference" + original.suffix)
            shutil.copyfile(original, reference)
        for operation in (["edit"] if args.edit_only else ["generate", "edit"]):
            if operation == "edit" and reference is None:
                report["skipped"].append({"agent": tool, "operations": ["edit"], "status": "NOT_TESTED",
                                          "reason": "no validated original; failed generation is not retried"})
                save()
                break
            entry = {"agent": tool, "version": preflight["cli"][tool]["version"], "kind": operation,
                     "status": "RUNNING", "timeout_seconds": 180, "native_timeout_seconds": 120 if tool == "antigravity" else None}
            report["calls"].append(entry)
            report["invocations"] += 1
            if prior_starts + report["invocations"] > 6:
                raise RuntimeError("media budget exceeded")
            save()
            audit_path = work / (operation + "-audit.json")
            env["XCLI_NATIVE_AUDIT"] = str(audit_path)
            prompt = ("Use only your native image generation tool" +
                      (" through the built-in image-generator subagent" if tool == "antigravity" else "") +
                      ". Make exactly ONE image, no retries, no external APIs or other AI CLIs, no code/SVG/HTML drawing. " +
                      ("A square illustration of a green leafy plant in a terracotta brown pot, on a cream background, no text. " if operation == "generate" else
                       "Edit the reference image at " + str(reference) + ": change ONLY the pot to vivid blue; preserve the plant, cream background and composition. Do not overwrite the reference. ") +
                      "Keep native file format. Do not convert or copy files. Wait for completion and return only JSON with status and artifact_path (the actual local absolute path). " +
                      "If the native tool or permission is unavailable, stop and return status NOT_SUPPORTED or BLOCKED with a short reason. Never change settings, permissions, billing, login or trust.")
            extra = policy + (["--image", str(reference)] if tool == "codex" and reference is not None else [])
            argv = [binary, "--config", str(config), "run", "--agent", tool, "--cwd", str(work), "--json", prompt, "--"] + extra
            started = time.monotonic()
            wall_started = time.time()
            process = subprocess.Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=env, start_new_session=True)
            result = {}
            artifact = None
            try:
                stdout, stderr = process.communicate(timeout=180)
                entry["xcli_exit_code"] = process.returncode
                try:
                    result = json.loads(stdout)
                except ValueError:
                    pass
                entry.update({key: result[key] for key in ["exit_code", "parse_status", "error_kind", "usage"] if key in result})
                entry["model_output_status"] = result.get("status", "no_result")
                body = result.get("output", "")
                entry["output_sha256"] = hashlib.sha256(body.encode()).hexdigest()
                category = classify(result, stderr)
                entry["diagnostic_category"] = category
                paths = re.findall(r'(?:/[^\s<>"`]+|~/[^\s<>"`]+)\.(?:png|jpg|jpeg|webp)', body, re.I)
                for candidate in paths:
                    candidate = Path(candidate).expanduser()
                    if allowed_image(candidate, work) and candidate.exists() and candidate.stat().st_mtime >= wall_started - 2:
                        try:
                            info = image_info(candidate)
                        except (ValueError, OSError):
                            continue
                        suffix = {"PNG": ".png", "JPEG": ".jpg", "WEBP": ".webp"}[info["format"]]
                        destination = artifacts / (tool + "-" + operation + suffix)
                        if destination.exists():
                            raise RuntimeError("artifact destination must be new")
                        shutil.copyfile(candidate, destination)
                        info["evidence_file"] = destination.name
                        info["native_origin_path_redacted"] = True
                        entry["artifact"] = info
                        artifact = candidate
                        break
                if audit_path.exists():
                    entry["native_audit"] = json.loads(audit_path.read_text())
                if operation == "edit":
                    entry["original_unchanged"] = image_info(reference)["sha256"] == reference_hash
                    if artifact is not None:
                        with Image.open(reference) as before, Image.open(artifact) as after:
                            a = before.convert("RGB").resize((256, 256))
                            b = after.convert("RGB").resize((256, 256))
                            entry["normalized_mean_pixel_difference"] = round(sum(ImageStat.Stat(ImageChops.difference(a, b)).mean) / (3 * 255), 6)
                valid = (artifact is not None and result.get("status") == "success" and process.returncode == 0
                         and (operation != "edit" or (entry.get("original_unchanged") and entry.get("normalized_mean_pixel_difference", 0) > 0.005)))
                entry["status"] = media_status(valid, entry.get("native_audit", {}).get("image_tool_events", 0), category)
                if entry["status"].startswith("PASS_BYTES") and operation == "generate" and entry.get("native_audit", {}).get("image_tool_events", 0) > 0:
                    reference = artifact
                    reference_hash = entry["artifact"]["sha256"]
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.communicate(timeout=5)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.communicate()
                entry.update({"status": "BLOCKED", "diagnostic_category": "timeout", "xcli_exit_code": process.returncode})
            finally:
                entry["duration_seconds"] = round(time.monotonic() - started, 3)
                save()
                print(json.dumps({key: entry[key] for key in ["agent", "kind", "status", "duration_seconds"]}), flush=True)
    report["ended_at"] = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    save()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
