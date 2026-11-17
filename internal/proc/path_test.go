//go:build unix

package proc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSearchPathConfigured(t *testing.T) {
	t.Setenv("SHELL", "/missing-shell")
	if got := SearchPath("/a:/b", ""); got != "/a:/b" {
		t.Fatal(got)
	}
}

func TestSearchPathShellAndCache(t *testing.T) {
	old := shellProbeTimeout
	shellProbeTimeout = 10 * time.Second
	t.Cleanup(func() { shellProbeTimeout = old })
	dir := t.TempDir()
	shell := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf '/custom/bin:/custom/bin:/bin'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/bin:/usr/bin")
	cache := filepath.Join(dir, "cache", "env.json")
	want := SearchPath("", cache)
	if !strings.HasPrefix(want, "/custom/bin:/bin:/usr/bin") || strings.Count(want, "/custom/bin") != 1 {
		t.Fatal(want)
	}
	if err := os.Remove(shell); err != nil {
		t.Fatal(err)
	}
	if got := SearchPath("", cache); got != want {
		t.Fatalf("cached path = %s, want %s", got, want)
	}
}

func TestSearchPathProbeFailureNotCached(t *testing.T) {
	old := shellProbeTimeout
	shellProbeTimeout = time.Second
	t.Cleanup(func() { shellProbeTimeout = old })
	for _, tc := range []struct{ name, script string }{
		{"missing", ""},
		{"failed", "printf '/partial/bin\\n'\nexit 1\n"},
		{"empty", "printf ' \\n'\n"},
		{"timed_out", "printf '/partial/bin\\n'\ntrap 'exit 0' TERM\nwhile :; do /bin/sleep 10; done\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			shell := filepath.Join(dir, "fake-shell")
			if tc.script != "" {
				if err := os.WriteFile(shell, []byte("#!/bin/sh\n"+tc.script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("SHELL", shell)
			t.Setenv("PATH", "/fallback/bin")
			cache := filepath.Join(dir, "cache", "env.json")
			got := SearchPath("", cache)
			if !strings.HasPrefix(got, "/fallback/bin:") || strings.Contains(got, "/partial/bin") {
				t.Errorf("failed probe polluted fallback PATH: %s", got)
			}
			if _, err := os.Stat(cache); !os.IsNotExist(err) {
				t.Fatalf("failed probe wrote cache: %v", err)
			}
		})
	}
}

func TestSearchPathIgnoresEmptyCache(t *testing.T) {
	dir := t.TempDir()
	shell := filepath.Join(dir, "fake-shell")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\nprintf '/recovered/bin'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", "/bin")
	cache := filepath.Join(dir, "env.json")
	raw := []byte(`{"path":"","checked_at":"` + time.Now().UTC().Format(time.RFC3339Nano) + `"}`)
	if err := os.WriteFile(cache, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got := SearchPath("", cache); !strings.HasPrefix(got, "/recovered/bin:/bin:") {
		t.Fatalf("empty cached probe prevented recovery: %s", got)
	}
}

func TestEnvReplacesPath(t *testing.T) {
	got := Env([]string{"PATH=/old", "A=one", "PATH=/duplicate"}, "/new", map[string]string{"A": "two", "B": "three"})
	paths := 0
	for _, value := range got {
		if strings.HasPrefix(value, "PATH=") {
			paths++
			if value != "PATH=/new" {
				t.Fatal(value)
			}
		}
	}
	if paths != 1 || !strings.Contains(strings.Join(got, "\n"), "A=two") || !strings.Contains(strings.Join(got, "\n"), "B=three") {
		t.Fatal(got)
	}
}

func TestLookPathUsesProvidedPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fakecli")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, err := LookPath("fakecli", dir); err != nil || got != path {
		t.Fatalf("lookup: %s, %v", got, err)
	}
	if _, err := LookPath("fakecli", "/missing"); err == nil {
		t.Fatal("lookup searched outside configured PATH")
	}
}
