// Package agentstore owns the independent single-writer Native Agent journal.
package agentstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/modelstore"
)

const maxFrame = 32 << 20
const maxJournal = 256 << 20

var ErrCorrupt = errors.New("store_corrupt")
var ErrUnavailable = errors.New("store_unavailable")

type SessionRecord struct {
	WorkspacePurged bool        `json:"workspace_purged"`
	Session         api.Session `json:"session"`
	Owner           string      `json:"owner"`
	NativeID        string      `json:"native_id"`
	Context         api.Context `json:"context"`
}
type TaskRecord struct {
	InputHashes     map[string]string `json:"input_hashes"`
	ConsentSHA256   string            `json:"consent_sha256"`
	GrantEpoch      int               `json:"grant_epoch"`
	Task            api.Task          `json:"task"`
	Owner           string            `json:"owner"`
	Request         api.TaskCreate    `json:"request"`
	Events          []api.Event       `json:"events"`
	InvocationID    string            `json:"invocation_id"`
	StartedAt       *time.Time        `json:"started_at"`
	ReservedSeconds int               `json:"reserved_seconds"`
	BudgetDay       string            `json:"budget_day"`
}
type Idempotency struct {
	Digest    string    `json:"digest"`
	Kind      string    `json:"kind"`
	ObjectID  string    `json:"object_id"`
	Status    int       `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Counter struct {
	Dispatches int     `json:"dispatches"`
	Starts     int     `json:"starts"`
	Seconds    float64 `json:"seconds"`
	Reserved   int     `json:"reserved"`
}
type InputRecord struct {
	Purged bool      `json:"purged,omitempty"`
	Owner  string    `json:"owner"`
	Input  api.Input `json:"input"`
}
type ArtifactRecord struct {
	Owner    string       `json:"owner"`
	Artifact api.Artifact `json:"artifact"`
}
type State struct {
	Version     int                       `json:"version"`
	InstanceID  string                    `json:"instance_id"`
	Sessions    map[string]*SessionRecord `json:"sessions"`
	Tasks       map[string]*TaskRecord    `json:"tasks"`
	Keys        map[string]Idempotency    `json:"keys"`
	Counters    map[string]Counter        `json:"counters"`
	Inputs      map[string]InputRecord    `json:"inputs"`
	Artifacts   map[string]ArtifactRecord `json:"artifacts"`
	Quarantined bool                      `json:"quarantined"`
}
type Store struct {
	mu          sync.Mutex
	Root        string
	state       State
	seq         uint64
	file, lease *os.File
	broken      bool
	closed      bool
	BeforeSync  func() error
}

func DefaultRoot() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "xcli", "agent-gateway"), nil
}
func Prepare(root string) (string, error) {
	if root == "" {
		var err error
		root, err = DefaultRoot()
		if err != nil {
			return "", err
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	// Existing ancestor symlinks (including macOS /tmp) are canonicalized once.
	if err = modelstore.PrivateDirectory(root); err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	marker := filepath.Join(root, "namespace.json")
	data, e := modelstore.ReadPrivate(marker, 1024)
	if os.IsNotExist(e) {
		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			return "", readErr
		}
		if len(entries) > 0 {
			return "", errors.New("unrecognized state root; an empty independent Agent Gateway root is required")
		}
		if e = modelstore.WritePrivate(marker, []byte(`{"kind":"xcli.native-agent-gateway","version":1}`)); e != nil {
			return "", e
		}
	} else if e != nil {
		return "", e
	} else {
		var ns struct {
			Kind    string `json:"kind"`
			Version int    `json:"version"`
		}
		if json.Unmarshal(data, &ns) != nil || ns.Kind != "xcli.native-agent-gateway" || ns.Version != 1 {
			return "", ErrCorrupt
		}
	}
	return root, nil
}
func Open(root string) (*Store, error) {
	root, err := Prepare(root)
	if err != nil {
		return nil, err
	}
	probe, err := modelstore.Lock(filepath.Join(root, "probe.lock"), true)
	if err != nil {
		return nil, err
	}
	lease, err := modelstore.Lock(filepath.Join(root, "journal.lock"), false)
	_ = probe.Close()
	if err != nil {
		return nil, err
	}
	s := &Store{Root: root, lease: lease}
	fail := func(e error) (*Store, error) {
		lease.Close()
		if s.file != nil {
			s.file.Close()
		}
		return nil, e
	}
	path := filepath.Join(root, "journal")
	if _, err = os.Lstat(path); err == nil {
		if _, err = modelstore.ReadPrivate(path, maxJournal); err != nil {
			return fail(err)
		}
	} else if !os.IsNotExist(err) {
		return fail(err)
	}
	s.file, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fail(err)
	}
	if err = s.replay(); err != nil {
		return fail(err)
	}
	if s.seq == 0 {
		s.state = State{Version: 1, InstanceID: api.ID("agt_instance"), Sessions: map[string]*SessionRecord{}, Tasks: map[string]*TaskRecord{}, Keys: map[string]Idempotency{}, Counters: map[string]Counter{}, Inputs: map[string]InputRecord{}, Artifacts: map[string]ArtifactRecord{}}
		if err = s.commit(s.state); err != nil {
			return fail(err)
		}
		if err = syncDirectory(root); err != nil {
			return fail(err)
		}
	}
	if e := s.writeWatermark(s.seq, s.state.InstanceID, api.Digest(s.state)); e != nil {
		return fail(e)
	}
	grantLease, e := (&Clients{Root: s.Root}).LockGrants()
	if e != nil {
		return fail(e)
	}
	entries, e := os.ReadDir(s.Root)
	if e != nil {
		grantLease.Close()
		return fail(e)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".journal-compact-") || strings.HasPrefix(entry.Name(), ".xcli-state-") {
			if e = os.Remove(filepath.Join(s.Root, entry.Name())); e != nil {
				grantLease.Close()
				return fail(e)
			}
		}
	}
	grantLease.Close()
	return s, nil
}
func (s *Store) replay() error {
	info, err := s.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > maxJournal {
		return ErrUnavailable
	}
	watermark, e := s.readWatermark()
	if e != nil {
		return e
	}
	var watermarkDigest, firstDigest string
	var firstSeq uint64
	var firstMagic string
	var offset int64
	pendingTrim := false
	for offset < info.Size() {
		header := make([]byte, 84)
		n, e := io.ReadFull(s.file, header)
		if e == io.EOF || e == io.ErrUnexpectedEOF {
			if n == 0 && e == io.EOF {
				break
			}
			pendingTrim = true
			break
		}
		if e != nil {
			return e
		}
		sum := sha256.Sum256(header[:52])
		if (string(header[:4]) != "NAG1" && (string(header[:4]) != "NAGC" || offset != 0)) || !bytes.Equal(sum[:], header[52:]) {
			return ErrCorrupt
		}
		seq := binary.BigEndian.Uint64(header[4:12])
		size := binary.BigEndian.Uint64(header[12:20])
		if ((offset != 0 || string(header[:4]) != "NAGC") && seq != s.seq+1) || seq == 0 || size == 0 || size > maxFrame {
			return ErrCorrupt
		}
		payload := make([]byte, int(size)+8)
		if _, err = io.ReadFull(s.file, payload); err == io.EOF || err == io.ErrUnexpectedEOF {
			pendingTrim = true
			break
		} else if err != nil {
			return err
		}
		sum = sha256.Sum256(payload[:size])
		if string(payload[size:]) != "COMMIT1\n" || !bytes.Equal(sum[:], header[20:52]) {
			return ErrCorrupt
		}
		var st State
		if json.Unmarshal(payload[:size], &st) != nil || st.Version != 1 || st.InstanceID == "" || st.Sessions == nil || st.Tasks == nil || st.Keys == nil || st.Counters == nil || st.Inputs == nil || st.Artifacts == nil {
			return ErrCorrupt
		}
		if offset == 0 {
			firstSeq = seq
			firstMagic = string(header[:4])
			firstDigest = api.Hash(payload[:size])
		}
		if watermark != nil && seq == watermark.Sequence {
			watermarkDigest = api.Hash(payload[:size])
		}
		s.state = st
		s.seq = seq
		offset += int64(84 + size + 8)
	}
	if watermark != nil {
		if watermark.Sequence > s.seq || watermark.InstanceID != s.state.InstanceID {
			return ErrCorrupt
		}
		if firstSeq > watermark.Sequence {
			if firstMagic != "NAGC" || firstDigest != watermark.Digest {
				return ErrCorrupt
			}
		} else if watermarkDigest != watermark.Digest {
			return ErrCorrupt
		}
	} else if s.seq > 1 {
		return ErrCorrupt
	}
	if pendingTrim {
		return s.trim(offset)
	}
	_, err = s.file.Seek(0, io.SeekEnd)
	return err
}
func (s *Store) trim(offset int64) error {
	if err := s.file.Truncate(offset); err != nil {
		return err
	}
	if err := s.file.Sync(); err != nil {
		return err
	}
	_, err := s.file.Seek(offset, io.SeekStart)
	return err
}
func clone[T any](v T) T {
	b, _ := json.Marshal(v)
	var copy T
	_ = json.Unmarshal(b, &copy)
	return copy
}
func (s *Store) View(fn func(State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.broken {
		return ErrUnavailable
	}
	return fn(clone(s.state))
}

// Update publishes a new state only after its entire commit record is durable.
// Any ambiguous write/fsync failure poisons this writer. Recovery never executes.
func (s *Store) Update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.broken {
		return ErrUnavailable
	}
	next := clone(s.state)
	if err := fn(&next); err != nil {
		return err
	}
	if api.Digest(next) == api.Digest(s.state) {
		return nil
	}
	return s.commit(next)
}
func (s *Store) commit(st State) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	info, err := s.file.Stat()
	if err != nil {
		return err
	}
	if len(data) > maxFrame || info.Size()+int64(len(data)+92) > maxJournal {
		return ErrUnavailable
	}
	header := make([]byte, 84)
	copy(header, "NAG1")
	binary.BigEndian.PutUint64(header[4:12], s.seq+1)
	binary.BigEndian.PutUint64(header[12:20], uint64(len(data)))
	sum := sha256.Sum256(data)
	copy(header[20:52], sum[:])
	sum = sha256.Sum256(header[:52])
	copy(header[52:], sum[:])
	frame := append(header, data...)
	frame = append(frame, []byte("COMMIT1\n")...)
	if n, e := s.file.Write(frame); e != nil || n != len(frame) {
		s.broken = true
		return ErrUnavailable
	}
	if s.BeforeSync != nil {
		if err = s.BeforeSync(); err != nil {
			s.broken = true
			return ErrUnavailable
		}
	}
	if err = s.file.Sync(); err != nil {
		s.broken = true
		return ErrUnavailable
	}
	if err = s.writeWatermark(s.seq+1, st.InstanceID, api.Hash(data)); err != nil {
		s.broken = true
		return ErrUnavailable
	}
	s.state = st
	s.seq++
	if info.Size()+int64(len(frame)) > 16<<20 {
		if e := s.compactLocked(); e != nil {
			s.broken = true
			return ErrUnavailable
		}
		return nil
	}
	return nil
}
func (s *Store) Checkpoint() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broken || s.closed {
		return ErrUnavailable
	}
	b, _ := json.Marshal(struct {
		Version  int    `json:"version"`
		Sequence uint64 `json:"sequence"`
		Digest   string `json:"sha256"`
		State    State  `json:"state"`
	}{1, s.seq, api.Digest(s.state), s.state})
	if err := modelstore.WritePrivate(filepath.Join(s.Root, "checkpoint.json"), b); err != nil {
		return err
	}
	return syncDirectory(s.Root)
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return errors.Join(s.file.Close(), s.lease.Close())
}
func syncDirectory(root string) error {
	f, err := os.Open(root)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
