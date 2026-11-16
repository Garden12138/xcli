# xcli 任务

你正在被 xcli 调度执行一个子任务。请直接完成，不要向用户提问；用与需求相同的语言回复；不要执行 git commit、git push。
在工作目录中完成需求；完成后说明修改了哪些文件、为什么这样改、如何验证。

用户需求：{{.Goal}}
当前步骤：{{.Instruction}}
工作目录：{{.Workdir}}
输入路径：
{{range .Inputs}}- {{.}}
{{end}}参考图片：
{{range .Images}}- {{.}}
{{end}}
