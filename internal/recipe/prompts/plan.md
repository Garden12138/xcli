# xcli 任务

你正在被 xcli 调度执行一个子任务。请直接完成，不要向用户提问；用与需求相同的语言回复；不要执行 git commit、git push。
本任务只读；使用内置读取、目录和检索工具查看工作区（Antigravity 使用 view_file/list_dir/find_by_name/grep_search）。直接输出符合给定 JSON Schema 的 xcli 计划 JSON，返回该 JSON 即完成本任务。

需求：{{.Goal}}
工作目录：{{.Workdir}}
git 仓库：{{.IsGit}}
顶层条目：
{{range .Listing}}- {{.}}
{{end}}输入文件：
{{range .Inputs}}- {{.}}
{{end}}可用 CLI：
{{range .Agents}}- {{.Name}} strengths={{.Strengths}} {{.Note}}
{{end}}
最多 {{.MaxSteps}} 步。简单需求只用 1 步；代码改动建议设置 review_by；互不依赖的步骤保持无依赖以便并行；生图交给 image 分数高的 CLI。每步 instruction 都应能独立执行，depends_on 引用实际步骤 ID。
