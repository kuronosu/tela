package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGenerateMCPSource(t *testing.T) {
	for _, c := range []contract{contractPreview, contractPreviews, contractApp} {
		for _, hasApp := range []bool{false, true} {
			if c == contractApp && !hasApp {
				continue
			}
			src := generateMCPSource("widgets", c, hasApp)
			f, err := parser.ParseFile(token.NewFileSet(), mcpGeneratedFile, src, parser.AllErrors)
			if err != nil {
				t.Fatalf("%v (App %v): the generated source does not parse: %v\n%s", c, hasApp, err, src)
			}
			if f.Name.Name != "widgets" {
				t.Errorf("%v: package %q", c, f.Name.Name)
			}
			s := string(src)
			for _, want := range []string{"func TestTelaMCP(", "func telaBuild(", "telatest.Mount(", "telatest.AllowDev(\"\")", `"\u001etela-mcp "`} {
				if !strings.Contains(s, want) {
					t.Errorf("%v: the generated source lacks %q", c, want)
				}
			}
			if strings.Contains(s, "tela/internal/") {
				t.Errorf("%v: the generated source imports an internal package, which breaks modules outside tela", c)
			}
			if hasApp != strings.Contains(s, "telatest.FromApp(telaApp)") {
				t.Errorf("%v (App %v): FromApp present = %v", c, hasApp, !hasApp)
			}
		}
	}
}

// mcpClient drives serveMCP through pipes, as an MCP client would.
type mcpClient struct {
	t      *testing.T
	in     *io.PipeWriter
	mu     sync.Mutex
	nextID int
	resps  map[int]chan map[string]any
	done   chan error
	cancel context.CancelFunc
}

// mcpAppModule creates a module outside the repository that requires tela
// through a replace directive, with testdata/mcpapp as its package app.
func mcpAppModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gomod := fmt.Sprintf("module example.com/mcpapp\n\ngo 1.27\n\nrequire github.com/kuronosu/tela v0.0.0\n\nreplace github.com/kuronosu/tela => %s\n", filepath.ToSlash(repoRoot(t)))
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("testdata", "mcpapp", "mcpapp.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "mcpapp.go"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	// Resolve the requirements from the module cache, without the network.
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	if out, err := tidy.CombinedOutput(); err != nil {
		t.Skipf("cannot resolve the dependencies of a module outside the repository offline (go mod tidy: %v):\n%s", err, out)
	}
	return dir
}

func startMCP(t *testing.T, goBin string, opts mcpOptions) *mcpClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	c := &mcpClient{t: t, in: inW, resps: map[int]chan map[string]any{}, done: make(chan error, 1), cancel: cancel}
	var stderr syncBuffer
	go func() {
		err := serveMCP(ctx, goBin, opts, inR, outW, &stderr)
		outW.Close()
		c.done <- err
	}()
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(nil, 64<<20)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("the server wrote a line that is not JSON: %.200q", sc.Text())
				continue
			}
			id, _ := m["id"].(float64)
			c.mu.Lock()
			ch := c.resps[int(id)]
			c.mu.Unlock()
			if ch == nil {
				t.Errorf("a response to no request: %.300v", m)
				continue
			}
			ch <- m
		}
	}()
	t.Cleanup(func() {
		inW.Close()
		select {
		case err := <-c.done:
			if err != nil {
				t.Errorf("serveMCP: %v", err)
			}
		case <-time.After(30 * time.Second):
			cancel()
			t.Errorf("the server did not end when its input closed")
		}
		if t.Failed() {
			t.Logf("server log:\n%s", stderr.String())
		}
	})
	return c
}

// request sends a request and waits for its response.
func (c *mcpClient) request(method string, params any) map[string]any {
	c.t.Helper()
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan map[string]any, 1)
	c.resps[id] = ch
	c.mu.Unlock()
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
	select {
	case m := <-ch:
		return m
	case <-time.After(e2eWait):
		c.t.Fatalf("no response to %s within %v", method, e2eWait)
	}
	return nil
}

// mcpToolResult is a decoded tools/call result.
type mcpToolResult struct {
	text    string
	images  [][]byte
	isError bool
}

// tool calls a tool and returns its result.
func (c *mcpClient) tool(name string, args map[string]any) mcpToolResult {
	c.t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	m := c.request("tools/call", map[string]any{"name": name, "arguments": args})
	res, ok := m["result"].(map[string]any)
	if !ok {
		c.t.Fatalf("tools/call %s: no result: %v", name, m)
	}
	var r mcpToolResult
	r.isError, _ = res["isError"].(bool)
	for _, it := range res["content"].([]any) {
		item := it.(map[string]any)
		switch item["type"] {
		case "text":
			r.text += item["text"].(string)
		case "image":
			if item["mimeType"] != "image/png" {
				c.t.Errorf("image of type %v", item["mimeType"])
			}
			b, err := base64.StdEncoding.DecodeString(item["data"].(string))
			if err != nil {
				c.t.Fatalf("image data: %v", err)
			}
			r.images = append(r.images, b)
		}
	}
	return r
}

// refOf returns the ref of the snapshot line that starts with the element
// (role and quoted name, as the snapshot prints them).
func refOf(t *testing.T, text, element string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^\s*- '?` + regexp.QuoteMeta(element) + `[^\n]*\[ref=(e\d+)\]`).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("no ref for %s in:\n%s", element, text)
	}
	return m[1]
}

// TestMCPEndToEnd runs tela mcp on testdata/mcpapp copied into a module of
// its own: real builds, a real instance, edits of its source.
func TestMCPEndToEnd(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go command is not in PATH")
	}
	dir := mcpAppModule(t)
	t.Chdir(dir)
	src := filepath.Join(dir, "app", "mcpapp.go")
	edit := func(old, new string) {
		t.Helper()
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(b, []byte(old)) {
			t.Fatalf("%q is not in the source", old)
		}
		if err := os.WriteFile(src, bytes.Replace(b, []byte(old), []byte(new), 1), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := startMCP(t, goBin, mcpOptions{Pkg: "./app", W: 480, H: 320, Scale: 1})

	initRes := c.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})
	if v := initRes["result"].(map[string]any)["protocolVersion"]; v != "2025-06-18" {
		t.Errorf("protocolVersion %v", v)
	}

	// The snapshot carries refs.
	r := c.tool("snapshot", nil)
	if r.isError || !strings.Contains(r.text, `- heading "Version 1" [level=1] [ref=e`) || !strings.Contains(r.text, "Built and mounted") {
		t.Fatalf("snapshot:\n%s", r.text)
	}
	count, name, locked := refOf(t, r.text, `button "Count: 0"`), refOf(t, r.text, `textbox "Name"`), refOf(t, r.text, `button "Locked"`)

	// A click by ref changes the snapshot; the counter keeps its ref.
	r = c.tool("click", map[string]any{"ref": count})
	if r.isError || !strings.Contains(r.text, `'button "Count: 1" [ref=`+count+`]'`) {
		t.Fatalf("click %s:\n%s", count, r.text)
	}

	// A double click at a point is one double click: OnDoubleClick of the
	// title (a block across the window, below its 24 px padding) runs.
	r = c.tool("click", map[string]any{"x": 100, "y": 40, "double": true})
	if r.isError || !strings.Contains(r.text, "Double-clicked at (100, 40).") || !strings.Contains(r.text, "The title was double-clicked") {
		t.Fatalf("double click at a point:\n%s", r.text)
	}

	// fill, then press Enter submits the form.
	r = c.tool("fill", map[string]any{"ref": name, "text": "Ana"})
	if r.isError || !strings.Contains(r.text, `textbox "Name" [ref=`+name+`]: Ana`) {
		t.Fatalf("fill:\n%s", r.text)
	}
	r = c.tool("press", map[string]any{"key": "Enter"})
	if r.isError || !strings.Contains(r.text, "Hello, Ana!") {
		t.Fatalf("press Enter:\n%s", r.text)
	}

	// A screenshot is a PNG of the window.
	r = c.tool("screenshot", nil)
	if r.isError || len(r.images) != 1 {
		t.Fatalf("screenshot: error %v, %d images\n%s", r.isError, len(r.images), r.text)
	}
	img, err := png.Decode(bytes.NewReader(r.images[0]))
	if err != nil {
		t.Fatalf("the screenshot is not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 480 || b.Dy() != 320 {
		t.Errorf("screenshot %v, want 480x320", b)
	}

	// Failed actions are errors of their call; the instance stays.
	r = c.tool("click", map[string]any{"ref": "e999"})
	if !r.isError || !strings.Contains(r.text, "unknown ref") {
		t.Errorf("unknown ref:\n%s", r.text)
	}
	r = c.tool("click", map[string]any{"ref": locked})
	if !r.isError || !strings.Contains(r.text, "disabled") {
		t.Errorf("disabled button by ref:\n%s", r.text)
	}
	r = c.tool("click", map[string]any{"role": "button", "name": "Locked"})
	if !r.isError || !strings.Contains(r.text, "is disabled") || !strings.Contains(r.text, "Hello, Ana!") {
		t.Errorf("disabled button by role (telatest's failure, with the snapshot):\n%s", r.text)
	}
	r = c.tool("click", map[string]any{"role": "button", "name": "Crash"})
	if !r.isError || !strings.Contains(r.text, "boom: the crash button") || !strings.Contains(r.text, "mounted afresh") {
		t.Errorf("a panicking handler:\n%s", r.text)
	}
	r = c.tool("click", map[string]any{"text": "Count: 0"})
	if r.isError || !strings.Contains(r.text, `button "Count: 1"`) {
		t.Fatalf("the instance does not answer after the failures:\n%s", r.text)
	}

	// An edit rebuilds and mounts afresh.
	edit(`"Version 1"`, `"Version two"`)
	r = c.tool("snapshot", nil)
	if r.isError || !strings.Contains(r.text, "rebuilt") || !strings.Contains(r.text, `heading "Version two"`) || !strings.Contains(r.text, `"Count: 0"`) {
		t.Fatalf("after an edit:\n%s", r.text)
	}

	// An unknown class: a dev error of the screen and a lint finding.
	edit(`"rounded border px-2 py-1"`, `"rounded-xxl border px-2 py-1"`)
	r = c.tool("snapshot", nil)
	if r.isError || !strings.Contains(r.text, "### Dev errors") || !strings.Contains(r.text, "### Lint") || !strings.Contains(r.text, `"rounded-2xl"`) {
		t.Errorf("an unknown class:\n%s", r.text)
	}

	// A compile error is the call's error; the last good build answers.
	edit(`const title = "Version two"`, `const title = VersionThree`)
	r = c.tool("snapshot", nil)
	if !r.isError || !strings.Contains(r.text, "undefined: VersionThree") || !strings.Contains(r.text, "last good build") {
		t.Fatalf("a compile error:\n%s", r.text)
	}
	r = c.tool("click", map[string]any{"text": "Count: 0"})
	if r.isError || !strings.Contains(r.text, `button "Count: 1"`) || !strings.Contains(r.text, `heading "Version two"`) {
		t.Fatalf("the old instance does not answer under a compile error:\n%s", r.text)
	}

	// Fixing it rebuilds.
	edit(`const title = VersionThree`, `const title = "Version three"`)
	r = c.tool("snapshot", nil)
	if r.isError || !strings.Contains(r.text, `heading "Version three"`) || !strings.Contains(r.text, `"Count: 0"`) {
		t.Fatalf("after the fix:\n%s", r.text)
	}

	// A build that compiles but whose screen cannot be mounted keeps the
	// last good one, and the session still describes that one: its
	// previews can be opened.
	const previewsDecl = `var Previews = tela.Previews{`
	const unmountable = "func Preview() tela.Node { panic(\"no screen yet\") }\n\nvar previews = tela.Previews{"
	edit(previewsDecl, unmountable)
	r = c.tool("snapshot", nil)
	if !r.isError || !strings.Contains(r.text, "cannot be mounted") || !strings.Contains(r.text, "last good build") {
		t.Fatalf("a build whose screen cannot be mounted:\n%s", r.text)
	}
	r = c.tool("open", map[string]any{"preview": "Other"})
	if r.isError || !strings.Contains(r.text, `heading "Other screen"`) {
		t.Errorf("open a preview of the running build after a failed mount:\n%s", r.text)
	}
	edit(unmountable, previewsDecl)

	// open lists the previews and mounts another.
	r = c.tool("open", nil)
	if r.isError || !strings.Contains(r.text, "Form, Other") {
		t.Errorf("open without arguments:\n%s", r.text)
	}
	r = c.tool("open", map[string]any{"preview": "Other"})
	if r.isError || !strings.Contains(r.text, `heading "Other screen"`) {
		t.Errorf("open Other:\n%s", r.text)
	}
}
