package cmd

import (
	"fmt"
	"net"

	"github.com/Garden12138/xcli/internal/modelgateway"
	"github.com/Garden12138/xcli/internal/modelstore"
	"github.com/spf13/cobra"
)

func (a *app) newModelCommand() *cobra.Command {
	group := &cobra.Command{Use: "model", Short: "Serve model APIs independently of native Agent tasks"}
	serve := &cobra.Command{Use: "serve", Short: "Serve local model APIs in the foreground", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, _, err := modelstore.Load(a.configPath)
		if err != nil {
			return err
		}
		registry, err := cfg.Registry()
		if err != nil {
			return err
		}
		s, err := modelgateway.New(modelgateway.Options{Registry: registry})
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
	group.AddCommand(serve, list)
	return group
}
