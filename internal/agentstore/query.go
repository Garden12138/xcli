package agentstore

import "time"

func (s *Store) Identity() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.broken {
		return "", ErrUnavailable
	}
	return s.state.InstanceID, nil
}
func (s *Store) HasQueued() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.broken {
		return false, ErrUnavailable
	}
	for _, r := range s.state.Tasks {
		if r.Task.Status == "queued" {
			return true, nil
		}
	}
	return false, nil
}
func (s *Store) TaskStatus(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.broken {
		return "", ErrUnavailable
	}
	r := s.state.Tasks[id]
	if r == nil {
		return "", ErrUnavailable
	}
	return r.Task.Status, nil
}
func (s *Store) MaintenanceDue(now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.broken {
		return false, ErrUnavailable
	}
	for _, r := range s.state.Tasks {
		if (r.Task.Status == "awaiting_review" && r.Task.ReviewDeadlineAt != nil && !now.Before(*r.Task.ReviewDeadlineAt)) || (!now.Before(r.Task.ExpiresAt) && (r.Task.Result.Text != nil || len(r.Request.Input) > 0 || len(r.Events) > 0)) {
			return true, nil
		}
	}
	for _, r := range s.state.Sessions {
		if !now.Before(r.Session.ExpiresAt) && !r.WorkspacePurged {
			uncertain := false
			if r.Session.State == "needs_reconcile" {
				for _, task := range s.state.Tasks {
					if task.Task.SessionID == r.Session.ID && task.InvocationID != "" && (task.Task.Execution.LocalStopConfirmed == nil || !*task.Task.Execution.LocalStopConfirmed) {
						uncertain = true
					}
				}
			}
			if !uncertain {
				return true, nil
			}
		}
	}
	for _, r := range s.state.Inputs {
		if !r.Purged && !now.Before(r.Input.ExpiresAt) {
			ses := s.state.Sessions[r.Input.SessionID]
			if ses == nil || ses.Session.ActiveTaskID == nil {
				return true, nil
			}
		}
	}
	for _, r := range s.state.Artifacts {
		if !now.Before(r.Artifact.ExpiresAt) && r.Artifact.DeliveryState != "expired" {
			return true, nil
		}
	}
	for _, r := range s.state.Keys {
		if !now.Before(r.ExpiresAt) {
			return true, nil
		}
	}
	return false, nil
}
func (s *Store) GatewayState() (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.broken {
		return "", false, ErrUnavailable
	}
	return s.state.InstanceID, s.state.Quarantined, nil
}

// Uncertain indicates that the current writer cannot safely acknowledge mutations.
func (s *Store) Uncertain() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.broken }
func (s *Store) BinaryBlockers() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.broken {
		return nil, ErrUnavailable
	}
	out := map[string]string{}
	for _, r := range s.state.Tasks {
		if r.Task.Error != nil && (r.Task.Error.Code == "profile_changed" || r.Task.Error.Code == "agent_not_installed") {
			out[r.Task.ProfileSHA256] = r.Task.Error.Code
		}
	}
	return out, nil
}
