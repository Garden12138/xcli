package agentpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/modelstore"
	xruntime "github.com/Garden12138/xcli/internal/runtime"
)

type CachedInstallation struct {
	AgentID      string    `json:"agent_id"`
	Binary       string    `json:"binary"`
	BinarySHA256 string    `json:"binary_sha256"`
	Version      string    `json:"version"`
	CheckedAt    time.Time `json:"checked_at"`
	Source       string    `json:"source"`
}
type installationCache struct {
	Version int                           `json:"version"`
	Entries map[string]CachedInstallation `json:"entries"`
}

func readCache(root string) (installationCache, error) {
	b, e := modelstore.ReadPrivate(filepath.Join(root, "installations.json"), 1<<20)
	if os.IsNotExist(e) {
		return installationCache{1, map[string]CachedInstallation{}}, nil
	}
	if e != nil {
		return installationCache{}, e
	}
	var cache installationCache
	if json.Unmarshal(b, &cache) != nil || cache.Version != 1 || cache.Entries == nil {
		return cache, errors.New("invalid installation cache")
	}
	return cache, nil
}

// CachedProfiles never starts a process and never reads native authentication.
func CachedProfiles(root string) ([]Profile, error) {
	cache, e := readCache(root)
	if e != nil {
		return nil, e
	}
	profiles := Defaults()
	for i, p := range profiles {
		if r, ok := cache.Entries[p.AgentID]; ok {
			p.Binary = r.Binary
			p.BinarySHA256 = r.BinarySHA256
			p.Version = &r.Version
			profiles[i] = p.Freeze()
		}
	}
	return profiles, nil
}

// Refresh is an owner-only, explicit --version probe, separate from dispatch.
// It cannot enable a profile or modify native authentication/configuration.
func Refresh(ctx context.Context, root, agentID, binary string) (CachedInstallation, error) {
	if !slices.Contains([]string{"codex", "cursor", "antigravity"}, agentID) || !filepath.IsAbs(binary) {
		return CachedInstallation{}, errors.New("registered Agent and absolute binary path required")
	}
	resolved, e := filepath.EvalSymlinks(binary)
	if e != nil {
		return CachedInstallation{}, e
	}
	info, e := os.Stat(resolved)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Size() > 512<<20 {
		return CachedInstallation{}, errors.New("regular bounded executable required")
	}
	b, e := os.ReadFile(resolved)
	if e != nil {
		return CachedInstallation{}, e
	}
	digest := api.Hash(b)
	work, e := os.MkdirTemp(root, "refresh-")
	if e != nil {
		return CachedInstallation{}, e
	}
	defer os.RemoveAll(work)
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	result, e := xruntime.RunProcess(ctx, agent.CommandSpec{Command: resolved, Args: []string{"--version"}}, xruntime.ProcessOptions{Dir: work, Env: Environment(nil, work), CaptureStdout: true, SeparateProcess: true, BoundedCleanup: true})
	// Fixed, non-sensitive failure codes: never print the native stderr tail.
	switch {
	case e != nil:
		return CachedInstallation{}, errors.New("version_probe_start_failed")
	case result.WaitIncomplete || !result.StopConfirmed:
		return CachedInstallation{}, errors.New("version_probe_stop_unconfirmed")
	case result.TimedOut || result.Canceled:
		return CachedInstallation{}, errors.New("version_probe_timeout_or_canceled")
	case result.ExitCode != 0:
		return CachedInstallation{}, errors.New("version_probe_exit_nonzero")
	case result.CaptureTruncated || len(result.Stdout) > 64<<10:
		return CachedInstallation{}, errors.New("version_probe_output_limit")
	case result.IOError:
		return CachedInstallation{}, errors.New("version_probe_io_failed")
	}
	version := strings.TrimSpace(string(result.Stdout))
	if len(version) < 1 || len(version) > 200 || strings.ContainsAny(version, "\r\n") {
		return CachedInstallation{}, errors.New("invalid version response")
	}
	after, e := os.ReadFile(resolved)
	if e != nil || api.Hash(after) != digest {
		return CachedInstallation{}, errors.New("binary changed during refresh")
	}
	r := CachedInstallation{AgentID: agentID, Binary: resolved, BinarySHA256: digest, Version: version, CheckedAt: time.Now().UTC(), Source: "owner_explicit_non_model_probe"}
	cache, e := readCache(root)
	if e != nil {
		return r, e
	}
	cache.Entries[agentID] = r
	data, _ := json.Marshal(cache)
	return r, modelstore.WritePrivate(filepath.Join(root, "installations.json"), data)
}
