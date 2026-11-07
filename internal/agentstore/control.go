package agentstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/modelstore"
)

type ControlState struct {
	Version      int        `json:"version"`
	InstanceID   string     `json:"instance_id"`
	GenerationID string     `json:"generation_id"`
	Address      string     `json:"address"`
	ConfigSHA256 string     `json:"config_sha256"`
	Active       bool       `json:"active"`
	StartedAt    time.Time  `json:"started_at"`
	StoppedAt    *time.Time `json:"stopped_at"`
}

func (s *Store) Publish(address, digest string) (ControlState, error) {
	var instance string
	if e := s.View(func(st State) error { instance = st.InstanceID; return nil }); e != nil {
		return ControlState{}, e
	}
	state := ControlState{Version: 1, InstanceID: instance, GenerationID: api.ID("generation"), Address: address, ConfigSHA256: digest, Active: true, StartedAt: time.Now().UTC()}
	b, _ := json.Marshal(state)
	return state, modelstore.WritePrivate(filepath.Join(s.Root, "control.json"), b)
}
func (s *Store) Unpublish(state ControlState) error {
	state.Active = false
	now := time.Now().UTC()
	state.StoppedAt = &now
	b, _ := json.Marshal(state)
	return modelstore.WritePrivate(filepath.Join(s.Root, "control.json"), b)
}
func Status(root string) (ControlState, error) {
	root, e := Prepare(root)
	if e != nil {
		return ControlState{}, e
	}
	probe, e := modelstore.Lock(filepath.Join(root, "probe.lock"), true)
	if e != nil {
		return ControlState{}, e
	}
	defer probe.Close()
	lock, e := modelstore.Lock(filepath.Join(root, "journal.lock"), false)
	active := errors.Is(e, modelstore.ErrLocked)
	if e != nil && !active {
		return ControlState{}, e
	}
	if lock != nil {
		defer lock.Close()
	}
	b, e := modelstore.ReadPrivate(filepath.Join(root, "control.json"), 64<<10)
	if os.IsNotExist(e) && !active {
		return ControlState{Version: 1}, nil
	}
	if e != nil {
		return ControlState{}, e
	}
	var state ControlState
	if json.Unmarshal(b, &state) != nil || state.Version != 1 {
		return state, ErrCorrupt
	}
	state.Active = active
	return state, nil
}
func WatchStop(ctx context.Context, root string, state ControlState, cancel context.CancelFunc) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			b, e := modelstore.ReadPrivate(filepath.Join(root, "stop.json"), 1024)
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				cancel()
				return e
			}
			var req struct {
				InstanceID   string `json:"instance_id"`
				GenerationID string `json:"generation_id"`
			}
			if json.Unmarshal(b, &req) != nil {
				cancel()
				return ErrCorrupt
			}
			if req.InstanceID == state.InstanceID && req.GenerationID == state.GenerationID {
				cancel()
				return nil
			}
		}
	}
}
func Stop(ctx context.Context, root string) error {
	root, e := Prepare(root)
	if e != nil {
		return e
	}
	state, e := Status(root)
	if e != nil || !state.Active {
		return e
	}
	b, _ := json.Marshal(map[string]string{"instance_id": state.InstanceID, "generation_id": state.GenerationID})
	if e = modelstore.WritePrivate(filepath.Join(root, "stop.json"), b); e != nil {
		return e
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		next, e := Status(root)
		if e != nil {
			return e
		}
		if !next.Active || next.GenerationID != state.GenerationID {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("gateway stop is still pending")
		case <-ticker.C:
		}
	}
}
