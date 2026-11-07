package agentstore

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Garden12138/xcli/internal/modelstore"
)

type watermark struct {
	Version    int    `json:"version"`
	Sequence   uint64 `json:"sequence"`
	InstanceID string `json:"instance_id"`
	Digest     string `json:"sha256"`
}

func (s *Store) readWatermark() (*watermark, error) {
	b, e := modelstore.ReadPrivate(filepath.Join(s.Root, "watermark.json"), 1024)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var w watermark
	if json.Unmarshal(b, &w) != nil || w.Version != 1 || w.Sequence < 1 || w.InstanceID == "" || len(w.Digest) != 64 {
		return nil, ErrCorrupt
	}
	return &w, nil
}
func (s *Store) writeWatermark(seq uint64, instance, digest string) error {
	b, _ := json.Marshal(watermark{1, seq, instance, digest})
	if e := modelstore.WritePrivate(filepath.Join(s.Root, "watermark.json"), b); e != nil {
		return e
	}
	return syncDirectory(s.Root)
}

// Compact replaces historical private bodies only after the current state is
// durable. Current event windows, budgets and idempotency remain authoritative.
func (s *Store) Compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.broken {
		return ErrUnavailable
	}
	return s.compactLocked()
}
func (s *Store) compactLocked() error {
	data, e := json.Marshal(s.state)
	if e != nil {
		return e
	}
	if len(data) > maxFrame {
		return ErrUnavailable
	}
	header := make([]byte, 84)
	copy(header, "NAGC")
	binary.BigEndian.PutUint64(header[4:12], s.seq+1)
	binary.BigEndian.PutUint64(header[12:20], uint64(len(data)))
	sum := sha256.Sum256(data)
	copy(header[20:52], sum[:])
	sum = sha256.Sum256(header[:52])
	copy(header[52:], sum[:])
	frame := append(header, data...)
	frame = append(frame, []byte("COMMIT1\n")...)
	file, e := os.CreateTemp(s.Root, ".journal-compact-")
	if e != nil {
		return e
	}
	name := file.Name()
	defer os.Remove(name)
	if _, e = file.Write(frame); e != nil {
		file.Close()
		return e
	}
	if e = file.Sync(); e != nil {
		file.Close()
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	if e = os.Rename(name, filepath.Join(s.Root, "journal")); e != nil {
		return e
	}
	if e = syncDirectory(s.Root); e != nil {
		s.broken = true
		return ErrUnavailable
	}
	old := s.file
	s.file, e = os.OpenFile(filepath.Join(s.Root, "journal"), os.O_RDWR|os.O_APPEND, 0)
	_ = old.Close()
	if e != nil {
		s.broken = true
		return ErrUnavailable
	}
	s.seq++
	sum = sha256.Sum256(data)
	if e = s.writeWatermark(s.seq, s.state.InstanceID, fmtDigest(sum)); e != nil {
		s.broken = true
		return ErrUnavailable
	}
	if e = os.Remove(filepath.Join(s.Root, "checkpoint.json")); e != nil && !os.IsNotExist(e) {
		s.broken = true
		return ErrUnavailable
	}
	if e = syncDirectory(s.Root); e != nil {
		s.broken = true
		return ErrUnavailable
	}
	return nil
}
func fmtDigest(sum [32]byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 64)
	for i, b := range sum {
		out[2*i] = digits[b>>4]
		out[2*i+1] = digits[b&15]
	}
	return string(out)
}
