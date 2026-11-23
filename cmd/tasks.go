package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/engine"
	"github.com/Garden12138/xcli/internal/store"
	"github.com/spf13/cobra"
)

func lifecycleCommands() []*cobra.Command {
	commands := []*cobra.Command{}
	for _, name := range []string{"status", "wait", "logs", "result", "cancel", "apply"} {
		var asJSON, follow, remove bool
		var after int
		var timeout time.Duration
		c := &cobra.Command{Use: name + " <id>", Short: map[string]string{"status": "查看任务状态", "wait": "等待任务结束并输出 Result", "logs": "读取任务事件 JSONL", "result": "输出 Result JSON", "cancel": "取消任务", "apply": "应用隔离工作区的代码改动"}[name], Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			e, err := engine.New(rootOpts.home)
			if err != nil {
				return err
			}
			id := args[0]
			switch name {
			case "status":
				r, err := e.Status(id)
				if err != nil {
					return err
				}
				if asJSON {
					return json.NewEncoder(c.OutOrStdout()).Encode(r)
				}
				fmt.Fprintf(c.OutOrStdout(), "任务 ID：%s\n状态：%s\n需求：%s\n", id, r.Status, r.Request.Goal)
				if r.Result != nil {
					fmt.Fprintln(c.OutOrStdout(), r.Result.Summary)
				}
				return nil
			case "wait":
				ctx, stop := taskContext(c.Context())
				defer stop()
				if timeout < 0 {
					return fmt.Errorf("请将 --timeout 设为正时长")
				}
				if timeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, timeout)
					defer cancel()
				}
				r, err := e.Wait(ctx, id)
				if err != nil {
					return fmt.Errorf("请运行 xcli status %s 查看任务进度：%w", id, err)
				}
				return (outputOptions{asJSON: true}).finish(c, r)
			case "logs":
				if after < 0 {
					return fmt.Errorf("请将 --after 设为 0 或正整数")
				}
				write := func(ev api.Event) error { return json.NewEncoder(c.OutOrStdout()).Encode(ev) }
				if follow {
					ctx, stop := taskContext(c.Context())
					defer stop()
					return e.Follow(ctx, id, after, write)
				}
				t, err := e.Store.Load(id)
				if err != nil {
					return err
				}
				events, err := store.ReadEvents(t.Dir(), after)
				if err != nil {
					return err
				}
				for _, ev := range events {
					if err := write(ev); err != nil {
						return err
					}
				}
				return nil
			case "result":
				r, err := e.Status(id)
				if err != nil {
					return err
				}
				if r.Result == nil {
					return fmt.Errorf("任务正在执行，请运行 xcli wait %s 等待结果", id)
				}
				return json.NewEncoder(c.OutOrStdout()).Encode(r.Result)
			case "cancel":
				if err := e.Cancel(id); err != nil {
					return err
				}
				fmt.Fprintln(c.OutOrStdout(), "已发送取消信号："+id)
				return nil
			case "apply":
				if err := e.Apply(id, remove); err != nil {
					return err
				}
				fmt.Fprintln(c.OutOrStdout(), "已应用改动："+id)
				return nil
			}
			return nil
		}}
		switch name {
		case "status":
			c.Flags().BoolVar(&asJSON, "json", false, "输出任务记录 JSON")
		case "wait":
			c.Flags().DurationVar(&timeout, "timeout", 0, "等待时长，默认一直等待")
		case "logs":
			c.Flags().BoolVarP(&follow, "follow", "f", false, "跟随事件直到任务结束")
			c.Flags().IntVar(&after, "after", 0, "只输出 seq 大于该值的事件")
		case "apply":
			c.Flags().BoolVar(&remove, "remove-worktree", false, "应用后移除 worktree")
		}
		commands = append(commands, c)
	}
	var limit int
	var listJSON bool
	list := &cobra.Command{Use: "list", Short: "列出最近的任务", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		if limit < 1 {
			return fmt.Errorf("请将 --limit 设为正整数")
		}
		e, err := engine.New(rootOpts.home)
		if err != nil {
			return err
		}
		rows, err := e.Store.List(limit)
		if err != nil {
			return err
		}
		for i, r := range rows {
			rows[i], err = e.Status(r.ID)
			if err != nil {
				return err
			}
		}
		if listJSON {
			return json.NewEncoder(c.OutOrStdout()).Encode(rows)
		}
		w := tabwriter.NewWriter(c.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\t状态\t能力\t需求\t创建时间")
		for _, r := range rows {
			goal := []rune(strings.ReplaceAll(r.Request.Goal, "\n", " "))
			if len(goal) > 40 {
				goal = append(goal[:40], '…')
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Status, r.Request.Capability, string(goal), r.CreatedAt.Local().Format("2006-01-02 15:04:05"))
		}
		return w.Flush()
	}}
	list.Flags().IntVar(&limit, "limit", 20, "最多显示多少任务")
	list.Flags().BoolVar(&listJSON, "json", false, "输出任务记录 JSON")
	var resumeOutput outputOptions
	var resumeApp string
	resume := &cobra.Command{Use: "resume <id> <追加要求…>", Short: "续接已结束任务的 CLI、会话与工作目录", RunE: func(c *cobra.Command, args []string) error {
		if len(args) < 2 {
			return resumeOutput.invalid(c, fmt.Errorf("请提供任务 ID 和追加要求"))
		}
		e, err := engine.New(rootOpts.home)
		if err != nil {
			return resumeOutput.invalid(c, err)
		}
		req, err := e.Resume(args[0], strings.Join(args[1:], " "))
		if err != nil {
			return resumeOutput.invalid(c, err)
		}
		if err := applyApp(&req, resumeApp, c.Flags().Changed("app"), ""); err != nil {
			return resumeOutput.invalid(c, err)
		}
		return runRequest(c, req, resumeOutput)
	}}
	resume.Flags().StringVar(&resumeApp, "app", "", "覆盖父任务的调用方应用名称")
	resumeOutput.add(resume, true)
	var older string
	gc := &cobra.Command{Use: "gc", Short: "清理已结束的旧任务和 worktree", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		d, err := retention(older)
		if err != nil || d <= 0 {
			return fmt.Errorf("请将 --older-than 设为正时长，例如 7d")
		}
		e, err := engine.New(rootOpts.home)
		if err != nil {
			return err
		}
		n, err := e.GC(d)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.OutOrStdout(), "已清理 %d 个任务\n", n)
		return nil
	}}
	gc.Flags().StringVar(&older, "older-than", "7d", "保留时长，例如 7d 或 24h")
	return append(commands, list, resume, gc)
}

func retention(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(s, "d"), 64)
		if err != nil {
			return 0, err
		}
		return time.ParseDuration(strconv.FormatFloat(n*24, 'f', -1, 64) + "h")
	}
	return time.ParseDuration(s)
}
