package modelgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"unicode/utf8"

	"github.com/Garden12138/xcli/internal/model"
)

const maxJSONBody = 4 << 20

func readObject(w http.ResponseWriter, r *http.Request, limit int64) (model.Object, error) {
	typ, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if typ != "application/json" {
		return nil, model.Failure(415, "invalid_content_type", "application/json is required")
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			return nil, model.Failure(413, "request_too_large", "request exceeds byte limit")
		}
		return nil, model.Failure(400, "invalid_request", "cannot read request body")
	}
	var out model.Object
	if !utf8.Valid(b) || json.Unmarshal(b, &out) != nil || out == nil {
		return nil, model.Failure(400, "invalid_request", "request must be a valid UTF-8 JSON object")
	}
	return out, nil
}

func validateChat(req model.Object) ([]model.Capability, int, error) {
	required := []model.Capability{model.Text}
	n := 1
	if raw, ok := req["stream"]; ok {
		var stream bool
		if json.Unmarshal(raw, &stream) != nil || bytes.Equal(raw, []byte("null")) {
			return nil, 0, model.Failure(400, "invalid_request", "stream must be a boolean")
		}
		if stream {
			required = append(required, model.Stream)
		}
	}
	if raw, ok := req["n"]; ok {
		if json.Unmarshal(raw, &n) != nil || n < 1 || n > 8 {
			return nil, 0, model.Failure(400, "invalid_request", "n must be between 1 and 8")
		}
	}
	for _, field := range []string{"tools", "tool_choice", "parallel_tool_calls", "functions", "function_call"} {
		if _, ok := req[field]; ok {
			required = append(required, model.Tools)
		}
	}
	if raw, ok := req["response_format"]; ok {
		var format model.Object
		if json.Unmarshal(raw, &format) != nil {
			return nil, 0, model.Failure(400, "invalid_request", "invalid response_format")
		}
		switch format.String("type") {
		case "text":
		case "json_object":
			required = append(required, model.JSONObject)
		case "json_schema":
			required = append(required, model.JSONSchema)
		default:
			return nil, 0, model.Failure(422, "unsupported_capability", "unsupported response format")
		}
	}
	var messages []model.Object
	if json.Unmarshal(req["messages"], &messages) != nil || len(messages) == 0 || len(messages) > 4096 {
		return nil, 0, model.Failure(400, "invalid_request", "messages must be a nonempty bounded array")
	}
	for _, m := range messages {
		if !slices.Contains([]string{"system", "developer", "user", "assistant", "tool"}, m.String("role")) {
			return nil, 0, model.Failure(400, "invalid_request", "unsupported message role")
		}
		if m.String("role") == "tool" || m["tool_calls"] != nil {
			required = append(required, model.Tools)
		}
		var text string
		if json.Unmarshal(m["content"], &text) == nil && !bytes.Equal(m["content"], []byte("null")) {
			continue
		}
		if m.String("role") == "assistant" && m["tool_calls"] != nil && (m["content"] == nil || bytes.Equal(m["content"], []byte("null"))) {
			continue
		}
		var parts []model.Object
		if json.Unmarshal(m["content"], &parts) != nil || len(parts) == 0 {
			return nil, 0, model.Failure(400, "invalid_request", "message content must be text or supported content parts")
		}
		for _, part := range parts {
			switch part.String("type") {
			case "text":
				if json.Unmarshal(part["text"], &text) != nil {
					return nil, 0, model.Failure(400, "invalid_request", "content part text must be a string")
				}
			case "image_url":
				var imageURL model.Object
				if json.Unmarshal(part["image_url"], &imageURL) != nil {
					return nil, 0, model.Failure(422, "invalid_image", "invalid image_url content part")
				}
				if _, err := model.DecodeDataImage(imageURL.String("url")); err != nil {
					return nil, 0, err
				}
				required = append(required, model.Vision)
			default:
				return nil, 0, model.Failure(422, "unsupported_capability", "unsupported message content part")
			}
		}
	}
	return required, n, nil
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request, c Client) {
	req, err := readObject(w, r, maxJSONBody)
	if err != nil {
		writeError(w, err)
		return
	}
	caps, n, err := validateChat(req)
	if err != nil {
		writeError(w, err)
		return
	}
	id := req.String("model")
	d, err := s.resolve(c, id, caps...)
	if err != nil {
		writeError(w, err)
		return
	}
	if err = s.consent(r, c, d); err != nil {
		writeError(w, err)
		return
	}
	backend := s.options.Chats[d.BackendID]
	if backend == nil {
		writeError(w, model.Failure(503, "backend_unavailable", "configured chat backend is unavailable"))
		return
	}
	if !req.Bool("stream") {
		result, err := backend.Complete(r.Context(), req.WithModel(d.UpstreamModel))
		if r.Context().Err() != nil {
			err = r.Context().Err()
		}
		if err != nil {
			writeError(w, requestError(err))
			return
		}
		if err = validateCompletion(result, n); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, result.WithModel(id))
		return
	}
	started := false
	state := model.NewChatStream(n)
	send := func(chunk model.Object) error {
		if err := state.Observe(chunk); err != nil {
			return err
		}
		if !started {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		b, err := json.Marshal(chunk.WithModel(id))
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return err
		}
		return http.NewResponseController(w).Flush()
	}
	err = backend.Stream(r.Context(), req.WithModel(d.UpstreamModel), send)
	if r.Context().Err() != nil {
		err = r.Context().Err()
	}
	if err == nil {
		err = state.Finish()
	}
	if err != nil {
		if !started {
			writeError(w, requestError(err))
			return
		}
		e := requestError(err)
		data, _ := json.Marshal(map[string]any{"error": e})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		_ = http.NewResponseController(w).Flush()
		return
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	_ = http.NewResponseController(w).Flush()
}

func requestError(err error) *model.Error {
	var e *model.Error
	if errors.As(err, &e) {
		return e
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return model.Failure(504, "request_timeout", "model request timed out")
	}
	if errors.Is(err, context.Canceled) {
		return model.Failure(499, "cancelled", "model request was cancelled")
	}
	return model.Failure(502, "upstream_error", "upstream request failed")
}

func validateCompletion(out model.Object, n int) error {
	if out.String("object") != "chat.completion" || out.String("id") == "" || out["created"] == nil {
		return model.Failure(502, "invalid_upstream_response", "upstream completion envelope is invalid")
	}
	var choices []model.Object
	if json.Unmarshal(out["choices"], &choices) != nil || len(choices) != n {
		return model.Failure(502, "invalid_upstream_response", "upstream completion choices are invalid")
	}
	seen := map[int]bool{}
	for _, choice := range choices {
		var index int
		var message model.Object
		if json.Unmarshal(choice["index"], &index) != nil || index < 0 || index >= n || seen[index] || json.Unmarshal(choice["message"], &message) != nil || message.String("role") != "assistant" || !model.ValidFinishReason(choice.String("finish_reason")) {
			return model.Failure(502, "invalid_upstream_response", "upstream choice is invalid or incomplete")
		}
		seen[index] = true
	}
	return nil
}
