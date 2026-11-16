//go:build unix

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const probeSchema = `{"type":"object","additionalProperties":false,"required":["answer"],"properties":{"answer":{"type":"string"}}}`

var probeCases = []string{"ask_ok", "schema_ok", "resume_ok", "write_ok", "image_ok", "error_bad_model"}

func main() {
	agent := flag.String("agent", "codex", "实测 CLI：codex/cursor/antigravity")
	out := flag.String("out", "internal/driver/testdata", "样本目录")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := probe(ctx, *agent, *out, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func probe(ctx context.Context, agent, dest string, log io.Writer) error {
	command := map[string]string{"codex": "codex", "cursor": "cursor-agent", "antigravity": "agy"}[agent]
	if command == "" {
		return fmt.Errorf("请选择 -agent codex/cursor/antigravity")
	}
	exe, err := exec.LookPath(command)
	if err != nil {
		fmt.Fprintf(log, "%s 尚未安装，请安装并登录后重新实测。\n", command)
		return nil
	}
	tmp, err := os.MkdirTemp("", "xcli-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	home, _ := os.UserHomeDir()
	repo, out := filepath.Join(tmp, "repo"), filepath.Join(tmp, "out")
	dest, err = filepath.Abs(filepath.Join(dest, agent))
	if err != nil {
		return err
	}
	for _, dir := range []string{repo, out, dest} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	schema := filepath.Join(tmp, "schema.json")
	for path, content := range map[string]string{
		filepath.Join(repo, "add.go"):           "package demo\n\nfunc Add(a, b int) int { return a + b }\n",
		schema:                                  probeSchema,
		filepath.Join(tmp, "image-schema.json"): `{"type":"object","additionalProperties":false,"required":["files"],"properties":{"files":{"type":"array","items":{"type":"string"}}}}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return err
		}
	}
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "add.go"},
		{"-c", "user.name=xcli-probe", "-c", "user.email=probe@localhost", "-c", "commit.gpgsign=false", "commit", "-qm", "init"},
	} {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
		if data, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("请检查 git 是否可用后重试：%w：%s", err, sanitize(data, tmp, home))
		}
	}
	var sid string
	for _, name := range probeCases {
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "resume_ok" && sid == "" {
			fmt.Fprintln(log, "ask_ok 未返回会话 ID，请检查样本后重新实测续接。")
			continue
		}
		caseCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		stdout, _, err := recordCase(caseCtx, exe, probeArgs(agent, name, repo, out, schema, sid), repo, tmp, home, dest, name, log)
		cancel()
		if err != nil {
			fmt.Fprintf(log, "%s：请检查样本日志后重试：%v\n", name, err)
		}
		if name == "ask_ok" {
			sid = sessionID(agent, stdout)
		}
		if name == "write_ok" {
			_, statErr := os.Stat(filepath.Join(repo, "hello.txt"))
			fmt.Fprintf(log, "write_ok hello.txt=%t\n", statErr == nil)
		}
		if name == "image_ok" {
			for _, root := range []string{out, filepath.Join(home, ".codex"), filepath.Join(home, ".gemini"), filepath.Join(home, ".cursor")} {
				for _, path := range recentImages(root, time.Now().Add(-10*time.Minute)) {
					fmt.Fprintf(log, "图片：%s\n", sanitize([]byte(path), tmp, home))
				}
			}
		}
	}
	return ctx.Err()
}

func probeArgs(agent, name, repo, out, schema, sid string) []string {
	prompt := "用一句话说明 add.go 中 Add 函数的作用。"
	write := name == "write_ok" || name == "image_ok"
	switch name {
	case "schema_ok":
		prompt = "用一句话说明 add.go 的作用，按 JSON Schema 输出。"
		if agent == "cursor" {
			prompt += "最终只输出符合以下 JSON Schema 的 JSON：" + probeSchema
		}
	case "resume_ok":
		prompt = "我上一个问题问的是哪个函数？只回答函数名。"
	case "write_ok":
		prompt = "在当前目录创建 hello.txt，内容为 hi，不要做其他修改。"
	case "image_ok":
		hint := map[string]string{"codex": "使用内置 image_gen 工具", "cursor": "使用内置 GenerateImage 工具", "antigravity": "调用内置 image-generator 子代理（通过子代理调用）"}[agent]
		prompt = fmt.Sprintf("%s，生成一张米白背景上的绿色植物插画，方形，无文字。保存到 %s/image_ok-1.<ext>，最终只回复 {\"files\":[\"<绝对路径>\"]}。", hint, out)
	}
	var args []string
	switch agent {
	case "codex":
		args = []string{"exec"}
		if name == "resume_ok" {
			args = append(args, "resume", "--json", "--skip-git-repo-check", "-c", `sandbox_mode="read-only"`)
		} else {
			perm := "read-only"
			if write {
				perm = "workspace-write"
			}
			args = append(args, "--json", "--skip-git-repo-check", "-C", repo, "-s", perm)
			if write {
				args = append(args, "-c", "sandbox_workspace_write.network_access=true")
			}
		}
	case "cursor":
		args = []string{"-p", "--output-format", "stream-json", "--trust", "--workspace", repo}
		if write {
			args = append(args, "--force", "--sandbox", "enabled")
		} else {
			args = append(args, "--mode", "ask")
		}
		if name == "resume_ok" {
			args = append(args, "--resume", sid)
		}
	case "antigravity":
		args = []string{"--output-format", "stream-json", "--print-timeout", "0"}
		if write {
			args = append(args, "--dangerously-skip-permissions", "--sandbox")
		} else {
			args = append(args, "--mode", "plan")
		}
		if name == "resume_ok" {
			args = append(args, "--conversation", sid)
		}
	}
	if name == "error_bad_model" {
		modelFlag := "--model"
		if agent == "codex" {
			modelFlag = "-m"
		}
		args = append(args, modelFlag, "nonexistent-model-xyz")
	}
	if (name == "schema_ok" || name == "image_ok") && agent != "cursor" {
		if name == "image_ok" {
			schema = filepath.Join(filepath.Dir(schema), "image-schema.json")
		}
		schemaFlag := "--output-schema"
		if agent == "antigravity" {
			schemaFlag = "--json-schema"
		}
		args = append(args, schemaFlag, schema)
	}
	if name == "image_ok" {
		args = append(args, "--add-dir", out)
	}
	if agent == "codex" {
		args = append(args, "--")
		if name == "resume_ok" {
			args = append(args, sid)
		}
	} else if agent == "antigravity" {
		args = append(args, "--print")
	}
	return append(args, prompt)
}

func recordCase(ctx context.Context, exe string, args []string, repo, tmp, home, dest, name string, log io.Writer) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir, cmd.SysProcAttr = repo, &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = ctx.Err()
	}
	argv, _ := json.Marshal(args)
	fmt.Fprintf(log, "%s argv=%s duration=%s exit=%d\n", name, sanitize(argv, tmp, home), time.Since(start).Round(time.Millisecond), code)
	for ext, data := range map[string][]byte{".jsonl": stdout.Bytes(), ".stderr": stderr.Bytes()} {
		if writeErr := os.WriteFile(filepath.Join(dest, name+ext), sanitize(data, tmp, home), 0600); writeErr != nil {
			return stdout.Bytes(), code, writeErr
		}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		err = nil // 非零退出也是待录制的 CLI 行为。
	}
	return stdout.Bytes(), code, err
}

func sanitize(raw []byte, tmp, home string) []byte {
	canonical, _ := filepath.EvalSymlinks(tmp)
	for _, pair := range [][2]string{{canonical, "/TMP"}, {tmp, "/TMP"}, {os.Getenv("CODEX_HOME"), "/CODEX_HOME"}, {home, "/HOME"}} {
		if pair[0] != "" {
			raw = bytes.ReplaceAll(raw, []byte(pair[0]), []byte(pair[1]))
		}
	}
	return raw
}

func sessionID(agent string, stdout []byte) string {
	for _, line := range bytes.Split(stdout, []byte{'\n'}) {
		var v map[string]json.RawMessage
		if json.Unmarshal(line, &v) != nil {
			continue
		}
		key := map[string]string{"codex": "thread_id", "cursor": "session_id", "antigravity": "conversation_id"}[agent]
		if agent == "antigravity" {
			for _, nested := range []string{"step_update", "result"} {
				if json.Unmarshal(v[nested], &v) == nil {
					break
				}
			}
		}
		var sid string
		_ = json.Unmarshal(v[key], &sid)
		if sid != "" {
			return sid
		}
	}
	return ""
}

func recentImages(root string, since time.Time) (paths []string) {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		count++
		if count > 2000 {
			return filepath.SkipAll
		}
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() && strings.Count(rel, string(os.PathSeparator)) >= 2 {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.Contains("|.png|.jpg|.jpeg|.webp|.gif|", "|"+strings.ToLower(filepath.Ext(path))+"|") {
			if info, err := d.Info(); err == nil && !info.ModTime().Before(since) {
				paths = append(paths, path)
			}
		}
		return nil
	})
	return paths
}
