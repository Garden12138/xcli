# 03 接口规范（协议 v1）

## 1. 命令行

### 1.1 命令一览

| 命令 | 作用 |
| --- | --- |
| `xcli do "<需求>"` | 智能编排（`capability=auto`）：CLI 生成计划后执行 |
| `xcli plan "<需求>"` | 只生成计划不执行，等价于 `do --plan-only` |
| `xcli ask "<要求>"` | 问答、文本生成、分析、结构化输出（只读） |
| `xcli code "<需求>"` | 编码；默认安排另一个 CLI 审查并迭代修复 |
| `xcli review "<审查要求>"` | 只读代码审查 |
| `xcli image "<描述>"` | 生图；带 `--image` 时为改图 |
| `xcli submit --request <file\|->` | 从 JSON 读取完整 Request（应用接入首选） |
| `xcli status <id>` | 查看任务状态（摘要） |
| `xcli wait <id> [--timeout 10m]` | 等待任务结束并输出 Result |
| `xcli logs <id> [-f] [--after N]` | 输出事件流（JSONL） |
| `xcli result <id>` | 输出 Result JSON |
| `xcli cancel <id>` | 取消前台或后台 worker 任务；HTTP 任务返回对应 HTTP 取消接口指引 |
| `xcli list [--limit 20]` | 列出最近的任务 |
| `xcli resume <id> "<追加要求>"` | 基于已结束的任务继续：复用最后一步的 CLI、会话、工作目录和输出目录 |
| `xcli apply <id>` | 把 `--isolate` 任务的改动应用回原仓库 |
| `xcli agents [--json] [--reset]` | 列出 CLI：是否安装、版本、登录状态、冷却、strengths |
| `xcli config show\|init\|path` | 查看生效配置 / 写入默认配置 / 显示配置路径 |
| `xcli serve [--listen 127.0.0.1:7788]` | 本机 HTTP 任务接口（第 5 阶段） |
| `xcli gc [--older-than 7d]` | 清理旧任务 |
| `xcli stats [--since 30d] [--until YYYY-MM-DD] [--app NAME] [--json]` | 查看接入应用的调用量、耗时、失败原因和用量 |

### 1.2 通用参数（`do` / `plan` / `ask` / `code` / `review` / `image`）

| 参数 | 对应 Request 字段 | 说明 |
| --- | --- | --- |
| `-C, --workdir <dir>` | `workdir` | 工作目录 |
| `-a, --agent <name>` | `agent` | 所有步骤都用这个 CLI，不做路由，也不切换 |
| `--agents a,b` | `agents` | 允许使用的 CLI 及偏好顺序 |
| `--app <name>` | `metadata.app` | 标记调用方应用，终端默认 `terminal` |
| `-i, --input <path>` | `inputs` | 输入文件或目录，可重复 |
| `--image <path>` | `images` | 参考图，可重复 |
| `-o, --out <dir>` | `output_dir` | 产物目录 |
| `--schema <file>` | `schema_file` | 输出 JSON Schema |
| `--permission read\|write\|full` | `permission` | 默认 write（ask / review 固定 read） |
| `--isolate` | `isolate` | 在 git worktree 中执行代码改动 |
| `--review` / `--no-review` | `review` | 覆盖默认审查策略 |
| `--reviewer <name>` | `reviewer` | 指定审查者 |
| `--verify "<cmd>"` | `verify` | 验证命令，可重复 |
| `--max-fix-rounds N` | `max_fix_rounds` | 默认 2 |
| `--model agent=model` | `models` | 指定某个 CLI 的模型，可重复 |
| `--timeout 30m` | `timeout_sec` | 任务总超时 |
| `--no-fallback` | `fallback=false` | 关闭自动切换 |
| `--plan <file>` | `plan` | 直接执行给定计划 |
| `--plan-only` | `plan_only` | 只规划 |
| `--detach` | — | 后台运行，立即输出 `{"task_id": "..."}` |
| `--json` | — | stdout 只输出最终 Result JSON |
| `--events` | — | stdout 输出事件 JSONL，最后一条是 `task.finished`（携带 Result） |

默认输出（不带 `--json` / `--events`）：进度以人类可读的形式写 stderr，结束后在 stdout 打印摘要、产物路径和任务 ID。

### 1.3 退出码

| 退出码 | 含义 |
| --- | --- |
| 0 | `completed` 或 `planned` |
| 1 | `failed` |
| 2 | `invalid`（参数或请求不合法） |
| 3 | `needs_attention` |
| 130 | `cancelled` |

提交或执行任务时，`--json` 模式会把完整的 Result JSON 写到 stdout。调用方先解析 stdout，再看退出码。

## 2. Request

```json
{
  "version": 1,
  "capability": "code",
  "goal": "修复 TTL 恰好到期时仍被当作有效的 Bug",
  "workdir": "/Users/andy/repo",
  "inputs": ["/Users/andy/notes/bug.md"],
  "images": [],
  "output_dir": "",
  "schema": null,
  "schema_file": "",
  "agent": "",
  "agents": [],
  "models": {},
  "permission": "write",
  "isolate": false,
  "review": null,
  "reviewer": "",
  "verify": ["go test ./..."],
  "max_fix_rounds": null,
  "fallback": null,
  "timeout_sec": 0,
  "plan": null,
  "plan_only": false,
  "parent_task": "",
  "metadata": {"app": "contribution-os"}
}
```

| 字段 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `version` | int | 否 | 1 | 协议版本；不是 1 时返回 invalid |
| `capability` | string | 否 | `auto` | `auto` / `ask` / `code` / `review` / `image` |
| `goal` | string | 是 | — | 需求描述 |
| `workdir` | string | `code` / `review` 必填 | — | 绝对路径或相对 cwd 的路径，必须存在 |
| `inputs` | string[] | 否 | [] | 输入文件/目录，复制快照后提供给 CLI |
| `images` | string[] | 否 | [] | 参考图（图片理解或改图） |
| `output_dir` | string | 否 | 任务目录下的 `out/` | 产物目录，不存在时创建 |
| `schema` | object | 否 | null | 内联 JSON Schema（只对 ask 生效） |
| `schema_file` | string | 否 | "" | Schema 文件路径；与 `schema` 二选一 |
| `agent` | string | 否 | "" | 锁定单个 CLI |
| `agents` | string[] | 否 | 全部启用的 CLI | 允许的 CLI 与偏好顺序 |
| `models` | map | 否 | {} | agent → 模型名 |
| `permission` | string | 否 | `write` | `read` / `write` / `full` |
| `isolate` | bool | 否 | false | 代码改动放在 git worktree 中 |
| `review` | bool\|null | 否 | null | null 表示按配置（code 默认审查） |
| `reviewer` | string | 否 | "" | 指定审查者 |
| `verify` | string[] | 否 | [] | shell 验证命令 |
| `max_fix_rounds` | int\|null | 否 | 配置值（2） | 最大修复轮数 |
| `fallback` | bool\|null | 否 | 配置值（true） | 自动切换 CLI |
| `timeout_sec` | int | 否 | 配置值（5400） | 任务总超时（秒） |
| `plan` | Plan\|null | 否 | null | 显式计划，跳过规划 |
| `plan_only` | bool | 否 | false | 只规划 |
| `parent_task` | string | 否 | "" | `resume` 时由 xcli 填写 |
| `metadata` | map | 否 | {} | 调用方自定义字段，原样写回 Result |

接入应用通过 `metadata.app` 填写稳定的应用名称，例如 `worklog`、`gardenflow`、`chat-insight`。统计时去掉首尾空白并转为小写，归组使用规范化后匹配 `^[a-z0-9][a-z0-9._-]{0,39}$` 的名称，缺失或格式无效的值归为 `unknown`。`do`、`plan`、`ask`、`code`、`review`、`image` 可通过 `--app` 设置，默认使用 `terminal`。`submit` 和 HTTP 从请求中读取应用名称。`resume` 继承父任务的名称，可用 `--app` 覆盖。无效的 `--app` 返回 `invalid`（退出码 2）。

Plan 与 Step：

```json
{
  "source": "planner",
  "reasoning": "文案和配图互不依赖，可以并行",
  "steps": [
    {"id": "copy", "kind": "ask", "agent": "antigravity", "instruction": "写一段 README 介绍", "depends_on": [], "review_by": null, "output": null},
    {"id": "cover", "kind": "image", "agent": "auto", "instruction": "生成 16:9 封面图", "depends_on": [], "review_by": null, "output": "cover"},
    {"id": "apply", "kind": "code", "agent": "codex", "instruction": "把 copy 的文案写入 README，并引用 cover 的图片", "depends_on": ["copy", "cover"], "review_by": "auto", "output": null}
  ]
}
```

CLI 规划成功时，结果中的可选字段 `plan.planner` 记录完成规划的 CLI 名称。

## 3. Result

```json
{
  "version": 1,
  "task_id": "t-20261113-170102-a3f9",
  "status": "completed",
  "capability": "code",
  "summary": "修复了 cache.go 中的边界判断，并补充了 2 个测试",
  "output": null,
  "artifacts": [
    {"path": "/Users/andy/.xcli/tasks/t-.../out/cover/cover-1.png", "kind": "image", "mime": "image/png", "width": 1254, "height": 1254, "bytes": 1832211, "sha256": "…", "step_id": "cover", "agent": "codex"}
  ],
  "changes": {
    "workdir": "/Users/andy/repo",
    "isolation": "none",
    "branch": "",
    "diff_file": "/Users/andy/.xcli/tasks/t-.../changes.patch",
    "files": ["cache.go", "cache_test.go"]
  },
  "verify": [
    {"command": "go test ./...", "round": 1, "exit_code": 1, "passed": false, "log_file": "…"},
    {"command": "go test ./...", "round": 2, "exit_code": 0, "passed": true, "log_file": "…"}
  ],
  "review": {"agent": "cursor", "round": 2, "verdict": "pass", "summary": "…", "issues": []},
  "plan": {"source": "rules", "reasoning": "", "steps": []},
  "steps": [
    {"id": "main", "kind": "code", "agent": "codex", "status": "completed", "rounds": 2, "session_id": "…", "summary": "…", "fallbacks": [], "duration_ms": 182000, "error": null}
  ],
  "error": null,
  "metadata": {"app": "contribution-os"},
  "started_at": "2026-11-13T09:01:02Z",
  "finished_at": "2026-11-13T09:04:30Z",
  "duration_ms": 208000
}
```

| 字段 | 说明 |
| --- | --- |
| `status` | `completed` / `needs_attention` / `failed` / `cancelled` / `planned` / `invalid` |
| `summary` | 面向用户的最终说明。单步任务是该步的最终文本；多步任务是最后一个（没有下游的）步骤的文本，多个末端步骤时按计划顺序拼接 |
| `output` | ask 带 Schema 时是校验通过的 JSON；否则为 null |
| `artifacts` | 所有步骤产出的有效图片，按步骤顺序排列 |
| `changes` | 有 code 步骤时出现；`isolation` 为 `none` 或 `worktree` |
| `verify` | 每一轮每条验证命令的结果 |
| `review` | 最后一次审查的结论（每轮审查都记录在事件和步骤目录里） |
| `steps[].fallbacks` | 切换记录，例如 `["codex: quota"]` |
| `error` | `{"kind": "...", "message": "...", "step_id": "..."}`；kind 取值见架构文档 4.4，另有 `invalid_request`、`worker_lost` |

固定的 CLI 不可用时，任务状态为 `failed`，`error.kind` 为冷却原因（`auth`、`quota`、`unavailable`）或 `not_installed`。`message` 提供重试时间或安装指引。计划结构错误仍使用 `invalid_request`。

## 4. 事件（JSONL）

每行一个对象：

```json
{"v":1,"ts":"2026-11-13T09:01:05.120Z","task":"t-20261113-170102-a3f9","seq":7,"type":"step.progress","step":"main","agent":"codex","data":{"text":"正在运行 go test"}}
```

| type | data |
| --- | --- |
| `task.started` | `{"capability", "goal"}` |
| `plan.ready` | `{"plan"}` |
| `step.waiting` | `{"reason"}`，例如等待并发槽位 |
| `step.started` | `{"kind", "round"}` |
| `step.progress` | `{"text"}` 或 `{"tool"}` |
| `step.finished` | `{"status", "summary", "error"}` |
| `verify.result` | `{"command", "round", "passed", "exit_code"}` |
| `review.result` | `{"round", "verdict", "issues"}` |
| `artifact` | Artifact 对象 |
| `agent.fallback` | `{"from", "to", "reason"}` |
| `notice` | `{"text"}` |
| `task.finished` | 完整 Result |

`seq` 从 1 开始递增；`xcli logs <id> --after N` 只输出 `seq > N` 的事件，便于断线续读。

## 5. HTTP 任务接口（第 5 阶段，可选）

`xcli serve` 默认监听 `127.0.0.1:7788`。首次启动时生成 `~/.xcli/server.token`（权限 0600）；所有请求都要带 `Authorization: Bearer <token>`。

| 方法与路径 | 说明 |
| --- | --- |
| `GET /v1/health` | `{"ok": true, "version": "2.1.0"}`，不需要令牌 |
| `GET /v1/agents` | 同 `xcli agents --json` |
| `POST /v1/tasks` | 请求体是 Request；`?wait=true` 时阻塞到结束并返回 Result，否则返回 `202 {"task_id"}` |
| `GET /v1/tasks?limit=20` | 任务列表 |
| `GET /v1/tasks/{id}` | Result；未结束时 status 为 `running`，含当前步骤状态 |
| `GET /v1/tasks/{id}/events?after=N` | SSE：`event: <type>`、`id: <seq>`、`data: <事件 JSON>`；任务结束后发出 `task.finished` 并关闭连接 |
| `POST /v1/tasks/{id}/cancel` | 取消 |
| `GET /v1/tasks/{id}/artifacts/{index}` | 下载第 index 个产物（只允许 Result 中列出的文件） |

HTTP 接口与命令行共用 engine，任务由 serve 进程内的 goroutine 运行（不另起 worker）。通过创建任务的服务调用 `POST /v1/tasks/{id}/cancel`，只取消该任务；`xcli cancel` 对 HTTP 任务返回这个接口的使用指引。serve 退出时取消全部服务内任务，等待 `cancelled` 结果和 `task.finished` 落盘。

Docker 中的调用方：macOS 上的 Docker Desktop / OrbStack 可以通过 `http://host.docker.internal:7788` 访问宿主机 loopback。请求里的 `workdir`、`inputs` 必须是**宿主机路径**，由调用方自己把容器路径转换成宿主机路径。

## 6. 四个应用的接入

### 6.1 场景映射

| 应用 | 场景 | 请求要点 |
| --- | --- | --- |
| GardenFlow | 文章生成、改写 | `capability: ask`，`inputs` 放素材；需要结构化时给 `schema` |
| GardenFlow | 封面生图 | `capability: image`，`output_dir` 指向应用的临时媒体目录 |
| GardenFlow | 参考图编辑 | `capability: image`，`images: [原图]`；原图不会被覆盖 |
| GardenFlow | 图片理解 | `capability: ask`，`images: [图片]` |
| GardenFlow | 继续修改上一张图 | `xcli resume <task-id> "把花盆改成蓝色"` |
| Worklog | 日报、周报、模板填充 | `capability: ask`，`inputs: [导出的工作项 JSON]`，`schema` 为报告结构 |
| Chat Insight | 选定会话的总结、分析 | `capability: ask`，`inputs: [导出的会话片段]`；Embedding 不支持，继续用现有向量服务 |
| Contribution OS | 生成实施计划 | `capability: ask`，`workdir: 仓库`，`schema` 为计划结构（只读） |
| Contribution OS | 实现 + 审查 | `capability: code`，`isolate: true`，`verify: ["pytest -q"]` |
| Contribution OS | 审查现有改动 | `capability: review`，`workdir: 仓库` |

CLI 任务耗时通常在 10 秒到几分钟之间，应用应该把它当作异步任务处理：显示进度（`step.progress`），允许取消，不要当作即时模型调用。

### 6.2 Electron（Node，主进程）

```ts
import { spawn } from "node:child_process";
import readline from "node:readline";

const XCLI = "/Users/andy/.local/bin/xcli"; // 用绝对路径，GUI 应用的 PATH 不可靠

export function runXcli(request: object, onEvent: (e: any) => void): { promise: Promise<any>; cancel: () => void } {
  const child = spawn(XCLI, ["submit", "--request", "-", "--events"], { stdio: ["pipe", "pipe", "pipe"] });
  const promise = new Promise((resolve, reject) => {
    let result: any = null;
    readline.createInterface({ input: child.stdout }).on("line", (line) => {
      const event = JSON.parse(line);
      onEvent(event);
      if (event.type === "task.finished") result = event.data;
    });
    child.on("close", (code) => (result ? resolve(result) : reject(new Error(`xcli exited with ${code}`))));
  });
  child.stdin.end(JSON.stringify(request));
  return { promise, cancel: () => child.kill("SIGTERM") }; // xcli 收到 SIGTERM 会取消任务并结束 CLI 子进程
}

// 用法：封面生图
const { promise } = runXcli(
  { capability: "image", goal: "米白背景、绿色植物的方形插画，无文字", output_dir: "/tmp/gardenflow-media" },
  (e) => console.log(e.type, e.data),
);
const result = await promise; // result.artifacts[0].path / mime / width / height / sha256
```

### 6.3 Tauri（Rust）

```rust
use std::io::Write;
use std::process::{Command, Stdio};

pub fn run_xcli(xcli: &str, request: &serde_json::Value) -> anyhow::Result<serde_json::Value> {
    let mut child = Command::new(xcli)
        .args(["submit", "--request", "-", "--json"])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()?;
    child.stdin.take().unwrap().write_all(request.to_string().as_bytes())?; // 写完后 drop，关闭 stdin
    let output = child.wait_with_output()?;
    Ok(serde_json::from_slice(&output.stdout)?) // 失败时也是 Result JSON，看 status 字段
}

// Worklog 周报：
// run_xcli(XCLI, &json!({
//   "capability": "ask",
//   "goal": "根据工作项生成本周周报：完成事项、阻碍、下周计划",
//   "inputs": ["/tmp/worklog-export/week-46.json"],
//   "schema": {"type":"object","additionalProperties":false,"required":["summary","items","blockers","next"],
//              "properties":{"summary":{"type":"string"},"items":{"type":"array","items":{"type":"string"}},
//                            "blockers":{"type":"array","items":{"type":"string"}},"next":{"type":"array","items":{"type":"string"}}}}
// }))?["output"]
```

需要进度时改用 `--events`，逐行读取 stdout（`BufReader::lines()`），在异步任务里处理。

### 6.4 Python

```python
import json
import subprocess

XCLI = "/Users/andy/.local/bin/xcli"

def run_xcli(request: dict, timeout: int = 3600) -> dict:
    proc = subprocess.run(
        [XCLI, "submit", "--request", "-", "--json"],
        input=json.dumps(request), capture_output=True, text=True, timeout=timeout,
    )
    return json.loads(proc.stdout)

result = run_xcli({
    "capability": "code",
    "goal": "修复 issue #42：分页参数为 0 时崩溃",
    "workdir": "/Users/andy/src/project",
    "isolate": True,
    "verify": ["pytest -q"],
    "metadata": {"app": "contribution-os", "issue": "42"},
})
print(result["status"], result["changes"]["diff_file"], result["review"])
```

长任务建议 `--detach` 拿到 `task_id`，再用 `xcli wait <id> --json` 或轮询 `xcli status <id>`，避免调用方进程退出导致任务中断。

## 7. 兼容性承诺

- Request / Result / Event 带版本号 `1`。v1 内只新增可选字段，不删除、不改名、不改语义。
- 调用方必须忽略不认识的字段和事件类型。
- 命令行参数在 2.x 内保持兼容；人类可读的输出格式不做承诺，程序请使用 `--json` / `--events`。

## 8. 统计账本

任务结束后，xcli 向数据目录的 `stats/usage.jsonl` 追加一条记录。`xcli stats` 只读汇总账本和可用的历史任务，按任务 ID 去重。账本字段、权限和写入时机见 [接入应用统计第 2.1 节](05-app-stats.md#21-统计账本)。
