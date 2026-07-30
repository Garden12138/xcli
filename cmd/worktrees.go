package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/Garden12138/xcli/internal/worktree"
	"github.com/spf13/cobra"
)

type worktreeView struct {
	worktree.Metadata
	Status string `json:"status"`
}

func (a *app) newWorktreesCommand() *cobra.Command {
	command := &cobra.Command{Use: "worktrees", Short: "Inspect and remove xcli-managed Git worktrees"}
	command.AddCommand(a.newWorktreesListCommand(), a.newWorktreesShowCommand(), a.newWorktreesRemoveCommand())
	return command
}

func (a *app) newWorktreesListCommand() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List xcli-managed Git worktrees",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			manager, err := worktree.New()
			if err != nil {
				return err
			}
			metadata, err := manager.List()
			if err != nil {
				return err
			}
			views, err := worktreeViews(metadata)
			if err != nil {
				return err
			}
			if asJSON {
				return encodeJSON(command.OutOrStdout(), views)
			}
			writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "RUN\tSTATUS\tBRANCH\tPATH\tCREATED")
			for _, view := range views {
				fmt.Fprintf(
					writer, "%s\t%s\t%s\t%s\t%s\n",
					view.RunID, view.Status, view.Branch, view.Path,
					view.CreatedAt.Local().Format(time.RFC3339),
				)
			}
			return writer.Flush()
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func (a *app) newWorktreesShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show one managed worktree as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			manager, err := worktree.New()
			if err != nil {
				return err
			}
			metadata, err := manager.Load(args[0])
			if err != nil {
				return err
			}
			views, err := worktreeViews([]worktree.Metadata{metadata})
			if err != nil {
				return err
			}
			return encodeJSON(command.OutOrStdout(), views[0])
		},
	}
}

func (a *app) newWorktreesRemoveCommand() *cobra.Command {
	var force bool
	var yes bool
	var asJSON bool
	command := &cobra.Command{
		Use:   "remove <run-id>",
		Short: "Remove a terminal run's managed worktree while retaining its branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if asJSON && !yes {
				return errors.New("--json requires --yes for worktree removal")
			}
			manager, err := worktree.New()
			if err != nil {
				return err
			}
			metadata, err := manager.Load(args[0])
			if err != nil {
				return err
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			record, err := store.Load(metadata.RunID)
			if errors.Is(err, os.ErrNotExist) && !force {
				return errors.New("associated run record is missing; pass --force to remove the orphaned worktree")
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("load associated run record: %w", err)
			}
			if err == nil && record.Background {
				record, err = (jobs.Manager{Store: store}).Reconcile(record)
				if err != nil {
					return err
				}
			}
			if err == nil && !jobs.IsTerminal(record.Status) {
				return fmt.Errorf("run %q is still %s; stop it before removing its worktree", record.ID, record.Status)
			}
			if !asJSON {
				fmt.Fprintf(command.OutOrStdout(), "Run: %s\nBranch retained: %s\nWorktree removed: %s\n", metadata.RunID, metadata.Branch, metadata.Path)
				if force {
					fmt.Fprintln(command.OutOrStdout(), "Warning: --force discards uncommitted changes in the worktree.")
				}
			}
			confirmed, err := confirmWorktreeRemoval(command, yes)
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(command.OutOrStdout(), "Canceled.")
				return nil
			}
			if err := manager.Remove(metadata, force); err != nil {
				return err
			}
			result := struct {
				RunID          string `json:"run_id"`
				RemovedPath    string `json:"removed_path"`
				RetainedBranch string `json:"retained_branch"`
			}{RunID: metadata.RunID, RemovedPath: metadata.Path, RetainedBranch: metadata.Branch}
			if asJSON {
				return encodeJSON(command.OutOrStdout(), result)
			}
			fmt.Fprintf(command.OutOrStdout(), "Removed worktree for %s; branch %s was retained.\n", metadata.RunID, metadata.Branch)
			return nil
		},
	}
	command.Flags().BoolVar(&force, "force", false, "discard uncommitted worktree changes")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "remove without a confirmation prompt")
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON output")
	return command
}

func worktreeViews(metadata []worktree.Metadata) ([]worktreeView, error) {
	store, err := newStore()
	if err != nil {
		return nil, err
	}
	result := make([]worktreeView, 0, len(metadata))
	for _, item := range metadata {
		status := "unknown"
		if record, err := store.Load(item.RunID); err == nil {
			if record.Background {
				record, err = (jobs.Manager{Store: store}).Reconcile(record)
				if err != nil {
					return nil, err
				}
			}
			status = record.Status
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		result = append(result, worktreeView{Metadata: item, Status: status})
	}
	return result, nil
}

func confirmWorktreeRemoval(command *cobra.Command, yes bool) (bool, error) {
	if yes {
		return true, nil
	}
	fmt.Fprint(command.OutOrStdout(), "Remove this managed worktree? [y/N] ")
	line, err := bufio.NewReader(command.InOrStdin()).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
