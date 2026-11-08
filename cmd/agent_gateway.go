package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentgateway"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentservice"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/modelstore"
	"github.com/Garden12138/xcli/internal/nativeadapter"
	"github.com/spf13/cobra"
)

func (a *app) newAgentGatewayCommand() *cobra.Command {
	var root, path string
	group := &cobra.Command{Use: "agent-gateway", Short: "Independent local native Agent task gateway (native profiles gated)"}
	group.PersistentFlags().StringVar(&root, "state-dir", "", "independent private Agent Gateway data root")
	group.PersistentFlags().StringVar(&path, "gateway-config", "", "independent agent-gateway.yaml (never uses --config)")
	group.AddCommand(&cobra.Command{Use: "serve", Short: "Serve in foreground; default dispatch budget is zero", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, e := agentpolicy.Load(path)
		if e != nil {
			return e
		}
		st, e := agentstore.Open(root)
		if e != nil {
			return e
		}
		defer st.Close()
		clients := &agentstore.Clients{Root: st.Root}
		profiles, e := agentpolicy.CachedProfiles(st.Root)
		if e != nil {
			return e
		}
		profiles, e = agentpolicy.BindLimited(st.Root, cfg, profiles)
		if e != nil {
			return e
		}
		svc, e := agentservice.New(st, clients, cfg, profiles)
		if e != nil {
			return e
		}
		listener, e := net.Listen("tcp", cfg.Address())
		if e != nil {
			return e
		}
		defer listener.Close()
		ctx, cancel := signalContext(c.Context())
		defer cancel()
		go func() { <-ctx.Done(); svc.StopAccepting() }()
		state, e := st.Publish(listener.Addr().String(), api.Digest(cfg))
		if e != nil {
			return e
		}
		defer st.Unpublish(state)
		watch := make(chan error, 1)
		go func() { watch <- agentstore.WatchStop(ctx, st.Root, state, cancel) }()
		worker := make(chan error, 1)
		go func() {
			err := svc.Run(ctx, nativeadapter.Exec{})
			worker <- err
			if err != nil {
				cancel()
			}
		}()
		fmt.Fprintf(c.OutOrStdout(), "Native Agent Gateway listening on %s; native profiles remain gated\n", state.Address)
		server := &agentgateway.Server{Service: svc}
		serveErr := server.Serve(ctx, listener)
		cancel()
		workerErr := <-worker
		watchErr := <-watch
		return errors.Join(serveErr, workerErr, watchErr, st.Checkpoint())
	}}, &cobra.Command{Use: "status", Short: "Read service generation and lock without native probes", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		state, e := agentstore.Status(root)
		if e != nil {
			return e
		}
		return encodeJSON(c.OutOrStdout(), state)
	}}, &cobra.Command{Use: "stop", Short: "Stop this service generation without signalling stored PIDs", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
		defer cancel()
		if e := agentstore.Stop(ctx, root); e != nil {
			return e
		}
		fmt.Fprintln(c.OutOrStdout(), "Native Agent Gateway stopped")
		return nil
	}})
	var refreshAgent, refreshBinary string
	refresh := &cobra.Command{Use: "refresh", Short: "Explicit non-model version probe; service must be stopped; never enables profiles", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		st, e := agentstore.Open(root)
		if e != nil {
			return e
		}
		defer st.Close()
		r, e := agentpolicy.Refresh(c.Context(), st.Root, refreshAgent, refreshBinary)
		if e != nil {
			return e
		}
		return encodeJSON(c.OutOrStdout(), r)
	}}
	refresh.Flags().StringVar(&refreshAgent, "agent", "", "codex, cursor or antigravity")
	refresh.Flags().StringVar(&refreshBinary, "binary", "", "owner-selected absolute official executable path")
	var preflightAgent string
	preflight := &cobra.Command{Use: "preflight", Short: "Inspect native text blockers without spawning a CLI; service must be stopped", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, e := agentpolicy.Load(path)
		if e != nil {
			return e
		}
		st, e := agentstore.Open(root)
		if e != nil {
			return e
		}
		defer st.Close()
		profiles, e := agentpolicy.CachedProfiles(st.Root)
		if e != nil {
			return e
		}
		for _, p := range profiles {
			if p.AgentID == preflightAgent {
				r, err := nativeadapter.InspectNativeText(p, cfg)
				if err != nil {
					return err
				}
				return encodeJSON(c.OutOrStdout(), r)
			}
		}
		return errors.New("registered --agent required")
	}}
	preflight.Flags().StringVar(&preflightAgent, "agent", "", "codex, cursor or antigravity")
	group.AddCommand(refresh, preflight, agentClientCommand(&root), a.limitedCodexCommand(&root, &path))
	return group
}
func agentClientCommand(root *string) *cobra.Command {
	group := &cobra.Command{Use: "client", Short: "Manage independent application tokens; secrets go only to new private files"}
	for _, rotate := range []bool{false, true} {
		name := "issue"
		if rotate {
			name = "rotate"
		}
		var file string
		var reauthorize bool
		grant := agentstore.Client{}
		command := &cobra.Command{Use: name + " <app-id>", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
			if file == "" {
				return errors.New("--token-file is required")
			}
			stateRoot, e := agentstore.Prepare(*root)
			if e != nil {
				return e
			}
			clients := &agentstore.Clients{Root: stateRoot}
			grant.ID = args[0]
			if rotate {
				list, err := clients.List()
				if err != nil {
					return err
				}
				found := false
				for _, old := range list {
					if old.ID == args[0] {
						grant = old
						found = true
						break
					}
				}
				if !found {
					return errors.New("rotation target is missing")
				}
				if grant.Revoked && !reauthorize {
					return errors.New("revoked grant requires explicit --reauthorize-revoked")
				}
			}
			if e = modelstore.PrivateDirectory(filepath.Dir(file)); e != nil {
				return e
			}
			f, e := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if e != nil {
				return errors.New("token delivery file must be new")
			}
			token, e := clients.Issue(grant, rotate)
			if e != nil {
				f.Close()
				os.Remove(file)
				return e
			}
			_, we := fmt.Fprintln(f, token)
			se := f.Sync()
			ce := f.Close()
			if e = errors.Join(we, se, ce); e != nil {
				_ = clients.Revoke(grant.ID)
				_ = os.Remove(file)
				return errors.New("token delivery failed; grant revoked")
			}
			fmt.Fprintf(c.OutOrStdout(), "Client %s token delivered to private file\n", grant.ID)
			return nil
		}}
		command.Flags().StringVar(&file, "token-file", "", "new private token delivery file")
		if rotate {
			command.Flags().BoolVar(&reauthorize, "reauthorize-revoked", false, "explicitly restore a revoked grant; old queued work remains blocked")
		}
		if !rotate {
			command.Flags().StringSliceVar(&grant.Scopes, "scope", []string{"session:create", "session:read", "session:close", "input:write", "task:create", "task:read", "task:cancel", "task:review", "artifact:read"}, "explicit HTTP scopes (no origin attestation by default)")
			command.Flags().StringSliceVar(&grant.Agents, "agent", nil, "allowed registered Agent IDs")
			command.Flags().StringSliceVar(&grant.Policies, "policy", nil, "allowed registered policy IDs")
			command.Flags().StringSliceVar(&grant.Operations, "operation", []string{"text_task"}, "allowed operations")
			command.Flags().BoolVar(&grant.AllowRemote, "allow-remote", false, "allow remote/unknown inference with per-task consent")
			command.Flags().IntVar(&grant.DailyDispatches, "daily-dispatches", 0, "daily local dispatch ceiling")
			command.Flags().IntVar(&grant.DailyProcessStarts, "daily-process-starts", 0, "daily process start ceiling")
			command.Flags().IntVar(&grant.DailyExecutionSeconds, "daily-execution-seconds", 0, "daily reserved execution ceiling")
		}
		group.AddCommand(command)
	}
	group.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		r, e := agentstore.Prepare(*root)
		if e != nil {
			return e
		}
		cs, e := (&agentstore.Clients{Root: r}).List()
		if e != nil {
			return e
		}
		return encodeJSON(c.OutOrStdout(), cs)
	}}, &cobra.Command{Use: "revoke <app-id>", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		r, e := agentstore.Prepare(*root)
		if e != nil {
			return e
		}
		if e = (&agentstore.Clients{Root: r}).Revoke(args[0]); e != nil {
			return e
		}
		fmt.Fprintln(c.OutOrStdout(), "Client revoked; active turns will be stopped by their controller")
		return nil
	}})
	return group
}
