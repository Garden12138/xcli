// Package modelgateway exposes bounded model APIs without Agent execution privileges.
package modelgateway

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/model"
)

type Client = model.Client

type Authenticator func(string) (Client, error)

type Options struct {
	Registry      *model.Registry
	Authenticate  Authenticator
	MaxConcurrent int
	Timeout       time.Duration
	Chats         map[string]model.ChatBackend
	Images        map[string]model.ImageBackend
	Embeddings    map[string]model.EmbeddingBackend
	Audit         func(AuditRecord)
}

type Server struct {
	options Options
	slots   chan struct{}
	mu      sync.Mutex
	rates   map[string]rateWindow
}
type rateWindow struct {
	start time.Time
	count int
}

func New(options Options) (*Server, error) {
	if options.Registry == nil {
		return nil, errors.New("model registry is required")
	}
	if options.MaxConcurrent == 0 {
		options.MaxConcurrent = 3
	}
	if options.MaxConcurrent < 1 || options.MaxConcurrent > 64 {
		return nil, errors.New("invalid concurrent request limit")
	}
	if options.Timeout == 0 {
		options.Timeout = 180 * time.Second
	}
	if options.Timeout < 0 {
		return nil, errors.New("invalid request timeout")
	}
	return &Server{options: options, slots: make(chan struct{}, options.MaxConcurrent), rates: map[string]rateWindow{}}, nil
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	ip, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil || net.ParseIP(ip) == nil || !net.ParseIP(ip).IsLoopback() {
		return errors.New("gateway listener must be loopback")
	}
	h := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	finished := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.Shutdown(shutdown)
			_ = h.Close()
		case <-finished:
		}
	}()
	err = h.Serve(listener)
	close(finished)
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	observed, finish := s.audit(w, r)
	defer finish()
	w = observed
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if !localHost(r.Host) || len(r.Header.Values("Origin")) > 0 {
		writeError(w, model.Failure(403, "local_access_only", "only trusted local application requests are allowed"))
		return
	}
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		writeJSON(w, 200, map[string]string{"status": "ok"})
		return
	}
	auth := r.Header.Values("Authorization")
	if len(auth) != 1 || !strings.HasPrefix(auth[0], "Bearer ") || s.options.Authenticate == nil {
		writeError(w, model.Failure(401, "invalid_api_key", "a valid local application token is required"))
		return
	}
	client, err := s.options.Authenticate(strings.TrimPrefix(auth[0], "Bearer "))
	if err != nil {
		writeError(w, model.Failure(401, "invalid_api_key", "a valid local application token is required"))
		return
	}
	observed.record.ClientID = client.ID
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/readyz":
		visible := s.visibleModels(client)
		if len(visible) == 0 {
			writeError(w, model.Failure(503, "not_ready", "no models are available for this client"))
			return
		}
		writeJSON(w, 200, map[string]any{"status": "configured", "models": len(visible), "upstream_verified": false})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		writeJSON(w, 200, map[string]any{"object": "list", "data": s.visibleModels(client)})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		s.inference(w, r, client, s.chat)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/images/generations":
		s.inference(w, r, client, s.generate)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/images/edits":
		s.inference(w, r, client, s.edit)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/embeddings":
		s.inference(w, r, client, s.embed)
	default:
		writeError(w, model.Failure(404, "not_found", "API route not found"))
	}
}

func (s *Server) inference(w http.ResponseWriter, r *http.Request, c Client, handler func(http.ResponseWriter, *http.Request, Client)) {
	if c.RequestsPerMinute > 0 {
		s.mu.Lock()
		window := s.rates[c.ID]
		now := time.Now()
		if now.Sub(window.start) >= time.Minute {
			window = rateWindow{start: now}
		}
		window.count++
		s.rates[c.ID] = window
		s.mu.Unlock()
		if window.count > c.RequestsPerMinute {
			w.Header().Set("Retry-After", "60")
			writeError(w, model.Failure(429, "client_rate_limit", "client request rate exceeded"))
			return
		}
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		writeError(w, model.Failure(429, "concurrency_limit", "gateway concurrent request limit reached"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.options.Timeout)
	defer cancel()
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(s.options.Timeout + time.Second))
	_ = controller.SetReadDeadline(time.Now().Add(min(s.options.Timeout, 30*time.Second)))
	handler(w, r.WithContext(ctx), c)
}

func (s *Server) resolve(c Client, id string, caps ...model.Capability) (model.Descriptor, error) {
	if !allowed(c, id) {
		return model.Descriptor{}, model.Failure(403, "model_access_denied", "client is not authorized for this model alias")
	}
	d, err := s.options.Registry.Resolve(id, caps...)
	if err != nil {
		return d, err
	}
	if d.Metadata.InferenceLocation != "local" && !c.AllowRemote {
		return model.Descriptor{}, model.Failure(403, "data_location_denied", "client is not authorized to send data to remote or unknown inference")
	}
	return d, nil
}

func (s *Server) consent(r *http.Request, c Client, d model.Descriptor) error {
	if c.RequireConsent && d.Metadata.InferenceLocation != "local" && r.Header.Get("X-Xcli-Remote-Consent") != "allow" {
		return model.Failure(403, "cloud_consent_required", "explicit request consent is required for remote or unknown inference")
	}
	return nil
}

func (s *Server) visibleModels(c Client) []model.Descriptor {
	out := []model.Descriptor{}
	for _, d := range s.options.Registry.List() {
		if allowed(c, d.ID) {
			out = append(out, d)
		}
	}
	return out
}

func allowed(c Client, id string) bool {
	for _, m := range c.AllowedModels {
		if m == id {
			return true
		}
	}
	return false
}

func localHost(host string) bool {
	if strings.Contains(host, ":") {
		h, p, err := net.SplitHostPort(host)
		if err != nil {
			return false
		}
		port, err := strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return false
		}
		host = h
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	var e *model.Error
	if !errors.As(err, &e) {
		e = model.Failure(500, "internal_error", "model gateway request failed")
	}
	if observed, ok := w.(*auditWriter); ok {
		observed.record.ErrorCode = e.Code
	}
	writeJSON(w, e.Status, map[string]any{"error": e})
}
