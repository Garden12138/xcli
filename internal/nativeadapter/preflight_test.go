package nativeadapter_test

import (
	"os"
	"path/filepath"
	"testing"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

func TestPreflightCannotPromoteOrSpawnNative(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "must-not-execute")
	file := filepath.Join(root, "selected-cli")
	body := []byte("#!/bin/sh\ntouch '" + marker + "'\nexit 93\n")
	if err := os.WriteFile(file, body, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := agentpolicy.DefaultConfig()
	cfg.DailyDispatches, cfg.DailyProcessStarts, cfg.DailyExecutionSeconds = 1, 1, 120
	for _, p := range agentpolicy.Defaults() {
		p.Blocker = ""
		v := "installed-fixture"
		p.Version, p.Binary, p.BinarySHA256 = &v, file, api.Hash(body)
		p = p.Freeze()
		r, err := nativeadapter.InspectNativeText(p, cfg)
		if err != nil || r.CanExecute || r.ProcessStarts != 0 || r.ModelRequests != 0 {
			t.Fatal(r, err)
		}
		checks := map[string]nativeadapter.PreflightCheck{}
		for _, c := range r.Checks {
			checks[c.ID] = c
		}
		if checks["selected_file_integrity"].Status != "PASS" || checks["execution_policy"].Evidence != "policy_unenforceable" || checks["account_entitlement"].Status != "NOT_TESTED" {
			t.Fatal(checks)
		}
		p.BinarySHA256 = "changed"
		r, err = nativeadapter.InspectNativeText(p, cfg)
		if err != nil || r.Checks[3].Evidence != "profile_changed" {
			t.Fatal(r, err)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("preflight executed a binary", err)
	}
	if _, err := nativeadapter.InspectNativeText(agentpolicy.Profile{}, cfg); err == nil {
		t.Fatal("fixture accepted as native evidence")
	}
}
