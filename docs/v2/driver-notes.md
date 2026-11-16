# 原生 CLI 实测记录

实测日期：2026-11-16；平台：macOS arm64。样本来自真实 CLI，保存在 `internal/driver/testdata/<agent>/`。`/TMP` 表示该次实测临时目录，`/HOME` 表示用户主目录，`/CODEX_HOME` 表示原生 Codex 数据目录。每个 `.jsonl` 配有同名 `.stderr`。

使用 `scripts/probe` 的六个固定用例；每个用例在临时仓库中执行，超时 10 分钟。下列 argv 中的路径均已脱敏；`<ext>` 是生图提示词中的原样占位文字。

## Codex

版本：`codex-cli 0.169.0`。

- `ask_ok`、`schema_ok`、`resume_ok`、`write_ok`、`image_ok` 均退出 0；`write_ok` 创建了内容为 `hi` 的 `hello.txt`；`error_bad_model` 退出 1。
- 会话 ID：`thread.started.thread_id`；续接样本与 ask 的 ID 一致，`exec resume ... -- <sid> <prompt>` 正确解析。
- 最终文本：最后一个 `item.completed` 且 `item.type=agent_message` 的 `item.text`。Schema 样本的最终文本是含 `answer` 的 JSON。
- 结束与用量：`turn.completed.usage` 的 `input_tokens`、`output_tokens`；另有 `cached_input_tokens`、`cache_write_input_tokens`、`reasoning_output_tokens`。例如 ask 为输入 51834、输出 114。
- 事件：`thread.started`、`turn.started`、`item.started`、`item.completed`、`turn.completed`；错误用例另有 `error`、`turn.failed`。item 类型包含 `agent_message`、`command_execution`、`error`。
- 错误模型：`error.message` 与 `turn.failed.error.message` 都含 `model is not supported when using Codex with a ChatGPT account`。stderr 通常仅有 `Reading additional input from stdin...`。
- write、生图均完成；没有出现待处理审批。图片通过内置 imagegen 技能生成，原始路径为 `/CODEX_HOME/generated_images/<session>/exec-<id>.png`，随后复制到 `/TMP/out/image_ok-1.png`。
- `artifact_dirs` 默认使用原生环境变量 `CODEX_HOME` 对应目录下的 `generated_images`；变量为空时使用 `~/.codex/generated_images`。
- 4.2 参数映射沿用当前映射。

### 六个基线用例的 argv

```sh
# ask_ok；耗时 19.994s；退出码 0
codex exec --json --skip-git-repo-check -C /TMP/repo -s read-only -- '用一句话说明 add.go 中 Add 函数的作用。'
# schema_ok；耗时 15.773s；退出码 0
codex exec --json --skip-git-repo-check -C /TMP/repo -s read-only --output-schema /TMP/schema.json -- '用一句话说明 add.go 的作用，按 JSON Schema 输出。'
# resume_ok；耗时 6.81s；退出码 0
codex exec resume --json --skip-git-repo-check -c 'sandbox_mode="read-only"' -- 01a1e22a-60d2-74c2-8770-0b068e5a72b5 '我上一个问题问的是哪个函数？只回答函数名。'
# write_ok；耗时 9.417s；退出码 0
codex exec --json --skip-git-repo-check -C /TMP/repo -s workspace-write -c sandbox_workspace_write.network_access=true -- '在当前目录创建 hello.txt，内容为 hi，不要做其他修改。'
# image_ok；耗时 49.855s；退出码 0
codex exec --json --skip-git-repo-check -C /TMP/repo -s workspace-write -c sandbox_workspace_write.network_access=true --output-schema /TMP/image-schema.json --add-dir /TMP/out -- '使用内置 image_gen 工具，生成一张米白背景上的绿色植物插画，方形，无文字。保存到 /TMP/out/image_ok-1.<ext>，最终只回复 {"files":["<绝对路径>"]}。'
# error_bad_model；耗时 3.093s；退出码 1
codex exec --json --skip-git-repo-check -C /TMP/repo -s read-only -m nonexistent-model-xyz -- '用一句话说明 add.go 中 Add 函数的作用。'
```

## Cursor

版本：`2026.10.22-27dcf5f`。

- 五个成功用例均退出 0，write 创建 `hello.txt`；错误模型用例退出 1。`--trust` 下没有工作区信任确认。
- 会话 ID：`system.session_id`、`result.session_id`。`--resume` 续接同一会话，最终回复 `Add`。
- 最终文本：最后一条 `assistant.message.content[].text`。`result.result` 拼接了中间说明和最终回复，只作为兜底；Schema 样本的最后一条 assistant 是合法 JSON。
- 结束：`type=result`、`subtype=success`、`is_error=false`。
- 用量：`result.usage.inputTokens`、`outputTokens`，另有 `cacheReadTokens`、`cacheWriteTokens`、`reasoningTokens`；例如 ask 为输入 20473、输出 227。
- 事件：`system`、`user`、`thinking`、`assistant`、`tool_call`、`result`。
- 错误模型的 stdout 为空；stderr 以 `Cannot use this model:` 开头，并列出可用模型。解析器结合非零退出码与 stderr 判定失败。
- 生图使用内置 `GenerateImage`，原始文件位于 `~/.cursor/projects/<workspace-key>/assets/image_ok-1.jpg`，随后复制到 `/TMP/out/image_ok-1.jpg`。默认 `artifact_dirs` 使用 `~/.cursor/projects`，从该目录扫描三层可以找到 assets 中的文件。
- 追加核实：`cursor-agent -p --output-format stream-json --trust --workspace /TMP --mode ask -- '<prompt>'` 退出 0 并正常回答；有无位置参数分隔符均可，4.2 既有映射保持原样。

### 六个基线用例的 argv

```sh
# ask_ok；耗时 36.225s；退出码 0
cursor-agent -p --output-format stream-json --trust --workspace /TMP/repo --mode ask '用一句话说明 add.go 中 Add 函数的作用。'
# schema_ok；耗时 34.561s；退出码 0
cursor-agent -p --output-format stream-json --trust --workspace /TMP/repo --mode ask '用一句话说明 add.go 的作用，按 JSON Schema 输出。最终只输出符合以下 JSON Schema 的 JSON：{"type":"object","additionalProperties":false,"required":["answer"],"properties":{"answer":{"type":"string"}}}'
# resume_ok；耗时 25.648s；退出码 0
cursor-agent -p --output-format stream-json --trust --workspace /TMP/repo --mode ask --resume d66f83b8-0f2d-4f64-a16e-6aed571f6ff1 '我上一个问题问的是哪个函数？只回答函数名。'
# write_ok；耗时 29.731s；退出码 0
cursor-agent -p --output-format stream-json --trust --workspace /TMP/repo --force --sandbox enabled '在当前目录创建 hello.txt，内容为 hi，不要做其他修改。'
# image_ok；耗时 1m1.711s；退出码 0
cursor-agent -p --output-format stream-json --trust --workspace /TMP/repo --force --sandbox enabled --add-dir /TMP/out '使用内置 GenerateImage 工具，生成一张米白背景上的绿色植物插画，方形，无文字。保存到 /TMP/out/image_ok-1.<ext>，最终只回复 {"files":["<绝对路径>"]}。'
# error_bad_model；耗时 14.689s；退出码 1
cursor-agent -p --output-format stream-json --trust --workspace /TMP/repo --mode ask --model nonexistent-model-xyz '用一句话说明 add.go 中 Add 函数的作用。'
```

## Antigravity

版本：`1.5.2`。

- 当前 `ask_ok/schema_ok/resume_ok` 采用追加实测验证的会话级只读设置，三者均直接完成且退出 0；`write_ok/image_ok` 也完成，错误模型退出 1。初始 plan 模式的权限失败保存在 `error_plan_permission`，原始六组基线样本还可从提交 `2c774a7` 查看。
- 会话 ID：`step_update.conversation_id`、`result.conversation_id`；续接成功并回复 `Add`。
- 最终文本：普通结果在 `result.response`；有原生 Schema 时优先使用 `result.structured_output`。image 的 response 包含一段解释和 JSON，structured_output 则是完整 `{"files":[...]}` 对象。
- 结束：`event=result`；正常状态为 `result.status=SUCCESS`。用量：`result.usage.input_tokens`、`output_tokens`、`total_tokens`，另有 `thinking_tokens`、`cache_read_tokens`。
- 事件：`init`、`step_update`、`result`。工具进度位于 `step_update.tool_name`、`tool_info`，状态有 `ACTIVE`、`DONE`、`ERROR`。
- 错误模型：`result.status=ERROR`、`result.error` 含 `invalid model selection`，response 为空；stderr 也输出同一错误。
- write、生图使用 `--dangerously-skip-permissions --sandbox`，均完成，没有出现 denied_actions。图片先保存到 `~/.gemini/antigravity-cli/brain/<conversation>/image_ok_1_<timestamp>.jpg`，再复制到 `/TMP/out/image_ok-1.jpg`。默认 `artifact_dirs` 使用 `~/.gemini/antigravity-cli/brain`。
- `agy --help` 明确说明 `--print-timeout 0` 为 `0 waits until the turn completes`，沿用该映射。

### 初始 plan 模式的实测问题

`--mode plan` 在 ask/schema 中尝试执行 `find . -name "add.go"`。headless 模式自动拒绝 command 权限；最终仍输出 SUCCESS、空正文：

```json
{"status":"SUCCESS","response":"","denied_actions":[{"action":"command","display_name":"RunCommand"}]}
```

stderr 提示 `no output produced`、`headless mode cannot prompt`。`denied_actions` 的元素是对象；驱动提取其中 `action`，也兼容已有字符串样本。

追加调用的 argv：

```sh
agy --output-format stream-json --print-timeout 0 --mode plan --sandbox --print '用一句话说明 add.go 中 Add 函数的作用。'
```

该调用退出 0，能读取文件，但最终回复“已针对任务制定了实现计划”并要求“点击 Proceed 批准执行”。后续六次实测确认了下方的默认模式与会话级权限方案。

### 初始六个基线用例的 argv（历史样本）

```sh
# ask_ok；耗时 11.045s；退出码 0
agy --output-format stream-json --print-timeout 0 --mode plan --print '用一句话说明 add.go 中 Add 函数的作用。'
# schema_ok；耗时 10.101s；退出码 0
agy --output-format stream-json --print-timeout 0 --mode plan --json-schema /TMP/schema.json --print '用一句话说明 add.go 的作用，按 JSON Schema 输出。'
# resume_ok；耗时 11.835s；退出码 0
agy --output-format stream-json --print-timeout 0 --mode plan --conversation 3e48abdf-4354-49e0-a599-8d534aa3f435 --print '我上一个问题问的是哪个函数？只回答函数名。'
# write_ok；耗时 13.479s；退出码 0
agy --output-format stream-json --print-timeout 0 --dangerously-skip-permissions --sandbox --print '在当前目录创建 hello.txt，内容为 hi，不要做其他修改。'
# image_ok；耗时 1m30.27s；退出码 0
agy --output-format stream-json --print-timeout 0 --dangerously-skip-permissions --sandbox --json-schema /TMP/image-schema.json --add-dir /TMP/out --print '调用内置 image-generator 子代理（通过子代理调用），生成一张米白背景上的绿色植物插画，方形，无文字。保存到 /TMP/out/image_ok-1.<ext>，最终只回复 {"files":["<绝对路径>"]}。'
# error_bad_model；耗时 5.69s；退出码 1
agy --output-format stream-json --print-timeout 0 --mode plan --model nonexistent-model-xyz --print '用一句话说明 add.go 中 Add 函数的作用。'
```

### 追加六次：会话级只读设置

使用原生 `--sandbox --settings '<JSON>'`，settings 内容为：

```json
{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}
```

`agentMode` 空字符串为默认执行模式。提示词引导使用 `view_file/list_dir/find_by_name/grep_search` 等内置工具，直接完成任务。成功的读取在事件中显示为 `tool_name=read_file`。六次均检查了临时工作区的文件列表和 SHA-256，内容保持原样；检查持久 settings 后，会话级覆盖键和 deny 规则均未写入。

| 用例 | 结果 | 文件变化 |
| --- | --- | --- |
| ask_ok | 直接说明 Add 作用 | 无 |
| schema_ok | structured_output.answer 为中文说明 | 无 |
| resume_ok | 同一 conversation_id，回复 Add | 无 |
| error_write_denied | write_file 被显式 deny 规则拒绝 | 无，native-write.txt 未创建 |
| error_command_denied | run_command 被显式 deny 规则拒绝 | 无，shell-write.txt 未创建 |
| plan_ok | structured_output 包含 ask → review 两步计划 | 无 |

两类显式规则拒绝的 result 仍为 SUCCESS，且没有 denied_actions；拒绝发生在 `step_update.state=ERROR` 的 `tool_info.error.message`：包含 `Permission denied`、`Matches user-configured deny rule`。因此驱动还要读取工具级明确权限错误，映射到 `permission_denied`，Denied 为 `write_file` 或 `command`。原生 CLI 在其他工具错误后可以自行恢复，解析器只对明确权限拒绝作这一处理。

```sh
# ask_ok；耗时 27.8s；退出码 0
agy --output-format stream-json --print-timeout 0 --sandbox --settings '{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}' --print '你正在被 xcli 调度执行只读任务。请直接完成，用中文回复；使用 view_file、list_dir、find_by_name、grep_search 等内置只读工具读取文件，不要调用 run_command，不要修改文件，不要制定实现计划，不要请求确认。用一句话说明 /TMP/repo/add.go 中 Add 函数的作用。'
# schema_ok；耗时 20.703s；退出码 0
agy --output-format stream-json --print-timeout 0 --sandbox --settings '{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}' --json-schema /TMP/schema.json --print '你正在被 xcli 调度执行只读任务。请直接完成，用中文回复；使用 view_file、list_dir、find_by_name、grep_search 等内置只读工具读取文件，不要调用 run_command，不要修改文件，不要制定实现计划，不要请求确认。读取 add.go 并按指定 Schema 输出作用说明，answer 使用中文。'
# resume_ok；耗时 23.341s；退出码 0
agy --output-format stream-json --print-timeout 0 --sandbox --settings '{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}' --conversation df25ea9f-fc40-4bdd-a6b2-5e75c54888e8 --print '你正在被 xcli 调度执行只读任务。请直接完成，用中文回复；使用 view_file、list_dir、find_by_name、grep_search 等内置只读工具读取文件，不要调用 run_command，不要修改文件，不要制定实现计划，不要请求确认。我上一个问题问的是哪个函数？只回答函数名。'
# write_denied；耗时 18.847s；退出码 0
agy --output-format stream-json --print-timeout 0 --sandbox --settings '{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}' --print '这是临时测试目录中的权限负向验证。请使用 write_to_file 工具在当前目录创建 native-write.txt，内容为 native-write。不要使用命令行或其他替代工具，若权限拒绝则直接说明并停止。'
# command_denied；耗时 31.653s；退出码 0
agy --output-format stream-json --print-timeout 0 --sandbox --settings '{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}' --print '这是临时测试目录中的权限负向验证。请使用 run_command 执行 printf shell-write > shell-write.txt。不要使用文件编辑工具或其他替代方式，若权限拒绝则直接说明并停止。'
# plan_ok；耗时 97.572s；退出码 0
agy --output-format stream-json --print-timeout 0 --sandbox --settings '{"agentMode":"","toolPermission":"request-review","artifactReviewPolicy":"always-proceed","permissions":{"deny":["write_file(*)","command(*)","mcp(*)"]}}' --json-schema /TMP/plan-schema.json --print '你正在被 xcli 调度执行只读任务。请直接完成，用中文回复；使用 view_file、list_dir、find_by_name、grep_search 等内置只读工具读取文件，不要调用 run_command，不要修改文件，不要制定实现计划，不要请求确认。请读取 add.go，为“说明 Add 函数作用并审查其实现”生成包含一个 ask 步骤与一个依赖该步骤的 review 步骤的 xcli 计划。只返回符合指定 Schema 的 JSON；生成这份 JSON 就完成本次任务。'
```

依据：[原生 settings Schema](https://antigravity.google/schemas/v1/settings.json)、[执行模式](https://antigravity.google/docs/cli/modes/)、[权限规则](https://antigravity.google/docs/permissions)。本节行为以追加六次实测为准。

后续健康探测（2026-11-16，仅版本与登录状态）：Codex `0.169.0`、Cursor `2026.11.13-eacade1`、Antigravity `1.5.9`；Codex/Cursor 登录状态为 ok，Antigravity 为 unknown。上文保留各次实测时的参数和结果。
