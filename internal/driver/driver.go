package driver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/proc"
)

type Invocation struct {
	Prompt     string
	Workdir    string
	Permission api.Permission
	Images     []string
	SchemaFile string
	SessionID  string
	Model      string
	AddDirs    []string
	Network    bool
	ExtraArgs  []string
}

type Event struct {
	Kind string // "session" | "message" | "tool" | "notice"
	Text string
	Tool string
}

type ErrorKind string

const (
	ErrNone             ErrorKind = ""
	ErrNotInstalled     ErrorKind = "not_installed"
	ErrAuth             ErrorKind = "auth"
	ErrQuota            ErrorKind = "quota"
	ErrUnavailable      ErrorKind = "unavailable"
	ErrTrust            ErrorKind = "trust"
	ErrPermissionDenied ErrorKind = "permission_denied"
	ErrNoArtifact       ErrorKind = "no_artifact"
	ErrSchemaMismatch   ErrorKind = "schema_mismatch"
	ErrTimeout          ErrorKind = "timeout"
	ErrCancelled        ErrorKind = "cancelled"
	ErrProtocol         ErrorKind = "protocol"
	ErrFailed           ErrorKind = "failed"
)

// Switchable 表示换一个 CLI 有可能成功（02 架构文档 4.4 节）。

type Result struct {
	Text      string
	SessionID string
	Usage     *api.Usage
	ErrorKind ErrorKind
	Error     string
	Denied    []string
}

type Health struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Auth      string `json:"auth"` // "ok" | "missing" | "unknown"
	Error     string `json:"error,omitempty"`
}

type Parser interface {
	Feed(line []byte) []Event
	Finish(exitCode int, stderrTail string) Result
}

type Driver interface {
	Name() string
	Command() string
	Build(inv Invocation) ([]string, error)
	NewParser() Parser
	Probe(ctx context.Context, exe string, env []string) Health
	NativeSchema() bool
}

func New(name string, cfg config.AgentConfig) (Driver, error) {
	switch name {
	case "codex":
		return &codex{cfg: cfg}, nil
	case "cursor":
		return &cursor{cfg: cfg}, nil
	case "antigravity":
		return &antigravity{cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("请检查 agent %q，可选 codex/cursor/antigravity", name)
	}
}

func All(cfg config.Config) map[string]Driver {
	drivers := make(map[string]Driver)
	for _, name := range config.AgentOrder {
		if cfg.Enabled(name) {
			if d, err := New(name, cfg.Agents[name]); err == nil {
				drivers[name] = d
			}
		}
	}
	return drivers
}

func probe(ctx context.Context, d Driver, exe string, env []string, statusArgs ...string) Health {
	h := Health{Name: d.Name(), Path: exe, Auth: "unknown"}
	if exe == "" {
		h.Error = "请安装 " + d.Command() + " 后重试"
		return h
	}
	var text string
	read := func(line []byte) {
		if s := strings.TrimSpace(string(line)); s != "" {
			text = s
		}
	}
	out := proc.Run(ctx, proc.Spec{Path: exe, Args: []string{"--version"}, Env: env}, proc.Options{Timeout: 5 * time.Second, OnLine: read, RedactProxy: true})
	h.Installed = out.StartErr == nil
	if out.StartErr != nil || out.ExitCode != 0 {
		h.Error = "请在终端运行 " + d.Command() + " --version 检查安装"
		return h
	}
	h.Version = text
	if len(statusArgs) > 0 {
		text = ""
		out = proc.Run(ctx, proc.Spec{Path: exe, Args: statusArgs, Env: env}, proc.Options{Timeout: 5 * time.Second, OnLine: read, RedactProxy: true})
		if out.StartErr == nil && !out.Cancelled && !out.TimedOut {
			h.Auth = "ok"
			if out.ExitCode != 0 || ClassifyText("", text) == ErrAuth {
				h.Auth = "missing"
			}
		}
	}
	return h
}

type parserState struct {
	result Result
	done   bool
	name   string
}

func (p *parserState) fail(text string) {
	p.result.ErrorKind = ClassifyText("", text)
	if p.result.ErrorKind == ErrNone {
		p.result.ErrorKind = ErrFailed
	}
}

func (p *parserState) finish(exitCode int, stderr string) Result {
	if p.result.ErrorKind == ErrNone && exitCode != 0 {
		p.result.ErrorKind = ClassifyText(stderr, "")
		if p.result.ErrorKind == ErrNone {
			p.result.ErrorKind = ErrFailed
		}
	}
	if p.result.ErrorKind == ErrNone && !p.done {
		p.result.ErrorKind = ErrProtocol
	}
	if p.result.ErrorKind != ErrNone {
		p.result.Error = errorMessage(p.name, p.result.ErrorKind)
	}
	return p.result
}
