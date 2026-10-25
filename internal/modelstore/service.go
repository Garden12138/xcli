package modelstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

var ErrLocked = errors.New("gateway state is locked by another process")

type ServiceState struct {
	Schema       int        `json:"schema"`
	InstanceID   string     `json:"instance_id"`
	PID          int        `json:"pid"`
	Address      string     `json:"address"`
	ConfigDigest string     `json:"config_digest"`
	Version      string     `json:"version"`
	StartedAt    time.Time  `json:"started_at"`
	StoppedAt    *time.Time `json:"stopped_at,omitempty"`
	Active       bool       `json:"active"`
	Reachable    bool       `json:"reachable"`
}

type Service struct {
	store *Store
	lock  *os.File
	State ServiceState
}

func (s *Store) AcquireService(address, digest, version string) (*Service, error) {
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || port == "" {
		return nil, errors.New("service address must be a bound loopback endpoint")
	}
	lock, err := Lock(filepath.Join(s.Root, "gateway.lock"), false)
	if err != nil {
		return nil, err
	}
	if previous, readErr := ReadPrivate(filepath.Join(s.Root, "service.json"), 64<<10); readErr == nil {
		var old ServiceState
		if json.Unmarshal(previous, &old) != nil || old.Schema != 1 {
			lock.Close()
			return nil, errors.New("unsupported or corrupt service state; explicit recovery required")
		}
	} else if !os.IsNotExist(readErr) {
		lock.Close()
		return nil, readErr
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		lock.Close()
		return nil, err
	}
	state := ServiceState{Schema: 1, InstanceID: hex.EncodeToString(nonce), PID: os.Getpid(), Address: address, ConfigDigest: digest, Version: version, StartedAt: time.Now().UTC(), Active: true}
	data, _ := json.Marshal(state)
	if err = WritePrivate(filepath.Join(s.Root, "service.json"), data); err != nil {
		lock.Close()
		return nil, err
	}
	return &Service{store: s, lock: lock, State: state}, nil
}

func Probe(ctx context.Context, state ServiceState) bool {
	if !state.Active {
		return false
	}
	host, port, err := net.SplitHostPort(state.Address)
	ip := net.ParseIP(host)
	number, numberErr := strconv.Atoi(port)
	if err != nil || ip == nil || !ip.IsLoopback() || numberErr != nil || number < 1 || number > 65535 {
		return false
	}
	client := &http.Client{Timeout: 500 * time.Millisecond, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+state.Address+"/healthz", nil)
	if err != nil {
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1024))
	var body struct {
		Status string `json:"status"`
	}
	return err == nil && res.StatusCode == 200 && json.Unmarshal(data, &body) == nil && body.Status == "ok"
}

func (s *Service) Close() error {
	if s.lock == nil {
		return nil
	}
	when := time.Now().UTC()
	s.State.Active = false
	s.State.StoppedAt = &when
	data, _ := json.Marshal(s.State)
	err := WritePrivate(filepath.Join(s.store.Root, "service.json"), data)
	closeErr := s.lock.Close()
	s.lock = nil
	return errors.Join(err, closeErr)
}

func (s *Store) Status() (ServiceState, error) {
	lock, err := Lock(filepath.Join(s.Root, "gateway.lock"), false)
	active := errors.Is(err, ErrLocked)
	if err != nil && !active {
		return ServiceState{}, err
	}
	if lock != nil {
		defer lock.Close()
	}
	data, err := ReadPrivate(filepath.Join(s.Root, "service.json"), 64<<10)
	if os.IsNotExist(err) && !active {
		return ServiceState{Schema: 1}, nil
	}
	if err != nil {
		return ServiceState{}, err
	}
	var state ServiceState
	if json.Unmarshal(data, &state) != nil || state.Schema != 1 {
		return ServiceState{}, errors.New("unsupported or corrupt service state; no process was signalled")
	}
	state.Active = active
	return state, nil
}

// WatchStop binds stop requests to a random service instance, never to a reusable PID.
func (s *Service) WatchStop(ctx context.Context, cancel context.CancelFunc) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			data, err := ReadPrivate(filepath.Join(s.store.Root, "stop.json"), 1024)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				cancel()
				return err
			}
			var request struct {
				InstanceID string `json:"instance_id"`
			}
			if json.Unmarshal(data, &request) != nil {
				cancel()
				return errors.New("invalid private stop request")
			}
			if request.InstanceID == s.State.InstanceID {
				cancel()
				return nil
			}
		}
	}
}

func (s *Store) Stop(ctx context.Context) error {
	state, err := s.Status()
	if err != nil || !state.Active {
		return err
	}
	data, _ := json.Marshal(map[string]string{"instance_id": state.InstanceID})
	if err = WritePrivate(filepath.Join(s.Root, "stop.json"), data); err != nil {
		return err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := s.Status()
		if err != nil {
			return err
		}
		if !current.Active || current.InstanceID != state.InstanceID {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("gateway stop is still pending; inspect status")
		case <-ticker.C:
		}
	}
}
