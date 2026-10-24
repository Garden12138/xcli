package modelstore

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/Garden12138/xcli/internal/model"
)

type Store struct{ Root string }

type clientRecord struct {
	model.Client
	Hash string `json:"token_hash"`
}

type clientFile struct {
	Version int                     `json:"version"`
	Clients map[string]clientRecord `json:"clients"`
}

var clientID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

func DefaultRoot() (string, error) {
	root := os.Getenv("XDG_DATA_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(root, "xcli", "model-gateway"), nil
}

func New(root string) (*Store, error) {
	var err error
	if root == "" {
		root, err = DefaultRoot()
		if err != nil {
			return nil, err
		}
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	if err = checkPrivate(root, true); err != nil {
		return nil, err
	}
	return &Store{Root: root}, nil
}

func (s *Store) readClients() (clientFile, error) {
	b, err := ReadPrivate(filepath.Join(s.Root, "clients.json"), 1<<20)
	if os.IsNotExist(err) {
		return clientFile{Version: 1, Clients: map[string]clientRecord{}}, nil
	}
	if err != nil {
		return clientFile{}, err
	}
	var f clientFile
	if json.Unmarshal(b, &f) != nil || f.Version != 1 || f.Clients == nil {
		return clientFile{}, errors.New("unsupported or corrupt gateway client store")
	}
	return f, nil
}

// Issue only stores a digest. The caller must deliver the returned secret privately.
// rotate requires an existing ID; issue refuses to overwrite one.
func (s *Store) Issue(c model.Client, rotate bool) (string, error) {
	if c.ID == "chat-insight" {
		c.RequireConsent = true
	}
	if !clientID.MatchString(c.ID) || len(c.AllowedModels) == 0 || c.RequestsPerMinute < 1 || c.RequestsPerMinute > 60000 {
		return "", errors.New("client needs a valid ID, model scope and request rate")
	}
	lock, err := Lock(filepath.Join(s.Root, "clients.lock"), true)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	f, err := s.readClients()
	if err != nil {
		return "", err
	}
	_, exists := f.Clients[c.ID]
	if exists != rotate {
		return "", errors.New("client already exists or rotation target is missing")
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	f.Clients[c.ID] = clientRecord{Client: c, Hash: hex.EncodeToString(sum[:])}
	data, _ := json.Marshal(f)
	if err = WritePrivate(filepath.Join(s.Root, "clients.json"), data); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) Revoke(id string) error {
	lock, err := Lock(filepath.Join(s.Root, "clients.lock"), true)
	if err != nil {
		return err
	}
	defer lock.Close()
	f, err := s.readClients()
	if err != nil {
		return err
	}
	delete(f.Clients, id)
	data, _ := json.Marshal(f)
	return WritePrivate(filepath.Join(s.Root, "clients.json"), data)
}

func (s *Store) Clients() ([]model.Client, error) {
	f, err := s.readClients()
	if err != nil {
		return nil, err
	}
	out := []model.Client{}
	for _, c := range f.Clients {
		out = append(out, c.Client)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) Authenticate(token string) (model.Client, error) {
	if len(token) != 43 {
		return model.Client{}, errors.New("invalid token")
	}
	f, err := s.readClients()
	if err != nil {
		return model.Client{}, err
	}
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	for _, c := range f.Clients {
		if subtle.ConstantTimeCompare([]byte(hash), []byte(c.Hash)) == 1 {
			return c.Client, nil
		}
	}
	return model.Client{}, errors.New("invalid token")
}
