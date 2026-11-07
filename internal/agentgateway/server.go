// Package agentgateway exposes only the Native Agent Gateway task protocol.
package agentgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	api "github.com/Garden12138/xcli/internal/agentapi"
	"github.com/Garden12138/xcli/internal/agentservice"
	"github.com/Garden12138/xcli/internal/agentstore"
)

type Server struct {
	Service   *agentservice.Service
	Address   string
	once      sync.Once
	slots     chan struct{}
	bodySlots chan struct{}
	sseSlots  chan struct{}
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	host, _, e := net.SplitHostPort(listener.Addr().String())
	if e != nil || (host != "127.0.0.1" && host != "::1") {
		return errors.New("literal loopback listener required")
	}
	s.Address = listener.Addr().String()
	h := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.Shutdown(c)
			_ = h.Close()
		case <-done:
		}
	}()
	e = h.Serve(listener)
	close(done)
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
func JSON(w http.ResponseWriter, status int, v any) {
	b, e := json.Marshal(v)
	if e != nil || len(b) > 8<<20 {
		status = 503
		b = []byte(`{"api_version":1,"error":{"code":"store_unavailable","category":"internal","message":"response unavailable","accepted":false,"task_id":null,"automatic_retry_allowed":false,"remediation":"none"}}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}
func Fail(w http.ResponseWriter, e error) {
	var wire *api.Error
	if !errors.As(e, &wire) {
		code := "store_unavailable"
		if errors.Is(e, agentstore.ErrCorrupt) {
			code = "store_corrupt"
		}
		wire = api.Failure(503, code)
	}
	JSON(w, wire.Status, map[string]any{"api_version": 1, "error": wire})
}
func authenticate(s *Server, r *http.Request) (agentstore.Client, error) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") || strings.ContainsAny(strings.TrimPrefix(values[0], "Bearer "), " \t\r\n") {
		return agentstore.Client{}, api.Failure(401, "invalid_client_token")
	}
	return s.Service.Clients.Authenticate(strings.TrimPrefix(values[0], "Bearer "))
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	host, _, e := net.SplitHostPort(s.Address)
	if e != nil || (host != "127.0.0.1" && host != "::1") || r.Host != s.Address || r.URL.IsAbs() || len(r.Header.Values("Origin")) != 0 {
		Fail(w, api.Failure(403, "origin_denied"))
		return
	}
	for name := range r.Header {
		n := strings.ToLower(name)
		if n == "forwarded" || n == "via" || strings.HasPrefix(n, "x-forwarded-") || n == "proxy-authorization" {
			Fail(w, api.Failure(403, "origin_denied"))
			return
		}
	}
	if r.URL.RawQuery != "" || r.URL.RawPath != "" {
		Fail(w, api.Failure(400, "invalid_request"))
		return
	}
	if r.Method == "GET" && r.URL.Path == "/healthz" {
		JSON(w, 200, map[string]any{"api_version": 1, "status": "ok"})
		return
	}
	s.once.Do(func() {
		s.slots = make(chan struct{}, 8)
		s.bodySlots = make(chan struct{}, 2)
		s.sseSlots = make(chan struct{}, 8)
	})
	isSSE := r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/events")
	slots := s.slots
	if isSSE {
		slots = s.sseSlots
	}
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	default:
		Fail(w, api.Failure(429, "concurrency_limit"))
		return
	}
	c, err := authenticate(s, r)
	if err != nil {
		Fail(w, err)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, api.Prefix)
	if path == r.URL.Path {
		Fail(w, api.Failure(404, "not_found"))
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if strings.Contains(path, "//") || strings.HasSuffix(path, "/") {
		Fail(w, api.Failure(404, "not_found"))
		return
	}
	k := ""
	if r.Method == "POST" {
		values := r.Header.Values("Idempotency-Key")
		if len(values) != 1 || !validKey(values[0]) {
			Fail(w, api.Failure(400, "invalid_request"))
			return
		}
		k = values[0]
	}
	scope := ""
	if r.Method == "POST" {
		switch {
		case path == "/sessions":
			scope = "session:create"
		case path == "/tasks":
			scope = "task:create"
		case path == "/inputs":
			scope = "input:write"
		case len(parts) == 3 && parts[0] == "sessions" && parts[2] == "close":
			scope = "session:close"
		case len(parts) == 3 && parts[0] == "tasks" && parts[2] == "cancel":
			scope = "task:cancel"
		case len(parts) == 3 && parts[0] == "tasks" && parts[2] == "review":
			scope = "task:review"
		}
	}
	if scope != "" && !c.Has(scope) {
		Fail(w, api.Failure(403, "scope_denied"))
		return
	}
	if r.Method == "POST" || (r.Method == "GET" && strings.HasSuffix(path, "/content")) {
		select {
		case s.bodySlots <- struct{}{}:
			defer func() { <-s.bodySlots }()
		default:
			Fail(w, api.Failure(429, "concurrency_limit"))
			return
		}
	}
	replayed := false
	var out any
	status := 200
	switch {
	case r.Method == "GET" && path == "/capabilities":
		out, err = s.Service.Capabilities(c)
	case r.Method == "POST" && path == "/sessions":
		var req api.SessionCreate
		if err = readJSON(w, r, &req); err == nil {
			out, replayed, err = s.Service.CreateSession(c, req, k)
			status = 201
		}
	case r.Method == "POST" && path == "/tasks":
		var req api.TaskCreate
		if err = readJSON(w, r, &req); err == nil {
			out, replayed, err = s.Service.CreateTask(c, req, k)
			status = 202
		}
	case r.Method == "POST" && path == "/inputs":
		if len(r.Header.Values("X-Xcli-Session-ID")) != 1 || len(r.Header.Values("X-Xcli-Content-SHA256")) != 1 {
			err = api.Failure(400, "invalid_request")
			break
		}
		typ, limit, typeErr := api.InputType(r.Header.Get("Content-Type"))
		if typeErr != nil {
			err = typeErr
			break
		}
		var data []byte
		data, err = io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
		if err != nil {
			err = api.Failure(413, "input_too_large")
			break
		}
		out, replayed, err = s.Service.Upload(c, r.Header.Get("X-Xcli-Session-ID"), typ, r.Header.Get("X-Xcli-Content-SHA256"), k, data)
		status = 201
	case r.Method == "POST" && len(parts) == 3 && parts[0] == "tasks" && parts[2] == "review":
		var review api.Review
		if err = readJSON(w, r, &review); err == nil {
			out, replayed, err = s.Service.Review(c, parts[1], k, review)
		}
	case r.Method == "GET" && len(parts) == 2 && parts[0] == "artifacts":
		out, err = s.Service.GetArtifact(c, parts[1])
	case r.Method == "GET" && len(parts) == 3 && parts[0] == "artifacts" && parts[2] == "content":
		var meta api.Artifact
		var data []byte
		meta, data, err = s.Service.ArtifactContent(c, parts[1])
		if err != nil {
			break
		}
		ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp"}[meta.MediaType]
		w.Header().Set("Content-Type", meta.MediaType)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("ETag", "\""+meta.SHA256+"\"")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+meta.ID+"."+ext+"\"")
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	case r.Method == "GET" && len(parts) == 2 && parts[0] == "sessions":
		out, err = s.Service.GetSession(c, parts[1])
	case r.Method == "GET" && len(parts) == 2 && parts[0] == "tasks":
		out, err = s.Service.GetTask(c, parts[1])
	case r.Method == "POST" && len(parts) == 3 && parts[0] == "tasks" && parts[2] == "cancel":
		var req struct{}
		if err = readJSON(w, r, &req); err == nil {
			var t api.Task
			t, replayed, err = s.Service.Cancel(c, parts[1], k)
			out = t
			if !api.Terminal(t.Status) {
				status = 202
			}
		}
	case r.Method == "POST" && len(parts) == 3 && parts[0] == "sessions" && parts[2] == "close":
		var req struct{}
		if err = readJSON(w, r, &req); err == nil {
			var ses api.Session
			ses, replayed, err = s.Service.CloseSession(c, parts[1], k)
			out = ses
			if ses.State == "closing" {
				status = 202
			}
		}
	case r.Method == "GET" && len(parts) == 3 && parts[0] == "tasks" && parts[2] == "events":
		s.events(w, r, c, parts[1])
		return
	default:
		err = api.Failure(404, "not_found")
	}
	if err != nil {
		if r.Method == "POST" && s.Service.Store.Uncertain() {
			panic(http.ErrAbortHandler)
		}
		Fail(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	JSON(w, status, out)
}
func validKey(v string) bool {
	if len(v) < 8 || len(v) > 128 {
		return false
	}
	for _, ch := range []byte(v) {
		if ch < 32 || ch > 126 {
			return false
		}
	}
	return true
}
func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	typ, params, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || typ != "application/json" || len(params) > 1 || (len(params) == 1 && strings.ToLower(params["charset"]) != "utf-8") {
		return api.Failure(400, "invalid_request")
	}
	b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, api.MaxJSON))
	if e != nil {
		var size *http.MaxBytesError
		if errors.As(e, &size) {
			return api.Failure(413, "request_too_large")
		}
		return api.Failure(400, "invalid_request")
	}
	return api.StrictDecode(b, dst)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request, c agentstore.Client, id string) {
	var cursor int64
	values := r.Header.Values("Last-Event-ID")
	if len(values) > 1 {
		Fail(w, api.Failure(400, "invalid_cursor"))
		return
	}
	if len(values) == 1 {
		var e error
		cursor, e = strconv.ParseInt(values[0], 10, 64)
		if e != nil {
			Fail(w, api.Failure(400, "invalid_cursor"))
			return
		}
	}
	events, terminal, latest, err := s.Service.Events(c, id, cursor)
	if err != nil {
		w.Header().Set("X-Xcli-Latest-Event-ID", strconv.FormatInt(latest, 10))
		Fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(time.Second))
	if controller.Flush() != nil {
		return
	}
	send := func(b []byte) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(time.Second))
		if _, e := w.Write(b); e != nil {
			return false
		}
		return controller.Flush() == nil
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		for _, event := range events {
			b, _ := json.Marshal(event)
			if len(b) > 64<<10 {
				return
			}
			frame := []byte("id: " + strconv.FormatInt(event.Seq, 10) + "\nevent: " + event.Type + "\ndata: " + string(b) + "\n\n")
			if !send(frame) {
				return
			}
			cursor = event.Seq
		}
		if terminal {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if !send([]byte(": heartbeat\n\n")) {
				return
			}
		case <-ticker.C:
			if _, err = authenticate(s, r); err != nil {
				return
			}
			events, terminal, _, err = s.Service.Events(c, id, cursor)
			if err != nil {
				return
			}
		}
	}
}
