package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// The transport half of tela lsp: the Language Server Protocol's base
// protocol, JSON-RPC 2.0 messages framed by a Content-Length header, on
// encoding/json (the sibling of mcp_rpc.go, whose messages are one per
// line). Nothing but framed messages is ever written to the output.

// LSP error codes (and the JSON-RPC ones this server uses).
const (
	lspParseError     = -32700
	lspInvalidRequest = -32600
	lspMethodNotFound = -32601
	lspInvalidParams  = -32602
	lspInternalError  = -32603
	lspServerNotInit  = -32002
)

// lspMaxMessage bounds the body of one message, so that a corrupt header
// cannot make the server allocate without limit.
const lspMaxMessage = 64 << 20

// lspMessage is a JSON-RPC message of either direction. A request has a
// method and an id, a notification a method only, a response an id and a
// result or an error.
type lspMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// lspConn reads and writes framed messages. Writes are safe for concurrent
// use; reads are made by one goroutine.
type lspConn struct {
	r *bufio.Reader

	mu sync.Mutex // guards w
	w  io.Writer
}

func newLSPConn(in io.Reader, out io.Writer) *lspConn {
	return &lspConn{r: bufio.NewReaderSize(in, 64<<10), w: out}
}

// read returns the body of the next message. It returns io.EOF when the
// input ends between messages.
func (c *lspConn) read() ([]byte, error) {
	length := -1
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" && length < 0 {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("reading a header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if length < 0 {
				// Stray blank line between messages: tolerate it.
				continue
			}
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("malformed header %q", line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 || n > lspMaxMessage {
				return nil, fmt.Errorf("bad Content-Length %q", strings.TrimSpace(value))
			}
			length = n
		}
		// Content-Type, and any header to come, is not needed: the
		// content is UTF-8 JSON-RPC.
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, fmt.Errorf("reading a message of %d bytes: %w", length, err)
	}
	return body, nil
}

// write sends one message.
func (c *lspConn) write(msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Content-Length: %d\r\n\r\n", len(body))
	buf.Write(body)
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err = c.w.Write(buf.Bytes())
	return err
}

// respond answers request id with a result (null when result is nil).
func (c *lspConn) respond(id json.RawMessage, result any) error {
	return c.write(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  any             `json:"result"`
	}{"2.0", id, result})
}

// fail answers request id with an error.
func (c *lspConn) fail(id json.RawMessage, code int, msg string) error {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return c.write(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   rpcError        `json:"error"`
	}{"2.0", id, rpcError{Code: code, Message: msg}})
}

// notify sends a notification.
func (c *lspConn) notify(method string, params any) error {
	return c.write(struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{"2.0", method, params})
}
