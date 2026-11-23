package main

import (
	"bytes"
	"encoding/json"
	"text/template"
	"time"
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

// mcpLoc is a telatest locator, as the instance builds it: the element of
// a node identity (NodeID: a ref of the snapshot, App.NodeLocator), or
// ByRole (with Name), ByText (Exact) or ByTestID, then Nth.
type mcpLoc struct {
	Role   string `json:"role,omitempty"`
	Name   string `json:"name,omitempty"`
	Text   string `json:"text,omitempty"`
	Exact  bool   `json:"exact,omitempty"`
	TestID string `json:"testId,omitempty"`
	Nth    *int   `json:"nth,omitempty"`
	NodeID uint64 `json:"node,omitempty"`
	// Ref is the ref a NodeID stands for, for messages.
	Ref string `json:"ref,omitempty"`
	// Stable, on the server only, is the locator a test uses for the
	// element a ref names (see refEntry).
	Stable *mcpLoc `json:"-"`
}

// mcpPoint is a point in logical pixels.
type mcpPoint struct{ X, Y float64 }

// mcpPrefer are the preferences the user of the app is said to have, beyond
// dark mode (mcpReq.Dark): what the tool prefer sets. Reduced motion, high
// contrast and the text scale are the system's settings telatest changes
// while the app runs; the locale is tela/i18n's; the direction is an h.Dir
// around the screen, so changing it mounts the screen afresh.
type mcpPrefer struct {
	ReducedMotion bool    `json:"reducedMotion,omitempty"`
	HighContrast  bool    `json:"highContrast,omitempty"`
	TextScale     float64 `json:"textScale,omitempty"` // 0: 1
	Locale        string  `json:"locale,omitempty"`
	LocaleSet     bool    `json:"localeSet,omitempty"` // Locale "" with it: the system's
	Direction     string  `json:"direction,omitempty"` // "", "ltr" or "rtl"
}

// mcpComponent is a telatest.Component as the instance reports it (the
// fields of the Go struct, matched by name).
type mcpComponent struct {
	Name, Key, Source string
	Props             []struct{ Name, Value string }
	Hooks             []struct {
		Index               int
		Kind, Source, Value string
	}
	Builds  int
	Renders []struct {
		Frame   int
		Reasons []string
	}
}

// mcpFrame is a telatest.Frame as the instance reports it.
type mcpFrame struct {
	Number                 int
	Duration               time.Duration
	Phases                 map[string]time.Duration
	Built, Styled, LaidOut int
	Damage                 int64
}

// mcpReq is a command to the instance. Op is mount, info, snapshot, count,
// bounds, screenshot, layout, click, hover, fill, press, scroll, drag,
// resize, wait, state, perf or prefer; the other fields are its arguments. The op cancel (with
// Target, the id of the command to stop) is handled the moment it is read,
// while that command runs, and has no reply.
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
	// film: Ms is the span and Every the interval, in milliseconds of the
	// app's clock, and Inner the command that starts the motion (nil: the
	// app as it is).
	Every float64 `json:"every,omitempty"`
	Inner *mcpReq `json:"inner,omitempty"`
	// Snapshot asks for the aria snapshot after the command.
	Snapshot bool `json:"snapshot,omitempty"`
	// Prefer, with mount and prefer: the preferences the screen is mounted
	// with, or the ones to be in (Dark says the same of dark mode).
	Prefer *mcpPrefer `json:"prefer,omitempty"`
	// Count and FromAction, with perf: the last Count frames, or the frames
	// since the last command that acted. Max, with state without an
	// element: the components to list.
	Count      int  `json:"count,omitempty"`
	FromAction bool `json:"fromAction,omitempty"`
	Max        int  `json:"max,omitempty"`
	// Target is the id of the command a cancel stops.
	Target int `json:"target,omitempty"`
}

// mcpFilmFrame is a frame of a film: a PNG of the window.
type mcpFilmFrame struct {
	At  int64  `json:"at"`
	PNG []byte `json:"png"`
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

	YAML *string `json:"yaml,omitempty"`
	// Nodes, with YAML, is the identity of the element each line prints
	// (0: none), when the instance's telatest gives them
	// (AriaSnapshotNodes).
	Nodes []uint64 `json:"nodes,omitempty"`
	// TestIDs, with Nodes, is the test id of each line's element when no
	// other element has it (nil when none has one): the test tool names
	// it by ByTestID.
	TestIDs []string `json:"testIds,omitempty"`
	PNG     []byte   `json:"png,omitempty"`
	Layout  string   `json:"layout,omitempty"`
	// Subtree says Layout is the subtree of the located element
	// (LayoutJSONOf); without it Layout is the whole window's and, for a
	// located element, Bounds says where it is (an instance built against a
	// tela that predates LayoutJSONOf, ADR-194).
	Subtree bool        `json:"subtree,omitempty"`
	Bounds  *[4]float64 `json:"bounds,omitempty"` // x, y, width, height
	Counts  []int       `json:"counts,omitempty"`
	// Moved, with scroll, says whether a scroll offset changed (nil: an
	// instance that cannot say); Scrolled is the identity of the scroll
	// container the instance picked for a scroll without an element.
	Moved    *bool  `json:"moved,omitempty"`
	Scrolled uint64 `json:"scrolled,omitempty"`

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
	// Film, for film: the frames, At in milliseconds of the app's clock.
	Film []mcpFilmFrame `json:"film,omitempty"`

	// state: the component of the element, or the components of the screen
	// that have props or hooks (Total of them in all; Instrumented false
	// when this tela cannot say).
	Components []mcpComponent `json:"components,omitempty"`
	Total      int            `json:"total,omitempty"`
	// perf: the frames, and the number of the last frame of the app.
	Frames   []mcpFrame `json:"frames,omitempty"`
	FrameNow int        `json:"frameNow,omitempty"`
	// prefer: what each preference did ("dark: live", "direction: mounted
	// the screen afresh"), whether the screen was mounted again, and the
	// preferences it is in now.
	Applied   []string   `json:"applied,omitempty"`
	Remounted bool       `json:"remounted,omitempty"`
	Prefer    *mcpPrefer `json:"prefer,omitempty"`
	// DevTools says the instance was built with this tela's DevTools (the
	// source lines, state, perf and prefer); without it those tools tell
	// the user to update tela.
	DevTools bool `json:"devTools,omitempty"`
}

// generateMCPSource returns the source of the generated test file of tela
// mcp for a package called pkgName that implements contract c (Preview,
// Previews or App); hasApp says the package declares `var App`.
func generateMCPSource(pkgName string, c contract, hasApp bool, feats ...genFeatures) []byte {
	var f genFeatures
	if len(feats) > 0 {
		f = feats[0]
	}
	var buf bytes.Buffer
	prefix, _ := json.Marshal(mcpReplyPrefix)
	err := mcpTemplate.Execute(&buf, struct {
		Package  string
		Kind     string
		HasApp   bool
		Prefix   string
		DevTools bool
		I18N     bool
		Film     bool
		Kinds    bool
		Scrolls  bool
	}{Package: pkgName, Kind: c.String(), HasApp: hasApp, Prefix: string(prefix), DevTools: f.DevTools, I18N: f.DevTools && f.I18N, Film: f.Film, Kinds: f.PendingKinds, Scrolls: f.Scrollables})
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
	telaatomic "sync/atomic"
	telatesting "testing"
	telatime "time"

	telatela "github.com/kuronosu/tela"
{{- if .DevTools}}
	telah "github.com/kuronosu/tela/h"
{{- end}}
{{- if .I18N}}
	telai18n "github.com/kuronosu/tela/i18n"
{{- end}}
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
	// running is the id of the command being run (0: none, or one that
	// cannot be cancelled) and cancelled says the client cancelled it:
	// Helper, which telatest calls at the start of its steps and on each
	// try of a wait, then ends the command.
	running      telaatomic.Int64
	cancelled    telaatomic.Bool
	cancelTarget telaatomic.Int64 // the id the client last asked to cancel
}

// cancel is called on the goroutine that reads the commands, while one
// runs.
func (t *telaMCPTB) cancel(id int) {
	t.cancelTarget.Store(int64(id))
	if id != 0 && t.running.Load() == int64(id) {
		t.cancelled.Store(true)
	}
}

// telaMCPCancelled is the message of the failure that ends a cancelled
// command.
const telaMCPCancelled = "cancelled by the client"

func (t *telaMCPTB) Helper() {
	if t.running.Load() != 0 && t.cancelled.Load() {
		panic(telaMCPFail{telaMCPCancelled})
	}
}
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
	NodeID uint64 ` + "`json:\"node\"`" + `
	Ref    string ` + "`json:\"ref\"`" + `
}

// telaMCPNodes is what an App of a telatest that has AriaSnapshotNodes and
// NodeLocator does; the user's module may require an older tela, so the
// instance asks through an interface.
type telaMCPNodes interface {
	AriaSnapshotNodes() (string, []uint64)
	NodeLocator(uint64) telatest.Locator
}

func (l *telaMCPLoc) locator(app *telatest.App) telatest.Locator {
	var loc telatest.Locator
	switch {
	case l.NodeID != 0:
		n, ok := any(app).(telaMCPNodes)
		if !ok {
			panic(telaMCPFail{"this version of tela cannot name an element by its identity; take a new snapshot"})
		}
		loc = n.NodeLocator(l.NodeID)
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
	Every    float64       ` + "`json:\"every\"`" + `
	Inner    *telaMCPReq   ` + "`json:\"inner\"`" + `
	Snapshot bool          ` + "`json:\"snapshot\"`" + `
	Target   int           ` + "`json:\"target\"`" + `
{{- if .DevTools}}
	Prefer     *telaMCPPrefer ` + "`json:\"prefer\"`" + `
	Count      int            ` + "`json:\"count\"`" + `
	FromAction bool           ` + "`json:\"fromAction\"`" + `
	Max        int            ` + "`json:\"max\"`" + `
{{- end}}
}

type telaMCPFilmFrame struct {
	At  int64  ` + "`json:\"at\"`" + `
	PNG []byte ` + "`json:\"png\"`" + `
}

type telaMCPReply struct {
	ID         int         ` + "`json:\"id\"`" + `
	Error      string      ` + "`json:\"error,omitempty\"`" + `
	Panic      string      ` + "`json:\"panic,omitempty\"`" + `
	MountError string      ` + "`json:\"mountError,omitempty\"`" + `
	Logs       []string    ` + "`json:\"logs,omitempty\"`" + `
	Output     string      ` + "`json:\"output,omitempty\"`" + `
	YAML       *string     ` + "`json:\"yaml,omitempty\"`" + `
	Nodes      []uint64    ` + "`json:\"nodes,omitempty\"`" + `
	TestIDs    []string    ` + "`json:\"testIds,omitempty\"`" + `
	PNG        []byte      ` + "`json:\"png,omitempty\"`" + `
	Layout     string      ` + "`json:\"layout,omitempty\"`" + `
	Subtree    bool        ` + "`json:\"subtree,omitempty\"`" + `
	Bounds     *[4]float64 ` + "`json:\"bounds,omitempty\"`" + `
	Counts     []int       ` + "`json:\"counts,omitempty\"`" + `
	Moved      *bool       ` + "`json:\"moved,omitempty\"`" + `
	Scrolled   uint64      ` + "`json:\"scrolled,omitempty\"`" + `
	Preview    string      ` + "`json:\"preview,omitempty\"`" + `
	Names      []string    ` + "`json:\"names,omitempty\"`" + `
	W          float64     ` + "`json:\"w,omitempty\"`" + `
	H          float64     ` + "`json:\"h,omitempty\"`" + `
	Scale      float64     ` + "`json:\"scale,omitempty\"`" + `
	Dark       bool        ` + "`json:\"dark,omitempty\"`" + `
	Note       string      ` + "`json:\"note,omitempty\"`" + `
	Film       []telaMCPFilmFrame ` + "`json:\"film,omitempty\"`" + `
{{- if .DevTools}}
	DevTools   bool            ` + "`json:\"devTools,omitempty\"`" + `
	Components []telatest.Component ` + "`json:\"components,omitempty\"`" + `
	Total      int             ` + "`json:\"total,omitempty\"`" + `
	Frames     []telatest.Frame ` + "`json:\"frames,omitempty\"`" + `
	FrameNow   int             ` + "`json:\"frameNow,omitempty\"`" + `
	Applied    []string        ` + "`json:\"applied,omitempty\"`" + `
	Remounted  bool            ` + "`json:\"remounted,omitempty\"`" + `
	Prefer     *telaMCPPrefer  ` + "`json:\"prefer,omitempty\"`" + `
{{- end}}
}

{{if .DevTools -}}
// telaMCPPrefer is the mcpPrefer of the server.
type telaMCPPrefer struct {
	ReducedMotion bool    ` + "`json:\"reducedMotion\"`" + `
	HighContrast  bool    ` + "`json:\"highContrast\"`" + `
	TextScale     float64 ` + "`json:\"textScale\"`" + `
	Locale        string  ` + "`json:\"locale\"`" + `
	LocaleSet     bool    ` + "`json:\"localeSet\"`" + `
	Direction     string  ` + "`json:\"direction\"`" + `
}

// telaMCPStart is the origin of the clock the frame profile reads: real
// time, unlike telatest's deterministic default, so perf measures.
var telaMCPStart = telatime.Now()

func telaMCPClock() telatime.Duration { return telatime.Since(telaMCPStart) }

func (p telaMCPPrefer) normalized() telaMCPPrefer {
	if p.TextScale <= 0 {
		p.TextScale = 1
	}
	return p
}

// telaMCPDirection mounts root under an h.Dir (a display: contents
// element, so the layout is the same but for the direction); ""
// leaves it alone. A preview keeps its Theme and ColorScheme.
func telaMCPDirection(root telatela.Component, dir string) telatela.Component {
	if dir == "" {
		return root
	}
	around := func(n telatela.Node) telatela.Node {
		return telah.Div(telah.Dir(dir), telah.Class("contents"), n)
	}
	if p, ok := root.(telatela.Preview); ok {
		wrap := p.Wrap
		p.Wrap = func(n telatela.Node) telatela.Node {
			if wrap != nil {
				n = wrap(n)
			}
			return around(n)
		}
		return p
	}
	return telatela.Preview{Root: root, Wrap: around}
}
{{- if .I18N}}

// telaMCPSetLocale is i18n.SetLocale, which panics for a tag that is not
// BCP 47.
func telaMCPSetLocale(tag string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = telafmt.Errorf("%v", r)
		}
	}()
	telai18n.SetLocale(tag)
	return nil
}
{{- end}}
{{- end}}

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
{{- if .DevTools}}
	prefer      telaMCPPrefer // the preferences the screen is in
	fixedScheme bool          // App.ColorScheme or the preview's decides the scheme
	schemeOf    string
	actionFrom  int // the frame the last command that acted started at
{{- end}}
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
{{- if .DevTools}}
		// The element source lines, the state of the components and the
		// frame profile come from the instrumentation.
		opts = append(opts, telatest.Instrument(), telatest.DebugClock(telaMCPClock))
		var pf telaMCPPrefer
		if req.Prefer != nil {
			pf = *req.Prefer
		}
		pf = pf.normalized()
		if pf.ReducedMotion {
			opts = append(opts, telatest.ReducedMotion())
		}
		if pf.HighContrast {
			opts = append(opts, telatest.HighContrast())
		}
		if pf.TextScale != 1 {
			opts = append(opts, telatest.TextScale(pf.TextScale))
		}
{{- if .I18N}}
		if pf.LocaleSet {
			if err := telaMCPSetLocale(pf.Locale); err != nil {
				panic(telaMCPFail{err.Error()})
			}
		}
{{- end}}
		root = telaMCPDirection(root, pf.Direction)
{{- end}}
		if dark {
			opts = append(opts, telatest.Dark())
		}
		scheme, schemeOf := telatela.System, ""
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
			scheme, schemeOf = telaApp.ColorScheme, "App.ColorScheme"
		}
{{- end}}
		// A preview's ColorScheme beats the App's: telatest applies it
		// after the options (ADR-250).
		if p, ok := root.(telatela.Preview); ok && p.ColorScheme != telatela.System {
			scheme, schemeOf = p.ColorScheme, telafmt.Sprintf("the ColorScheme of preview %q", preview)
		}
		switch scheme {
		case telatela.Light:
			if dark {
				s.note = "dark has no effect: " + schemeOf + " is tela.Light, which overrides the system's preference"
			}
			dark = false
		case telatela.Dark:
			if !dark {
				s.note = "the screen is dark: " + schemeOf + " is tela.Dark, which overrides the system's preference"
			}
			dark = true
		}
		s.w, s.h, s.scale, s.dark = w, h, req.Scale, dark
{{- if .DevTools}}
		s.prefer, s.fixedScheme, s.schemeOf = pf, scheme != telatela.System, schemeOf
		s.actionFrom = 0
{{- end}}
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
{{- if .DevTools}}
		if telaMCPActs[req.Op] {
			// Frames after this one are the ones the command causes.
			telaMCPProtect(func() { s.actionFrom = s.app.Stats().Frames })
		}
{{- end}}
		s.tb.cancelled.Store(false)
		s.tb.running.Store(int64(req.ID))
		if s.tb.cancelTarget.Load() == int64(req.ID) {
			s.tb.cancelled.Store(true) // the cancel was read before the command began
		}
		fail, panicked := telaMCPProtect(func() { s.do(req, &rep) })
		s.tb.running.Store(0)
{{- if .DevTools}}
		if telaMCPActs[req.Op] && panicked == "" {
			// The frames the command caused travel with its reply.
			telaMCPProtect(func() { rep.Frames = s.app.Frames(s.actionFrom) })
		}
{{- end}}
		rep.Error = fail
		if panicked != "" {
			rep.Panic = panicked
			s.remount()
		}
	}
	rep.MountError = s.mountErr
	if req.Snapshot && s.app != nil {
		var y string
		var nodes []uint64
		var tids []string
		if fail, panicked := telaMCPProtect(func() { y, nodes = s.snapshot(); tids = s.testIDs(nodes) }); fail == "" && panicked == "" {
			rep.YAML, rep.Nodes, rep.TestIDs = &y, nodes, tids
		} else if panicked != "" {
			rep.Panic += panicked
			s.remount()
			rep.MountError = s.mountErr
		}
	}
	rep.Preview, rep.Names = s.preview, s.names
	rep.W, rep.H, rep.Scale, rep.Dark, rep.Note = s.w, s.h, s.scale, s.dark, s.note
{{- if .DevTools}}
	rep.DevTools = true
{{- end}}
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

// testIDs returns, for each element of a snapshot, its test id when it
// has one that no other element of the window has (ByTestID finds it
// alone), "" otherwise; nil when no element has one, or when this version
// of telatest cannot say (TestIDOf).
func (s *telaMCPSession) testIDs(nodes []uint64) []string {
	t, ok := any(s.app).(interface{ TestIDOf(uint64) string })
	if !ok {
		return nil
	}
	var out []string
	alone := map[string]bool{}
	for i, n := range nodes {
		if n == 0 {
			continue
		}
		id := t.TestIDOf(n)
		if id == "" {
			continue
		}
		one, seen := alone[id]
		if !seen {
			one = s.app.Count(telatest.ByTestID(id)) == 1
			alone[id] = one
		}
		if !one {
			continue
		}
		if out == nil {
			out = make([]string, len(nodes))
		}
		out[i] = id
	}
	return out
}

// snapshot returns the aria snapshot and, when this version of telatest
// gives them, the identity of the element of each line.
func (s *telaMCPSession) snapshot() (string, []uint64) {
{{- if .DevTools}}
	return s.app.AriaSnapshotSources()
{{- end}}
	if n, ok := any(s.app).(telaMCPNodes); ok {
		return n.AriaSnapshotNodes()
	}
	return s.app.AriaSnapshot(), nil
}

{{if .Kinds -}}
// telaMCPQuiet reports whether an item of App.Pending cannot change what
// is on the screen (ADR-308): the blink of a caret, the fade of a
// scrollbar, an animation that never ends, the debug overlays. A miss that
// waits on nothing else will not turn into a match.
func telaMCPQuiet(k telatest.PendingKind) bool {
	switch k {
	case telatest.PendingCaret, telatest.PendingScrollbar, telatest.PendingEndless, telatest.PendingDebug:
		return true
	}
	return false
}

// settled reports whether nothing pending can change the screen. An
// endless animation (a spinner) asks for a frame every refresh, so
// App.Pending lists a frame request beside it: the request is quiet when
// everything else is, and only then. A UseAsync run or a UseGo goroutine
// that has not returned can still post what the screen waits for.
func (s *telaMCPSession) settled() bool {
	frame, others := false, 0
	for _, p := range s.app.Pending() {
		if p.Kind == telatest.PendingFrame {
			frame = true
			continue
		}
		if !telaMCPQuiet(p.Kind) {
			return false
		}
		others++
	}
	return !frame || others > 0 // else a frame is owed: let telatest settle and look again
}
{{- else -}}
// telaMCPQuiet reports whether an item of App.Pending cannot change what
// is on the screen: the blink of a caret, the fade of a scrollbar, an
// animation that never ends, the debug overlays. A tela older than
// ADR-308 names them only by their text.
func telaMCPQuiet(why string) bool {
	for _, q := range []string{"the caret of the focused text field", "overlay scrollbars that start fading", "an endless animation", "the debug overlays"} {
		if telastrings.Contains(why, q) {
			return true
		}
	}
	return false
}

// settled reports whether nothing pending can change the screen (see the
// version for kinds); an older telatest without Pending never says so.
func (s *telaMCPSession) settled() bool {
	p, ok := any(s.app).(interface{ Pending() []string })
	if !ok {
		return false // an older telatest: it waits as it always did
	}
	frame, others := false, 0
	for _, why := range p.Pending() {
		if telastrings.HasSuffix(why, "a frame request") {
			frame = true
			continue
		}
		if !telaMCPQuiet(why) {
			return false
		}
		others++
	}
	return !frame || others > 0
}
{{- end}}

// telaMCPSettledWhy ends the messages of a miss that waiting cannot help.
const telaMCPSettledWhy = "the screen is settled (nothing is animating, timed, queued or loading), so waiting would not help"

// preflight fails at once for an element that is not on the screen when
// nothing is pending: telatest waits for it to appear, which only helps
// while something (a frame, an animation, a timer, a queued update, a
// load) can still change the screen. It leaves the rest to telatest: a
// locator that matches, or one that matches several (its strictness
// message), or an app where something is pending.
func (s *telaMCPSession) preflight(l *telaMCPLoc) {
	if l == nil {
		return
	}
	loc := l.locator(s.app)
	if s.app.Count(loc) > 0 || !s.settled() {
		return
	}
	panic(telaMCPFail{"no element matches " + loc.String() + "; " + telaMCPSettledWhy})
}

// waitFailsFast fails a wait for a text at once when waiting cannot bring
// it (or, gone, take it away), as preflight does for an element. The
// locator's match ignores case where the page's text does not, so a text
// to be gone fails fast only when an element shows exactly it.
func (s *telaMCPSession) waitFailsFast(text string, gone bool) {
	if gone {
		if s.app.Count(telatest.ByText(text, telatest.Exact())) > 0 && s.settled() {
			panic(telaMCPFail{telafmt.Sprintf("the text %q is still on the screen; %s", text, telaMCPSettledWhy)})
		}
		return
	}
	if s.app.Count(telatest.ByText(text)) == 0 && s.settled() {
		panic(telaMCPFail{telafmt.Sprintf("the window does not show the text %q; %s", text, telaMCPSettledWhy)})
	}
}

func (s *telaMCPSession) do(req telaMCPReq, rep *telaMCPReply) {
	app := s.app
	need := func(what string, l *telaMCPLoc) telatest.Locator {
		if l == nil {
			panic(telaMCPFail{what + " needs an element"})
		}
		return l.locator(app)
	}
	if req.Op != "count" {
		s.preflight(req.Loc)
		s.preflight(req.To)
	}
	switch req.Op {
	case "snapshot", "info":
		// The snapshot comes after every command that asks for it, and the
		// screen's settings with every reply.
	case "count":
		for i := range req.Locs {
			n := -1
			if _, panicked := telaMCPProtect(func() { n = app.Count(req.Locs[i].locator(app)) }); panicked != "" {
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
			r := app.Bounds(req.Loc.locator(app))
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
			rep.Layout, rep.Subtree = string(of.LayoutJSONOf(req.Loc.locator(app))), true
		case req.Loc != nil:
			rep.Layout = string(app.LayoutJSON())
			r := app.Bounds(req.Loc.locator(app))
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
			app.DoubleClick(req.Loc.locator(app))
		case req.Loc != nil:
			app.Click(req.Loc.locator(app), opts...)
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
			app.Focus(req.Loc.locator(app))
		}
		app.Press(req.Key)
	case "scroll":
		// Whether anything moved: a scroll offset moves the painted bounds
		// of what the container holds, which the layout tree gives.
		moved := func(do func()) bool {
			before := string(app.LayoutJSON())
			do()
			return string(app.LayoutJSON()) != before
		}
		yes := true
		if req.Loc != nil {
			loc := need("scroll", req.Loc)
			if !moved(func() { app.Scroll(loc, float32(req.DX), float32(req.DY)) }) {
				yes = false
			}
			rep.Moved = &yes
			break
		}
{{- if .Scrolls}}
		if req.At == nil {
			// Without an element or a point: the largest scroll container
			// that moves that way, as a person scrolls the page; the wheel
			// at its center may move a container inside it, as it would.
			ids := app.Scrollables()
			for _, id := range ids {
				if moved(func() { app.Scroll(app.NodeLocator(id), float32(req.DX), float32(req.DY)) }) {
					rep.Moved, rep.Scrolled = &yes, id
					break
				}
			}
			if rep.Moved == nil {
				no := false
				rep.Moved = &no
			}
			break
		}
{{- end}}
		// A wheel at a point (the window's center unless given), which
		// scrolls the nearest scroll container under it, as a pointer
		// resting there would.
		at := telaMCPPoint{s.w / 2, s.h / 2}
		if req.At != nil {
			at = *req.At
		}
		sc, ok := any(app).(interface{ ScrollAt(x, y, dx, dy float32) })
		if !ok {
			panic(telaMCPFail{"this version of tela cannot scroll at a point; name an element to scroll over"})
		}
		if !moved(func() { sc.ScrollAt(float32(at.X), float32(at.Y), float32(req.DX), float32(req.DY)) }) {
			yes = false
		}
		rep.Moved = &yes
	case "drag":
		switch {
		case req.At != nil && req.ToAt != nil:
			app.DragFromTo(telatela.Point{X: float32(req.At.X), Y: float32(req.At.Y)}, telatela.Point{X: float32(req.ToAt.X), Y: float32(req.ToAt.Y)})
		case req.Loc != nil && req.To != nil:
			app.Drag(req.Loc.locator(app), req.To.locator(app))
		case req.Loc != nil:
			app.DragBy(req.Loc.locator(app), float32(req.DX), float32(req.DY))
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
	case "film":
{{- if .Film}}
		// The motion that Inner starts (or the app as it is), filmed on the
		// app's clock; the answer's snapshot, taken after, is the settled
		// state.
		span := telatime.Duration(req.Ms * float64(telatime.Millisecond))
		every := telatime.Duration(req.Every * float64(telatime.Millisecond))
		for _, f := range app.Film(span, every, func() {
			if req.Inner != nil {
				var ignored telaMCPReply
				s.do(*req.Inner, &ignored)
			}
		}) {
			var buf telaMCPBuffer
			enc := telapng.Encoder{CompressionLevel: telapng.BestSpeed}
			if err := enc.Encode(&buf, f.Image); err != nil {
				panic(telaMCPFail{err.Error()})
			}
			rep.Film = append(rep.Film, telaMCPFilmFrame{f.At.Milliseconds(), buf.b})
		}
{{- else}}
		panic(telaMCPFail{"this package's tela module predates telatest's App.Film; update it with go get github.com/kuronosu/tela@latest, and film works after the next rebuild"})
{{- end}}
	case "wait":
		// In steps of 100 ms of app time, so that a cancel ends a long wait
		// at the next one.
		for left := telatime.Duration(req.Ms * float64(telatime.Millisecond)); left > 0; {
			step := min(left, 100*telatime.Millisecond)
			app.Advance(step)
			left -= step
		}
		if req.Text != "" {
			s.waitFailsFast(req.Text, false)
			app.ExpectText(req.Text)
		}
		if req.Gone != "" {
			s.waitFailsFast(req.Gone, true)
			app.ExpectNoText(req.Gone)
		}
{{- if .DevTools}}
	case "state":
		s.state(req, rep)
	case "perf":
		now := app.Stats().Frames
		rep.FrameNow = now
		if req.FromAction {
			rep.Frames = app.Frames(s.actionFrom)
		} else {
			rep.Frames = app.Frames(max(0, now-max(req.Count, 1)))
		}
	case "prefer":
		s.preferOp(req, rep)
{{- end}}
	default:
		panic(telaMCPFail{telafmt.Sprintf("unknown command %q", req.Op)})
	}
}
{{if .DevTools}}
// telaMCPActs are the commands that act on the app: the frames after the
// start of one are what it caused (perf).
var telaMCPActs = map[string]bool{"click": true, "hover": true, "fill": true, "press": true, "scroll": true, "drag": true, "resize": true, "wait": true, "prefer": true, "film": true}

// state answers with the component of the element req.Loc names (its
// node identity), or with the components that have props or hooks.
func (s *telaMCPSession) state(req telaMCPReq, rep *telaMCPReply) {
	if req.Loc != nil {
		id := req.Loc.NodeID
		if id == 0 {
			// Role and name, text or test id: the element's identity, asked
			// through an interface (a telatest older than NodeOf).
			n, ok := any(s.app).(interface{ NodeOf(telatest.Locator) uint64 })
			if !ok {
				panic(telaMCPFail{"this version of tela names an element for state only by its ref from the latest snapshot"})
			}
			id = n.NodeOf(req.Loc.locator(s.app))
		}
		c, ok := s.app.ComponentOf(id)
		if !ok {
			panic(telaMCPFail{"the element has no component above it that can be inspected (it was unmounted, or its screen was built before the instrumentation)"})
		}
		rep.Components, rep.Total = []telatest.Component{c}, 1
		return
	}
	limit := req.Max
	if limit <= 0 {
		limit = 40
	}
	for _, c := range s.app.Components() {
		if len(c.Hooks) == 0 && len(c.Props) == 0 {
			continue
		}
		rep.Total++
		if len(rep.Components) < limit {
			rep.Components = append(rep.Components, c)
		}
	}
}

// preferOp puts the screen in the preferences req.Prefer and req.Dark
// say: reduced motion, high contrast, the text scale, the locale and dark
// mode as the system changes them while the app runs; the direction, which
// is part of the tree, by mounting the screen afresh.
func (s *telaMCPSession) preferOp(req telaMCPReq, rep *telaMCPReply) {
	app := s.app
	var want telaMCPPrefer
	if req.Prefer != nil {
		want = *req.Prefer
	}
	want = want.normalized()
	cur := s.prefer
	if want.Direction != cur.Direction {
		s.mount.Prefer, s.mount.Dark = &want, req.Dark
		s.remount()
		rep.Remounted = true
		rep.Applied = append(rep.Applied, "direction "+telafmt.Sprintf("%q", want.Direction)+": mounted the screen afresh (state reset), with every preference set")
		rep.Prefer = &want
		return
	}
	live := func(name string, changed bool, set func()) {
		if changed {
			set()
			rep.Applied = append(rep.Applied, name+": live")
		}
	}
	if req.Dark != s.mount.Dark {
		s.mount.Dark = req.Dark
		if s.fixedScheme {
			rep.Applied = append(rep.Applied, "dark: no effect, "+s.schemeOf+" fixes the color scheme")
		} else {
			live("dark", true, func() { app.SetDarkMode(req.Dark); s.dark = req.Dark })
		}
	}
	live("reducedMotion", want.ReducedMotion != cur.ReducedMotion, func() { app.SetReducedMotion(want.ReducedMotion) })
	live("highContrast", want.HighContrast != cur.HighContrast, func() { app.SetHighContrast(want.HighContrast) })
	live("textScale", want.TextScale != cur.TextScale, func() { app.SetTextScale(want.TextScale) })
	if want.LocaleSet && (!cur.LocaleSet || want.Locale != cur.Locale) {
{{- if .I18N}}
		if err := telaMCPSetLocale(want.Locale); err != nil {
			panic(telaMCPFail{err.Error()})
		}
		app.Advance(telatime.Millisecond) // the components that read the locale build again
		rep.Applied = append(rep.Applied, "locale: live (the components that read it with i18n.UseLocale, UseT or UseFormatter built again)")
{{- else}}
		rep.Applied = append(rep.Applied, "locale: no effect, the package does not import tela/i18n")
		want.LocaleSet, want.Locale = false, ""
{{- end}}
	}
	s.prefer = want
	s.mount.Prefer = &want
	rep.Prefer = &want
}
{{end}}

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
	// The commands are read on a goroutine of their own, so that a cancel
	// reaches the command that is running (the server sends one command
	// at a time, so at most one line waits for the loop).
	type telaMCPLine struct {
		data []byte
		err  error
	}
	lines := make(chan telaMCPLine)
	go func() {
		for {
			line, err := in.ReadBytes('\n')
			if telastrings.Contains(string(line), "\"cancel\"") {
				var c telaMCPReq
				if telajson.Unmarshal(line, &c) == nil && c.Op == "cancel" {
					s.tb.cancel(c.Target)
					line = nil
					if err == nil {
						continue
					}
				}
			}
			lines <- telaMCPLine{line, err}
			if err != nil {
				return
			}
		}
	}()
	for {
		l := <-lines
		line, err := l.data, l.err
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
