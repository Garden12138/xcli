package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Garden12138/xcli/internal/api"
)

func TestDefaultMatchesDesign(t *testing.T) {
	c := Default()
	if c.Defaults.Permission != api.PermWrite || !c.Defaults.Network || !c.Defaults.Fallback || c.Defaults.Review != "auto" || c.Defaults.Planner != "codex" {
		t.Fatalf("defaults: %+v", c.Defaults)
	}
	if c.Defaults.MaxFixRounds != 2 || c.Defaults.MaxSteps != 6 || c.Defaults.MaxParallel != 3 || c.Defaults.StepTimeout.D() != 20*time.Minute || c.Defaults.TaskTimeout.D() != 90*time.Minute || c.Defaults.PlannerTimeout.D() != 3*time.Minute || c.Defaults.VerifyTimeout.D() != 10*time.Minute {
		t.Fatalf("limits: %+v", c.Defaults)
	}
	for name, command := range map[string]string{"codex": "codex", "cursor": "cursor-agent", "antigravity": "agy"} {
		if c.Agents[name].Command != command || !c.Enabled(name) || c.Agents[name].MaxConcurrency != 2 {
			t.Errorf("%s: %+v", name, c.Agents[name])
		}
	}
	if c.Strength("codex", "image") != 3 || c.Strength("antigravity", "image") != 2 || c.Strength("cursor", "image") != 1 || c.Strength("cursor", "review") != 3 || c.Enabled("unknown") {
		t.Fatal("agent strengths or availability")
	}
	for _, name := range AgentOrder {
		if c.Strength(name, "ask") != 2 {
			t.Errorf("%s ask strength = %d, want 2", name, c.Strength(name, "ask"))
		}
	}
}

func TestParseMergesFieldByField(t *testing.T) {
	c, err := Parse([]byte("defaults:\n  network: false\n  max_fix_rounds: 0\nagents:\n  codex:\n    model: gpt-5.5\n    strengths: {image: 0}\n    env: {EXAMPLE: value}\n    artifact_dirs: []\n  cursor:\n    enabled: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Defaults.Network || c.Defaults.MaxSteps != 6 || c.Defaults.MaxFixRounds != 0 {
		t.Fatalf("defaults merge: %+v", c.Defaults)
	}
	if c.Agents["codex"].Command != "codex" || c.Agents["codex"].Model != "gpt-5.5" || c.Agents["codex"].Env["EXAMPLE"] != "value" || len(c.Agents["codex"].ArtifactDirs) != 0 {
		t.Fatalf("agent merge: %+v", c.Agents["codex"])
	}
	if c.Strength("codex", "image") != 0 || c.Strength("codex", "code") != 3 || c.Enabled("cursor") || !c.Enabled("antigravity") {
		t.Fatal("strength or enabled merge")
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	for _, tc := range []struct{ input, field string }{
		{"defaults: {bogus: 1}", "bogus"},
		{"agents: {codex: {bogus: 1}}", "bogus"},
		{"agents: {claude: {}}", "可选 codex/cursor/antigravity"},
		{"defaults: {permission: other}", "permission"},
		{"defaults: {review: other}", "review"},
		{"defaults: {max_parallel: 0}", "max_parallel"},
		{"defaults: {max_fix_rounds: -1}", "max_fix_rounds"},
		{"defaults: {step_timeout: 0s}", "step_timeout"},
		{"defaults: {planner: other}", "planner"},
		{"agents: {codex: {max_concurrency: 0}}", "max_concurrency"},
		{"agents: {codex: {command: ''}}", "command"},
		{"defaults: {task_timeout: nonsense}", "nonsense"},
		{"defaults: {network: false}\n---\npath: /extra", "YAML"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			_, err := Parse([]byte(tc.input))
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error = %v, want %s", err, tc.field)
			}
		})
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil || !reflect.DeepEqual(c, Default()) {
		t.Fatalf("missing config: %+v, %v", c, err)
	}
	if _, err := Parse(nil); err != nil {
		t.Fatal(err)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	c := Default()
	raw, err := Marshal(c)
	if err != nil || !strings.Contains(string(raw), "step_timeout: 20m0s") {
		t.Fatalf("marshal: %s, %v", raw, err)
	}
	got, err := Parse(raw)
	if err != nil || !reflect.DeepEqual(c, got) {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	got, err = Load(home)
	if err != nil || !reflect.DeepEqual(c, got) {
		t.Fatalf("load: %+v, %v", got, err)
	}
}

func TestNativeArtifactDirs(t *testing.T) {
	t.Setenv("CODEX_HOME", "/native/codex")
	c := Default()
	if !reflect.DeepEqual(c.Agents["codex"].ArtifactDirs, []string{"/native/codex/generated_images"}) || !reflect.DeepEqual(c.Agents["cursor"].ArtifactDirs, []string{"~/.cursor/projects"}) || !reflect.DeepEqual(c.Agents["antigravity"].ArtifactDirs, []string{"~/.gemini/antigravity-cli/brain"}) {
		t.Fatal("artifact directories differ from driver notes")
	}
}
