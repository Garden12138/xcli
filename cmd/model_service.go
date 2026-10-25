package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Garden12138/xcli/internal/jobs"
	"github.com/Garden12138/xcli/internal/modelstore"
	"github.com/spf13/cobra"
)

func (a *app) newModelServiceCommands(stateDir *string) []*cobra.Command {
	start := &cobra.Command{Use: "start", Short: "Start or discover the local gateway without model requests", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		cfg, path, err := modelstore.Load(a.configPath)
		if err != nil {
			return err
		}
		store, err := modelstore.New(*stateDir)
		if err != nil {
			return err
		}
		lock, err := modelstore.Lock(filepath.Join(store.Root, "start.lock"), true)
		if err != nil {
			return err
		}
		defer lock.Close()
		state, err := store.Status()
		if err != nil {
			return err
		}
		if state.Active {
			if state.ConfigDigest != modelstore.ConfigDigest(cfg) {
				return errors.New("gateway already runs with different configuration; stop it before changing routes")
			}
			state.Reachable = modelstore.Probe(c.Context(), state)
			if !state.Reachable {
				return errors.New("gateway holds its instance lock but is not healthy; inspect model logs")
			}
			return encodeJSON(c.OutOrStdout(), state)
		}
		// Rotate only this service-owned diagnostic file. It never contains content.
		logPath := filepath.Join(store.Root, "service.log")
		if info, err := os.Lstat(logPath); err == nil {
			if _, err = modelstore.ReadPrivate(logPath, 4<<20); err != nil {
				return err
			}
			if info.Size() >= 2<<20 {
				if err = os.Remove(logPath); err != nil {
					return err
				}
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		log, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer log.Close()
		binary, err := executablePath()
		if err != nil {
			return err
		}
		worker := exec.Command(binary, "--config", path, "model", "--state-dir", store.Root, "serve")
		jobs.ConfigureDetached(worker)
		null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer null.Close()
		worker.Stdin = null
		worker.Stdout = null
		worker.Stderr = log
		if err = worker.Start(); err != nil {
			return err
		}
		finished := make(chan error, 1)
		go func() { finished <- worker.Wait() }()
		ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
		defer cancel()
		ticker := time.NewTicker(30 * time.Millisecond)
		defer ticker.Stop()
		for {
			state, err = store.Status()
			if err != nil {
				_ = worker.Process.Kill()
				<-finished
				return err
			}
			if state.Active && state.ConfigDigest == modelstore.ConfigDigest(cfg) {
				state.Reachable = modelstore.Probe(ctx, state)
				if state.Reachable {
					return encodeJSON(c.OutOrStdout(), state)
				}
			}
			select {
			case err := <-finished:
				if err == nil {
					return errors.New("gateway exited during startup; inspect model logs")
				}
				return errors.New("gateway could not start; check model doctor, port conflicts and model logs")
			case <-ctx.Done():
				// Only the newly started process handle is terminated, never a stored PID.
				_ = worker.Process.Kill()
				<-finished
				return errors.New("gateway startup timed out; inspect model logs")
			case <-ticker.C:
			}
		}
	}}
	status := &cobra.Command{Use: "status", Short: "Inspect the OS service lock and actual bound address", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		store, err := modelstore.New(*stateDir)
		if err != nil {
			return err
		}
		state, err := store.Status()
		if err != nil {
			return err
		}
		state.Reachable = modelstore.Probe(c.Context(), state)
		return encodeJSON(c.OutOrStdout(), state)
	}}
	var timeout time.Duration
	stop := &cobra.Command{Use: "stop", Short: "Stop the current gateway instance and cancel active requests", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if timeout <= 0 {
			return errors.New("stop timeout must be positive")
		}
		store, err := modelstore.New(*stateDir)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), timeout)
		defer cancel()
		if err = store.Stop(ctx); err != nil {
			return err
		}
		fmt.Fprintln(c.OutOrStdout(), "Model Gateway stopped")
		return nil
	}}
	stop.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "maximum wait for this service instance to stop")
	logs := &cobra.Command{Use: "logs", Short: "Read bounded private gateway diagnostics (no prompts or credentials)", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		store, err := modelstore.New(*stateDir)
		if err != nil {
			return err
		}
		for _, name := range []string{"service.log", "requests.jsonl"} {
			data, err := modelstore.ReadPrivate(filepath.Join(store.Root, name), 4<<20)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if _, err = c.OutOrStdout().Write(data); err != nil {
				return err
			}
		}
		return nil
	}}
	return []*cobra.Command{start, status, stop, logs}
}
