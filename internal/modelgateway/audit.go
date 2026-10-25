package modelgateway

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

// AuditRecord intentionally cannot hold prompts, URLs, headers, images or responses.
type AuditRecord struct {
	Time      time.Time `json:"time"`
	RequestID string    `json:"request_id"`
	Route     string    `json:"route"`
	ClientID  string    `json:"client_id,omitempty"`
	Status    int       `json:"status"`
	ErrorCode string    `json:"error_code,omitempty"`
	ElapsedMS int64     `json:"elapsed_ms"`
}

type auditWriter struct {
	http.ResponseWriter
	record AuditRecord
}

func (w *auditWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *auditWriter) WriteHeader(status int) {
	if w.record.Status != 0 {
		return
	}
	w.record.Status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *auditWriter) Write(b []byte) (int, error) {
	if w.record.Status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) (*auditWriter, func()) {
	start := time.Now()
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	route := "unknown"
	switch r.URL.Path {
	case "/healthz", "/readyz", "/v1/models", "/v1/chat/completions", "/v1/images/generations", "/v1/images/edits", "/v1/embeddings":
		route = r.URL.Path
	}
	wrapped := &auditWriter{ResponseWriter: w, record: AuditRecord{Time: start.UTC(), RequestID: hex.EncodeToString(id), Route: route}}
	w.Header().Set("X-Request-ID", wrapped.record.RequestID)
	return wrapped, func() {
		wrapped.record.ElapsedMS = time.Since(start).Milliseconds()
		if wrapped.record.Status == 0 {
			wrapped.record.Status = 200
		}
		if s.options.Audit != nil {
			s.options.Audit(wrapped.record)
		}
	}
}
