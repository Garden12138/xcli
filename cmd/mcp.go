package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/Garden12138/xcli/internal/agent"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/mcp"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
	"github.com/spf13/cobra"
)

func (a *app) newMCPCommand() *cobra.Command {
	command := &cobra.Command{Use: "mcp", Short: "Import, plan, synchronize, and serve MCP configurations"}
	command.AddCommand(a.newMCPPlanCommand(), a.newMCPSyncCommand(), a.newMCPImportCommand(), a.newMCPServeCommand())
	return command
}

func (a *app) newMCPPlanCommand() *cobra.Command {
	var targets []string
	var launcher string
	var scope string
	var project string
	var asJSON bool
	command := &cobra.Command{
		Use:   "plan",
		Short: "Preview MCP configuration changes",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			plan, _, _, err := a.buildMCPPlan(command, targets, launcher, scope, project, false)
			if err != nil {
				return err
			}
			if err := writeMCPPlan(command, plan, asJSON); err != nil {
				return err
			}
			if !plan.Applicable {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	command.Flags().StringArrayVar(&targets, "target", nil, "MCP target to include (repeatable)")
	command.Flags().StringVar(&launcher, "launcher", "", "stdio launcher (user path or project PATH command)")
	command.Flags().StringVar(&scope, "scope", mcp.ScopeUser, "configuration scope: user or project")
	command.Flags().StringVar(&project, "project", "", "project directory (required for project scope)")
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func (a *app) newMCPSyncCommand() *cobra.Command {
	var targets []string
	var launcher string
	var scope string
	var project string
	var yes bool
	var force bool
	var asJSON bool
	command := &cobra.Command{
		Use:   "sync",
		Short: "Synchronize managed MCP servers to installed agents",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			plan, managers, state, err := a.buildMCPPlan(command, targets, launcher, scope, project, force)
			if err != nil {
				return err
			}
			if asJSON && !yes {
				return errors.New("--json requires --yes for MCP sync")
			}
			if !asJSON {
				if err := writeMCPPlan(command, plan, false); err != nil {
					return err
				}
			}
			if !plan.Applicable {
				if asJSON {
					_ = writeMCPPlan(command, plan, true)
				}
				return &ExitError{Code: 1}
			}
			if !hasMCPChanges(plan) {
				if state.NeedsSave() {
					if err := state.Save(); err != nil {
						return err
					}
				}
				plan.Applied = true
				if asJSON {
					return writeMCPPlan(command, plan, true)
				}
				fmt.Fprintln(command.OutOrStdout(), "MCP configuration is already synchronized.")
				return nil
			}
			if plan.Scope == mcp.ScopeUser && launcher == "" && plan.UsesLauncherChanges() && pathWithinTemp(plan.Launcher) {
				return errors.New("refusing to synchronize a temporary xcli launcher; install xcli or pass --launcher")
			}
			if !yes {
				input := command.InOrStdin()
				file, ok := input.(*os.File)
				if !ok || !isTerminal(file) {
					return errors.New("refusing to synchronize MCP configuration without a terminal; pass --yes to confirm")
				}
				fmt.Fprint(command.OutOrStdout(), "Apply MCP changes? [y/N] ")
				answer, _ := bufio.NewReader(input).ReadString('\n')
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "y" && answer != "yes" {
					fmt.Fprintln(command.OutOrStdout(), "Canceled.")
					return nil
				}
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			if err := mcp.ApplyPlan(ctx, &plan, managers, state); err != nil {
				if asJSON {
					_ = writeMCPPlan(command, plan, true)
				}
				return err
			}
			if asJSON {
				return writeMCPPlan(command, plan, true)
			}
			fmt.Fprintln(command.OutOrStdout(), "MCP synchronization complete.")
			return nil
		},
	}
	command.Flags().StringArrayVar(&targets, "target", nil, "MCP target to include (repeatable)")
	command.Flags().StringVar(&launcher, "launcher", "", "stdio launcher (user path or project PATH command)")
	command.Flags().StringVar(&scope, "scope", mcp.ScopeUser, "configuration scope: user or project")
	command.Flags().StringVar(&project, "project", "", "project directory (required for project scope)")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "apply without a confirmation prompt")
	command.Flags().BoolVar(&force, "force", false, "take over or overwrite conflicting MCP entries")
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func (a *app) newMCPServeCommand() *cobra.Command {
	var projectConfig string
	command := &cobra.Command{
		Use:   "serve <server>",
		Short: "Run a configured stdio MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			var cfg config.Config
			var path string
			if projectConfig != "" {
				if a.configPath != "" {
					return errors.New("--project-config cannot be combined with --config")
				}
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				path, err = mcp.ResolveProjectConfig(cwd, projectConfig)
				if err != nil {
					return err
				}
				cfg, _, err = config.Load(path)
				if err != nil {
					return err
				}
			} else {
				var err error
				cfg, path, _, err = a.load()
				if err != nil {
					return err
				}
			}
			spec, err := mcp.BuildServeSpec(cfg, path, args[0], os.Environ())
			if err != nil {
				return err
			}
			if _, err := exec.LookPath(spec.Command.Command); err != nil {
				return fmt.Errorf("MCP server command %q was not found", spec.Command.Command)
			}
			ctx, cancel := signalContext(command.Context())
			defer cancel()
			result, err := xruntime.RunProcess(ctx, spec.Command, xruntime.ProcessOptions{
				Dir: spec.Dir, Env: spec.Env, Stdin: command.InOrStdin(),
				Stdout: command.OutOrStdout(), Stderr: command.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			if result.ExitCode != 0 {
				return &ExitError{Code: result.ExitCode}
			}
			return nil
		},
	}
	command.Flags().StringVar(&projectConfig, "project-config", "", "project-relative xcli configuration path")
	return command
}

func (a *app) buildMCPPlan(command *cobra.Command, requested []string, launcherValue, scope, projectValue string, force bool) (mcp.Plan, map[string]mcp.Manager, *mcp.State, error) {
	cfg, source, registry, err := a.load()
	if err != nil {
		return mcp.Plan{}, nil, nil, err
	}
	var launcher string
	var projectDir string
	var projectConfig string
	switch scope {
	case mcp.ScopeUser:
		if projectValue != "" {
			return mcp.Plan{}, nil, nil, errors.New("--project is only valid with --scope project")
		}
		launcher, err = resolveLauncher(launcherValue)
	case mcp.ScopeProject:
		projectDir, source, projectConfig, err = resolveMCPProject(projectValue, source)
		if err == nil {
			launcher, err = resolvePortableLauncher(launcherValue)
		}
	default:
		err = fmt.Errorf("invalid MCP scope %q; expected user or project", scope)
	}
	if err != nil {
		return mcp.Plan{}, nil, nil, err
	}
	targets, unavailable, err := resolveMCPTargets(command, cfg, registry, requested)
	if err != nil {
		return mcp.Plan{}, nil, nil, err
	}
	var managers map[string]mcp.Manager
	if scope == mcp.ScopeProject {
		managers, err = mcp.NewProjectManagers(cfg, projectDir)
	} else {
		managers, err = mcp.NewManagers(cfg)
	}
	if err != nil {
		return mcp.Plan{}, nil, nil, err
	}
	state, err := mcp.LoadState()
	if err != nil {
		return mcp.Plan{}, nil, nil, err
	}
	plan, err := mcp.BuildPlan(command.Context(), cfg, managers, state, mcp.BuildOptions{
		SourceConfig: source, Launcher: launcher, Scope: scope, ProjectDir: projectDir,
		ProjectConfig: projectConfig, Targets: targets, Unavailable: unavailable, Force: force,
	})
	return plan, managers, state, err
}

func resolveMCPProject(value, source string) (string, string, string, error) {
	if value == "" {
		return "", "", "", errors.New("--project is required with --scope project")
	}
	project, err := canonicalExistingPath(value, true)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve MCP project: %w", err)
	}
	canonicalSource, err := canonicalExistingPath(source, false)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve MCP source configuration: %w", err)
	}
	relative, err := filepath.Rel(project, canonicalSource)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", "", fmt.Errorf("MCP source configuration %s is outside project %s", canonicalSource, project)
	}
	return project, canonicalSource, filepath.ToSlash(relative), nil
}

func canonicalExistingPath(value string, wantDirectory bool) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if wantDirectory && !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", canonical)
	}
	if !wantDirectory && !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", canonical)
	}
	return canonical, nil
}

func resolvePortableLauncher(value string) (string, error) {
	if value == "" {
		value = "xcli"
	}
	if filepath.IsAbs(value) || strings.ContainsAny(value, `/\\`) {
		return "", fmt.Errorf("project MCP launcher %q must be a PATH command name", value)
	}
	path, err := exec.LookPath(value)
	if err != nil {
		return "", fmt.Errorf("resolve project MCP launcher %q: %w", value, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("project MCP launcher %q is not executable", value)
	}
	return value, nil
}

func resolveMCPTargets(command *cobra.Command, cfg config.Config, registry *agent.Registry, requested []string) ([]string, []string, error) {
	values := requested
	if len(values) == 0 {
		values = append([]string(nil), mcp.Targets...)
	}
	seen := map[string]bool{}
	available := []string{}
	unavailable := []string{}
	for _, target := range values {
		if !mcp.IsTarget(target) {
			return nil, nil, fmt.Errorf("unknown MCP target %q", target)
		}
		if seen[target] {
			return nil, nil, fmt.Errorf("MCP target %q was specified more than once", target)
		}
		seen[target] = true
		definition, err := registry.Get(target)
		if err != nil || definition.Config.Adapter != target {
			if len(requested) > 0 {
				unavailable = append(unavailable, target)
			}
			continue
		}
		if definition.Detect(command.Context()).Installed {
			available = append(available, target)
		} else if len(requested) > 0 {
			unavailable = append(unavailable, target)
		}
	}
	if len(available) == 0 && len(unavailable) == 0 {
		return nil, nil, errors.New("no installed MCP targets found")
	}
	sort.Strings(available)
	sort.Strings(unavailable)
	return available, unavailable, nil
}

func resolveLauncher(value string) (string, error) {
	var path string
	var err error
	if value == "" {
		path, err = os.Executable()
	} else {
		path, err = exec.LookPath(value)
	}
	if err != nil {
		return "", fmt.Errorf("resolve MCP launcher: %w", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
		path = resolved
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("MCP launcher %s is not executable", path)
	}
	return path, nil
}

func pathWithinTemp(path string) bool {
	temp, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		temp = os.TempDir()
	}
	path, _ = filepath.EvalSymlinks(path)
	relative, err := filepath.Rel(temp, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func hasMCPChanges(plan mcp.Plan) bool {
	for _, change := range plan.Changes {
		if change.Action == mcp.ActionAdd || change.Action == mcp.ActionUpdate || change.Action == mcp.ActionRemove {
			return true
		}
	}
	return false
}

func writeMCPPlan(command *cobra.Command, plan mcp.Plan, asJSON bool) error {
	if asJSON {
		return encodeJSON(command.OutOrStdout(), plan)
	}
	writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "TARGET\tSERVER\tACTION\tSTATUS\tDETAIL")
	for _, change := range plan.Changes {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", change.Target, change.Server, change.Action, change.Status, change.Detail)
	}
	return writer.Flush()
}
