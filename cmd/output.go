package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/engine"
	"github.com/spf13/cobra"
)

type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string { return e.Message }

type outputOptions struct{ asJSON, events, detach bool }

func (o *outputOptions) add(c *cobra.Command, detach bool) {
	c.Flags().BoolVar(&o.asJSON, "json", false, "输出最终 Result JSON")
	c.Flags().BoolVar(&o.events, "events", false, "输出事件 JSONL")
	if detach {
		c.Flags().BoolVar(&o.detach, "detach", false, "后台运行并返回任务 ID")
	}
	c.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return o.invalid(c, fmt.Errorf("请检查命令参数并运行 xcli %s --help：%w", c.Name(), err))
	})
}

func (o outputOptions) sink(c *cobra.Command) func(api.Event) {
	return func(ev api.Event) {
		if o.events {
			_ = json.NewEncoder(c.OutOrStdout()).Encode(ev)
			return
		}
		if o.asJSON {
			return
		}
		var data map[string]any
		_ = json.Unmarshal(ev.Data, &data)
		text, _ := data["text"].(string)
		if tool, ok := data["tool"].(string); ok {
			text = "正在调用 " + tool
		}
		if ev.Type == "step.started" {
			text = "开始执行"
		}
		if ev.Type == "step.waiting" {
			text, _ = data["reason"].(string)
		}
		if ev.Type == "agent.fallback" {
			text = fmt.Sprintf("切换到 %v：%v", data["to"], data["reason"])
		}
		if text != "" {
			if ev.Step != "" {
				fmt.Fprintf(c.ErrOrStderr(), "[步骤 %s · %s] %s\n", ev.Step, ev.Agent, text)
			} else {
				fmt.Fprintln(c.ErrOrStderr(), text)
			}
		}
	}
}

func (o outputOptions) finish(c *cobra.Command, r api.Result) error {
	if !o.events {
		if o.asJSON {
			if err := json.NewEncoder(c.OutOrStdout()).Encode(r); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(c.OutOrStdout(), "状态：%s\n%s\n", r.Status, r.Summary)
			for _, a := range r.Artifacts {
				fmt.Fprintln(c.OutOrStdout(), "产物："+a.Path)
			}
			if r.Changes != nil && r.Changes.DiffFile != "" {
				fmt.Fprintln(c.OutOrStdout(), "改动："+r.Changes.DiffFile)
			}
			if r.TaskID != "" {
				fmt.Fprintf(c.OutOrStdout(), "任务 ID：%s\n查看详情：xcli result %s\n", r.TaskID, r.TaskID)
			}
		}
	}
	code := 0
	switch r.Status {
	case api.StatusInvalid:
		code = 2
	case api.StatusNeedsAttention:
		code = 3
	case api.StatusCancelled:
		code = 130
	case api.StatusFailed:
		code = 1
	}
	if code != 0 {
		return &ExitError{Code: code}
	}
	return nil
}

func (o outputOptions) invalid(c *cobra.Command, err error) error {
	r := engine.InvalidResult(err)
	if o.events {
		ev, _ := api.NewEvent("", 1, "task.finished", "", "", r)
		_ = json.NewEncoder(c.OutOrStdout()).Encode(ev)
	}
	return o.finish(c, r)
}
