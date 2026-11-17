#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' '将进行约 10 次真实 CLI 调用，会消耗各 CLI 的额度，按回车继续'
if ! IFS= read -r answer; then
  printf '%s\n' '请在终端运行 scripts/smoke.sh 并按回车继续。' >&2
  exit 1
fi

for tool in git go python3; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    printf '请安装 %s 并加入 PATH 后重新运行。\n' "$tool" >&2
    exit 1
  fi
done
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
if [[ -x "$root/bin/xcli" ]]; then
  xcli="$root/bin/xcli"
elif command -v xcli >/dev/null 2>&1; then
  xcli=$(command -v xcli)
else
  printf '%s\n' '请先在仓库根目录执行 go build -o bin/xcli . 后重新运行。' >&2
  exit 1
fi

work=$(mktemp -d "${TMPDIR:-/tmp}/xcli-smoke.XXXXXX")
repo="$work/repo"
mkdir -p "$repo"
cat >"$repo/go.mod" <<'EOF'
module xcli-smoke

go 1.26.0
EOF
cat >"$repo/add.go" <<'EOF'
package demo

func Add(a, b int) int { return a + b }
EOF
printf '# demo\n' >"$repo/README.md"
git -C "$repo" init -q
git -C "$repo" add .
git -C "$repo" -c user.name=xcli-smoke -c user.email=smoke@localhost commit -qm init
cat >"$work/functions.schema.json" <<'EOF'
{"type":"object","additionalProperties":false,"required":["functions"],"properties":{"functions":{"type":"array","items":{"type":"string"}}}}
EOF

passed=0
failed=0
run() {
  local name=$1 rc
  shift
  if "$xcli" "$@" --json >"$work/$name.json" 2>"$work/$name.log"; then
    rc=0
  else
    rc=$?
  fi
  if python3 - "$name" "$work/$name.json" "$rc" "$repo" <<'PY'
import json
import pathlib
import sys

name, output, code, repo = sys.argv[1:]
task = "-"

def require(ok, message):
    if not ok:
        raise ValueError(message)

try:
    data = json.loads(pathlib.Path(output).read_text())
    if isinstance(data, dict):
        task = data.get("task_id") or "-"
    require(int(code) == 0, f"退出码 {code}；请查看结果 JSON 与日志")
    if name == "agents":
        require(isinstance(data, list), "请检查 agents 的 JSON 输出")
        agents = {a["name"]: a for a in data}
        for agent in ("codex", "cursor", "antigravity"):
            row = agents.get(agent, {})
            require(row.get("installed") and row.get("available") and row.get("auth") != "missing",
                    f"请安装并登录 {agent}；冷却结束后可用 agents --reset 刷新")
    else:
        require(data.get("status") == "completed", data.get("summary") or "请查看任务结果")
        require(task != "-", "请检查结果中的 task_id")
    if name == "schema":
        require(isinstance(data.get("output"), dict) and "Add" in data["output"].get("functions", []),
                "请检查 output.functions 中的 Add 函数")
    if name in ("image", "do"):
        artifacts = data.get("artifacts") or []
        require(bool(artifacts), "请检查生图步骤和 artifacts")
        for image in artifacts:
            path = pathlib.Path(image["path"])
            require(path.is_file() and path.stat().st_size > 0 and image.get("width", 0) > 0 and image.get("height", 0) > 0,
                    f"请检查图片文件：{path}")
    if name == "code":
        require(data.get("review") is not None, "请检查另一个 CLI 的审查结果 review")
        require(data.get("verify") and all(v.get("passed") for v in data["verify"]),
                "请查看 verify 日志并修复测试")
        require((pathlib.Path(repo) / "add_test.go").is_file(), "请检查生成的 add_test.go")
    if name == "do":
        cover = pathlib.Path(repo) / "docs/cover.png"
        readme = (pathlib.Path(repo) / "README.md").read_text()
        require(cover.is_file() and cover.read_bytes().startswith(b"\x89PNG\r\n\x1a\n"),
                "请检查 docs/cover.png 的 PNG 文件")
        require("docs/cover.png" in readme and len(readme.strip()) > len("# demo"),
                "请检查 README 的项目介绍和封面引用")
    print(f"PASS {name} task={task}")
except (ValueError, OSError, KeyError, TypeError, AttributeError) as error:
    print(f"FAIL {name} task={task}: {error}")
    sys.exit(1)
PY
  then
    passed=$((passed + 1))
  else
    failed=$((failed + 1))
    printf '结果：%s\n日志：%s\n' "$work/$name.json" "$work/$name.log" >&2
  fi
}

printf '临时工作目录：%s\n' "$work"
run agents agents
for agent in codex cursor antigravity; do
  run "ask-$agent" ask '用一句话说明 add.go 的作用' -a "$agent" -C "$repo"
done
run schema ask '按 Schema 输出 add.go 的函数列表' -C "$repo" --schema "$work/functions.schema.json"
run image image '米白背景上的绿色植物插画，方形，无文字' -o "$work/images"
run code code '为 Add 写一个表驱动测试，保存为 add_test.go' -C "$repo" --verify 'go test ./...'
run do do '给 README 写一段项目介绍，并生成一张封面图放到 docs/cover.png，最后在 README 里引用它' -C "$repo"
printf '汇总：%d PASS / %d FAIL\n工作目录：%s\n' "$passed" "$failed" "$work"
[[ "$failed" -eq 0 ]]
