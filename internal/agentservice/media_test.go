package agentservice

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentstore"
	"github.com/Garden12138/xcli/internal/nativeadapter"
)

func imageFixture() []byte {
	im := image.NewRGBA(image.Rect(0, 0, 4, 3))
	im.Set(2, 2, color.RGBA{B: 255, A: 255})
	var out bytes.Buffer
	_ = png.Encode(&out, im)
	return out.Bytes()
}

type imageDriver struct {
	data       []byte
	malicious  string
	provenance bool
	calls      int
}

func (d *imageDriver) StartTurn(_ context.Context, b nativeadapter.BoundRequest, _ func(nativeadapter.Observation) error) nativeadapter.TurnResult {
	d.calls++
	zero := 0
	r := nativeadapter.TurnResult{Text: "fixture image bytes", NativeID: "fixture-image-session", SessionObserved: true, ExitCode: &zero, Terminal: "success", Protocol: "complete", LocalStopConfirmed: true}
	if d.malicious == "only-text" {
		return r
	}
	if d.malicious == "overwrite-input" {
		_ = os.Chmod(b.ReferencePath, 0o600)
		_ = os.WriteFile(b.ReferencePath, []byte("changed"), 0o600)
	}
	name := "result.png"
	_ = os.WriteFile(filepath.Join(b.OutputRoot, name), d.data, 0o600)
	candidate := nativeadapter.Candidate{RelativePath: name}
	if d.malicious == "escape" {
		candidate.RelativePath = "../../outside.png"
	}
	if d.provenance {
		tool := "fixture-native-image-tool"
		hash := api.Hash([]byte("synthetic tool evidence"))
		candidate.Tool = &tool
		candidate.EvidenceSHA256 = &hash
	}
	r.Candidates = []nativeadapter.Candidate{candidate}
	return r
}
func mediaFixture(t *testing.T) (*Service, agentstore.Client, api.Session) {
	s, c, _ := fixture(t)
	p := s.Profiles[0]
	p.Operations = []string{"text_task", "image_generate", "image_edit"}
	s.Profiles[0] = p.Freeze()
	c.Operations = p.Operations // fixtures have no production profile or real model
	c.ID = "gardenflow"
	if _, e := s.Clients.Issue(c, false); e != nil {
		t.Fatal(e)
	}
	ses := createSession(t, s, c, s.Profiles[0])
	return s, c, ses
}
func mediaTask(t *testing.T, s *Service, c agentstore.Client, ses api.Session, operation string, input *api.Input) api.Task {
	t.Helper()
	r := taskRequest(ses)
	r.Operation = operation
	if input != nil {
		r.Input = append(r.Input, api.Part{Type: "file", InputID: &input.ID})
	}
	out, _, e := s.CreateTask(c, r, api.ID("key"))
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestMediaPendingProvenanceReviewAndAuthenticatedBytes(t *testing.T) {
	s, c, ses := mediaFixture(t)
	d := &imageDriver{data: imageFixture()}
	out := mediaTask(t, s, c, ses, "image_generate", nil)
	if _, e := s.ExecuteNext(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	out, _ = s.GetTask(c, out.ID)
	if out.Status != "awaiting_review" || out.Result.Final || len(out.Result.Artifacts) != 1 {
		t.Fatal(out)
	}
	id := out.Result.Artifacts[0]
	meta, data, e := s.ArtifactContent(c, id)
	if e != nil || meta.DeliveryState != "review_only" || api.Hash(data) != meta.SHA256 {
		t.Fatal(meta, e)
	}
	other := c
	other.ID = "other"
	if _, _, e = s.ArtifactContent(other, id); e == nil || e.Error() != "not_found" {
		t.Fatal(e)
	}
	review := api.Review{Decision: "accept", ArtifactID: id, ExpectedSHA256: meta.SHA256, SemanticCheck: "generated_as_requested"}
	regular := c
	regular.Scopes = []string{"task:review", "artifact:read"}
	if _, _, e = s.Review(regular, out.ID, "review-missing-origin", review); e == nil || e.Error() != "provenance_unverified" {
		t.Fatal(e)
	}
	review.ProvenanceAttestation = &api.Attestation{Source: "native_task_trace", EvidenceSHA256: api.Hash([]byte("fixture owner trace review"))}
	out, _, e = s.Review(c, out.ID, "owner-review", review)
	if e != nil || out.Status != "succeeded" || !out.Result.Final || d.calls != 1 {
		t.Fatal(out, e, d.calls)
	}
	if _, replayed, e := s.Review(c, out.ID, "owner-review", review); e != nil || !replayed {
		t.Fatal(e)
	}
	meta, data, e = s.ArtifactContent(c, id)
	if e != nil || meta.DeliveryState != "ready" || meta.Provenance.Method == nil || *meta.Provenance.Method != "task_trace_attestation" || !bytes.Equal(data, d.data) {
		t.Fatal(meta, e)
	}
	events, _, _, _ := s.Events(c, out.ID, 0)
	ready, complete := 0, 0
	for _, event := range events {
		if event.Type == "artifact.ready" {
			ready++
		}
		if event.Type == "task.completed" {
			complete++
		}
	}
	if ready != 1 || complete != 1 {
		t.Fatal(events)
	}
}
func TestImageEditReferenceImmutableAndLateCancellation(t *testing.T) {
	for _, mode := range []string{"valid", "overwrite-input", "cancel", "only-text", "escape"} {
		t.Run(mode, func(t *testing.T) {
			s, c, ses := mediaFixture(t)
			data := imageFixture()
			input, _, e := s.Upload(c, ses.ID, "image/png", api.Hash(data), "reference-image", data)
			if e != nil {
				t.Fatal(e)
			}
			out := mediaTask(t, s, c, ses, "image_edit", &input)
			d := &imageDriver{data: data, malicious: mode, provenance: true}
			if _, e = s.ExecuteNext(context.Background(), d); e != nil {
				t.Fatal(e)
			}
			out, _ = s.GetTask(c, out.ID)
			switch mode {
			case "valid", "cancel":
				if out.Status != "awaiting_review" {
					t.Fatal(out)
				}
				id := out.Result.Artifacts[0]
				meta, _ := s.GetArtifact(c, id)
				if meta.ReferenceSHA256 == nil || *meta.ReferenceSHA256 != input.SHA256 {
					t.Fatal(meta)
				}
				if mode == "cancel" {
					out, _, e = s.Cancel(c, out.ID, "cancel-review")
					if e != nil || out.Status != "canceled" {
						t.Fatal(out, e)
					}
					if _, _, e = s.ArtifactContent(c, id); e == nil {
						t.Fatal("canceled preview accessible")
					}
				} else {
					_, _, e = s.Review(c, out.ID, "accept-edit", api.Review{Decision: "accept", ArtifactID: id, ExpectedSHA256: meta.SHA256, SemanticCheck: "edit_matches_requested_change"})
					if e != nil {
						t.Fatal(e)
					}
				}
			default:
				if out.Status == "succeeded" || out.Status == "awaiting_review" {
					t.Fatal("unsafe media accepted", out)
				}
			}
		})
	}
}
func TestInputOwnershipLimitsAndHashBinding(t *testing.T) {
	s, c, ses := mediaFixture(t)
	data := []byte("selected material")
	if _, _, e := s.Upload(c, ses.ID, "text/plain; charset=utf-8", api.Hash([]byte("other")), "bad-input", data); e == nil {
		t.Fatal("hash mismatch")
	}
	input, _, e := s.Upload(c, ses.ID, "text/plain; charset=utf-8", api.Hash(data), "text-input", data)
	if e != nil {
		t.Fatal(e)
	}
	other := createSession(t, s, c, s.Profiles[0])
	r := taskRequest(other)
	r.Input = append(r.Input, api.Part{Type: "file", InputID: &input.ID})
	if _, _, e = s.CreateTask(c, r, "wrong-session-input"); e == nil || e.Error() != "not_found" {
		t.Fatal(e)
	}
	r = taskRequest(ses)
	r.Input = append(r.Input, api.Part{Type: "file", InputID: &input.ID})
	out, _, e := s.CreateTask(c, r, "bound-input")
	if e != nil {
		t.Fatal(e)
	}
	_ = s.Store.View(func(st agentstore.State) error {
		if st.Tasks[out.ID].InputHashes[input.ID] != input.SHA256 || st.Tasks[out.ID].ConsentSHA256 == "" {
			t.Fatal("input not bound")
		}
		return nil
	})
}
func TestReviewExpiryAndRejectNeverBecomeReady(t *testing.T) {
	for _, mode := range []string{"expire", "reject"} {
		t.Run(mode, func(t *testing.T) {
			s, c, ses := mediaFixture(t)
			out := mediaTask(t, s, c, ses, "image_generate", nil)
			d := &imageDriver{data: imageFixture(), provenance: true}
			if _, e := s.ExecuteNext(context.Background(), d); e != nil {
				t.Fatal(e)
			}
			out, _ = s.GetTask(c, out.ID)
			id := out.Result.Artifacts[0]
			meta, _ := s.GetArtifact(c, id)
			if mode == "expire" {
				if e := s.Maintain(time.Now().Add(25 * time.Hour)); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, _, e := s.Review(c, out.ID, "reject-image", api.Review{Decision: "reject", ArtifactID: id, ExpectedSHA256: meta.SHA256}); e != nil {
					t.Fatal(e)
				}
			}
			out, _ = s.GetTask(c, out.ID)
			if out.Status == "succeeded" {
				t.Fatal(out)
			}
			if _, _, e := s.ArtifactContent(c, id); e == nil {
				t.Fatal("rejected/expired bytes delivered")
			}
			if d.calls != 1 {
				t.Fatal("review caused generation")
			}
		})
	}
}
