// Package agentpolicy contains server-owned, immutable native execution profiles.
package agentpolicy

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"bytes"
	"errors"
	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/config"
	"gopkg.in/yaml.v3"
	"io"
)

type Config struct {
	Version               int    `yaml:"version" json:"version"`
	Host                  string `yaml:"host" json:"host"`
	Port                  int    `yaml:"port" json:"port"`
	DailyDispatches       int    `yaml:"daily_dispatches" json:"daily_dispatches"`
	DailyProcessStarts    int    `yaml:"daily_process_starts" json:"daily_process_starts"`
	DailyExecutionSeconds int    `yaml:"daily_execution_seconds" json:"daily_execution_seconds"`
	QueueLimit            int    `yaml:"queue_limit" json:"queue_limit"`
}

func DefaultConfig() Config {
	return Config{Version: 1, Host: "127.0.0.1", Port: 18742, QueueLimit: 16}
}
func (c Config) Validate() error {
	if c.Version != 1 || !slices.Contains([]string{"127.0.0.1", "::1"}, c.Host) || c.Port < 0 || c.Port > 65535 || c.DailyDispatches < 0 || c.DailyDispatches > 1000 || c.DailyProcessStarts < 0 || c.DailyProcessStarts > 1000 || c.DailyExecutionSeconds < 0 || c.DailyExecutionSeconds > 86400 || c.QueueLimit < 1 || c.QueueLimit > 128 {
		return errors.New("invalid independent Agent Gateway configuration")
	}
	return nil
}
func (c Config) Address() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }
func Load(path string) (Config, error) {
	c := DefaultConfig()
	if path == "" {
		p, e := config.DefaultPath()
		if e != nil {
			return c, e
		}
		path = filepath.Join(filepath.Dir(p), "agent-gateway.yaml")
	}
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return c, c.Validate()
	}
	if e != nil {
		return c, e
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("one configuration document required")
	}
	return c, c.Validate()
}

type Profile struct {
	AgentID            string
	AdapterID          string
	PolicyID           string
	Revision           string
	CapabilityRevision string
	SHA256             string
	Binary             string
	BinarySHA256       string
	Version            *string
	InstallationDomain string
	Location           string
	Native             bool
	Blocker            string
	Operations         []string
}

func (p Profile) Freeze() Profile {
	p.SHA256 = ""
	p.Operations = slices.Clone(p.Operations)
	p.SHA256 = api.Digest(p)
	return p
}
func (p Profile) Check(operation string) error {
	if !slices.Contains(p.Operations, operation) {
		return api.Failure(422, "capability_missing")
	}
	if p.Blocker != "" {
		return api.Failure(503, p.Blocker)
	}
	// No production native profile is promoted by a configuration boolean. L04
	// must establish a per-invocation filesystem/tool/network boundary first.
	if p.Native {
		return api.Failure(503, "policy_unenforceable")
	}
	return nil
}
func (p Profile) CheckBinary() error {
	if p.Binary == "" {
		return api.Failure(503, "agent_not_installed")
	}
	b, e := os.ReadFile(p.Binary)
	if e != nil {
		return api.Failure(503, "agent_not_installed")
	}
	if api.Hash(b) != p.BinarySHA256 {
		return api.Failure(409, "profile_changed")
	}
	return nil
}
func Defaults() []Profile {
	out := []Profile{}
	for _, pair := range [][2]string{{"codex", "codex-exec-v1"}, {"antigravity", "antigravity-print-v1"}, {"cursor", "cursor-headless-v1"}} {
		p := Profile{AgentID: pair[0], AdapterID: pair[1], PolicyID: "synthetic-text-readonly-v1", Revision: "1", CapabilityRevision: "nag-native-unverified-v1", InstallationDomain: pair[0], Location: "remote", Native: true, Blocker: "policy_unenforceable", Operations: []string{"text_task", "image_generate", "image_edit"}}
		out = append(out, p.Freeze())
	}
	return out
}
func Catalog(profiles []Profile, instance string, clientAgents, clientPolicies, clientOperations []string) api.Capabilities {
	c := api.Capabilities{APIVersion: 1, InstanceID: instance, Revision: api.Digest(profiles), GeneratedAt: time.Now().UTC(), Agents: []api.Agent{}}
	if len(profiles) > 0 {
		c.Revision = profiles[0].CapabilityRevision
	}
	for _, p := range profiles {
		if !slices.Contains(clientAgents, p.AgentID) || !slices.Contains(clientPolicies, p.PolicyID) {
			continue
		}
		ops := map[string]api.Operation{}
		for _, op := range p.Operations {
			if !slices.Contains(clientOperations, op) {
				continue
			}
			available, reason, evidence := "blocked", p.Blocker, "unverified"
			if p.Check(op) == nil {
				available = "available"
				reason = "offline_fixture_only"
				evidence = "verified"
			}
			if reason == "" {
				reason = "policy_unenforceable"
			}
			ops[op] = api.Operation{Documented: p.Native, NativeEvidence: "unverified", GatewayEvidence: evidence, Availability: available, Reason: reason}
		}
		var digest *string
		if p.BinarySHA256 != "" {
			v := p.BinarySHA256
			digest = &v
		}
		c.Agents = append(c.Agents, api.Agent{AgentID: p.AgentID, AdapterID: p.AdapterID, InstalledVersion: p.Version, BinarySHA256: digest, InferenceLocation: p.Location, Authentication: "unknown", Operations: ops, Features: map[string]any{"activity_events": true, "native_text_delta": false, "native_json_schema": false, "embedding": false, "tool_approval": "deny_only"}, Policies: []string{p.PolicyID}})
	}
	return c
}

// Environment never inherits arbitrary API keys, bearer tokens, proxies,
// loader/shell injection or app credentials. Auth roots need an explicit profile.
func Environment(parent []string, home string) []string {
	out := []string{"HOME=" + home, "PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C.UTF-8"}
	_ = parent
	return out
}
func MarshalConfig(c Config) []byte { b, _ := json.Marshal(c); return b }
