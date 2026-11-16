package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

type FakeStep struct {
	StdoutFile string            `json:"stdout_file,omitempty"` // 回放的样本文件（绝对路径）
	Stdout     []string          `json:"stdout,omitempty"`      // 直接给出的 stdout 行
	Stderr     string            `json:"stderr,omitempty"`
	Exit       int               `json:"exit"`
	SleepMS    int               `json:"sleep_ms,omitempty"`
	WriteFiles map[string]string `json:"write_files,omitempty"` // 相对 cwd 或绝对路径 → 内容
	WritePNG   string            `json:"write_png,omitempty"`   // 在该路径写一张 8x8 PNG；"{{OUT}}/x.png" 中 {{OUT}} 替换为提示词里"输出目录："后的路径
}

type FakeScript map[string][]FakeStep // 键为 "codex" | "cursor-agent" | "agy"

type FakeCall struct {
	Name string   `json:"name"`
	Args []string `json:"args"`
	Cwd  string   `json:"cwd"`
}

var fakeBuild struct {
	sync.Once
	path string
	err  error
}

func InstallFakeCLIs(t *testing.T, script FakeScript) string {
	t.Helper()
	fakeBuild.Do(func() {
		var dir string
		dir, fakeBuild.err = os.MkdirTemp("", "xcli-fakecli-build-")
		if fakeBuild.err != nil {
			return
		}
		fakeBuild.path = filepath.Join(dir, "fakecli")
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", fakeBuild.path, "./internal/testutil/fakecli")
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			fakeBuild.err = fmt.Errorf("编译 fakecli：%w：%s", err, output)
		}
	})
	if fakeBuild.err != nil {
		t.Fatal(fakeBuild.err)
	}
	dir := t.TempDir()
	binary, err := os.ReadFile(fakeBuild.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codex", "cursor-agent", "agy"} {
		if err := os.WriteFile(filepath.Join(dir, name), binary, 0700); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(script)
	if err != nil {
		t.Fatal(err)
	}
	scriptFile := filepath.Join(dir, "script.json")
	if err := os.WriteFile(scriptFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XCLI_FAKE_SCRIPT", scriptFile)
	t.Setenv("XCLI_FAKE_STATE", filepath.Join(dir, "state"))
	t.Setenv("XCLI_FAKE_LOG", filepath.Join(dir, "calls.jsonl"))
	return dir + ":/bin:/usr/bin"
}

func FakeCalls(t *testing.T) []FakeCall {
	t.Helper()
	raw, err := os.ReadFile(os.Getenv("XCLI_FAKE_LOG"))
	if os.IsNotExist(err) {
		return []FakeCall{}
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls []FakeCall
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var call FakeCall
		if err := json.Unmarshal(line, &call); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, call)
	}
	return calls
}
