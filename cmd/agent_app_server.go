package cmd

import (
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/nativeadapter"
	"github.com/spf13/cobra"
)

func appServerCommand(root, path *string) *cobra.Command {
	group := &cobra.Command{Use: "app-server", Short: "Codex stdio adapter controls; real turns remain gated"}
	group.AddCommand(&cobra.Command{Use: "readiness", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, e := agentpolicy.Load(*path)
		if e != nil {
			return e
		}
		return encodeJSON(c.OutOrStdout(), map[string]any{"state": agentpolicy.IsolatedRuntimeRequired, "engineering_state": agentpolicy.IsolatedRuntimeRequired, "user_confirmation_state": agentpolicy.UserConfirmationRequired, "enabled": false, "feature_flag": cfg.CodexAppServerEnabled, "call_authorized": false, "model_requests": 0, "blockers": []string{"authenticated_process_and_tool_credentials_not_separated", "whole_process_offline_boundary_has_no_approved_model_transport", "real_builtin_tool_behavior_not_tested"}})
	}}, &cobra.Command{Use: "disable", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		r, e := agentstore.Prepare(*root)
		if e != nil {
			return e
		}
		return agentpolicy.DisableAppServer(r)
	}})
	var binary string
	var outer bool
	probe := &cobra.Command{Use: "probe", Short: "Explicit no-turn probe using only disposable fake credentials", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		st, e := agentstore.Open(*root)
		if e != nil {
			return e
		}
		defer st.Close()
		r, e := nativeadapter.ProbeAppServer(c.Context(), binary, outer)
		// Return the scoped receipt even when initialization failed.
		if writeErr := encodeJSON(c.OutOrStdout(), r); writeErr != nil {
			return writeErr
		}
		return e
	}}
	probe.Flags().StringVar(&binary, "binary", "", "owner-selected absolute official Codex executable")
	probe.Flags().BoolVar(&outer, "outer-isolation", false, "wrap the entire credential-free process in offline OS isolation")
	group.AddCommand(probe)
	return group
}
