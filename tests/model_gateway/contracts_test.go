package model_gateway_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Garden12138/xcli/internal/model"
	"github.com/Garden12138/xcli/internal/modelbackend/fake"
	"github.com/Garden12138/xcli/internal/modelgateway"
)

func obj(t *testing.T, s string) model.Object {
	t.Helper()
	var o model.Object
	if err := json.Unmarshal([]byte(s), &o); err != nil {
		t.Fatal(err)
	}
	return o
}

// Actual Python HTTP consumers exercise the four app-shaped interfaces. This is
// deliberately not named application E2E: external application code is not run.
func TestDesktopConsumerProfiles(t *testing.T) {
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	imageData, _ := json.Marshal(map[string]any{"created": 1, "data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(imageBytes.Bytes())}}})
	fixture := &fake.Backend{
		CompleteFunc: func(_ context.Context, r model.Object) (model.Object, error) {
			if r["tools"] != nil {
				return obj(t, `{"id":"fixture","object":"chat.completion","created":1,"choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_fixture","type":"function","function":{"name":"synthetic_decision","arguments":"{\"ok\":true}"}}]},"finish_reason":"tool_calls"}]}`), nil
			}
			return obj(t, `{"id":"fixture","object":"chat.completion","created":1,"choices":[{"index":0,"message":{"role":"assistant","content":"{\"ok\":true}"},"finish_reason":"stop"}]}`), nil
		},
		StreamFunc: func(_ context.Context, r model.Object, send func(model.Object) error) error {
			for _, chunk := range []string{`{"id":"fixture","object":"chat.completion.chunk","created":1,"choices":[{"index":0,"delta":{"role":"assistant","content":"synthetic"},"finish_reason":null}]}`, `{"id":"fixture","object":"chat.completion.chunk","created":1,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`} {
				if err := send(obj(t, chunk)); err != nil {
					return err
				}
			}
			return nil
		},
		GenerateFunc: func(context.Context, model.Object) (model.Object, error) { return obj(t, string(imageData)), nil },
		EditFunc: func(context.Context, model.ImageEditRequest) (model.Object, error) {
			return obj(t, string(imageData)), nil
		},
		EmbedFunc: func(context.Context, model.Object) (model.Object, error) {
			return obj(t, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]},{"object":"embedding","index":1,"embedding":[0.4,0.5,0.6]}]}`), nil
		},
	}
	meta := func(kind string, caps ...model.Capability) model.Metadata {
		return model.Metadata{Kind: kind, Capabilities: caps, InferenceLocation: "local", Verification: "probe_verified", VerifiedAt: "2026-10-25T00:00:00Z", Evidence: "synthetic-offline-only"}
	}
	embed := meta("embedding", model.Embeddings)
	embed.Dimensions = 3
	embed.IndexVersion = "fixture-v1"
	embed.IndexFingerprint = "fixture-identity"
	registry, err := model.NewRegistry([]model.Descriptor{
		{ID: "xcli/text-default", BackendID: "fixture", UpstreamModel: "fixture-text", Metadata: meta("text", model.Text, model.Stream, model.JSONObject, model.JSONSchema, model.Tools)},
		{ID: "xcli/image-default", BackendID: "fixture", UpstreamModel: "fixture-image", Metadata: meta("image", model.ImageGenerate, model.ImageEdit)},
		{ID: "xcli/embed-default", BackendID: "fixture", UpstreamModel: "fixture-vector", Metadata: embed},
	})
	if err != nil {
		t.Fatal(err)
	}
	scopes := map[string][]string{"worklog": {"xcli/text-default"}, "garden-flow": {"xcli/text-default", "xcli/image-default"}, "chat-insight": {"xcli/text-default", "xcli/embed-default"}, "contribution-os": {"xcli/text-default"}}
	server, err := modelgateway.New(modelgateway.Options{Registry: registry, Chats: map[string]model.ChatBackend{"fixture": fixture}, Images: map[string]model.ImageBackend{"fixture": fixture}, Embeddings: map[string]model.EmbeddingBackend{"fixture": fixture}, Authenticate: func(token string) (modelgateway.Client, error) {
		scope, ok := scopes[token]
		if !ok {
			return modelgateway.Client{}, model.Failure(401, "invalid_api_key", "invalid fixture identity")
		}
		return modelgateway.Client{ID: token, AllowedModels: scope}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	for _, profile := range []string{"worklog", "garden-flow", "chat-insight", "contribution-os"} {
		t.Run(profile, func(t *testing.T) {
			command := exec.Command("python3", filepath.Join("..", "..", "examples", "model-gateway", "smoke.py"), "--base-url", httpServer.URL+"/v1", "--profile", profile, "--execute", "--max-model-requests", "3", "--image-size", "2x2")
			command.Env = append(os.Environ(), "XCLI_APP_TOKEN="+profile)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %v %s", profile, err, output)
			}
			var report map[string]any
			if json.Unmarshal(output, &report) != nil || report["application_e2e"] != "unverified" {
				t.Fatalf("invalid evidence report: %s", output)
			}
		})
	}
}
