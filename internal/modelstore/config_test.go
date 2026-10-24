package modelstore

import (
	"strings"
	"testing"
)

const fixtureConfig = `version: 1
backends:
  main:
    type: openai-compatible
    endpoint: http://127.0.0.1:11434/v1
    inference_location: local
models:
  xcli/text-default:
    backend: main
    upstream_model: test-model
    kind: text
    capabilities: [text.generate, text.stream]
`

func TestConfigValidatesBoundaries(t *testing.T) {
	c, err := Decode([]byte(fixtureConfig))
	if err != nil {
		t.Fatal(err)
	}
	if c.Address() != "127.0.0.1:18741" {
		t.Fatal(c.Address())
	}
	for _, bad := range []string{
		fixtureConfig + "unknown: true\n", fixtureConfig + "---\nversion: 1\n",
		strings.Replace(fixtureConfig, "version: 1", "version: 2", 1),
		fixtureConfig + "server:\n  host: 0.0.0.0\n",
		strings.Replace(fixtureConfig, "http://127.0.0.1:11434/v1", "http://example.com/v1", 1),
		strings.Replace(fixtureConfig, "http://127.0.0.1:11434/v1", "https://example.com/v1", 1),
		strings.Replace(fixtureConfig, "openai-compatible", "native-cli-text", 1),
		strings.Replace(fixtureConfig, "backend: main", "backend: missing", 1),
	} {
		if _, err := Decode([]byte(bad)); err == nil {
			t.Fatalf("invalid config accepted: %s", bad)
		}
	}
}
