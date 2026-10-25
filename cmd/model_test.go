package cmd

import (
	"bytes"
	"github.com/Garden12138/xcli/internal/modelstore"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelClientCLIAndConfigurationIsolation(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "private")
	if err := modelstore.PrivateDirectory(private); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "gateway.yaml")
	config := `version: 1
backends:
  main:
    type: openai-compatible
    endpoint: http://127.0.0.1:11434/v1
    inference_location: local
models:
  xcli/text-default:
    backend: main
    upstream_model: fixture
    kind: text
    capabilities: [text.generate]
`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(private, "state")
	tokenFile := filepath.Join(private, "token")
	run := func(args ...string) (string, error) {
		command := newRootCommand()
		var out bytes.Buffer
		command.SetOut(&out)
		command.SetErr(&out)
		command.SetArgs(append([]string{"--config", path, "model", "--state-dir", state}, args...))
		err := command.Execute()
		return out.String(), err
	}
	out, err := run("client", "issue", "worklog", "--model", "xcli/text-default", "--token-file", tokenFile)
	if err != nil {
		t.Fatal(out, err)
	}
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, strings.TrimSpace(string(token))) {
		t.Fatal("secret printed")
	}
	out, err = run("client", "list")
	if err != nil || !strings.Contains(out, "worklog") || strings.Contains(out, "token_hash") {
		t.Fatal(out, err)
	}
	if _, err = run("client", "issue", "worklog", "--model", "xcli/text-default", "--token-file", tokenFile); err == nil {
		t.Fatal("existing token overwritten")
	}
	if out, err = run("doctor", "--no-billable-requests"); err != nil || !strings.Contains(out, `"billable_requests": 0`) {
		t.Fatal(out, err)
	}
	if out, err = run("client", "revoke", "worklog"); err != nil {
		t.Fatal(out, err)
	}
	for _, entry := range []string{"task", "run", "use", "resume"} {
		if _, _, err := newRootCommand().Find([]string{entry}); err != nil {
			t.Fatal(entry, err)
		}
	}
}
