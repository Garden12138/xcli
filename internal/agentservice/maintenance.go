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
	e := s.Store.Update(func(st *agentstore.State) error {
		for _, r := range st.Tasks {
			if r.Task.Status == "awaiting_review" && r.Task.ReviewDeadlineAt != nil && !now.Before(*r.Task.ReviewDeadlineAt) {
				finish(st, r, "expired", "object_expired")
			}
			if !now.Before(r.Task.ExpiresAt) && api.Terminal(r.Task.Status) {
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
				workspaces = append(workspaces, id)
				continue
			}
			if r.Session.State != "expired" {
				r.Session.State = "expired"
				r.Session.Revision++
			}
			workspaces = append(workspaces, id)
		}
		for id, r := range st.Inputs {
			if now.Before(r.Input.ExpiresAt) {
				continue
			}
			ses := st.Sessions[r.Input.SessionID]
			if ses != nil && ses.Session.ActiveTaskID != nil {
				continue
			}
			inputs = append(inputs, id)
			delete(st.Inputs, id)
		}
		for id, r := range st.Artifacts {
			if now.Before(r.Artifact.ExpiresAt) {
				continue
			}
			r.Artifact.DeliveryState = "expired"
			st.Artifacts[id] = r
			artifacts = append(artifacts, id)
		}
		return nil
	})
	if e != nil {
		return e
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
	return nil
}
