// Package model defines model inference contracts, separate from Agent/Task execution.
package model

import (
	"context"
	"encoding/json"
)

type Capability string

type Client struct {
	ID                string   `json:"id"`
	AllowedModels     []string `json:"allowed_models"`
	AllowRemote       bool     `json:"allow_remote"`
	RequireConsent    bool     `json:"require_consent"`
	RequestsPerMinute int      `json:"requests_per_minute"`
}

const (
	Text           Capability = "text.generate"
	Stream         Capability = "text.stream"
	Vision         Capability = "vision.analyze"
	ImageGenerate  Capability = "image.generate"
	ImageEdit      Capability = "image.edit"
	ImageMask      Capability = "image.mask"
	ImageReference Capability = "image.multi_reference"
	Embeddings     Capability = "embeddings"
	Tools          Capability = "tool_calls"
	JSONObject     Capability = "json_object"
	JSONSchema     Capability = "json_schema"
)

// Metadata is safe for clients. Never add endpoints, credential references or account IDs.
type Metadata struct {
	Kind              string       `json:"kind" yaml:"kind"`
	Capabilities      []Capability `json:"capabilities" yaml:"capabilities"`
	SupportsStream    bool         `json:"supports_stream" yaml:"-"`
	InferenceLocation string       `json:"inference_location" yaml:"-"`
	ProviderFamily    string       `json:"provider_family" yaml:"-"`
	Verification      string       `json:"verification" yaml:"verification"`
	VerifiedAt        string       `json:"verified_at,omitempty" yaml:"verified_at,omitempty"`
	Evidence          string       `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	Validated         bool         `json:"validated" yaml:"-"`
	Dimensions        int          `json:"dimensions,omitempty" yaml:"dimensions,omitempty"`
	IndexVersion      string       `json:"index_version,omitempty" yaml:"index_version,omitempty"`
}

type Descriptor struct {
	ID            string   `json:"id"`
	Object        string   `json:"object"`
	Created       int64    `json:"created"`
	OwnedBy       string   `json:"owned_by"`
	Metadata      Metadata `json:"xcli"`
	BackendID     string   `json:"-"`
	UpstreamModel string   `json:"-"`
}

// Object retains provider-supported optional fields without converting numeric values.
type Object map[string]json.RawMessage

func (o Object) String(key string) string {
	var s string
	_ = json.Unmarshal(o[key], &s)
	return s
}

func (o Object) Bool(key string) bool {
	var b bool
	_ = json.Unmarshal(o[key], &b)
	return b
}

func (o Object) WithModel(id string) Object {
	copy := make(Object, len(o))
	for k, v := range o {
		copy[k] = v
	}
	copy["model"], _ = json.Marshal(id)
	return copy
}

type ChatBackend interface {
	Complete(context.Context, Object) (Object, error)
	Stream(context.Context, Object, func(Object) error) error
}

type ImageInput struct {
	Name string
	MIME string
	Data []byte
}

type ImageEditRequest struct {
	Fields map[string]string
	Images []ImageInput
	Mask   *ImageInput
}

type ImageBackend interface {
	Generate(context.Context, Object) (Object, error)
	Edit(context.Context, ImageEditRequest) (Object, error)
}

type EmbeddingBackend interface {
	Embed(context.Context, Object) (Object, error)
}
