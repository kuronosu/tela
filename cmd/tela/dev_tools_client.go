package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/core"
)

// toolsClient is a client of a tela dev session's tools socket
// (dev_tools.go), for tela devtools and tela mcp --attach. Calls are safe
// from any goroutine and may run at once; the session answers each
// client's calls in order. Notifications of the events it subscribed to
// arrive on Notes, queued without bound so that a reader that is slow, or
// busy making calls, blocks nothing.
type toolsClient struct {
	nc  net.Conn
	wmu sync.Mutex // writes

	mu      sync.Mutex
	next    int64
	pending map[int64]chan toolsReply
	err     error // why the connection ended

	nmu    sync.Mutex
	queue  []toolsNote
	signal chan struct{}
	notes  chan toolsNote
	done   chan struct{}
}

// toolsNote is a notification: its event and parameters.
type toolsNote struct {
	Method string
	Params json.RawMessage
}

type toolsReply struct {
	result json.RawMessage
	err    error
}

// toolsCallError is an error a session answered a call with.
type toolsCallError struct {
	Code    int
	Message string
}

func (e *toolsCallError) Error() string { return "tela dev: " + e.Message }

// dialTools connects to the session at addr or, when addr is empty, to the
// only session running (findDevSession in the user cache's discovery
// files).
func dialTools(addr string) (*toolsClient, error) {
	if addr == "" {
		dir, err := devSessionDir()
		if err != nil {
			return nil, err
		}
		s, err := findDevSession(dir)
		if err != nil {
			return nil, err
		}
		addr = s.Addr
	}
	nc, err := net.DialTimeout("unix", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the tela dev session at %s: %w", addr, err)
	}
	c := &toolsClient{nc: nc, pending: map[int64]chan toolsReply{}, signal: make(chan struct{}, 1),
		notes: make(chan toolsNote), done: make(chan struct{})}
	go c.read()
	go c.pump()
	return c, nil
}

// Close ends the connection; the session lets go of what the client held
// (the instrumentation, its subscriptions).
func (c *toolsClient) Close() error { return c.nc.Close() }

// Notes delivers the notifications, in order; it is closed after the
// connection ended and the queue was read.
func (c *toolsClient) Notes() <-chan toolsNote { return c.notes }

// Done is closed when the connection ended.
func (c *toolsClient) Done() <-chan struct{} { return c.done }

func (c *toolsClient) read() {
	sc := bufio.NewScanner(c.nc)
	sc.Buffer(make([]byte, 64<<10), 256<<20)
	for sc.Scan() {
		var m struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		if m.ID == nil {
			if m.Method != "" {
				c.nmu.Lock()
				c.queue = append(c.queue, toolsNote{m.Method, append(json.RawMessage(nil), m.Params...)})
				c.nmu.Unlock()
				select {
				case c.signal <- struct{}{}:
				default:
				}
			}
			continue
		}
		c.mu.Lock()
		ch := c.pending[*m.ID]
		delete(c.pending, *m.ID)
		c.mu.Unlock()
		if ch == nil {
			continue
		}
		r := toolsReply{result: append(json.RawMessage(nil), m.Result...)}
		if m.Error != nil {
			r.err = &toolsCallError{m.Error.Code, m.Error.Message}
		}
		ch <- r
	}
	err := sc.Err()
	if err == nil {
		err = errors.New("the tela dev session closed the connection")
	}
	c.mu.Lock()
	c.err = err
	for id, ch := range c.pending {
		ch <- toolsReply{err: err}
		delete(c.pending, id)
	}
	c.mu.Unlock()
	close(c.done)
}

// pump hands the queued notifications to Notes.
func (c *toolsClient) pump() {
	defer close(c.notes)
	for {
		c.nmu.Lock()
		q := c.queue
		c.queue = nil
		c.nmu.Unlock()
		for _, n := range q {
			c.notes <- n
		}
		if len(q) > 0 {
			continue
		}
		select {
		case <-c.signal:
		case <-c.done:
			c.nmu.Lock()
			empty := len(c.queue) == 0
			c.nmu.Unlock()
			if empty {
				return
			}
		}
	}
}

// call calls method with params and decodes its result into result (nil:
// ignored).
func (c *toolsClient) call(ctx context.Context, method string, params, result any) error {
	if params == nil {
		params = struct{}{}
	}
	ch := make(chan toolsReply, 1)
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.next++
	id := c.next
	c.pending[id] = ch
	c.mu.Unlock()
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	c.wmu.Lock()
	_, err = c.nc.Write(append(b, '\n'))
	c.wmu.Unlock()
	if err != nil {
		return err
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if result != nil {
			return json.Unmarshal(r.result, result)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// --- typed calls ---

// toolsInfo is the session's info.
type toolsInfo struct {
	Pkg      string    `json:"pkg"`
	PID      int       `json:"pid"`
	Started  time.Time `json:"started"`
	Build    int       `json:"build"`
	Protocol int       `json:"protocol"`
}

func (c *toolsClient) Info(ctx context.Context) (toolsInfo, error) {
	var r toolsInfo
	return r, c.call(ctx, "info", nil, &r)
}

func (c *toolsClient) Windows(ctx context.Context) ([]toolsWindow, error) {
	var r struct {
		Windows []toolsWindow `json:"windows"`
	}
	return r.Windows, c.call(ctx, "windows", nil, &r)
}

// Subscribe subscribes to events (picked, frames, logs, reload, windows).
func (c *toolsClient) Subscribe(ctx context.Context, events ...string) error {
	return c.call(ctx, "subscribe", map[string]any{"events": events}, nil)
}

func (c *toolsClient) Unsubscribe(ctx context.Context, events ...string) error {
	return c.call(ctx, "unsubscribe", map[string]any{"events": events}, nil)
}

func (c *toolsClient) Instrument(ctx context.Context, on bool) error {
	return c.call(ctx, "instrument", map[string]any{"on": on}, nil)
}

func (c *toolsClient) Elements(ctx context.Context, window uint32, detail bool) ([]core.ElementInfo, error) {
	var r struct {
		Elements []core.ElementInfo `json:"elements"`
	}
	return r.Elements, c.call(ctx, "elements", map[string]any{"window": window, "detail": detail}, &r)
}

func (c *toolsClient) Element(ctx context.Context, window uint32, id core.ElementID) (core.ElementInfo, error) {
	var r core.ElementInfo
	return r, c.call(ctx, "element", map[string]any{"window": window, "id": id}, &r)
}

// Inspect returns the layout of element id, or of the whole tree for 0.
func (c *toolsClient) Inspect(ctx context.Context, window uint32, id core.ElementID) (core.InspectNode, error) {
	var r core.InspectNode
	return r, c.call(ctx, "inspect", map[string]any{"window": window, "id": id}, &r)
}

// toolsSemantics is the aria snapshot of a window with, per line, its node
// and element (0 for text and /url lines).
type toolsSemantics struct {
	YAML     string           `json:"yaml"`
	Nodes    []uint64         `json:"nodes"`
	Elements []core.ElementID `json:"elements"`
	// Srcs is, per line, where the app's code made its element
	// ("settings.go:42", "" for none).
	Srcs []string `json:"srcs"`
}

func (c *toolsClient) Semantics(ctx context.Context, window uint32) (toolsSemantics, error) {
	var r toolsSemantics
	return r, c.call(ctx, "semantics", map[string]any{"window": window}, &r)
}

// Profiles returns the frames recorded after frame since.
func (c *toolsClient) Profiles(ctx context.Context, window uint32, since int) ([]core.FrameProfile, error) {
	var r struct {
		Profiles []core.FrameProfile `json:"profiles"`
	}
	return r.Profiles, c.call(ctx, "profiles", map[string]any{"window": window, "since": since}, &r)
}

// Logs returns the entries after since and the last sequence number.
func (c *toolsClient) Logs(ctx context.Context, since uint64) ([]toolsLogEntry, uint64, error) {
	var r struct {
		Entries []toolsLogEntry `json:"entries"`
		Last    uint64          `json:"last"`
	}
	err := c.call(ctx, "logs", map[string]any{"since": since}, &r)
	return r.Entries, r.Last, err
}

// Pick turns pick mode on or off; the element picked comes as a picked
// notification (toolsPicked).
func (c *toolsClient) Pick(ctx context.Context, window uint32, on bool) error {
	return c.call(ctx, "pick", map[string]any{"window": window, "on": on}, nil)
}

// toolsPicked is a picked notification.
type toolsPicked struct {
	Window  uint32            `json:"window"`
	ID      core.ElementID    `json:"id"`
	Element *core.ElementInfo `json:"element"`
}

// toolsFrames is a frames notification.
type toolsFrames struct {
	Window   uint32              `json:"window"`
	Profiles []core.FrameProfile `json:"profiles"`
}

func (c *toolsClient) Highlight(ctx context.Context, window uint32, id core.ElementID) error {
	return c.call(ctx, "highlight", map[string]any{"window": window, "id": id}, nil)
}

// Screenshot returns the frame the window presents, at the app's
// resolution.
func (c *toolsClient) Screenshot(ctx context.Context, window uint32) (image.Image, error) {
	var r struct {
		PNG string `json:"png"`
	}
	if err := c.call(ctx, "screenshot", map[string]any{"window": window}, &r); err != nil {
		return nil, err
	}
	b, err := base64.StdEncoding.DecodeString(r.PNG)
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(b))
}

// Pointer injects a pointer action ("move", "down", "up", "click") of
// button ("left" by default) at the logical point x, y.
func (c *toolsClient) Pointer(ctx context.Context, window uint32, action string, x, y float32, button string) error {
	return c.call(ctx, "pointer", map[string]any{"window": window, "action": action, "x": x, "y": y, "button": button}, nil)
}

// Clicks injects count clicks (2: a double click) of button at x, y.
func (c *toolsClient) Clicks(ctx context.Context, window uint32, x, y float32, button string, count int) error {
	return c.call(ctx, "pointer", map[string]any{"window": window, "action": "click", "x": x, "y": y, "button": button, "count": count}, nil)
}

// Click is Pointer's left click.
func (c *toolsClient) Click(ctx context.Context, window uint32, x, y float32) error {
	return c.Pointer(ctx, window, "click", x, y, "")
}

// Scroll injects a wheel of dx, dy logical pixels at x, y.
func (c *toolsClient) Scroll(ctx context.Context, window uint32, x, y, dx, dy float32) error {
	return c.call(ctx, "scroll", map[string]any{"window": window, "x": x, "y": y, "dx": dx, "dy": dy}, nil)
}

// Key presses and releases a combination in telatest.Press syntax
// ("Control+s"); action "press" or "release" sends only that half.
func (c *toolsClient) Key(ctx context.Context, window uint32, combo, action string) error {
	return c.call(ctx, "key", map[string]any{"window": window, "key": combo, "action": action}, nil)
}

// Text types text as key presses.
func (c *toolsClient) Text(ctx context.Context, window uint32, text string) error {
	return c.call(ctx, "text", map[string]any{"window": window, "text": text}, nil)
}
