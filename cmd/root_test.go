package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootVersion(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"--version", "--home", t.TempDir()}} {
		root := newRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(out.String()); got != "xcli version 2.0.1" {
			t.Fatalf("version = %q", got)
		}
	}
}

func TestRootHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, tc := range []struct {
		name, env, flag, want string
	}{
		{"default", "", "", filepath.Join(home, ".xcli")},
		{"environment", filepath.Join(home, "env"), "", filepath.Join(home, "env")},
		{"flag", filepath.Join(home, "env"), filepath.Join(home, "flag"), filepath.Join(home, "flag")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XCLI_HOME", tc.env)
			root := newRootCommand()
			root.SetOut(new(bytes.Buffer))
			args := []string{"--help"}
			if tc.flag != "" {
				args = append(args, "--home", tc.flag)
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if rootOpts.home != tc.want {
				t.Fatalf("home = %q, want %q", rootOpts.home, tc.want)
			}
		})
	}
}
