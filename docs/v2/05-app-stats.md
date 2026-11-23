# 05 接入应用统计（v2.1.0）

日期：2026-11-23。

## 1. 目标与范围

新增 `xcli stats`，在终端查看各接入应用（Worklog、GardenFlow、Chat Insight 及终端直接使用）调用 xcli 的情况：

- 调用量与成功率、最近使用时间；
- 耗时（平均、P50、P90）；
- 各 CLI 的调用次数、成功率、失败切换；
- 失败原因分布；
- Token 用量（CLI 上报了才有）；
- 按天趋势。

输出为终端表格，`--json` 输出完整结构。不做 HTML 报告和 HTTP 接口。统计包含 Chat Insight 在沙箱中运行的记录，只读扫描，不修改 Chat Insight 的沙箱。

## 2. 数据来源

### 2.1 统计账本

任务结束时向 `<home>/stats/usage.jsonl` 追加一行。`xcli gc` 只清理任务目录，不动账本，所以清理旧任务后统计仍在。

```json
{"v":1,"task_id":"t-20261119-030133-ea72","app":"gardenflow","capability":"image","status":"completed","error_kind":"","error_step":"","started_at":"2026-11-19T03:01:33Z","finished_at":"2026-11-19T03:02:34Z","duration_ms":61557,"parent_task":"","planner":"",
 "steps":[{"id":"main","kind":"image","agent":"antigravity","status":"completed","duration_ms":61200,"rounds":1,"fallbacks":[],"input_tokens":0,"output_tokens":0,"has_usage":false}],
 "reviews":[],
 "calls":[{"agent":"antigravity","purpose":"run","step":"main","round":1,"duration_ms":61150,"error_kind":"","input_tokens":0,"output_tokens":0,"has_usage":false}],
 "approximate":false}
```

| 字段 | 来源 |
| --- | --- |
| `app` | `request.metadata.app`，规范化见第 3 节 |
| `status`、`error_kind`、`error_step` | `result.status`、`result.error.kind`、`result.error.step_id` |
| `steps[]` | `result.steps[]`：最终执行的 CLI、状态、耗时、轮数、切换记录、用量 |
| `steps[].fallbacks` | 由 `"codex: quota"` 形式拆成 `[{"agent":"codex","reason":"quota"}]` |
| `reviews[]` | 任务事件中每条 `review.result`：`{"step":"main","agent":"cursor","round":1,"verdict":"pass"}` |
| `planner` | 新增的可选字段 `plan.planner`（由 CLI 规划时记录规划成功的 CLI） |
| `calls[]` | 本任务每一次实际启动 CLI 的记录，由 `orchestrator.Env.invoke` 采集（见下文） |
| `approximate` | `calls[]` 是否为估算：v2.1.0 起正常结束的任务为 `false`；从旧任务补录或 `worker_lost` 的任务为 `true` |

**`calls[]` 的采集：** xcli 启动 CLI 只有一个入口 `Env.invoke`（规划、执行、Schema 修复、代码修复轮、审查都经过它）。每次调用结束时追加一条记录：

| 字段 | 含义 |
| --- | --- |
| `agent` | 本次启动的 CLI |
| `purpose` | `plan`（规划，包括失败的尝试）/ `review`（审查，包括代码循环中的审查和 `review` 类型的步骤）/ `repair`（Schema 修复、代码修复轮，包括续接失败后的新会话重试）/ `run`（其他：步骤的首次执行，切换后在新 CLI 上的执行也算 `run`） |
| `step`、`round` | 所属步骤 ID（规划为 `plan`）和轮次 |
| `duration_ms` | 本次调用从启动进程到结束的耗时 |
| `error_kind` | 本次调用的错误类型；成功为空 |
| `input_tokens`、`output_tokens`、`has_usage` | 本次调用上报的用量 |

找不到可执行文件、没有启动进程时不记录；调用中途被取消时照常记录，`error_kind` 为 `cancelled`。采集需要并发安全（并行步骤会同时调用）。`worker_lost` 时进程已经退出，内存中的采集结果丢失，按补录规则估算。

**隐私：** 账本只记录上面两张表中的字段，不记录目标描述、提示词、输入路径、输出文本、产物路径或除 `app` 以外的 metadata。

**写入：** 打开时使用 `O_APPEND|O_CREATE|O_WRONLY`（文件 0600，目录 0700），加 `flock` 后写入一整行。写入失败不影响任务结果，只在 stderr 输出一行提示。写入时机：

1. `engine.Run` 收尾、`task.finished` 事件写入之后（前台、后台 worker、HTTP 服务都经过这里）；
2. `engine.Status` 把任务判定为 `worker_lost` 之后。

`invalid` 请求不建任务，不记录。`planned`（只规划）会记录，因为规划也调用了 CLI。

### 2.2 补录与多个数据目录

`xcli stats` 读取以下数据目录，结果按 `task_id` 去重：

1. 当前数据目录（默认 `~/.xcli`）；
2. 配置 `stats.include` 中的数据目录，相对路径基于当前数据目录展开 glob。默认值是 `["chat-insight/*/xcli"]`，即 Chat Insight 每次沙箱运行的 `XCLI_HOME`。

每个数据目录依次读取：

1. `stats/usage.jsonl`：损坏的行跳过并计数；
2. `tasks/*/task.json`：目录名（即任务 ID）已在账本中出现的直接跳过、不读取；其余已结束的任务用同一个转换函数补录，审查信息从同目录的 `events.jsonl` 读取，`calls[]` 按第 4 节的估算规则生成，`approximate` 为 `true`。

这样升级前的历史任务、Chat Insight 用旧版 xcli 留下的任务都能统计到。

## 3. 调用方标识

- 约定：调用方在请求里设置 `metadata.app`。现有三个应用已经设置了：`worklog`、`gardenflow`、`chat-insight`。
- 规范化：去掉首尾空白、转小写；必须匹配 `^[a-z0-9][a-z0-9._-]{0,39}$`，否则记为 `unknown`；缺失也记为 `unknown`。
- 终端直接使用：`do`、`plan`、`ask`、`code`、`review`、`image` 新增 `--app <名称>`；没有给出且请求中也没有 `metadata.app` 时，默认填 `terminal`。`submit` 和 HTTP 接口不填默认值，缺失时记为 `unknown`。
- `resume` 继承父任务的 `metadata.app`，`--app` 可以覆盖。
- `--app` 的值不符合上面的格式时，返回 `invalid`，退出码 2。

## 4. 统计口径

| 指标 | 定义 |
| --- | --- |
| 任务数 | 时间范围内结束的任务，按 `finished_at` 归属（没有时用 `started_at`） |
| 成功、需关注、失败、取消 | 成功 = `completed` + `planned`；需关注 = `needs_attention`；失败 = `failed`；取消 = `cancelled` |
| 成功率 | 成功 ÷（成功 + 需关注 + 失败）；取消不计入分母；分母为 0 时显示 `—` |
| 应用耗时 | 任务 `duration_ms`，不含取消的任务 |
| P50、P90 | 最近秩法（nearest-rank）：排序后取第 ⌈p × n⌉ 个 |
| CLI 调用 | `calls[]` 的条数，按 `agent` 归组；调用成功 = `error_kind` 为空。精确记录直接使用 `calls[]`；估算记录的 `calls[]` 由下一行规则生成 |
| 估算规则（`approximate: true`） | 每条切换记录 → 该 CLI 一次失败的 `run`（`error_kind` 取切换原因）；每个步骤的最终 CLI → `max(rounds, 1)` 次调用，前面的轮次记为成功的 `repair`，最后一次的成败取步骤状态（`completed`、`needs_attention` 为成功）；`code` 步骤的每条 `review.result` → 审查 CLI 一次成功的 `review`；`review` 类型步骤只按步骤本身计 1 次，不再计它的 `review.result`；`planner` 非空 → 一次成功的 `plan`（失败的规划尝试无法估算）。Schema 修复无法从旧记录识别，不计 |
| CLI 耗时 | 精确记录用每次调用的 `duration_ms`。估算记录只使用"没有切换且只有一轮"的步骤：步骤耗时归到最终 CLI 的那次调用；其他估算调用不参与耗时统计 |
| 被切走、接手 | 被切走 = 该 CLI 出现在切换记录中的次数；接手 = 发生过切换的步骤最终由该 CLI 完成的次数 |
| 失败原因 | 失败任务的 `error_kind`，并列出涉及的应用和 CLI（`error_step` 对应步骤的最终 CLI） |
| Token | 应用和总览按步骤汇总 `has_usage` 为真的步骤，同时显示有用量数据的步骤数 ÷ 总步骤数；CLI 按 `calls[]` 汇总（估算记录的步骤用量归到最终 CLI 的最后一次调用） |
| 按天 | 本机时区自然日；范围内每天一行，没有任务的日子也显示 |

## 5. 命令

```
xcli stats [--since 30d] [--until YYYY-MM-DD] [--app NAME] [--json]
```

- `--since`：`7d` / `30d` / `90d` / `all` / `YYYY-MM-DD`，默认 `30d`。
- `--until`：`YYYY-MM-DD`（含当天），默认现在。
- `--app`：只看某个应用。
- 参数不合法时以退出码 2 退出，并说明正确格式。

终端输出示例（数字仅为示意）：

```
xcli 应用统计 · 2026-10-25 ～ 2026-11-23（近 30 天，Asia/Shanghai）
任务 37 · 成功率 60%（成功 21 / 需关注 0 / 失败 14）· 取消 2
Token：输入 1.2M / 输出 34K（12/40 个步骤有用量数据）

按应用
应用         任务  成功率  失败  取消  平均耗时  P50   P90    最近使用
gardenflow   31    58%     13    2     1m32s     58s   3m35s  11-19 15:02
worklog      6     83%     1     0     24s       22s   31s    11-21 09:10

按 CLI（含审查和规划）
CLI          调用  成功率  平均耗时  P90    被切走  接手  Token 输入/输出
codex        30    80%     52s       2m36s  1       2     1.1M / 30K
antigravity  9     33%     2m40s     3m35s  2       0     —
cursor       4     100%    1m06s     1m10s  0       1     —

失败原因
原因         次数  占比  涉及应用    涉及 CLI
failed       6     43%   gardenflow  antigravity
unavailable  4     29%   gardenflow  antigravity

按天
日期        任务  成功  失败  平均耗时
11-19 周四  12    7     5     1m48s  ████████████
11-20 周五  0     0     0     —
...

数据来源：账本 30 条，补录 7 条，跳过损坏 0 行；数据目录 2 个
其中 7 个任务为 v2.1.0 之前的记录，CLI 调用次数和耗时为估算值
```

没有估算记录时不显示最后一行。

`--json` 结构：

```json
{
  "version": 1,
  "range": {"since": "2026-10-25T00:00:00+08:00", "until": "2026-11-23T10:30:00+08:00", "timezone": "Asia/Shanghai"},
  "overview": {"tasks": 37, "succeeded": 21, "needs_attention": 0, "failed": 14, "cancelled": 2, "success_rate": 0.6,
               "input_tokens": 1200000, "output_tokens": 34000, "steps_with_usage": 12, "steps": 40},
  "apps": [{"app": "gardenflow", "tasks": 31, "succeeded": 18, "needs_attention": 0, "failed": 13, "cancelled": 2, "success_rate": 0.58,
            "avg_ms": 92000, "p50_ms": 58000, "p90_ms": 215000, "last_used": "2026-11-19T15:02:00+08:00", "input_tokens": 0, "output_tokens": 0}],
  "agents": [{"agent": "codex", "calls": 30, "succeeded": 24, "failed": 6, "success_rate": 0.8, "avg_ms": 52000, "p90_ms": 156000,
              "fallback_from": 1, "fallback_to": 2, "input_tokens": 1100000, "output_tokens": 30000}],
  "errors": [{"kind": "failed", "count": 6, "share": 0.43, "apps": ["gardenflow"], "agents": ["antigravity"]}],
  "daily": [{"date": "2026-11-19", "tasks": 12, "succeeded": 7, "failed": 5, "avg_ms": 108000}],
  "sources": {"ledger_records": 30, "backfilled_tasks": 7, "approximate_tasks": 7, "skipped_lines": 0, "homes": ["~/.xcli", "~/.xcli/chat-insight/0f3a.../xcli"]}
}
```

`success_rate` 分母为 0 时为 `null`。`homes` 中的主目录显示为 `~`。

## 6. 配置

```yaml
stats:
  include: ["chat-insight/*/xcli"]   # 额外统计的数据目录，相对路径基于当前数据目录
```

## 7. 实施计划（给 Codex）

全局约束沿用 `CODEX_HANDOFF.md`：不新增依赖；单元测试不启动真实 CLI；每个任务先写测试再实现，`go build ./... && go vet ./... && go test ./... -race` 通过后单独提交；不 push。

### 任务 1：账本记录与写入（`internal/stats/record.go`、`ledger.go`）

```go
package stats

type Fallback struct{ Agent, Reason string }   // JSON: agent, reason
type StepRecord struct {
	ID, Kind, Agent, Status   string
	DurationMS                int64
	Rounds                    int
	Fallbacks                 []Fallback
	InputTokens, OutputTokens int64
	HasUsage                  bool
}
type ReviewRecord struct{ Step, Agent, Verdict string; Round int }
type Call struct {
	Agent, Purpose, Step      string // Purpose: plan | run | repair | review
	Round                     int
	DurationMS                int64
	ErrorKind                 string
	InputTokens, OutputTokens int64
	HasUsage                  bool
}
type Record struct {
	V                                int
	TaskID, App, Capability, Status  string
	ErrorKind, ErrorStep             string
	StartedAt, FinishedAt            time.Time
	DurationMS                       int64
	ParentTask, Planner              string
	Steps                            []StepRecord
	Reviews                          []ReviewRecord
	Calls                            []Call
	Approximate                      bool
}
// JSON 字段名按第 2.1 节的 snake_case。

func NormalizeApp(value string) string // 第 3 节规则
// calls 为 nil 时按第 4 节估算规则生成 Calls，并置 Approximate = true；未结束的任务返回 false。
func FromTask(rec store.Record, events []api.Event, calls []Call) (Record, bool)
func Append(home string, r Record) error // 第 2.1 节的写入方式
func ReadLedger(path string) (records []Record, skipped int, err error)
```

- **调用采集（`internal/orchestrator`）：** `Env` 增加一个并发安全的调用记录器；`invoke` 增加 `purpose` 参数（取值见第 2.1 节），在每次启动过 CLI 的调用结束时追加一条 `stats.Call`（用量、错误类型、耗时都来自本次调用的结果）。按第 2.1 节的定义更新全部调用点：规划、步骤首次执行、Schema 修复、代码修复轮（含续接失败后的新会话重试）、审查。`stats` 包不得依赖 `orchestrator`，避免循环引用：类型放在 `stats`，由 `orchestrator` 引用。
- `api.Plan` 增加 `Planner string \`json:"planner,omitempty"\``；`orchestrator.Env.Plan` 在 CLI 规划成功时写入所用 CLI。
- 测试（采集，用 fakecli）：ask 步骤 Schema 修复一次 → `run` + `repair` 两条；代码步骤两轮、两次审查 → 编码 CLI 的 `run`、`repair` 各一条，审查 CLI 的 `review` 两条；切换一次 → 原 CLI 失败的 `run`（`error_kind` 为切换原因）+ 新 CLI 的 `run`；规划第一个 CLI 失败、第二个成功 → 两条 `plan`；`review` 类型步骤 → 一条 `review`；调用中途取消 → 一条 `error_kind: cancelled`；可执行文件不存在 → 不记录。
- 测试（记录）：`FromTask` 传入 calls 时原样使用、`Approximate` 为 false；传 nil 时按估算规则生成，覆盖：切换、两轮代码加审查、独立审查步骤只计 1 次、规划；切换记录拆分、审查事件提取、用量有无、`app` 规范化（大写、空白、非法字符、缺失）。`Append` 在 20 个 goroutine 并发写入后，文件恰好 20 行且每行都是合法 JSON；`ReadLedger` 跳过损坏行并计数；账本中不出现 `goal`、`summary` 等字段（对 JSON 键做白名单断言）。

### 任务 2：写入时机与调用方标识（`internal/engine`、`cmd`）

- `engine.Run` 收尾：`task.finished` 写入后调用 `stats.Append(home, FromTask(记录, 事件, 本次采集的 calls))`；`engine.Status` 判定 `worker_lost` 后同样调用，但 calls 传 nil（按估算）。写入失败只向 stderr 输出一行提示。
- `cmd/request_flags.go`：新增 `--app`，按第 3 节校验和填默认值；`submit` 不填默认值。
- `engine.Resume`：继承父任务的 `metadata.app`。
- 测试：前台运行、`--detach` 后台运行、HTTP 运行各产生一行账本；`worker_lost` 产生一行；`xcli gc` 后账本不变；`ask hello`（终端）记为 `terminal`；`submit` 不带 app 时记为 `unknown`；`--app Bad!` 返回 `invalid`、退出码 2；`resume` 继承父任务的 app。

### 任务 3：收集与汇总（`internal/stats/collect.go`、`aggregate.go`）

```go
type Sources struct{ LedgerRecords, BackfilledTasks, ApproximateTasks, SkippedLines int; Homes []string }
func Homes(home string, include []string) []string
func Collect(homes []string) ([]Record, Sources, error)          // 第 2.2 节，按 task_id 去重
type Filter struct{ Since, Until time.Time; App string; Location *time.Location }
type Report struct { /* 与第 5 节 JSON 结构一一对应 */ }
func Aggregate(records []Record, f Filter) Report               // 第 4 节口径
```

- `config` 增加 `stats.include`，默认 `["chat-insight/*/xcli"]`。
- 测试：账本与任务目录重复时只计一次；已在账本中的任务目录不读取 `task.json`（可用损坏的 `task.json` 断言没有报错）；`include` 的 glob 能找到两个模拟的 Chat Insight 数据目录；成功率不含取消；P50、P90 的最近秩法；CLI 调用直接按 `calls[]` 计数与计算耗时；估算记录中"有切换或多轮"的步骤不参与 CLI 耗时；精确与估算记录混合时 `approximate_tasks` 计数正确；被切走、接手；失败原因的涉及应用和 CLI；按天在固定时区（测试里用 `time.FixedZone("CST", 8*3600)`）下补齐空白日期；`--app` 过滤。

### 任务 4：`xcli stats` 命令（`cmd/stats.go`）

- 参数与输出按第 5 节；中文列名对齐时按显示宽度计算（中文字符算 2 列）；耗时格式 `45s`、`1m32s`、`1h05m`；Token 用 `K`、`M` 缩写；按天的条形长度按当天任务数与最大值的比例，最长 20 格。
- 测试：用临时数据目录构造账本，断言表头、各节标题、`--json` 可被解析且字段齐全；`--since 7d`、`--since 2026-11-01`、`--since all` 的范围；`--since foo` 退出码 2；没有任何记录时输出"时间范围内没有任务"。

### 任务 5：文档与发布准备

- `03-interfaces.md`：命令表增加 `xcli stats`；Request 一节说明 `metadata.app` 约定与 `--app`；新增「统计账本」一节，引用本文第 2.1 节。
- `02-architecture.md`：第 9 节存储布局加入 `stats/usage.jsonl`；第 10 节配置加入 `stats.include`。
- `USER_GUIDE.md`：增加「查看接入应用统计」一小节（不超过 10 行）。
- 版本号改为 `2.1.0`，新增 `.github/release-notes/v2.1.0.md`。不打 tag、不推送。
- 用当前机器的真实数据运行一次 `go run . stats` 和 `go run . stats --json`（只读，不消耗额度），把输出贴在汇报里，确认现有 37 个历史任务都被补录进来。
