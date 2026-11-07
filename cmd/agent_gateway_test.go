package cmd

import (
	"bytes"
	"github.com/Garden12138/xcli/internal/agentstore"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentGatewayClientCLIIsolation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	tokenFile := filepath.Join(t.TempDir(), "private", "token")
	run := func(args ...string) (string, error) {
		c := newRootCommand()
		var b bytes.Buffer
		c.SetOut(&b)
		c.SetErr(&b)
		c.SetArgs(append([]string{"agent-gateway", "--state-dir", root}, args...))
		e := c.Execute()
		return b.String(), e
	}
	out, e := run("client", "issue", "worklog", "--agent", "codex", "--policy", "synthetic-text-readonly-v1", "--token-file", tokenFile)
	if e != nil {
		t.Fatal(e)
	}
	token, _ := os.ReadFile(tokenFile)
	if strings.Contains(out, strings.TrimSpace(string(token))) || !strings.HasPrefix(string(token), "nag1_") {
		t.Fatal(out)
	}
	cs := &agentstore.Clients{Root: root}
	if _, e = cs.Authenticate(strings.TrimSpace(string(token))); e != nil {
		t.Fatal(e)
	}
	if _, e = run("client", "revoke", "worklog"); e != nil {
		t.Fatal(e)
	}
	if _, e = cs.Authenticate(strings.TrimSpace(string(token))); e == nil {
		t.Fatal("revoked token works")
	}
	if _, e = os.Stat(filepath.Join(root, "../model-gateway/clients.json")); e == nil {
		t.Fatal("changed model clients")
	}
}
