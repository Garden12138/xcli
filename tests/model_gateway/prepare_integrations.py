#!/usr/bin/env python3
"""Prepare a reviewable Worklog patch without modifying the external repository."""
import argparse
import difflib
from pathlib import Path


def replace_once(text, before, after):
    if text.count(before) != 1:
        raise ValueError("integration source drifted; review the current repository before patching")
    return text.replace(before, after, 1)


def prepare_worklog(root):
    changes = {
        "src-tauri/src/commands.rs": [
            ('    match provider {\n', '    match provider {\n        "xcli" => Ok(("http://127.0.0.1:18741/v1", "xcli/text-default", 180)),\n'),
            ('        assert!(provider_defaults("unsupported").is_err());', '        assert_eq!(provider_defaults("xcli").unwrap(), ("http://127.0.0.1:18741/v1", "xcli/text-default", 180));\n        assert!(provider_defaults("unsupported").is_err());')],
        "frontend/src/types.ts": [
            ('export type Provider = "openai"', 'export type Provider = "xcli" | "openai"')],
        "frontend/src/main.tsx": [
            ('const providerLabels: Record<Provider, string> = {\n', 'const providerLabels: Record<Provider, string> = {\n  xcli: "xcli 本机网关（推理位置取决于上游）",\n'),
            ('const providerDefaults: Record<Provider, { base_url: string; model: string; timeout_seconds: number }> = {\n', 'const providerDefaults: Record<Provider, { base_url: string; model: string; timeout_seconds: number }> = {\n  xcli: { base_url: "http://127.0.0.1:18741/v1", model: "xcli/text-default", timeout_seconds: 180 },\n'),
            ('                <option value="openai">OpenAI</option>', '                <option value="xcli">xcli 本机网关</option>\n                <option value="openai">OpenAI</option>')],
    }
    output = []
    for name, replacements in changes.items():
        before = (root / name).read_text()
        after = before
        for old, new in replacements:
            after = replace_once(after, old, new)
        output.extend(difflib.unified_diff(before.splitlines(keepends=True), after.splitlines(keepends=True),
                                         fromfile="a/" + name, tofile="b/" + name))
    return "".join(output)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--worklog", type=Path, required=True)
    p.add_argument("--output", type=Path, required=True)
    args = p.parse_args()
    patch = prepare_worklog(args.worklog.resolve())
    with args.output.open("x") as output:
        output.write(patch)
    print("Prepared a reviewable patch; external source remains unchanged")


if __name__ == "__main__":
    main()
