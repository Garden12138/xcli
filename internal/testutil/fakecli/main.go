//go:build unix

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Garden12138/xcli/internal/testutil"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--version" {
		fmt.Println("fake 1.0")
		return
	}
	if len(args) > 0 && (args[0] == "status" || len(args) > 1 && args[0] == "login" && args[1] == "status") {
		fmt.Println("ok")
		return
	}
	code, err := run(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "请检查 fakecli 剧本与临时目录：", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func run(args []string) (int, error) {
	name := filepath.Base(os.Args[0])
	raw, err := os.ReadFile(os.Getenv("XCLI_FAKE_SCRIPT"))
	if err != nil {
		return 0, err
	}
	var script testutil.FakeScript
	if err := json.Unmarshal(raw, &script); err != nil {
		return 0, err
	}
	steps := script[name]
	if len(steps) == 0 {
		return 0, fmt.Errorf("请为 %s 提供剧本步骤", name)
	}
	index, err := nextIndex(name)
	if err != nil {
		return 0, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	log, err := os.OpenFile(os.Getenv("XCLI_FAKE_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return 0, err
	}
	err = json.NewEncoder(log).Encode(testutil.FakeCall{Name: name, Args: args, Cwd: cwd})
	if err = errors.Join(err, log.Close()); err != nil {
		return 0, err
	}
	step := steps[min(index, len(steps)-1)]
	time.Sleep(time.Duration(step.SleepMS) * time.Millisecond)
	for path, content := range step.WriteFiles {
		path = strings.ReplaceAll(path, "{{CWD}}", cwd)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return 0, err
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return 0, err
		}
	}
	if step.WritePNG != "" {
		path := step.WritePNG
		if strings.Contains(path, "{{OUT}}") {
			prompt := strings.Join(args, "\n")
			_, rest, ok := strings.Cut(prompt, "输出目录：")
			if !ok {
				return 0, errors.New("请在提示词中提供 输出目录：<路径>")
			}
			out, _, _ := strings.Cut(rest, "\n")
			path = strings.ReplaceAll(path, "{{OUT}}", strings.TrimSpace(out))
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return 0, err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return 0, err
		}
		err = png.Encode(file, image.NewRGBA(image.Rect(0, 0, 8, 8)))
		if err = errors.Join(err, file.Close()); err != nil {
			return 0, err
		}
	}
	if step.StdoutFile != "" {
		data, err := os.ReadFile(step.StdoutFile)
		if err != nil {
			return 0, err
		}
		fmt.Print(strings.ReplaceAll(string(data), "{{CWD}}", cwd))
	}
	for _, line := range step.Stdout {
		fmt.Println(strings.ReplaceAll(line, "{{CWD}}", cwd))
	}
	fmt.Fprint(os.Stderr, step.Stderr)
	return step.Exit, nil
}

func nextIndex(name string) (int, error) {
	dir := os.Getenv("XCLI_FAKE_STATE")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return 0, err
	}
	file, err := os.OpenFile(filepath.Join(dir, name+".count"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return 0, err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	raw, err := io.ReadAll(file)
	if err != nil {
		return 0, err
	}
	index := 0
	if len(raw) > 0 {
		index, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			return 0, err
		}
	}
	if err := file.Truncate(0); err != nil {
		return 0, err
	}
	if _, err := file.Seek(0, 0); err != nil {
		return 0, err
	}
	_, err = fmt.Fprint(file, index+1)
	return index, err
}
