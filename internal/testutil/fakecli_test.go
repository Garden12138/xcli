package testutil

import (
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/proc"
)

func TestFakeCLIRunsScriptInOrder(t *testing.T) {
	cfg := config.Default()
	cfg.Path = InstallFakeCLIs(t, FakeScript{"codex": {{Stdout: []string{"a"}}, {Stdout: []string{"b"}, Exit: 3}}})
	exe, err := proc.LookPath("codex", cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		text string
		exit int
	}{{"a", 0}, {"b", 3}, {"b", 3}} {
		cmd := exec.Command(exe, "exec", "--", "hello")
		raw, err := cmd.Output()
		code := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if string(raw) != tc.text+"\n" || code != tc.exit {
			t.Fatalf("output=%q code=%d", raw, code)
		}
	}
	calls := FakeCalls(t)
	if len(calls) != 3 || calls[0].Name != "codex" || !reflect.DeepEqual(calls[0].Args, []string{"exec", "--", "hello"}) || calls[0].Cwd == "" {
		t.Fatalf("calls=%+v", calls)
	}
}

func TestFakeCLIWritesPNG(t *testing.T) {
	out := t.TempDir()
	cfg := config.Default()
	cfg.Path = InstallFakeCLIs(t, FakeScript{"codex": {{WritePNG: "{{OUT}}/img.png", WriteFiles: map[string]string{"file.txt": "hello"}, Stdout: []string{"{{CWD}}"}}}})
	exe, err := proc.LookPath("codex", cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "exec", "--", "# xcli 任务\n输出目录："+out+"\n")
	cmd.Dir = out
	if raw, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(raw)) != out {
		t.Fatalf("output=%q, %v", raw, err)
	}
	file, err := os.Open(filepath.Join(out, "img.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image, err := png.DecodeConfig(file)
	if err != nil || image.Width != 8 || image.Height != 8 {
		t.Fatalf("PNG=%+v, %v", image, err)
	}
	if raw, err := os.ReadFile(filepath.Join(out, "file.txt")); err != nil || string(raw) != "hello" {
		t.Fatalf("file=%q, %v", raw, err)
	}
}

func TestFakeCLIReplaysStdoutFile(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "fixture.jsonl")
	text := "{\"cwd\":\"{{CWD}}\"}\n"
	if err := os.WriteFile(fixture, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Path = InstallFakeCLIs(t, FakeScript{"cursor-agent": {{StdoutFile: fixture, Stderr: "example error", Exit: 4}}})
	exe, err := proc.LookPath("cursor-agent", cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-p", "hello")
	cmd.Dir = dir
	raw, err := cmd.Output()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 4 || string(exit.Stderr) != "example error" || string(raw) != strings.ReplaceAll(text, "{{CWD}}", dir) {
		t.Fatalf("replay: output=%q, error=%v", raw, err)
	}
}

func TestFakeCLICountsConcurrentCallsAndSkipsProbes(t *testing.T) {
	cfg := config.Default()
	cfg.Path = InstallFakeCLIs(t, FakeScript{"codex": {{Stdout: []string{"a"}}, {Stdout: []string{"b"}}, {Stdout: []string{"c"}}}})
	exe, err := proc.LookPath("codex", cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--version"}, {"login", "status"}} {
		if _, err := exec.Command(exe, args...).Output(); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	outputs := make(chan string, 3)
	for range 3 {
		wg.Go(func() {
			data, err := exec.Command(exe, "exec", "hello").Output()
			if err != nil {
				t.Error(err)
			}
			outputs <- string(data)
		})
	}
	wg.Wait()
	close(outputs)
	seen := map[string]bool{}
	for out := range outputs {
		seen[out] = true
	}
	if len(seen) != 3 || len(FakeCalls(t)) != 3 {
		t.Fatalf("concurrent outputs=%v calls=%+v", seen, FakeCalls(t))
	}
}
