package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fixtureLine returns the 1-based line of the fixture source that contains
// text.
func fixtureLine(t *testing.T, src, text string) int {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, text) {
			return i + 1
		}
	}
	t.Fatalf("%q is not in %s", text, src)
	return 0
}

// layoutX returns the x of the first box in a layout JSON.
func layoutX(t *testing.T, text string) float64 {
	t.Helper()
	m := regexp.MustCompile(`"x": (-?[0-9.]+)`).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no x in the layout:\n%s", text)
	}
	x, _ := strconv.ParseFloat(m[1], 64)
	return x
}

// TestMCPDevTools runs the DevTools of tela mcp (ADR-299) on the fixture:
// the source line of each element, state, perf, logs and prefer.
func TestMCPDevTools(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := mcpAppModule(t)
	t.Chdir(dir)
	src := filepath.Join(dir, "app", "mcpapp.go")
	c := startMCP(t, goBin, mcpOptions{Pkg: "./app", W: 480, H: 320, Scale: 1})
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})

	// Each element line ends with the line that made it, after its ref.
	titleLine := fixtureLine(t, src, `H1(Class("text-2xl font-bold")`)
	r := c.tool("snapshot", nil)
	want := `- heading "Version 1" [level=1] [ref=e`
	if r.isError || !strings.Contains(r.text, want) || !strings.Contains(r.text, `] [src=mcpapp.go:`+strconv.Itoa(titleLine)+`]`) {
		t.Fatalf("snapshot lacks the source line of the heading (line %d):\n%s", titleLine, r.text)
	}
	count := refOf(t, r.text, `button "Count: 0"`)
	buttonLine := fixtureLine(t, src, `Button(Class("rounded bg-sky-500`)
	if m := regexp.MustCompile(`button "Count: 0" \[ref=` + count + `\] \[src=mcpapp\.go:(\d+)\]`).FindStringSubmatch(r.text); m == nil || m[1] != strconv.Itoa(buttonLine) && m[1] != strconv.Itoa(buttonLine+1) {
		t.Errorf("the count button is made at line %d (or the next):\n%s", buttonLine, r.text)
	}
	// The layout JSON has it too.
	r = c.tool("inspect", map[string]any{"ref": count})
	if r.isError || !regexp.MustCompile(`"src": "mcpapp\.go:\d+"`).MatchString(r.text) {
		t.Errorf("the layout lacks src:\n%s", r.text)
	}

	// perf: the frames an action caused, and the last ones.
	r = c.tool("click", map[string]any{"ref": count})
	if r.isError || !strings.Contains(r.text, `button "Count: 1"`) {
		t.Fatalf("click:\n%s", r.text)
	}
	r = c.tool("perf", map[string]any{"action": true})
	if r.isError || !strings.Contains(r.text, "(the frames the last acting call caused)") || !strings.Contains(r.text, "Slowest phase:") ||
		!regexp.MustCompile(`- frame \d+: [0-9.]+ ms .*built [1-9]`).MatchString(r.text) {
		t.Errorf("perf of the action:\n%s", r.text)
	}
	r = c.tool("perf", map[string]any{"frames": 3})
	if r.isError || !strings.Contains(r.text, "(the last 3 requested)") {
		t.Errorf("perf of the last frames:\n%s", r.text)
	}

	// state: the setter that made the component render, with its line.
	r = c.tool("state", map[string]any{"ref": count})
	hookLine := fixtureLine(t, src, `count, setCount := tela.UseState(0)`)
	for _, w := range []string{"### app.form", "hook #3 UseState at mcpapp.go:" + strconv.Itoa(hookLine) + " = 1", "built 2 time(s)",
		"UseState #3 at mcpapp.go:" + strconv.Itoa(hookLine) + " was set", "mounted"} {
		if r.isError || !strings.Contains(r.text, w) {
			t.Errorf("state of the count button lacks %q:\n%s", w, r.text)
		}
	}
	r = c.tool("state", nil)
	if r.isError || !strings.Contains(r.text, "### app.form") || !strings.Contains(r.text, "components with props or hooks") {
		t.Errorf("state of the screen:\n%s", r.text)
	}

	// logs: a dev error of the screen, with its level, once.
	r = c.tool("logs", nil)
	if r.isError || !strings.Contains(r.text, "No new log entries") {
		t.Errorf("logs of a quiet screen:\n%s", r.text)
	}
	b, _ := os.ReadFile(src)
	if err := os.WriteFile(src, []byte(strings.Replace(string(b), `"rounded border px-2 py-1"`, `"rounded-xxl border px-2 py-1"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	r = c.tool("logs", map[string]any{"level": "error"})
	if r.isError || !strings.Contains(r.text, "[error] dev: ") || !strings.Contains(r.text, "rounded-xxl") {
		t.Errorf("logs after an unknown class:\n%s", r.text)
	}
	r = c.tool("logs", nil)
	if r.isError || !strings.Contains(r.text, "No new log entries") {
		t.Errorf("logs again:\n%s", r.text)
	}
	r = c.tool("logs", map[string]any{"all": true, "level": "warn"})
	if r.isError || !strings.Contains(r.text, "rounded-xxl") {
		t.Errorf("logs of everything:\n%s", r.text)
	}

	// prefer: live where the app can take it, a remount for the direction.
	r = c.tool("open", map[string]any{"preview": "Prefs"})
	if r.isError || !strings.Contains(r.text, "Reduced motion: false") {
		t.Fatalf("open Prefs:\n%s", r.text)
	}
	r = c.tool("prefer", map[string]any{"reducedMotion": true, "highContrast": true, "textScale": 1.5, "dark": true, "locale": "es"})
	for _, w := range []string{"Reduced motion: true", "High contrast: true", "Text scale: 1.5", "Dark scheme: true", "Locale: es",
		"reducedMotion: live", "highContrast: live", "textScale: live", "dark: live", "locale: live"} {
		if r.isError || !strings.Contains(r.text, w) {
			t.Errorf("prefer lacks %q:\n%s", w, r.text)
		}
	}
	if strings.Contains(r.text, "afresh") {
		t.Errorf("a live preference mounted the screen again:\n%s", r.text)
	}
	r = c.tool("snapshot", nil)
	first, second := refOf(t, r.text, `button "First"`), refOf(t, r.text, `button "Second"`)
	xOf := func(ref string) float64 {
		t.Helper()
		l := c.tool("inspect", map[string]any{"ref": ref})
		return layoutX(t, l.text)
	}
	if xOf(first) >= xOf(second) {
		t.Fatalf("in ltr the first button is not at the left of the second")
	}
	r = c.tool("prefer", map[string]any{"direction": "rtl"})
	for _, w := range []string{"Direction: rtl", "mounted the screen afresh (state reset)", "Reduced motion: true", "Text scale: 1.5", "Dark scheme: true", "Locale: es"} {
		if r.isError || !strings.Contains(r.text, w) {
			t.Errorf("prefer rtl lacks %q:\n%s", w, r.text)
		}
	}
	first, second = refOf(t, r.text, `button "First"`), refOf(t, r.text, `button "Second"`)
	if xOf(first) <= xOf(second) {
		t.Errorf("in rtl the first button is not at the right of the second")
	}
	r = c.tool("prefer", nil)
	if r.isError || !strings.Contains(r.text, "direction rtl") || !strings.Contains(r.text, "reducedMotion on") {
		t.Errorf("prefer without arguments:\n%s", r.text)
	}
	r = c.tool("prefer", map[string]any{"locale": "not a tag!"})
	if !r.isError {
		t.Errorf("an invalid locale was accepted:\n%s", r.text)
	}
	r = c.tool("prefer", map[string]any{"direction": "sideways"})
	if !r.isError || !strings.Contains(r.text, `"ltr", "rtl" or "default"`) {
		t.Errorf("an invalid direction:\n%s", r.text)
	}
}
