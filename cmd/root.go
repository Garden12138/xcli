package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/runstore"
	"github.com/spf13/cobra"
)

var Version = "0.2.0-dev"

type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("command exited with code %d", e.Code)
}

type app struct {
	configPath string
}

func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	a := &app{}
	root := &cobra.Command{
		Use:           "xcli",
		Short:         "Run and orchestrate AI coding agent CLIs",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&a.configPath, "config", "", "explicit configuration file")
	root.AddCommand(
		a.newAgentsCommand(),
		a.newDoctorCommand(),
		a.newInstallCommand(),
		a.newAuthCommand(),
		a.newDefaultCommand(),
		a.newUseCommand(),
		a.newRouteCommand(),
		a.newRunCommand(),
		a.newWorkflowCommand(),
		a.newRunsCommand(),
		a.newUsageCommand(),
		a.newConfigCommand(),
	)
	return root
}

func (a *app) load() (config.Config, string, *agent.Registry, error) {
	cfg, path, err := config.Load(a.configPath)
	if err != nil {
		return config.Config{}, path, nil, err
	}
	return cfg, path, agent.NewRegistry(cfg), nil
}

func newStore() (*runstore.Store, error) {
	return runstore.New()
}

func encodeJSON(writer io.Writer, value interface{}) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
}

func resolveCwd(value string) (string, error) {
	if value == "" {
		return os.Getwd()
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", absolute)
	}
	return absolute, nil
}

func splitNativeArgs(command *cobra.Command, args []string) ([]string, []string) {
	index := command.ArgsLenAtDash()
	if index < 0 {
		return args, nil
	}
	return args[:index], args[index:]
}

func structToCommand(command string, args []string) agent.CommandSpec {
	return agent.CommandSpec{Command: command, Args: args}
}
