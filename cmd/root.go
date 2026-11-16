package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var Version = "2.0.0-dev"

type rootOptions struct {
	home string
}

var rootOpts rootOptions

func defaultHome() string {
	if v := os.Getenv("XCLI_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".xcli"
	}
	return filepath.Join(home, ".xcli")
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "xcli",
		Short:         "用真实的 Codex / Cursor / Antigravity CLI 完成任务，并自动编排多个 CLI",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&rootOpts.home, "home", defaultHome(), "xcli 数据目录")
	return root
}

func Execute() error {
	return newRootCommand().Execute()
}
