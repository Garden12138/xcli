//go:build unix

package proc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var shellProbeTimeout = 3 * time.Second

func SearchPath(configured, cacheFile string) string {
	if configured != "" {
		return configured
	}
	var cache struct {
		Path      string    `json:"path"`
		CheckedAt time.Time `json:"checked_at"`
	}
	raw, _ := os.ReadFile(cacheFile)
	if json.Unmarshal(raw, &cache) != nil || strings.TrimSpace(cache.Path) == "" || time.Since(cache.CheckedAt) >= 24*time.Hour {
		cache.Path = ""
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		out := Run(context.Background(), Spec{Path: shell, Args: []string{"-lic", `printf %s "$PATH"`}}, Options{Timeout: shellProbeTimeout, OnLine: func(line []byte) { cache.Path = strings.TrimSpace(string(line)) }})
		if out.StartErr != nil || out.TimedOut || out.Cancelled || out.ExitCode != 0 {
			cache.Path = ""
		}
		if cache.Path != "" && cacheFile != "" && os.MkdirAll(filepath.Dir(cacheFile), 0700) == nil {
			cache.CheckedAt = time.Now().UTC()
			raw, _ = json.Marshal(cache)
			_ = os.WriteFile(cacheFile, raw, 0600)
		}
	}
	home, _ := os.UserHomeDir()
	paths := []string{}
	seen := map[string]bool{}
	for _, path := range strings.Split(cache.Path+":"+os.Getenv("PATH")+":"+filepath.Join(home, ".local/bin")+":/opt/homebrew/bin:/usr/local/bin", ":") {
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return strings.Join(paths, ":")
}

func LookPath(name, pathList string) (string, error) {
	paths := filepath.SplitList(pathList)
	if strings.ContainsRune(name, os.PathSeparator) {
		paths = []string{""}
	}
	for _, dir := range paths {
		path, err := filepath.Abs(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			return path, nil
		}
	}
	return "", fmt.Errorf("请安装命令 %q 或配置其可执行文件路径：%w", name, exec.ErrNotFound)
}

func Env(base []string, pathList string, extra map[string]string) []string {
	values := map[string]string{}
	for _, entry := range base {
		if key, value, ok := strings.Cut(entry, "="); ok {
			values[key] = value
		}
	}
	values["PATH"] = pathList
	for key, value := range extra {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}
