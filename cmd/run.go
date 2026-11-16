package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/engine"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/spf13/cobra"
)

func taskCommands() []*cobra.Command {
	commands := []*cobra.Command{}
	for _, name := range []string{"do", "plan", "ask", "code", "review", "image"} {
		var f requestFlags
		c := &cobra.Command{Use: name + " <要求…>", Short: map[string]string{"do": "自动编排并执行需求", "plan": "只生成计划", "ask": "问答、分析与结构化输出", "code": "编码并验证、审查、修复", "review": "只读代码审查", "image": "生成或修改图片"}[name], RunE: func(c *cobra.Command, args []string) error {
			capability := api.Capability(name)
			if name == "do" || name == "plan" {
				capability = api.CapAuto
			}
			req, err := f.read(capability, strings.Join(args, " "))
			if err != nil {
				return f.invalid(c, err)
			}
			if name == "plan" {
				req.PlanOnly = true
			}
			return runRequest(c, req, f.outputOptions)
		}}
		f.add(c)
		commands = append(commands, c)
	}
	var requestFile string
	var output outputOptions
	submit := &cobra.Command{Use: "submit --request <file|->", Short: "从 JSON 读取完整请求", RunE: func(c *cobra.Command, args []string) error {
		if len(args) > 0 || requestFile == "" {
			return output.invalid(c, fmt.Errorf("请用 --request <file|-> 提供 JSON 请求"))
		}
		var input io.Reader = c.InOrStdin()
		if requestFile != "-" {
			f, err := os.Open(requestFile)
			if err != nil {
				return output.invalid(c, err)
			}
			defer f.Close()
			input = f
		}
		var req api.Request
		if err := decodeJSON(input, &req); err != nil {
			return output.invalid(c, err)
		}
		return runRequest(c, req, output)
	}}
	submit.Flags().StringVar(&requestFile, "request", "", "JSON 请求文件，- 表示 stdin")
	output.add(submit, true)
	return append(commands, submit)
}

func runRequest(c *cobra.Command, req api.Request, o outputOptions) error {
	if o.asJSON && o.events {
		return o.invalid(c, fmt.Errorf("请在 --json 和 --events 中选择一种输出模式"))
	}
	e, err := engine.New(rootOpts.home)
	if err != nil {
		return o.invalid(c, err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return o.invalid(c, err)
	}
	req, err = e.Normalize(req, cwd)
	if err != nil {
		return o.invalid(c, err)
	}
	t, err := e.Create(req)
	if err != nil {
		return o.invalid(c, err)
	}
	if o.detach {
		if err := e.Detach(t); err != nil {
			if r := t.Snapshot().Result; r != nil {
				if o.events {
					events, _ := store.ReadEvents(t.Dir(), 0)
					for _, ev := range events {
						o.sink(c)(ev)
					}
				}
				return o.finish(c, *r)
			}
			return err
		}
		return json.NewEncoder(c.OutOrStdout()).Encode(map[string]string{"task_id": t.ID()})
	}
	ctx, stop := taskContext(c.Context())
	defer stop()
	return o.finish(c, e.Run(ctx, t, o.sink(c)))
}
