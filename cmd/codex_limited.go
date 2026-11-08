package cmd

import (
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/modelstore"
	"github.com/spf13/cobra"
	"slices"
)

func (a *app) limitedCodexCommand(root, path *string) *cobra.Command {
	group := &cobra.Command{Use: "codex-limited", Short: "Owner-only synthetic Codex controls; enabling never authorizes inference"}
	var binary, authHome, ref, confirmationFile string
	prepare := &cobra.Command{Use: "prepare", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		st, e := agentstore.Open(*root)
		if e != nil {
			return e
		}
		defer st.Close()
		r, e := agentpolicy.PrepareLimited(c.Context(), st.Root, binary, authHome)
		if e != nil {
			return e
		}
		return encodeJSON(c.OutOrStdout(), map[string]any{"workspace": r.Workspace, "state": "DISABLED", "model_requests": 0})
	}}
	prepare.Flags().StringVar(&binary, "binary", "", "absolute selected official Codex executable")
	prepare.Flags().StringVar(&authHome, "auth-home", "", "canonical existing CODEX_HOME; only official CLI reads authentication")
	group.AddCommand(prepare)
	for _, name := range []string{"readiness", "probe", "boundary-probe", "confirmation-template", "confirm", "enable", "authorize"} {
		command := &cobra.Command{Use: name, Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
			cfg, e := agentpolicy.Load(*path)
			if e != nil {
				return e
			}
			st, e := agentstore.Open(*root)
			if e != nil {
				return e
			}
			defer st.Close()
			switch c.Name() {
			case "confirmation-template":
				r, e := agentpolicy.LimitedConfirmationTemplate(st.Root, cfg.CodexLimited)
				if e != nil {
					return e
				}
				return encodeJSON(c.OutOrStdout(), r)
			case "confirm":
				b, e := modelstore.ReadPrivate(confirmationFile, 16<<10)
				if e != nil {
					return e
				}
				return agentpolicy.ConfirmLimitedJSON(st.Root, cfg.CodexLimited, b)
			case "readiness":
				r := agentpolicy.LimitedStatus(st.Root, cfg.CodexLimited)
				client, err := (&agentstore.Clients{Root: st.Root}).Get(cfg.CodexLimited.ClientID)
				r.ClientReady = err == nil && client.AllowRemote && len(client.Agents) == 1 && client.Agents[0] == "codex" && len(client.Policies) == 1 && client.Policies[0] == agentpolicy.LimitedPolicy && len(client.Operations) == 1 && client.Operations[0] == "text_task" && client.DailyDispatches == 1 && client.DailyProcessStarts == 1 && client.DailyExecutionSeconds == 120 && len(client.Scopes) == 6
				for _, scope := range []string{"session:create", "session:read", "session:close", "task:create", "task:read", "task:cancel"} {
					r.ClientReady = r.ClientReady && slices.Contains(client.Scopes, scope)
				}
				if !r.ClientReady {
					r.State = agentpolicy.EngineeringBlocked
					r.EngineeringState = agentpolicy.EngineeringBlocked
					r.Blockers = append(r.Blockers, "limited_client_grant_not_ready")
				}
				return encodeJSON(c.OutOrStdout(), r)
			case "probe":
				r, e := agentpolicy.ProbeLimited(c.Context(), st.Root)
				if e != nil {
					return e
				}
				return encodeJSON(c.OutOrStdout(), r)
			case "boundary-probe":
				r, e := agentpolicy.ProbeLimitedBoundary(c.Context(), st.Root)
				if e != nil {
					return e
				}
				return encodeJSON(c.OutOrStdout(), r)
			case "enable":
				return agentpolicy.EnableLimited(st.Root, cfg.CodexLimited)
			case "authorize":
				return agentpolicy.AuthorizeLimited(st.Root, ref, cfg.CodexLimited)
			}
			return nil
		}}
		if name == "authorize" {
			command.Flags().StringVar(&ref, "authorization-ref", "", "new explicit owner inference authorization; never inherited")
		}
		if name == "confirm" {
			command.Flags().StringVar(&confirmationFile, "file", "", "private owner-authored JSON; never inferred by xcli")
		}
		group.AddCommand(command)
	}
	group.AddCommand(&cobra.Command{Use: "disable", Short: "Immediately revoke the profile and its pending call authorization", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		// Private atomic control file; unlike prepare/probe this works while serving.
		root, e := agentstore.Prepare(*root)
		if e != nil {
			return e
		}
		return agentpolicy.DisableLimited(root)
	}})
	return group
}
