// Package launchtest installs explicit synthetic budgets for offline fixtures.
// It is imported only by tests; production never creates an implicit window.
package launchtest

import (
	"github.com/Garden12138/xcli/internal/launch"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func Main(m *testing.M) {
	if os.Getenv("XCLI_LAUNCH_TEST") == "1" && os.Getenv("XCLI_LAUNCH_STORE") != "" && os.Getenv("XCLI_LAUNCH_WINDOW") != "" {
		os.Exit(m.Run())
	}
	dir, e := os.MkdirTemp("", "xcli-launch-fixture-")
	if e != nil {
		panic(e)
	}
	_ = os.Chmod(dir, 0700)
	n := 0
	ref := launch.Ref{Root: filepath.Join(dir, "ledger"), Window: "offline"}
	if e = launch.Create(ref, launch.Window{ID: "offline", Limit: 100000, MaxConcurrent: 256, External: launch.External{Agent: "fixture-only", Count: &n, Source: "explicit Go fake-CLI test fixture", Confirmed: true, Scope: "excluded"}}); e != nil {
		panic(e)
	}
	_ = os.Setenv("XCLI_LAUNCH_TEST", "1")
	_ = os.Setenv("XCLI_LAUNCH_STORE", ref.Root)
	_ = os.Setenv("XCLI_LAUNCH_WINDOW", ref.Window)
	if runtime.GOOS != "windows" {
		bin := filepath.Join(dir, "deny-real")
		_ = os.Mkdir(bin, 0700)
		for _, name := range []string{"codex", "cursor-agent", "agy", "claude", "gemini", "opencode"} {
			_ = os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 127\n"), 0700)
		}
		_ = os.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
