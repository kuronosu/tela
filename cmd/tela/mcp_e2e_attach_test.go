package main

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"regexp"
	"strings"
	"testing"
)

// TestAttachEndToEnd runs tela mcp --attach on a real tela dev of
// testdata/devapp (dev_e2e_test.go, a headless window): the tools of the
// attached server act on the real window through the tools socket, and a
// source edit is a reload the refs survive.
func TestAttachEndToEnd(t *testing.T) {
	dir := t.TempDir()
	e := startE2E(t, func(c *supervisorConfig) { c.tools, c.toolsDir = true, dir })
	e.expect(0, "devapp: counter", "devapp: counter n=0 pkg=0")
	e.until("the first frame", func() bool { return e.window().Frame() != nil })
	sess, err := findDevSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	tc, err := dialTools(sess.Addr)
	if err != nil {
		t.Fatal(err)
	}
	waitBuild(t, tc, 1) // the app is the session's active one
	tc.Close()
	c := startMCPServer(t, func(ctx context.Context, in io.Reader, out, stderr io.Writer) error {
		return serveAttach(ctx, mcpOptions{Attach: true, Addr: sess.Addr, Verbose: true}, in, out, stderr)
	})

	res := c.request("tools/list", map[string]any{})
	if tools, _ := res["result"].(map[string]any)["tools"].([]any); len(tools) < 17 {
		t.Fatalf("tools/list: %v", res)
	}

	r := c.tool("windows", nil)
	if r.isError || !strings.Contains(r.text, `window 1 "devapp": 480x400 logical pixels, scale 1`) {
		t.Fatalf("windows:\n%s", r.text)
	}

	r = c.tool("snapshot", nil)
	if r.isError {
		t.Fatalf("snapshot:\n%s", r.text)
	}
	inc := refOf(t, r.text, `button "Increment"`)
	if !strings.Contains(r.text, "Count: 0") {
		t.Errorf("snapshot:\n%s", r.text)
	}

	// A click by ref, at the button's real position: the app rebuilds and
	// the new snapshot is of the settled app.
	r = c.tool("click", map[string]any{"ref": inc})
	if r.isError || !strings.Contains(r.text, "Count: 1") || !strings.Contains(r.text, "Clicked button") {
		t.Fatalf("click:\n%s", r.text)
	}
	e.expect(0, "devapp: counter", "devapp: counter n=1 pkg=1")
	// Creation sites exist for what was built since the session was
	// instrumented (the first snapshot did it): the counter just was.
	if !regexp.MustCompile(`button "Increment" \[ref=e\d+\] \[src=main\.go:\d+\]`).MatchString(r.text) {
		t.Errorf("the snapshot has no source line for the rebuilt button:\n%s", r.text)
	}
	if inc2 := refOf(t, r.text, `button "Increment"`); inc2 != inc {
		t.Errorf("the ref of the button changed from %s to %s", inc, inc2)
	}

	// The perf of the click, and the component behind it.
	if r = c.tool("perf", map[string]any{"action": true}); r.isError || !strings.Contains(r.text, "frame") {
		t.Errorf("perf of the action:\n%s", r.text)
	}
	r = c.tool("state", map[string]any{"ref": inc})
	if r.isError || !strings.Contains(r.text, "main.Counter") || !strings.Contains(r.text, "UseState") {
		t.Errorf("state:\n%s", r.text)
	}
	if r = c.tool("inspect", map[string]any{"ref": inc}); r.isError || !strings.Contains(r.text, `"kind": "button"`) {
		t.Errorf("inspect:\n%s", r.text)
	}

	// A field: clicked, selected, typed.
	r = c.tool("snapshot", nil)
	field := regexp.MustCompile(`(?m)^\s*- textbox[^\n]*\[ref=(e\d+)\]`).FindStringSubmatch(r.text)
	if field == nil {
		t.Fatalf("no text field in:\n%s", r.text)
	}
	r = c.tool("fill", map[string]any{"ref": field[1], "text": "hi"})
	if r.isError {
		t.Fatalf("fill:\n%s", r.text)
	}
	e.expect(0, "devapp: input", `devapp: input "hi"`)
	if !regexp.MustCompile(`(?m)^\s*- textbox[^\n]*: hi`).MatchString(r.text) {
		t.Errorf("the snapshot after fill does not show the text:\n%s", r.text)
	}

	// The presented frame, in real pixels.
	r = c.tool("screenshot", nil)
	if r.isError || len(r.images) != 1 || !strings.Contains(r.text, "scale 1") {
		t.Fatalf("screenshot:\n%s", r.text)
	}
	img, err := png.Decode(bytes.NewReader(r.images[0]))
	if err != nil || img.Bounds().Dx() != 480 || img.Bounds().Dy() != 400 {
		t.Errorf("the screenshot is %v, %v", img.Bounds(), err)
	}
	r = c.tool("screenshot", map[string]any{"ref": inc})
	if r.isError || len(r.images) != 1 {
		t.Fatalf("element screenshot:\n%s", r.text)
	}
	if img, err = png.Decode(bytes.NewReader(r.images[0])); err != nil || img.Bounds().Dx() >= 480 || img.Bounds().Dx() < 100 {
		t.Errorf("the element screenshot is %v, %v", img.Bounds(), err)
	}

	// A source edit is a reload: the new build shows the new text, and
	// the old ref still finds the button.
	_, mark := e.edit(func(s string) string { return strings.Replace(s, `Text("Version A")`, `Text("Version B")`, 1) })
	e.expect(mark, "devapp: counter", "devapp: counter n=1 pkg=1")
	r = c.tool("snapshot", nil)
	for i := 0; i < 50 && !strings.Contains(r.text, "Version B"); i++ {
		r = c.tool("snapshot", nil)
	}
	if !strings.Contains(r.text, "Version B") || !strings.Contains(r.text, "tela dev built the app again") {
		t.Fatalf("snapshot after the reload:\n%s", r.text)
	}
	if inc2 := refOf(t, r.text, `button "Increment"`); inc2 != inc {
		t.Errorf("after the reload the button is %s, was %s", inc2, inc)
	}
	r = c.tool("click", map[string]any{"ref": inc})
	if r.isError || !strings.Contains(r.text, "Count: 2") {
		t.Fatalf("click after the reload:\n%s", r.text)
	}
	e.expect(mark, "devapp: counter", "devapp: counter n=2 pkg=2")

	// Not available here.
	if r = c.tool("resize", map[string]any{"width": 300, "height": 200}); !r.isError || !strings.Contains(r.text, "not available while attached") {
		t.Errorf("resize:\n%s", r.text)
	}
	if r = c.tool("snapshot", map[string]any{"window": 9}); !r.isError || !strings.Contains(r.text, "no window 9") {
		t.Errorf("a window that does not exist:\n%s", r.text)
	}
}

// TestAttachNoSessionDiscovery: with no tela dev running, the tools say to
// start one (the cache directory is an empty one).
func TestAttachNoSessionDiscovery(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)
	t.Setenv("LocalAppData", cache)
	s := newAttachSession("", func(string, ...any) {})
	text, res := s.tool(t, "windows", `{}`)
	if !res.IsError || !strings.Contains(text, "no tela dev session is running; start one with tela dev") {
		t.Errorf("without a session: %v %s", res.IsError, text)
	}
}
