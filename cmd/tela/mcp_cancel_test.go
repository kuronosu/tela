package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

// cancelTools is a fakeTools whose calls wait until release is closed or
// their context ends, and say which of the two happened.
type cancelTools struct {
	fakeTools
	started chan string // the arguments of each call that begins
	release chan struct{}
	stopped chan string // the arguments of each call whose context ended
}

func (c *cancelTools) call(ctx context.Context, name string, args json.RawMessage) (mcpResult, bool) {
	c.started <- string(args)
	select {
	case <-ctx.Done():
		c.stopped <- string(args)
		return mcpResult{Content: []mcpContent{{Type: "text", Text: "stopped"}}, IsError: true}, true
	case <-c.release:
		return c.fakeTools.call(ctx, name, args)
	}
}

// rpcPipe is a server on pipes, read as a client would.
type rpcPipe struct {
	t     *testing.T
	in    *io.PipeWriter
	resps chan map[string]any
	done  chan error
	once  sync.Once
}

func startRPC(t *testing.T, tools mcpTools) *rpcPipe {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	p := &rpcPipe{t: t, in: inW, resps: make(chan map[string]any, 64), done: make(chan error, 1)}
	s := &rpcServer{tools: tools, name: "tela", version: "v0.0.0-test", logf: func(string, ...any) {}, out: outW}
	go func() {
		p.done <- s.serve(context.Background(), inR)
		outW.Close()
	}()
	go func() {
		defer close(p.resps)
		sc := bufio.NewScanner(outR)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			var m map[string]any
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				p.resps <- m
			} else {
				var batch []map[string]any
				if json.Unmarshal(sc.Bytes(), &batch) == nil {
					p.resps <- map[string]any{"batch": batch}
				}
			}
		}
	}()
	t.Cleanup(func() { p.close() })
	return p
}

func (p *rpcPipe) send(line string) {
	p.t.Helper()
	if _, err := io.WriteString(p.in, line+"\n"); err != nil {
		p.t.Fatal(err)
	}
}

func (p *rpcPipe) call(id any, args string) {
	p.send(fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"method":"tools/call","params":{"name":"echo","arguments":%s}}`, id, args))
}

func (p *rpcPipe) cancel(requestID any, reason string) {
	p.send(fmt.Sprintf(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":%v,"reason":%q}}`, requestID, reason))
}

// close ends the input and waits for the server.
func (p *rpcPipe) close() {
	p.once.Do(func() {
		p.in.Close()
		select {
		case err := <-p.done:
			if err != nil {
				p.t.Errorf("serve: %v", err)
			}
		case <-time.After(10 * time.Second):
			p.t.Error("serve did not return after its input ended")
		}
	})
}

// next returns the next response, or fails after a timeout.
func (p *rpcPipe) next() map[string]any {
	p.t.Helper()
	select {
	case m, ok := <-p.resps:
		if !ok {
			p.t.Fatal("the server ended without the expected response")
		}
		return m
	case <-time.After(10 * time.Second):
		p.t.Fatal("no response within 10 s")
	}
	return nil
}

func expectStart(t *testing.T, c *cancelTools, args string) {
	t.Helper()
	select {
	case got := <-c.started:
		if got != args {
			t.Fatalf("call started with %s, want %s", got, args)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("the call %s did not start", args)
	}
}

// TestMCPCancelledStopsARunningCall checks that notifications/cancelled
// ends the context of the call in progress and that the server then sends
// no response for it (MCP: the receiver should stop and not respond), while
// the rest of the session goes on.
func TestMCPCancelledStopsARunningCall(t *testing.T) {
	tools := &cancelTools{started: make(chan string, 8), release: make(chan struct{}), stopped: make(chan string, 8)}
	p := startRPC(t, tools)
	p.call(1, `{"long":true}`)
	expectStart(t, tools, `{"long":true}`)
	p.cancel(1, "the user pressed escape")
	select {
	case got := <-tools.stopped:
		if got != `{"long":true}` {
			t.Errorf("stopped %s", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the context of the running call did not end after notifications/cancelled")
	}
	// The next call runs and is answered; nothing came for call 1.
	close(tools.release)
	p.call(2, `{"after":true}`)
	m := p.next()
	if m["id"] != 2.0 || m["result"] == nil {
		t.Fatalf("the first response is %v, want the one of call 2 (none for the cancelled call)", m)
	}
	p.close()
	for m := range p.resps {
		t.Errorf("a response after the session ended: %v", m)
	}
}

// TestMCPCancelledBeforeItStarts cancels a call that waits in the queue
// behind another: it never runs and gets no response.
func TestMCPCancelledBeforeItStarts(t *testing.T) {
	tools := &cancelTools{started: make(chan string, 8), release: make(chan struct{}), stopped: make(chan string, 8)}
	p := startRPC(t, tools)
	p.call(1, `{"first":true}`)
	expectStart(t, tools, `{"first":true}`)
	p.call(`"two"`, `{"queued":true}`)
	p.call(3, `{"third":true}`)
	p.cancel(`"two"`, "")
	// An id of another type, an unknown id and a malformed notification
	// cancel nothing.
	p.cancel(2, "")
	p.cancel(99, "")
	p.send(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{}}`)
	p.send(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`) // answered at once: reading went on
	if m := p.next(); m["id"] != "ping" {
		t.Fatalf("first response %v, want the ping", m)
	}
	close(tools.release)
	if m := p.next(); m["id"] != 1.0 {
		t.Fatalf("response %v, want call 1", m)
	}
	if m := p.next(); m["id"] != 3.0 {
		t.Fatalf("response %v, want call 3 (call \"two\" was cancelled)", m)
	}
	expectStart(t, tools, `{"third":true}`)
	p.close()
	select {
	case got := <-tools.started:
		t.Errorf("the cancelled call ran: %s", got)
	default:
	}
	for m := range p.resps {
		t.Errorf("an unexpected response: %v", m)
	}
}

// TestMCPCancelledCallInABatch checks that a batch is answered with the
// responses of the calls that were not cancelled, and not at all when all
// of its calls were.
func TestMCPCancelledCallInABatch(t *testing.T) {
	tools := &cancelTools{started: make(chan string, 8), release: make(chan struct{}), stopped: make(chan string, 8)}
	p := startRPC(t, tools)
	call := func(id int) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"echo","arguments":{"n":%d}}}`, id, id)
	}
	p.send("[" + call(1) + "," + call(2) + "]")
	expectStart(t, tools, `{"n":1}`)
	p.cancel(2, "")
	p.send("[" + call(3) + "]")
	p.cancel(3, "")
	p.send(`{"jsonrpc":"2.0","id":"ping","method":"ping"}`) // answered once the cancels were read
	if m := p.next(); m["id"] != "ping" {
		t.Fatalf("first response %v, want the ping", m)
	}
	close(tools.release)
	m := p.next()
	batch, _ := m["batch"].([]map[string]any)
	if len(batch) != 1 || batch[0]["id"] != 1.0 {
		t.Fatalf("batch response %v, want only call 1's", m)
	}
	p.close()
	for m := range p.resps {
		t.Errorf("a response for the all-cancelled batch: %v", m)
	}
}
