// Package modelstore owns gateway configuration and private local state.
package modelstore

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/model"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Version  int                `yaml:"version"`
	Server   Server             `yaml:"server"`
	Backends map[string]Backend `yaml:"backends"`
	Models   map[string]Model   `yaml:"models"`
}
type Server struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	MaxConcurrent  int    `yaml:"max_concurrent_requests"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}
type Backend struct {
	Type              string `yaml:"type"`
	Endpoint          string `yaml:"endpoint"`
	APIKeyRef         string `yaml:"api_key_ref"`
	InferenceLocation string `yaml:"inference_location"`
}
type Model struct {
	Backend        string `yaml:"backend"`
	UpstreamModel  string `yaml:"upstream_model"`
	model.Metadata `yaml:",inline"`
}

func DefaultPath() (string, error) {
	p, err := config.DefaultPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "model-gateway.yaml"), nil
}

func Load(path string) (Config, string, error) {
	var err error
	if path == "" {
		path, err = DefaultPath()
	}
	if err != nil {
		return Config{}, "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Config{}, "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, path, fmt.Errorf("read model gateway config: %w", err)
	}
	c, err := Decode(b)
	return c, path, err
}

func Decode(b []byte) (Config, error) {
	c := Config{Server: Server{Host: "127.0.0.1", Port: 18741, MaxConcurrent: 3, TimeoutSeconds: 180}}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("invalid gateway YAML: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return Config{}, fmt.Errorf("gateway config must contain exactly one YAML document")
	}
	if c.Version != 1 {
		return Config{}, fmt.Errorf("unsupported gateway config version")
	}
	ip := net.ParseIP(c.Server.Host)
	if ip == nil || !ip.IsLoopback() {
		return Config{}, fmt.Errorf("gateway host must be a loopback IP literal")
	}
	if c.Server.Port < 0 || c.Server.Port > 65535 || c.Server.MaxConcurrent < 1 || c.Server.MaxConcurrent > 64 || c.Server.TimeoutSeconds < 1 || c.Server.TimeoutSeconds > 3600 {
		return Config{}, fmt.Errorf("invalid gateway server limits")
	}
	for _, b := range c.Backends {
		if b.Type != "openai-compatible" && b.Type != "openai-images" && b.Type != "openai-compatible-embeddings" {
			return Config{}, fmt.Errorf("unsupported backend type (native CLI bridge is disabled)")
		}
		u, err := url.Parse(b.Endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
			return Config{}, fmt.Errorf("invalid upstream endpoint")
		}
		local := net.ParseIP(u.Hostname())
		isLocal := local != nil && local.IsLoopback()
		if u.Scheme == "http" && !isLocal {
			return Config{}, fmt.Errorf("upstream HTTP requires a loopback IP literal")
		}
		if b.InferenceLocation != "local" && b.InferenceLocation != "remote" && b.InferenceLocation != "unknown" {
			return Config{}, fmt.Errorf("backend inference_location is required")
		}
		if b.InferenceLocation == "local" && !isLocal {
			return Config{}, fmt.Errorf("local inference requires a loopback upstream IP")
		}
	}
	if _, err := c.Registry(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Address() string { return net.JoinHostPort(c.Server.Host, strconv.Itoa(c.Server.Port)) }

func (c Config) Registry() (*model.Registry, error) {
	descriptors := make([]model.Descriptor, 0, len(c.Models))
	for id, m := range c.Models {
		b, ok := c.Backends[m.Backend]
		if !ok {
			return nil, fmt.Errorf("model %q references missing backend", id)
		}
		m.Metadata.InferenceLocation, m.Metadata.ProviderFamily = b.InferenceLocation, b.Type
		descriptors = append(descriptors, model.Descriptor{ID: id, BackendID: m.Backend, UpstreamModel: m.UpstreamModel, Metadata: m.Metadata})
	}
	return model.NewRegistry(descriptors)
}
