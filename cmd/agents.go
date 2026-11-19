package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/health"
	"github.com/Garden12138/xcli/internal/proc"
	"github.com/spf13/cobra"
)

type agentRow struct {
	Name           string         `json:"name"`
	Installed      bool           `json:"installed"`
	Path           string         `json:"path"`
	Version        string         `json:"version"`
	Auth           string         `json:"auth"`
	Available      bool           `json:"available"`
	CooldownUntil  time.Time      `json:"cooldown_until,omitempty"`
	CooldownReason string         `json:"cooldown_reason,omitempty"`
	Strengths      map[string]int `json:"strengths"`
}

func newAgentsCommand() *cobra.Command {
	var asJSON, reset bool
	command := &cobra.Command{Use: "agents", Short: "查看 CLI 安装、登录和冷却状态", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(rootOpts.home)
		if err != nil {
			return err
		}
		cache := health.Open(rootOpts.home)
		if reset {
			if err := cache.Reset(); err != nil {
				return err
			}
		}
		pathList, proxy := proc.SearchEnvironment(cfg.Path, filepath.Join(rootOpts.home, "cache", "env.json"), cfg.Defaults.InheritShellProxy)
		rows := []agentRow{}
		for _, name := range config.AgentOrder {
			d, err := driver.New(name, cfg.Agents[name])
			if err != nil {
				return err
			}
			exe, _ := proc.LookPath(d.Command(), pathList)
			e := cache.Probe(cmd.Context(), d, exe, proc.CLIEnv(pathList, cfg.Agents[name].Env, proxy), 10*time.Minute)
			h := e.Health
			rows = append(rows, agentRow{Name: name, Installed: h.Installed, Path: h.Path, Version: h.Version, Auth: h.Auth,
				Available:     cfg.Enabled(name) && h.Installed && h.Auth != "missing" && cache.Available(name, time.Now()),
				CooldownUntil: e.CooldownUntil, CooldownReason: e.CooldownReason, Strengths: cfg.Agents[name].Strengths})
		}
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
		}
		out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(out, "NAME\tINSTALLED\tVERSION\tAUTH\tSTATUS\tCODE\tREVIEW\tASK\tIMAGE")
		for _, row := range rows {
			status := "可用"
			if !cfg.Enabled(row.Name) {
				status = "已禁用"
			} else if !row.Installed {
				status = "请安装 " + cfg.Agents[row.Name].Command
			} else if row.CooldownUntil.After(time.Now()) {
				status = "冷却中（" + row.CooldownReason + "）至 " + row.CooldownUntil.Local().Format("15:04")
			} else if row.Auth == "missing" {
				status = "请在终端登录 CLI"
			}
			fmt.Fprintf(out, "%s\t%t\t%s\t%s\t%s\t%d\t%d\t%d\t%d\n", row.Name, row.Installed, row.Version, row.Auth, status,
				row.Strengths["code"], row.Strengths["review"], row.Strengths["ask"], row.Strengths["image"])
		}
		return out.Flush()
	}}
	command.Flags().BoolVar(&asJSON, "json", false, "输出 JSON")
	command.Flags().BoolVar(&reset, "reset", false, "清除 CLI 冷却")
	return command
}
