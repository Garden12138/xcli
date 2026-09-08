//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// Replace the wrapper with Codex so xcli still owns its native process. Capture
// only cwd as independent evidence; do not log native args or account settings.
func nativeExec(binary, capture string, args []string) {
	cwd, err := os.Getwd()
	must(err)
	f, err := os.OpenFile(capture, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	must(err)
	must(json.NewEncoder(f).Encode(map[string]string{"cwd": cwd}))
	must(f.Close())
	must(syscall.Exec(binary, append([]string{binary}, args...), os.Environ()))
}

type smokeCall struct {
	Name           string   `json:"name"`
	Command        []string `json:"command"`
	ExitCode       int      `json:"exit_code"`
	TimedOut       bool     `json:"timed_out"`
	DurationMS     int64    `json:"duration_ms"`
	Status         string   `json:"status"`
	Output         string   `json:"output"`
	SessionPresent bool     `json:"session_present"`
	Usage          any      `json:"usage,omitempty"`
	Diagnostic     string   `json:"diagnostic,omitempty"`
}

type smokeReport struct {
	GeneratedAt      string      `json:"generated_at"`
	Kind             string      `json:"kind"`
	Platform         string      `json:"platform"`
	Commit           string      `json:"commit"`
	BinarySHA256     string      `json:"binary_sha256"`
	CodexVersion     string      `json:"codex_version"`
	Account          string      `json:"account"`
	Model            string      `json:"model"`
	InvocationLimit  int         `json:"invocation_limit"`
	Invocations      int         `json:"invocations"`
	Status           string      `json:"status"`
	Detail           string      `json:"detail"`
	Calls            []smokeCall `json:"calls"`
	SameSession      bool        `json:"same_session"`
	NativeCwdCorrect bool        `json:"native_cwd_correct"`
	RecordsLinked    bool        `json:"records_linked"`
}

func realSmoke(binary, commit, codex, output string) (resultErr error) {
	if output == "" {
		return errors.New("real smoke requires a fresh -out report path")
	}
	// Reserve the evidence path before any invocation. An accidental repeat with
	// the same path must not spend another two calls or overwrite earlier evidence.
	reportFile, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer reportFile.Close()
	r := smokeReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Kind: "real", Platform: runtime.GOOS + "/" + runtime.GOARCH, Commit: commit, Account: "existing local login (identity omitted)", Model: "existing CLI default; no model override", InvocationLimit: 2, Status: "blocked"}
	defer func() {
		if err := json.NewEncoder(reportFile).Encode(r); err != nil {
			resultErr = fmt.Errorf("write smoke evidence: %w", err)
		} else if resultErr == nil && r.Status != "passed" {
			resultErr = fmt.Errorf("real smoke %s: %s", r.Status, r.Detail)
		}
	}()
	binary, err = filepath.Abs(binary)
	if err != nil {
		return err
	}
	codex, err = filepath.Abs(codex)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return err
	}
	r.BinarySHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	version, err := exec.CommandContext(ctx, codex, "--version").Output()
	cancel()
	if err != nil {
		r.Detail = "Codex version probe failed; no model call"
		return nil
	}
	r.CodexVersion = strings.TrimSpace(string(version))
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	loginErr := exec.CommandContext(ctx, codex, "login", "status").Run()
	cancel()
	if loginErr != nil {
		r.Detail = "existing Codex login unavailable; no model call; no login attempted"
		return nil
	}
	f, err := newFixture(binary)
	if err != nil {
		return err
	}
	defer os.RemoveAll(f.root)
	// Inherit the existing account and model. Only xcli's data location is isolated;
	// HOME and CODEX_HOME are neither changed nor copied.
	f.env = nil
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "XDG_DATA_HOME=") {
			f.env = append(f.env, entry)
		}
	}
	f.env = append(f.env, "XDG_DATA_HOME="+filepath.Join(f.root, "data"))
	capture := filepath.Join(f.root, "invocations.jsonl")
	cfg := map[string]any{"version": 1, "agents": map[string]any{"codex": map[string]any{"adapter": "codex", "command": f.self, "args": []string{"--native", codex, capture, "-c", "sandbox_mode=\"read-only\"", "-c", "approval_policy=\"never\""}}}}
	data, err = yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	write(f.config, data)
	write(filepath.Join(f.repo, "marker.txt"), []byte("phase0-fixture\n"))
	for _, args := range [][]string{{"init", "-q"}, {"add", "marker.txt"}, {"-c", "user.name=Phase0 Fixture", "-c", "user.email=phase0@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}} {
		c := exec.Command("git", args...)
		c.Dir = f.repo
		c.Env = append(f.env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		if err := c.Run(); err != nil {
			r.Detail = "temporary Git repository setup failed"
			return err
		}
	}
	call := func(name string, args ...string) map[string]any {
		r.Invocations++
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, binary, append([]string{"--config", f.config}, args...)...)
		c.Dir, c.Env = f.root, f.env
		c.Cancel = func() error { return c.Process.Signal(os.Interrupt) }
		c.WaitDelay = 10 * time.Second
		var stdout, stderr bytes.Buffer
		c.Stdout, c.Stderr = &stdout, &stderr
		start := time.Now()
		err := c.Run()
		s := smokeCall{Name: name, Command: append([]string{"<xcli>", "--config", "<fixture>/repo/.xcli/config.yaml"}, args...), DurationMS: time.Since(start).Milliseconds(), TimedOut: ctx.Err() != nil}
		if err != nil {
			s.ExitCode = 1
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				s.ExitCode = ee.ExitCode()
			}
		}
		var result map[string]any
		if json.Unmarshal(stdout.Bytes(), &result) != nil {
			s.Diagnostic = "no normalized JSON result"
		}
		s.Status, _ = result["status"].(string)
		s.SessionPresent = result["session_id"] != nil && result["session_id"] != ""
		s.Usage = result["usage"]
		// Only publish responses in the fixture's tiny allowlist. Do not publish
		// arbitrary model output or native stderr which can contain account details.
		answer, _ := result["output"].(string)
		if answer == "PHASE0_READY" || answer == "PHASE0_MEMORY_7Q2" {
			s.Output = answer
		} else {
			s.Output = "<unexpected response omitted>"
		}
		if err != nil && s.Diagnostic == "" {
			s.Diagnostic = "native call failed; raw stderr omitted"
		}
		// Classify only known diagnostics; never publish arbitrary native output.
		if strings.Contains(stderr.String(), "usage_limit_exceeded") || strings.Contains(stderr.String(), "You've hit your usage limit") || strings.Contains(stdout.String(), "usage_limit_exceeded") {
			s.Diagnostic = "usage_limit_exceeded"
		}
		for i := range s.Command {
			s.Command[i] = f.clean(s.Command[i])
		}
		r.Calls = append(r.Calls, s)
		return result
	}
	first := call("run-json", "run", "codex", "Remember the exact token PHASE0_MEMORY_7Q2 for the next turn. Do not use tools or modify files. Reply exactly PHASE0_READY.", "--cwd", f.repo, "--json")
	if r.Calls[0].ExitCode != 0 || first["status"] != "success" || first["output"] != "PHASE0_READY" || !r.Calls[0].SessionPresent {
		r.Status = "failed"
		r.Detail = "first call failed acceptance; resume omitted; no retries"
		return nil
	}
	paths, err := filepath.Glob(filepath.Join(f.root, "data", "xcli", "runs", "*.json"))
	if err != nil {
		return err
	}
	if len(paths) != 1 {
		r.Status = "failed"
		r.Detail = "first run did not save exactly one record"
		return nil
	}
	original := readObject(paths[0])
	id, _ := original["id"].(string)
	if id == "" {
		r.Status = "failed"
		r.Detail = "first record lacks id"
		return nil
	}
	second := call("resume-json", "resume", id, "Without tools, repeat exactly the token I asked you to remember in the previous turn. Do not modify files.", "--json")
	// A run ID is private local metadata; keep linkage booleans, not its value.
	for i := range r.Calls[1].Command {
		if r.Calls[1].Command[i] == id {
			r.Calls[1].Command[i] = "<run-id>"
		}
	}
	r.SameSession = second["session_id"] == first["session_id"]
	invocations, err := os.ReadFile(capture)
	if err != nil {
		return err
	}
	lines := bytes.Split(bytes.TrimSpace(invocations), []byte("\n"))
	r.NativeCwdCorrect = len(lines) == 2
	for _, line := range lines {
		var v map[string]string
		if json.Unmarshal(line, &v) != nil || v["cwd"] != f.repo {
			r.NativeCwdCorrect = false
		}
	}
	paths, err = filepath.Glob(filepath.Join(f.root, "data", "xcli", "runs", "*.json"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		v := readObject(path)
		if v["resumed_from"] == id && v["cwd"] == f.repo && v["session_id"] == first["session_id"] {
			r.RecordsLinked = true
		}
	}
	if r.Calls[1].ExitCode == 0 && second["status"] == "success" && second["output"] == "PHASE0_MEMORY_7Q2" && r.SameSession && r.NativeCwdCorrect && r.RecordsLinked {
		r.Status = "passed"
		r.Detail = "run/JSON and same-session resume passed; cwd captured independently at native exec; no project files requested to change"
	} else {
		r.Status = "failed"
		r.Detail = "resume acceptance failed; no retries"
	}
	return nil
}
