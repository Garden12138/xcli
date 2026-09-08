//go:build darwin || linux

// phase0 is an opt-in black-box characterization runner, not a regression test
// that requires defects to remain present. See docs/phase0/validation.zh-CN.md.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type observation struct {
	Args     []string `json:"args"`
	Cwd      string   `json:"cwd"`
	ExitCode int      `json:"exit_code"`
	Stdout   string   `json:"stdout"`
	Stderr   string   `json:"stderr,omitempty"`
}

type finding struct {
	ID       string        `json:"id"`
	Status   string        `json:"status"`
	Detail   string        `json:"detail"`
	Commands []observation `json:"commands"`
}

type suite struct {
	Line         string    `json:"line"`
	Commit       string    `json:"commit"`
	BinarySHA256 string    `json:"binary_sha256"`
	Findings     []finding `json:"findings"`
}

type report struct {
	GeneratedAt string  `json:"generated_at"`
	Platform    string  `json:"platform"`
	GoVersion   string  `json:"go_version"`
	Kind        string  `json:"kind"`
	Suites      []suite `json:"suites"`
}

type fixture struct {
	root, repo, binary, self, config string
	env                              []string
	commands                         []observation
}

func main() {
	if strings.HasPrefix(filepath.Base(os.Args[0]), "fake-agent-") {
		fake(strings.TrimPrefix(filepath.Base(os.Args[0]), "fake-agent-"))
		return
	}
	if len(os.Args) > 3 && os.Args[1] == "--native" {
		nativeExec(os.Args[2], os.Args[3], os.Args[4:])
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--fake" {
		fake(os.Args[2])
		return
	}
	stable := flag.String("stable", "", "stable xcli binary (required)")
	development := flag.String("development", "", "optional development xcli binary")
	source := flag.String("source", ".", "stable source tree, for original examples and commit")
	devSource := flag.String("development-source", "", "development source tree")
	stableCommit := flag.String("stable-commit", "", "commit used to build stable binary (required)")
	devCommit := flag.String("development-commit", "", "commit used to build development binary")
	output := flag.String("out", "", "write sanitized JSON report; default stdout")
	realCodex := flag.String("real-codex", "", "opt in to real smoke using this Codex binary; maximum two model invocations, requires a fresh -out")
	flag.Parse()
	if *stable == "" || *stableCommit == "" || (*development != "" && (*devSource == "" || *devCommit == "")) {
		flag.Usage()
		os.Exit(2)
	}
	if *realCodex != "" {
		if err := realSmoke(*stable, *stableCommit, *realCodex, *output); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	r := report{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), Kind: "simulated"}
	hadError := false
	for _, input := range []struct{ line, binary, source, commit string }{
		{"stable", *stable, *source, *stableCommit}, {"development", *development, *devSource, *devCommit},
	} {
		if input.binary == "" {
			continue
		}
		binary, err := filepath.Abs(input.binary)
		must(err)
		src, err := filepath.Abs(input.source)
		must(err)
		data, err := os.ReadFile(binary)
		must(err)
		sum := sha256.Sum256(data)
		s := suite{Line: input.line, Commit: input.commit, BinarySHA256: hex.EncodeToString(sum[:])}
		cases := []struct {
			id  string
			run func(*fixture) (bool, bool, string)
		}{
			{"F02", mcpDrift}, {"F03", importEnv}, {"F04", resumeCwd},
			{"F05", largeEvent}, {"F06", func(f *fixture) (bool, bool, string) { return exampleCwd(f, src) }}, {"F07", doctor},
		}
		if input.line == "development" {
			cases = append(cases, struct {
				id  string
				run func(*fixture) (bool, bool, string)
			}{"F01", worktreeCwd})
		}
		for _, c := range cases {
			f, err := newFixture(binary)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			v := runCase(f, c.id, c.run)
			s.Findings = append(s.Findings, v)
			if v.Status == "execution_error" {
				hadError = true
			}
			must(os.RemoveAll(f.root)) // only the directory created by newFixture
		}
		r.Suites = append(r.Suites, s)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	must(err)
	data = append(data, '\n')
	if *output == "" {
		_, err = os.Stdout.Write(data)
	} else {
		err = os.WriteFile(*output, data, 0o600)
	}
	must(err)
	if hadError {
		os.Exit(2)
	}
}

func runCase(f *fixture, id string, run func(*fixture) (bool, bool, string)) (v finding) {
	v.ID = id
	defer func() {
		if p := recover(); p != nil {
			v.Status = "execution_error"
			v.Detail = fmt.Sprint(p)
		}
		v.Commands = f.commands
		v.Detail = f.clean(v.Detail)
	}()
	broken, fixed, detail := run(f)
	v.Detail = detail
	v.Status = "execution_error"
	if broken && !fixed {
		v.Status = "reproduced"
	}
	if fixed && !broken {
		v.Status = "meets_fix_expectation"
	}
	return
}

func newFixture(binary string) (*fixture, error) {
	root, err := os.MkdirTemp("", "xcli-phase0-case-")
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	f := &fixture{root: root, repo: filepath.Join(root, "repo"), binary: binary, self: self}
	f.config = filepath.Join(f.repo, ".xcli", "config.yaml")
	// Do not inherit credentials, Agent settings or user Git configuration. No
	// native CLI is launched by simulated cases; each command points to this runner.
	f.env = []string{"PATH=" + filepath.Join(root, "bin") + ":/usr/bin:/bin", "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_DATA_HOME=" + filepath.Join(root, "data"), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "LC_ALL=C"}
	for _, dir := range []string{f.repo, filepath.Join(f.repo, "sub"), filepath.Dir(f.config)} {
		must(os.MkdirAll(dir, 0700))
	}
	must(os.MkdirAll(filepath.Join(root, "bin"), 0700))
	must(os.Symlink(binary, filepath.Join(root, "bin", "xcli")))
	f.configure("cwd", nil)
	return f, nil
}

func (f *fixture) configure(mode string, servers any) {
	command := filepath.Join(f.root, "bin", "fake-agent-"+mode)
	if _, err := os.Lstat(command); os.IsNotExist(err) {
		must(os.Symlink(f.self, command))
	}
	agents := map[string]any{}
	for _, name := range []string{"codex", "claude", "gemini", "opencode"} {
		agents[name] = map[string]any{"adapter": name, "command": command, "args": []string{"--fake", mode}}
	}
	cfg := map[string]any{"version": 1, "agents": agents}
	if servers != nil {
		cfg["mcp"] = map[string]any{"servers": servers}
	}
	data, err := yaml.Marshal(cfg)
	must(err)
	write(f.config, data)
}

func (f *fixture) clean(s string) string {
	s = strings.ReplaceAll(s, f.root, "<fixture>")
	s = strings.ReplaceAll(s, f.binary, "<xcli>")
	s = strings.ReplaceAll(s, f.self, "<fake-agent>")
	return s
}

func (f *fixture) run(cwd string, args ...string) (observation, map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, f.binary, append([]string{"--config", f.config}, args...)...)
	command.Dir, command.Env = cwd, f.env
	command.Cancel = func() error { return command.Process.Signal(os.Interrupt) }
	command.WaitDelay = 5 * time.Second
	var out, stderr bytes.Buffer
	command.Stdout, command.Stderr = &out, &stderr
	err := command.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ctx.Err() != nil {
			panic(fmt.Sprintf("command could not finish: %v", err))
		}
		code = ee.ExitCode()
	}
	o := observation{Args: append([]string{"--config", f.config}, args...), Cwd: cwd, ExitCode: code, Stdout: out.String(), Stderr: stderr.String()}
	safe := o
	safe.Args = append([]string(nil), o.Args...)
	for i := range safe.Args {
		safe.Args[i] = f.clean(safe.Args[i])
	}
	safe.Cwd, safe.Stdout, safe.Stderr = f.clean(safe.Cwd), f.clean(safe.Stdout), f.clean(safe.Stderr)
	// Large success outputs remain verifiable without putting 5 MiB in a report.
	if len(safe.Stdout) > 8192 {
		sum := sha256.Sum256([]byte(safe.Stdout))
		safe.Stdout = fmt.Sprintf("<%d bytes sha256:%x>", len(safe.Stdout), sum)
	}
	f.commands = append(f.commands, safe)
	var value map[string]any
	_ = json.Unmarshal(out.Bytes(), &value)
	return o, value
}

func write(path string, data []byte) {
	must(os.MkdirAll(filepath.Dir(path), 0700))
	must(os.WriteFile(path, data, 0600))
}
func jsonFile(path string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	must(err)
	write(path, append(data, '\n'))
}
func readObject(path string) map[string]any {
	data, err := os.ReadFile(path)
	must(err)
	var value map[string]any
	must(json.Unmarshal(data, &value))
	return value
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func require(ok bool, message string) {
	if !ok {
		panic(message)
	}
}

func fake(mode string) {
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			if mode == "version-fail" {
				os.Exit(9)
			}
			fmt.Println("phase0-fake 1.0")
			return
		}
	}
	if mode == "version-fail" {
		os.Exit(9)
	}
	cwd, err := os.Getwd()
	must(err)
	if mode == "marker" {
		write(filepath.Join(cwd, "escape-marker"), []byte("fixture\n"))
	}
	encoder := json.NewEncoder(os.Stdout)
	must(encoder.Encode(map[string]any{"type": "thread.started", "thread_id": "phase0-session"}))
	output := cwd
	if mode == "large" {
		output = strings.Repeat("x", 5*1024*1024)
	}
	must(encoder.Encode(map[string]any{"type": "item.completed", "item": map[string]any{"type": "agent_message", "text": output}}))
	must(encoder.Encode(map[string]any{"type": "turn.completed", "usage": map[string]any{"input_tokens": 7, "output_tokens": 3, "total_tokens": 10}}))
}
