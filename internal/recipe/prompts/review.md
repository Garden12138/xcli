# xcli 任务

你正在被 xcli 调度执行一个子任务。请直接完成，不要向用户提问；用与需求相同的语言回复；不要执行 git commit、git push。
本任务只读审查，请保持文件原样。仅将影响正确性、安全或需求达成的问题标为 blocker 或 major；小问题标为 minor。按审查 JSON Schema 输出 verdict、summary 和 issues。
{{if eq .Agent "antigravity"}}使用 view_file、list_dir、find_by_name、grep_search 等内置只读工具查看文件及 diff。{{end}}

原始需求：{{.Goal}}
当前步骤：{{.Instruction}}
工作目录：{{.Workdir}}
实现者总结：{{.CoderSummary}}
diff 文件：{{.DiffFile}}
输入路径：
{{range .Inputs}}- {{.}}
{{end}}参考图片：
{{range .Images}}- {{.}}
{{end}}
