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

var proxyKeys = []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"}

type shellEnvironment struct {
	Path      string            `json:"path"`
	Proxy     map[string]string `json:"proxy"`
	CheckedAt time.Time         `json:"checked_at"`
}

func SearchPath(configured, cacheFile string) string {
	if configured != "" {
		return configured
	}
	path, _ := SearchEnvironment(configured, cacheFile, true)
	return path
}

// SearchEnvironment keeps explicit PATH configuration and independently resolves proxy defaults.
func SearchEnvironment(configured, cacheFile string, inheritProxy bool) (string, map[string]string) {
	needProxy := inheritProxy && !hasProxy(os.Environ())
	if configured != "" && !needProxy {
		return configured, nil
	}
	var cache shellEnvironment
	raw, _ := os.ReadFile(cacheFile)
	valid := json.Unmarshal(raw, &cache) == nil && strings.TrimSpace(cache.Path) != "" &&
		time.Since(cache.CheckedAt) < 24*time.Hour
	if !valid {
		cache = shellEnvironment{}
	}
	if !valid || (needProxy && cache.Proxy == nil) {
		if fresh := probeShell(needProxy); fresh.Path != "" {
			cache = fresh
			if cacheFile != "" {
				saveShellEnvironment(cacheFile, cache)
			}
		}
	}
	var proxy map[string]string
	if needProxy {
		proxy = cache.Proxy
	}
	if configured != "" {
		return configured, proxy
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
	return strings.Join(paths, ":"), proxy
}

func probeShell(needProxy bool) shellEnvironment {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	format, args := `\nxcli-env:\000%s\000`, ` "$PATH"`
	if needProxy {
		for _, key := range proxyKeys {
			format += `%s\000`
			args += ` "` + "$" + key + `"`
		}
	}
	var frame strings.Builder
	started, complete := false, false
	expected := 1
	if needProxy {
		expected += len(proxyKeys)
	}
	fields := 0
	out := Run(context.Background(), Spec{Path: shell, Args: []string{"-lic", "printf '" + format + `\n'` + args}}, Options{
		Timeout: shellProbeTimeout,
		OnLine: func(line []byte) {
			if complete {
				return
			}
			if value, ok := strings.CutPrefix(string(line), "xcli-env:\x00"); ok && !started {
				started, line = true, []byte(value)
			}
			if started && frame.Len()+len(line) < maxOutputLine {
				frame.Write(line)
				fields += strings.Count(string(line), "\x00")
				complete = fields == expected && len(line) > 0 && line[len(line)-1] == 0
				if !complete {
					frame.WriteByte('\n')
				}
			}
		},
	})
	if !complete || out.StartErr != nil || out.TimedOut || out.Cancelled || out.ExitCode != 0 {
		return shellEnvironment{}
	}
	values := strings.Split(strings.TrimSuffix(frame.String(), "\x00"), "\x00")
	cache := shellEnvironment{Path: strings.TrimSpace(values[0]), CheckedAt: time.Now().UTC()}
	if needProxy {
		cache.Proxy = make(map[string]string)
		for i, key := range proxyKeys {
			if values[i+1] != "" {
				cache.Proxy[key] = values[i+1]
			}
		}
	}
	return cache
}

func saveShellEnvironment(path string, cache shellEnvironment) {
	dir := filepath.Dir(path)
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	file, err := os.CreateTemp(dir, ".env-*")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	err = json.NewEncoder(file).Encode(cache)
	if closeErr := file.Close(); err == nil && closeErr == nil {
		_ = os.Rename(file.Name(), path)
	}
}

func hasProxy(env []string) bool {
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if isProxyKey(key) {
			return true
		}
	}
	return false
}

func isProxyKey(key string) bool {
	for _, candidate := range proxyKeys {
		if key == candidate {
			return true
		}
	}
	return false
}

// CLIEnv applies native process settings after the optional shell defaults.
func CLIEnv(path string, extra, shellProxy map[string]string) []string {
	base := os.Environ()
	if !hasProxy(base) {
		for _, key := range proxyKeys {
			if value, ok := shellProxy[key]; ok {
				base = append(base, key+"="+value)
			}
		}
	}
	return Env(base, path, extra)
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
