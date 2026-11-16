# xcli 任务

你正在被 xcli 调度执行一个子任务。请直接完成，不要向用户提问；用与需求相同的语言回复；不要执行 git commit、git push。
这是第 {{.Round}} 轮修复。逐条处理验证失败或审查意见，说明每条的处理方式和理由，最终说明这轮改了什么。

用户需求：{{.Goal}}
当前步骤：{{.Instruction}}
工作目录：{{.Workdir}}
验证输出或审查反馈：
{{.Feedback}}
输入路径：
{{range .Inputs}}- {{.}}
{{end}}参考图片：
{{range .Images}}- {{.}}
{{end}}
