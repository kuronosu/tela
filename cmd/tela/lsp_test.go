package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/lint"
)

// lspClient is a client of tela lsp over pipes: it frames what it sends,
// reads what the server writes on a goroutine, and keeps the notifications
// it has not been asked for.
type lspClient struct {
	t    *testing.T
	in   *io.PipeWriter
	msgs chan lspMessage
	done chan int // the exit status of the server

	nextID int
	notes  []lspMessage // notifications read while waiting for something else
}

// startLSP serves tela lsp to the test in process, with the process's
// cache of the module context, as tela lsp does. The tests that lint run in
// parallel: the first lint loads what the module needs, and the cache
// makes the others check only what they change.
func startLSP(t *testing.T) *lspClient {
	t.Helper()
	return startLSPCache(t, lspCache())
}

// startLSPCache is startLSP with a cache of the test's own.
func startLSPCache(t *testing.T, cache *lint.Cache) *lspClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &lspClient{t: t, in: inW, msgs: make(chan lspMessage, 1024), done: make(chan int, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		code := serveLSP(ctx, inR, outW, func(format string, args ...any) { t.Logf("server: "+format, args...) }, cache)
		outW.Close()
		c.done <- code
	}()
	go func() {
		conn := newLSPConn(outR, io.Discard)
		for {
			body, err := conn.read()
			if err != nil {
				close(c.msgs)
				return
			}
			var m lspMessage
			if err := json.Unmarshal(body, &m); err != nil {
				t.Errorf("the server wrote %q, which is not JSON: %v", body, err)
				continue
			}
			c.msgs <- m
		}
	}()
	t.Cleanup(func() {
		cancel()
		inW.Close()
	})
	return c
}

func (c *lspClient) send(v any) {
	c.t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
		c.t.Fatalf("writing to the server: %v", err)
	}
}

func (c *lspClient) notify(method string, params any) {
	c.t.Helper()
	c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// request sends a request and returns its result (JSON) or error.
func (c *lspClient) request(method string, params any) (json.RawMessage, *rpcError) {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	want := fmt.Sprint(id)
	timeout := time.After(60 * time.Second)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				c.t.Fatalf("the server closed its output while %s was waiting for an answer", method)
			}
			if m.Method != "" {
				c.notes = append(c.notes, m)
				continue
			}
			if string(m.ID) != want {
				c.t.Fatalf("answer to request %s, want %s", m.ID, want)
			}
			return m.Result, m.Error
		case <-timeout:
			c.t.Fatalf("no answer to %s", method)
		}
	}
}

// mustRequest is request for a request that must succeed.
func (c *lspClient) mustRequest(method string, params any) json.RawMessage {
	c.t.Helper()
	res, rerr := c.request(method, params)
	if rerr != nil {
		c.t.Fatalf("%s: error %d: %s", method, rerr.Code, rerr.Message)
	}
	return res
}

// diagnostics waits for a publishDiagnostics of uri that satisfies ok.
func (c *lspClient) diagnostics(uri string, ok func([]lspDiagnostic) bool) []lspDiagnostic {
	c.t.Helper()
	check := func(m lspMessage) ([]lspDiagnostic, bool) {
		if m.Method != "textDocument/publishDiagnostics" {
			return nil, false
		}
		var p struct {
			URI         string          `json:"uri"`
			Diagnostics []lspDiagnostic `json:"diagnostics"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil || p.URI != uri {
			return nil, false
		}
		return p.Diagnostics, ok(p.Diagnostics)
	}
	for i, m := range c.notes {
		if d, found := check(m); found {
			c.notes = c.notes[i+1:]
			return d
		}
	}
	c.notes = nil
	timeout := time.After(90 * time.Second)
	for {
		select {
		case m, open := <-c.msgs:
			if !open {
				c.t.Fatalf("the server closed its output while waiting for diagnostics of %s", uri)
			}
			if d, found := check(m); found {
				return d
			}
		case <-timeout:
			c.t.Fatalf("no matching diagnostics of %s", uri)
		}
	}
}

// note waits for a notification of the method.
func (c *lspClient) note(method string) lspMessage {
	c.t.Helper()
	for i, m := range c.notes {
		if m.Method == method {
			c.notes = append(c.notes[:i:i], c.notes[i+1:]...)
			return m
		}
	}
	timeout := time.After(60 * time.Second)
	for {
		select {
		case m, ok := <-c.msgs:
			if !ok {
				c.t.Fatalf("the server closed its output while waiting for %s", method)
			}
			if m.Method == method {
				return m
			}
			if m.Method != "" {
				c.notes = append(c.notes, m)
			}
		case <-timeout:
			c.t.Fatalf("no %s notification", method)
		}
	}
}

func (c *lspClient) initialize() map[string]any {
	c.t.Helper()
	res := c.mustRequest("initialize", map[string]any{
		"processId": nil, "rootUri": nil,
		"capabilities": map[string]any{"general": map[string]any{"positionEncodings": []string{"utf-16"}}},
	})
	c.notify("initialized", map[string]any{})
	var out map[string]any
	if err := json.Unmarshal(res, &out); err != nil {
		c.t.Fatal(err)
	}
	return out
}

func (c *lspClient) open(uri, text string) {
	c.t.Helper()
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": uri, "languageId": "go", "version": 1, "text": text,
	}})
}

func (c *lspClient) change(uri string, version int, text string) {
	c.t.Helper()
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]any{{"text": text}},
	})
}

func (c *lspClient) completion(uri string, pos lspPosition) *lspCompletionList {
	c.t.Helper()
	res := c.mustRequest("textDocument/completion", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": pos})
	if string(res) == "null" {
		return nil
	}
	var list lspCompletionList
	if err := json.Unmarshal(res, &list); err != nil {
		c.t.Fatalf("completion result %s: %v", res, err)
	}
	return &list
}

// hover returns the Markdown of the hover, or "" for none.
func (c *lspClient) hover(uri string, pos lspPosition) string {
	c.t.Helper()
	res := c.mustRequest("textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": uri}, "position": pos})
	if string(res) == "null" {
		return ""
	}
	var h struct {
		Contents struct{ Kind, Value string }
	}
	if err := json.Unmarshal(res, &h); err != nil {
		c.t.Fatalf("hover result %s: %v", res, err)
	}
	if h.Contents.Kind != "markdown" {
		c.t.Errorf("hover kind %q, want markdown", h.Contents.Kind)
	}
	return h.Contents.Value
}

// fileURI returns the file: URI of a path.
func fileURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// withCursor returns text without its "|" marker and the position of it.
func withCursor(t *testing.T, text string) (string, lspPosition) {
	t.Helper()
	i := strings.Index(text, "|")
	if i < 0 {
		t.Fatalf("no cursor in %q", text)
	}
	text = text[:i] + text[i+1:]
	d := &lspDoc{text: text}
	return text, d.position(i)
}

// bufferFor returns the path and URI of a buffer in the fixture package,
// that does not exist on disk.
func bufferFor(t *testing.T, name string) (path, uri string) {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "lspapp"))
	if err != nil {
		t.Fatal(err)
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	path = filepath.Join(dir, name)
	return path, fileURI(path)
}

const lspBufferHead = `package lspapp

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

`

func TestLSPLifecycle(t *testing.T) {
	c := startLSP(t)
	// Nothing is served before initialize.
	if _, rerr := c.request("textDocument/hover", map[string]any{}); rerr == nil || rerr.Code != lspServerNotInit {
		t.Errorf("hover before initialize: %+v, want error %d", rerr, lspServerNotInit)
	}
	res := c.initialize()
	caps, _ := res["capabilities"].(map[string]any)
	if caps["hoverProvider"] != true || caps["completionProvider"] == nil {
		t.Errorf("capabilities = %v, want hover and completion", caps)
	}
	sync, _ := caps["textDocumentSync"].(map[string]any)
	if sync["openClose"] != true || sync["change"] != float64(1) {
		t.Errorf("textDocumentSync = %v, want open/close and full sync", sync)
	}
	if caps["positionEncoding"] != "utf-16" {
		t.Errorf("positionEncoding = %v, want utf-16", caps["positionEncoding"])
	}
	if info, _ := res["serverInfo"].(map[string]any); info["name"] != "tela lsp" {
		t.Errorf("serverInfo = %v", res["serverInfo"])
	}
	// What gopls serves is not served here, and says so.
	_, rerr := c.request("textDocument/definition", map[string]any{})
	if rerr == nil || rerr.Code != lspMethodNotFound || !strings.Contains(rerr.Message, "gopls") {
		t.Errorf("definition: %+v, want method not found mentioning gopls", rerr)
	}
	// Shutdown, then exit: status 0.
	if res := c.mustRequest("shutdown", nil); string(res) != "null" {
		t.Errorf("shutdown result %s, want null", res)
	}
	if _, rerr := c.request("textDocument/hover", map[string]any{}); rerr == nil || rerr.Code != lspInvalidRequest {
		t.Errorf("request after shutdown: %+v, want invalid request", rerr)
	}
	c.notify("exit", nil)
	select {
	case code := <-c.done:
		if code != 0 {
			t.Errorf("exit status %d after shutdown, want 0", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not stop after exit")
	}
}

func TestLSPExitWithoutShutdown(t *testing.T) {
	c := startLSP(t)
	c.initialize()
	c.notify("exit", nil)
	select {
	case code := <-c.done:
		if code != 1 {
			t.Errorf("exit status %d without shutdown, want 1", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server did not stop after exit")
	}
}

func TestLSPFraming(t *testing.T) {
	// Extra headers, a lowercase name and a blank line between messages
	// are accepted; a message may be split across writes.
	in, out := io.Pipe()
	c := newLSPConn(in, io.Discard)
	go func() {
		body := `{"jsonrpc":"2.0","method":"a"}`
		fmt.Fprintf(out, "\r\ncontent-length: %d\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n", len(body))
		out.Write([]byte(body[:10]))
		time.Sleep(10 * time.Millisecond)
		out.Write([]byte(body[10:]))
		out.Close()
	}()
	body, err := c.read()
	if err != nil || string(body) != `{"jsonrpc":"2.0","method":"a"}` {
		t.Fatalf("read = %q, %v", body, err)
	}
	if _, err := c.read(); err != io.EOF {
		t.Errorf("read at the end = %v, want io.EOF", err)
	}
	bad := newLSPConn(strings.NewReader("Content-Length: nope\r\n\r\n"), io.Discard)
	if _, err := bad.read(); err == nil || !strings.Contains(err.Error(), "Content-Length") {
		t.Errorf("bad Content-Length: err = %v", err)
	}
}

// Completion and hover read the text only: a file outside any module has no
// diagnostics (the client is told why, once) and still has them.
func TestLSPOutsideModule(t *testing.T) {
	c := startLSP(t)
	c.initialize()
	path := filepath.Join(t.TempDir(), "main.go")
	uri := fileURI(path)
	text, pos := withCursor(t, "package main\n\nfunc main() { Class(\"p-|\") }\n")
	c.open(uri, text)
	if got := c.completion(uri, pos); got == nil || !slices.ContainsFunc(got.Items, func(i lspCompletionItem) bool { return i.Label == "p-4" && i.Detail == "padding: 1rem" }) {
		t.Errorf("completion outside a module = %+v, want p-4", got)
	}
	m := c.note("window/logMessage")
	var lm struct{ Message string }
	if err := json.Unmarshal(m.Params, &lm); err != nil || !strings.Contains(lm.Message, "cannot lint") {
		t.Fatalf("logMessage %s, want the reason the package cannot be linted", m.Params)
	}
}

func TestLSPBadInput(t *testing.T) {
	c := startLSP(t)
	c.initialize()
	c.send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "textDocument/hover", "params": "nope"})
	select {
	case m := <-c.msgs:
		if m.Error == nil || m.Error.Code != lspInvalidParams {
			t.Errorf("hover with bad parameters: %+v, want error %d", m, lspInvalidParams)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no answer")
	}
	// An unknown notification is ignored: the next request is answered.
	c.notify("$/setTrace", map[string]any{"value": "off"})
	c.mustRequest("shutdown", nil)
}

func TestLSPCommand(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{"help", "lsp"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "tela lsp [-v]") {
		t.Errorf("help lsp: code %d, output %q", code, out.String())
	}
	out.Reset()
	if code := run([]string{"lsp", "./x"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "takes no arguments") {
		t.Errorf("lsp with an argument: code %d, stderr %q", code, errOut.String())
	}
	out.Reset()
	if code := run([]string{"help"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "lsp ") {
		t.Errorf("help does not list lsp: %q", out.String())
	}
}

func TestLSPDiagnostics(t *testing.T) {
	t.Parallel()
	needGo(t) // it lints, which runs go list
	c := startLSP(t)
	c.initialize()
	path, uri := bufferFor(t, "buffer.go")
	_ = path
	// The emoji is four bytes and two UTF-16 units: the range must count
	// units, not bytes or runes.
	text := lspBufferHead + `func Buffer() tela.Node {
	return Div(Text("😀"), Class("flex bg-rde-500 btn"))
}
`
	c.open(uri, text)
	diags := c.diagnostics(uri, func(d []lspDiagnostic) bool { return len(d) > 0 })
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %+v, want one", diags)
	}
	d := diags[0]
	if d.Source != "tela lint" || d.Code != "class" || d.Severity != 1 ||
		!strings.Contains(d.Message, `unknown class "bg-rde-500"`) || !strings.Contains(d.Message, `did you mean "bg-red-500"`) {
		t.Errorf("diagnostic = %+v, want the class rule's unknown class with the suggestion", d)
	}
	doc := &lspDoc{text: text}
	// The class starts at the b of bg-rde-500, after Class("flex , and
	// the emoji counts two units.
	wantCol := utf16Col(t, text, 8, `bg-rde-500`)
	want := lspRange{Start: lspPosition{8, wantCol}, End: lspPosition{8, wantCol + len("bg-rde-500")}}
	if d.Range != want {
		t.Errorf("range = %+v, want %+v (text %q)", d.Range, want, text[doc.offset(want.Start):doc.offset(want.End)])
	}

	// Fixing the buffer clears the diagnostics, without a save.
	c.change(uri, 2, strings.Replace(text, "bg-rde-500", "bg-red-500", 1))
	c.diagnostics(uri, func(d []lspDiagnostic) bool { return len(d) == 0 })

	// A buffer that does not compile (an unrelated type error) is still
	// linted.
	broken := strings.Replace(text, "func Buffer", "var _ int = \"x\"\n\nfunc Buffer", 1)
	c.change(uri, 3, broken)
	diags = c.diagnostics(uri, func(d []lspDiagnostic) bool { return len(d) > 0 })
	if len(diags) != 1 || diags[0].Code != "class" {
		t.Errorf("diagnostics of a buffer with a type error = %+v, want the class finding", diags)
	}

	// Closing the document clears what was published for it.
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": uri}})
	c.diagnostics(uri, func(d []lspDiagnostic) bool { return len(d) == 0 })
}

// utf16Col returns the UTF-16 column of the first occurrence of sub in
// line l of text.
func utf16Col(t *testing.T, text string, l int, sub string) int {
	t.Helper()
	d := &lspDoc{text: text}
	line := d.lineText(l)
	i := strings.Index(line, sub)
	if i < 0 {
		t.Fatalf("%q not in line %d %q", sub, l, line)
	}
	return len(utf16Of(line[:i]))
}

func utf16Of(s string) []uint16 {
	var out []uint16
	for _, r := range s {
		if r >= 0x10000 {
			out = append(out, 0, 0)
		} else {
			out = append(out, 0)
		}
	}
	return out
}

func TestLSPCompletion(t *testing.T) {
	t.Parallel()
	c := startLSP(t)
	c.initialize()
	_, uri := bufferFor(t, "complete.go")

	open := func(body string) lspPosition {
		t.Helper()
		text, pos := withCursor(t, lspBufferHead+body)
		c.open(uri, text)
		return pos
	}
	byLabel := func(list *lspCompletionList, label string) *lspCompletionItem {
		t.Helper()
		if list == nil {
			t.Fatalf("no completion list, want one with %q", label)
		}
		for i := range list.Items {
			if list.Items[i].Label == label {
				return &list.Items[i]
			}
		}
		t.Fatalf("no item %q among %d", label, len(list.Items))
		return nil
	}

	// A class prefix, answered at once: no type check needed.
	pos := open("func screen() tela.Node { return Div(Class(\"flex bg-|\")) }\n")
	list := c.completion(uri, pos)
	item := byLabel(list, "bg-red-500")
	if item.Detail != "background-color: #fb2c36" || item.Kind != lspKindColor || item.Documentation != "#fb2c36" {
		t.Errorf("bg-red-500 = %+v, want the color in the detail and as a color item", item)
	}
	if item.TextEdit == nil || item.TextEdit.NewText != "bg-red-500" ||
		item.TextEdit.Range.Start.Character != pos.Character-len("bg-") || item.TextEdit.Range.End.Character != pos.Character {
		t.Errorf("bg-red-500 edit = %+v, want to replace the %q typed before the cursor %+v", item.TextEdit, "bg-", pos)
	}
	if got := byLabel(list, "bg-center").Detail; got != "background-position: center" {
		t.Errorf("bg-center detail = %q", got)
	}
	for _, it := range list.Items {
		if !strings.HasPrefix(it.Label, "bg-") {
			t.Errorf("item %q does not start with the prefix", it.Label)
		}
	}

	// A spacing class with its CSS.
	text, pos := withCursor(t, lspBufferHead+"func screen() tela.Node { return Div(Class(\"p-|\")) }\n")
	c.change(uri, 2, text)
	if got := byLabel(c.completion(uri, pos), "p-4").Detail; got != "padding: 1rem" {
		t.Errorf("p-4 detail = %q, want padding: 1rem", got)
	}

	// Variants stack: the edit replaces only what follows the last colon.
	text, pos = withCursor(t, lspBufferHead+"func screen() tela.Node { return Div(Class(\"md:hover:bg-|\")) }\n")
	c.change(uri, 3, text)
	list = c.completion(uri, pos)
	item = byLabel(list, "bg-red-500")
	if item.TextEdit.Range.Start.Character != pos.Character-len("bg-") {
		t.Errorf("edit starts at %d, want %d (after the variants)", item.TextEdit.Range.Start.Character, pos.Character-len("bg-"))
	}

	// A variant, before a utility.
	text, pos = withCursor(t, lspBufferHead+"func screen() tela.Node { return Div(Class(\"flex ho|\")) }\n")
	c.change(uri, 4, text)
	item = byLabel(c.completion(uri, pos), "hover:")
	if item.Kind != lspKindKeyword || item.Detail != "on hover" || item.TextEdit.NewText != "hover:" {
		t.Errorf("hover: = %+v", item)
	}
	text, pos = withCursor(t, lspBufferHead+"func screen() tela.Node { return Div(Class(\"da|\")) }\n")
	c.change(uri, 41, text)
	if item := byLabel(c.completion(uri, pos), "dark:"); item.Detail != "in dark mode" {
		t.Errorf("dark: detail = %q", item.Detail)
	}

	// Arbitrary values.
	text, pos = withCursor(t, lspBufferHead+"func screen() tela.Node { return Div(Class(\"w-[|\")) }\n")
	c.change(uri, 5, text)
	item = byLabel(c.completion(uri, pos), "w-[…]")
	if item.InsertTextFormat != 2 || item.TextEdit.NewText != "w-[${1:320px}]" || !strings.Contains(item.Detail, "width: 320px") {
		t.Errorf("w-[…] = %+v, want a snippet with the width it gives", item)
	}

	// Fields of struct literals, and the values of ClassAliases maps.
	text, pos = withCursor(t, lspBufferHead+"var _ = tela.App{RootClass: \"flex-|\"}\n")
	c.change(uri, 6, text)
	byLabel(c.completion(uri, pos), "flex-col")
	text, pos = withCursor(t, lspBufferHead+"var _ = tela.App{ClassAliases: map[string]string{\"x\": \"gap-|\"}}\n")
	c.change(uri, 7, text)
	byLabel(c.completion(uri, pos), "gap-4")

	// Not a class string: nothing.
	for name, body := range map[string]string{
		"text":      "func screen() tela.Node { return Div(Text(\"bg-|\")) }\n",
		"alias key": "var _ = tela.App{ClassAliases: map[string]string{\"bg-|\": \"flex\"}}\n",
		"comment":   "// Class(\"bg-|\")\n",
		"no string": "func screen() tela.Node { return Div(Cl|ass(\"flex\")) }\n",
	} {
		text, pos := withCursor(t, lspBufferHead+body)
		c.change(uri, 8, text)
		if list := c.completion(uri, pos); list != nil {
			t.Errorf("%s: completion = %d items, want none", name, len(list.Items))
		}
	}
}

func TestLSPAliases(t *testing.T) {
	t.Parallel()
	needGo(t) // it lints, which runs go list
	c := startLSP(t)
	c.initialize()
	_, uri := bufferFor(t, "aliases.go")
	text, pos := withCursor(t, lspBufferHead+"func screen() tela.Node { return Div(Class(\"bt|\")) }\n")
	c.open(uri, text)
	// The aliases are declared in another file of the package: they are
	// known once the first lint run has read it.
	c.diagnostics(uri, func([]lspDiagnostic) bool { return true })

	list := c.completion(uri, pos)
	var alias *lspCompletionItem
	for i := range list.Items {
		if list.Items[i].Label == "btn" {
			alias = &list.Items[i]
		}
	}
	if alias == nil || alias.Kind != lspKindClass || !strings.Contains(alias.Detail, "alias: px-4 py-2 rounded-md") {
		t.Fatalf("btn = %+v, want an alias item with its expansion", alias)
	}
	if !slices.ContainsFunc(list.Items, func(i lspCompletionItem) bool { return i.Label == "btn-big" }) {
		t.Error("btn-big is not completed")
	}

	// Hover over the alias: its expansion and what each class does.
	text, pos = withCursor(t, lspBufferHead+"func screen() tela.Node { return Div(Class(\"flex b|tn\")) }\n")
	c.change(uri, 2, text)
	md := c.hover(uri, pos)
	for _, want := range []string{"is an alias for `px-4 py-2 rounded-md bg-primary text-primary-foreground`",
		"padding-inline: 1rem;", "padding-block: 0.5rem;", "border-radius: 0.375rem;", "background-color: #"} {
		if !strings.Contains(md, want) {
			t.Errorf("alias hover lacks %q:\n%s", want, md)
		}
	}
	// An alias with a variant, built on another alias.
	text, pos = withCursor(t, lspBufferHead+"func screen() tela.Node { return Div(Class(\"hover:btn-b|ig\")) }\n")
	c.change(uri, 3, text)
	md = c.hover(uri, pos)
	for _, want := range []string{"on hover", "is an alias for `btn text-lg`", "font-size: 1.125rem;", "padding-inline: 1rem;"} {
		if !strings.Contains(md, want) {
			t.Errorf("nested alias hover lacks %q:\n%s", want, md)
		}
	}
	// And the use of an alias is not an error.
	c.change(uri, 4, strings.ReplaceAll(text, "|", ""))
	if d := c.diagnostics(uri, func([]lspDiagnostic) bool { return true }); len(d) != 0 {
		t.Errorf("diagnostics = %+v, want none: btn is an alias of the package", d)
	}
}

func TestLSPHover(t *testing.T) {
	t.Parallel()
	c := startLSP(t)
	c.initialize()
	_, uri := bufferFor(t, "hover.go")
	hover := func(body string) string {
		t.Helper()
		text, pos := withCursor(t, lspBufferHead+body)
		c.open(uri, text)
		return c.hover(uri, pos)
	}
	if md := hover("func screen() tela.Node { return Div(Class(\"flex |p-4 gap-2\")) }\n"); !strings.Contains(md, "**`p-4`**") || !strings.Contains(md, "padding: 1rem;") {
		t.Errorf("hover over p-4:\n%s", md)
	}
	// The cursor in the middle of a class, and at its end.
	if md := hover("func screen() tela.Node { return Div(Class(\"flex p-|4 gap-2\")) }\n"); !strings.Contains(md, "padding: 1rem;") {
		t.Errorf("hover inside p-4:\n%s", md)
	}
	if md := hover("func screen() tela.Node { return Div(Class(\"flex p-4| gap-2\")) }\n"); !strings.Contains(md, "padding: 1rem;") {
		t.Errorf("hover at the end of p-4:\n%s", md)
	}
	md := hover("func screen() tela.Node { return Div(Class(\"md:dark:hover:|bg-red-500/50\")) }\n")
	for _, want := range []string{"background-color: #fb2c3680;", "on hover", "in dark mode", "at least 48rem wide (md)"} {
		if !strings.Contains(md, want) {
			t.Errorf("hover over a class with variants lacks %q:\n%s", want, md)
		}
	}
	// An unknown class: the error of dev mode, with its suggestion.
	if md := hover("func screen() tela.Node { return Div(Class(\"bg-r|de-500\")) }\n"); !strings.Contains(md, `unknown class "bg-rde-500"`) || !strings.Contains(md, `did you mean "bg-red-500"`) {
		t.Errorf("hover over an unknown class:\n%s", md)
	}
	// The mask utilities are described too.
	if md := hover("func screen() tela.Node { return Div(Class(\"mask-t-from-|50%\")) }\n"); !strings.Contains(md, "--tw-mask-top-from-position: 50%;") {
		t.Errorf("hover over a mask class:\n%s", md)
	}
	// A class with no CSS equivalent still says what it is.
	if md := hover("func screen() tela.Node { return Div(Class(\"scrollbar-|overlay\")) }\n"); !strings.Contains(md, "scrollbar") || !strings.Contains(md, "no CSS summary") {
		t.Errorf("hover over a class with no CSS:\n%s", md)
	}
	// Between classes, and outside class strings: nothing.
	for name, body := range map[string]string{
		"space":   "func screen() tela.Node { return Div(Class(\"flex | p-4\")) }\n",
		"text":    "func screen() tela.Node { return Div(Text(\"p|-4\")) }\n",
		"outside": "func screen() tela.Node { return D|iv(Class(\"p-4\")) }\n",
	} {
		if md := hover(body); md != "" {
			t.Errorf("%s: hover = %q, want none", name, md)
		}
	}
	// A class in a Class field, with a raw string over several lines.
	if md := hover("var _ = tela.App{RootClass: `flex\n  items-|center`}\n"); !strings.Contains(md, "align-items: center;") {
		t.Errorf("hover in a raw string:\n%s", md)
	}
}

// The type-checked strings refine the syntactic ones: the fields of a kit
// recipe hold classes, which no token of the syntax says.
func TestLSPTypedClassStrings(t *testing.T) {
	t.Parallel()
	needGo(t) // it lints, which runs go list
	c := startLSP(t)
	c.initialize()
	_, uri := bufferFor(t, "typed.go")
	head := strings.Replace(lspBufferHead, "\t. \"github.com/kuronosu/tela/h\"\n", "\t. \"github.com/kuronosu/tela/h\"\n\t\"github.com/kuronosu/tela/kit\"\n", 1)
	text, pos := withCursor(t, head+"var _ tela.Node\nvar _ = Div\nvar _ = kit.ButtonStyle{Root: \"rounded-|lg p-4\"}\n")
	c.open(uri, text)
	if md := c.hover(uri, pos); md != "" {
		t.Errorf("hover before the lint run = %q, want none: the syntax does not say Root holds classes", md)
	}
	c.diagnostics(uri, func([]lspDiagnostic) bool { return true })
	// Once the lint run has read this very text, it does.
	if md := c.hover(uri, pos); !strings.Contains(md, "border-radius: 0.5rem;") {
		t.Errorf("hover over a recipe field after the lint run:\n%s", md)
	}
	// An edit that leaves the string where it is keeps it a class string
	// before the next run, and so does one that moves it.
	c.change(uri, 2, strings.Replace(text, "p-4", "p-8", 1))
	if md := c.hover(uri, pos); !strings.Contains(md, "border-radius: 0.5rem;") {
		t.Errorf("hover after an edit inside the string, before the next run:\n%s", md)
	}
	cut := strings.Index(text, "rounded-") + len("rounded-")
	moved, mpos := withCursor(t, "// a comment\n"+text[:cut]+"|"+text[cut:])
	c.change(uri, 3, moved)
	if md := c.hover(uri, mpos); !strings.Contains(md, "border-radius: 0.5rem;") {
		t.Errorf("hover after an edit above the string, before the next run:\n%s", md)
	}
	// An edit that breaks the literal does not.
	c.change(uri, 4, strings.Replace(text, "{Root: \"rounded", "{Root: rounded", 1))
	if md := c.hover(uri, pos); md != "" {
		t.Errorf("hover over a broken literal = %q, want none", md)
	}
}

func TestLSPPositions(t *testing.T) {
	d := &lspDoc{text: "ab😀c\r\n\u00f1x\nlast"}
	tests := []struct {
		off int
		pos lspPosition
	}{
		{0, lspPosition{0, 0}},
		{2, lspPosition{0, 2}},
		{6, lspPosition{0, 4}}, // after the emoji: two units
		{7, lspPosition{0, 5}},
		{9, lspPosition{1, 0}},
		{11, lspPosition{1, 1}}, // ñ is two bytes, one unit
		{13, lspPosition{2, 0}},
		{17, lspPosition{2, 4}},
	}
	for _, tc := range tests {
		if got := d.position(tc.off); got != tc.pos {
			t.Errorf("position(%d) = %+v, want %+v", tc.off, got, tc.pos)
		}
		if got := d.offset(tc.pos); got != tc.off {
			t.Errorf("offset(%+v) = %d, want %d", tc.pos, got, tc.off)
		}
	}
	// Positions past the end are clamped.
	if got := d.offset(lspPosition{9, 0}); got != len(d.text) {
		t.Errorf("offset past the last line = %d, want %d", got, len(d.text))
	}
	if got := d.offset(lspPosition{0, 99}); got != 7 {
		t.Errorf("offset past the end of line 0 = %d, want 7 (before the line break)", got)
	}
}

func TestSiteAt(t *testing.T) {
	tests := []struct {
		name   string
		src    string // with the cursor as |
		found  bool
		class  bool
		prefix string // the class typed before the cursor
	}{
		{"call", `x := Class("flex p-|4")`, true, true, "p-"},
		{"first arg", `Class("|")`, true, true, ""},
		{"after a comma", `Class("a", "b-|")`, true, true, "b-"},
		{"concatenation", `Class("a " + "b-|")`, true, true, "b-"},
		{"qualified", `h.Class("g|")`, true, true, "g"},
		{"exit", `h.Exit("g|")`, true, true, "g"},
		{"root class", `telatest.RootClass("g|")`, true, true, "g"},
		{"field", `kit.Button{Class: "g|"}`, true, true, "g"},
		{"suffix field", `kit.Card{HeaderClass: "g|"}`, true, true, "g"},
		{"assigned", `b.Class = "g|"`, true, true, "g"},
		{"appended", `b.Class += "g|"`, true, true, "g"},
		{"raw", "Class(`a\nb-|`)", true, true, "b-"},
		{"unterminated", `Class("a b-|`, true, true, "b-"},
		{"alias value", `tela.App{ClassAliases: map[string]string{"k": "g|"}}`, true, true, "g"},
		{"alias value after other entries", `tela.App{ClassAliases: map[string]string{"a": "x", "k": "g|"}}`, true, true, "g"},
		{"alias key", `tela.App{ClassAliases: map[string]string{"k|": "g"}}`, true, false, "k"},
		{"alias map without the type", `app.ClassAliases = map[string]string{"k": "g|"}`, true, true, "g"},
		{"after the alias map", `f(tela.App{ClassAliases: map[string]string{"k": "v"}}, Div(Text("g|")))`, true, false, "g"},
		{"other call", `Text("g|")`, true, false, "g"},
		{"other field", `kit.Button{Label: "g|"}`, true, false, "g"},
		{"variable", `class := "g|"`, true, false, "g"},
		{"before the quote", `Class(|"g")`, false, false, ""},
		{"after the string", `Class("g"|)`, false, false, ""},
		{"comment", `// Class("g|")`, false, false, ""},
		{"rune", `x := '|'`, false, false, ""},
	}
	for _, tc := range tests {
		i := strings.Index(tc.src, "|")
		src := tc.src[:i] + tc.src[i+1:]
		site, found := siteAt(src, i)
		if found != tc.found {
			t.Errorf("%s: found = %v, want %v", tc.name, found, tc.found)
			continue
		}
		if !found {
			continue
		}
		if site.class != tc.class {
			t.Errorf("%s: class = %v, want %v", tc.name, site.class, tc.class)
		}
		start, _, _ := classAt(src, site, i)
		if got := src[start:i]; got != tc.prefix {
			t.Errorf("%s: prefix = %q, want %q", tc.name, got, tc.prefix)
		}
	}
}

// TestLSPLatency logs how long the diagnostics of a buffer of examples/dashboard
// (a package of 5,000 lines that imports the whole framework) take, the
// first time and after a change, once the first run has loaded the package.
// It asserts nothing about the time (machines differ); the numbers are in
// the log with -v. The server has a cache of its own, so the first lint is
// a session's first, not one after the other tests.
//
// It runs in the full suite only. What it measures is a lint inside Tela's
// repository, whose context is the whole repository (every Go file is
// looked at for aliases), so it reads every file, and any edit would run
// cmd/tela's tests again; a fixture module large enough to stand for it
// would be a copy of the framework to keep in step.
func TestLSPLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("lints inside the repository, reading every file of it; the full suite runs it")
	}
	t.Parallel()
	file, err := filepath.Abs(filepath.Join("..", "..", "examples", "dashboard", "overview.go"))
	if err != nil {
		t.Fatal(err)
	}
	if real, err := filepath.EvalSymlinks(file); err == nil {
		file = real
	}
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	uri := fileURI(file)
	probe := func(n int) string {
		return string(src) + fmt.Sprintf("\nfunc latencyProbe() tela.Node { return Div(Class(\"flex bg-rde-%d\")) }\n", n)
	}
	c := startLSPCache(t, lint.NewCache())
	c.initialize()
	start := time.Now()
	c.open(uri, probe(400))
	c.diagnostics(uri, func(d []lspDiagnostic) bool { return len(d) == 1 })
	t.Logf("first diagnostics of overview.go: %v", time.Since(start).Round(time.Millisecond))
	var worst, total time.Duration
	const rounds = 5
	for i := range rounds {
		start := time.Now()
		c.change(uri, 2+i, probe(500+i))
		c.diagnostics(uri, func(d []lspDiagnostic) bool {
			return len(d) == 1 && strings.Contains(d[0].Message, fmt.Sprintf("bg-rde-%d", 500+i))
		})
		el := time.Since(start) - lspDebounce
		total += el
		worst = max(worst, el)
	}
	t.Logf("diagnostics after a change, without the %v pause: mean %v, worst %v",
		lspDebounce, (total / time.Duration(rounds)).Round(time.Millisecond), worst.Round(time.Millisecond))

	// Completion and hover do not wait for any of it.
	text, pos := withCursor(t, probe(600)+"\nvar _ = Div(Class(\"flex bg-|\"))\n")
	c.change(uri, 20, text)
	start = time.Now()
	list := c.completion(uri, pos)
	t.Logf("completion of bg-: %d items in %v", len(list.Items), time.Since(start).Round(100*time.Microsecond))
}
