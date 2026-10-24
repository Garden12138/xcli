package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRegistryHardFilteringAndSafeMetadata(t *testing.T) {
	d := Descriptor{ID: "xcli/text-default", BackendID: "private-backend", UpstreamModel: "private-upstream", Metadata: Metadata{Kind: "text", Capabilities: []Capability{Text, Stream, Tools}, InferenceLocation: "remote"}}
	r, err := NewRegistry([]Descriptor{d})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(d.ID, Text, Stream); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(d.ID, Tools); err == nil {
		t.Fatal("unverified tools must be rejected")
	}
	if _, err := r.Resolve("unknown", Text); err == nil {
		t.Fatal("unknown model accepted")
	}
	if _, err := r.Resolve(d.ID, ImageGenerate); err == nil {
		t.Fatal("silent image fallback")
	}
	list := r.List()
	list[0].Metadata.Capabilities[0] = ImageGenerate
	if _, err := r.Resolve(d.ID, Text); err != nil {
		t.Fatal("registry was mutated by caller")
	}
	b, _ := json.Marshal(r.List())
	if strings.Contains(string(b), "private-") {
		t.Fatal("internal routing metadata leaked")
	}
	d.Metadata.Verification = "probe_verified"
	if _, err := NewRegistry([]Descriptor{d}); err == nil {
		t.Fatal("verification without evidence accepted")
	}
	d.Metadata.VerifiedAt = "2026-10-24T00:00:00Z"
	d.Metadata.Evidence = "offline-fixture-v1"
	r, err = NewRegistry([]Descriptor{d})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Resolve(d.ID, Tools); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryRejectsInvalidDeclarations(t *testing.T) {
	for _, cap := range []Capability{"", "shell.execute"} {
		_, err := NewRegistry([]Descriptor{{ID: "a", BackendID: "b", UpstreamModel: "c", Metadata: Metadata{Kind: "text", Capabilities: []Capability{cap}}}})
		if err == nil {
			t.Fatalf("accepted %q", cap)
		}
	}
}
