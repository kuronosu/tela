package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kuronosu/tela/internal/lint"
	"github.com/kuronosu/tela/internal/style"
)

const lspUsage = `Usage:

	tela lsp [-v]

Runs a Language Server Protocol server on standard input and output, for an
editor to run beside gopls on the Go files of a Tela app. It knows what Tela
knows about Tailwind classes, and nothing else about Go:

	diagnostics   the findings of "tela lint" for the open files, from their
	              unsaved text: unknown classes with "did you mean", roles,
	              aria tokens, keys, hooks. Published on open, on change
	              (after a short pause) and on save
	code actions  a quick fix on an unknown class: replace it with the
	              suggestion of the message ("did you mean")
	completion    inside a class string: utilities and theme keys, variants
	              (hover:, md:, dark:, stacked), the app's class aliases and
	              arbitrary values (w-[...]); each with the CSS it produces
	hover         over a class: the CSS it produces after the theme is
	              resolved and when it applies; over an alias, its expansion

A string is a class string when it is an argument of Class(...), the value of
a field named Class or ending in Class, or a value of ClassAliases; once the
file has type-checked, every string that tela lint checks is one, the
constants that are used as classes (const card = "..."; Class(card)) among
them. Completion
and hover read the text only, so they answer while the file does not
compile. Diagnostics need the go command in PATH and a module that requires
Tela; the first run of a package builds the export data of its dependencies,
the later ones take a fraction of a second. Aliases and theme colors are
found in the package of the file and in the other packages of its module that
declare them (the app's main package, a theme package), as "tela lint" finds
them in the packages it lints.

The messages are the Language Server Protocol's, with Content-Length framing
and UTF-16 positions; the text of a file is synchronized in full. See
"tela help lsp" and the CLI wiki page for configuring VS Code, Neovim, Helix
and Zed.

Flags:

	-v   log what the server does to standard error
`

// lspDebounce is how long after the last change of a buffer its diagnostics
// are computed.
const lspDebounce = 300 * time.Millisecond

type lspOptions struct {
	Verbose bool
}

func parseLSPArgs(args []string) (lspOptions, error) {
	var o lspOptions
	fs := flag.NewFlagSet("tela lsp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.Verbose, "v", false, "")
	fs.BoolVar(&o.Verbose, "verbose", false, "")
	pos, err := parsePositionals("lsp", fs, args)
	if err != nil {
		return o, err
	}
	if len(pos) > 0 {
		return o, fmt.Errorf("tela lsp takes no arguments, got %s: an editor starts it in the folder it opened", strings.Join(pos, ", "))
	}
	return o, nil
}

// runLSP implements tela lsp.
func runLSP(args []string, stdout, stderr io.Writer) int {
	opts, err := parseLSPArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, lspUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "tela lsp: %v\n\n%s", err, lspUsage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logf := func(string, ...any) {}
	if opts.Verbose {
		var mu sync.Mutex
		logf = func(format string, args ...any) {
			mu.Lock()
			fmt.Fprintf(stderr, "tela lsp: "+format+"\n", args...)
			mu.Unlock()
		}
	}
	return serveLSP(ctx, os.Stdin, stdout, logf)
}

// serveLSP serves one client on in and out until it exits, in ends or ctx
// is done. It returns the exit status the protocol asks for: 0 after a
// shutdown request and an exit notification, 1 when the client goes away
// without them.
func serveLSP(ctx context.Context, in io.Reader, out io.Writer, logf func(string, ...any)) int {
	s := newLSPServer(newLSPConn(in, out), logf)
	defer s.close()
	// The input is read on a goroutine of its own, so that a signal ends
	// the server while it waits for input.
	type msg struct {
		body []byte
		err  error
	}
	msgs := make(chan msg)
	go func() {
		for {
			body, err := s.conn.read()
			select {
			case msgs <- msg{body, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case m := <-msgs:
			if m.err != nil {
				if !errors.Is(m.err, io.EOF) {
					logf("%v", m.err)
				}
				return 1
			}
			if exit, code := s.handle(m.body); exit {
				return code
			}
		case <-ctx.Done():
			return 1
		}
	}
}

// lspServer is the state of one client's session.
type lspServer struct {
	conn     *lspConn
	logf     func(string, ...any)
	debounce time.Duration

	mu          sync.Mutex // guards everything below but runMu
	initialized bool
	shutdown    bool
	closed      bool
	docs        map[string]*lspDoc // by URI
	dirty       map[string]bool    // directories whose packages must be linted again
	timer       *time.Timer
	state       map[string]*lspDirState // what the last run of a directory found, by directory
	published   map[string]string       // the diagnostics last sent, by URI, as JSON
	fixes       map[string]lspFixes     // the quick fixes of the diagnostics last sent, by URI
	logged      map[string]bool         // errors already told to the client

	runMu sync.Mutex // one lint run at a time
}

// lspFixes are the quick fixes of the diagnostics sent for one version of a
// document: a request for an older text gets none, since the ranges moved.
type lspFixes struct {
	version int
	list    []lspFix
}

// lspFix replaces the text of a range, where the class a diagnostic is about
// is, with the suggestion of the diagnostic's message.
type lspFix struct {
	diag       lspDiagnostic
	suggestion string
}

// lspDirState is what the last lint run of a directory knows: the class
// parser (with the aliases and colors of the package) and the class string
// literals it checked, by file, for the version of the text it read.
type lspDirState struct {
	parser  *style.Parser
	strings map[string]lspTypedStrings

	indexOnce sync.Once
	index     *classIndex
}

// classIndex is what completion reads of the parser, built when first
// needed (a run makes a new parser, so most never are).
func (st *lspDirState) classIndex() *classIndex {
	st.indexOnce.Do(func() { st.index = newClassIndex(st.parser) })
	return st.index
}

// defaultIndex serves a file whose package has not been linted yet.
var defaultIndex = sync.OnceValue(func() *classIndex { return newClassIndex(style.DefaultParser()) })

// classSource is what a request reads classes with.
type classSource struct {
	parser *style.Parser
	index  func() *classIndex
}

type lspTypedStrings struct {
	version int
	text    string // the text the spans are offsets of
	spans   []lint.ClassString
}

func newLSPServer(conn *lspConn, logf func(string, ...any)) *lspServer {
	return &lspServer{
		conn: conn, logf: logf, debounce: lspDebounce,
		docs: map[string]*lspDoc{}, dirty: map[string]bool{},
		state: map[string]*lspDirState{}, published: map[string]string{}, logged: map[string]bool{},
		fixes: map[string]lspFixes{},
	}
}

// close stops the work in the background.
func (s *lspServer) close() {
	s.mu.Lock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.mu.Unlock()
	// Let a run in progress finish: it must not outlive the server's
	// temporary files.
	s.runMu.Lock()
	defer s.runMu.Unlock()
}

// handle serves one message. exit is true after the exit notification, with
// the status the process ends with.
func (s *lspServer) handle(body []byte) (exit bool, code int) {
	var m lspMessage
	if err := json.Unmarshal(body, &m); err != nil {
		s.conn.fail(nil, lspParseError, "the message is not JSON: "+err.Error())
		return false, 0
	}
	if m.Method == "" {
		return false, 0 // a response: the server sends no requests
	}
	isRequest := len(m.ID) > 0
	s.mu.Lock()
	initialized, shutdown := s.initialized, s.shutdown
	s.mu.Unlock()
	switch {
	case m.Method == "exit":
		if shutdown {
			return true, 0
		}
		return true, 1
	case m.Method == "initialize":
	case !initialized:
		if isRequest {
			s.conn.fail(m.ID, lspServerNotInit, "the server is not initialized: send initialize first")
		}
		return false, 0
	case shutdown:
		if isRequest {
			s.conn.fail(m.ID, lspInvalidRequest, "the server is shutting down: send exit")
		}
		return false, 0
	}
	defer func() {
		if v := recover(); v != nil {
			s.logf("internal error in %s: %v", m.Method, v)
			if isRequest {
				s.conn.fail(m.ID, lspInternalError, fmt.Sprintf("internal error: %v (please report it)", v))
			}
		}
	}()
	result, err := s.dispatch(m)
	if !isRequest {
		return false, 0
	}
	var ce *lspCodedError
	switch {
	case errors.As(err, &ce):
		s.conn.fail(m.ID, ce.code, ce.msg)
	case err != nil:
		s.conn.fail(m.ID, lspInternalError, err.Error())
	default:
		s.conn.respond(m.ID, result)
	}
	return false, 0
}

type lspCodedError struct {
	code int
	msg  string
}

func (e *lspCodedError) Error() string { return e.msg }

func badParams(err error) error {
	return &lspCodedError{lspInvalidParams, "bad parameters: " + err.Error()}
}

func (s *lspServer) dispatch(m lspMessage) (any, error) {
	switch m.Method {
	case "initialize":
		return s.initialize(m.Params)
	case "initialized":
		return nil, nil
	case "shutdown":
		s.mu.Lock()
		s.shutdown = true
		s.mu.Unlock()
		return nil, nil
	case "textDocument/didOpen":
		return nil, s.didOpen(m.Params)
	case "textDocument/didChange":
		return nil, s.didChange(m.Params)
	case "textDocument/didSave":
		return nil, s.didSave(m.Params)
	case "textDocument/didClose":
		return nil, s.didClose(m.Params)
	case "textDocument/completion":
		return s.completion(m.Params)
	case "textDocument/hover":
		return s.hover(m.Params)
	case "textDocument/codeAction":
		return s.codeAction(m.Params)
	}
	if len(m.ID) > 0 {
		return nil, &lspCodedError{lspMethodNotFound, "method " + m.Method + " is not supported by tela lsp, which serves class strings only; gopls serves the rest"}
	}
	return nil, nil // an unknown notification ($/..., workspace/...) is ignored
}

// ---------------------------------------------------------------------
// Lifecycle.

func (s *lspServer) initialize(params json.RawMessage) (any, error) {
	var p struct {
		Capabilities struct {
			General struct {
				PositionEncodings []string `json:"positionEncodings"`
			} `json:"general"`
		} `json:"capabilities"`
	}
	_ = json.Unmarshal(params, &p) // all optional
	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()
	caps := map[string]any{
		"textDocumentSync": map[string]any{
			"openClose": true,
			"change":    1, // full
			"save":      map[string]any{"includeText": false},
		},
		"completionProvider": map[string]any{
			"triggerCharacters": []string{"\"", " ", "-", ":", "[", "/", "`"},
		},
		"hoverProvider": true,
		"codeActionProvider": map[string]any{
			"codeActionKinds": []string{"quickfix"},
		},
	}
	// UTF-16 is what every client speaks and the default; say it when the
	// client lists encodings.
	if len(p.Capabilities.General.PositionEncodings) > 0 {
		caps["positionEncoding"] = "utf-16"
	}
	return map[string]any{
		"capabilities": caps,
		"serverInfo":   map[string]any{"name": "tela lsp", "version": mcpServerVersion()},
	}, nil
}

// ---------------------------------------------------------------------
// Documents.

type lspTextDocumentItem struct {
	URI     string `json:"uri"`
	Text    string `json:"text"`
	Version int    `json:"version"`
}

func (s *lspServer) didOpen(params json.RawMessage) error {
	var p struct {
		TextDocument lspTextDocumentItem `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return badParams(err)
	}
	doc := &lspDoc{uri: p.TextDocument.URI, path: uriToPath(p.TextDocument.URI), version: p.TextDocument.Version, text: p.TextDocument.Text}
	s.mu.Lock()
	s.docs[doc.uri] = doc
	s.markDirty(doc)
	s.mu.Unlock()
	s.schedule(0)
	return nil
}

func (s *lspServer) didChange(params json.RawMessage) error {
	var p struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Range *lspRange `json:"range"`
			Text  string    `json:"text"`
		} `json:"contentChanges"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return badParams(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc := s.docs[p.TextDocument.URI]
	if doc == nil {
		return nil
	}
	// Documents are replaced, not edited, so that a request being served
	// keeps the text it started with.
	text := doc.text
	for _, c := range p.ContentChanges {
		if c.Range == nil {
			text = c.Text
			continue
		}
		cur := &lspDoc{text: text}
		text = text[:cur.offset(c.Range.Start)] + c.Text + text[cur.offset(c.Range.End):]
	}
	s.docs[doc.uri] = &lspDoc{uri: doc.uri, path: doc.path, version: p.TextDocument.Version, text: text}
	s.markDirty(doc)
	s.scheduleLocked(s.debounce)
	return nil
}

func (s *lspServer) didSave(params json.RawMessage) error {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return badParams(err)
	}
	s.mu.Lock()
	doc := s.docs[p.TextDocument.URI]
	if doc != nil {
		s.markDirty(doc)
	}
	s.mu.Unlock()
	if doc != nil {
		s.schedule(0)
	}
	return nil
}

func (s *lspServer) didClose(params json.RawMessage) error {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return badParams(err)
	}
	s.mu.Lock()
	doc := s.docs[p.TextDocument.URI]
	delete(s.docs, p.TextDocument.URI)
	_, hadDiagnostics := s.published[p.TextDocument.URI]
	delete(s.published, p.TextDocument.URI)
	delete(s.fixes, p.TextDocument.URI)
	if doc != nil {
		s.markDirty(doc)
		s.scheduleLocked(s.debounce)
	}
	s.mu.Unlock()
	if hadDiagnostics {
		s.conn.notify("textDocument/publishDiagnostics", map[string]any{"uri": p.TextDocument.URI, "diagnostics": []any{}})
	}
	return nil
}

// markDirty records that the package of doc must be linted again. Callers
// hold s.mu.
func (s *lspServer) markDirty(doc *lspDoc) {
	if doc.path != "" && strings.HasSuffix(doc.path, ".go") {
		s.dirty[filepath.Dir(doc.path)] = true
		// What declares aliases and colors is read by every package of
		// the module: they are linted again with the new declarations.
		if strings.Contains(doc.text, "ClassAliases") || strings.Contains(doc.text, "SchemeColor") {
			for _, other := range s.docs {
				if other.path != "" && strings.HasSuffix(other.path, ".go") {
					s.dirty[filepath.Dir(other.path)] = true
				}
			}
		}
	}
}

func (s *lspServer) schedule(delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduleLocked(delay)
}

func (s *lspServer) scheduleLocked(delay time.Duration) {
	if s.closed {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(delay, s.analyze)
}

// ---------------------------------------------------------------------
// Diagnostics.

// lspDiagnostic is a diagnostic of the protocol.
type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Code     string   `json:"code,omitempty"`
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

// analyze lints the packages of the documents that changed, one run at a
// time, and publishes what it finds.
func (s *lspServer) analyze() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	defer func() {
		if v := recover(); v != nil {
			s.tellOnce("panic", fmt.Sprintf("tela lsp: internal error while linting: %v (please report it)", v))
		}
	}()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	byDir := map[string][]*lspDoc{}
	for _, doc := range s.docs {
		if doc.path == "" || !strings.HasSuffix(doc.path, ".go") {
			continue
		}
		if dir := filepath.Dir(doc.path); s.dirty[dir] {
			byDir[dir] = append(byDir[dir], doc)
		}
	}
	clear(s.dirty)
	s.mu.Unlock()
	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		s.lintDir(dir, byDir[dir])
	}
}

// lintDir lints the package in dir with the text of the open documents of
// it, and publishes their diagnostics.
func (s *lspServer) lintDir(dir string, docs []*lspDoc) {
	start := time.Now()
	overlay := make(map[string][]byte, len(docs))
	tests := false
	for _, d := range docs {
		overlay[d.path] = []byte(d.text)
		tests = tests || strings.HasSuffix(d.path, "_test.go")
	}
	res, err := lint.Run(lint.Options{
		Dir: dir, Patterns: []string{"."}, Context: lspModuleContext(dir), Tests: tests,
		Overlay: overlay, AbsPaths: true, Lenient: true,
	})
	if err != nil {
		s.tellOnce("lint-"+dir, fmt.Sprintf("tela lsp: cannot lint %s: %v", dir, err))
		return
	}
	s.logf("linted %s in %s: %d findings, %d load errors", dir, time.Since(start).Round(time.Millisecond), len(res.Diagnostics), len(res.LoadErrors))
	for _, e := range res.LoadErrors {
		s.logf("  %s", e)
	}
	st := &lspDirState{parser: res.Parser, strings: map[string]lspTypedStrings{}}
	if st.parser == nil {
		st.parser = style.DefaultParser()
	}
	byPath := map[string]*lspDoc{}
	for _, d := range docs {
		byPath[d.path] = d
		st.strings[d.path] = lspTypedStrings{version: d.version, text: d.text}
	}
	for _, cs := range res.ClassStrings {
		if ts, ok := st.strings[cs.File]; ok {
			ts.spans = append(ts.spans, cs)
			st.strings[cs.File] = ts
		}
	}
	diags := map[string][]lspDiagnostic{}
	fixes := map[string][]lspFix{}
	for _, d := range res.Diagnostics {
		if doc := byPath[d.File]; doc != nil {
			ld := toLSPDiagnostic(doc, d)
			diags[doc.path] = append(diags[doc.path], ld)
			if sugg := classSuggestion(d); sugg != "" {
				fixes[doc.path] = append(fixes[doc.path], lspFix{ld, sugg})
			}
		}
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.state[dir] = st
	type out struct {
		uri     string
		version int
		diags   []lspDiagnostic
		key     string
	}
	var sends []out
	for _, d := range docs {
		if s.docs[d.uri] == nil {
			continue // closed while the run went on
		}
		s.fixes[d.uri] = lspFixes{version: d.version, list: fixes[d.path]}
		list := diags[d.path]
		if list == nil {
			list = []lspDiagnostic{}
		}
		b, _ := json.Marshal(list)
		if s.published[d.uri] == string(b) {
			continue
		}
		s.published[d.uri] = string(b)
		sends = append(sends, out{d.uri, d.version, list, string(b)})
	}
	s.mu.Unlock()
	for _, o := range sends {
		s.conn.notify("textDocument/publishDiagnostics", map[string]any{"uri": o.uri, "version": o.version, "diagnostics": o.diags})
	}
}

// tellOnce tells the client a problem the first time it happens.
func (s *lspServer) tellOnce(key, msg string) {
	s.mu.Lock()
	seen := s.logged[key+msg]
	s.logged[key+msg] = true
	s.mu.Unlock()
	if !seen {
		s.logf("%s", msg)
		s.conn.notify("window/logMessage", map[string]any{"type": 2, "message": msg})
	}
}

// toLSPDiagnostic converts a finding at a line and byte column of the
// document's text.
func toLSPDiagnostic(doc *lspDoc, d lint.Diagnostic) lspDiagnostic {
	starts := doc.lineStarts()
	line := max(0, min(d.Line-1, len(starts)-1))
	from := starts[line] + max(0, d.Col-1)
	from = min(from, len(doc.text))
	to := from
	if d.Rule == lint.RuleClass {
		// The class: up to the next space or quote.
		for to < len(doc.text) && !strings.ContainsRune(" \t\r\n\"`", rune(doc.text[to])) {
			to++
		}
	} else {
		// The identifier or selector expression at the position.
		for to < len(doc.text) && isIdentByte(doc.text[to]) {
			to++
		}
	}
	if to == from {
		to = min(from+1, len(doc.text))
	}
	sev := 1
	if d.Severity == lint.SeverityWarning {
		sev = 2
	}
	return lspDiagnostic{
		Range: doc.rangeOf(from, to), Severity: sev, Code: d.Rule, Source: "tela lint", Message: d.Message,
	}
}

func isIdentByte(b byte) bool {
	return b == '_' || b == '.' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}

// ---------------------------------------------------------------------
// Completion and hover.

type lspTextDocumentPosition struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position lspPosition `json:"position"`
}

// siteFor finds the class string at the position of a request, and the
// parser to read it with.
func (s *lspServer) siteFor(params json.RawMessage) (doc *lspDoc, site classSite, off int, src classSource, ok bool, err error) {
	var tp lspTextDocumentPosition
	if err := json.Unmarshal(params, &tp); err != nil {
		return nil, site, 0, src, false, badParams(err)
	}
	s.mu.Lock()
	doc = s.docs[tp.TextDocument.URI]
	var st *lspDirState
	if doc != nil && doc.path != "" {
		st = s.state[filepath.Dir(doc.path)]
	}
	s.mu.Unlock()
	if doc == nil {
		return nil, site, 0, src, false, nil
	}
	off = doc.offset(tp.Position)
	site, found := siteAt(doc.text, off)
	if !found {
		return doc, site, off, src, false, nil
	}
	src = classSource{parser: style.DefaultParser(), index: defaultIndex}
	if st != nil {
		src = classSource{parser: st.parser, index: st.classIndex}
		// The type checker knows more strings than the syntax shows, from
		// the text it read.
		if ts, ok := st.strings[doc.path]; ok && !site.class {
			for _, sp := range ts.spans {
				start, end := sp.Start, sp.End
				if ts.version != doc.version {
					// An edit since the run: the literal is where the edit
					// leaves it, if it is still a literal.
					var ok bool
					if start, end, ok = shiftSpan(ts.text, doc.text, start, end); !ok {
						continue
					}
				}
				if start == site.litStart && end == site.litEnd {
					site.class = true
					break
				}
			}
		}
	}
	return doc, site, off, src, site.class, nil
}

func (s *lspServer) completion(params json.RawMessage) (any, error) {
	doc, site, off, src, ok, err := s.siteFor(params)
	if err != nil || !ok {
		return nil, err
	}
	start, end, _ := classAt(doc.text, site, off)
	return completeClass(src.parser, src.index(), doc, start, off, max(end, off)), nil
}

func (s *lspServer) hover(params json.RawMessage) (any, error) {
	doc, site, off, src, ok, err := s.siteFor(params)
	if err != nil || !ok {
		return nil, err
	}
	start, end, found := classAt(doc.text, site, off)
	if !found {
		return nil, nil
	}
	return map[string]any{
		"contents": map[string]any{"kind": "markdown", "value": hoverMarkdown(src.parser, doc.text[start:end])},
		"range":    doc.rangeOf(start, end),
	}, nil
}

// ---------------------------------------------------------------------
// URIs.

var windowsDriveURI = regexp.MustCompile(`^/[A-Za-z]:`)

// uriToPath returns the path of a file: URI, cleaned, with symbolic links
// resolved when the file or its folder exists (the go command names
// directories that way, and the overlay is matched by name); "" for any
// other URI.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if windowsDriveURI.MatchString(p) {
		p = p[1:]
	}
	p = filepath.Clean(filepath.FromSlash(p))
	if dir, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		p = filepath.Join(dir, filepath.Base(p))
	}
	return p
}
