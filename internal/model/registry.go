package model

import (
	"fmt"
	"slices"
	"sort"
	"time"
)

type Registry struct{ models map[string]Descriptor }

func NewRegistry(descriptors []Descriptor) (*Registry, error) {
	r := &Registry{models: make(map[string]Descriptor, len(descriptors))}
	known := []Capability{Text, Stream, Vision, ImageGenerate, ImageEdit, ImageMask, ImageReference, Embeddings, Tools, JSONObject, JSONSchema}
	for _, d := range descriptors {
		if d.ID == "" || d.BackendID == "" || d.UpstreamModel == "" {
			return nil, fmt.Errorf("model ID, backend and upstream model are required")
		}
		if _, ok := r.models[d.ID]; ok {
			return nil, fmt.Errorf("duplicate model alias %q", d.ID)
		}
		if d.Metadata.Kind != "text" && d.Metadata.Kind != "image" && d.Metadata.Kind != "embedding" {
			return nil, fmt.Errorf("invalid model kind for %q", d.ID)
		}
		if d.Metadata.InferenceLocation == "" {
			d.Metadata.InferenceLocation = "unknown"
		}
		if !slices.Contains([]string{"local", "remote", "unknown"}, d.Metadata.InferenceLocation) {
			return nil, fmt.Errorf("invalid inference location")
		}
		if d.Metadata.Verification == "" {
			d.Metadata.Verification = "declared"
		}
		if !slices.Contains([]string{"declared", "probe_verified", "runtime_observed"}, d.Metadata.Verification) {
			return nil, fmt.Errorf("invalid capability verification")
		}
		d.Metadata.Validated = d.Metadata.Verification != "declared"
		if d.Metadata.Validated {
			if _, err := time.Parse(time.RFC3339, d.Metadata.VerifiedAt); err != nil || d.Metadata.Evidence == "" {
				return nil, fmt.Errorf("verified model %q needs RFC3339 verified_at and evidence", d.ID)
			}
		}
		if len(d.Metadata.Capabilities) == 0 {
			return nil, fmt.Errorf("model %q needs capabilities", d.ID)
		}
		seen := map[Capability]bool{}
		for _, cap := range d.Metadata.Capabilities {
			if !slices.Contains(known, cap) || seen[cap] {
				return nil, fmt.Errorf("unknown or duplicate capability %q", cap)
			}
			seen[cap] = true
		}
		d.Metadata.Capabilities = slices.Clone(d.Metadata.Capabilities)
		d.Metadata.SupportsStream = seen[Stream]
		d.Object, d.OwnedBy = "model", "xcli"
		r.models[d.ID] = d
	}
	return r, nil
}

func (r *Registry) List() []Descriptor {
	out := make([]Descriptor, 0, len(r.models))
	for _, d := range r.models {
		d.Metadata.Capabilities = slices.Clone(d.Metadata.Capabilities)
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Registry) Resolve(id string, required ...Capability) (Descriptor, error) {
	d, ok := r.models[id]
	if !ok {
		return Descriptor{}, Failure(404, "model_not_found", "configured model alias not found")
	}
	for _, cap := range required {
		if !slices.Contains(d.Metadata.Capabilities, cap) {
			return Descriptor{}, Unsupported(cap)
		}
		if cap != Text && cap != Stream && cap != Embeddings && !d.Metadata.Validated {
			return Descriptor{}, Unsupported(cap)
		}
	}
	d.Metadata.Capabilities = slices.Clone(d.Metadata.Capabilities)
	return d, nil
}
