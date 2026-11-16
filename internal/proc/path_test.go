//go:build unix

package proc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchPathConfigured(t *testing.T) {
	t.Setenv("SHELL", "/missing-shell")
	if got := SearchPath("/a:/b", ""); got != "/a:/b" {
		t.Fatal(got)
	}
}

func TestSearchPathShellAndCache(t *testing.T) {
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
