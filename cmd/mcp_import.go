package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/Garden12138/xcli/internal/mcp"
	"github.com/spf13/cobra"
)

type mcpImportFlags struct {
	scope   string
	project string
	targets []string
	asJSON  bool
	force   bool
	yes     bool
}

func (a *app) newMCPImportCommand() *cobra.Command {
	command := &cobra.Command{Use: "import", Short: "Import native MCP configurations into xcli"}
	command.AddCommand(a.newMCPImportPlanCommand(), a.newMCPImportApplyCommand())
	return command
}

func (a *app) newMCPImportPlanCommand() *cobra.Command {
	flags := &mcpImportFlags{}
	command := &cobra.Command{
		Use:   "plan",
		Short: "Preview native MCP entries that can be imported",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			plan, _, err := a.buildMCPImportPlan(flags)
			if err != nil {
				return err
			}
			if err := writeMCPImportPlan(command, plan, flags.asJSON); err != nil {
				return err
			}
			if !plan.Applicable {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	addMCPImportFlags(command, flags, false)
	return command
}

func (a *app) newMCPImportApplyCommand() *cobra.Command {
	flags := &mcpImportFlags{}
	command := &cobra.Command{
		Use:   "apply",
		Short: "Import native MCP entries into the xcli source configuration",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			plan, state, err := a.buildMCPImportPlan(flags)
			if err != nil {
				return err
			}
			if flags.asJSON && !flags.yes {
				return errors.New("--json requires --yes for MCP import apply")
			}
			if !flags.asJSON {
				if err := writeMCPImportPlan(command, plan, false); err != nil {
					return err
				}
			}
			if !plan.Applicable {
				if flags.asJSON {
					_ = writeMCPImportPlan(command, plan, true)
				}
				return &ExitError{Code: 1}
			}
			if !plan.HasChanges() {
				if state.NeedsSave() {
					if err := state.Save(); err != nil {
						return err
					}
				}
				plan.Applied = true
				if flags.asJSON {
					return writeMCPImportPlan(command, plan, true)
				}
				fmt.Fprintln(command.OutOrStdout(), "No importable MCP changes found.")
				return nil
			}
			if !flags.yes {
				input := command.InOrStdin()
				file, ok := input.(*os.File)
				if !ok || !isTerminal(file) {
					return errors.New("refusing to import MCP configuration without a terminal; pass --yes to confirm")
				}
				fmt.Fprint(command.OutOrStdout(), "Apply MCP import changes? [y/N] ")
				answer, _ := bufio.NewReader(input).ReadString('\n')
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "y" && answer != "yes" {
					fmt.Fprintln(command.OutOrStdout(), "Canceled.")
					return nil
				}
			}
			if err := mcp.ApplyImportPlan(&plan, state); err != nil {
				if flags.asJSON {
					_ = writeMCPImportPlan(command, plan, true)
				}
				return err
			}
			if flags.asJSON {
				return writeMCPImportPlan(command, plan, true)
			}
			fmt.Fprintln(command.OutOrStdout(), "MCP import complete.")
			return nil
		},
	}
	addMCPImportFlags(command, flags, true)
	return command
}

func addMCPImportFlags(command *cobra.Command, flags *mcpImportFlags, apply bool) {
	command.Flags().StringVar(&flags.scope, "scope", mcp.ScopeUser, "configuration scope: user or project")
	command.Flags().StringVar(&flags.project, "project", "", "project directory (required for project scope)")
	command.Flags().StringArrayVar(&flags.targets, "target", nil, "native MCP target to read (repeatable)")
	command.Flags().BoolVar(&flags.asJSON, "json", false, "print JSON output")
	if apply {
		command.Flags().BoolVarP(&flags.yes, "yes", "y", false, "apply without a confirmation prompt")
		command.Flags().BoolVar(&flags.force, "force", false, "overwrite source conflicts or accept native ownership drift")
	}
}

func (a *app) buildMCPImportPlan(flags *mcpImportFlags) (mcp.ImportPlan, *mcp.State, error) {
	cfg, source, _, err := a.load()
	if err != nil {
		return mcp.ImportPlan{}, nil, err
	}
	projectDir := ""
	switch flags.scope {
	case mcp.ScopeUser:
		if flags.project != "" {
			return mcp.ImportPlan{}, nil, errors.New("--project is only valid with --scope project")
		}
		source, err = filepath.Abs(source)
	case mcp.ScopeProject:
		projectDir, source, _, err = resolveMCPProject(flags.project, source)
	default:
		err = fmt.Errorf("invalid MCP scope %q; expected user or project", flags.scope)
	}
	if err != nil {
		return mcp.ImportPlan{}, nil, err
	}
	options := mcp.ImportReadOptions{Scope: flags.scope, ProjectDir: projectDir, SourceConfig: source}
	paths, err := mcp.NativeConfigPaths(cfg, options)
	if err != nil {
		return mcp.ImportPlan{}, nil, err
	}
	targets, err := resolveMCPImportTargets(flags.targets, paths)
	if err != nil {
		return mcp.ImportPlan{}, nil, err
	}
	snapshots := map[string]mcp.NativeSnapshot{}
	for _, target := range targets {
		snapshot, err := mcp.ReadNativeSnapshot(target, paths[target], options)
		if err != nil {
			return mcp.ImportPlan{}, nil, fmt.Errorf("read %s native MCP configuration %s: %w", target, paths[target], err)
		}
		snapshots[target] = snapshot
	}
	state, err := mcp.LoadState()
	if err != nil {
		return mcp.ImportPlan{}, nil, err
	}
	sourceData, sourceErr := os.ReadFile(source)
	sourceExists := sourceErr == nil
	if sourceErr != nil && !errors.Is(sourceErr, os.ErrNotExist) {
		return mcp.ImportPlan{}, nil, sourceErr
	}
	plan, err := mcp.BuildImportPlan(cfg, snapshots, state, mcp.ImportBuildOptions{
		SourceConfig: source, Scope: flags.scope, ProjectDir: projectDir, Targets: targets, Force: flags.force,
		VerifySource: true, SourceExists: sourceExists, SourceFingerprint: importContentFingerprint(sourceData),
	})
	return plan, state, err
}

func importContentFingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func resolveMCPImportTargets(requested []string, paths map[string]string) ([]string, error) {
	if len(requested) > 0 {
		seen := map[string]bool{}
		result := make([]string, 0, len(requested))
		for _, target := range requested {
			if !mcp.IsTarget(target) {
				return nil, fmt.Errorf("unknown MCP target %q", target)
			}
			if seen[target] {
				return nil, fmt.Errorf("MCP target %q was specified more than once", target)
			}
			seen[target] = true
			result = append(result, target)
		}
		sort.Strings(result)
		return result, nil
	}
	result := []string{}
	for _, target := range mcp.Targets {
		if _, err := os.Stat(paths[target]); err == nil {
			result = append(result, target)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if len(result) == 0 {
		return nil, errors.New("no native MCP configuration files found for this scope")
	}
	sort.Strings(result)
	return result, nil
}

func writeMCPImportPlan(command *cobra.Command, plan mcp.ImportPlan, asJSON bool) error {
	if asJSON {
		return encodeJSON(command.OutOrStdout(), plan)
	}
	writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "SERVER\tTARGETS\tACTION\tSTATUS\tDETAIL")
	for _, change := range plan.Changes {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", change.Server, strings.Join(change.Targets, ","), change.Action, change.Status, change.Detail)
	}
	return writer.Flush()
}
