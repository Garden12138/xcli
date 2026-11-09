package agentpolicy

import (
	"os"
	"path/filepath"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/modelstore"
)

const AppServerAdapter = "codex-app-server-v1"
const AppServerPolicy = "codex-app-server-readonly-v1"
const IsolatedRuntimeRequired = "ISOLATED_RUNTIME_REQUIRED"

type AppServerBinding struct{ Root string }

func (b *AppServerBinding) Check(native bool) error {
	// No owner-writable readiness claim may promote the shared-UID authenticated
	// runtime. G6's whole-process sandbox is credential-free, offline-only.
	if native {
		return api.Failure(503, "policy_unenforceable")
	}
	_, e := os.Lstat(filepath.Join(b.Root, "app-server-disabled.json"))
	if !os.IsNotExist(e) {
		return api.Failure(503, "policy_unenforceable")
	}
	return nil
}
func DisableAppServer(root string) error {
	return modelstore.WritePrivate(filepath.Join(root, "app-server-disabled.json"), []byte(`{"disabled":true}`))
}
func BindAppServer(root string, cfg Config, profiles []Profile) []Profile {
	if !cfg.CodexAppServerEnabled {
		return profiles
	}
	p := Profile{AgentID: "codex", AdapterID: AppServerAdapter, PolicyID: AppServerPolicy, Revision: "1", CapabilityRevision: "nag-app-server-unverified-v1", InstallationDomain: "codex", Location: "remote", Native: true, Blocker: "policy_unenforceable", Operations: []string{"text_task"}, AppServer: &AppServerBinding{Root: root}}
	for _, old := range profiles {
		if old.AgentID == "codex" {
			p.Binary = old.Binary
			p.BinarySHA256 = old.BinarySHA256
			p.Version = old.Version
			break
		}
	}
	return append(profiles, p.Freeze())
}
