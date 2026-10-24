package modelbackend

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/Garden12138/xcli/internal/model"
)

// parseSSE bounds both lines and accumulated event data, including multi-line data fields.
func parseSSE(reader io.Reader, send func(model.Object) error) error {
	const eventLimit = 1 << 20
	scanner := bufio.NewScanner(io.LimitReader(reader, MaxResponseBytes+1))
	scanner.Buffer(make([]byte, 4096), eventLimit)
	var data bytes.Buffer
	total := 0
	dispatch := func() (bool, error) {
		if data.Len() == 0 {
			return false, nil
		}
		payload := bytes.TrimSuffix(data.Bytes(), []byte("\n"))
		if bytes.Equal(payload, []byte("[DONE]")) {
			data.Reset()
			return true, nil
		}
		var out model.Object
		if !utf8.Valid(payload) || json.Unmarshal(payload, &out) != nil || out == nil || out["error"] != nil {
			return false, model.Failure(502, "invalid_upstream_stream", "upstream SSE event is invalid or reports failure")
		}
		err := send(out)
		data.Reset()
		return false, err
	}
	for scanner.Scan() {
		line := scanner.Text()
		total += len(line) + 1
		if total > MaxResponseBytes {
			return model.Failure(502, "upstream_response_too_large", "upstream stream exceeds byte limit")
		}
		if line == "" {
			done, err := dispatch()
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			if data.Len()+len(value)+1 > eventLimit {
				return model.Failure(502, "upstream_event_too_large", "upstream SSE event exceeds limit")
			}
			data.WriteString(value)
			data.WriteByte('\n')
		}
	}
	if scanner.Err() != nil {
		return model.Failure(502, "invalid_upstream_stream", "cannot read bounded upstream SSE stream")
	}
	return model.Failure(502, "incomplete_upstream_stream", "upstream stream ended without [DONE]")
}
