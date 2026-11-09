// Package codexrpc implements bounded app-server stdio framing. It never logs
// raw messages and has distinct task and non-inference probe method allowlists.
package codexrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/Garden12138/xcli/internal/agent"
	api "github.com/Garden12138/xcli/internal/agentapi"
	xr "github.com/Garden12138/xcli/internal/runtime"
)

const MaxFrame = 1 << 20
const MaxStream = 8 << 20

var ErrProtocol = errors.New("protocol_error")

type Message struct {
	ID                    json.RawMessage
	Method                string
	Params, Result, Error json.RawMessage
}
type Frame struct {
	Message Message
	Err     error
}
type Outcome struct {
	Process xr.ProcessResult
	Err     error
}
type Client struct {
	Frames  chan Frame
	Done    chan Outcome
	input   *os.File
	output  *os.File
	cancel  context.CancelFunc
	writeMu sync.Mutex
	pending map[string]string
	next    int
	probe   bool
	closed  chan struct{}
	once    sync.Once
}

// The caller owns ctx and all process cleanup. Runtime's existing process-group
// cancellation is reused; stdin/stdout are real FDs, not blocking copy readers.
func Start(ctx context.Context, spec agent.CommandSpec, dir string, env []string, probe bool) (*Client, error) {
	inR, inW, e := os.Pipe()
	if e != nil {
		return nil, e
	}
	outR, outW, e := os.Pipe()
	if e != nil {
		inR.Close()
		inW.Close()
		return nil, e
	}
	processCtx, cancel := context.WithCancel(ctx)
	c := &Client{Frames: make(chan Frame, 32), Done: make(chan Outcome, 1), input: inW, output: outR, cancel: cancel, pending: map[string]string{}, probe: probe, closed: make(chan struct{})}
	go func() {
		p, e := xr.RunProcess(processCtx, spec, xr.ProcessOptions{Dir: dir, Env: env, Stdin: inR, Stdout: outW, SeparateProcess: true, BoundedCleanup: true})
		inR.Close()
		outW.Close()
		c.Done <- Outcome{p, e}
	}()
	go func() {
		defer close(c.Frames)
		scanner := bufio.NewScanner(outR)
		scanner.Buffer(make([]byte, 4096), MaxFrame+1)
		total := 0
		for scanner.Scan() {
			raw := scanner.Bytes()
			total += len(raw) + 1
			var m Message
			var e error
			if total > MaxStream {
				e = errors.New("output_limit")
			} else {
				m, e = Decode(raw)
			}
			select {
			case c.Frames <- Frame{m, e}:
			case <-c.closed:
				return
			}
			if e != nil {
				return
			}
		}
		if scanner.Err() != nil {
			select {
			case c.Frames <- Frame{Err: errors.New("output_limit")}:
			case <-c.closed:
			}
		}
	}()
	return c, nil
}
func Decode(raw []byte) (Message, error) {
	var fields map[string]json.RawMessage
	if len(raw) == 0 || len(raw) > MaxFrame || api.StrictDecode(raw, &fields) != nil {
		return Message{}, ErrProtocol
	}
	for key := range fields {
		if key != "id" && key != "method" && key != "params" && key != "result" && key != "error" && key != "jsonrpc" && key != "emittedAtMs" {
			return Message{}, ErrProtocol
		}
	}
	if timestamp, ok := fields["emittedAtMs"]; ok {
		var n int64
		if json.Unmarshal(timestamp, &n) != nil || n < 0 {
			return Message{}, ErrProtocol
		}
	}
	if version, ok := fields["jsonrpc"]; ok && string(version) != `"2.0"` {
		return Message{}, ErrProtocol
	}
	m := Message{ID: fields["id"], Params: fields["params"], Result: fields["result"], Error: fields["error"]}
	if method, ok := fields["method"]; ok {
		if json.Unmarshal(method, &m.Method) != nil || m.Method == "" || len(m.Method) > 128 || len(m.Result) != 0 || len(m.Error) != 0 {
			return Message{}, ErrProtocol
		}
		if len(m.Params) == 0 || len(m.Params) > 0 && m.Params[0] != '{' {
			return Message{}, ErrProtocol
		}
	} else if len(m.ID) == 0 || (len(m.Result) == 0) == (len(m.Error) == 0) || len(m.Params) != 0 {
		return Message{}, ErrProtocol
	}
	if len(m.ID) > 0 {
		var value any
		if len(m.ID) > 128 || json.Unmarshal(m.ID, &value) != nil {
			return Message{}, ErrProtocol
		}
		switch v := value.(type) {
		case string:
			if v == "" {
				return Message{}, ErrProtocol
			}
		case float64:
			if v != float64(int64(v)) {
				return Message{}, ErrProtocol
			}
		default:
			return Message{}, ErrProtocol
		}
	}
	return m, nil
}
func (c *Client) write(v any) error {
	b, e := json.Marshal(v)
	if e != nil || len(b) > MaxFrame {
		return ErrProtocol
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if e = c.input.SetWriteDeadline(time.Now().Add(250 * time.Millisecond)); e != nil {
		return e
	}
	_, e = c.input.Write(append(b, '\n'))
	return e
}

// Only server-owned code can issue these requests. The probe cannot send turns,
// threads or auth methods, even if a caller accidentally asks it to.
func (c *Client) Request(method string, params any) (string, error) {
	allowed := method == "initialize"
	if c.probe {
		allowed = allowed || method == "command/exec" || method == "fs/readFile"
	} else {
		allowed = allowed || method == "thread/start" || method == "thread/resume" || method == "turn/start" || method == "turn/interrupt"
	}
	if !allowed {
		return "", ErrProtocol
	}
	c.next++
	idBytes, _ := json.Marshal(c.next)
	id := string(idBytes)
	c.pending[id] = method
	if e := c.write(map[string]any{"id": c.next, "method": method, "params": params}); e != nil {
		return "", e
	}
	return id, nil
}
func (c *Client) Accept(m Message) (string, error) {
	method, ok := c.pending[string(m.ID)]
	if !ok || m.Method != "" {
		return "", ErrProtocol
	}
	delete(c.pending, string(m.ID))
	return method, nil
}
func (c *Client) Initialized() error {
	return c.write(map[string]any{"method": "initialized", "params": map[string]any{}})
}
func (c *Client) Reject(m Message, result any) error {
	if m.Method == "" || len(m.ID) == 0 {
		return ErrProtocol
	}
	if result == nil {
		return c.write(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": "method not permitted"}})
	}
	return c.write(map[string]any{"id": m.ID, "result": result})
}
func (c *Client) CloseInput() { c.writeMu.Lock(); defer c.writeMu.Unlock(); c.input.Close() }
func (c *Client) Close() {
	c.once.Do(func() { close(c.closed); c.cancel(); c.input.Close(); c.output.Close() })
}
