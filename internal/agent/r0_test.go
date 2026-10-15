package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/config"
)

func TestNewAdapterCommandsAndResume(t *testing.T) {
	for _, name := range []string{"cursor", "antigravity"} {
		d := Definition{Name: name, Config: config.Defaults().Agents[name]}
		command, err := d.Run("two words", true, nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"--output-format", "stream-json"}
		if name == "antigravity" {
			want = append(want, "--print-timeout", "120s")
		}
		want = append(want, "--print", "two words")
		if !reflect.DeepEqual(command.Args, want) {
			t.Fatalf("%s: %+v", name, command)
		}
		resumed, err := d.Resume("session", "continue", true, nil)
		if err != nil {
			t.Fatal(err)
		}
		flag := "--resume"
		if name == "antigravity" {
			flag = "--conversation"
		}
		if resumed.Args[0] != flag || resumed.Args[1] != "session" {
			t.Fatalf("%+v", resumed)
		}
		for _, arg := range append(command.Args, resumed.Args...) {
			if arg == "--force" || arg == "--dangerously-skip-permissions" || arg == "--approve-mcps" {
				t.Fatal("added unauthorized permission bypass")
			}
		}
		interactive, err := d.Resume("session", "", false, nil)
		if err != nil || !reflect.DeepEqual(interactive.Args, []string{flag, "session"}) {
			t.Fatalf("%+v %v", interactive, err)
		}
	}
}

func TestDetectDistinguishesMissingFailedAndTimedOut(t *testing.T) {
	shell, err := os.Stat("/bin/sh")
	if err != nil || shell.IsDir() {
		t.Skip("requires POSIX shell")
	}
	missing := Definition{Config: config.AgentConfig{Command: filepath.Join(t.TempDir(), "missing")}}
	if missing.Detect(context.Background()).Installed {
		t.Fatal("missing CLI reported installed")
	}
	for _, body := range []string{"exit 9", "exec sleep 30"} {
		path := filepath.Join(t.TempDir(), "cli")
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		result := (Definition{Config: config.AgentConfig{Command: path}}).Detect(ctx)
		cancel()
		if !result.Installed || result.Error == "" || result.Version != "" {
			t.Fatalf("%+v", result)
		}
	}
}

func TestProbeCapabilitiesAreHelpEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli")
	body := "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 1.0; else echo '--print --output-format --resume'; fi\n"
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	result := (Definition{Config: config.AgentConfig{Command: path, Adapter: "cursor"}}).Probe(context.Background())
	if result.Error != "" || result.Authentication != "unknown" || result.Capabilities["headless"].Source != "help" || !result.Capabilities["resume"].Supported {
		t.Fatalf("%+v", result)
	}
	if strings.Contains(result.Capabilities["read_only"].Source, "verified") || result.Capabilities["read_only"].Supported {
		t.Fatal("claimed unverified sandbox enforcement")
	}
}
