package cmd

import (
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/Garden12138/xcli/internal/model"
	"github.com/Garden12138/xcli/internal/modelbackend"
	"github.com/Garden12138/xcli/internal/modelgateway"
	"github.com/Garden12138/xcli/internal/modelstore"
	"github.com/spf13/cobra"
)

func (a *app) newModelCommand() *cobra.Command {
	group := &cobra.Command{Use: "model", Short: "Serve model APIs independently of native Agent tasks"}
	var stateDir string
	group.PersistentFlags().StringVar(&stateDir, "state-dir", "", "private gateway state directory (defaults to XDG data directory)")
	serve := &cobra.Command{Use: "serve", Short: "Serve local model APIs in the foreground", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, configPath, err := modelstore.Load(a.configPath)
		if err != nil {
			return err
		}
		registry, err := cfg.Registry()
		if err != nil {
			return err
		}
		store, err := modelstore.New(stateDir)
		if err != nil {
			return err
		}
		chats := map[string]model.ChatBackend{}
		for id, b := range cfg.Backends {
			key, err := modelstore.ResolveSecret(b.APIKeyRef, filepath.Dir(configPath))
			if err != nil {
				return fmt.Errorf("backend %s: %w", id, err)
			}
			backend, err := modelbackend.NewOpenAI(b.Endpoint, key)
			if err != nil {
				return err
			}
			defer backend.Close()
			if b.Type == "openai-compatible" {
				chats[id] = backend
			}
		}
		s, err := modelgateway.New(modelgateway.Options{Registry: registry, Authenticate: store.Authenticate, Chats: chats, MaxConcurrent: cfg.Server.MaxConcurrent, Timeout: time.Duration(cfg.Server.TimeoutSeconds) * time.Second})
		if err != nil {
			return err
		}
		listener, err := net.Listen("tcp", cfg.Address())
		if err != nil {
			return fmt.Errorf("listen on configured gateway address (port may be occupied): %w", err)
		}
		defer listener.Close()
		ctx, cancel := signalContext(c.Context())
		defer cancel()
		fmt.Fprintf(c.ErrOrStderr(), "Model Gateway listening on %s\n", listener.Addr())
		return s.Serve(ctx, listener)
	}}
	list := &cobra.Command{Use: "list", Short: "List configured models without contacting providers", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, _, err := modelstore.Load(a.configPath)
		if err != nil {
			return err
		}
		r, err := cfg.Registry()
		if err != nil {
			return err
		}
		return encodeJSON(c.OutOrStdout(), r.List())
	}}
	doctor := &cobra.Command{Use: "doctor", Short: "Validate configuration and credential references without model calls", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, path, err := modelstore.Load(a.configPath)
		if err != nil {
			return err
		}
		for _, b := range cfg.Backends {
			if _, err = modelstore.ResolveSecret(b.APIKeyRef, filepath.Dir(path)); err != nil {
				return err
			}
		}
		return encodeJSON(c.OutOrStdout(), map[string]any{"configuration": "valid", "models": len(cfg.Models), "upstream": "unverified", "billable_requests": 0})
	}}
	doctor.Flags().Bool("no-billable-requests", true, "configuration-only diagnostics; never sends model requests")
	group.AddCommand(serve, list, doctor, a.newModelClientCommand(&stateDir))
	return group
}
