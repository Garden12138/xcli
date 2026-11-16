//go:build unix

package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Garden12138/xcli/internal/engine"
	"github.com/spf13/cobra"
)

func taskContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
}

func newWorkerCommand() *cobra.Command {
	return &cobra.Command{Use: "__worker <id>", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		e, err := engine.New(rootOpts.home)
		if err != nil {
			return err
		}
		t, err := e.Store.Load(args[0])
		if err != nil {
			return err
		}
		ctx, stop := taskContext(c.Context())
		defer stop()
		e.Run(ctx, t, nil)
		return nil
	}}
}
