package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
)

func TestNormalize(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "input.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	schema := `{"type":"object","properties":{"answer":{"type":"string"}}}`
	if err := os.WriteFile(filepath.Join(cwd, "schema.json"), []byte(schema), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		req     api.Request
		invalid bool
	}{
		{"defaults", api.Request{Goal: "x"}, false},
		{"ask", api.Request{Goal: "x", Capability: api.CapAsk, Permission: api.PermFull}, false},
		{"review", api.Request{Goal: "x", Capability: api.CapReview, Workdir: "."}, false},
		{"code", api.Request{Goal: "x", Capability: api.CapCode, Workdir: ".", Permission: api.PermFull}, false},
		{"read code", api.Request{Goal: "x", Capability: api.CapCode, Workdir: ".", Permission: api.PermRead}, true},
		{"read image", api.Request{Goal: "x", Capability: api.CapImage, Permission: api.PermRead}, true},
		{"read auto", api.Request{Goal: "x", Permission: api.PermRead}, false},
		{"paths", api.Request{Goal: "x", Workdir: ".", Inputs: []string{"input.txt"}, Images: []string{"input.txt"}, OutputDir: "new/out"}, false},
		{"missing workdir", api.Request{Goal: "x", Workdir: "missing"}, true},
		{"file workdir", api.Request{Goal: "x", Workdir: "input.txt"}, true},
		{"missing input", api.Request{Goal: "x", Inputs: []string{"missing"}}, true},
		{"missing image", api.Request{Goal: "x", Images: []string{"missing"}}, true},
		{"code needs workdir", api.Request{Goal: "x", Capability: api.CapCode}, true},
		{"review needs workdir", api.Request{Goal: "x", Capability: api.CapReview}, true},
		{"unknown agent", api.Request{Goal: "x", Agent: "other"}, true},
		{"disabled agent", api.Request{Goal: "x", Agent: "cursor"}, true},
		{"unknown allowed", api.Request{Goal: "x", Agents: []string{"other"}}, true},
		{"unknown model agent", api.Request{Goal: "x", Models: map[string]string{"other": "m"}}, true},
		{"schema file", api.Request{Goal: "x", Capability: api.CapAsk, SchemaFile: "schema.json"}, false},
		{"bad schema", api.Request{Goal: "x", Schema: json.RawMessage(`{"type":"made-up"}`)}, true},
		{"bad version", api.Request{Goal: "x", Version: 99}, true},
		{"no goal", api.Request{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Default()
			off := false
			a := cfg.Agents["cursor"]
			a.Enabled = &off
			cfg.Agents["cursor"] = a
			e := &Engine{Config: cfg}
			r, err := e.Normalize(tc.req, cwd)
			if (err != nil) != tc.invalid {
				t.Fatalf("request=%+v error=%v", r, err)
			}
			if err != nil {
				return
			}
			if r.Version != 1 || r.Capability == "" || r.TimeoutSec != 5400 {
				t.Fatalf("defaults: %+v", r)
			}
			want := tc.req.Permission
			if want == "" {
				want = api.PermWrite
			}
			if r.Capability == api.CapAsk || r.Capability == api.CapReview {
				want = api.PermRead
			}
			if r.Permission != want {
				t.Fatalf("permission=%s, want %s", r.Permission, want)
			}
			for _, path := range append(append([]string{}, r.Inputs...), r.Images...) {
				if !filepath.IsAbs(path) {
					t.Fatal(path)
				}
			}
			if r.Workdir != "" && !filepath.IsAbs(r.Workdir) {
				t.Fatal(r.Workdir)
			}
			if r.OutputDir != "" && !filepath.IsAbs(r.OutputDir) {
				t.Fatal(r.OutputDir)
			}
			if tc.req.SchemaFile != "" && (string(r.Schema) != schema || r.SchemaFile != "") {
				t.Fatalf("schema: %+v", r)
			}
		})
	}
}
