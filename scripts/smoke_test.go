package scripts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/testutil"
)

var smokeBinary struct {
	sync.Once
	path string
	err  error
}

func codexReply(text string) []string {
	raw, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]string{"type": "agent_message", "text": text}})
	return []string{`{"type":"thread.started","thread_id":"smoke-codex"}`, string(raw), `{"type":"turn.completed"}`}
}

func cursorReply(text string) []string {
	raw, _ := json.Marshal(map[string]any{"type": "result", "subtype": "success", "session_id": "smoke-cursor", "result": text})
	return []string{string(raw)}
}

func agyReply(text string) []string {
	raw, _ := json.Marshal(map[string]any{"event": "result", "result": map[string]any{"status": "SUCCESS", "conversation_id": "smoke-agy", "response": text}})
	return []string{string(raw)}
}

func smokeSetup(t *testing.T, failAsk bool) string {
	t.Helper()
	// Read the script first so tests fail before implementation is added.
	script, err := os.ReadFile("smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(file))
	smokeBinary.Do(func() {
		var dir string
		dir, smokeBinary.err = os.MkdirTemp("", "xcli-smoke-build-")
		if smokeBinary.err != nil {
			return
		}
		smokeBinary.path = filepath.Join(dir, "xcli")
		cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", smokeBinary.path, ".")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			smokeBinary.err = fmt.Errorf("build xcli: %w: %s", err, out)
		}
	})
	if smokeBinary.err != nil {
		t.Fatal(smokeBinary.err)
	}
	plan := `{"reasoning":"并行文案与配图，再写入 README","steps":[{"id":"copy","kind":"ask","agent":"cursor","instruction":"写介绍","depends_on":[],"review_by":null,"output":null},{"id":"cover","kind":"image","agent":"antigravity","instruction":"生成封面","depends_on":[],"review_by":null,"output":"cover"},{"id":"apply","kind":"code","agent":"codex","instruction":"写 README 并引用封面","depends_on":["copy","cover"],"review_by":"auto","output":null}]}`
	pass := `{"verdict":"pass","summary":"通过","issues":[]}`
	fake := testutil.FakeScript{
		"codex": {
			{Stdout: codexReply("add.go 提供 Add 函数")},
			{Stdout: codexReply(`{"functions":["Add"]}`)},
			{Stdout: codexReply("添加表驱动测试"), WriteFiles: map[string]string{"add_test.go": "package demo\nimport \"testing\"\nfunc TestAdd(t *testing.T) { for _, c := range []struct{a,b,want int}{{1,2,3},{-1,1,0}} { if got:=Add(c.a,c.b);got!=c.want {t.Fatal(got)} } }\n"}},
			{Stdout: codexReply(plan)},
			{Stdout: codexReply("更新 README"), WriteFiles: map[string]string{"README.md": "# demo\n\nAdd 示例项目。\n\n![封面](docs/cover.png)\n"}, WritePNG: "docs/cover.png"},
		},
		"cursor-agent": {{Stdout: cursorReply("Add 返回两数之和")}, {Stdout: cursorReply(pass)}, {Stdout: cursorReply("Add 示例项目。")}, {Stdout: cursorReply(pass)}},
		"agy":          {{Stdout: agyReply("Add 返回两数之和")}, {Stdout: agyReply(`{"files":[]}`), WritePNG: "{{OUT}}/image.png"}, {Stdout: agyReply(`{"files":[]}`), WritePNG: "{{OUT}}/cover.png"}},
	}
	if failAsk {
		fake["codex"][0] = testutil.FakeStep{Stdout: []string{`{"type":"turn.failed","error":{"message":"scripted failure"}}`}, Exit: 1}
	}
	cfg := config.Default()
	cfg.Defaults.InheritShellProxy = false
	cfg.Path = testutil.InstallFakeCLIs(t, fake) + ":" + filepath.Join(runtime.GOROOT(), "bin")
	for name, agent := range cfg.Agents {
		agent.ArtifactDirs = []string{}
		cfg.Agents[name] = agent
	}
	home := t.TempDir()
	raw, err := config.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XCLI_HOME", home)
	t.Setenv("TMPDIR", t.TempDir())
	dir := filepath.Join(t.TempDir(), "smoke with spaces")
	for _, path := range []string{"scripts", "bin"} {
		if err := os.MkdirAll(filepath.Join(dir, path), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "smoke.sh"), script, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(smokeBinary.path, filepath.Join(dir, "bin", "xcli")); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "scripts", "smoke.sh")
}

func runSmoke(t *testing.T, script, input string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("smoke timeout: %s", out)
	}
	return string(out), err
}

func TestSmokeSuccess(t *testing.T) {
	out, err := runSmoke(t, smokeSetup(t, false), "\n")
	if err != nil {
		t.Fatalf("smoke: %v\n%s", err, out)
	}
	for _, name := range []string{"agents", "ask-codex", "ask-cursor", "ask-antigravity", "schema", "image", "code", "do"} {
		if !strings.Contains(out, "PASS "+name+" task=") {
			t.Fatalf("missing result for %s:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "汇总：8 PASS / 0 FAIL") {
		t.Fatal(out)
	}
	if calls := testutil.FakeCalls(t); len(calls) != 12 {
		t.Fatalf("fake calls=%d want=12\n%s", len(calls), out)
	}
}

func TestSmokeFailureStillSummarizes(t *testing.T) {
	out, err := runSmoke(t, smokeSetup(t, true), "\n")
	if err == nil || !strings.Contains(out, "FAIL ask-codex task=t-") || !strings.Contains(out, "PASS do task=t-") || !strings.Contains(out, "汇总：7 PASS / 1 FAIL") {
		t.Fatalf("failure handling: %v\n%s", err, out)
	}
}

func TestSmokeEOFDoesNotCallCLI(t *testing.T) {
	out, err := runSmoke(t, smokeSetup(t, false), "")
	if err == nil || len(testutil.FakeCalls(t)) != 0 || !strings.Contains(out, "按回车继续") {
		t.Fatalf("EOF: %v\n%s", err, out)
	}
}
