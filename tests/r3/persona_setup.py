#!/usr/bin/env python3
"""Isolated environments for AI persona walkthroughs, never human observations.

The xcli executable is real; native Agents are deterministic simulation fixtures.
No model calls, credentials, telemetry, or participant attestations are generated.
"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

from common import digest, write_json
from fixtures import PROJECT_FILES, WORK_FILES, put_files


def prepare(root, binary):
    root.mkdir(mode=0o700, parents=True, exist_ok=False)
    repository = Path(__file__).resolve().parents[2]
    profiles = {"novice": "初学者，已有原生 CLI 使用经验，首次使用 xcli",
                "regular": "熟练 CLI 用户，习惯 Git、JSON 和项目测试",
                "expert": "资深工程师，关注后台恢复、脚本协议与错误边界"}
    for name in profiles:
        folder = root / name
        (folder / ".control").mkdir(parents=True, mode=0o700)
        (folder / "bin").mkdir()
        (folder / "candidate").mkdir()
        shutil.copyfile(binary, folder / "candidate/xcli")
        (folder / "candidate/xcli").chmod(0o700)
        for file in ("fake_agent.py", "value_fixtures.py"):
            shutil.copyfile(Path(__file__).with_name(file), folder / ".control" / file)
        for file in ("README.md", "README.zh-CN.md"):
            shutil.copyfile(repository / file, folder / file)
        agents = {}
        for agent, adapter in (("codex", "codex"), ("cursor", "cursor")):
            wrapper = folder / "bin" / agent
            wrapper.write_text(f"#!{sys.executable}\nimport os,runpy\nos.environ['XCLI_R3_SIMULATION']='1'\nos.environ['XCLI_R3_FAKE_ADAPTER']={adapter!r}\nrunpy.run_path({str(folder / '.control/fake_agent.py')!r},run_name='__main__')\n")
            wrapper.chmod(0o700)
            agents[agent] = {"adapter": adapter, "command": str(wrapper)}
        for agent in ("claude", "gemini", "opencode", "antigravity"):
            agents[agent] = {"adapter": agent, "command": str(folder / ".control/not-enabled")}
        write_json(folder / ".control/config.json", {"version": 1, "default_agent": "codex", "agents": agents,
                                                      "recording": {"output": True}})
        environment = {"XDG_CONFIG_HOME": str(folder / "config"), "XDG_DATA_HOME": str(folder / "data"),
                       "GOCACHE": str(folder / "cache/go"), "GOMODCACHE": str(folder / "cache/modules"),
                       "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local"}
        launcher = folder / "xcli"
        launcher.write_text(f"#!{sys.executable}\nimport os,sys\nos.environ.update({environment!r})\nos.execv({str(folder / 'candidate/xcli')!r},[{str(folder / 'candidate/xcli')!r},'--config',{str(folder / '.control/config.json')!r},*sys.argv[1:]])\n")
        launcher.chmod(0o700)
        if name == "novice":
            put_files(folder / "materials", WORK_FILES)
        elif name == "regular":
            put_files(folder / "project", PROJECT_FILES)
            for args in (["init", "-q"], ["add", "."], ["-c", "user.name=Persona simulation", "-c", "user.email=persona@example.invalid", "commit", "-qm", "simulation baseline"]):
                subprocess.run(["git", "-C", str(folder / "project"), *args], check=True, capture_output=True)
        else:
            put_files(folder / "materials", {"facts.txt": "North: 12\nSouth: 9\nTotal: 21\n"})
        write_json(folder / "environment.json", {"kind": "ai_persona_simulation", "persona": name,
                    "experience_assumption": profiles[name], "real_human": False,
                    "native_backend": "deterministic fake Agent", "native_model_calls": 0,
                    "binary_sha256": digest(binary), "launcher_only_injects_isolated_paths": True,
                    "next_day_observation": None, "retention_observation": None})
    report = {"kind": "r3_persona_preparation", "profile_names": list(profiles),
              "binary_sha256": digest(binary), "native_model_calls": 0, "real_participants": 0,
              "status": "prepared", "setup_source_sha256": digest(__file__)}
    write_json(root / "preparation.json", report)
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--study", type=Path, required=True)
    parser.add_argument("--binary", type=Path, required=True)
    args = parser.parse_args()
    os.umask(0o077)
    result = prepare(args.study.resolve(), args.binary.resolve(strict=True))
    print(json.dumps(result))


if __name__ == "__main__":
    main()
