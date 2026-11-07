package agentstore

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/modelstore"
)

var Scopes = []string{"session:create", "session:read", "session:close", "input:write", "task:create", "task:read", "task:cancel", "task:review", "artifact:read", "artifact:attest-origin"}
var label = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

type Client struct {
	ID                    string   `json:"id"`
	Scopes                []string `json:"scopes"`
	Agents                []string `json:"agents"`
	Operations            []string `json:"operations"`
	Policies              []string `json:"policies"`
	AllowRemote           bool     `json:"allow_remote"`
	DailyDispatches       int      `json:"daily_dispatches"`
	DailyProcessStarts    int      `json:"daily_process_starts"`
	DailyExecutionSeconds int      `json:"daily_execution_seconds"`
	Revision              int      `json:"revision"`
	Revoked               bool     `json:"revoked"`
}
type clientRecord struct {
	Client    Client `json:"client"`
	TokenHash string `json:"token_hash"`
}
type clientFile struct {
	Version int                     `json:"version"`
	Clients map[string]clientRecord `json:"clients"`
}
type Clients struct{ Root string }

func (c Client) Has(scope string) bool { return slices.Contains(c.Scopes, scope) }
func (c *Clients) read() (clientFile, error) {
	b, e := modelstore.ReadPrivate(filepath.Join(c.Root, "clients.json"), 1<<20)
	if os.IsNotExist(e) {
		return clientFile{1, map[string]clientRecord{}}, nil
	}
	if e != nil {
		return clientFile{}, e
	}
	var f clientFile
	if json.Unmarshal(b, &f) != nil || f.Version != 1 || f.Clients == nil {
		return f, ErrCorrupt
	}
	return f, nil
}
func (c *Clients) Issue(client Client, rotate bool) (string, error) {
	if !label.MatchString(client.ID) || len(client.Scopes) == 0 || len(client.Agents) == 0 || len(client.Operations) == 0 || len(client.Policies) == 0 || client.DailyDispatches < 0 || client.DailyDispatches > 1000 || client.DailyProcessStarts < 0 || client.DailyProcessStarts > 1000 || client.DailyExecutionSeconds < 0 || client.DailyExecutionSeconds > 86400 {
		return "", errors.New("invalid client grant")
	}
	for _, s := range client.Scopes {
		if !slices.Contains(Scopes, s) {
			return "", errors.New("unknown scope")
		}
	}
	for _, s := range client.Operations {
		if !slices.Contains([]string{"text_task", "image_generate", "image_edit"}, s) {
			return "", errors.New("unknown operation")
		}
	}
	lock, e := modelstore.Lock(filepath.Join(c.Root, "clients.lock"), true)
	if e != nil {
		return "", e
	}
	defer lock.Close()
	f, e := c.read()
	if e != nil {
		return "", e
	}
	old, exists := f.Clients[client.ID]
	if exists != rotate {
		return "", errors.New("client exists or rotation target is missing")
	}
	if rotate {
		client = old.Client
	}
	client.Revision = old.Client.Revision + 1
	client.Revoked = false
	b := make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return "", e
	}
	token := "nag1_" + base64.RawURLEncoding.EncodeToString(b)
	f.Clients[client.ID] = clientRecord{client, api.Hash([]byte(token))}
	data, _ := json.Marshal(f)
	if e = modelstore.WritePrivate(filepath.Join(c.Root, "clients.json"), data); e != nil {
		return "", e
	}
	return token, nil
}
func (c *Clients) Revoke(id string) error {
	lock, e := modelstore.Lock(filepath.Join(c.Root, "clients.lock"), true)
	if e != nil {
		return e
	}
	defer lock.Close()
	f, e := c.read()
	if e != nil {
		return e
	}
	r, ok := f.Clients[id]
	if !ok {
		return api.Failure(404, "not_found")
	}
	r.Client.Revoked = true
	r.Client.Revision++
	r.TokenHash = ""
	f.Clients[id] = r
	b, _ := json.Marshal(f)
	return modelstore.WritePrivate(filepath.Join(c.Root, "clients.json"), b)
}
func (c *Clients) Authenticate(token string) (Client, error) {
	if len(token) != 48 {
		return Client{}, api.Failure(401, "invalid_client_token")
	}
	f, e := c.read()
	if e != nil {
		return Client{}, e
	}
	sum := api.Hash([]byte(token))
	for _, r := range f.Clients {
		if !r.Client.Revoked && subtle.ConstantTimeCompare([]byte(sum), []byte(r.TokenHash)) == 1 {
			return r.Client, nil
		}
	}
	return Client{}, api.Failure(401, "invalid_client_token")
}
func (c *Clients) Get(id string) (Client, error) {
	f, e := c.read()
	if e != nil {
		return Client{}, e
	}
	r, ok := f.Clients[id]
	if !ok || r.Client.Revoked {
		return Client{}, api.Failure(401, "invalid_client_token")
	}
	return r.Client, nil
}
func (c *Clients) List() ([]Client, error) {
	f, e := c.read()
	if e != nil {
		return nil, e
	}
	out := []Client{}
	for _, r := range f.Clients {
		out = append(out, r.Client)
	}
	slices.SortFunc(out, func(a, b Client) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out, nil
}
