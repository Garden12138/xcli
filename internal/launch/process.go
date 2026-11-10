package launch

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type refKey struct{}
type metadataKey struct{}

func WithRef(ctx context.Context, r Ref) context.Context { return context.WithValue(ctx, refKey{}, r) }
func Reference(ctx context.Context) Ref {
	if r, ok := ctx.Value(refKey{}).(Ref); ok {
		return r
	}
	return Ref{os.Getenv("XCLI_LAUNCH_STORE"), os.Getenv("XCLI_LAUNCH_WINDOW")}
}
func WithMetadata(ctx context.Context, m Metadata) context.Context {
	old, _ := ctx.Value(metadataKey{}).(Metadata)
	if m.TaskID == "" {
		m.TaskID = old.TaskID
	}
	if m.RunID == "" {
		m.RunID = old.RunID
	}
	if m.ParentTask == "" {
		m.ParentTask = old.ParentTask
	}
	if m.Phase == "" {
		m.Phase = old.Phase
	}
	return context.WithValue(ctx, metadataKey{}, m)
}
func MetadataFrom(ctx context.Context) Metadata {
	m, _ := ctx.Value(metadataKey{}).(Metadata)
	return m
}
func ChildEnvironment(ctx context.Context, env []string) []string {
	r := Reference(ctx)
	out := []string{}
	for _, v := range env {
		if !strings.HasPrefix(v, "XCLI_LAUNCH_STORE=") && !strings.HasPrefix(v, "XCLI_LAUNCH_WINDOW=") {
			out = append(out, v)
		}
	}
	return append(out, "XCLI_LAUNCH_STORE="+r.Root, "XCLI_LAUNCH_WINDOW="+r.Window)
}
func KnownCLI(command string) bool {
	switch strings.TrimSuffix(filepath.Base(command), ".exe") {
	case "codex", "cursor-agent", "agy", "claude", "gemini", "opencode":
		return true
	}
	return false
}

// Start is the native spawn boundary. A successful durable intent precedes
// Cmd.Start; no error after the OS start can release budget.
func Start(ctx context.Context, cmd *exec.Cmd, m Metadata) (*Ticket, error) {
	ctx = WithMetadata(ctx, m)
	m = MetadataFrom(ctx)
	executable := cmd.Path
	if absolute, e := filepath.Abs(executable); e == nil {
		executable = absolute
	}
	if resolved, e := filepath.EvalSymlinks(executable); e == nil {
		executable = resolved
	}
	if m.Agent == "" {
		m.Agent = filepath.Base(executable)
	}
	if m.Kind == "" {
		m.Kind = "native"
	}
	parts := append([]string{m.Agent, m.Adapter, m.Kind, m.Phase, cmd.Dir}, cmd.Args...)
	t, e := Reserve(Reference(ctx), m, executable, Digest(parts...))
	if e != nil {
		return nil, e
	}
	if ctx.Err() != nil {
		e = t.NotStarted("context_done_before_start")
		return t, errors.Join(ctx.Err(), e)
	}
	if e = t.Starting(); e != nil {
		t.Close()
		return t, e
	}
	if e = cmd.Start(); e != nil {
		if cmd.Process == nil {
			return t, errors.Join(e, t.NotStarted("os_start_failed"))
		}
		t.Close()
		return t, e
	}
	if e = t.Started(cmd.Process.Pid); e != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Close()
		return t, e
	}
	return t, nil
}

// Run is used by the read-only version/help probes, with the same reservations
// and lifecycle as model-bearing executions. stdout/stderr stay with the caller.
func Run(ctx context.Context, cmd *exec.Cmd, m Metadata) (string, error) {
	t, e := Start(ctx, cmd, m)
	if e != nil {
		if t != nil {
			return t.ID, e
		}
		return "", e
	}
	e = cmd.Wait()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	finish := t.Finish(code, errors.Is(ctx.Err(), context.Canceled), errors.Is(ctx.Err(), context.DeadlineExceeded), cmd.ProcessState != nil)
	return t.ID, errors.Join(e, finish)
}

func SameRef(a, b Ref) bool {
	if a.Root == "" {
		a.Root, _ = DefaultRoot()
	}
	if b.Root == "" {
		b.Root, _ = DefaultRoot()
	}
	a.Root, _ = filepath.Abs(a.Root)
	b.Root, _ = filepath.Abs(b.Root)
	return a == b
}
