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
	"github.com/Garden12138/xcli/internal/launch"
	"github.com/Garden12138/xcli/internal/runstore"
	"github.com/spf13/cobra"
)

var Version = "0.5.0-r4.1-dev"

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
	configPath                              string
	experimentalGateways                    bool
	launchRoot, launchWindow, launchRequest string
}

func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	a := &app{}
	root := &cobra.Command{
		Use:           "xcli",
		Short:         "Complete tasks with native Agent CLIs and verifiable results",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&a.configPath, "config", "", "explicit configuration file")
	root.PersistentFlags().BoolVar(&a.experimentalGateways, "experimental-gateways", false, "explicitly opt into frozen Gateway experiments; never enables native profiles or SIWC")
	_ = root.PersistentFlags().MarkHidden("experimental-gateways")
	root.PersistentFlags().StringVar(&a.launchRoot, "launch-store", os.Getenv("XCLI_LAUNCH_STORE"), "shared private native launch ledger root")
	root.PersistentFlags().StringVar(&a.launchWindow, "launch-window", os.Getenv("XCLI_LAUNCH_WINDOW"), "explicit process budget window; required for native starts")
	root.PersistentFlags().StringVar(&a.launchRequest, "launch-request", "", "idempotency key for one direct run/resume/use launch")
	root.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		root := a.launchRoot
		if root == "" {
			var e error
			root, e = launch.DefaultRoot()
			if e != nil {
				return e
			}
		}
		c.SetContext(launch.WithRef(c.Context(), launch.Ref{Root: root, Window: a.launchWindow}))
		return nil
	}
	root.AddCommand(a.newLaunchesCommand(),
		a.newTaskCommand(),
		a.newTaskWorkerCommand(), a.newTaskAttemptCommand(),
		a.newAgentsCommand(),
		a.newDoctorCommand(),
		a.newUseCommand(),
		a.newMCPCommand(),
		a.newRunCommand(),
		a.newJobWorkerCommand(),
		a.newJobsCommand(),
		a.newResumeCommand(),
		a.newRunsCommand(),
		a.newConfigCommand(),
		a.newModelCommand(),
		a.newAgentGatewayCommand(),
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
