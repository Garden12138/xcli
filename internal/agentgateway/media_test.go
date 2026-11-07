package agentgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

type galleryDriver struct {
	bytes []byte
	calls int
}

func (d *galleryDriver) StartTurn(_ context.Context, b nativeadapter.BoundRequest, _ func(nativeadapter.Observation) error) nativeadapter.TurnResult {
	d.calls++
	_ = os.WriteFile(filepath.Join(b.OutputRoot, "image.png"), d.bytes, 0o600)
	zero := 0
	return nativeadapter.TurnResult{Text: "offline gallery fixture", NativeID: "gallery-native", SessionObserved: true, ExitCode: &zero, Terminal: "success", Protocol: "complete", LocalStopConfirmed: true, Candidates: []nativeadapter.Candidate{{RelativePath: "image.png"}}}
}
func TestGardenFlowStyleHTTPPreviewReviewDownload(t *testing.T) {
	s, _ := setup(t)
	c, _ := s.Service.Clients.Get("worklog")
	c.ID = "gardenflow"
	c.Operations = []string{"text_task", "image_generate", "image_edit"}
	token, e := s.Service.Clients.Issue(c, false)
	if e != nil {
		t.Fatal(e)
	}
	p := s.Service.Profiles[0]
	p.Operations = c.Operations
	s.Service.Profiles[0] = p.Freeze()
	var data bytes.Buffer
	_ = png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 4, 3)))
	d := &galleryDriver{bytes: data.Bytes()}
	w := call(s, token, "POST", api.Prefix+"/sessions", "gallery-session", encode(api.SessionCreate{AgentID: p.AgentID, AdapterID: p.AdapterID, PolicyID: p.PolicyID, CapabilityRevision: p.CapabilityRevision, Budget: api.Budget{MaxDispatches: 2, MaxProcessStarts: 2, MaxExecutionSeconds: 300}, Context: api.Context{App: "gardenflow", Workflow: "synthetic-media", OpaqueRef: "offline"}}))
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var ses api.Session
	_ = json.Unmarshal(w.Body.Bytes(), &ses)
	// Raw input upload has hash/owner binding and no native start.
	r := httptest.NewRequest("POST", api.Prefix+"/inputs", bytes.NewReader(d.bytes))
	r.Host = s.Address
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Idempotency-Key", "gallery-input")
	r.Header.Set("Content-Type", "image/png")
	r.Header.Set("X-Xcli-Session-ID", ses.ID)
	r.Header.Set("X-Xcli-Content-SHA256", api.Hash(d.bytes))
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 201 || d.calls != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	var input api.Input
	_ = json.Unmarshal(w.Body.Bytes(), &input)
	text := "synthetic edit fixture"
	req := api.TaskCreate{SessionID: ses.ID, ExpectedSessionRevision: ses.Revision, Operation: "image_edit", Input: []api.Part{{Type: "text", Text: &text}, {Type: "file", InputID: &input.ID}}, Budget: api.Budget{MaxDispatches: 1, MaxProcessStarts: 1, TimeoutSeconds: 120}, Consent: api.Consent{AllowRemote: true, ContextSHA256: ses.ContextSHA256}}
	w = call(s, token, "POST", api.Prefix+"/tasks", "gallery-task", encode(req))
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var task api.Task
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	if _, e = s.Service.ExecuteNext(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	w = call(s, token, "GET", api.Prefix+"/tasks/"+task.ID, "", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	if task.Status != "awaiting_review" {
		t.Fatal(w.Body.String())
	}
	id := task.Result.Artifacts[0]
	w = call(s, token, "GET", api.Prefix+"/artifacts/"+id+"/content", "", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || api.Hash(w.Body.Bytes()) != input.SHA256 {
		t.Fatal(w.Code, w.Body.String())
	}
	review := api.Review{Decision: "accept", ArtifactID: id, ExpectedSHA256: input.SHA256, SemanticCheck: "edit_matches_requested_change", ProvenanceAttestation: &api.Attestation{Source: "native_task_trace", EvidenceSHA256: api.Hash([]byte("synthetic gallery trace"))}}
	w = call(s, token, "POST", api.Prefix+"/tasks/"+task.ID+"/review", "gallery-review", encode(review))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &task)
	if task.Status != "succeeded" || d.calls != 1 {
		t.Fatal(task, d.calls)
	}
	w = call(s, token, "GET", api.Prefix+"/artifacts/"+id, "", nil)
	var artifact api.Artifact
	_ = json.Unmarshal(w.Body.Bytes(), &artifact)
	if artifact.DeliveryState != "ready" || artifact.ReferenceSHA256 == nil || *artifact.ReferenceSHA256 != input.SHA256 {
		t.Fatal(artifact)
	}
	w = call(s, token, "GET", api.Prefix+"/artifacts/"+id+"/content", "", nil)
	if api.Hash(w.Body.Bytes()) != artifact.SHA256 || w.Header().Get("ETag") != "\""+artifact.SHA256+"\"" {
		t.Fatal("delivery hash mismatch")
	}
}
