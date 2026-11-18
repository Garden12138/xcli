package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/api"
	"github.com/Garden12138/xcli/internal/config"
	"github.com/Garden12138/xcli/internal/driver"
	"github.com/Garden12138/xcli/internal/engine"
	"github.com/Garden12138/xcli/internal/proc"
)

// Version 由 serve 命令在创建 handler 前设为 cmd.Version。
var Version = "2.0.0"

type service struct {
	e              *engine.Engine
	token, version string
	mux            *http.ServeMux
	mu             sync.Mutex
	jobs           map[string]context.CancelFunc
	closing        bool
	wg             sync.WaitGroup
}

func New(e *engine.Engine, token string) http.Handler {
	s := &service{e: e, token: token, version: Version, mux: http.NewServeMux(), jobs: map[string]context.CancelFunc{}}
	s.mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"ok": true, "version": s.version})
	})
	s.mux.HandleFunc("GET /v1/agents", s.agents)
	s.mux.HandleFunc("POST /v1/tasks", s.create)
	s.mux.HandleFunc("GET /v1/tasks", s.list)
	s.mux.HandleFunc("GET /v1/tasks/{id}", s.get)
	s.mux.HandleFunc("GET /v1/tasks/{id}/events", s.events)
	s.mux.HandleFunc("POST /v1/tasks/{id}/cancel", s.cancel)
	s.mux.HandleFunc("GET /v1/tasks/{id}/artifacts/{index}", s.artifact)
	return s
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	public := (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.URL.Path == "/v1/health"
	if !public && (s.token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.token)) != 1) {
		http.Error(w, "请读取 server.token 并使用 Authorization: Bearer <token>", 401)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func queryInt(r *http.Request, key string, def int) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return def, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 || (key == "limit" && n == 0) {
		return 0, fmt.Errorf("请将 %s 设为%s整数", key, map[bool]string{true: "正", false: "非负"}[key == "limit"])
	}
	return n, nil
}

func (s *service) create(w http.ResponseWriter, r *http.Request) {
	var req api.Request
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&req)
	if err == nil {
		var extra any
		if dec.Decode(&extra) != io.EOF {
			err = errors.New("请只提供一个完整的 Request JSON")
		}
	}
	if err != nil {
		reply(w, 400, engine.InvalidResult(fmt.Errorf("请检查请求 JSON：%w", err)))
		return
	}
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		http.Error(w, "服务正在退出，请重新启动 xcli serve 后提交任务", 503)
		return
	}
	t, err := s.e.Create(req)
	if err != nil {
		s.mu.Unlock()
		reply(w, 400, engine.InvalidResult(err))
		return
	}
	if err := os.WriteFile(t.Path("http"), nil, 0600); err != nil {
		_ = s.e.Store.Remove(t.ID())
		s.mu.Unlock()
		http.Error(w, "请检查任务目录写入权限后重新提交", 500)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.jobs[t.ID()] = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer cancel()
		defer func() { s.mu.Lock(); delete(s.jobs, t.ID()); s.mu.Unlock() }()
		s.e.Run(ctx, t, nil)
	}()
	if r.URL.Query().Get("wait") == "true" {
		result, err := s.e.Wait(r.Context(), t.ID())
		if err != nil {
			if r.Context().Err() == nil {
				http.Error(w, "请查询任务状态后重新等待："+err.Error(), 500)
			}
			return
		}
		reply(w, 200, result)
		return
	}
	reply(w, http.StatusAccepted, map[string]string{"task_id": t.ID()})
}

func (s *service) get(w http.ResponseWriter, r *http.Request) {
	rec, err := s.e.Status(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if rec.Result != nil {
		reply(w, 200, rec.Result)
		return
	}
	reply(w, 200, api.Result{Version: api.Version, TaskID: rec.ID, Status: api.StatusRunning, Capability: rec.Request.Capability, Summary: "任务正在运行", Artifacts: []api.Artifact{}, Plan: rec.Plan, Steps: append([]api.StepResult{}, rec.Steps...), Metadata: rec.Request.Metadata, StartedAt: rec.CreatedAt})
}

func (s *service) list(w http.ResponseWriter, r *http.Request) {
	limit, err := queryInt(r, "limit", 20)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	rows, err := s.e.Store.List(limit)
	if err != nil {
		http.Error(w, "请检查任务目录："+err.Error(), 500)
		return
	}
	for i, row := range rows {
		rows[i], err = s.e.Status(row.ID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	reply(w, 200, rows)
}

func (s *service) cancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := s.e.Status(id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if rec.Status.Finished() {
		http.Error(w, "任务已结束，请查询任务结果", 409)
		return
	}
	s.mu.Lock()
	cancel := s.jobs[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	} else if err := s.e.Cancel(id); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	reply(w, 200, map[string]string{"task_id": id})
}

func (s *service) events(w http.ResponseWriter, r *http.Request) {
	after, err := queryInt(r, "after", 0)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if _, err := s.e.Status(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	flush, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "请直接连接 xcli serve 读取事件流", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	flush.Flush()
	_ = s.e.Follow(r.Context(), r.PathValue("id"), after, func(ev api.Event) error {
		raw, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "event: %s\nid: %d\ndata: %s\n\n", ev.Type, ev.Seq, raw)
		flush.Flush()
		return err
	})
}

func (s *service) artifact(w http.ResponseWriter, r *http.Request) {
	rec, err := s.e.Status(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || index < 0 || rec.Result == nil || index >= len(rec.Result.Artifacts) {
		http.Error(w, "请查询任务结果中的 artifacts 索引后重试", 404)
		return
	}
	a := rec.Result.Artifacts[index]
	file, err := os.Open(a.Path)
	if err != nil {
		http.Error(w, "请检查任务产物文件是否仍存在", 404)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "请检查产物文件读取权限", 500)
		return
	}
	w.Header().Set("Content-Type", a.MIME)
	http.ServeContent(w, r, filepath.Base(a.Path), info.ModTime(), file)
}

func (s *service) agents(w http.ResponseWriter, r *http.Request) {
	rows := []map[string]any{}
	for _, name := range config.AgentOrder {
		d, err := driver.New(name, s.e.Config.Agents[name])
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		exe, _ := proc.LookPath(d.Command(), s.e.PathList)
		entry := s.e.Health.Probe(r.Context(), d, exe, proc.Env(os.Environ(), s.e.PathList, s.e.Config.Agents[name].Env), 10*time.Minute)
		h := entry.Health
		rows = append(rows, map[string]any{"name": name, "installed": h.Installed, "path": h.Path, "version": h.Version, "auth": h.Auth, "available": s.e.Config.Enabled(name) && h.Installed && h.Auth != "missing" && s.e.Health.Available(name, time.Now()), "cooldown_until": entry.CooldownUntil, "cooldown_reason": entry.CooldownReason, "strengths": s.e.Config.Agents[name].Strengths})
	}
	reply(w, 200, rows)
}

// Close 取消服务内任务并等待结果和结束事件落盘；重复调用安全。
func (s *service) Close() error {
	s.mu.Lock()
	s.closing = true
	for _, cancel := range s.jobs {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return nil
}

func LoadOrCreateToken(home string) (string, error) {
	if err := os.MkdirAll(home, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(home, "server.token")
	read := func() (string, error) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		token := strings.TrimSpace(string(raw))
		decoded, err := hex.DecodeString(token)
		if err != nil || len(decoded) != 32 {
			return "", fmt.Errorf("请删除 %s 后重新启动 xcli serve 生成令牌", path)
		}
		return token, os.Chmod(path, 0600)
	}
	if token, err := read(); !errors.Is(err, os.ErrNotExist) {
		return token, err
	}
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(home, ".server-token-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(hex.EncodeToString(bytes[:]))
	err = errors.Join(err, tmp.Close())
	if err != nil {
		return "", err
	}
	if err := os.Link(tmp.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	return read()
}
