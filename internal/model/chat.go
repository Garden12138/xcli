package model

import (
	"encoding/json"
	"slices"
)

func ValidFinishReason(s string) bool {
	return slices.Contains([]string{"stop", "length", "tool_calls", "content_filter", "function_call"}, s)
}

func ValidCreated(raw json.RawMessage) bool {
	var value int64
	return string(raw) != "null" && json.Unmarshal(raw, &value) == nil && value >= 0
}

// ChatStream validates termination per choice without buffering content or tool arguments.
type ChatStream struct {
	n        int
	id       string
	seen     map[int]bool
	finished map[int]bool
}

func NewChatStream(n int) *ChatStream {
	return &ChatStream{n: n, seen: map[int]bool{}, finished: map[int]bool{}}
}

func (s *ChatStream) Observe(chunk Object) error {
	fail := func() error {
		return Failure(502, "invalid_upstream_stream", "upstream chunk is invalid or out of sequence")
	}
	if chunk.String("object") != "chat.completion.chunk" || chunk.String("id") == "" || !ValidCreated(chunk["created"]) {
		return fail()
	}
	if s.id == "" {
		s.id = chunk.String("id")
	}
	if s.id != chunk.String("id") {
		return fail()
	}
	var choices []Object
	if json.Unmarshal(chunk["choices"], &choices) != nil || choices == nil {
		return fail()
	}
	if len(choices) == 0 && chunk["usage"] == nil {
		return fail()
	}
	for _, choice := range choices {
		var index int
		var delta Object
		if json.Unmarshal(choice["index"], &index) != nil || index < 0 || index >= s.n || s.finished[index] || json.Unmarshal(choice["delta"], &delta) != nil || delta == nil {
			return fail()
		}
		s.seen[index] = true
		if raw, ok := delta["content"]; ok && string(raw) != "null" {
			var content string
			if json.Unmarshal(raw, &content) != nil {
				return fail()
			}
		}
		if raw, ok := delta["role"]; ok {
			var role string
			if json.Unmarshal(raw, &role) != nil || role != "assistant" {
				return fail()
			}
		}
		if reason := choice.String("finish_reason"); reason != "" {
			if !ValidFinishReason(reason) {
				return fail()
			}
			s.finished[index] = true
		}
	}
	return nil
}

func (s *ChatStream) Finish() error {
	if len(s.finished) != s.n || len(s.seen) != s.n {
		return Failure(502, "incomplete_upstream_stream", "upstream did not finish every requested choice")
	}
	return nil
}
