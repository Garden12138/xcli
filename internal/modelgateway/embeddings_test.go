package modelgateway

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Garden12138/xcli/internal/model"
	"github.com/Garden12138/xcli/internal/modelbackend"
)

const embeddingBody = `{"model":"embed","input":["synthetic one","synthetic two"],"encoding_format":"float"}`
const embeddingResult = `{"object":"list","model":"upstream-embed","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]},{"object":"embedding","index":1,"embedding":[0.4,0.5,0.6]}],"usage":{"prompt_tokens":4,"total_tokens":4}}`

func embeddingServer(t *testing.T, b model.EmbeddingBackend, location string, client Client) *Server {
	t.Helper()
	registry, err := model.NewRegistry([]model.Descriptor{{ID: "embed", BackendID: "embedding-backend", UpstreamModel: "upstream-embed", Metadata: model.Metadata{Kind: "embedding", Capabilities: []model.Capability{model.Embeddings}, InferenceLocation: location, Dimensions: 3, IndexVersion: "fixture-v1", IndexFingerprint: "fixture-fingerprint"}}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{Registry: registry, Embeddings: map[string]model.EmbeddingBackend{"embedding-backend": b}, Authenticate: func(string) (Client, error) { return client, nil }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func embeddingPost(t *testing.T, s http.Handler, consent bool, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "http://127.0.0.1:18741/v1/embeddings", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer fixture")
	if consent {
		r.Header.Set("X-Xcli-Remote-Consent", "allow")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestEmbeddingPassthroughAndCloudAuthorization(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/embeddings" {
			t.Error(r.URL.Path)
		}
		var req model.Object
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.String("model") != "upstream-embed" {
			t.Error(req, err)
		}
		if r.Header.Get("X-Xcli-Remote-Consent") != "" {
			t.Error("consent header leaked")
		}
		fmt.Fprint(w, embeddingResult)
	}))
	defer upstream.Close()
	b, err := modelbackend.NewOpenAI(upstream.URL+"/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	client := Client{ID: "chat-insight", AllowedModels: []string{"embed"}, AllowRemote: true, RequireConsent: true}
	for _, location := range []string{"remote", "unknown"} {
		s := embeddingServer(t, b, location, client)
		before := calls.Load()
		w := embeddingPost(t, s, false, embeddingBody)
		if w.Code != 403 || calls.Load() != before {
			t.Fatal("cloud denial leaked data", w.Code, w.Body.String())
		}
		w = embeddingPost(t, s, true, embeddingBody)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		out := object(t, w.Body.String())
		if out.String("model") != "embed" || !bytes.Equal(out["data"], object(t, embeddingResult)["data"]) {
			t.Fatal("vectors mutated", w.Body.String())
		}
		var meta model.Object
		if json.Unmarshal(out["xcli"], &meta) != nil || meta.String("inference_location") != location {
			t.Fatal(meta)
		}
	}
	client.AllowRemote = false
	s := embeddingServer(t, b, "remote", client)
	before := calls.Load()
	if w := embeddingPost(t, s, true, embeddingBody); w.Code != 403 || calls.Load() != before {
		t.Fatal(w.Code)
	}
	s = embeddingServer(t, b, "local", client)
	if w := embeddingPost(t, s, false, embeddingBody); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	s = embeddingServer(t, nil, "local", client)
	if w := embeddingPost(t, s, false, embeddingBody); w.Code != 503 {
		t.Fatal("missing embedding service fabricated vectors", w.Code)
	}
}

func TestEmbeddingInputAndResultValidation(t *testing.T) {
	for _, tc := range []struct {
		input string
		count int
	}{{`"one"`, 1}, {`["a","b"]`, 2}, {`[1,2,3]`, 1}, {`[[1,2],[3]]`, 2}} {
		count, err := embeddingCount(json.RawMessage(tc.input))
		if err != nil || count != tc.count {
			t.Fatal(tc, count, err)
		}
	}
	for _, bad := range []string{`null`, `[]`, `""`, `[""]`, `["a",1]`, `[-1]`, `[[]]`} {
		if _, err := embeddingCount(json.RawMessage(bad)); err == nil {
			t.Fatal("invalid input", bad)
		}
	}
	for _, bad := range []string{
		strings.Replace(embeddingResult, `"index":1`, `"index":0`, 1),
		strings.Replace(embeddingResult, `[0.1,0.2,0.3]`, `[0.1]`, 1),
		strings.Replace(embeddingResult, `0.1`, `null`, 1),
	} {
		if err := validateEmbeddings(object(t, bad), 2, 3, "float"); err == nil {
			t.Fatal("invalid vector response accepted")
		}
	}
	encoded := make([]byte, 12)
	for i, v := range []float32{0.1, 0.2, 0.3} {
		binary.LittleEndian.PutUint32(encoded[i*4:], math.Float32bits(v))
	}
	result := fmt.Sprintf(`{"object":"list","data":[{"index":0,"object":"embedding","embedding":%q}]}`, base64.StdEncoding.EncodeToString(encoded))
	if err := validateEmbeddings(object(t, result), 1, 3, "base64"); err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(encoded, math.Float32bits(float32(math.Inf(1))))
	result = fmt.Sprintf(`{"object":"list","data":[{"index":0,"object":"embedding","embedding":%q}]}`, base64.StdEncoding.EncodeToString(encoded))
	if err := validateEmbeddings(object(t, result), 1, 3, "base64"); err == nil {
		t.Fatal("infinite vector accepted")
	}
}
