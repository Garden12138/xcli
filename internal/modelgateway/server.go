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
	"time"

	"github.com/Garden12138/xcli/internal/model"
)

type Client struct {
	ID            string
	AllowedModels []string
	AllowRemote   bool
}

type Authenticator func(string) (Client, error)

type Options struct {
	Registry      *model.Registry
	Authenticate  Authenticator
	MaxConcurrent int
	Timeout       time.Duration
}

type Server struct{ options Options }

func New(options Options) (*Server, error) {
	if options.Registry == nil {
		return nil, errors.New("model registry is required")
	}
	return &Server{options: options}, nil
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	ip, _, err := net.SplitHostPort(listener.Addr().String())
	if err != nil || net.ParseIP(ip) == nil || !net.ParseIP(ip).IsLoopback() {
		return errors.New("gateway listener must be loopback")
	}
	h := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	finished := make(chan struct{})
	defer close(finished)
	go func() {
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
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/readyz":
		writeJSON(w, 200, map[string]any{"status": "ready", "models": len(s.visibleModels(client))})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		writeJSON(w, 200, map[string]any{"object": "list", "data": s.visibleModels(client)})
	default:
		writeError(w, model.Failure(404, "not_found", "API route not found"))
	}
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
	writeJSON(w, e.Status, map[string]any{"error": e})
}
