package main

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestMCPRefsScrollFailFastAndCancel runs tela mcp on testdata/mcpapp and
// checks what the refs, scroll, auto-wait and cancellation of the tools do
// in a real instance: a ref follows its element when the rows before it
// go, a ref whose element is gone and a locator that cannot match fail at
// once, scroll works without an element, and a cancelled wait ends without
// a response and without losing the state of the app.
func TestMCPRefsScrollFailFastAndCancel(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("the go command is not in PATH: %v", err)
	}
	dir := mcpAppModule(t)
	t.Chdir(dir)
	c := startMCP(t, goBin, mcpOptions{Pkg: "./app", Preview: "List", W: 480, H: 320, Scale: 1})
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})

	// Three buttons with the same name: a ref is the element, not its
	// place among those with its name.
	r := c.tool("snapshot", nil)
	removes := regexp.MustCompile(`button "Remove" \[ref=(e\d+)\]`).FindAllStringSubmatch(r.text, -1)
	if r.isError || len(removes) != 3 {
		t.Fatalf("snapshot with three Remove buttons:\n%s", r.text)
	}
	first, second, third := removes[0][1], removes[1][1], removes[2][1]
	r = c.tool("click", map[string]any{"ref": first})
	_, snap, _ := strings.Cut(r.text, "### Snapshot")
	if r.isError || strings.Contains(snap, "Item 1") || !strings.Contains(snap, "Item 2") ||
		!strings.Contains(snap, `button "Remove" [ref=`+second+`]`) || !strings.Contains(snap, `button "Remove" [ref=`+third+`]`) ||
		strings.Contains(snap, "[ref="+first+"]") {
		t.Fatalf("after removing the first row the others keep their refs (%s %s) and %s is gone:\n%s", second, third, first, r.text)
	}
	r = c.tool("click", map[string]any{"ref": third})
	if r.isError || !strings.Contains(r.text, "Item 2") || strings.Contains(r.text, "Item 3") {
		t.Fatalf("a ref acts on its own row (%s is Item 3):\n%s", third, r.text)
	}
	// A ref whose element was unmounted fails at once, with a clear
	// message, and the instance goes on.
	start := time.Now()
	r = c.tool("click", map[string]any{"ref": first})
	if !r.isError || !strings.Contains(r.text, "ref "+first+" is gone: take a new snapshot") {
		t.Errorf("a ref whose element is gone:\n%s", r.text)
	}
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("a ref that is gone took %v to fail: telatest's wait is 2 s", d)
	}
	r = c.tool("click", map[string]any{"ref": third})
	if !r.isError || !strings.Contains(r.text, "ref "+third+" is gone") {
		t.Errorf("the ref of the row removed by its own click:\n%s", r.text)
	}

	// A locator that cannot match fails at once while nothing is pending.
	start = time.Now()
	r = c.tool("click", map[string]any{"role": "button", "name": "Nowhere"})
	if !r.isError || !strings.Contains(r.text, `no element matches getByRole("button", name="Nowhere")`) || !strings.Contains(r.text, "waiting would not help") {
		t.Errorf("a locator that matches nothing:\n%s", r.text)
	}
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("a locator that cannot match took %v to fail: telatest's wait is 2 s", d)
	}

	// scroll without an element is a wheel at the window's center, or at a
	// point.
	t.Run("scroll", func(t *testing.T) {
		r := c.tool("open", map[string]any{"preview": "Long"})
		if r.isError || strings.Contains(r.text, "]: Scrolled") {
			t.Fatalf("open Long:\n%s", r.text)
		}
		r = c.tool("scroll", map[string]any{"dy": 400})
		if r.isError || !strings.Contains(r.text, "over the window") || !strings.Contains(r.text, "]: Scrolled") {
			t.Errorf("scroll the window:\n%s", r.text)
		}
		r = c.tool("open", map[string]any{"preview": "Long"})
		r = c.tool("scroll", map[string]any{"dy": 100, "x": 20, "y": 200})
		if r.isError || !strings.Contains(r.text, "over the point (20, 200)") || !strings.Contains(r.text, "]: Scrolled") {
			t.Errorf("scroll at a point:\n%s", r.text)
		}
		r = c.tool("scroll", map[string]any{"dy": 100, "x": 20})
		if !r.isError || !strings.Contains(r.text, "give both x and y") {
			t.Errorf("scroll at half a point:\n%s", r.text)
		}
	})

	// A cancelled wait ends without a response, and the app keeps its
	// state: it is not mounted again.
	t.Run("cancel", func(t *testing.T) {
		r := c.tool("open", map[string]any{"preview": "Spinner"})
		if r.isError {
			t.Fatalf("open Spinner:\n%s", r.text)
		}
		r = c.tool("click", map[string]any{"role": "button", "name": "Ticks: 0"})
		if r.isError || !strings.Contains(r.text, `"Ticks: 1"`) {
			t.Fatalf("click Ticks:\n%s", r.text)
		}
		waitID, waited := c.start("tools/call", map[string]any{"name": "wait", "arguments": map[string]any{"ms": 600000}})
		time.Sleep(300 * time.Millisecond)
		select {
		case m := <-waited:
			t.Fatalf("ten minutes of a spinner ended before the cancel: %v", m)
		default:
		}
		c.notify("notifications/cancelled", map[string]any{"requestId": waitID, "reason": "test"})
		_, snapped := c.start("tools/call", map[string]any{"name": "snapshot", "arguments": map[string]any{}})
		select {
		case m := <-snapped:
			text, _ := m["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
			if !strings.Contains(text, `"Ticks: 1"`) || strings.Contains(text, "mounted afresh") {
				t.Errorf("after the cancel the app keeps its state:\n%s", text)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("the call after a cancelled wait never ran: the wait did not stop")
		}
		select {
		case m := <-waited:
			t.Errorf("a response for the cancelled wait: %v", m)
		default:
		}
	})
}
