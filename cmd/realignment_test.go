package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestR4FrozenGatewaysDefaultClosedBeforeIO(t *testing.T) {
	for _, args := range [][]string{
		{"model", "serve"}, {"model", "start"}, {"model", "client", "issue", "fixture"},
		{"agent-gateway", "serve"}, {"agent-gateway", "refresh"},
		{"agent-gateway", "client", "issue", "fixture"},
		{"agent-gateway", "codex-limited", "prepare"}, {"agent-gateway", "codex-limited", "enable"},
		{"agent-gateway", "codex-limited", "probe"}, {"agent-gateway", "app-server", "probe"},
	} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "must-not-be-created")
			c := newRootCommand()
			c.SetOut(&bytes.Buffer{})
			c.SetErr(&bytes.Buffer{})
			argv := append([]string{"--config", filepath.Join(root, "missing.yaml"), args[0], "--state-dir", root}, args[1:]...)
			c.SetArgs(argv)
			err := c.Execute()
			if err == nil || !strings.Contains(err.Error(), "experimental_gateway_disabled") {
				t.Fatal("frozen action reached configuration or execution", err)
			}
			if _, err = os.Stat(root); !os.IsNotExist(err) {
				t.Fatal("default-disabled command changed state")
			}
		})
	}
}

func TestR4CoreHelpAndFrozenRecoveryCompatibility(t *testing.T) {
	c := newRootCommand()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"--help"})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"task", "agents", "doctor", "run", "use", "resume", "runs", "jobs", "config"} {
		if !strings.Contains(out.String(), name) {
			t.Fatal("core entry missing", name)
		}
	}
	for _, name := range []string{"model", "agent-gateway"} {
		group, _, err := newRootCommand().Find([]string{name})
		if err != nil || !group.Hidden {
			t.Fatal("experiment advertised as core", name)
		}
	}
	if strings.Contains(out.String(), "experimental-gateways") || strings.Contains(out.String(), "Frozen experiment") {
		t.Fatal("root help recommends experiments")
	}
	for _, args := range [][]string{{"model", "stop"}, {"model", "status"}, {"model", "logs"}, {"model", "client", "list"}, {"model", "client", "revoke", "fixture"}, {"agent-gateway", "stop"}, {"agent-gateway", "status"}, {"agent-gateway", "client", "revoke", "fixture"}, {"agent-gateway", "codex-limited", "disable"}, {"agent-gateway", "app-server", "disable"}} {
		c := newRootCommand()
		c.SetOut(&bytes.Buffer{})
		c.SetErr(&bytes.Buffer{})
		state := filepath.Join(t.TempDir(), "state")
		c.SetArgs(append([]string{args[0], "--state-dir", state}, args[1:]...))
		err := c.Execute()
		if err != nil && strings.Contains(err.Error(), "experimental_gateway_disabled") {
			t.Fatal("recovery/inspection requires enabling service", args, err)
		}
	}
	// Opt-in only removes R4's outer gate: an absent config still fails before
	// any upstream or native process. It does not confer provider authorization.
	c = newRootCommand()
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	c.SetArgs([]string{"--experimental-gateways", "--config", filepath.Join(t.TempDir(), "missing.yaml"), "model", "serve"})
	if err := c.Execute(); err == nil || strings.Contains(err.Error(), "experimental_gateway_disabled") {
		t.Fatal("explicit opt-in contract", err)
	}
}
