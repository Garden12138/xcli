package modelgateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Garden12138/xcli/internal/model"
	"github.com/Garden12138/xcli/internal/modelbackend"
	"github.com/Garden12138/xcli/internal/modelbackend/fake"
)

func pngFixture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func imageResult(data []byte) string {
	b, _ := json.Marshal(map[string]any{"created": 1, "data": []any{map[string]string{"b64_json": base64.StdEncoding.EncodeToString(data)}}})
	return string(b)
}

func imageServer(t *testing.T, b model.ImageBackend, caps ...model.Capability) *Server {
	t.Helper()
	registry, err := model.NewRegistry([]model.Descriptor{{ID: "image", BackendID: "image-backend", UpstreamModel: "real-image-model", Metadata: model.Metadata{Kind: "image", Capabilities: caps, Verification: "probe_verified", VerifiedAt: "2026-10-25T00:00:00Z", Evidence: "offline-fixtures-only", InferenceLocation: "local"}}})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{Registry: registry, Images: map[string]model.ImageBackend{"image-backend": b}, Authenticate: func(string) (Client, error) { return Client{ID: "garden-flow", AllowedModels: []string{"image"}}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func imagePost(t *testing.T, s http.Handler, path, typ string, body *bytes.Buffer) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "http://127.0.0.1:18741"+path, body)
	req.Header.Set("Content-Type", typ)
	req.Header.Set("Authorization", "Bearer fixture-token")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func editBody(t *testing.T, count int, mask bool, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for key, value := range map[string]string{"model": "image", "prompt": "synthetic edit", "size": "2x2", "response_format": "b64_json"} {
		if err := w.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range fields {
		if err := w.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < count; i++ {
		part, err := w.CreateFormFile("image[]", "../../ignored.png")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(pngFixture(t))
	}
	if mask {
		part, _ := w.CreateFormFile("mask", "mask.png")
		_, _ = part.Write(pngFixture(t))
	}
	_ = w.Close()
	return &b, w.FormDataContentType()
}

func TestImageGenerationAndEditsThroughCompatibleUpstream(t *testing.T) {
	var paths []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "generations") {
			var req model.Object
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.String("model") != "real-image-model" {
				t.Error(req, err)
			}
		} else {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			defer r.MultipartForm.RemoveAll()
			if r.FormValue("model") != "real-image-model" || len(r.MultipartForm.File["image"]) != 1 || len(r.MultipartForm.File["mask"]) != 1 {
				t.Error(r.MultipartForm)
			}
		}
		fmt.Fprint(w, imageResult(pngFixture(t)))
	}))
	defer upstream.Close()
	b, err := modelbackend.NewOpenAI(upstream.URL+"/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	s := imageServer(t, b, model.ImageGenerate, model.ImageEdit, model.ImageMask)
	w := imagePost(t, s, "/v1/images/generations", "application/json", bytes.NewBufferString(`{"model":"image","prompt":"cover fixture","size":"2x2","n":1,"response_format":"b64_json"}`))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	body, typ := editBody(t, 1, true, nil)
	w = imagePost(t, s, "/v1/images/edits", typ, body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(paths) != 2 {
		t.Fatal(paths)
	}
}

func TestImageCapabilityAndResultFailures(t *testing.T) {
	s := imageServer(t, nil, model.ImageEdit)
	for _, tc := range []struct {
		count int
		mask  bool
	}{{1, true}, {2, false}} {
		body, typ := editBody(t, tc.count, tc.mask, nil)
		w := imagePost(t, s, "/v1/images/edits", typ, body)
		if w.Code != 422 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, result := range []string{`{"created":1,"data":[{"url":"http://169.254.169.254/secret"}]}`, `{"created":1,"data":[{"b64_json":"ZmFrZQ=="}]}`, `{"created":1,"data":[]}`} {
		if err := validateImages(object(t, result), 1, "2x2"); err == nil {
			t.Fatal("invalid upstream image accepted")
		}
	}
	if err := validateImages(object(t, imageResult(pngFixture(t))), 1, "3x3"); err == nil {
		t.Fatal("wrong dimensions accepted")
	}
	for _, bad := range []string{`{"model":"image","prompt":"x","response_format":"url"}`, `{"model":"image","prompt":"x","mask":"ignored"}`} {
		w := imagePost(t, s, "/v1/images/generations", "application/json", bytes.NewBufferString(bad))
		if w.Code != 422 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	body, typ := editBody(t, 1, false, map[string]string{"unknown": "value"})
	if w := imagePost(t, s, "/v1/images/edits", typ, body); w.Code != 422 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestVisionInputRequiresVerifiedCapabilityAndInlineImage(t *testing.T) {
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngFixture(t))
	request := func(url string) model.Object {
		b, _ := json.Marshal(map[string]any{"model": "xcli/text-default", "messages": []any{map[string]any{"role": "user", "content": []any{map[string]string{"type": "text", "text": "describe fixture"}, map[string]any{"type": "image_url", "image_url": map[string]string{"url": url}}}}}})
		return object(t, string(b))
	}
	caps, _, err := validateChat(request(valid))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = testRegistry(t).Resolve("xcli/text-default", caps...); err == nil {
		t.Fatal("unverified Vision routed")
	}
	for _, url := range []string{"http://127.0.0.1/secret", "file:///private/path", "https://example.com/image"} {
		if _, _, err = validateChat(request(url)); err == nil {
			t.Fatal("unsafe URL accepted")
		}
	}
	registry, err := model.NewRegistry([]model.Descriptor{{ID: "xcli/text-default", BackendID: "vision", UpstreamModel: "upstream-vision", Metadata: model.Metadata{Kind: "text", Capabilities: []model.Capability{model.Text, model.Vision}, Verification: "probe_verified", VerifiedAt: "2026-10-25T00:00:00Z", Evidence: "offline-vision-fixture", InferenceLocation: "local"}}})
	if err != nil {
		t.Fatal(err)
	}
	called := false
	b := &fake.Backend{CompleteFunc: func(_ context.Context, req model.Object) (model.Object, error) {
		called = true
		if req.String("model") != "upstream-vision" || !bytes.Contains(req["messages"], []byte(valid)) {
			t.Error("vision input changed")
		}
		return object(t, completionBody), nil
	}}
	s, err := New(Options{Registry: registry, Chats: map[string]model.ChatBackend{"vision": b}, Authenticate: func(string) (Client, error) {
		return Client{ID: "vision-client", AllowedModels: []string{"xcli/text-default"}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(request(valid))
	if w := post(t, s, string(data)); w.Code != 200 || !called {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestImageMultiReferenceAndInvalidUpload(t *testing.T) {
	called := false
	b := &fake.Backend{EditFunc: func(_ context.Context, req model.ImageEditRequest) (model.Object, error) {
		called = true
		if len(req.Images) != 2 || req.Mask != nil || req.Fields["model"] != "real-image-model" {
			t.Error(req)
		}
		return object(t, imageResult(pngFixture(t))), nil
	}}
	s := imageServer(t, b, model.ImageEdit, model.ImageReference)
	body, typ := editBody(t, 2, false, nil)
	w := imagePost(t, s, "/v1/images/edits", typ, body)
	if w.Code != 200 || !called {
		t.Fatal(w.Code, w.Body.String())
	}
	called = false
	var invalid bytes.Buffer
	writer := multipart.NewWriter(&invalid)
	_ = writer.WriteField("model", "image")
	_ = writer.WriteField("prompt", "synthetic")
	file, _ := writer.CreateFormFile("image", "../../not-read.png")
	_, _ = file.Write([]byte("fake image"))
	_ = writer.Close()
	w = imagePost(t, s, "/v1/images/edits", writer.FormDataContentType(), &invalid)
	if w.Code != 422 || called {
		t.Fatal(w.Code, w.Body.String())
	}
}
