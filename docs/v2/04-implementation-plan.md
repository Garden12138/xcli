# xcli v2 实施计划

> **给执行者（Codex）：** 按任务顺序逐个实现，每个任务结束时必须满足"验收"一节并单独提交。步骤用 `- [ ]` 勾选跟踪。设计依据是 [02 架构设计](02-architecture.md) 和 [03 接口规范](03-interfaces.md)；两者与本计划冲突时以设计文档为准，并在提交说明里写明。

**目标：** 在 `v2` 分支上重建 xcli，使它以真实 CLI 进程的方式，把 Codex / Cursor / Antigravity 的编码、文本、审查、生图能力通过子进程 JSON 协议（以及可选的本机 HTTP）提供给桌面应用，并能按需求自动编排多个 CLI。

**架构：** 驱动层统一三个 CLI 的参数和事件流；编排层用"规则 / CLI 规划 → 计划校验 → DAG 执行（含实现-验证-审查-修复循环与失败切换）"完成任务；engine 负责任务持久化、前台/后台运行、状态与取消；cmd 和 server 都是 engine 的薄封装。

**技术栈：** Go 1.26、cobra、yaml.v3、golang.org/x/image/webp、github.com/santhosh-tekuri/jsonschema/v6、标准库 net/http。

## 全局约束

- 模块路径 `github.com/Garden12138/xcli`；`go.mod` 保持 `go 1.26.0`。
- 版本号 `2.0.0-dev`，定义在 `cmd/root.go` 的 `var Version`。
- 只允许以下第三方依赖：`github.com/spf13/cobra`、`gopkg.in/yaml.v3`、`golang.org/x/image`、`github.com/santhosh-tekuri/jsonschema/v6`。需要新增依赖时先停下来问用户。
- 只支持 macOS 和 Linux。需要 `syscall` 的文件加 `//go:build unix`；不写 Windows 实现。
- 单元测试不得启动真实的 codex / cursor-agent / agy：测试里一律通过配置 `path` 字段把 PATH 指向 fakecli 目录加 `/bin:/usr/bin`。
- 不得加入任何启动窗口、启动账本、预算预留、授权回执、外部启动计数之类的门禁。
- 面向用户的文案（错误信息、进度、摘要）用简体中文；代码标识符、JSON 字段用英文。
- 非测试 Go 代码目标 ≤ 6000 行；超过 8000 行必须停下来，向用户说明原因。统计命令：`git ls-files '*.go' | grep -v _test.go | xargs wc -l | tail -1`。
- 每个任务结束前运行 `go build ./... && go vet ./... && go test ./... -race`，全部通过才能提交。
- 提交信息使用 conventional commits（`feat:`、`fix:`、`test:`、`docs:`、`chore:`、`refactor:`）。
- 不 push，除非用户明确要求。

## 文件结构

```
main.go                                  入口，调用 cmd.Execute()
cmd/
  root.go                                根命令、版本、全局 --home
  request_flags.go                       do/plan/ask/code/review/image 共享参数 → api.Request
  run.go                                 do / plan / ask / code / review / image / submit
  tasks.go                               status / wait / logs / result / cancel / list / resume / apply / gc
  agents.go                              agents
  config.go                              config show / init / path
  worker.go                              隐藏命令 __worker
  serve.go                               serve（任务 18）
  output.go                              --json / --events / 人类可读输出
internal/api/types.go                    协议 v1 类型
internal/config/config.go                配置
internal/store/store.go                  任务存储
internal/store/events.go                 事件读写与跟随
internal/proc/run.go                     子进程运行
internal/proc/slot.go                    并发槽位
internal/proc/path.go                    PATH 修复
internal/driver/driver.go                接口、错误类型、注册表
internal/driver/classify.go              通用错误分类
internal/driver/codex.go                 codex 参数与解析
internal/driver/cursor.go                cursor 参数与解析
internal/driver/antigravity.go           antigravity 参数与解析
internal/driver/testdata/<agent>/*.jsonl 真实录制样本
internal/health/health.go                健康缓存与冷却
internal/recipe/recipe.go                渲染、JSON 提取、Schema 校验
internal/recipe/schemas.go               内置 Schema
internal/recipe/prompts/*.md             提示词模板
internal/workspace/workspace.go          工作目录、输入快照、worktree
internal/workspace/changes.go            变更检测
internal/workspace/verify.go             验证命令、apply
internal/artifact/artifact.go            图片收集与校验
internal/orchestrator/env.go             Env、invoke（调用驱动的唯一入口）
internal/orchestrator/step.go            RunStep：ask / image / review / code 循环
internal/orchestrator/router.go          Router
internal/orchestrator/plan.go            RulesPlan、ValidatePlan、KeywordCapability
internal/orchestrator/planner.go         CLI 规划
internal/orchestrator/execute.go         DAG 执行器、失败切换、结果汇总
internal/engine/engine.go                Engine
internal/engine/normalize.go             请求规范化与校验
internal/engine/lifecycle.go             Detach / Status / Wait / Cancel / Resume / GC
internal/server/server.go                HTTP 接口
internal/testutil/fakecli/main.go        假 CLI 程序
internal/testutil/fakecli.go             构建与安装假 CLI 的测试辅助函数
scripts/probe/main.go                    第 0 阶段真实 CLI 实测程序
scripts/smoke.sh                         真实冒烟（用户手动运行）
docs/v2/driver-notes.md                  实测记录（任务 2 产出）
docs/v2/USER_GUIDE.md                    一页用户手册（任务 19 产出）
```

---

## 第 0 阶段：清场与实测

### 任务 1：清场与骨架

**文件：**
- 删除：除下列保留项之外的所有已跟踪文件
- 保留：`.gitattributes`、`.gitignore`、`.github/`、`.goreleaser.yaml`、`LICENSE`、`go.mod`、`go.sum`、`main.go`、`docs/v2/`
- 新建：`cmd/root.go`
- 修改：`README.md`、`README.zh-CN.md`（重写为简短的 v2 说明）、`.goreleaser.yaml`、`.github/workflows/*`

**接口：**
- 产出：`cmd.Execute() error`；`cmd.Version = "2.0.0-dev"`；根命令的全局参数 `--home`（默认取 `XCLI_HOME`，再默认 `~/.xcli`），供后续命令读取 `rootOpts.home`。

- [x] **步骤 1：确认基线**

```bash
git branch --show-current          # 期望输出 v2
git tag -l 'archive/*'             # 期望包含 archive/r4.1-final
```

- [x] **步骤 2：删除旧代码与旧文档**

```bash
git rm -r -q cmd internal tests examples xcli-cli-first-acceptance bin 2>/dev/null || true
git ls-files docs | grep -v '^docs/v2/' | xargs git rm -q
git status --short | head
```

- [x] **步骤 3：写最小根命令**

`cmd/root.go`：

```go
package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var Version = "2.0.0-dev"

type rootOptions struct {
	home string
}

var rootOpts rootOptions

func defaultHome() string {
	if v := os.Getenv("XCLI_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".xcli"
	}
	return filepath.Join(home, ".xcli")
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "xcli",
		Short:         "用真实的 Codex / Cursor / Antigravity CLI 完成任务，并自动编排多个 CLI",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&rootOpts.home, "home", defaultHome(), "xcli 数据目录")
	return root
}

func Execute() error {
	return newRootCommand().Execute()
}
```

`main.go` 保持调用 `cmd.Execute()`，出错时打印到 stderr 并 `os.Exit(1)`。

- [x] **步骤 4：整理依赖与发布配置**

```bash
go mod tidy
```

`.goreleaser.yaml` 的 `goos` 只保留 `darwin`、`linux`，`goarch` 为 `amd64`、`arm64`；ldflags 写入 `-X github.com/Garden12138/xcli/cmd.Version={{.Version}}`。检查 `.github/workflows` 中的 Go 版本为 1.26，去掉对已删除脚本的引用，只保留 `go vet ./...` 和 `go test ./... -race`。

- [x] **步骤 5：重写 README**

`README.md` / `README.zh-CN.md` 各不超过 40 行：一句话定位、安装（`go build -o bin/xcli .`）、四个示例命令（同 `docs/v2/README.md`）、指向 `docs/v2/`、说明 v0.x 的成果在 tag `archive/r4.1-final`。

- [x] **步骤 6：验证并提交**

```bash
go build ./... && go vet ./... && go test ./... -race
go run . --version                 # 期望：xcli version 2.0.0-dev
git add -A && git commit -m "chore: reset v2 branch to minimal skeleton"
```

**验收：** 仓库只剩保留项和新骨架；`go run . --version` 输出 `2.0.0-dev`。

---

### 任务 2：真实 CLI 实测与样本录制

这是主要的真实 CLI 实测任务，约 21 次调用（步骤 2 约 18 次，步骤 3 约 3 次），会消耗用户额度。**运行 `scripts/probe` 之前必须先征得用户同意。** 用户不同意时跳到步骤 5 的兜底方案。

**文件：**
- 新建：`scripts/probe/main.go`
- 新建：`internal/driver/testdata/{codex,cursor,antigravity}/*.jsonl` 与同名 `.stderr`
- 新建：`docs/v2/driver-notes.md`

**接口：**
- 产出：每个 CLI 的样本文件，供任务 7、8 的解析测试使用，文件名固定为：`ask_ok`、`schema_ok`、`resume_ok`、`write_ok`、`image_ok`、`error_bad_model`（后缀 `.jsonl` 为 stdout，`.stderr` 为 stderr）。

- [x] **步骤 1：写实测程序**

`scripts/probe/main.go`（`package main`，不超过 300 行）。用法：`go run ./scripts/probe -agent codex -out internal/driver/testdata`。逻辑：

1. 在 `os.MkdirTemp` 下创建 git 仓库 `repo/`，写入 `add.go`（`package demo\n\nfunc Add(a, b int) int { return a + b }\n`）并提交；创建 `out/` 目录和 `schema.json`：
   `{"type":"object","additionalProperties":false,"required":["answer"],"properties":{"answer":{"type":"string"}}}`。
2. 按 `docs/v2/02-architecture.md` 4.2 节的映射，硬编码每个 CLI 6 个用例的 argv（这里不依赖 `internal/driver`，因为它还没写）：

| 用例 | 权限 | 提示词 |
| --- | --- | --- |
| `ask_ok` | read | `用一句话说明 add.go 中 Add 函数的作用。` |
| `schema_ok` | read | `用一句话说明 add.go 的作用，按 JSON Schema 输出。`（cursor 没有 Schema 参数，在提示词末尾附上 Schema 并要求只输出 JSON） |
| `resume_ok` | read | 续接 `ask_ok` 的会话：`我上一个问题问的是哪个函数？只回答函数名。` |
| `write_ok` | write | `在当前目录创建 hello.txt，内容为 hi，不要做其他修改。` |
| `image_ok` | write | 按 02 架构文档第 11 节的生图约定：使用该 CLI 的内置生图能力，生成一张"米白背景上的绿色植物插画，方形，无文字"，保存到 `<out>/image_ok-1.<ext>`，最终回复 `{"files":["<绝对路径>"]}` |
| `error_bad_model` | read | 与 `ask_ok` 相同，但加上模型参数 `nonexistent-model-xyz` |

3. 每个用例记录：argv、耗时、退出码，stdout 写入 `<agent>/<case>.jsonl`，stderr 写入 `<agent>/<case>.stderr`。写入前把字符串中的 `$HOME` 替换成 `/HOME`、临时目录及其规范路径替换成 `/TMP`，原生 `CODEX_HOME` 替换成 `/CODEX_HOME`。
4. 会话 ID 从 stdout 中按 02 架构文档 4.3 节的字段提取（codex `thread_id`、cursor `session_id`、antigravity `conversation_id`），用于 `resume_ok`。
5. `write_ok` 结束后检查 `repo/hello.txt` 是否存在；`image_ok` 结束后列出 `out/` 里的新文件，并在 `$HOME/.codex`、`$HOME/.gemini`、`$HOME/.cursor` 下查找 10 分钟内新建的图片文件（最多 3 层），把找到的路径打印出来。
6. 每个用例超时 10 分钟。CLI 未安装时跳过并提示。

- [x] **步骤 2：征得用户同意后运行**

```bash
go run ./scripts/probe -agent codex -out internal/driver/testdata
go run ./scripts/probe -agent cursor -out internal/driver/testdata
go run ./scripts/probe -agent antigravity -out internal/driver/testdata
```

- [x] **步骤 3：追加核实三个参数细节**（每项 1 次调用，同样需要同意）
  - `codex exec resume ... -- <sid> <prompt>` 中的 `--` 能否被正确识别；
  - `cursor-agent` 位置参数前加 `--` 是否可用；
  - `agy --print-timeout 0` 是否表示不限时（`agy --help` 中写的是 "0 waits until the turn completes"，确认即可）。

2026-11-16：Codex 的 `resume_ok` 已核实 `--`；另调用一次 Cursor 核实 `--`。Antigravity 的 help 已确认 timeout 语义，另调用一次 `--mode plan --sandbox` 核实只读适配问题。本轮共 20 次模型调用；行为差异详见 `driver-notes.md`。

用户另行批准的六次 Antigravity 调用已完成：默认执行模式加会话级只读 settings，直接回答、Schema、续接、规划 JSON 均成功，原生写文件和终端命令被显式 deny 规则阻止。当前 `ask_ok/schema_ok/resume_ok` 使用这组新样本；追加 `error_plan_permission`、`error_write_denied`、`error_command_denied`、`plan_ok`（均含 `.jsonl/.stderr`）。只读参数、提示词和解析期望按 02 第 4.2/4.3 节更新；流式 `text_delta` 中的临时路径片段也要脱敏。

- [x] **步骤 4：写 `docs/v2/driver-notes.md`**

每个 CLI 一节，包含：版本号；6 个用例实际使用的 argv；观察到的事件 `type` 列表；会话 ID、最终文本、用量分别在哪个字段；错误输出的格式（`error_bad_model` 的 stdout / stderr 关键行）；write 模式是否出现 `denied_actions` 或审批卡住；生成图片实际落在哪里（据此填写配置默认值 `artifact_dirs`）；需要修改 02 架构文档 4.2 节参数映射的地方。若映射需要改，同步修改 02 架构文档。

- [ ] **步骤 5：兜底方案（用户不同意实测时）**

按 02 架构文档 4.3 节的格式手写最小样本，文件名加前缀 `synthetic-`（例如 `synthetic-ask_ok.jsonl`），并在 `driver-notes.md` 开头注明"样本为合成数据，未经实测"。任务 7、8 的测试在找不到真实样本时改读 `synthetic-` 版本。

codex 合成样本示例 `synthetic-ask_ok.jsonl`：

```
{"type":"thread.started","thread_id":"019a-synthetic"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"Add 返回两个整数之和。"}}
{"type":"turn.completed","usage":{"input_tokens":120,"cached_input_tokens":0,"output_tokens":12}}
```

cursor 合成样本示例：

```
{"type":"system","subtype":"init","session_id":"c-synthetic","model":"auto"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Add 返回两个整数之和。"}]},"session_id":"c-synthetic"}
{"type":"result","subtype":"success","is_error":false,"result":"Add 返回两个整数之和。","session_id":"c-synthetic"}
```

antigravity 合成样本示例：

```
{"event":"step_update","step_update":{"conversation_id":"a-synthetic"}}
{"event":"result","result":{"conversation_id":"a-synthetic","status":"SUCCESS","response":"Add 返回两个整数之和。","usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110}}}
```

- [x] **步骤 6：提交**

```bash
git add scripts/probe internal/driver/testdata docs/v2
git commit -m "test: record native CLI fixtures and driver notes"
```

**验收：** 三个 CLI 各有 6 个样本（或合成样本）；`driver-notes.md` 写明了参数映射是否需要调整以及图片落盘位置。

---

## 第 1 阶段：基础

### 任务 3：协议类型 `internal/api`

**文件：**
- 新建：`internal/api/types.go`
- 测试：`internal/api/types_test.go`

**接口：**
- 产出：下面代码中的全部类型、常量和 `NewEvent`、`(Request).Validate`。后续所有任务都使用这些名称。

- [x] **步骤 1：写类型**

```go
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const Version = 1

type Capability string

const (
	CapAuto   Capability = "auto"
	CapAsk    Capability = "ask"
	CapCode   Capability = "code"
	CapReview Capability = "review"
	CapImage  Capability = "image"
)

type Kind string

const (
	KindAsk    Kind = "ask"
	KindCode   Kind = "code"
	KindReview Kind = "review"
	KindImage  Kind = "image"
)

type Permission string

const (
	PermRead  Permission = "read"
	PermWrite Permission = "write"
	PermFull  Permission = "full"
)

type Status string

const (
	StatusQueued         Status = "queued"
	StatusRunning        Status = "running"
	StatusCompleted      Status = "completed"
	StatusNeedsAttention Status = "needs_attention"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
	StatusPlanned        Status = "planned"
	StatusInvalid        Status = "invalid"
	StatusPending        Status = "pending"
	StatusWaiting        Status = "waiting"
	StatusSkipped        Status = "skipped"
)

func (s Status) Finished() bool {
	switch s {
	case StatusCompleted, StatusNeedsAttention, StatusFailed, StatusCancelled, StatusPlanned, StatusInvalid:
		return true
	}
	return false
}

type Request struct {
	Version      int               `json:"version,omitempty"`
	Capability   Capability        `json:"capability,omitempty"`
	Goal         string            `json:"goal"`
	Workdir      string            `json:"workdir,omitempty"`
	Inputs       []string          `json:"inputs,omitempty"`
	Images       []string          `json:"images,omitempty"`
	OutputDir    string            `json:"output_dir,omitempty"`
	Schema       json.RawMessage   `json:"schema,omitempty"`
	SchemaFile   string            `json:"schema_file,omitempty"`
	Agent        string            `json:"agent,omitempty"`
	Agents       []string          `json:"agents,omitempty"`
	Models       map[string]string `json:"models,omitempty"`
	Permission   Permission        `json:"permission,omitempty"`
	Isolate      bool              `json:"isolate,omitempty"`
	Review       *bool             `json:"review,omitempty"`
	Reviewer     string            `json:"reviewer,omitempty"`
	Verify       []string          `json:"verify,omitempty"`
	MaxFixRounds *int              `json:"max_fix_rounds,omitempty"`
	Fallback     *bool             `json:"fallback,omitempty"`
	TimeoutSec   int               `json:"timeout_sec,omitempty"`
	Plan         *Plan             `json:"plan,omitempty"`
	PlanOnly     bool              `json:"plan_only,omitempty"`
	ParentTask   string            `json:"parent_task,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// HasSchema 把 JSON 里的 "schema": null 视为没有 Schema。
func (r Request) HasSchema() bool {
	s := strings.TrimSpace(string(r.Schema))
	return s != "" && s != "null"
}

// Validate 只做结构校验；路径、agent 名称、默认值由 engine.Normalize 处理。
func (r Request) Validate() error {
	if r.Version != 0 && r.Version != Version {
		return fmt.Errorf("不支持的协议版本 %d，当前版本为 %d", r.Version, Version)
	}
	if strings.TrimSpace(r.Goal) == "" {
		return errors.New("goal 不能为空")
	}
	switch r.Capability {
	case "", CapAuto, CapAsk, CapCode, CapReview, CapImage:
	default:
		return fmt.Errorf("未知的 capability %q，可选 auto/ask/code/review/image", r.Capability)
	}
	switch r.Permission {
	case "", PermRead, PermWrite, PermFull:
	default:
		return fmt.Errorf("未知的 permission %q，可选 read/write/full", r.Permission)
	}
	if r.HasSchema() && r.SchemaFile != "" {
		return errors.New("schema 与 schema_file 只能二选一")
	}
	if r.MaxFixRounds != nil && *r.MaxFixRounds < 0 {
		return errors.New("max_fix_rounds 不能为负数")
	}
	if r.TimeoutSec < 0 {
		return errors.New("timeout_sec 不能为负数")
	}
	return nil
}

type Plan struct {
	Source    string `json:"source"`
	Reasoning string `json:"reasoning"`
	Steps     []Step `json:"steps"`
}

type Step struct {
	ID          string   `json:"id"`
	Kind        Kind     `json:"kind"`
	Agent       string   `json:"agent"`
	Instruction string   `json:"instruction"`
	DependsOn   []string `json:"depends_on"`
	ReviewBy    *string  `json:"review_by"`
	Output      *string  `json:"output"`
}

type Usage struct {
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
	TotalTokens  int64 `json:"total_tokens,omitempty"`
}

type Artifact struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	MIME   string `json:"mime"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	StepID string `json:"step_id,omitempty"`
	Agent  string `json:"agent,omitempty"`
}

type Changes struct {
	Workdir   string   `json:"workdir"`
	Isolation string   `json:"isolation"`
	Branch    string   `json:"branch,omitempty"`
	DiffFile  string   `json:"diff_file,omitempty"`
	Files     []string `json:"files"`
}

type VerifyResult struct {
	Command  string `json:"command"`
	Round    int    `json:"round"`
	ExitCode int    `json:"exit_code"`
	Passed   bool   `json:"passed"`
	LogFile  string `json:"log_file"`
}

type Issue struct {
	Severity    string  `json:"severity"`
	File        *string `json:"file"`
	Description string  `json:"description"`
}

type ReviewResult struct {
	Agent   string  `json:"agent"`
	Round   int     `json:"round"`
	Verdict string  `json:"verdict"`
	Summary string  `json:"summary"`
	Issues  []Issue `json:"issues"`
}

type Error struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
	StepID  string `json:"step_id,omitempty"`
}

type StepResult struct {
	ID         string   `json:"id"`
	Kind       Kind     `json:"kind"`
	Agent      string   `json:"agent"`
	Status     Status   `json:"status"`
	Rounds     int      `json:"rounds,omitempty"`
	SessionID  string   `json:"session_id,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	Fallbacks  []string `json:"fallbacks"`
	Usage      *Usage   `json:"usage,omitempty"`
	DurationMS int64    `json:"duration_ms"`
	Error      *Error   `json:"error"`
}

type Result struct {
	Version    int               `json:"version"`
	TaskID     string            `json:"task_id"`
	Status     Status            `json:"status"`
	Capability Capability        `json:"capability"`
	Summary    string            `json:"summary"`
	Output     json.RawMessage   `json:"output"`
	Artifacts  []Artifact        `json:"artifacts"`
	Changes    *Changes          `json:"changes,omitempty"`
	Verify     []VerifyResult    `json:"verify,omitempty"`
	Review     *ReviewResult     `json:"review,omitempty"`
	Plan       *Plan             `json:"plan,omitempty"`
	Steps      []StepResult      `json:"steps"`
	Error      *Error            `json:"error"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	DurationMS int64             `json:"duration_ms"`
}

type Event struct {
	V     int             `json:"v"`
	TS    time.Time       `json:"ts"`
	Task  string          `json:"task"`
	Seq   int             `json:"seq"`
	Type  string          `json:"type"`
	Step  string          `json:"step,omitempty"`
	Agent string          `json:"agent,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

func NewEvent(task string, seq int, typ, step, agent string, data any) (Event, error) {
	e := Event{V: Version, TS: time.Now().UTC(), Task: task, Seq: seq, Type: typ, Step: step, Agent: agent}
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return Event{}, err
		}
		e.Data = raw
	}
	return e, nil
}
```

- [x] **步骤 2：写测试并运行**

`internal/api/types_test.go` 覆盖：
- `TestRequestValidate`：表驱动，空 goal、`version: 2`、未知 capability、未知 permission、schema 与 schema_file 同时出现、负的 max_fix_rounds 都返回错误；`{"goal":"x"}` 通过。
- `TestHasSchemaNull`：`json.Unmarshal([]byte(`{"goal":"x","schema":null}`), &r)` 后 `r.HasSchema()` 为 false；`{"schema":{"type":"object"}}` 为 true。
- `TestResultJSONShape`：零值 `Result{Version: 1, Artifacts: []Artifact{}, Steps: []StepResult{}}` 序列化后包含 `"output":null`、`"error":null`、`"artifacts":[]`。
- `TestNewEventRoundTrip`：`NewEvent("t1", 3, "notice", "", "", map[string]string{"text": "hi"})` 序列化再反序列化，`Seq == 3`，`Data` 为 `{"text":"hi"}`。

```bash
go test ./internal/api/ -race -v
```

- [x] **步骤 3：提交**

```bash
git add internal/api && git commit -m "feat(api): add protocol v1 types"
```

**验收：** 测试通过；类型与 03 接口规范第 2～4 节一致。

---

### 任务 4：配置 `internal/config`

**文件：**
- 新建：`internal/config/config.go`
- 测试：`internal/config/config_test.go`

**接口：**
- 消费：`api.Permission`。
- 产出：

```go
type Duration time.Duration                    // 实现 yaml.Unmarshaler，解析 "20m" 这类字符串
func (d Duration) D() time.Duration

type Defaults struct {
	Permission     api.Permission `yaml:"permission"`
	Network        bool           `yaml:"network"`
	Review         string         `yaml:"review"` // "auto" | "off"
	Fallback       bool           `yaml:"fallback"`
	MaxFixRounds   int            `yaml:"max_fix_rounds"`
	MaxSteps       int            `yaml:"max_steps"`
	MaxParallel    int            `yaml:"max_parallel"`
	StepTimeout    Duration       `yaml:"step_timeout"`
	TaskTimeout    Duration       `yaml:"task_timeout"`
	PlannerTimeout Duration       `yaml:"planner_timeout"`
	VerifyTimeout  Duration       `yaml:"verify_timeout"`
	Planner        string         `yaml:"planner"`
}

type AgentConfig struct {
	Command        string            `yaml:"command"`
	Enabled        *bool             `yaml:"enabled"`
	Model          string            `yaml:"model"`
	MaxConcurrency int               `yaml:"max_concurrency"`
	Env            map[string]string `yaml:"env"`
	ExtraArgs      []string          `yaml:"extra_args"`
	ArtifactDirs   []string          `yaml:"artifact_dirs"`
	Strengths      map[string]int    `yaml:"strengths"`
}

type Config struct {
	Defaults Defaults               `yaml:"defaults"`
	Path     string                 `yaml:"path"`
	Agents   map[string]AgentConfig `yaml:"agents"`
}

var AgentOrder = []string{"codex", "cursor", "antigravity"}

func Default() Config
func Parse(data []byte) (Config, error)
func Load(home string) (Config, error)          // 读取 <home>/config.yaml；文件不存在时返回 Default()
func (c Config) Enabled(name string) bool       // Enabled 为 nil 视为 true
func (c Config) Strength(name, kind string) int
func Marshal(c Config) ([]byte, error)          // config show / init 使用
```

- [x] **步骤 1：写失败的测试**

```go
func TestDefaultMatchesDesign(t *testing.T) {
	c := Default()
	if c.Defaults.Permission != api.PermWrite || !c.Defaults.Network || !c.Defaults.Fallback {
		t.Fatalf("unexpected defaults: %+v", c.Defaults)
	}
	if c.Defaults.MaxFixRounds != 2 || c.Defaults.MaxSteps != 6 || c.Defaults.MaxParallel != 3 {
		t.Fatalf("unexpected limits: %+v", c.Defaults)
	}
	if c.Defaults.StepTimeout.D() != 20*time.Minute || c.Defaults.TaskTimeout.D() != 90*time.Minute {
		t.Fatalf("unexpected timeouts: %+v", c.Defaults)
	}
	if c.Agents["cursor"].Command != "cursor-agent" || c.Agents["antigravity"].Command != "agy" {
		t.Fatalf("unexpected commands: %+v", c.Agents)
	}
	if c.Strength("antigravity", "image") != 3 || c.Strength("cursor", "review") != 3 {
		t.Fatal("unexpected strengths")
	}
}

func TestParseMergesFieldByField(t *testing.T) {
	c, err := Parse([]byte("defaults:\n  network: false\nagents:\n  codex:\n    model: gpt-5.5\n    strengths: {image: 0}\n  cursor:\n    enabled: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Defaults.Network || c.Defaults.MaxSteps != 6 {
		t.Fatalf("defaults merge wrong: %+v", c.Defaults)
	}
	if c.Agents["codex"].Command != "codex" || c.Agents["codex"].Model != "gpt-5.5" {
		t.Fatalf("agent merge wrong: %+v", c.Agents["codex"])
	}
	if c.Strength("codex", "image") != 0 || c.Strength("codex", "code") != 3 {
		t.Fatal("strengths should merge key by key")
	}
	if c.Enabled("cursor") || !c.Enabled("antigravity") {
		t.Fatal("enabled merge wrong")
	}
}
```

另加：`TestParseRejectsUnknownField`（`defaults: {bogus: 1}` 报错）、`TestParseRejectsUnknownAgent`（`agents: {claude: {}}` 报错，错误信息包含"可选 codex/cursor/antigravity"）、`TestParseRejectsBadPermission`、`TestLoadMissingFileReturnsDefault`（`t.TempDir()` 中没有 config.yaml）。

```bash
go test ./internal/config/ -race     # 期望失败：未定义
```

- [x] **步骤 2：实现**

要点：
- `Default()` 的值严格等于 02 架构文档第 10 节；`max_concurrency` 默认 2；`artifact_dirs` 按 `driver-notes.md` 的结论填写（没有结论就留空）。
- `Parse`：先 `cfg := Default()`，记下 `defaultAgents := cfg.Agents` 后置 `cfg.Agents = nil`；用 `yaml.NewDecoder` + `KnownFields(true)` 解码到 `&cfg`（空文档 `io.EOF` 视为成功），只接受单个 YAML 文档。再把 agents 的 YAML 节点通过 `mergeAgents` 逐字段解码到同名默认 agent 上，保留显式 false、0 和空列表；`Env` 和 `Strengths` 按键合并，列表整体替换。最后 `validate`（permission 合法、review ∈ {auto, off}、数值为正、max_fix_rounds 可为 0、planner 是已知 agent、不允许 AgentOrder 之外的 agent 名）。此处按 02 第 10 节逐字段覆盖规则修正初始“非零字段覆盖”的实现提示。

- [x] **步骤 3：运行测试并提交**

```bash
go test ./internal/config/ -race -v
git add internal/config && git commit -m "feat(config): add zero-config defaults and field-level merge"
```

**验收：** 测试通过；没有配置文件时得到完整默认值。

---

### 任务 5：任务存储 `internal/store`

**文件：**
- 新建：`internal/store/store.go`、`internal/store/events.go`
- 测试：`internal/store/store_test.go`

**接口：**
- 消费：`api.Request`、`api.Plan`、`api.StepResult`、`api.Result`、`api.Event`、`api.NewEvent`。
- 产出：

```go
type Record struct {
	ID        string           `json:"id"`
	Status    api.Status       `json:"status"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	PID       int              `json:"pid,omitempty"`
	Request   api.Request      `json:"request"`
	Plan      *api.Plan        `json:"plan,omitempty"`
	Steps     []api.StepResult `json:"steps,omitempty"`
	Result    *api.Result      `json:"result,omitempty"`
}

type Store struct{ Root string } // <home>/tasks

func Open(home string) (*Store, error)
func NewID(now time.Time) string                         // t-20261113-170102-a3f9
func (s *Store) Create(req api.Request) (*Task, error)   // 建目录，status=queued
func (s *Store) Load(id string) (*Task, error)
func (s *Store) List(limit int) ([]Record, error)        // 按 CreatedAt 倒序
func (s *Store) Remove(id string) error

type Task struct { /* Dir string + 私有字段：mu、rec、seq */ }
func (t *Task) ID() string
func (t *Task) Dir() string
func (t *Task) Snapshot() Record
func (t *Task) Update(fn func(*Record)) error            // 加锁、修改、刷新 UpdatedAt、原子写 task.json
func (t *Task) Emit(typ, step, agent string, data any) (api.Event, error) // seq 自增，追加写 events.jsonl
func (t *Task) Path(parts ...string) string              // filepath.Join(t.Dir(), parts...)
func (t *Task) StepDir(stepID string, round int) (string, error) // steps/<id>-r<round>，自动创建

func ReadEvents(taskDir string, after int) ([]api.Event, error)
func Follow(ctx context.Context, taskDir string, after int, fn func(api.Event) error) error
// Follow 每 300ms 轮询新事件，读到 type=="task.finished" 后返回 nil；ctx 结束返回 ctx.Err()
```

- [x] **步骤 1：写失败的测试**

- `TestCreateLoadRoundTrip`：Create 后 Load，Request 与 Status（queued）一致；目录权限 0700。
- `TestUpdateIsAtomic`：并发 20 个 goroutine 调用 `Update`，每次追加一个 StepResult，结束后 Load 得到 20 个；`task.json` 始终是合法 JSON（同时起一个 goroutine 循环读取并 `json.Unmarshal`，不能出错）。
- `TestEmitSeqAndRead`：Emit 3 次，`ReadEvents(dir, 1)` 返回 seq 2、3；重新 Load 后再 Emit，seq 为 4。
- `TestFollowStopsOnFinished`：后台 goroutine 每 50ms Emit 一次，第 3 次是 `task.finished`；Follow 收到 3 个事件后返回 nil。
- `TestListNewestFirst`、`TestNewIDFormat`（正则 `^t-\d{8}-\d{6}-[0-9a-f]{4}$`）。

- [x] **步骤 2：实现**

要点：`task.json` 先写同目录下的 `.task.json.tmp` 再 `os.Rename`；`events.jsonl` 用 `O_APPEND|O_CREATE|O_WRONLY` 打开，每次写一整行；Load 时 seq 取 events.jsonl 的行数；所有文件权限 0600，目录 0700。

- [x] **步骤 3：运行测试并提交**

```bash
go test ./internal/store/ -race -v
git add internal/store && git commit -m "feat(store): add file-based task store and event log"
```

---

### 任务 6：子进程 `internal/proc`

**文件：**
- 新建：`internal/proc/run.go`、`internal/proc/slot.go`、`internal/proc/path.go`（均 `//go:build unix`）
- 测试：`internal/proc/run_test.go`、`internal/proc/slot_test.go`、`internal/proc/path_test.go`

**接口：**
- 产出：

```go
type Spec struct {
	Path string   // 可执行文件绝对路径
	Args []string
	Dir  string
	Env  []string // nil 表示 os.Environ()
}

type Options struct {
	Timeout    time.Duration    // 0 表示不限
	OnLine     func(line []byte) // 每一行 stdout（不含换行），在同一个 goroutine 中按顺序调用
	StdoutFile string            // 原样保存 stdout；空则不保存
	StderrFile string
}

type Outcome struct {
	ExitCode   int
	TimedOut   bool
	Cancelled  bool
	StartErr   error
	StderrTail string // 最后 64 KB
	Duration   time.Duration
}

func Run(ctx context.Context, s Spec, o Options) Outcome

func AcquireSlot(ctx context.Context, lockDir, name string, max int, onWait func()) (release func(), err error)

func SearchPath(configured, cacheFile string) string      // 02 架构文档 8.4 节
func LookPath(name, pathList string) (string, error)       // 在给定 PATH 中查找
func Env(base []string, pathList string, extra map[string]string) []string // 替换 PATH 并叠加 extra
```

- [x] **步骤 1：写失败的测试**

```go
func TestRunTimeoutKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := fmt.Sprintf("sleep 30 & echo $! > %s; wait", pidFile)
	out := Run(context.Background(), Spec{Path: "/bin/sh", Args: []string{"-c", script}, Dir: dir}, Options{Timeout: 300 * time.Millisecond})
	if !out.TimedOut {
		t.Fatalf("expected timeout, got %+v", out)
	}
	raw, _ := os.ReadFile(pidFile)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	time.Sleep(200 * time.Millisecond)
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("grandchild %d still alive", pid)
	}
}
```

另外：
- `TestRunLinesInOrder`：`printf 'a\nb\nc'` → OnLine 依次收到 a、b、c（最后一行没有换行也要送达）；StdoutFile 内容完全一致。
- `TestRunCancel`：ctx 在 200ms 后取消，`Cancelled == true`，总耗时小于 6 秒。
- `TestRunExitCode`：`exit 7` → `ExitCode == 7`。
- `TestRunStartErr`：Path 不存在 → `StartErr != nil`。
- `TestStderrTail`：stderr 输出 100 KB → `len(StderrTail) == 64*1024`，且是末尾部分。
- `TestLongLineDropped`：输出一行 9 MB 后再输出 `ok` → OnLine 只收到 `ok`。
- `TestAcquireSlotExclusive`：max=1，第一次获取后第二次阻塞，调用 release 后 1.5 秒内第二次成功；`onWait` 至少被调用一次。
- `TestSearchPathConfigured`：`SearchPath("/a:/b", "")` 返回 `/a:/b`，不执行 shell。
- `TestEnvReplacesPath`：结果中只有一个 `PATH=`，extra 中的变量存在。

- [x] **步骤 2：实现**

要点：
- `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`；超时或 ctx 结束时 `syscall.Kill(-pgid, SIGTERM)`，5 秒后仍未退出就 `SIGKILL`；设置 `cmd.WaitDelay = 6 * time.Second`。
- stdout 用 `bufio.Reader` 自己按 `\n` 切行，单行超过 8 MB 时丢弃到下一个换行。
- stderr 写入文件并保留一个 64 KB 环形缓冲。
- `AcquireSlot`：在 `lockDir` 下依次尝试 `<name>-0.lock` … `<name>-(max-1).lock`，`syscall.Flock(fd, LOCK_EX|LOCK_NB)`；全部失败时调用 `onWait`，等 1 秒重试，直到 ctx 结束。release 解锁并关闭文件。
- `SearchPath`：`configured` 非空直接返回；否则读缓存（24 小时内有效），缓存无效时执行 `$SHELL -lic 'printf %s "$PATH"'`（3 秒超时，失败忽略），与 `os.Getenv("PATH")` 合并去重，再追加 `~/.local/bin`、`/opt/homebrew/bin`、`/usr/local/bin`，写缓存（`cacheFile` 为空时不读写缓存）。

- [x] **步骤 3：运行测试并提交**

```bash
go test ./internal/proc/ -race -v
git add internal/proc && git commit -m "feat(proc): run native CLIs with process groups, timeouts and slots"
```

---

### 任务 7：驱动接口与 codex 驱动

**文件：**
- 新建：`internal/driver/driver.go`、`internal/driver/classify.go`、`internal/driver/codex.go`
- 测试：`internal/driver/codex_test.go`、`internal/driver/classify_test.go`
- 参考旧代码：`git show archive/r4.1-final:internal/agent/stream.go`、`git show archive/r4.1-final:internal/agent/usage.go`

**接口：**
- 消费：`api.Permission`、`api.Usage`、`api.Kind`、`config.AgentConfig`、`proc.Run`。
- 产出：

```go
type Invocation struct {
	Prompt     string
	Workdir    string
	Permission api.Permission
	Images     []string
	SchemaFile string
	SessionID  string
	Model      string
	AddDirs    []string
	Network    bool
	ExtraArgs  []string
}

type Event struct {
	Kind string // "session" | "message" | "tool" | "notice"
	Text string
	Tool string
}

type ErrorKind string

const (
	ErrNone             ErrorKind = ""
	ErrNotInstalled     ErrorKind = "not_installed"
	ErrAuth             ErrorKind = "auth"
	ErrQuota            ErrorKind = "quota"
	ErrTrust            ErrorKind = "trust"
	ErrPermissionDenied ErrorKind = "permission_denied"
	ErrNoArtifact       ErrorKind = "no_artifact"
	ErrSchemaMismatch   ErrorKind = "schema_mismatch"
	ErrTimeout          ErrorKind = "timeout"
	ErrCancelled        ErrorKind = "cancelled"
	ErrProtocol         ErrorKind = "protocol"
	ErrFailed           ErrorKind = "failed"
)

// Switchable 表示换一个 CLI 有可能成功（02 架构文档 4.4 节）。
func (k ErrorKind) Switchable(kind api.Kind) bool

type Result struct {
	Text      string
	SessionID string
	Usage     *api.Usage
	ErrorKind ErrorKind
	Error     string
	Denied    []string
}

type Health struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Auth      string `json:"auth"` // "ok" | "missing" | "unknown"
	Error     string `json:"error,omitempty"`
}

type Parser interface {
	Feed(line []byte) []Event
	Finish(exitCode int, stderrTail string) Result
}

type Driver interface {
	Name() string
	Command() string
	Build(inv Invocation) ([]string, error)
	NewParser() Parser
	Probe(ctx context.Context, exe string, env []string) Health
	NativeSchema() bool
}

func New(name string, cfg config.AgentConfig) (Driver, error) // 只认 codex / cursor / antigravity
func All(cfg config.Config) map[string]Driver                  // 为全部已启用的 agent 创建驱动

// classify.go
func ClassifyText(stderrTail string, text string) ErrorKind    // 通用关键词：auth / quota / trust
```

- [x] **步骤 1：写参数拼装测试（codex）**

```go
func TestCodexBuild(t *testing.T) {
	d, _ := New("codex", config.Default().Agents["codex"])
	cases := []struct {
		name string
		inv  Invocation
		want []string
	}{
		{"read", Invocation{Prompt: "hi", Workdir: "/w", Permission: api.PermRead},
			[]string{"exec", "--json", "--skip-git-repo-check", "-C", "/w", "-s", "read-only", "--", "hi"}},
		{"write all options", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermWrite, Network: true, Model: "gpt-5.5",
			SchemaFile: "/s.json", Images: []string{"/a.png", "/b.png"}, AddDirs: []string{"/in"}, ExtraArgs: []string{"--enable", "x"}},
			[]string{"exec", "--json", "--skip-git-repo-check", "-C", "/w", "-s", "workspace-write",
				"-c", "sandbox_workspace_write.network_access=true", "-m", "gpt-5.5", "--output-schema", "/s.json",
				"-i", "/a.png", "-i", "/b.png", "--add-dir", "/in", "--enable", "x", "--", "p"}},
		{"full", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermFull},
			[]string{"exec", "--json", "--skip-git-repo-check", "-C", "/w", "--dangerously-bypass-approvals-and-sandbox", "--", "p"}},
		{"resume write", Invocation{Prompt: "fix", Workdir: "/w", Permission: api.PermWrite, Network: true, SessionID: "sid"},
			[]string{"exec", "resume", "--json", "--skip-git-repo-check", "-c", `sandbox_mode="workspace-write"`,
				"-c", "sandbox_workspace_write.network_access=true", "--", "sid", "fix"}},
		{"resume full", Invocation{Prompt: "fix", Workdir: "/w", Permission: api.PermFull, SessionID: "sid"},
			[]string{"exec", "resume", "--json", "--skip-git-repo-check", "--dangerously-bypass-approvals-and-sandbox", "--", "sid", "fix"}},
	}
	for _, c := range cases {
		got, err := d.Build(c.inv)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q (err %v)", c.name, got, c.want, err)
		}
	}
}
```

如果任务 2 的实测要求改映射，以 `driver-notes.md` 为准修改期望值，并在测试旁边注释引用对应小节。续接时 `AddDirs` 被忽略（codex resume 不支持）。

- [x] **步骤 2：写解析测试（codex）**

用 `internal/driver/testdata/codex/` 中的样本（找不到真实样本时读 `synthetic-` 版本）：
- `ask_ok`：`Text` 非空，`SessionID` 非空，`ErrorKind == ErrNone`，Usage 的 input/output 非零（样本里有 usage 时）。
- `schema_ok`：`Text` 可以被 `json.Unmarshal` 解析，并含有 `answer` 字段。
- `resume_ok`：`SessionID` 与 `ask_ok` 相同。
- `error_bad_model`：`ErrorKind != ErrNone`。
- 内联用例：

```go
func TestCodexParserEdgeCases(t *testing.T) {
	feed := func(lines ...string) Result {
		p := (&codex{}).NewParser()
		for _, l := range lines {
			p.Feed([]byte(l))
		}
		return p.Finish(0, "")
	}
	r := feed(`{"type":"thread.started","thread_id":"t1"}`,
		`{"type":"error","message":"Reconnecting... 1/5"}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"first"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"final"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":5,"output_tokens":2}}`)
	if r.ErrorKind != ErrNone || r.Text != "final" || r.SessionID != "t1" {
		t.Fatalf("reconnect notice must not fail and last message wins: %+v", r)
	}
	r = feed(`{"type":"thread.started","thread_id":"t1"}`,
		`{"type":"turn.failed","error":{"message":"You've hit your usage limit."}}`)
	if r.ErrorKind != ErrQuota {
		t.Fatalf("want quota, got %+v", r)
	}
	r = feed(`{"type":"thread.started","thread_id":"t1"}`)
	if r.ErrorKind != ErrProtocol {
		t.Fatalf("missing turn.completed must be protocol error, got %+v", r)
	}
	r = feed(`not json`, `{"type":"turn.completed"}`)
	if r.ErrorKind != ErrNone {
		t.Fatalf("a damaged line must not fail the run: %+v", r)
	}
}
```

- [x] **步骤 3：写分类测试**

`TestClassifyText` 表驱动：`"Not logged in. Run codex login"` → auth；`"401 Unauthorized"` → auth；`"429 Too Many Requests"`、`"rate limit exceeded"`、`"quota exceeded"` → quota；`"Workspace Trust Required"` → trust；`"something else"` → ErrNone。`TestSwitchable`：auth / quota / trust / not_installed 对所有 kind 为 true；no_artifact 只对 `KindImage` 为 true；permission_denied、timeout、schema_mismatch 为 false。

```bash
go test ./internal/driver/ -race     # 期望失败
```

- [x] **步骤 4：实现**

要点：
- 参数顺序严格按测试；`ExtraArgs` 放在 `--` 之前。
- 解析器：`thread.started` → session 事件；`item.completed` 且 `item.type == "agent_message"` → 记录为最终文本并产生 message 事件；其他 `item.*` 中带 `command` 或工具名的产生 tool 事件；`turn.completed` → 结束，读取 usage；`error` 匹配 `^Reconnecting\.\.\. \d+/\d+` → notice；其余 `error` 或 `turn.failed` → 失败，先判断额度（`usage_limit_exceeded`、`quota_exceeded`、"hit your usage limit"），再用 `ClassifyText` 判断，都不是则为 `ErrFailed`。
- `Finish`：原生错误事件优先；`exitCode != 0` 且尚无错误 → `ClassifyText(stderrTail, "")`，仍无结果则为 `ErrFailed`；没有明确运行错误且缺少结束事件 → `ErrProtocol`。按 02 第 4.4 节优先识别 stderr 中的 auth/quota，保证原生 CLI 在认证失败时只写 stderr 也能触发切换。错误信息用中文给出下一步建议（例如 auth："请在终端运行 codex login 登录后重试"）。
- `Probe`：`<exe> --version`（5 秒超时）取最后一个非空行作为版本；`<exe> login status` 退出码 0 → `ok`，非 0 → `missing`，执行失败 → `unknown`。
- `NativeSchema()` 返回 true。

- [x] **步骤 5：运行测试并提交**

```bash
go test ./internal/driver/ -race -v
git add internal/driver && git commit -m "feat(driver): add driver interface and codex driver"
```

---

### 任务 8：cursor 与 antigravity 驱动

**文件：**
- 新建：`internal/driver/cursor.go`、`internal/driver/antigravity.go`
- 测试：`internal/driver/cursor_test.go`、`internal/driver/antigravity_test.go`

**接口：**
- 消费：任务 7 的全部类型。
- 产出：`New("cursor", …)`、`New("antigravity", …)` 可用；`cursor.NativeSchema() == false`，`antigravity.NativeSchema() == true`。

- [x] **步骤 1：写参数拼装测试**

cursor 期望：

```go
{"read", Invocation{Prompt: "hi", Workdir: "/w", Permission: api.PermRead},
	[]string{"-p", "--output-format", "stream-json", "--trust", "--workspace", "/w", "--mode", "ask", "hi"}},
{"write", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermWrite, Model: "m", AddDirs: []string{"/in"}},
	[]string{"-p", "--output-format", "stream-json", "--trust", "--workspace", "/w", "--force", "--sandbox", "enabled",
		"--model", "m", "--add-dir", "/in", "p"}},
{"full", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermFull},
	[]string{"-p", "--output-format", "stream-json", "--trust", "--workspace", "/w", "--force", "--sandbox", "disabled", "p"}},
{"resume", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermRead, SessionID: "sid"},
	[]string{"-p", "--output-format", "stream-json", "--trust", "--workspace", "/w", "--mode", "ask", "--resume", "sid", "p"}},
```

antigravity 期望：

```go
{"read", Invocation{Prompt: "hi", Workdir: "/w", Permission: api.PermRead},
	[]string{"--output-format", "stream-json", "--print-timeout", "0", "--sandbox", "--settings",
		`{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}`, "--print", "hi"}},
{"write all options", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermWrite, Model: "m", SchemaFile: "/s.json", AddDirs: []string{"/in"}},
	[]string{"--output-format", "stream-json", "--print-timeout", "0", "--dangerously-skip-permissions", "--sandbox",
		"--model", "m", "--json-schema", "/s.json", "--add-dir", "/in", "--print", "p"}},
{"full", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermFull},
	[]string{"--output-format", "stream-json", "--print-timeout", "0", "--dangerously-skip-permissions", "--print", "p"}},
{"resume", Invocation{Prompt: "p", Workdir: "/w", Permission: api.PermRead, SessionID: "sid"},
	[]string{"--output-format", "stream-json", "--print-timeout", "0", "--sandbox", "--settings",
		`{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}`, "--conversation", "sid", "--print", "p"}},
```

两个驱动都忽略 `Images`（参考图通过提示词里的路径和 `AddDirs` 传递，由 step runner 负责）。cursor 忽略 `SchemaFile` 和 `Network`。

- [x] **步骤 2：写解析测试**

Cursor 用样本文件测试 `ask_ok` / `schema_ok` / `resume_ok` / `error_bad_model`（规则同任务 7）；用量读取 `result.usage.inputTokens/outputTokens`。

根据 2026-11-16 追加六次实测调整：Antigravity 当前 `ask_ok/schema_ok/resume_ok` 按任务 7 的成功规则断言，`resume_ok` 文本为 `Add` 且 SessionID 与 ask 一致；`image_ok` 优先读取 `result.structured_output`，Text 为包含 files 的合法 JSON；`error_bad_model` 从 `result.error` 识别失败。`error_plan_permission` 期望 `ErrPermissionDenied`、`Denied == ["command"]`；`error_write_denied`、`error_command_denied` 通过 `step_update` 中的明确权限错误返回 `ErrPermissionDenied`，Denied 分别为 `["write_file"]`、`["command"]`，即使最终 SUCCESS 且没有 denied_actions。只读方案详见 `driver-notes.md`。内联用例：

```go
// cursor：最后一条 assistant 消息胜出；result 的 subtype 非 success 判失败
`{"type":"system","subtype":"init","session_id":"c1"}`
`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"working..."}]}}`
`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}`
`{"type":"result","subtype":"success","result":"working...done","session_id":"c1"}`
// 期望 Text == "done"，SessionID == "c1"

`{"type":"result","subtype":"error","is_error":true,"result":"Workspace Trust Required"}`
// 期望 ErrorKind == ErrTrust

// antigravity：denied_actions 非空时即使 SUCCESS 也判 permission_denied
`{"event":"result","result":{"conversation_id":"a1","status":"SUCCESS","response":"ok","denied_actions":["run_command"]}}`
// 期望 ErrorKind == ErrPermissionDenied，Denied == ["run_command"]，Text == "ok"

`{"event":"result","result":{"conversation_id":"a1","status":"SUCCESS","response":"","denied_actions":[{"action":"command","display_name":"RunCommand"}]}}`
// 期望 ErrorKind == ErrPermissionDenied，Denied == ["command"]

`{"event":"result","result":{"conversation_id":"a1","status":"SUCCESS","response":"说明文字和 JSON","structured_output":{"files":["/out/a.jpg"]}}}`
// 期望 Text == `{"files":["/out/a.jpg"]}`

`{"event":"result","result":{"conversation_id":"a1","status":"ERROR","response":"quota exceeded"}}`
// 期望 ErrorKind == ErrQuota
```

- [x] **步骤 3：实现并提交**

要点：cursor 的 `Probe` 认证检查用 `<exe> status --format json`（退出码 0 且输出不含 "not logged" → ok）；antigravity 的认证固定为 `unknown`。

```bash
go test ./internal/driver/ -race -v
git add internal/driver && git commit -m "feat(driver): add cursor and antigravity drivers"
```

---

### 任务 9：健康状态与 `xcli agents`

**文件：**
- 新建：`internal/health/health.go`、`cmd/agents.go`、`cmd/config.go`
- 测试：`internal/health/health_test.go`

**接口：**
- 消费：`driver.Driver`、`driver.Health`、`driver.ErrorKind`、`proc.SearchPath`、`proc.LookPath`、`proc.Env`、`config.Load`。
- 产出：

```go
type Entry struct {
	Health         driver.Health `json:"health"`
	CheckedAt      time.Time     `json:"checked_at"`
	CooldownUntil  time.Time     `json:"cooldown_until,omitempty"`
	CooldownReason string        `json:"cooldown_reason,omitempty"`
}

type Cache struct { /* file 路径 + 互斥锁 */ }

func Open(home string) *Cache                                     // <home>/cache/health.json
func (c *Cache) Probe(ctx context.Context, d driver.Driver, exe string, env []string, maxAge time.Duration) Entry
func (c *Cache) Cooldown(name string, kind driver.ErrorKind, now time.Time) // quota 30 分钟，auth 10 分钟，其他忽略
func (c *Cache) Available(name string, now time.Time) bool        // 不在冷却中
func (c *Cache) Entry(name string) Entry
func (c *Cache) Reset() error                                     // 清除所有冷却
```

读改写时对 `health.json.lock` 加 `flock`，避免多个进程互相覆盖。

- [ ] **步骤 1：测试**

- `TestCooldown`：`Cooldown("codex", ErrQuota, t0)` 后 `Available("codex", t0+29m) == false`，`t0+31m == true`；`ErrAuth` 为 10 分钟；`ErrFailed` 不冷却。
- `TestCooldownPersists`：一个 Cache 写入冷却，用同一个 home 新开的 Cache 能读到。
- `TestReset`。
- `TestProbeCached`：用一个计数的假 Driver，maxAge 内第二次 Probe 不再调用 `d.Probe`。

- [ ] **步骤 2：实现 `xcli agents` 与 `xcli config`**

`xcli agents`：对每个配置中的 agent（按 `config.AgentOrder`）解析 PATH 和可执行文件，调用 `Cache.Probe`（maxAge 10 分钟），输出表格：

```
NAME         INSTALLED  VERSION          AUTH     STATUS               CODE REVIEW ASK IMAGE
codex        yes        codex-cli 0.167  ok       可用                  3    2      2   2
cursor       yes        2026.10.22       ok       可用                  3    3      2   1
antigravity  yes        1.5.2            unknown  冷却中（额度）至 17:40  2    2      3   3
```

`--json` 输出 `[{name, installed, path, version, auth, available, cooldown_until, cooldown_reason, strengths}]`；`--reset` 先清除冷却再输出。

`xcli config show` 输出生效配置的 YAML；`config init` 在 `<home>/config.yaml` 不存在时写入默认配置（已存在则报错并提示路径）；`config path` 打印路径。

- [ ] **步骤 3：手动验证并提交**

```bash
go test ./internal/health/ -race -v
go run . agents                 # 本机三个 CLI 都应显示 INSTALLED=yes（只执行 --version 和登录状态检查，不消耗模型额度）
go run . config show
git add internal/health cmd && git commit -m "feat: add agent health cache and agents/config commands"
```

---

### 任务 10：测试用假 CLI `internal/testutil`

**文件：**
- 新建：`internal/testutil/fakecli/main.go`（`package main`）、`internal/testutil/fakecli.go`（`package testutil`）
- 测试：`internal/testutil/fakecli_test.go`

**接口：**
- 产出：

```go
package testutil

type FakeStep struct {
	StdoutFile string            `json:"stdout_file,omitempty"` // 回放的样本文件（绝对路径）
	Stdout     []string          `json:"stdout,omitempty"`      // 直接给出的 stdout 行
	Stderr     string            `json:"stderr,omitempty"`
	Exit       int               `json:"exit"`
	SleepMS    int               `json:"sleep_ms,omitempty"`
	WriteFiles map[string]string `json:"write_files,omitempty"` // 相对 cwd 或绝对路径 → 内容
	WritePNG   string            `json:"write_png,omitempty"`   // 在该路径写一张 8x8 PNG；"{{OUT}}/x.png" 中 {{OUT}} 替换为提示词里"输出目录："后的路径
}

type FakeScript map[string][]FakeStep // 键为 "codex" | "cursor-agent" | "agy"

type FakeCall struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
	Cwd  string   `json:"cwd"`
}

// InstallFakeCLIs 编译 fakecli 并以三个名字安装到临时目录，写入剧本，设置 XCLI_FAKE_* 环境变量（t.Setenv）。
// 返回值 pathList = "<临时目录>:/bin:/usr/bin"，测试把它写进 config.Path。
func InstallFakeCLIs(t *testing.T, script FakeScript) (pathList string)

// FakeCalls 读取调用记录。
func FakeCalls(t *testing.T) []FakeCall
```

fakecli 行为：从 `XCLI_FAKE_SCRIPT` 读剧本，用 `filepath.Base(os.Args[0])` 选出自己的步骤列表；用 `XCLI_FAKE_STATE` 目录下 `<name>.count` 文件计数（加 flock），调用次数 n 从 0 开始，第 n 次调用执行下标为 `min(n, len-1)` 的步骤（剧本用完后重复最后一步）；把 `{name, args, cwd}` 追加写入 `XCLI_FAKE_LOG`；stdout 中的 `{{CWD}}` 替换为当前目录；`--version` 参数直接输出 `fake 1.0` 并退出 0；`login status` / `status` 输出 `ok` 并退出 0。

- [ ] **步骤 1：测试**

`TestFakeCLIRunsScriptInOrder`：剧本让 codex 第一次输出 `a`、退出 0，第二次输出 `b`、退出 3；用 `exec.Command` 调用两次，检查输出和退出码；`FakeCalls` 有 2 条记录且 Args 正确。`TestFakeCLIWritesPNG`：提示词参数中包含 `输出目录：/tmp/x`（用 t.TempDir），`WritePNG: "{{OUT}}/img.png"` → 文件存在且能被 `png.DecodeConfig` 解码。

- [ ] **步骤 2：实现并提交**

编译用 `go build -o <dir>/codex ./internal/testutil/fakecli`（在 `InstallFakeCLIs` 中用 `sync.Once` 缓存编译结果到 `os.MkdirTemp`），再复制成 `cursor-agent` 和 `agy`。

```bash
go test ./internal/testutil/ -race -v
git add internal/testutil && git commit -m "test: add scripted fake native CLI"
```

---

## 第 2 阶段：能力

### 任务 11：提示词与 Schema `internal/recipe`

**文件：**
- 新建：`internal/recipe/recipe.go`、`internal/recipe/schemas.go`、`internal/recipe/prompts/{ask,code,fix,review,image,plan}.md`
- 测试：`internal/recipe/recipe_test.go`

**接口：**
- 产出：

```go
type Handoff struct {
	StepID    string
	Agent     string
	Kind      api.Kind
	Summary   string   // 截断到 8 KB
	Artifacts []string
	Files     []string
}

type Input struct {
	Kind         api.Kind
	Agent        string
	Goal         string
	Instruction  string
	Workdir      string
	OutputDir    string
	Inputs       []string
	Images       []string
	Upstream     []Handoff
	Feedback     string
	Round        int
	InlineSchema []byte // 驱动没有原生 Schema 参数时，把 Schema 附在提示词里
	DiffFile     string // review
	CoderSummary string // review
}

type AgentInfo struct {
	Name      string
	Strengths map[string]int
	Note      string
}

type PlannerInput struct {
	Goal      string
	Workdir   string
	IsGit     bool
	Listing   []string
	Inputs    []string
	Agents    []AgentInfo
	MaxSteps  int
}

func Render(in Input) (string, error)        // fix 模板在 Kind==KindCode && Round>1 时使用
func RenderPlanner(in PlannerInput) (string, error)
func ExtractJSON(text string) (json.RawMessage, error)
func Validate(schema []byte, doc json.RawMessage) error
func CheckSchema(schema []byte) error          // engine.Normalize 用来校验调用方给的 Schema
func ReviewSchema() []byte
func ImageResultSchema() []byte                 // {"files": string[]}，严格模式
func PlanSchema(agents []string, maxSteps int) []byte
func ImageHint(agent string) string
```

- [ ] **步骤 1：写模板**

所有模板第一行都是 `# xcli 任务`（保证提示词不以 `-` 开头），统一包含：

```
你正在被 xcli 调度执行一个子任务。请直接完成，不要向用户提问；用与需求相同的语言回复；不要执行 git commit、git push。
```

各模板的关键内容：
- `ask.md`：只读，不要修改任何文件；列出输入文件路径；有 InlineSchema 时追加"最终回复只能是一个符合下面 JSON Schema 的 JSON 对象，不要包含解释或代码块标记"并附上 Schema。
- `code.md`：在工作目录中完成需求；完成后在最终回复中说明修改了哪些文件、为什么这样改、如何验证。
- `fix.md`：说明这是第 N 轮修复，下面是验证失败的输出或审查意见；逐条处理，不采纳的说明理由；最终回复说明这轮改了什么。
- `review.md`：只读审查，不要修改文件；给出原始需求、实现者总结、diff 文件路径（`DiffFile`）；只有确实影响正确性、安全或需求达成的问题才标 blocker / major；按审查 Schema 输出。
- `image.md`：包含 `ImageHint(agent)`；"输出目录：<OutputDir>"（这一行的格式固定，fakecli 依赖它）；文件名以步骤 ID 开头；参考图路径与"不要覆盖原图"；最终回复 `{"files": [...]}`。
- `plan.md`：02 架构文档 6.3 节列出的输入和规则。
- 所有模板都渲染 `Upstream`：每个上游步骤一段"上游步骤 <id>（<agent>，<kind>）的结果"，包含摘要、产物路径、变更文件。
- 根据 Antigravity 只读实测，ask/review 模板在 `Agent=="antigravity"` 时，引导使用 `view_file/list_dir/find_by_name/grep_search` 等内置只读工具；plan 模板同样说明通过内置读取工具查看目录与文件，直接输出 xcli 计划 JSON。原生 plan 模式由 02 第 4.2 节的会话级 settings 替代。

`ImageHint`：codex → `请使用你内置的图片生成工具（image_gen）生成图片。`；antigravity → `请调用内置的 image-generator 子代理生成图片（通过子代理调用，不要使用 --agent 参数）。`；cursor → `请使用内置的 GenerateImage 工具生成图片。`

- [ ] **步骤 2：写测试**

- `TestRenderAskWithInlineSchema`：结果以 `# xcli 任务` 开头，包含 Schema 文本和输入路径。
- `TestRenderFixUsedForRound2`：`Kind=code, Round=2, Feedback="go test failed"` → 包含"第 2 轮"和反馈文本。
- `TestRenderImageHasOutputLine`：包含 `输出目录：/out`，codex 的提示词包含 `image_gen`。
- `TestExtractJSON`：表驱动——纯 JSON；` ```json\n{...}\n``` ` 代码块；前后有说明文字；数组；无 JSON 时报错。
- `TestValidate`：审查 Schema 对 `{"verdict":"pass","summary":"","issues":[]}` 通过，对 `{"verdict":"maybe"}` 失败。
- `TestBuiltinSchemasStrict`：遍历 `ReviewSchema()`、`ImageResultSchema()`、`PlanSchema([]string{"codex"}, 6)` 中的每个 object，断言都有 `additionalProperties:false`，且 `required` 覆盖全部 `properties` 键（这是 codex 严格模式的要求）。
- `TestPlanSchemaAgentsEnum`：`PlanSchema([]string{"codex","cursor"}, 4)` 中 agent 的 enum 为 `["auto","codex","cursor"]`，`maxItems` 为 4。

- [ ] **步骤 3：实现并提交**

`Validate` 和 `CheckSchema` 使用 `github.com/santhosh-tekuri/jsonschema/v6` 编译 Schema。`ExtractJSON`：先去掉代码块围栏，再找第一个 `{` 或 `[` 与最后一个对应的 `}` 或 `]`，用 `json.Valid` 判断。

```bash
go get github.com/santhosh-tekuri/jsonschema/v6
go test ./internal/recipe/ -race -v
git add internal/recipe go.mod go.sum && git commit -m "feat(recipe): add prompt templates and strict schemas"
```

---

### 任务 12：工作区 `internal/workspace`

**文件：**
- 新建：`internal/workspace/workspace.go`、`internal/workspace/changes.go`、`internal/workspace/verify.go`
- 测试：`internal/workspace/workspace_test.go`、`internal/workspace/changes_test.go`、`internal/workspace/verify_test.go`
- 参考旧代码：`git show archive/r4.1-final:internal/worktree/manager.go`

**接口：**
- 消费：`api.Request`、`api.Changes`、`api.VerifyResult`。
- 产出：

```go
type Workspace struct {
	Dir       string // 实际工作目录
	Original  string // 请求中的 workdir；没有则为空
	Isolation string // "none" | "worktree" | "temp"
	Branch    string
	Git       bool
	InputsDir string // <task>/inputs
	Inputs    []string // 快照后的输入绝对路径
	Images    []string // 快照后的参考图绝对路径
}

func Prepare(taskDir, taskID string, req api.Request) (*Workspace, []string, error) // 第二个返回值是 notice 文本
func (w *Workspace) Baseline() error
func (w *Workspace) Changes(patchPath string) (*api.Changes, error)
func RunVerify(ctx context.Context, dir string, cmds []string, timeout time.Duration, logDir string, round int) []api.VerifyResult
func Apply(patchPath, repo string) error
func RemoveWorktree(repo, path, branch string) error
func Listing(dir string, max int) []string // 顶层条目，给 planner 用
```

- [ ] **步骤 1：写测试**（用 `git init` 的临时仓库，测试里设置 `GIT_AUTHOR_NAME` 等环境变量）

- `TestPrepareTemp`：没有 workdir → `Dir == <task>/work`，`Isolation == "temp"`。
- `TestPrepareInPlace`：有 workdir 且不隔离 → `Dir == workdir`，`Isolation == "none"`，`Git` 判断正确。
- `TestPrepareWorktree`：`isolate=true` → `Dir == <task>/worktree`，分支 `xcli/<taskID>` 存在；仓库有未提交改动时 notice 中包含"未提交的改动不会带入"。
- `TestPrepareWorktreeNonGit`：`isolate=true` 但不是 git 仓库 → 报错"isolate 需要 git 仓库"。
- `TestSnapshotInputs`：文件和目录都被复制到 `<task>/inputs/`，返回绝对路径；总量超过上限报错。
- `TestChangesGitExcludesPreexisting`：仓库里先有一个未提交修改 `a.go` 和一个未跟踪文件 `old.txt`；`Baseline()`；然后修改 `b.go`、新建 `new.txt`；`Changes()` 的 Files 只有 `b.go` 和 `new.txt`，patch 文件同时包含两者；`git status --porcelain` 前后显示索引没有变化（`git diff --cached` 为空）。
- `TestChangesNonGit`：非 git 目录，修改一个文件、新增一个文件 → Files 包含两者，`DiffFile` 为空。
- `TestRunVerify`：`["true", "echo boom; exit 3"]` → 第一条通过，第二条 `ExitCode == 3`、日志包含 boom；超时的命令（`sleep 5`，timeout 200ms）判失败。
- `TestApply`：在 worktree 中修改并生成 patch，`Apply` 到原仓库后文件内容一致。

- [ ] **步骤 2：实现并提交**

变更检测严格按 02 架构文档 7.2 节：基线用 `git stash create`（输出为空则用 `git rev-parse HEAD`），记录基线时的未跟踪文件集合；diff 用 `git diff --binary <基线>`，新增的未跟踪文件逐个用 `git diff --no-index --binary /dev/null <file>`（该命令有差异时退出码为 1，属于正常情况）。全程不执行 `git add`、`git reset`、`git stash push`。

```bash
go test ./internal/workspace/ -race -v
git add internal/workspace && git commit -m "feat(workspace): prepare dirs, worktrees, change detection and verify"
```

---

### 任务 13：图片收集 `internal/artifact`

**文件：**
- 新建：`internal/artifact/artifact.go`
- 测试：`internal/artifact/artifact_test.go`

**接口：**
- 产出：

```go
type CollectOptions struct {
	OutDir    string
	Since     time.Time
	Text      string   // 最终文本，用于提取绝对路径
	Files     []string // 最终 JSON 中的 files
	ExtraDirs []string // 该 CLI 的 artifact_dirs
	Prefix    string   // 复制进 OutDir 时的文件名前缀（步骤 ID）
}

func Inspect(path string) (api.Artifact, error)
func CollectImages(o CollectOptions) ([]api.Artifact, error)
```

- [ ] **步骤 1：写测试**

用 `image/png`、`image/jpeg` 在测试里生成图片：
- `TestInspectPNG`：MIME `image/png`，宽高正确，SHA-256 与 `sha256sum` 一致。
- `TestCollectFromOutDir`：OutDir 中的新 PNG 被收集；修改时间早于 Since 的旧文件被忽略；无法解码的 `fake.png` 被忽略。
- `TestCollectFixesExtension`：JPEG 内容存成 `a.png` → 结果路径为 `a.jpg`，MIME `image/jpeg`，原 `a.png` 不再存在。
- `TestCollectCopiesExternal`：OutDir 之外的图片路径出现在 Text 中（`已保存到 /tmp/xxx/gen.png。`）→ 被复制进 OutDir，文件名为 `<Prefix>-gen.png`，原文件保留。
- `TestCollectExtraDirs`：ExtraDirs 第 2 层目录中的新图片被收集，第 4 层的不被收集。
- `TestCollectDedup`：同一文件同时出现在 OutDir、Files 和 Text 中，只返回一次。

- [ ] **步骤 2：实现并提交**

支持 png / jpeg / gif / webp（`import _ "golang.org/x/image/webp"`）；文本路径正则 `(/[^\s"'<>()]+\.(?i:png|jpe?g|webp|gif))`；ExtraDirs 中的 `~` 展开为用户主目录。

```bash
go test ./internal/artifact/ -race -v
git add internal/artifact go.mod go.sum && git commit -m "feat(artifact): collect and validate generated images"
```

---

### 任务 14：步骤执行 `internal/orchestrator`（env.go、step.go）

**文件：**
- 新建：`internal/orchestrator/env.go`、`internal/orchestrator/step.go`
- 测试：`internal/orchestrator/step_test.go`

**接口：**
- 消费：前面所有任务的产出。
- 产出：

```go
type Env struct {
	Config        config.Config
	Drivers       map[string]driver.Driver
	Health        *health.Cache
	Task          *store.Task
	WS            *workspace.Workspace
	Req           api.Request // 已规范化
	PathList      string
	LockDir       string      // <home>/locks
	ResumeSession string      // resume 时第一个步骤续接的会话
	Now           func() time.Time
}

type StepOutcome struct {
	Result    api.StepResult
	Text      string
	JSON      json.RawMessage
	Artifacts []api.Artifact
	Verify    []api.VerifyResult
	Review    *api.ReviewResult
	Changes   *api.Changes
	Files     []string // 变更文件，用于交接
}

// invoke 是调用 CLI 的唯一入口：抢槽位 → 解析可执行文件 → Build → proc.Run → 解析 → 落盘 → 发事件。
func (e *Env) invoke(ctx context.Context, agent string, stepID string, round int, inv driver.Invocation) driver.Result

// RunStep 执行一个步骤（不做 CLI 切换，切换由 Execute 负责）。
func (e *Env) RunStep(ctx context.Context, s api.Step, agent string, up []recipe.Handoff) StepOutcome
```

- [ ] **步骤 1：写测试（全部使用 fakecli）**

每个测试：`pathList := testutil.InstallFakeCLIs(t, script)`；`cfg := config.Default(); cfg.Path = pathList`；用 `store.Open(t.TempDir())` 创建任务；`workspace.Prepare` 准备工作区；构造 `Env`。

- `TestRunStepAsk`：codex 回放 `ask_ok` → 状态 `completed`，Text 非空，SessionID 非空；`FakeCalls()[0].Args` 包含 `-s read-only`；`steps/main-r1/prompt.md` 和 `stdout.jsonl` 存在。
- `TestRunStepAskSchemaRepair`：请求带 Schema；cursor 第一次回复不是 JSON，第二次回复合法 JSON → 状态 `completed`，`JSON` 合法，`FakeCalls` 有 2 条，第二条包含 `--resume`。
- `TestRunStepAskSchemaMismatch`：两次都不是合法 JSON → 状态 `failed`，`Error.Kind == "schema_mismatch"`。
- `TestRunStepImage`：codex 的步骤 `WritePNG: "{{OUT}}/main-1.png"`，stdout 回放 `image_ok` → 有 1 个 Artifact，`step_id == "main"`。
- `TestRunStepImageNoArtifact`：不写图片 → `Error.Kind == "no_artifact"`。
- `TestRunStepCodeVerifyFixLoop`：工作区为临时 git 仓库；codex 第一轮写 `a.txt`；verify 命令 `test -f b.txt`；codex 第二轮写 `b.txt`（第二次调用的参数包含 `resume`）→ 状态 `completed`，`Rounds == 2`，Verify 有两条（第一条失败、第二条通过）。
- `TestRunStepCodeReviewLoop`：`review_by = "cursor"`；cursor 第一次审查返回 `changes_requested` 且有一个 major 问题，第二次返回 `pass` → `Rounds == 2`，`Review.Verdict == "pass"`，cursor 的调用参数包含 `--mode ask`。
- `TestRunStepCodeMinorOnlyPasses`：审查只有 minor 问题 → 不进入修复，`Rounds == 1`，结果中保留该 minor 问题。
- `TestRunStepCodeMaxRounds`：verify 永远失败，`max_fix_rounds = 1` → 状态 `needs_attention`，`Rounds == 2`。
- `TestRunStepTimeout`：fakecli 休眠 5 秒，`step_timeout = 300ms` → `Error.Kind == "timeout"`。

- [ ] **步骤 2：实现**

要点：
- `invoke`：
  1. `proc.AcquireSlot(ctx, LockDir, agent, cfg.Agents[agent].MaxConcurrency, onWait)`，等待时发 `step.waiting`；
  2. `proc.LookPath(driver.Command(), PathList)`，找不到 → `ErrNotInstalled`；
  3. `Invocation.Model` 优先取 `Req.Models[agent]`，再取配置的 model；`ExtraArgs` 取配置；`Network` 取 `cfg.Defaults.Network`；
  4. `proc.Run`，超时取 `cfg.Defaults.StepTimeout`；stdout 每一行交给 Parser，Parser 返回的 message / tool 事件转成 `step.progress`（文本截断到 500 字）；
  5. 把 prompt、stdout、stderr、`result.json` 写进 `StepDir(stepID, round)`；
  6. Outcome 中的超时和取消优先于 Parser 结果；
  7. 结果为 auth / quota 时调用 `Health.Cooldown`。
- `RunStep` 按 kind 分派：
  - ask：权限固定 read；Schema 来自 `Req.Schema`（写到 `<task>/schema.json`）；驱动有原生 Schema 时传 `SchemaFile`，否则用 `InlineSchema`；拿到文本后 `ExtractJSON` + `Validate`，失败则在同一会话中发一次修正请求（提示词："你上一次的输出不符合 JSON Schema，错误如下：…… 请只输出修正后的 JSON。"），仍失败则 `schema_mismatch`。
  - image：输出目录 = `Req.OutputDir`（多步骤时加 `/<step.Output 或 step.ID>`），否则 `<task>/out/<step.ID>`；不在工作目录内时加入 `AddDirs`；codex 把参考图放进 `Images`，其他 CLI 把参考图所在目录加入 `AddDirs`；有原生 Schema 时用 `ImageResultSchema`；然后 `artifact.CollectImages`，每张图发一个 `artifact` 事件。
  - review：权限 read，审查 Schema；审查对象为工作目录。独立的 review 步骤（不是 code 循环里的审查）在 git 仓库中把 `git diff HEAD` 写进 `review.diff` 供审查者参考。
  - code：按 02 架构文档 6.5 节实现循环。第 1 轮之前调用 `WS.Baseline()`；每轮之后调用 `WS.Changes(<task>/changes.patch)`；修复轮续接同一会话，续接失败（ErrorKind 为 protocol / failed 且 stderr 包含 "session" 或 "not found"）时开新会话，提示词附上原需求和上一轮总结；`review_by == "auto"` 时用 Router 选审查者（Router 在任务 15 实现，本任务先只支持显式指定 agent 名，`auto` 时取 `cfg.Agents` 中 review 分数最高、且不是当前 agent 的已安装 CLI）；`ReviewBy == nil` 时不审查。
  - `Env.ResumeSession` 非空时，第一个步骤的第 1 轮用它续接。
- 每一轮发出 `step.started`（data 含 round）、`verify.result`、`review.result`；步骤结束发 `step.finished`。

- [ ] **步骤 3：运行测试并提交**

```bash
go test ./internal/orchestrator/ -race -v
git add internal/orchestrator && git commit -m "feat(orchestrator): run ask/image/review/code steps with verify and review loop"
```

---

### 任务 15：路由、规则计划与 DAG 执行器

**文件：**
- 新建：`internal/orchestrator/router.go`、`internal/orchestrator/plan.go`、`internal/orchestrator/execute.go`
- 修改：`internal/orchestrator/step.go`（`review_by == "auto"` 改为调用 Router）
- 测试：`internal/orchestrator/router_test.go`、`internal/orchestrator/plan_test.go`、`internal/orchestrator/execute_test.go`

**接口：**
- 产出：

```go
type Router struct { /* cfg、health、pathList、allowed */ }
func NewRouter(e *Env) *Router
func (r *Router) Candidates(kind api.Kind, exclude ...string) []string // 02 架构文档 6.2 节
func (r *Router) Pick(kind api.Kind, exclude ...string) (string, error)

func RulesPlan(req api.Request, cfg config.Config) api.Plan
func KeywordCapability(goal string, hasWorkdir bool) api.Capability
func ValidatePlan(p api.Plan, req api.Request, available []string, maxSteps int) error

func Execute(ctx context.Context, e *Env, plan api.Plan) api.Result
```

- [ ] **步骤 1：写测试**

Router：
- `TestRouterOrdersByStrength`：image → `[antigravity, codex, cursor]`；review → `[cursor, codex, antigravity]`。
- `TestRouterRespectsAllowedOrder`：`Req.Agents = ["cursor","codex"]`，ask（两者分数都是 2）→ `[cursor, codex]`。
- `TestRouterSkipsCooldownAndMissing`：codex 冷却中、agy 不在 PATH → code 只剩 `[cursor]`。
- `TestRouterExclude`：review 排除 cursor → `[codex, antigravity]`。
- `TestRouterLockedAgent`：`Req.Agent = "codex"` → 任何 kind 都只返回 `[codex]`。

规则计划：
- `TestRulesPlan`：ask / review / image → 单个步骤，ID `main`，`ReviewBy == nil`；code → `ReviewBy` 为 `"auto"`；code + `review=false` → nil；code + `reviewer="cursor"` → `"cursor"`；配置 `review: off` → nil；`Req.Agent` 非空时步骤的 Agent 等于它，否则为 `"auto"`；capability `auto` → 先用 `KeywordCapability` 判断再套用对应规则。
- `TestKeywordCapability`：`"画一张猫的封面图"` → image；有 workdir 的 `"修复登录 bug"` → code；没有 workdir 的 `"修复登录 bug"` → ask；`"总结这份文档"` → ask。
- `TestValidatePlan`：ID 重复、依赖不存在、有环（a→b→a）、agent 不可用、步数超限、`permission=read` 时出现 code 或 image 步骤、`review_by` 出现在非 code 步骤上，都报错；合法计划通过。

执行器（fakecli）：
- `TestExecuteParallel`：两个互不依赖的 ask 步骤，fakecli 各休眠 500ms → 总耗时小于 900ms；两个都 completed。
- `TestExecuteDependencyHandoff`：`b` 依赖 `a` → b 的 `prompt.md` 中包含 a 的最终文本。
- `TestExecuteSkipsDependents`：a 失败（非可切换错误）→ 依赖 a 的 b 为 `skipped`，与 a 无关的 c 为 completed，任务状态 `failed`。
- `TestExecuteSerializesCodeOnSameWorkdir`：两个互不依赖的 code 步骤，同一工作目录 → 按时间不重叠（用 fakecli 记录调用时间，或休眠 300ms 后检查总耗时 ≥ 600ms）。
- `TestExecuteFallbackOnQuota`：codex 返回额度错误 → 自动改用 cursor 完成；`Fallbacks == ["codex: quota"]`；发出 `agent.fallback` 事件；codex 进入冷却。
- `TestExecuteNoFallbackWhenLocked`：`Req.Agent = "codex"` 时额度错误直接失败。
- `TestExecuteSummaryAndStatus`：末端步骤的文本成为 `Summary`；包含 needs_attention 步骤时任务状态为 `needs_attention`。

- [ ] **步骤 2：实现**

要点：
- 执行器用一个调度循环：维护每个步骤的状态；每轮找出依赖已完成的 pending 步骤，用信号量限制 `max_parallel`；code 步骤按工作目录持锁（`map[string]*sync.Mutex`）。
- 每个步骤：`agent := step.Agent`；为 `"auto"` 时 `Router.Pick(kind)`；运行 `RunStep`；失败且 `ErrorKind.Switchable(kind)`、`Req.Fallback` 为 true（nil 时取配置）、`Req.Agent` 为空时，排除已失败的 agent 重新 Pick，最多切换 2 次。
- 汇总：Artifacts 按步骤顺序合并；Changes 取最后一个 code 步骤的结果；Verify 合并所有 code 步骤；Review 取最后一个；Error 取第一个失败步骤的错误；`Output` 取 ask 步骤的 JSON（多个时取最后一个末端 ask 步骤）。
- step.go 中 `review_by == "auto"` 改为 `NewRouter(e).Pick(api.KindReview, coderAgent)`；没有其他 CLI 可选时发 notice"没有可用的其他 CLI，跳过交叉审查"并跳过审查。

- [ ] **步骤 3：运行测试并提交**

```bash
go test ./internal/orchestrator/ -race -v
git add internal/orchestrator && git commit -m "feat(orchestrator): add router, rule plans and parallel DAG executor with fallback"
```

---

### 任务 16：engine 与命令行

**文件：**
- 新建：`internal/engine/engine.go`、`internal/engine/normalize.go`、`internal/engine/lifecycle.go`
- 新建：`cmd/request_flags.go`、`cmd/run.go`、`cmd/tasks.go`、`cmd/worker.go`、`cmd/output.go`
- 测试：`internal/engine/engine_test.go`、`internal/engine/normalize_test.go`、`cmd/run_test.go`

**接口：**
- 产出：

```go
type Engine struct {
	Home     string
	Config   config.Config
	Store    *store.Store
	Health   *health.Cache
	Drivers  map[string]driver.Driver
	PathList string
}

func New(home string) (*Engine, error)
func (e *Engine) Normalize(req api.Request, cwd string) (api.Request, error)
func (e *Engine) Create(req api.Request) (*store.Task, error)
func (e *Engine) Run(ctx context.Context, t *store.Task, sink func(api.Event)) api.Result
func (e *Engine) Detach(t *store.Task) error
func (e *Engine) Status(id string) (store.Record, error)
func (e *Engine) Wait(ctx context.Context, id string) (api.Result, error)
func (e *Engine) Cancel(id string) error
func (e *Engine) Follow(ctx context.Context, id string, after int, fn func(api.Event) error) error
func (e *Engine) Resume(parentID, instruction string) (api.Request, error)
func (e *Engine) Apply(id string, removeWorktree bool) error
func (e *Engine) GC(olderThan time.Duration) (int, error)
func InvalidResult(err error) api.Result // status=invalid，error.kind=invalid_request
```

- [ ] **步骤 1：写 Normalize 测试**

`TestNormalize` 表驱动：
- 默认值：`{"goal":"x"}` → capability auto、permission write、version 1。
- ask / review 的 permission 强制为 read；code 与 image 保留请求值（默认 write），但请求为 read 时报错"code/image 需要 write 或 full 权限"；auto 保留请求值。
- 相对路径的 workdir / inputs / images / output_dir 基于 cwd 转成绝对路径；不存在的 workdir、inputs、images 报错；output_dir 可以不存在。
- code / review 缺少 workdir 报错。
- 未知 agent、已禁用的 agent、`agents` 中有未知名字时报错。
- `schema_file` 被读入 `Schema` 并清空 `schema_file`；不合法的 Schema 报错（`recipe.CheckSchema`）。
- `timeout_sec == 0` 时取配置的 task_timeout。

- [ ] **步骤 2：写 engine 测试（fakecli）**

- `TestEngineRunAsk`：Create + Run → Result `completed`；`task.json` 中保存了 Result；`events.jsonl` 第一条是 `task.started`、最后一条是 `task.finished`，中间有 `plan.ready`。
- `TestEnginePlanOnly`：`plan_only=true` → 状态 `planned`，fakecli 没有被调用。
- `TestEngineDetachAndWait`：`Detach` 启动后台 worker（测试中用 `os.Args[0]` 无法作为 xcli，改为先 `go build -o <tmp>/xcli .`，并通过 `XCLI_SELF` 环境变量告诉 engine 用哪个可执行文件启动 worker）；`Wait` 拿到 completed 的 Result。
- `TestEngineCancel`：fakecli 休眠 10 秒；后台运行后调用 `Cancel` → 5 秒内状态变为 `cancelled`，fakecli 进程不存在。
- `TestEngineWorkerLost`：手动把 task.json 设为 running、PID 设为一个不存在的进程 → `Status` 返回 failed，`error.kind == "worker_lost"`。
- `TestEngineResume`：父任务是 image 任务且有一张图 → `Resume(parent, "改成蓝色")` 返回的 Request：capability image、agent 为父任务最后一步的 agent、images 包含父任务的图片、output_dir 与父任务相同、`parent_task` 已设置；运行后 fakecli 的参数包含父任务的会话 ID（codex `resume`）。
- `TestEngineGC`：结束 8 天的任务被删除，运行中的任务保留。

- [ ] **步骤 3：实现 engine**

要点：
- `New`：`config.Load`、`store.Open`、`health.Open`、`driver.All`、`proc.SearchPath(cfg.Path, <home>/cache/env.json)`。
- `Run`：
  1. `Update`：status running、PID = `os.Getpid()`；发 `task.started`；
  2. 用 `context.WithTimeout` 设置任务超时；
  3. `workspace.Prepare`（`PlanOnly` 时把请求的 `Isolate` 视为 false，不创建 worktree），notice 转成 `notice` 事件；
  4. 计划：`Req.Plan` 非空 → Source `explicit`，经过 `ValidatePlan`；capability auto → 任务 17 实现 planner 之前用 `RulesPlan`（内部走关键词分类）；其他 → `RulesPlan`；发 `plan.ready`；`PlanOnly` → status planned，结束；
  5. `orchestrator.Execute`；
  6. 填 Result 的 TaskID、Capability、Plan、Metadata、时间；ctx 被取消时 status 为 cancelled；
  7. `Update` 写入 Result 和最终状态；发 `task.finished`（data 为 Result）。
  - 每个事件同时写 store 和调用 sink。
- `Detach`：可执行文件取 `XCLI_SELF` 环境变量，否则 `os.Executable()`；`exec.Command(self, "--home", e.Home, "__worker", id)`，`SysProcAttr{Setsid: true}`，stdin 为 `/dev/null`，stdout/stderr 写 `worker.log`；`Start` 后 `Release`。
- `Cancel`：读 PID，向它发 SIGTERM；任务已结束时返回提示"任务已结束"。worker 和前台命令都用 `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)` 把信号转成 ctx 取消。
- `Status`：running 且 `syscall.Kill(pid, 0)` 失败 → 写入 failed + worker_lost。
- `Wait`：每 500ms 调用一次 `Status`，直到状态结束。
- `GC`：删除结束时间早于阈值的任务；如果任务有 worktree，先 `workspace.RemoveWorktree`。

- [ ] **步骤 4：实现命令**

- `request_flags.go`：按 03 接口规范 1.2 节定义参数，`func (f *requestFlags) Request(cap api.Capability, goal string) api.Request`；`--model codex=gpt-5.5` 解析成 map；`--review` / `--no-review` 都没给时 `Review == nil`。
- `run.go`：`do`、`plan`（设置 `PlanOnly`）、`ask`、`code`、`review`、`image` 共用一个构造函数；goal 取全部位置参数用空格拼接。`submit --request <file|->` 解析 JSON（`DisallowUnknownFields`），之后的流程与其他命令相同；命令行参数不覆盖 JSON。
- 流程：Normalize 失败 → `--json` 时输出 `InvalidResult`，退出码 2；`--detach` → Create + Detach，输出 `{"task_id":"…"}`；否则 Create + Run。
- `output.go`：三种输出模式（03 接口规范 1.2 节）；人类可读模式在 stderr 打印 `[步骤 main · codex] 正在运行 go test` 这样的行，结束后在 stdout 打印状态、摘要、产物路径、diff 文件路径、任务 ID，以及"查看详情：xcli result <id>"；退出码按 03 接口规范 1.3 节。
- `tasks.go`：`status`（人类可读摘要，`--json` 输出 Record）、`wait`（`--timeout`，超时退出码 1 但不取消任务）、`logs`（`-f` 跟随，`--after N`）、`result`、`cancel`、`list`（表格：ID、状态、能力、目标前 40 字、创建时间；`--json`）、`resume <id> <指令…>`（同样支持 `--json` / `--events` / `--detach`）、`apply <id> [--remove-worktree]`、`gc --older-than 7d`。
- `worker.go`：隐藏命令 `__worker <id>`：Load 任务后 Run，sink 为 nil。

- [ ] **步骤 5：写命令行测试**

`cmd/run_test.go`：设置 `XCLI_HOME=t.TempDir()`，写入 `config.yaml`（`path: <fake 目录>:/bin:/usr/bin`），通过 `newRootCommand()` + `SetArgs` + 捕获 stdout 执行：
- `ask hello --json` → stdout 是合法 JSON，`status == "completed"`。
- `submit --request -`，stdin 为 `{"capability":"ask","goal":"hi"}`，加 `--events` → 每行都是合法 JSON，最后一行 `type == "task.finished"`。
- `ask --json`（缺少 goal）→ status invalid，退出码 2。
- `code fix -C <非 git 临时目录> --no-review --json` → completed，`changes.files` 非空。

- [ ] **步骤 6：手动冒烟（真实 CLI，征得用户同意，约 2 次调用）**

```bash
go build -o bin/xcli .
./bin/xcli ask "用一句话介绍你自己" --json
./bin/xcli ask "用一句话介绍你自己" -a antigravity
```

- [ ] **步骤 7：提交**

```bash
go test ./... -race
git add internal/engine cmd && git commit -m "feat: add engine and task commands (ask/code/review/image/submit)"
```

**验收：** 03 接口规范第 1 节除 `do`（任务 17 完成规划后才智能化）、`serve` 外的命令全部可用。

---

## 第 3 阶段：智能编排

### 任务 17：CLI 规划器与 `do` / `plan`

**文件：**
- 新建：`internal/orchestrator/planner.go`
- 修改：`internal/engine/engine.go`（capability auto 时调用规划器）
- 测试：`internal/orchestrator/planner_test.go`

**接口：**
- 消费：`recipe.RenderPlanner`、`recipe.PlanSchema`、`recipe.ExtractJSON`、`recipe.Validate`、`ValidatePlan`、`RulesPlan`、`Env.invoke`。
- 产出：

```go
// Plan 调用规划 CLI 生成计划；任何失败都退回 RulesPlan，并通过 notice 事件说明原因。返回的 Plan.Source 为 "planner" 或 "rules"。
func (e *Env) Plan(ctx context.Context) api.Plan
```

- [ ] **步骤 1：写测试（fakecli）**

- `TestPlannerSuccess`：codex 回放一个合法计划（两个并行步骤 + 一个依赖它们的 code 步骤）→ `Source == "planner"`，步骤与回放内容一致；fakecli 参数包含 `--output-schema` 和 `-s read-only`。
- `TestPlannerInvalidFallsBack`：codex 返回有环的计划 → `Source == "rules"`，有 notice 事件，内容包含"计划校验失败"。
- `TestPlannerUnavailableTriesNext`：codex 返回 auth 错误 → 改用下一个 plan 分数最高的 CLI（antigravity，参数包含 `--json-schema`）生成计划。
- `TestPlannerTimeout`：所有规划 CLI 都超时（`planner_timeout = 300ms`）→ `Source == "rules"`。
- `TestPlannerPromptContents`：`prompt.md` 中包含需求、可用 CLI 列表及 strengths、`max_steps`、workdir 顶层条目。

- [ ] **步骤 2：实现**

要点：
- 规划 CLI 的候选：先 `cfg.Defaults.Planner`，再按 plan 分数排序的其他可用 CLI；`Req.Agent` 非空时只用它；最多尝试 2 个。
- 规划调用：权限 read，工作目录为 `WS.Dir`（没有 workdir 时用任务临时目录），步骤 ID 固定为 `plan`，round 为 1，超时 `planner_timeout`。
- engine 中 capability auto 的分支改为调用 `Env.Plan`（工作区在规划前已经准备好，见任务 16 步骤 3 的第 3 点）。
- `AgentInfo.Note` 默认值：codex `OpenAI Codex：擅长编码、规划，内置 image_gen 可生图`；cursor `Cursor Agent：擅长编码与代码审查`；antigravity `Google Antigravity：擅长长文本、资料分析，内置 image-generator 子代理可生图`。

- [ ] **步骤 3：真实冒烟（征得用户同意，约 4～6 次调用）**

```bash
go build -o bin/xcli .
D=$(mktemp -d) && git -C "$D" init -q && echo '# demo' > "$D/README.md" && git -C "$D" add . && git -C "$D" -c user.name=x -c user.email=x@x commit -qm init
./bin/xcli plan "给 README 写一段项目介绍，并生成一张封面图放到 docs/cover.png，最后在 README 里引用它" -C "$D" --json
./bin/xcli do   "给 README 写一段项目介绍，并生成一张封面图放到 docs/cover.png，最后在 README 里引用它" -C "$D"
```

期望：计划中有一个文案步骤和一个生图步骤（并行），以及一个依赖两者的 code 步骤；执行后 README 有新内容，`docs/cover.png` 是合法图片。

- [ ] **步骤 4：提交**

```bash
go test ./... -race
git add internal/orchestrator internal/engine && git commit -m "feat(orchestrator): add CLI-driven planner for do/plan"
```

---

## 第 4 阶段：HTTP 接口（可选）

### 任务 18：`xcli serve`

**文件：**
- 新建：`internal/server/server.go`、`cmd/serve.go`
- 测试：`internal/server/server_test.go`

**接口：**
- 消费：`engine.Engine` 的全部方法。
- 产出：

```go
func New(e *engine.Engine, token string) http.Handler
func LoadOrCreateToken(home string) (string, error) // <home>/server.token，32 字节随机数的十六进制，权限 0600
```

- [ ] **步骤 1：写测试（`httptest.NewServer` + fakecli）**

- `TestHealthNoAuth`：`GET /v1/health` 返回 200 和版本号。
- `TestAuthRequired`：不带令牌访问 `/v1/tasks` 返回 401。
- `TestCreateAndGet`：`POST /v1/tasks` 返回 202 和 task_id；轮询 `GET /v1/tasks/{id}` 直到 completed。
- `TestCreateWait`：`POST /v1/tasks?wait=true` 直接返回 Result。
- `TestInvalidRequest`：缺少 goal → 400，响应体为 `InvalidResult`。
- `TestEventsSSE`：读取 `/v1/tasks/{id}/events`，收到 `event: task.finished` 后连接关闭；`?after=2` 只返回 seq > 2 的事件。
- `TestCancel`：`POST /v1/tasks/{id}/cancel` 后状态为 cancelled。
- `TestArtifactDownload`：image 任务完成后 `GET /v1/tasks/{id}/artifacts/0` 返回图片字节，Content-Type 正确；`/artifacts/9` 返回 404。

- [ ] **步骤 2：实现**

使用 Go 1.22+ 的路由模式（`mux.HandleFunc("POST /v1/tasks", …)`）。任务在 serve 进程内的 goroutine 中运行，用 `context.WithCancel` 保存每个任务的取消函数（`Cancel` 优先调用它，不在 map 中时退回发信号）。serve 收到 SIGTERM 时取消所有运行中的任务并等待它们写完结果。`cmd/serve.go`：`--listen`（默认 `127.0.0.1:7788`），启动时在 stderr 打印监听地址和令牌文件路径。

- [ ] **步骤 3：提交**

```bash
go test ./... -race
git add internal/server cmd && git commit -m "feat(server): add local HTTP task API"
```

---

## 第 5 阶段：收尾

### 任务 19：冒烟脚本、文档与发布配置

**文件：**
- 新建：`scripts/smoke.sh`、`docs/v2/USER_GUIDE.md`
- 修改：`README.md`、`README.zh-CN.md`、`docs/v2/README.md`、`.goreleaser.yaml`

- [ ] **步骤 1：`scripts/smoke.sh`**

`set -euo pipefail`；运行前打印"将进行约 10 次真实 CLI 调用，会消耗各 CLI 的额度，按回车继续"。在临时 git 仓库里依次执行，每条都用 `--json`，用 `python3 -c` 或 `grep` 检查 `status`：

1. `xcli agents`
2. 三个 CLI 各一次 `ask -a <agent> "用一句话说明 add.go 的作用" -C <repo>`
3. `ask "按 Schema 输出 add.go 的函数列表" --schema <schema>`，检查 `output` 非空
4. `image "米白背景上的绿色植物插画，方形，无文字" -o <out>`，检查 `artifacts` 非空
5. `code "为 Add 写一个表驱动测试" -C <repo> --verify "go test ./..."`，检查 `review` 和 `verify`
6. `do "给 README 写一段介绍并配一张封面图" -C <repo>`

每一步打印 PASS / FAIL 和任务 ID，最后汇总。

- [ ] **步骤 2：`docs/v2/USER_GUIDE.md`（一页）**

内容：安装；第一次运行（`xcli agents` 确认三个 CLI 已登录）；五个能力各一个示例；后台任务（`--detach` / `wait` / `logs` / `cancel`）；`resume` 继续修改；`--isolate` + `apply`；配置文件常用项（关闭某个 CLI、改模型、改默认权限）；常见问题（未登录、额度用尽后的自动切换与 `agents --reset`、GUI 应用找不到命令时配置 `path`、Cursor 工作区信任已由 xcli 自动处理）。

- [ ] **步骤 3：更新 README、发布配置并统计代码规模**

```bash
git ls-files '*.go' | grep -v _test.go | xargs wc -l | tail -1   # 记录到提交说明；超过 8000 行先找用户复盘
goreleaser check 2>/dev/null || true
go build ./... && go vet ./... && go test ./... -race
git add -A && git commit -m "docs: add user guide, smoke script and release config for v2"
```

- [ ] **步骤 4：请用户运行 `scripts/smoke.sh`，根据结果修复问题**

---

## v2.0 完成标准

| 成功标准（01 文档第 2 节） | 由哪个任务保证 | 如何验证 |
| --- | --- | --- |
| 零配置 `xcli ask "你好" --json` 首次即成功 | 4、6、16 | 删除 `~/.xcli` 后运行 |
| 一次子进程调用拿到文本 / JSON / 图片 / 代码改动 | 11–16 | `scripts/smoke.sh` 第 2～5 步 |
| code 默认完成实现-验证-审查-修复闭环 | 14、15 | `TestRunStepCodeReviewLoop`、冒烟第 5 步 |
| `do` 自动生成多 CLI 计划并执行，`plan` 只规划 | 15、17 | `TestPlannerSuccess`、冒烟第 6 步 |
| CLI 不可用时自动切换并记录原因 | 9、15 | `TestExecuteFallbackOnQuota` |
| 一页用户手册；非测试代码 ≤ 6000 行（上限 8000） | 19 | `USER_GUIDE.md`；行数统计 |
