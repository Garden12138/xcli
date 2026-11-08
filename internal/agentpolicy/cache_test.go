package agentpolicy

import (
	"context"
	"github.com/Garden12138/xcli/internal/modelstore"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitRefreshIsNonModelAndCannotEnableNative(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	if e := modelstore.PrivateDirectory(root); e != nil {
		t.Fatal(e)
	}
	script := filepath.Join(root, "fixture-version")
	if e := os.WriteFile(script, []byte("#!/bin/sh\nif [ \"$1\" != --version ] || [ \"$#\" != 1 ]; then exit 13; fi\nprintf 'fixture-1\\n'\n"), 0o700); e != nil {
		t.Fatal(e)
	}
	r, e := Refresh(context.Background(), root, "codex", script)
	if e != nil || r.Version != "fixture-1" {
		t.Fatal(r, e)
	}
	profiles, e := CachedProfiles(root)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range profiles {
		if e = p.Check("text_task"); e == nil {
			t.Fatal("refresh enabled native profile")
		}
	}
}

func TestFailedVersionProbeDoesNotCacheOrExposeDiagnostics(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	if e := modelstore.PrivateDirectory(root); e != nil {
		t.Fatal(e)
	}
	script := filepath.Join(root, "fixture-version")
	if e := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'private-diagnostic-secret' >&2\nexit 17\n"), 0o700); e != nil {
		t.Fatal(e)
	}
	_, e := Refresh(context.Background(), root, "cursor", script)
	if e == nil || e.Error() != "version_probe_exit_nonzero" {
		t.Fatal(e)
	}
	profiles, e := CachedProfiles(root)
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range profiles {
		if p.Version != nil || p.Binary != "" || p.Check("text_task") == nil {
			t.Fatal("failed probe changed capability", p)
		}
	}
}
