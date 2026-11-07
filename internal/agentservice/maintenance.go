package agentservice

import (
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentartifact"
	"github.com/Garden12138/xcli/internal/agentstore"
)

// Maintain expires durable state before removing private material. Live and
// uncertain workers retain their workspaces; historical PIDs are never touched.
func (s *Service) Maintain(now time.Time) error {
	var inputs, artifacts, workspaces []string
	scrubbed := false
	e := s.Store.Update(func(st *agentstore.State) error {
		for _, r := range st.Tasks {
			if r.Task.Status == "awaiting_review" && r.Task.ReviewDeadlineAt != nil && !now.Before(*r.Task.ReviewDeadlineAt) {
				finish(st, r, "expired", "object_expired")
			}
			if !now.Before(r.Task.ExpiresAt) && api.Terminal(r.Task.Status) {
				if r.Task.Result.Text != nil || len(r.Request.Input) > 0 || len(r.Events) > 0 {
					scrubbed = true
				}
				r.Task.Result.Text = nil
				r.Task.Result.Final = false
				r.Events = nil
				r.Request.Input = nil
			}
		}
		for id, r := range st.Sessions {
			if now.Before(r.Session.ExpiresAt) {
				continue
			}
			if r.Session.ActiveTaskID != nil {
				task := st.Tasks[*r.Session.ActiveTaskID]
				if task != nil && !api.Terminal(task.Task.Status) {
					cancelTask(st, task)
				}
				continue
			}
			if r.Session.State == "needs_reconcile" {
				uncertain := false
				for _, task := range st.Tasks {
					if task.Task.SessionID == id && task.InvocationID != "" && (task.Task.Execution.LocalStopConfirmed == nil || !*task.Task.Execution.LocalStopConfirmed) {
						uncertain = true
					}
				}
				if uncertain {
					continue
				}
				if !r.WorkspacePurged {
					r.Context = api.Context{}
					workspaces = append(workspaces, id)
				}
				continue
			}
			if r.Session.State != "expired" {
				r.Context = api.Context{}
				r.Session.State = "expired"
				r.Session.Revision++
			}
			if !r.WorkspacePurged {
				workspaces = append(workspaces, id)
			}
		}
		for id, r := range st.Inputs {
			if now.Before(r.Input.ExpiresAt) {
				continue
			}
			ses := st.Sessions[r.Input.SessionID]
			if ses != nil && ses.Session.ActiveTaskID != nil {
				continue
			}
			if !r.Purged {
				inputs = append(inputs, id)
				r.Purged = true
				st.Inputs[id] = r
			}
		}
		for id, r := range st.Artifacts {
			if now.Before(r.Artifact.ExpiresAt) {
				continue
			}
			r.Artifact.DeliveryState = "expired"
			st.Artifacts[id] = r
			artifacts = append(artifacts, id)
		}
		liveInputs, liveArtifacts := map[string]bool{}, map[string]bool{}
		for id, r := range st.Inputs {
			if !r.Purged {
				liveInputs[id] = true
			}
			if r.Purged && now.After(r.Input.CreatedAt.Add(31*24*time.Hour)) {
				delete(st.Inputs, id)
			}
		}
		for id, r := range st.Artifacts {
			if now.Before(r.Artifact.ExpiresAt) {
				liveArtifacts[id] = true
			}
		}
		if e := agentartifact.Sweep(s.Store.Root, liveInputs, liveArtifacts); e != nil {
			return e
		}
		for k, v := range st.Keys {
			if !now.Before(v.ExpiresAt) {
				delete(st.Keys, k)
				scrubbed = true
			}
		}
		for id, r := range st.Tasks {
			if api.Terminal(r.Task.Status) && r.Task.Status != "needs_reconcile" && now.After(r.Task.CreatedAt.Add(31*24*time.Hour)) {
				delete(st.Tasks, id)
				scrubbed = true
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	if scrubbed {
		if e = s.Store.Compact(); e != nil {
			return e
		}
	}
	for _, id := range inputs {
		if e = agentartifact.Remove(s.Store.Root, "inputs", id); e != nil {
			return e
		}
	}
	for _, id := range artifacts {
		if e = agentartifact.Remove(s.Store.Root, "artifacts", id); e != nil {
			return e
		}
	}
	for _, id := range workspaces {
		if e = s.Store.RemoveWorkspace(id); e != nil {
			return e
		}
	}
	if len(workspaces) > 0 {
		if e = s.Store.Update(func(st *agentstore.State) error {
			for _, id := range workspaces {
				if r := st.Sessions[id]; r != nil {
					r.WorkspacePurged = true
				}
			}
			return nil
		}); e != nil {
			return e
		}
	}
	s.mu.Lock()
	for id, clock := range s.acceptedClocks {
		if now.Sub(clock) > 31*24*time.Hour {
			delete(s.acceptedClocks, id)
		}
	}
	s.mu.Unlock()
	return nil
}
