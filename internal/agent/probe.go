package agent

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Probe is read-only. Help confirms exposed flags, not login, quotas, sandbox
// enforcement or successful model execution.
func (d Definition) Probe(ctx context.Context) Installation {
	result := d.Detect(ctx)
	if !result.Installed || result.Error != "" || d.Config.Adapter == "generic" {
		return result
	}
	helpCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	args := append([]string{}, d.Config.Args...)
	args = append(args, "--help")
	command := exec.CommandContext(helpCtx, result.Path, args...)
	command.WaitDelay = 250 * time.Millisecond
	var output probeOutput
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if err != nil {
		result.Error = "capability help check failed"
		return result
	}
	if output.truncated {
		result.Error = "help output exceeds 64 KiB"
		return result
	}
	help := string(output.data)
	flags := map[string]string{"headless": "--print", "resume": "--resume", "structured_output": "--output-format"}
	switch d.Config.Adapter {
	case "codex":
		flags = map[string]string{"headless": "exec", "resume": "resume"}
	case "antigravity":
		flags["resume"] = "--conversation"
	case "opencode":
		flags = map[string]string{"headless": "run", "resume": "--session"}
	}
	for name, flag := range flags {
		result.Capabilities[name] = Capability{Supported: strings.Contains(help, flag), Source: "help"}
	}
	return result
}

type probeOutput struct {
	data      []byte
	truncated bool
}

func (o *probeOutput) Write(data []byte) (int, error) {
	n := len(data)
	available := (64 << 10) - len(o.data)
	if len(data) > available {
		data = data[:available]
		o.truncated = true
	}
	o.data = append(o.data, data...)
	return n, nil
}
