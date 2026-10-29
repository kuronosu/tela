package main

import (
	"bytes"
	"encoding/json"
	"text/template"
)

// The instance of tela mcp (ADR-174) is a test binary: like tela snapshot
// (driver.go), the server adds a generated _test.go file to the package
// through an overlay, so the instance runs inside the package with access
// to its declarations and works in any module that requires Tela; unlike
// it, the server compiles it once with `go test -c` and starts the binary
// itself, so a rebuild is under its control and the process stays up
// between calls.
//
// The generated test, TestTelaMCP, mounts the screen with the public
// telatest package and serves commands: one JSON object per line on its
// standard input, one reply per command on its standard output, each line
// prefixed with mcpReplyPrefix (anything else the process prints there, such
// as the PASS of the testing package at exit, is not a reply). While it
// serves, os.Stdout and os.Stderr point at a temporary file whose new bytes
// go back with each reply, so what the app prints reaches the call that made
// it print.
//
// A telatest action fails through testing.TB's Fatalf. The instance mounts
// with a testing.TB of its own, which embeds the real *testing.T (for the
// unexported method of the interface) and overrides the methods telatest
// calls: Fatalf panics with a value the command loop recovers, so a failed
// action becomes the error of its reply and the app stays mounted; Logf
// collects the dev warnings and errors telatest reports (AllowDev("") turns
// the dev errors from test failures into logs); Cleanup is kept for the
// next mount. Any other panic is the app's own (a handler, a build, a dev
// panic): the reply carries it with its stack, and the instance mounts the
// screen afresh.

// mcpReplyPrefix starts every reply line of the instance.
const mcpReplyPrefix = "\x1etela-mcp "

// mcpGeneratedFile is the name of the generated test file inside the
// package directory (it exists only in the overlay).
const mcpGeneratedFile = "zz_tela_mcp_test.go"

// mcpLoc is a telatest locator, as the instance builds it: ByRole (with
// Name), ByText (Exact) or ByTestID, then Nth.
type mcpLoc struct {
	Role   string `json:"role,omitempty"`
	Name   string `json:"name,omitempty"`
	Text   string `json:"text,omitempty"`
	Exact  bool   `json:"exact,omitempty"`
	TestID string `json:"testId,omitempty"`
	Nth    *int   `json:"nth,omitempty"`
}

// mcpPoint is a point in logical pixels.
type mcpPoint struct{ X, Y float64 }

// mcpReq is a command to the instance. Op is mount, info, snapshot, count,
// bounds, screenshot, layout, click, hover, fill, press, scroll, drag,
// resize or wait; the other fields are its arguments.
type mcpReq struct {
	ID int    `json:"id"`
	Op string `json:"op"`

	// mount and resize
	Preview string  `json:"preview,omitempty"`
	W       float64 `json:"w,omitempty"`
	H       float64 `json:"h,omitempty"`
	SizeSet bool    `json:"sizeSet,omitempty"`
	Scale   float64 `json:"scale,omitempty"`
	Dark    bool    `json:"dark,omitempty"`

	Loc    *mcpLoc   `json:"loc,omitempty"`
	At     *mcpPoint `json:"at,omitempty"`
	To     *mcpLoc   `json:"to,omitempty"`
	ToAt   *mcpPoint `json:"toAt,omitempty"`
	DX     float64   `json:"dx,omitempty"`
	DY     float64   `json:"dy,omitempty"`
	Locs   []mcpLoc  `json:"locs,omitempty"`
	Button string    `json:"button,omitempty"` // "right" for the secondary button
	Double bool      `json:"double,omitempty"`
	Text   string    `json:"text,omitempty"`
	Gone   string    `json:"gone,omitempty"` // wait: a text that must disappear
	Submit bool      `json:"submit,omitempty"`
	Key    string    `json:"key,omitempty"`
	Ms     float64   `json:"ms,omitempty"`
	// Snapshot asks for the aria snapshot after the command.
	Snapshot bool `json:"snapshot,omitempty"`
}

// mcpReply is the instance's answer to an mcpReq.
type mcpReply struct {
	ID int `json:"id"`
	// Error is the failure of a telatest call (its Fatalf message without
	// the aria snapshot telatest appends); the app is still mounted.
	Error string `json:"error,omitempty"`
	// Panic is a panic of the app with its stack; the instance mounted
	// the screen afresh, or failed to (MountError).
	Panic string `json:"panic,omitempty"`
	// MountError says why no screen is mounted.
	MountError string `json:"mountError,omitempty"`
	// Logs are what telatest logged since the previous reply: "dev
	// warning: ...", "dev message: ..." (dev errors) and its own notes.
	Logs []string `json:"logs,omitempty"`
	// Output is what the process printed since the previous reply.
	Output string `json:"output,omitempty"`

	YAML   *string `json:"yaml,omitempty"`
	PNG    []byte  `json:"png,omitempty"`
	Layout string  `json:"layout,omitempty"`
	// Subtree says Layout is the subtree of the located element
	// (LayoutJSONOf); without it Layout is the whole window's and, for a
	// located element, Bounds says where it is (an instance built against a
	// tela that predates LayoutJSONOf, ADR-194).
	Subtree bool        `json:"subtree,omitempty"`
	Bounds  *[4]float64 `json:"bounds,omitempty"` // x, y, width, height
	Counts  []int       `json:"counts,omitempty"`

	// The mounted screen: its preview and all the names (Previews), the
	// window's logical size and scale, and whether it is dark.
	Preview string   `json:"preview,omitempty"`
	Names   []string `json:"names,omitempty"`
	W       float64  `json:"w,omitempty"`
	H       float64  `json:"h,omitempty"`
	Scale   float64  `json:"scale,omitempty"`
	Dark    bool     `json:"dark,omitempty"`
	// Note is a remark of the mount ("--dark has no effect...").
	Note string `json:"note,omitempty"`
}

// generateMCPSource returns the source of the generated test file of tela
// mcp for a package called pkgName that implements contract c (Preview,
// Previews or App); hasApp says the package declares `var App`.
func generateMCPSource(pkgName string, c contract, hasApp bool) []byte {
	var buf bytes.Buffer
	prefix, _ := json.Marshal(mcpReplyPrefix)
	err := mcpTemplate.Execute(&buf, struct {
		Package string
		Kind    string
		HasApp  bool
		Prefix  string
	}{Package: pkgName, Kind: c.String(), HasApp: hasApp, Prefix: string(prefix)})
	if err != nil {
		panic(err) // the template is a constant
	}
	return buf.Bytes()
}

var mcpTemplate = template.Must(template.New("mcp").Parse(`// Code generated by "tela mcp"; DO NOT EDIT.

package {{.Package}}

import (
	telabufio "bufio"
	telajson "encoding/json"
	telafmt "fmt"
	telaimage "image"
	telapng "image/png"
	telaio "io"
	telaos "os"
	teladebug "runtime/debug"
	telasort "sort"
	telastrings "strings"
	telasync "sync"
	telatesting "testing"
	telatime "time"

	telatela "github.com/kuronosu/tela"
	telatest "github.com/kuronosu/tela/telatest"
)

var (
	_ = telasort.Strings
	_ = telastrings.Join
)

{{template "contract" .}}
// telaMCPFail is the panic of a failed telatest call.
type telaMCPFail struct{ msg string }

// telaMCPTB is the testing.TB the app is mounted with: the real test's,
// except that a failure panics with telaMCPFail (the command loop recovers
// it), logs are collected for the reply and cleanups wait for the next
// mount.
type telaMCPTB struct {
	telatesting.TB
	mu       telasync.Mutex
	logs     []string
	cleanups []func()
}

func (t *telaMCPTB) Helper()      {}
func (t *telaMCPTB) Name() string { return "TestTelaMCP" }
func (t *telaMCPTB) Failed() bool { return false }
func (t *telaMCPTB) Fail()        {}
func (t *telaMCPTB) Skipped() bool { return false }
func (t *telaMCPTB) log(s string) {
	t.mu.Lock()
	t.logs = append(t.logs, telastrings.TrimRight(s, "\n"))
	t.mu.Unlock()
}
func (t *telaMCPTB) Log(args ...any)                 { t.log(telafmt.Sprintln(args...)) }
func (t *telaMCPTB) Logf(format string, args ...any) { t.log(telafmt.Sprintf(format, args...)) }
func (t *telaMCPTB) Error(args ...any)               { t.log("error: " + telafmt.Sprintln(args...)) }
func (t *telaMCPTB) Errorf(format string, args ...any) {
	t.log("error: " + telafmt.Sprintf(format, args...))
}
func (t *telaMCPTB) Fatal(args ...any)                 { panic(telaMCPFail{telafmt.Sprintln(args...)}) }
func (t *telaMCPTB) Fatalf(format string, args ...any) { panic(telaMCPFail{telafmt.Sprintf(format, args...)}) }
func (t *telaMCPTB) FailNow()                          { panic(telaMCPFail{"failed"}) }
func (t *telaMCPTB) Skip(args ...any)                  { panic(telaMCPFail{telafmt.Sprintln(args...)}) }
func (t *telaMCPTB) Skipf(format string, args ...any)  { panic(telaMCPFail{telafmt.Sprintf(format, args...)}) }
func (t *telaMCPTB) SkipNow()                          { panic(telaMCPFail{"skipped"}) }
func (t *telaMCPTB) Cleanup(f func()) {
	t.mu.Lock()
	t.cleanups = append(t.cleanups, f)
	t.mu.Unlock()
}

func (t *telaMCPTB) takeLogs() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	l := t.logs
	t.logs = nil
	return l
}

// cleanup runs the cleanups (the app's Close), last first.
func (t *telaMCPTB) cleanup() {
	t.mu.Lock()
	fs := t.cleanups
	t.cleanups = nil
	t.mu.Unlock()
	for i := len(fs) - 1; i >= 0; i-- {
		telaMCPProtect(fs[i])
	}
}

// telaMCPProtect runs f and returns the message of a failed telatest call
// or the description of another panic.
func telaMCPProtect(f func()) (fail, panicked string) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(telaMCPFail); ok {
				fail = telastrings.TrimSpace(e.msg)
				// The aria snapshot telatest appends: the server sends its
				// own, with refs.
				if i := telastrings.Index(fail, "\n\naria snapshot:"); i >= 0 {
					fail = fail[:i]
				}
				fail = telastrings.TrimPrefix(fail, "telatest: ")
				return
			}
			panicked = telafmt.Sprintf("%v\n\n%s", r, teladebug.Stack())
		}
	}()
	f()
	return "", ""
}

type telaMCPLoc struct {
	Role   string ` + "`json:\"role\"`" + `
	Name   string ` + "`json:\"name\"`" + `
	Text   string ` + "`json:\"text\"`" + `
	Exact  bool   ` + "`json:\"exact\"`" + `
	TestID string ` + "`json:\"testId\"`" + `
	Nth    *int   ` + "`json:\"nth\"`" + `
}

func (l *telaMCPLoc) locator() telatest.Locator {
	var loc telatest.Locator
	switch {
	case l.TestID != "":
		loc = telatest.ByTestID(l.TestID)
	case l.Role != "":
		loc = telatest.ByRole(l.Role, l.Name)
	default:
		if l.Exact {
			loc = telatest.ByText(l.Text, telatest.Exact())
		} else {
			loc = telatest.ByText(l.Text)
		}
	}
	if l.Nth != nil {
		loc = loc.Nth(*l.Nth)
	}
	return loc
}

type telaMCPPoint struct{ X, Y float64 }

type telaMCPReq struct {
	ID       int           ` + "`json:\"id\"`" + `
	Op       string        ` + "`json:\"op\"`" + `
	Preview  string        ` + "`json:\"preview\"`" + `
	W        float64       ` + "`json:\"w\"`" + `
	H        float64       ` + "`json:\"h\"`" + `
	SizeSet  bool          ` + "`json:\"sizeSet\"`" + `
	Scale    float64       ` + "`json:\"scale\"`" + `
	Dark     bool          ` + "`json:\"dark\"`" + `
	Loc      *telaMCPLoc   ` + "`json:\"loc\"`" + `
	At       *telaMCPPoint ` + "`json:\"at\"`" + `
	To       *telaMCPLoc   ` + "`json:\"to\"`" + `
	ToAt     *telaMCPPoint ` + "`json:\"toAt\"`" + `
	DX       float64       ` + "`json:\"dx\"`" + `
	DY       float64       ` + "`json:\"dy\"`" + `
	Locs     []telaMCPLoc  ` + "`json:\"locs\"`" + `
	Button   string        ` + "`json:\"button\"`" + `
	Double   bool          ` + "`json:\"double\"`" + `
	Text     string        ` + "`json:\"text\"`" + `
	Gone     string        ` + "`json:\"gone\"`" + `
	Submit   bool          ` + "`json:\"submit\"`" + `
	Key      string        ` + "`json:\"key\"`" + `
	Ms       float64       ` + "`json:\"ms\"`" + `
	Snapshot bool          ` + "`json:\"snapshot\"`" + `
}

type telaMCPReply struct {
	ID         int         ` + "`json:\"id\"`" + `
	Error      string      ` + "`json:\"error,omitempty\"`" + `
	Panic      string      ` + "`json:\"panic,omitempty\"`" + `
	MountError string      ` + "`json:\"mountError,omitempty\"`" + `
	Logs       []string    ` + "`json:\"logs,omitempty\"`" + `
	Output     string      ` + "`json:\"output,omitempty\"`" + `
	YAML       *string     ` + "`json:\"yaml,omitempty\"`" + `
	PNG        []byte      ` + "`json:\"png,omitempty\"`" + `
	Layout     string      ` + "`json:\"layout,omitempty\"`" + `
	Subtree    bool        ` + "`json:\"subtree,omitempty\"`" + `
	Bounds     *[4]float64 ` + "`json:\"bounds,omitempty\"`" + `
	Counts     []int       ` + "`json:\"counts,omitempty\"`" + `
	Preview    string      ` + "`json:\"preview,omitempty\"`" + `
	Names      []string    ` + "`json:\"names,omitempty\"`" + `
	W          float64     ` + "`json:\"w,omitempty\"`" + `
	H          float64     ` + "`json:\"h,omitempty\"`" + `
	Scale      float64     ` + "`json:\"scale,omitempty\"`" + `
	Dark       bool        ` + "`json:\"dark,omitempty\"`" + `
	Note       string      ` + "`json:\"note,omitempty\"`" + `
}

// telaMCPSession is the mounted screen and how it was mounted.
type telaMCPSession struct {
	tb       *telaMCPTB
	app      *telatest.App
	mount    telaMCPReq // the last mount, for a remount after a panic
	mountErr string
	preview  string
	names    []string
	w, h     float64
	scale    float64
	dark     bool
	note     string
	out      *telaos.File // where the process's output goes
	outRead  int64
}

func (s *telaMCPSession) close() {
	s.tb.cleanup()
	s.app = nil
}

// doMount mounts the screen afresh as req says.
func (s *telaMCPSession) doMount(req telaMCPReq) (fail, panicked string) {
	s.close()
	s.mount = req
	s.mountErr, s.note = "", ""
	return telaMCPProtect(func() {
		root, preview, names, err := telaBuild(req.Preview)
		s.names = names
		if err != nil {
			panic(telaMCPFail{err.Error()})
		}
		s.preview = preview
		w, h, dark := req.W, req.H, req.Dark
		opts := []telatest.Option{telatest.Size(float32(w), float32(h)), telatest.Scale(req.Scale),
			telatest.AllowDev(""), telatest.Timeout(2 * telatime.Second)}
		if dark {
			opts = append(opts, telatest.Dark())
		}
{{- if .HasApp}}
		// The App's settings, as for tela snapshot; an explicit size beats
		// its Width and Height.
		if telaHasApp {
			opts = append(opts, telatest.FromApp(telaApp))
			if req.SizeSet {
				opts = append(opts, telatest.Size(float32(w), float32(h)))
			} else if telaApp.Width > 0 && telaApp.Height > 0 {
				w, h = float64(telaApp.Width), float64(telaApp.Height)
			}
			switch telaApp.ColorScheme {
			case telatela.Light:
				if dark {
					s.note = "dark has no effect: App.ColorScheme is tela.Light, which overrides the system's preference"
				}
				dark = false
			case telatela.Dark:
				if !dark {
					s.note = "the screen is dark: App.ColorScheme is tela.Dark, which overrides the system's preference"
				}
				dark = true
			}
		}
{{- end}}
		s.w, s.h, s.scale, s.dark = w, h, req.Scale, dark
		s.app = telatest.Mount(s.tb, root, opts...)
	})
}

// remount mounts the screen again after a panic of the app.
func (s *telaMCPSession) remount() {
	s.app = nil
	fail, panicked := s.doMount(s.mount)
	if fail != "" {
		s.mountErr = fail
	} else if panicked != "" {
		s.mountErr = "the screen panics when it is mounted: " + panicked
	}
	if s.mountErr != "" {
		s.close()
	}
}

func (s *telaMCPSession) handle(req telaMCPReq) telaMCPReply {
	rep := telaMCPReply{ID: req.ID}
	switch {
	case req.Op == "mount":
		fail, panicked := s.doMount(req)
		switch {
		case fail != "":
			s.mountErr = fail
		case panicked != "":
			s.mountErr = "the screen panics when it is mounted: " + panicked
		}
		if s.mountErr != "" {
			s.close()
		}
	case s.app == nil:
		if s.mountErr == "" {
			s.mountErr = "no screen is mounted"
		}
	default:
		fail, panicked := telaMCPProtect(func() { s.do(req, &rep) })
		rep.Error = fail
		if panicked != "" {
			rep.Panic = panicked
			s.remount()
		}
	}
	rep.MountError = s.mountErr
	if req.Snapshot && s.app != nil {
		var y string
		if fail, panicked := telaMCPProtect(func() { y = s.app.AriaSnapshot() }); fail == "" && panicked == "" {
			rep.YAML = &y
		} else if panicked != "" {
			rep.Panic += panicked
			s.remount()
			rep.MountError = s.mountErr
		}
	}
	rep.Preview, rep.Names = s.preview, s.names
	rep.W, rep.H, rep.Scale, rep.Dark, rep.Note = s.w, s.h, s.scale, s.dark, s.note
	rep.Logs = s.tb.takeLogs()
	rep.Output = s.output()
	return rep
}

// output returns what the process printed since the last call.
func (s *telaMCPSession) output() string {
	if s.out == nil {
		return ""
	}
	end, err := s.out.Seek(0, telaio.SeekEnd)
	if err != nil || end <= s.outRead {
		return ""
	}
	buf := make([]byte, end-s.outRead)
	n, _ := s.out.ReadAt(buf, s.outRead)
	s.outRead += int64(n)
	return string(buf[:n])
}

func (s *telaMCPSession) do(req telaMCPReq, rep *telaMCPReply) {
	app := s.app
	need := func(what string, l *telaMCPLoc) telatest.Locator {
		if l == nil {
			panic(telaMCPFail{what + " needs an element"})
		}
		return l.locator()
	}
	switch req.Op {
	case "snapshot", "info":
		// The snapshot comes after every command that asks for it, and the
		// screen's settings with every reply.
	case "count":
		for i := range req.Locs {
			n := -1
			if _, panicked := telaMCPProtect(func() { n = app.Count(req.Locs[i].locator()) }); panicked != "" {
				panic(panicked)
			}
			rep.Counts = append(rep.Counts, n)
		}
	case "bounds":
		r := app.Bounds(need("bounds", req.Loc))
		rep.Bounds = &[4]float64{float64(r.X), float64(r.Y), float64(r.Width), float64(r.Height)}
	case "screenshot":
		img := app.Screenshot()
		var sub telaimage.Image = img
		if req.Loc != nil {
			r := app.Bounds(req.Loc.locator())
			rep.Bounds = &[4]float64{float64(r.X), float64(r.Y), float64(r.Width), float64(r.Height)}
			const margin = 8
			sc := s.scale
			px := telaimage.Rect(int((float64(r.X)-margin)*sc), int((float64(r.Y)-margin)*sc),
				int((float64(r.X+r.Width)+margin)*sc+0.999), int((float64(r.Y+r.Height)+margin)*sc+0.999)).Intersect(img.Bounds())
			if px.Empty() {
				panic(telaMCPFail{"the element is outside the window"})
			}
			sub = img.SubImage(px)
		}
		var buf telaMCPBuffer
		enc := telapng.Encoder{CompressionLevel: telapng.BestSpeed}
		if err := enc.Encode(&buf, sub); err != nil {
			panic(telaMCPFail{err.Error()})
		}
		rep.PNG = buf.b
	case "layout":
		// LayoutJSONOf is asked through an interface: the package's own
		// module may require a tela that predates it (ADR-194); then the
		// whole layout comes back with the element's bounds.
		type layoutOf interface {
			LayoutJSONOf(telatest.Locator) []byte
		}
		switch of, ok := any(app).(layoutOf); {
		case req.Loc != nil && ok:
			rep.Layout, rep.Subtree = string(of.LayoutJSONOf(req.Loc.locator())), true
		case req.Loc != nil:
			rep.Layout = string(app.LayoutJSON())
			r := app.Bounds(req.Loc.locator())
			rep.Bounds = &[4]float64{float64(r.X), float64(r.Y), float64(r.Width), float64(r.Height)}
		default:
			rep.Layout = string(app.LayoutJSON())
		}
	case "click":
		var opts []telatest.ClickOption
		if req.Button == "right" {
			opts = append(opts, telatest.Right())
		}
		switch {
		case req.Loc != nil && req.Double:
			app.DoubleClick(req.Loc.locator())
		case req.Loc != nil:
			app.Click(req.Loc.locator(), opts...)
		case req.At != nil:
			app.ClickAt(float32(req.At.X), float32(req.At.Y), opts...)
			if req.Double {
				// A second ClickAt would break the click sequence before its
				// press (each click is a first click), so OnDoubleClick would
				// never run: the second press continues the sequence ClickAt
				// began, at the same point, as DoubleClick does.
				app.MouseDown()
				app.MouseUp()
			}
		default:
			panic(telaMCPFail{"click needs an element or x and y"})
		}
	case "hover":
		if req.At != nil {
			app.MouseMove(float32(req.At.X), float32(req.At.Y))
		} else {
			app.Hover(need("hover", req.Loc))
		}
	case "fill":
		loc := need("fill", req.Loc)
		app.Fill(loc, req.Text)
		if req.Submit {
			app.Press("Enter")
		}
	case "press":
		if req.Loc != nil {
			app.Focus(req.Loc.locator())
		}
		app.Press(req.Key)
	case "scroll":
		app.Scroll(need("scroll", req.Loc), float32(req.DX), float32(req.DY))
	case "drag":
		switch {
		case req.At != nil && req.ToAt != nil:
			app.DragFromTo(telatela.Point{X: float32(req.At.X), Y: float32(req.At.Y)}, telatela.Point{X: float32(req.ToAt.X), Y: float32(req.ToAt.Y)})
		case req.Loc != nil && req.To != nil:
			app.Drag(req.Loc.locator(), req.To.locator())
		case req.Loc != nil:
			app.DragBy(req.Loc.locator(), float32(req.DX), float32(req.DY))
		default:
			panic(telaMCPFail{"drag needs a source and a target"})
		}
	case "resize":
		if req.W > 0 && req.H > 0 && (req.W != s.w || req.H != s.h) {
			app.Resize(float32(req.W), float32(req.H))
			s.w, s.h = req.W, req.H
			s.mount.W, s.mount.H, s.mount.SizeSet = req.W, req.H, true
		}
		if req.Scale > 0 && req.Scale != s.scale {
			app.SetScale(req.Scale)
			s.scale = req.Scale
			s.mount.Scale = req.Scale
		}
	case "wait":
		if req.Ms > 0 {
			app.Advance(telatime.Duration(req.Ms * float64(telatime.Millisecond)))
		}
		if req.Text != "" {
			app.ExpectText(req.Text)
		}
		if req.Gone != "" {
			app.ExpectNoText(req.Gone)
		}
	default:
		panic(telaMCPFail{telafmt.Sprintf("unknown command %q", req.Op)})
	}
}

// telaMCPBuffer collects the output of an encoder.
type telaMCPBuffer struct{ b []byte }

func (w *telaMCPBuffer) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }

func TestTelaMCP(t *telatesting.T) {
	replies := telaos.Stdout
	// What the app prints goes to a file whose new bytes travel with each
	// reply, never into the replies.
	out, err := telaos.CreateTemp("", "tela-mcp-output-*")
	if err == nil {
		defer func() { out.Close(); telaos.Remove(out.Name()) }()
		telaos.Stdout, telaos.Stderr = out, out
	}
	s := &telaMCPSession{tb: &telaMCPTB{TB: t}, out: out}
	defer s.close()
	w := telabufio.NewWriter(replies)
	in := telabufio.NewReader(telaos.Stdin)
	for {
		line, err := in.ReadBytes('\n')
		if len(telastrings.TrimSpace(string(line))) > 0 {
			var req telaMCPReq
			var rep telaMCPReply
			if jerr := telajson.Unmarshal(line, &req); jerr != nil {
				rep = telaMCPReply{ID: req.ID, Error: "bad command: " + jerr.Error()}
			} else {
				rep = s.handle(req)
			}
			b, jerr := telajson.Marshal(rep)
			if jerr != nil {
				b, _ = telajson.Marshal(telaMCPReply{ID: req.ID, Error: "cannot encode the reply: " + jerr.Error()})
			}
			w.WriteString({{.Prefix}})
			w.Write(b)
			w.WriteByte('\n')
			w.Flush()
		}
		if err != nil {
			return // the server closed the pipe
		}
	}
}
` + contractTemplate))
