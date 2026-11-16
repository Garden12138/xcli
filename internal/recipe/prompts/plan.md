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
步骤类型和权限：ask 步骤只返回文本（或所需的结构化答案），交接内容由 xcli 放进下游提示词，由下游 code 步骤写入文件。ask/review 都是只读步骤；instruction 只安排读取、分析、文案或审查，output 设为 null。读取文件时使用工作目录内文件的绝对路径。image 负责生图并保存产物；code 负责创建、修改、复制项目文件并执行验证命令。
{{if ge .MaxSteps 3}}需求同时包含文案、配图和写入项目时，文案和图片互不依赖：安排独立的 ask 文案步骤与 image 生图步骤并行，再让 code 步骤依赖两者，使用交接的文案和图片完成项目改动。
{{end}}output 是产物子目录名（例如 cover）或 null；目标文件名写在 instruction 中。image 步骤把图片保存到 xcli 提供的输出目录，code 步骤读取交接的产物路径，复制到需求指定的位置并引用。
