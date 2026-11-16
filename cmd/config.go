package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCommand() *cobra.Command {
	parent := &cobra.Command{Use: "config", Short: "查看或创建可选配置"}
	for _, operation := range []string{"show", "init", "path"} {
		command := &cobra.Command{Use: operation, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			path, err := filepath.Abs(filepath.Join(rootOpts.home, "config.yaml"))
			if err != nil {
				return err
			}
			if operation == "path" {
				fmt.Fprintln(cmd.OutOrStdout(), path)
				return nil
			}
			cfg := config.Default()
			if operation == "show" {
				cfg, err = config.Load(rootOpts.home)
				if err != nil {
					return err
				}
			}
			raw, err := config.Marshal(cfg)
			if err != nil {
				return err
			}
			if operation == "show" {
				_, err = cmd.OutOrStdout().Write(raw)
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if errors.Is(err, os.ErrExist) {
				return fmt.Errorf("配置已存在于 %s，请运行 xcli config show 查看或直接编辑该文件", path)
			}
			if err != nil {
				return err
			}
			_, err = file.Write(raw)
			if err = errors.Join(err, file.Close()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		}}
		parent.AddCommand(command)
	}
	return parent
}
