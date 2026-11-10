// Explicit fake app-server for G7 container transport tests. No model, auth,
// networking, external process execution or account operation exists here.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	send := func(v any) { b, _ := json.Marshal(v); fmt.Println(string(b)) }
	thread, turn := "g7_fake_thread", "g7_fake_turn"
	for scanner.Scan() {
		var m struct {
			ID     json.RawMessage
			Method string
			Params map[string]json.RawMessage
		}
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			os.Exit(91)
		}
		reply := func(v any) { send(map[string]any{"id": m.ID, "result": v}) }
		terminal := func(status string) {
			send(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": thread, "turn": map[string]any{"id": turn, "status": status}}})
		}
		switch m.Method {
		case "initialize":
			reply(map[string]any{"userAgent": "G7_FAKE_NO_MODEL"})
		case "initialized":
		case "thread/start", "thread/resume":
			if m.Method == "thread/resume" {
				json.Unmarshal(m.Params["threadId"], &thread)
			}
			reply(map[string]any{"thread": map[string]any{"id": thread}})
		case "turn/start":
			var input []struct{ Type, Text string }
			if json.Unmarshal(m.Params["input"], &input) != nil || len(input) != 1 {
				os.Exit(92)
			}
			reply(map[string]any{"turn": map[string]any{"id": turn, "status": "inProgress"}})
			if input[0].Text == "g7_fake_crash" {
				os.Exit(17)
			}
			if input[0].Text == "g7_fake_wait" {
				continue
			}
			send(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": thread, "turnId": turn, "item": map[string]any{"id": "g7_fake_msg", "type": "agentMessage", "phase": "final_answer", "text": "G7_FAKE_SUCCESS"}}})
			terminal("completed")
		case "turn/interrupt":
			reply(map[string]any{})
			terminal("interrupted")
		default:
			send(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": "fake fixture methods only"}})
		}
	}
}
