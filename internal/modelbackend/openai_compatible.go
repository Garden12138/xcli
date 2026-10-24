// Package modelbackend implements explicitly configured upstream model protocols.
package modelbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Garden12138/xcli/internal/model"
)

const MaxResponseBytes = 32 << 20

type OpenAI struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

func NewOpenAI(endpoint, apiKey string) (*OpenAI, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("invalid upstream endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("HTTP upstream must be loopback")
	}
	if strings.ContainsAny(apiKey, "\r\n") {
		return nil, errors.New("invalid upstream credential")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Use explicit direct routing: ambient proxy settings must not send local/private data elsewhere.
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.MaxIdleConnsPerHost = 4
	return &OpenAI{endpoint: strings.TrimRight(endpoint, "/"), apiKey: apiKey, client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (b *OpenAI) Close() { b.client.CloseIdleConnections() }

func (b *OpenAI) request(ctx context.Context, path, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint+path, body)
	if err != nil {
		return nil, model.Failure(500, "configuration_error", "cannot create upstream request")
	}
	req.Header.Set("Content-Type", contentType)
	if b.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.apiKey)
	}
	res, err := b.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var n net.Error
		if errors.As(err, &n) && n.Timeout() {
			return nil, model.Failure(504, "upstream_timeout", "upstream timed out")
		}
		return nil, model.Failure(502, "upstream_unavailable", "cannot contact configured upstream")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		status := res.StatusCode
		code := "upstream_error"
		message := "upstream request failed"
		switch status {
		case 401, 403:
			code = "upstream_authentication_error"
			message = "upstream credentials or access were rejected"
		case 429:
			code = "upstream_rate_limit"
			message = "upstream quota or rate limit exceeded"
		default:
			if status < 400 || status > 599 {
				status = 502
			}
		}
		return nil, model.Failure(status, code, message)
	}
	return res, nil
}

func (b *OpenAI) jsonRequest(ctx context.Context, path string, req model.Object) (model.Object, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, model.Failure(400, "invalid_request", "invalid JSON request")
	}
	res, err := b.request(ctx, path, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	return decodeResponse(res.Body)
}

func decodeResponse(r io.Reader) (model.Object, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxResponseBytes {
		return nil, model.Failure(502, "upstream_response_too_large", "upstream response exceeds byte limit")
	}
	var out model.Object
	if json.Unmarshal(data, &out) != nil || out == nil || out["error"] != nil {
		return nil, model.Failure(502, "invalid_upstream_response", "upstream returned an invalid response")
	}
	return out, nil
}

func (b *OpenAI) Complete(ctx context.Context, req model.Object) (model.Object, error) {
	return b.jsonRequest(ctx, "/chat/completions", req)
}

func (b *OpenAI) Stream(ctx context.Context, req model.Object, send func(model.Object) error) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	res, err := b.request(ctx, "/chat/completions", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	typ, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if typ != "text/event-stream" {
		return model.Failure(502, "invalid_upstream_stream", "upstream did not return SSE")
	}
	return parseSSE(res.Body, send)
}
