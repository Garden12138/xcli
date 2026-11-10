package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sort"
	"time"

	"github.com/Garden12138/xcli/internal/launch"
	"github.com/spf13/cobra"
)

func (a *app) newLaunchesCommand() *cobra.Command {
	g := &cobra.Command{Use: "launches", Short: "Audit native process starts and explicit budget windows (no CLI probes)"}
	var kind string
	probe := &cobra.Command{Use: "probe <agent>", Short: "Explicit, budgeted version/help/read-only login-status probe", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		_, _, registry, e := a.load()
		if e != nil {
			return e
		}
		d, e := registry.Get(args[0])
		if e != nil {
			return e
		}
		switch kind {
		case "version":
			v := d.Detect(c.Context())
			if v.Error != "" {
				return errors.New(v.Error)
			}
			return encodeJSON(c.OutOrStdout(), v)
		case "help":
			v := d.Probe(c.Context())
			if v.Error != "" {
				return errors.New(v.Error)
			}
			return encodeJSON(c.OutOrStdout(), v)
		case "status":
			var argv []string
			switch d.Config.Adapter {
			case "codex":
				argv = []string{"login", "status"}
			case "cursor":
				argv = []string{"status", "--format", "json"}
			default:
				return errors.New("native_read_only_status_not_supported")
			}
			probeCtx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(probeCtx, d.Config.Command, argv...)
			cmd.WaitDelay = 250 * time.Millisecond
			var out boundedStatus
			cmd.Stdout = &out
			cmd.Stderr = io.Discard
			id, err := launch.Run(probeCtx, cmd, launch.Metadata{Agent: d.Name, Adapter: d.Config.Adapter, Kind: "login-status"})
			var authenticated *bool
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 0 {
				value := true
				if d.Config.Adapter == "cursor" {
					var body struct {
						Authenticated *bool `json:"isAuthenticated"`
					}
					if json.Unmarshal(out.Bytes(), &body) == nil {
						authenticated = body.Authenticated
					}
				} else {
					authenticated = &value
				}
			}
			if err != nil {
				return err
			}
			return encodeJSON(c.OutOrStdout(), map[string]any{"launch_id": id, "authenticated": authenticated, "exit_code": cmd.ProcessState.ExitCode()})
		default:
			return errors.New("probe kind must be version, help or status")
		}
	}}
	probe.Flags().StringVar(&kind, "kind", "version", "version (1 start), help (up to 2), or status (1)")
	g.AddCommand(probe)
	window := &cobra.Command{Use: "window", Short: "Create a new immutable budget; never reset old consumption"}
	var limit, concurrent, external int
	var agent, source, scope string
	var confirmed bool
	create := &cobra.Command{Use: "create <id>", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		if !c.Flags().Changed("limit") {
			return errors.New("explicit --limit is required")
		}
		var count *int
		if c.Flags().Changed("external-count") {
			count = &external
		}
		ref := launch.Reference(c.Context())
		e := launch.Create(ref, launch.Window{ID: args[0], Limit: limit, MaxConcurrent: concurrent, External: launch.External{Agent: agent, Count: count, Source: source, Scope: scope, Confirmed: confirmed}})
		if e != nil {
			return e
		}
		ref.Window = args[0]
		stats, e := launch.Stats(ref)
		if e != nil {
			return e
		}
		return encodeJSON(c.OutOrStdout(), stats)
	}}
	create.Flags().IntVar(&limit, "limit", 0, "total native starts, including scoped external baseline")
	create.Flags().IntVar(&concurrent, "concurrency", 1, "maximum active or unconfirmed-stopped native launches")
	create.Flags().IntVar(&external, "external-count", 0, "explicit external launch count; omission is UNKNOWN")
	create.Flags().StringVar(&agent, "external-agent", "", "external controller agent, or none")
	create.Flags().StringVar(&source, "external-source", "", "non-secret owner evidence/reference")
	create.Flags().StringVar(&scope, "external-scope", "unknown", "included, excluded or unknown; must be explicitly confirmed")
	create.Flags().BoolVar(&confirmed, "confirm-external", false, "owner confirms baseline count and scope; never inferred")
	window.AddCommand(create)
	g.AddCommand(window)
	g.AddCommand(&cobra.Command{Use: "budget", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		o, e := launch.Stats(launch.Reference(c.Context()))
		if e != nil {
			return e
		}
		return encodeJSON(c.OutOrStdout(), o)
	}})
	var taskID, runID string
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		ref := launch.Reference(c.Context())
		s, e := launch.Snapshot(ref)
		if e != nil {
			return e
		}
		out := []launch.Record{}
		for _, r := range s.Records {
			if ref.Window != "" && r.Window != ref.Window || taskID != "" && r.TaskID != taskID || runID != "" && r.RunID != runID {
				continue
			}
			out = append(out, r)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].ReservedAt.Before(out[j].ReservedAt) })
		return encodeJSON(c.OutOrStdout(), out)
	}}
	list.Flags().StringVar(&taskID, "task", "", "filter Task ID")
	list.Flags().StringVar(&runID, "run", "", "filter Run ID")
	g.AddCommand(list)
	g.AddCommand(&cobra.Command{Use: "show <launch-id>", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		s, e := launch.Snapshot(launch.Reference(c.Context()))
		if e != nil {
			return e
		}
		r, ok := s.Records[args[0]]
		if !ok {
			return errors.New("launch_not_found")
		}
		return encodeJSON(c.OutOrStdout(), r)
	}})
	g.AddCommand(&cobra.Command{Use: "windows", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		s, e := launch.Snapshot(launch.Reference(c.Context()))
		if e != nil {
			return e
		}
		return encodeJSON(c.OutOrStdout(), s.Windows)
	}})
	var proof string
	var stopConfirmed bool
	reconcile := &cobra.Command{Use: "confirm-stopped <launch-id>", Args: cobra.ExactArgs(1), RunE: func(c *cobra.Command, args []string) error {
		return launch.ConfirmStopped(launch.Reference(c.Context()), args[0], proof, stopConfirmed)
	}}
	reconcile.Flags().StringVar(&proof, "evidence", "", "owner-verified stop evidence (no secrets)")
	reconcile.Flags().BoolVar(&stopConfirmed, "confirm", false, "confirm stopped; never refunds startup budget")
	g.AddCommand(reconcile)
	var legacySource string
	legacy := &cobra.Command{Use: "record-legacy <id>", Args: cobra.ExactArgs(1), Short: "Record unprovable historical startup totals as UNKNOWN", RunE: func(c *cobra.Command, args []string) error {
		return launch.Create(launch.Reference(c.Context()), launch.Window{ID: args[0], MaxConcurrent: 1, External: launch.External{Agent: "unknown", Source: legacySource, Scope: "unknown"}})
	}}
	legacy.Flags().StringVar(&legacySource, "source", "", "historical report reference; no counts are reconstructed")
	g.AddCommand(legacy)
	return g
}

// Native status payloads may contain account data. Only a parsed boolean is
// returned; neither raw stdout nor stderr enters the ledger or CLI output.
type boundedStatus struct{ bytes.Buffer }

func (b *boundedStatus) Write(p []byte) (int, error) {
	n := len(p)
	if left := (64 << 10) - b.Len(); left > 0 {
		_, _ = b.Buffer.Write(p[:min(left, len(p))])
	}
	return n, nil
}
