package agentgateway

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentpolicy"
	"github.com/Garden12138/xcli/internal/agentservice"
	"github.com/Garden12138/xcli/internal/agentstore"
)

func benchmarkServer(b *testing.B) (*Server, string, string) {
	b.Helper()
	st, e := agentstore.Open(filepath.Join(b.TempDir(), "state"))
	if e != nil {
		b.Fatal(e)
	}
	b.Cleanup(func() { st.Close() })
	cs := &agentstore.Clients{Root: st.Root}
	c := agentstore.Client{ID: "worklog", Scopes: agentstore.Scopes, Agents: []string{"codex"}, Policies: []string{"synthetic-text-readonly-v1"}, Operations: []string{"text_task"}}
	token, e := cs.Issue(c, false)
	if e != nil {
		b.Fatal(e)
	}
	text := strings.Repeat("x", 1024)
	id := api.ID("tsk")
	_ = st.Update(func(state *agentstore.State) error {
		state.Tasks[id] = &agentstore.TaskRecord{Owner: c.ID, Task: api.Task{APIVersion: 1, ID: id, Status: "succeeded", Result: api.Result{Text: &text, Final: true, Artifacts: []string{}}, ExpiresAt: time.Now().Add(time.Hour)}}
		return nil
	})
	svc, e := agentservice.New(st, cs, agentpolicy.DefaultConfig(), agentpolicy.Defaults())
	if e != nil {
		b.Fatal(e)
	}
	return &Server{Service: svc, Address: "127.0.0.1:18742"}, token, id
}
func BenchmarkCachedCapabilitiesHTTP(b *testing.B) {
	s, token, _ := benchmarkServer(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r := httptest.NewRequest("GET", api.Prefix+"/capabilities", nil)
		r.Host = s.Address
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 200 {
			b.Fatal(w.Code)
		}
	}
}
func BenchmarkPersistedFakeTaskReadHTTP(b *testing.B) {
	s, token, id := benchmarkServer(b)
	b.ReportAllocs()
	b.SetBytes(1024)
	b.ResetTimer()
	for range b.N {
		r := httptest.NewRequest("GET", api.Prefix+"/tasks/"+id, nil)
		r.Host = s.Address
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 200 {
			b.Fatal(w.Code)
		}
	}
}
