package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sync"
)

// The protocol half of tela mcp: the Model Context Protocol's stdio
// transport, JSON-RPC 2.0 with one message per line, written on
// encoding/json (ADR-174). It implements the lifecycle (initialize, the
// initialized notification, ping) and the tools capability (tools/list,
// tools/call); the tools themselves are an mcpTools.
//
// Requests other than tools/call are answered at once on the reading
// goroutine; tool calls run one at a time, in order, on a goroutine of
// their own, so a ping is answered while a build runs. Notifications get
// no response, and neither do responses (the server sends no requests).
// Nothing else is ever written to the output.

// mcpProtocolVersions are the protocol revisions the server speaks, the
// latest first: initialize answers the client's when it is one of them, and
// the latest otherwise (the client then decides whether to go on).
var mcpProtocolVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

// JSON-RPC error codes.
const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603
)

// rpcError is a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// rpcResponse is a JSON-RPC response; ID is the request's (null when it
// could not be read).
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// mcpTool is a tool as tools/list describes it.
type mcpTool struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	InputSchema json.RawMessage  `json:"inputSchema"`
	Annotations *mcpToolAnnotate `json:"annotations,omitempty"`
}

// mcpToolAnnotate are the hints of a tool (protocol 2025-03-26 and later;
// earlier clients ignore them).
type mcpToolAnnotate struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

// mcpContent is an item of a tool result: text, or an image with its
// base64 data and MIME type.
type mcpContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// mcpResult is the result of tools/call. IsError marks a failure of the
// tool (a compile error, an element not found), which the model reads and
// can correct; protocol errors are JSON-RPC errors instead.
type mcpResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

// mcpTools is what the server serves.
type mcpTools interface {
	// list returns the tools.
	list() []mcpTool
	// call runs a tool. ok is false for an unknown tool (a protocol
	// error); bad arguments are a result with IsError, so the model can
	// correct them.
	call(ctx context.Context, name string, args json.RawMessage) (res mcpResult, ok bool)
	// instructions are the server's instructions to the model, sent with
	// the initialize result.
	instructions() string
}

// rpcServer serves one client.
type rpcServer struct {
	tools   mcpTools
	name    string
	version string
	logf    func(format string, args ...any) // -v; never nil

	mu  sync.Mutex // guards out
	out io.Writer
}

// toolCall is a tools/call waiting for the worker.
type toolCall struct {
	id   json.RawMessage
	name string
	args json.RawMessage
}

// serve reads messages from in until it ends or ctx is done, and returns
// once the tool call in progress, if any, has been answered (or abandoned
// with ctx).
func (s *rpcServer) serve(ctx context.Context, in io.Reader) error {
	calls := make(chan toolCall, 64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for c := range calls {
			if ctx.Err() != nil {
				continue
			}
			res, ok := s.tools.call(ctx, c.name, c.args)
			if !ok {
				s.write(rpcResponse{ID: c.id, Error: &rpcError{Code: rpcInvalidParams, Message: "unknown tool " + quoteJSON(c.name) + "; tools/list lists the tools"}})
				continue
			}
			if res.Content == nil {
				res.Content = []mcpContent{}
			}
			s.write(rpcResponse{ID: c.id, Result: res})
		}
	}()
	defer func() {
		close(calls)
		wg.Wait()
	}()

	// The input is read on a goroutine of its own, so that a signal (ctx)
	// ends the server while it waits for input.
	lines := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		r := bufio.NewReaderSize(in, 64<<10)
		for {
			line, err := r.ReadBytes('\n')
			if len(bytes.TrimSpace(line)) > 0 {
				select {
				case lines <- line:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				readErr <- err
				return
			}
		}
	}()
	for {
		select {
		case line := <-lines:
			s.handleLine(line, calls)
		case err := <-readErr:
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		case <-ctx.Done():
			return nil
		}
	}
}

// handleLine handles one line: a message or a batch of them.
func (s *rpcServer) handleLine(line []byte, calls chan<- toolCall) {
	line = bytes.TrimSpace(line)
	if line[0] == '[' {
		// A JSON-RPC batch (protocol 2025-03-26 allows them): the
		// responses of its requests other than tool calls go out as an
		// array; tool calls answer on their own.
		var items []json.RawMessage
		if err := json.Unmarshal(line, &items); err != nil {
			s.write(rpcResponse{ID: nullID, Error: &rpcError{Code: rpcParseError, Message: "parse error: " + err.Error()}})
			return
		}
		if len(items) == 0 {
			s.write(rpcResponse{ID: nullID, Error: &rpcError{Code: rpcInvalidRequest, Message: "invalid request: empty batch"}})
			return
		}
		var out []rpcResponse
		for _, it := range items {
			if r := s.handleMessage(it, calls); r != nil {
				out = append(out, *r)
			}
		}
		if len(out) > 0 {
			s.write(out)
		}
		return
	}
	if r := s.handleMessage(line, calls); r != nil {
		s.write(*r)
	}
}

var nullID = json.RawMessage("null")

// handleMessage handles one message and returns its response, or nil for
// a notification, a response, or a tool call (answered by the worker).
func (s *rpcServer) handleMessage(raw []byte, calls chan<- toolCall) *rpcResponse {
	if !json.Valid(raw) {
		return &rpcResponse{ID: nullID, Error: &rpcError{Code: rpcParseError, Message: "parse error: the message is not valid JSON"}}
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return &rpcResponse{ID: nullID, Error: &rpcError{Code: rpcInvalidRequest, Message: "invalid request: a message is a JSON object"}}
	}
	id, hasID := msg["id"]
	if hasID && !validID(id) {
		return &rpcResponse{ID: nullID, Error: &rpcError{Code: rpcInvalidRequest, Message: "invalid request: the id must be a string or a number"}}
	}
	if !hasID {
		id = nullID
	}
	var version string
	if v, ok := msg["jsonrpc"]; !ok || json.Unmarshal(v, &version) != nil || version != "2.0" {
		if !hasID {
			return nil // a notification gets no response, not even an error
		}
		return &rpcResponse{ID: id, Error: &rpcError{Code: rpcInvalidRequest, Message: `invalid request: "jsonrpc" must be "2.0"`}}
	}
	rawMethod, hasMethod := msg["method"]
	if !hasMethod {
		if _, isResult := msg["result"]; isResult {
			return nil // a response: the server sends no requests
		}
		if _, isError := msg["error"]; isError {
			return nil
		}
		return &rpcResponse{ID: id, Error: &rpcError{Code: rpcInvalidRequest, Message: `invalid request: no "method"`}}
	}
	var method string
	if err := json.Unmarshal(rawMethod, &method); err != nil {
		if !hasID {
			return nil
		}
		return &rpcResponse{ID: id, Error: &rpcError{Code: rpcInvalidRequest, Message: `invalid request: "method" must be a string`}}
	}
	params := msg["params"]
	if len(params) > 0 && params[0] != '{' && !bytes.Equal(params, nullID) {
		if !hasID {
			return nil
		}
		return &rpcResponse{ID: id, Error: &rpcError{Code: rpcInvalidParams, Message: "invalid params: params must be an object"}}
	}
	if !hasID {
		// notifications/initialized, notifications/cancelled (a call in
		// progress finishes; its response is then ignored), and the rest.
		s.logf("notification %s", method)
		return nil
	}
	s.logf("request %s", method)
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
			ClientInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"clientInfo"`
		}
		if len(params) > 0 && !bytes.Equal(params, nullID) {
			if err := json.Unmarshal(params, &p); err != nil {
				return &rpcResponse{ID: id, Error: &rpcError{Code: rpcInvalidParams, Message: "invalid params: " + err.Error()}}
			}
		}
		version := mcpProtocolVersions[0]
		if slices.Contains(mcpProtocolVersions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		s.logf("client %s %s, protocol %s (answered %s)", p.ClientInfo.Name, p.ClientInfo.Version, p.ProtocolVersion, version)
		return &rpcResponse{ID: id, Result: map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
			"instructions":    s.tools.instructions(),
		}}
	case "ping":
		return &rpcResponse{ID: id, Result: struct{}{}}
	case "tools/list":
		return &rpcResponse{ID: id, Result: map[string]any{"tools": s.tools.list()}}
	case "tools/call":
		var p struct {
			Name      *string         `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil || p.Name == nil {
			return &rpcResponse{ID: id, Error: &rpcError{Code: rpcInvalidParams, Message: `invalid params: tools/call needs {"name": "<tool>", "arguments": {...}}`}}
		}
		calls <- toolCall{id: id, name: *p.Name, args: p.Arguments}
		return nil
	}
	return &rpcResponse{ID: id, Error: &rpcError{Code: rpcMethodNotFound, Message: "method not found: " + method}}
}

// validID reports whether a JSON-RPC id is a string or a number (MCP
// forbids null).
func validID(id json.RawMessage) bool {
	if len(id) == 0 {
		return false
	}
	switch c := id[0]; {
	case c == '"':
		var s string
		return json.Unmarshal(id, &s) == nil
	case c == '-' || c >= '0' && c <= '9':
		var n json.Number
		return json.Unmarshal(id, &n) == nil
	}
	return false
}

// write sends one message (or a batch) as one line.
func (s *rpcServer) write(v any) {
	switch r := v.(type) {
	case rpcResponse:
		r.JSONRPC = "2.0"
		v = r
	case []rpcResponse:
		for i := range r {
			r[i].JSONRPC = "2.0"
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		// A result that cannot be encoded: say so instead.
		var id json.RawMessage = nullID
		if r, ok := v.(rpcResponse); ok {
			id = r.ID
		}
		b, _ = json.Marshal(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: rpcInternalError, Message: "internal error: " + err.Error()}})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.out.Write(append(b, '\n'))
}

// quoteJSON quotes s as a JSON string.
func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
