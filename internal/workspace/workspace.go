package workspace

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Garden12138/xcli/internal/api"
)

type Workspace struct {
	Dir       string
	Original  string
	Isolation string
	Branch    string
	Git       bool
	InputsDir string
	Inputs    []string
	Images    []string
	once      sync.Once
	baseErr   error
	base      string
	untracked map[string]bool
	files     map[string]fileState
}

func gitOutput(dir string, args ...string) ([]byte, error) {
	raw, err := exec.Command("git", append([]string{"--no-pager", "-c", "color.ui=false", "-C", dir}, args...)...).Output()
	if err != nil {
		var exit *exec.ExitError
		var detail string
		if errors.As(err, &exit) {
			detail = string(exit.Stderr)
		}
		return raw, fmt.Errorf("请检查工作目录与 git 参数后重试：%w：%s", err, detail)
	}
	return raw, nil
}

func Prepare(taskDir, taskID string, req api.Request) (*Workspace, []string, error) {
	taskDir, err := filepath.Abs(taskDir)
	if err != nil {
		return nil, nil, err
	}
	w := &Workspace{Dir: req.Workdir, Original: req.Workdir, Isolation: "none", InputsDir: filepath.Join(taskDir, "inputs"), Inputs: []string{}, Images: []string{}}
	if req.Workdir == "" {
		w.Dir, w.Isolation = filepath.Join(taskDir, "work"), "temp"
		if err := os.MkdirAll(w.Dir, 0700); err != nil {
			return nil, nil, err
		}
	} else if info, err := os.Stat(w.Dir); err != nil || !info.IsDir() {
		return nil, nil, errors.New("请为 workdir 提供现有目录")
	}
	raw, gitErr := gitOutput(w.Dir, "rev-parse", "--is-inside-work-tree")
	w.Git = gitErr == nil && strings.TrimSpace(string(raw)) == "true"
	if req.Isolate && !w.Git {
		return nil, nil, errors.New("isolate 需要 git 仓库，请选择仓库目录后重试")
	}
	if err := os.MkdirAll(w.InputsDir, 0700); err != nil {
		return nil, nil, err
	}
	remaining := int64(200 * 1024 * 1024)
	index := 0
	copies := map[string]string{}
	for _, group := range []struct {
		sources []string
		target  *[]string
	}{{req.Inputs, &w.Inputs}, {req.Images, &w.Images}} {
		for _, src := range group.sources {
			absolute, err := filepath.Abs(src)
			if err != nil {
				return nil, nil, err
			}
			dest := copies[absolute]
			if dest == "" {
				index++
				dest = filepath.Join(w.InputsDir, fmt.Sprintf("%03d-%s", index, filepath.Base(absolute)))
				if err := copyInput(absolute, dest, &remaining, map[string]bool{}); err != nil {
					return nil, nil, err
				}
				copies[absolute] = dest
			}
			*group.target = append(*group.target, dest)
		}
	}
	notices := []string{}
	if req.Isolate {
		status, err := gitOutput(w.Dir, "status", "--porcelain")
		if err != nil {
			return nil, nil, err
		}
		if len(status) > 0 {
			notices = append(notices, "worktree 基于 HEAD，未提交的改动不会带入")
		}
		w.Branch = "xcli/" + taskID
		isolated := filepath.Join(taskDir, "worktree")
		if _, err := gitOutput(w.Dir, "worktree", "add", "-b", w.Branch, isolated, "HEAD"); err != nil {
			return nil, notices, err
		}
		w.Dir, w.Isolation = isolated, "worktree"
	}
	return w, notices, nil
}

func copyInput(src, dest string, remaining *int64, stack map[string]bool) error {
	canonical, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("请检查输入路径 %s：%w", src, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if stack[canonical] {
			return fmt.Errorf("请移除输入目录 %s 中的符号链接循环后重试", src)
		}
		stack[canonical] = true
		defer delete(stack, canonical)
		if err := os.MkdirAll(dest, 0700); err != nil {
			return err
		}
		entries, err := os.ReadDir(canonical)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyInput(filepath.Join(canonical, entry.Name()), filepath.Join(dest, entry.Name()), remaining, stack); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("请将输入 %s 改为普通文件或目录", src)
	}
	if info.Size() > *remaining {
		return errors.New("输入快照总量超过 200 MB，请精简输入后重试")
	}
	input, err := os.Open(canonical)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	written, copyErr := io.CopyN(output, input, *remaining+1)
	*remaining -= written
	if errors.Is(copyErr, io.EOF) {
		copyErr = nil
	}
	if *remaining < 0 {
		copyErr = errors.New("输入快照总量超过 200 MB，请精简输入后重试")
	}
	return errors.Join(copyErr, output.Close())
}

func Listing(dir string, max int) []string {
	entries, _ := os.ReadDir(dir)
	listing := []string{}
	for _, entry := range entries {
		if len(listing) >= max {
			break
		}
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		listing = append(listing, name)
	}
	return listing
}
