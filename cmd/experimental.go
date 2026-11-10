package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// freezeGateway preserves old configuration, state and cleanup commands while
// keeping non-core services/probes/grants off by default. Wrap the handlers so
// a descendant PreRun hook cannot accidentally bypass the product boundary.
func (a *app) freezeGateway(group *cobra.Command) {
	group.Hidden = true
	group.Short = "Frozen experiment: " + group.Short
	safe := map[string]bool{
		"list": true, "doctor": true, "status": true, "stop": true, "logs": true,
		"preflight": true, "client list": true, "client revoke": true,
		"codex-limited readiness": true, "codex-limited disable": true,
		"app-server readiness": true, "app-server disable": true,
	}
	var walk func(*cobra.Command, string)
	walk = func(c *cobra.Command, path string) {
		if c.RunE != nil && !safe[path] {
			run := c.RunE
			c.RunE = func(cmd *cobra.Command, args []string) error {
				if !a.experimentalGateways {
					return fmt.Errorf("experimental_gateway_disabled: %s is frozen in R4; use task/run/use/resume for native CLI work (historical experiments require --experimental-gateways)", group.Name())
				}
				return run(cmd, args)
			}
		}
		for _, child := range c.Commands() {
			next := child.Name()
			if path != "" {
				next = path + " " + next
			}
			walk(child, next)
		}
	}
	walk(group, "")
}
