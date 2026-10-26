package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kuronosu/tela/internal/lint"
)

const mcpUsage = `Usage:

	tela mcp [--size WxH] [--scale N] [--dark] [--preview NAME] [-v] [pkg]

Serves the Model Context Protocol on standard input and output (JSON-RPC
2.0, one message per line), so that an agent can drive the package's screen:
the one "tela snapshot" renders, mounted headlessly and kept running between
calls (ADR-174). <pkg> is a package path or a relative directory; it
defaults to "." and may not exist yet when the server starts.

Before each call the server checks the package's sources (the files tela dev
watches); when they changed it rebuilds the instance and mounts the screen
afresh, so the state is reset. A compile error is the result of that call,
with the findings of tela lint, and the last good build keeps answering.

Tools:

	snapshot    the aria snapshot (YAML), each element that can be acted on
	            marked [ref=eN]
	screenshot  a PNG of the window, or of one element (ref)
	click       click an element (ref, or role and name, text, testId) or a
	            point (x, y); button "right", double
	hover       move the pointer over an element
	fill        replace the text of a field; submit presses Enter
	press       press a key or combo ("Enter", "Shift+Tab", "Control+a")
	scroll      scroll the wheel over an element by dx, dy
	drag        drag from an element or point to another, or by dx, dy
	resize      change the window's size or scale, keeping the state
	open        mount another preview, scheme or size afresh; without
	            arguments, list the previews
	wait        advance the app's clock (ms), or wait for a text
	inspect     the layout tree (JSON), whole or one element
	lint        tela lint -json on the package

Every tool that acts returns the new snapshot, and every result carries the
dev warnings and errors the screen logged meanwhile.

Clients start the server with a command line, such as, for Claude Code:

	claude mcp add tela -- tela mcp ./myapp

Flags:

	--size WxH       logical size of the window (default: the App's Width and
	                 Height when the package defines them, else 960x640)
	--scale N        display scale (default 1)
	--dark           the system prefers dark mode (App.ColorScheme still wins)
	--preview NAME   the named screen of a Previews variable
	-v               log the protocol, the builds and the instance's output
	                 on standard error
`

// mcpOptions are the arguments of tela mcp.
type mcpOptions struct {
	Pkg     string
	W, H    float64
	SizeSet bool
	Scale   float64
	Dark    bool
	Preview string
	Verbose bool
}

// parseMCPArgs parses the arguments of tela mcp. Flags and the package may
// come in any order; the package defaults to ".".
func parseMCPArgs(args []string) (mcpOptions, error) {
	o := mcpOptions{W: defaultWidth, H: defaultHeight, Scale: 1}
	var size string
	fs := flag.NewFlagSet("tela mcp", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&size, "size", "", "")
	fs.Float64Var(&o.Scale, "scale", 1, "")
	fs.BoolVar(&o.Dark, "dark", false, "")
	fs.StringVar(&o.Preview, "preview", "", "")
	fs.BoolVar(&o.Verbose, "v", false, "")
	fs.BoolVar(&o.Verbose, "verbose", false, "")
	var pkgs []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return o, err
			}
			return o, fmt.Errorf("%v (see \"tela help mcp\")", err)
		}
		args = fs.Args()
		if len(args) > 0 {
			pkgs = append(pkgs, args[0])
			args = args[1:]
		}
	}
	switch len(pkgs) {
	case 0:
		o.Pkg = "."
	case 1:
		o.Pkg = pkgs[0]
	default:
		return o, fmt.Errorf("expected at most one <pkg>, got %d (%s)", len(pkgs), strings.Join(pkgs, ", "))
	}
	ro := renderOptions{W: o.W, H: o.H, Scale: o.Scale}
	if err := ro.validate(size); err != nil {
		return o, err
	}
	o.W, o.H, o.SizeSet = ro.W, ro.H, ro.SizeSet
	return o, nil
}

// runMCP implements tela mcp.
func runMCP(args []string, stdout, stderr io.Writer) int {
	opts, err := parseMCPArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, mcpUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "tela mcp: %v\n\n%s", err, mcpUsage)
		return 2
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintln(stderr, "tela mcp: the go command was not found in PATH; install Go (https://go.dev/dl) and try again")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serveMCP(ctx, goBin, opts, os.Stdin, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "tela mcp: %v\n", err)
		return 1
	}
	return 0
}

// serveMCP serves one client on in and out until in ends or ctx is done.
// Logs go to stderr with -v only.
func serveMCP(ctx context.Context, goBin string, opts mcpOptions, in io.Reader, out, stderr io.Writer) error {
	logf := func(string, ...any) {}
	var childErr io.Writer
	if opts.Verbose {
		var mu sync.Mutex
		logf = func(format string, args ...any) {
			mu.Lock()
			fmt.Fprintf(stderr, "tela mcp: "+format+"\n", args...)
			mu.Unlock()
		}
		childErr = stderr
	}
	tmp, err := os.MkdirTemp("", "tela-mcp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	s := newMCPSession(goBin, opts, tmp, logf, childErr)
	defer s.close()

	// Build right away, so that the first call finds the instance ready.
	// The build stops when the server does.
	early, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		notes, _ := s.ensure(early)
		s.pending = notes
	}()

	srv := &rpcServer{tools: s, name: "tela", version: mcpServerVersion(), logf: logf, out: out}
	err = srv.serve(ctx, in)
	cancel()
	return err
}

// mcpServerVersion is the version of this binary's module, for serverInfo.
func mcpServerVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// mcpMount is how the screen is mounted.
type mcpMount struct {
	Preview string
	W, H    float64
	SizeSet bool
	Scale   float64
	Dark    bool
}

// mcpSession is the state of the server: the package, its build and the
// running instance. Its methods run with mu held, one call at a time.
type mcpSession struct {
	goBin  string
	pkgArg string
	tmp    string
	logf   func(string, ...any)
	errOut io.Writer // the instance's standard error (-v), or nil

	mu       sync.Mutex
	target   *mcpTarget
	gen      int
	built    bool   // a build of builtFP was attempted
	builtFP  uint64 // the fingerprint of the sources of the last build
	broken   string // the problem of the sources at builtFP, if any
	child    *mcpChild
	mount    mcpMount
	refs     *refTable
	pending  []string // notes for the next result
	carry    mcpReply // logs and output of a mount that no call returned yet
	contract contract // of the last build
	lintNote string   // lint findings of the last build, for the next result

	namesMu sync.Mutex
	names   []string // the previews, for the description of open
}

func newMCPSession(goBin string, opts mcpOptions, tmp string, logf func(string, ...any), errOut io.Writer) *mcpSession {
	return &mcpSession{
		goBin: goBin, pkgArg: opts.Pkg, tmp: tmp, logf: logf, errOut: errOut,
		mount: mcpMount{Preview: opts.Preview, W: opts.W, H: opts.H, SizeSet: opts.SizeSet, Scale: opts.Scale, Dark: opts.Dark},
		refs:  newRefTable(),
	}
}

// close stops the instance.
func (s *mcpSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.child != nil {
		s.child.stop()
		s.child = nil
	}
}

// ensure brings the instance up to date with the sources: it resolves the
// package, rebuilds and remounts when the fingerprint changed, and starts
// the instance again when it exited. It returns notes for the result and,
// when the call cannot go on, the problem to report.
func (s *mcpSession) ensure(ctx context.Context) (notes []string, problem string) {
	notes, s.pending = s.pending, nil
	if s.target == nil {
		t, err := resolveMCPTarget(s.goBin, s.pkgArg, s.tmp)
		if err != nil {
			return notes, err.Error() + "\n\n" + mcpCreateHelp(s.pkgArg)
		}
		s.target = t
		s.logf("package %s in %s", t.arg, t.modDir)
	}
	fp := s.target.fingerprint()
	if !s.built || fp != s.builtFP {
		if s.built {
			// The sources changed: the modules and embedded files may have
			// too.
			s.target.refreshDeps(s.goBin)
			fp = s.target.fingerprint()
		}
		notes = append(notes, s.rebuild(ctx, fp)...)
		if s.broken != "" && s.child == nil {
			return notes, s.broken
		}
		if s.broken != "" {
			return notes, s.broken + "\n\nThe last good build is still running: the other tools use it until the source is fixed."
		}
		return notes, ""
	}
	if s.broken != "" {
		if s.child == nil {
			return notes, s.broken
		}
		notes = append(notes, "The source still has the problem a previous result reported; this is the last good build.")
	}
	if s.child != nil && !s.child.alive() {
		why := s.child.exitReport()
		if err := s.restart(ctx); err != nil {
			return notes, err.Error()
		}
		notes = append(notes, "The instance had exited after the previous call ("+why+"); it was started again and the screen mounted afresh (state reset).")
	}
	if s.child == nil {
		return notes, "no instance is running"
	}
	return notes, ""
}

// mcpCreateHelp explains how to make a package the server can serve.
func mcpCreateHelp(pkg string) string {
	return fmt.Sprintf(`tela mcp serves the package %s, which must be in a module that requires github.com/kuronosu/tela (create one with "tela new %s", or "go mod init" and "go get github.com/kuronosu/tela"). The package defines the screen as one of

	func Preview() tela.Node        (or tela.Component)
	var Previews = tela.Previews{"Name": {Root: tela.C(Screen)}}
	var App = tela.App{Root: tela.C(Screen), ...}

Write it, then call snapshot again.`, pkg, pkg)
}

// rebuild builds the sources with fingerprint fp and mounts the new
// instance in place of the old one. A failure sets s.broken and keeps the
// old instance.
func (s *mcpSession) rebuild(ctx context.Context, fp uint64) (notes []string) {
	start := time.Now()
	first := !s.built
	s.built, s.builtFP, s.broken = true, fp, ""
	t := s.target
	pkg, err := listPackage(s.goBin, t.pkgArg)
	if err != nil {
		s.broken = err.Error()
		return nil
	}
	t.pkg = pkg
	det, err := scanPackage(pkg.Dir, pkg.GoFiles)
	if err != nil {
		// A syntax error: the compiler would say the same.
		msg := err.Error()
		if _, after, ok := strings.Cut(msg, ": "); ok && strings.HasPrefix(msg, "cannot read ") {
			msg = after
		}
		s.broken = "The package does not compile:\n\n" + msg
		return nil
	}
	switch det.contract {
	case contractNone:
		s.broken = fmt.Sprintf("package %s (%s) has no screen to serve: %s", pkg.Name, t.pkgArg, contractHelp)
		return nil
	case contractScene:
		s.broken = fmt.Sprintf("package %s (%s) only defines Scene, a display list without elements: tela mcp needs func Preview() tela.Node, a Previews variable or an App", pkg.Name, t.pkgArg)
		return nil
	}
	s.contract = det.contract
	if s.mount.Preview != "" && det.contract != contractPreviews {
		s.broken = fmt.Sprintf("--preview %q: %s", s.mount.Preview, singleScreen(pkg.Name, det.contract))
		return nil
	}
	if names := scanPreviewNames(pkg.Dir, pkg.GoFiles); names != nil || det.contract != contractPreviews {
		s.namesMu.Lock()
		s.names = names
		s.namesMu.Unlock()
	}

	lintDone := make(chan string, 1)
	go func() { lintDone <- s.lintText() }()
	s.gen++
	exe, err := buildInstance(ctx, s.goBin, t, det.contract, det.hasApp, s.tmp, s.gen)
	lintNote := <-lintDone
	if err != nil {
		if ctx.Err() != nil {
			s.built = false // the next call builds again
			s.broken = "cancelled"
			return nil
		}
		s.logf("build %d failed in %v", s.gen, time.Since(start).Round(time.Millisecond))
		s.broken = "The package does not compile:\n\n" + err.Error()
		if lintNote != "" {
			s.broken += "\n\n### Lint\n" + lintNote
		}
		return nil
	}
	built := time.Since(start)
	s.logf("build %d in %v", s.gen, built.Round(time.Millisecond))

	child, rep, err := s.startInstance(ctx, exe)
	if err != nil {
		os.Remove(exe)
		s.broken = "The new build does not start: " + err.Error()
		return nil
	}
	if rep.MountError != "" && s.child != nil {
		child.stop()
		os.Remove(exe)
		s.broken = "The new build compiles, but its screen cannot be mounted:\n\n" + cleanMountError(rep.MountError)
		if len(rep.Logs) > 0 {
			s.broken += "\n\n" + strings.Join(rep.Logs, "\n")
		}
		return nil
	}
	if old := s.child; old != nil {
		old.stop()
		if old.exe != exe {
			os.Remove(old.exe)
		}
	}
	s.child = child
	s.refs.remount()
	s.carry = rep
	s.lintNote = lintNote
	switch {
	case rep.MountError != "":
		notes = append(notes, fmt.Sprintf("Built in %.1fs.", time.Since(start).Seconds()))
	case first:
		notes = append(notes, fmt.Sprintf("Built and mounted in %.1fs.", time.Since(start).Seconds()))
	default:
		notes = append(notes, fmt.Sprintf("The source changed: rebuilt in %.1fs and mounted afresh (state reset).", time.Since(start).Seconds()))
	}
	return notes
}

// singleScreen explains that a package without Previews has one screen.
func singleScreen(pkg string, c contract) string {
	what := "Preview(), a single screen"
	if c == contractApp {
		what = "App, whose Root is a single screen"
	}
	return fmt.Sprintf("package %s defines %s; to have several, define var Previews = tela.Previews{\"Name\": {Root: tela.C(Screen)}}", pkg, what)
}

// startInstance starts exe and mounts the screen.
func (s *mcpSession) startInstance(ctx context.Context, exe string) (*mcpChild, mcpReply, error) {
	child, err := startChild(exe, s.target.pkg.Dir, s.errOut, s.logf)
	if err != nil {
		return nil, mcpReply{}, err
	}
	rep, err := child.call(ctx, s.mountReq(false))
	if err != nil {
		child.kill()
		return nil, mcpReply{}, err
	}
	return child, rep, nil
}

// restart starts the current build again after its instance exited.
func (s *mcpSession) restart(ctx context.Context) error {
	child, rep, err := s.startInstance(ctx, s.child.exe)
	if err != nil {
		return err
	}
	s.child = child
	s.refs.remount()
	s.carry = rep
	return nil
}

func (s *mcpSession) mountReq(snapshot bool) mcpReq {
	m := s.mount
	return mcpReq{Op: "mount", Preview: m.Preview, W: m.W, H: m.H, SizeSet: m.SizeSet, Scale: m.Scale, Dark: m.Dark, Snapshot: snapshot}
}

// lintText runs tela lint on the package and returns its findings, one per
// line, or "".
func (s *mcpSession) lintText() string {
	diags, err := s.lint()
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, d := range diags {
		b.WriteString("- " + d.String() + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (s *mcpSession) lint() ([]lint.Diagnostic, error) {
	start := time.Now()
	res, err := lint.Run(lint.Options{Patterns: []string{s.pkgArg}})
	if err != nil {
		return nil, err
	}
	s.logf("lint in %v: %d findings", time.Since(start).Round(time.Millisecond), len(res.Diagnostics))
	return res.Diagnostics, nil
}

// ---------------------------------------------------------------------
// Results.

// mcpOut builds the result of a call.
type mcpOut struct {
	notes    []string
	head     string
	isError  bool
	screen   string
	snapshot *string
	sections []string
	errs     []string
	warns    []string
	other    []string
	output   string
	lint     string
	images   []mcpContent
}

// absorb takes the logs and output of a reply.
func (o *mcpOut) absorb(rep mcpReply) {
	for _, l := range rep.Logs {
		l = strings.TrimSpace(l)
		switch {
		case l == "":
		case strings.HasPrefix(l, "dev message: "):
			o.errs = appendNew(o.errs, strings.TrimPrefix(l, "dev message: "))
		case strings.HasPrefix(l, "dev warning: "):
			o.warns = appendNew(o.warns, strings.TrimPrefix(l, "dev warning: "))
		default:
			o.other = appendNew(o.other, strings.TrimPrefix(l, "telatest: "))
		}
	}
	o.output += rep.Output
}

func appendNew(list []string, s string) []string {
	if slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// fail makes the result an error.
func (o *mcpOut) fail(msg string) {
	o.isError = true
	o.head = "Error: " + strings.TrimSpace(msg)
}

func (o *mcpOut) result() mcpResult {
	var parts []string
	if len(o.notes) > 0 {
		parts = append(parts, strings.Join(o.notes, "\n"))
	}
	if o.head != "" {
		parts = append(parts, o.head)
	}
	if o.screen != "" {
		parts = append(parts, "### Screen\n"+o.screen)
	}
	if o.snapshot != nil {
		snap := *o.snapshot
		if snap == "" {
			snap = "# (empty: nothing visible has a role or text)"
		}
		parts = append(parts, "### Snapshot\n```yaml\n"+snap+"\n```")
	}
	parts = append(parts, o.sections...)
	list := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		const max = 30
		more := ""
		if len(items) > max {
			more = fmt.Sprintf("\n- ... and %d more", len(items)-max)
			items = items[:max]
		}
		parts = append(parts, title+"\n- "+strings.Join(items, "\n- ")+more)
	}
	list("### Dev errors", o.errs)
	list("### Dev warnings", o.warns)
	list("### Notes", o.other)
	if out := strings.TrimSpace(o.output); out != "" {
		if len(out) > 4000 {
			out = "..." + out[len(out)-4000:]
		}
		parts = append(parts, "### Output of the app\n```\n"+out+"\n```")
	}
	if o.lint != "" {
		parts = append(parts, "### Lint\n"+o.lint)
	}
	content := []mcpContent{{Type: "text", Text: strings.Join(parts, "\n\n")}}
	content = append(content, o.images...)
	return mcpResult{Content: content, IsError: o.isError}
}

// describeScreen is the line about the mounted screen.
func describeScreen(rep mcpReply) string {
	var b strings.Builder
	if rep.Preview != "" {
		fmt.Fprintf(&b, "Preview %q", rep.Preview)
		if others := without(rep.Names, rep.Preview); len(others) > 0 {
			fmt.Fprintf(&b, " (others: %s)", strings.Join(others, ", "))
		}
		b.WriteString("; ")
	}
	scheme := "light"
	if rep.Dark {
		scheme = "dark"
	}
	fmt.Fprintf(&b, "window %gx%g at scale %g, %s", rep.W, rep.H, rep.Scale, scheme)
	if rep.Note != "" {
		b.WriteString("\nNote: " + rep.Note)
	}
	return b.String()
}

// begin starts a call that needs the instance: the notes of ensure, and
// the logs of a mount no call returned yet. ok is false when the call
// cannot go on (o holds the error).
func (s *mcpSession) begin(ctx context.Context, o *mcpOut) bool {
	notes, problem := s.ensure(ctx)
	o.notes = append(o.notes, notes...)
	if s.carry.ID != 0 || len(s.carry.Logs) > 0 || s.carry.Output != "" {
		o.absorb(s.carry)
		s.carry = mcpReply{}
	}
	if s.lintNote != "" {
		o.lint = s.lintNote + "\n(tela lint found these in the new build; they are what dev mode reports, found in the source)"
		s.lintNote = ""
	}
	if problem != "" {
		o.fail(problem)
		return false
	}
	return true
}

// send runs a command on the instance and reports a failure into o. It
// returns false when there is no reply to use.
func (s *mcpSession) send(ctx context.Context, o *mcpOut, req mcpReq) (mcpReply, bool) {
	rep, err := s.child.call(ctx, req)
	if err != nil {
		o.fail(err.Error())
		return rep, false
	}
	o.absorb(rep)
	o.screen = describeScreen(rep)
	if rep.MountError != "" {
		msg := "the screen cannot be mounted: " + cleanMountError(rep.MountError)
		if rep.Panic != "" {
			msg = "the app panicked: " + trimPanic(rep.Panic) + "\n\nMounting the screen afresh failed too: " + cleanMountError(rep.MountError)
		}
		s.refs.remount()
		o.fail(msg)
		o.screen = ""
		return rep, false
	}
	if rep.Panic != "" {
		s.refs.remount()
		o.fail("the app panicked: " + trimPanic(rep.Panic) + "\n\nThe screen was mounted afresh (state reset).")
	}
	if rep.Error != "" && rep.Panic == "" {
		o.fail(rep.Error)
	}
	if rep.YAML != nil {
		snap := s.withRefs(ctx, o, *rep.YAML)
		o.snapshot = &snap
	}
	return rep, rep.Error == "" && rep.Panic == ""
}

// trimPanic keeps a panic's value and the frames of the stack from the
// panicking function on, at most 12 of them (the frames that recovered it
// and the goroutine header say nothing to the reader; a frame is two
// lines, the function and its file:line).
func trimPanic(p string) string {
	value, stack, ok := strings.Cut(strings.TrimSpace(p), "\n\ngoroutine ")
	if !ok {
		return strings.TrimSpace(p)
	}
	lines := strings.Split(stack, "\n")
	start := 1 // after the goroutine header
	for i, l := range lines {
		if strings.HasPrefix(l, "panic(") {
			start = i + 2 // the frame and its file:line
			break
		}
	}
	if start > len(lines) {
		start = len(lines)
	}
	frames := lines[start:]
	if len(frames) > 24 {
		frames = append(frames[:24], "...")
	}
	return value + "\n\n" + strings.Join(frames, "\n")
}

// cleanMountError shortens the stack of a panic in a mount error.
func cleanMountError(msg string) string {
	const panics = "the screen panics when it is mounted: "
	if rest, ok := strings.CutPrefix(msg, panics); ok {
		return panics + trimPanic(rest)
	}
	return msg
}

// withRefs adds the refs to an aria snapshot.
func (s *mcpSession) withRefs(ctx context.Context, o *mcpOut, yaml string) string {
	lines, cands, groups, want := prepareRefs(yaml)
	if len(cands) == 0 {
		s.refs.current, s.refs.stale = map[string]refEntry{}, false
		return yaml
	}
	rep, err := s.child.call(ctx, mcpReq{Op: "count", Locs: groups})
	if err != nil || len(rep.Counts) != len(groups) {
		s.refs.current, s.refs.stale = map[string]refEntry{}, false
		return yaml
	}
	o.absorb(rep)
	return s.refs.assign(lines, cands, want, rep.Counts)
}

// ---------------------------------------------------------------------
// Tools.

// mcpToolDef is a tool and how it runs.
type mcpToolDef struct {
	tool  mcpTool
	props []string // the argument names, for the unknown-argument check
	run   func(s *mcpSession, ctx context.Context, args json.RawMessage) mcpResult
}

// elementArgs is how a tool names an element.
type elementArgs struct {
	Ref    string `json:"ref"`
	Role   string `json:"role"`
	Name   string `json:"name"`
	Text   string `json:"text"`
	Exact  bool   `json:"exact"`
	TestID string `json:"testId"`
	Nth    *int   `json:"nth"`
	// Element is a description of the element (Playwright MCP's tools take
	// one), accepted and not used.
	Element string `json:"element"`
}

func (a elementArgs) given() bool {
	return a.Ref != "" || a.Role != "" || a.Name != "" || a.Text != "" || a.TestID != ""
}

// mcpElement is a resolved element argument.
type mcpElement struct {
	loc      mcpLoc
	desc     string
	disabled bool
}

// element resolves an element argument. A ref given after the screen was
// mounted again (a rebuild in this very call) is looked up in a new
// snapshot.
func (s *mcpSession) element(ctx context.Context, o *mcpOut, a elementArgs) (mcpElement, error) {
	switch {
	case a.Ref != "":
		if s.refs.stale {
			if rep, err := s.child.call(ctx, mcpReq{Op: "snapshot", Snapshot: true}); err == nil && rep.YAML != nil {
				o.absorb(rep)
				s.withRefs(ctx, o, *rep.YAML)
			}
		}
		e, err := s.refs.lookup(a.Ref)
		if err != nil {
			return mcpElement{}, err
		}
		return mcpElement{loc: e.loc, desc: e.desc + " [ref=" + strings.TrimSpace(a.Ref) + "]", disabled: e.disabled}, nil
	case a.TestID != "":
		return mcpElement{loc: mcpLoc{TestID: a.TestID, Nth: a.Nth}, desc: "the element with test id " + strconv.Quote(a.TestID)}, nil
	case a.Role != "":
		d := a.Role
		if a.Name != "" {
			d += " " + strconv.Quote(a.Name)
		}
		return mcpElement{loc: mcpLoc{Role: a.Role, Name: a.Name, Nth: a.Nth}, desc: d}, nil
	case a.Name != "":
		return mcpElement{}, errors.New(`name needs a role: {"role": "button", "name": "Save"}; or use the element's ref from the snapshot`)
	case a.Text != "":
		return mcpElement{loc: mcpLoc{Text: a.Text, Exact: a.Exact, Nth: a.Nth}, desc: "the element with the text " + strconv.Quote(a.Text)}, nil
	}
	return mcpElement{}, errors.New(`give the element: its ref from the latest snapshot ({"ref": "e5"}), or role and name, text, or testId`)
}

// decodeArgs decodes the arguments of a tool into v.
func decodeArgs(args json.RawMessage, v any) error {
	if len(args) == 0 || string(args) == "null" {
		return nil
	}
	if err := json.Unmarshal(args, v); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) && te.Field != "" {
			return fmt.Errorf("argument %q must be a %s, not a %s", te.Field, jsonKind(te.Type.Kind().String()), te.Value)
		}
		return fmt.Errorf("the arguments are not a JSON object of the tool's parameters: %v", err)
	}
	return nil
}

func jsonKind(goKind string) string {
	switch goKind {
	case "string":
		return "string"
	case "bool":
		return "boolean"
	case "int", "int64":
		return "integer"
	case "float64", "float32":
		return "number"
	case "ptr":
		return "integer"
	}
	return goKind
}

// checkArgNames reports an argument the tool does not take.
func checkArgNames(def *mcpToolDef, args json.RawMessage) error {
	if len(args) == 0 || string(args) == "null" {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(args, &m); err != nil {
		return errors.New("the arguments must be a JSON object")
	}
	var unknown []string
	for k := range m {
		if !slices.Contains(def.props, k) && k != "element" {
			unknown = append(unknown, strconv.Quote(k))
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	takes := "no arguments"
	if len(def.props) > 0 {
		takes = strings.Join(def.props, ", ")
	}
	return fmt.Errorf("%s takes %s; unknown argument %s", def.tool.Name, takes, strings.Join(unknown, ", "))
}

// list implements mcpTools.
func (s *mcpSession) list() []mcpTool {
	var out []mcpTool
	for _, d := range s.defs() {
		out = append(out, d.tool)
	}
	return out
}

// call implements mcpTools.
func (s *mcpSession) call(ctx context.Context, name string, args json.RawMessage) (mcpResult, bool) {
	for _, d := range s.defs() {
		if d.tool.Name != name {
			continue
		}
		if err := checkArgNames(&d, args); err != nil {
			var o mcpOut
			o.fail(err.Error())
			return o.result(), true
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		start := time.Now()
		res := d.run(s, ctx, args)
		s.logf("%s in %v", name, time.Since(start).Round(time.Millisecond))
		return res, true
	}
	return mcpResult{}, false
}

// instructions implements mcpTools.
func (s *mcpSession) instructions() string {
	return fmt.Sprintf(`This server runs the Tela screen of the Go package %s headlessly and keeps it running between calls. Tela is a pure-Go UI framework: React-like components (tela.UseState, h.Div, h.Button...) styled with Tailwind CSS v4 classes (h.Class("flex gap-4 rounded-xl bg-white p-6 shadow-sm dark:bg-zinc-900")).
The loop: edit the Go source, then call snapshot (structure, cheap) or screenshot (how it looks). Every call first rebuilds and mounts the screen afresh when the source changed (the state resets) and reports compile errors, tela lint findings and dev-mode errors such as unknown classes with a suggestion.
Act on elements with click, fill, press, hover, scroll and drag, naming them by the [ref=eN] of the latest snapshot; each returns the new snapshot. resize keeps the state; open mounts another preview, dark mode or size afresh; wait advances the app's clock for timers and animations; inspect shows the layout tree when something is misplaced.
The package defines its screen as func Preview() tela.Node, var Previews = tela.Previews{...} or var App = tela.App{Root: ...}; if it does not exist yet, write it and call snapshot.`, s.pkgArg)
}

// defs returns the tools; the description of open lists the previews.
func (s *mcpSession) defs() []mcpToolDef {
	s.namesMu.Lock()
	names := s.names
	s.namesMu.Unlock()
	return mcpToolDefs(names)
}

// Schemas.

type schemaProp struct {
	name string
	typ  string // JSON type; "point" for {x, y}; "element" for an element object
	desc string
	enum []string
}

func elementProps(what string) []schemaProp {
	return []schemaProp{
		{"ref", "string", "Ref of the " + what + " in the latest snapshot, such as \"e5\" (the usual way)", nil},
		{"role", "string", "ARIA role of the " + what + ", such as \"button\"; with name", nil},
		{"name", "string", "Exact accessible name, with role", nil},
		{"text", "string", "Text the " + what + " contains (case-insensitive substring; exact: true for the whole text)", nil},
		{"exact", "boolean", "With text: match the whole text, case-sensitive", nil},
		{"testId", "string", "The h.TestID of the " + what, nil},
		{"nth", "integer", "0-based index when the locator matches several elements", nil},
	}
}

func schemaOf(props []schemaProp, required ...string) json.RawMessage {
	p := map[string]any{}
	for _, sp := range props {
		var v map[string]any
		switch sp.typ {
		case "element":
			inner := map[string]any{}
			for _, e := range append(elementProps("element"),
				schemaProp{"x", "number", "X in logical pixels from the window's left edge (instead of an element)", nil},
				schemaProp{"y", "number", "Y in logical pixels from the window's top edge", nil}) {
				inner[e.name] = map[string]any{"type": e.typ, "description": e.desc}
			}
			v = map[string]any{"type": "object", "description": sp.desc, "properties": inner, "additionalProperties": false}
		default:
			v = map[string]any{"type": sp.typ, "description": sp.desc}
			if len(sp.enum) > 0 {
				v["enum"] = sp.enum
			}
		}
		p[sp.name] = v
	}
	schema := map[string]any{"type": "object", "properties": p, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	b, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	return b
}

func propNames(props []schemaProp) []string {
	var out []string
	for _, p := range props {
		out = append(out, p.name)
	}
	return out
}

// mcpToolDefs returns the tools, with the preview names (if known) in the
// description of open.
func mcpToolDefs(previews []string) []mcpToolDef {
	ro := func(title string) *mcpToolAnnotate {
		return &mcpToolAnnotate{Title: title, ReadOnlyHint: true, IdempotentHint: true}
	}
	act := func(title string) *mcpToolAnnotate { return &mcpToolAnnotate{Title: title} }
	def := func(name, desc string, ann *mcpToolAnnotate, props []schemaProp, run func(*mcpSession, context.Context, json.RawMessage) mcpResult, required ...string) mcpToolDef {
		return mcpToolDef{tool: mcpTool{Name: name, Description: desc, InputSchema: schemaOf(props, required...), Annotations: ann}, props: propNames(props), run: run}
	}
	point := []schemaProp{
		{"x", "number", "X in logical pixels from the window's left edge (instead of an element)", nil},
		{"y", "number", "Y in logical pixels from the window's top edge", nil},
	}
	openDesc := `Mount the screen afresh (state reset): another preview of the package's Previews, dark or light mode, another size or scale. Without arguments, list the previews and the current settings. Returns the new snapshot. Example: {"preview": "Login", "dark": true}`
	if len(previews) > 0 {
		openDesc += "\nPreviews: " + strings.Join(previews, ", ")
	}
	return []mcpToolDef{
		def("snapshot", `Show the screen's structure as an aria snapshot (Playwright's YAML): roles, accessible names, states and texts, with [ref=eN] on each element you can act on. Cheap: use it first, after every edit and to get refs; use screenshot to judge how it looks. When the Go source changed, the screen is rebuilt and mounted afresh first (state reset), and compile errors, lint findings and dev errors (unknown Tailwind v4 classes, with a suggestion) are reported. Example: {}`,
			ro("Aria snapshot"), nil, (*mcpSession).toolSnapshot),
		def("screenshot", `Render the window, or one element (ref, with an 8 px margin), to a PNG at the current size and scale. Use it to judge the looks: spacing, alignment, colors, typography, dark mode. Classes are Tailwind v4 (rounded-2xl bg-white p-6 shadow-sm ring-1 ring-black/5 dark:bg-zinc-900). Example: {} or {"ref": "e5"}`,
			ro("Screenshot"), elementProps("element"), (*mcpSession).toolScreenshot),
		def("click", `Click an element with real pointer events (hover, press, release; focus moves to it) and return the new snapshot. Name it by its ref from the latest snapshot (or role and name, text, testId), or give x and y. button "right" opens a context menu; double: true double-clicks. A disabled element cannot be clicked. Example: {"ref": "e5"}`,
			act("Click"), append(append(elementProps("element"), point...),
				schemaProp{"button", "string", `"left" (default) or "right"`, []string{"left", "right"}},
				schemaProp{"double", "boolean", "Double-click", nil}), (*mcpSession).toolClick),
		def("hover", `Move the pointer over an element (or to x, y), so its hover: styles and tooltips apply, and return the new snapshot. Example: {"ref": "e3"}`,
			act("Hover"), append(elementProps("element"), point...), (*mcpSession).toolHover),
		def("fill", `Replace the text of a text field (h.Input, h.Textarea) as one edit (OnChange fires once) and return the new snapshot; submit: true presses Enter afterwards. Example: {"ref": "e7", "text": "ana@example.com"}`,
			act("Fill a field"), append(elementProps("field"),
				schemaProp{"text", "string", "The new value", nil},
				schemaProp{"submit", "boolean", "Press Enter after filling", nil}), (*mcpSession).toolFill, "text"),
		def("press", `Press a key or a combo, Playwright's syntax ("Enter", "Tab", "Shift+Tab", "Escape", "ArrowDown", "Control+a", "a"), on the focused element, or on an element after focusing it (ref). Returns the new snapshot. Example: {"key": "Tab"}`,
			act("Press a key"), append([]schemaProp{{"key", "string", `Key or combo, such as "Enter" or "Shift+Tab"`, nil}}, elementProps("element to focus first")...), (*mcpSession).toolPress, "key"),
		def("scroll", `Scroll the mouse wheel over an element by dx, dy logical pixels (positive: right, down): the nearest scrollable container under it moves. Name any element inside the area to scroll. Returns the new snapshot. Example: {"ref": "e12", "dy": 400}`,
			act("Scroll"), append(elementProps("element"),
				schemaProp{"dx", "number", "Horizontal distance in logical pixels (positive: right)", nil},
				schemaProp{"dy", "number", "Vertical distance in logical pixels (positive: down)", nil}), (*mcpSession).toolScroll),
		def("drag", `Drag with the primary button: press on from, move in steps, release on to (an element or a point), or by dx, dy from the start. For sliders, reordering, resizable panes. Returns the new snapshot. Example: {"from": {"ref": "e4"}, "dx": 120} or {"from": {"ref": "e4"}, "to": {"ref": "e9"}}`,
			act("Drag"), []schemaProp{
				{"from", "element", "Where the drag starts: {\"ref\": \"e4\"} (or role and name, text, testId), or {\"x\": 10, \"y\": 20}", nil},
				{"to", "element", "Where it ends: an element or a point", nil},
				{"dx", "number", "Without to: horizontal distance from the start", nil},
				{"dy", "number", "Without to: vertical distance from the start", nil},
			}, (*mcpSession).toolDrag, "from"),
		def("resize", `Change the window's logical size and/or display scale, keeping the state: the breakpoint variants (sm: md: lg: max-md:) and the layout follow. Returns the new snapshot. Example: {"width": 390, "height": 844} or {"scale": 2}`,
			act("Resize the window"), []schemaProp{
				{"width", "number", "Logical width in pixels", nil},
				{"height", "number", "Logical height in pixels", nil},
				{"scale", "number", "Display scale, such as 1, 1.5 or 2", nil},
			}, (*mcpSession).toolResize),
		def("open", openDesc,
			act("Open a screen"), []schemaProp{
				{"preview", "string", "Name of a preview of the package's Previews", nil},
				{"dark", "boolean", "The system prefers dark mode (App.ColorScheme still wins)", nil},
				{"width", "number", "Logical width in pixels (with height)", nil},
				{"height", "number", "Logical height in pixels (with width)", nil},
				{"scale", "number", "Display scale", nil},
			}, (*mcpSession).toolOpen),
		def("wait", `Advance the app's clock by ms (timers, UseTimeout, transitions, animations: the clock moves only when asked), and/or wait up to 2 s of real time until a text appears (text) or disappears (textGone), as for data loaded by a goroutine. Returns the new snapshot. Example: {"ms": 500}`,
			act("Wait"), []schemaProp{
				{"ms", "number", "Milliseconds of app time to run", nil},
				{"text", "string", "Wait until the window shows this text", nil},
				{"textGone", "string", "Wait until the window no longer shows this text", nil},
			}, (*mcpSession).toolWait),
		def("inspect", `Show the layout tree as JSON (tela inspect --layout): each box's rect in logical pixels, margin, border, padding, classes, class errors and computed style (display, flex, colors, font). With an element (ref), only its box and what it contains. Use it when something is misplaced, misaligned or has the wrong size. Example: {"ref": "e5"}`,
			ro("Layout tree"), elementProps("element"), (*mcpSession).toolInspect),
		def("lint", `Check the package's Go source for what dev mode reports at run time, without running it: unknown Tailwind classes (with a suggestion), ARIA roles and tokens, duplicate or missing keys, misused hooks. Prints tela lint -json. Example: {}`,
			ro("Lint"), nil, (*mcpSession).toolLint),
	}
}

// ---------------------------------------------------------------------
// The tools.

func (s *mcpSession) toolSnapshot(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	if !s.begin(ctx, &o) {
		return o.result()
	}
	s.send(ctx, &o, mcpReq{Op: "snapshot", Snapshot: true})
	return o.result()
}

func (s *mcpSession) toolScreenshot(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a elementArgs
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	req := mcpReq{Op: "screenshot"}
	desc := "the window"
	if a.given() {
		e, err := s.element(ctx, &o, a)
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		req.Loc, desc = &e.loc, e.desc
	}
	rep, ok := s.send(ctx, &o, req)
	if !ok {
		return o.result()
	}
	w, h := pngSize(rep.PNG)
	o.head = fmt.Sprintf("Screenshot of %s: %dx%d pixels.", desc, w, h)
	if rep.Bounds != nil {
		b := rep.Bounds
		o.head = fmt.Sprintf("Screenshot of %s at (%g, %g), %gx%g logical pixels, with an 8 px margin: %dx%d pixels.", desc, round2(b[0]), round2(b[1]), round2(b[2]), round2(b[3]), w, h)
	}
	o.images = append(o.images, mcpContent{Type: "image", Data: base64.StdEncoding.EncodeToString(rep.PNG), MimeType: "image/png"})
	return o.result()
}

// pngSize reads the size from a PNG header.
func pngSize(b []byte) (w, h int) {
	if len(b) < 24 {
		return 0, 0
	}
	be := func(p []byte) int { return int(p[0])<<24 | int(p[1])<<16 | int(p[2])<<8 | int(p[3]) }
	return be(b[16:20]), be(b[20:24])
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// action runs an action on one element (or a point) and answers with the
// new snapshot.
func (s *mcpSession) action(ctx context.Context, o *mcpOut, req mcpReq, done string) mcpResult {
	req.Snapshot = true
	if _, ok := s.send(ctx, o, req); ok {
		o.head = done
	}
	return o.result()
}

// actionTarget resolves the element of an acting tool, failing for a
// disabled element when enabled is true.
func (s *mcpSession) actionTarget(ctx context.Context, o *mcpOut, a elementArgs, verb string, enabled bool) (*mcpLoc, string, bool) {
	e, err := s.element(ctx, o, a)
	if err != nil {
		o.fail(err.Error())
		return nil, "", false
	}
	if enabled && e.disabled {
		o.fail(fmt.Sprintf("cannot %s %s: it is disabled (the snapshot marks it [disabled])", verb, e.desc))
		return nil, "", false
	}
	return &e.loc, e.desc, true
}

func (s *mcpSession) toolClick(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		elementArgs
		X      *float64 `json:"x"`
		Y      *float64 `json:"y"`
		Button string   `json:"button"`
		Double bool     `json:"double"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	switch a.Button {
	case "", "left", "right":
	default:
		o.fail(fmt.Sprintf(`button %q: use "left" or "right"`, a.Button))
		return o.result()
	}
	if a.Double && a.Button == "right" {
		o.fail(`double is for the left button; a right click opens a context menu once`)
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	req := mcpReq{Op: "click", Button: a.Button, Double: a.Double}
	verb := "Clicked"
	if a.Double {
		verb = "Double-clicked"
	} else if a.Button == "right" {
		verb = "Right-clicked"
	}
	var desc string
	if !a.given() && (a.X != nil || a.Y != nil) {
		if a.X == nil || a.Y == nil {
			o.fail("give both x and y")
			return o.result()
		}
		req.At = &mcpPoint{*a.X, *a.Y}
		desc = fmt.Sprintf("at (%g, %g)", *a.X, *a.Y)
	} else {
		loc, d, ok := s.actionTarget(ctx, &o, a.elementArgs, "click", true)
		if !ok {
			return o.result()
		}
		req.Loc, desc = loc, d
	}
	return s.action(ctx, &o, req, verb+" "+desc+".")
}

func (s *mcpSession) toolHover(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		elementArgs
		X *float64 `json:"x"`
		Y *float64 `json:"y"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	req := mcpReq{Op: "hover"}
	var desc string
	if !a.given() && a.X != nil && a.Y != nil {
		req.At = &mcpPoint{*a.X, *a.Y}
		desc = fmt.Sprintf("(%g, %g)", *a.X, *a.Y)
	} else {
		loc, d, ok := s.actionTarget(ctx, &o, a.elementArgs, "hover", false)
		if !ok {
			return o.result()
		}
		req.Loc, desc = loc, d
	}
	return s.action(ctx, &o, req, "The pointer is over "+desc+".")
}

func (s *mcpSession) toolFill(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		elementArgs
		Value  *string `json:"text"`
		Submit bool    `json:"submit"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.Value == nil {
		o.fail(`fill needs the text: {"ref": "e7", "text": "ana@example.com"}`)
		return o.result()
	}
	// text is the value here, so the field is named by ref, role and name
	// or testId.
	a.Text = ""
	if !s.begin(ctx, &o) {
		return o.result()
	}
	loc, desc, ok := s.actionTarget(ctx, &o, a.elementArgs, "fill", true)
	if !ok {
		return o.result()
	}
	done := fmt.Sprintf("Filled %s with %q.", desc, *a.Value)
	if a.Submit {
		done += " Pressed Enter."
	}
	return s.action(ctx, &o, mcpReq{Op: "fill", Loc: loc, Text: *a.Value, Submit: a.Submit}, done)
}

func (s *mcpSession) toolPress(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		elementArgs
		Key string `json:"key"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if strings.TrimSpace(a.Key) == "" {
		o.fail(`press needs a key: {"key": "Enter"}, {"key": "Shift+Tab"}`)
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	req := mcpReq{Op: "press", Key: a.Key}
	done := fmt.Sprintf("Pressed %s.", a.Key)
	if a.given() {
		loc, desc, ok := s.actionTarget(ctx, &o, a.elementArgs, "focus", false)
		if !ok {
			return o.result()
		}
		req.Loc = loc
		done = fmt.Sprintf("Focused %s and pressed %s.", desc, a.Key)
	}
	return s.action(ctx, &o, req, done)
}

func (s *mcpSession) toolScroll(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		elementArgs
		DX float64 `json:"dx"`
		DY float64 `json:"dy"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.DX == 0 && a.DY == 0 {
		o.fail(`scroll needs a distance: {"ref": "e12", "dy": 400} (positive dy scrolls down)`)
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	loc, desc, ok := s.actionTarget(ctx, &o, a.elementArgs, "scroll", false)
	if !ok {
		return o.result()
	}
	return s.action(ctx, &o, mcpReq{Op: "scroll", Loc: loc, DX: a.DX, DY: a.DY}, fmt.Sprintf("Scrolled by (%g, %g) over %s.", a.DX, a.DY, desc))
}

// dragEnd is the from or to of drag.
type dragEnd struct {
	elementArgs
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
}

func (s *mcpSession) toolDrag(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		From *dragEnd `json:"from"`
		To   *dragEnd `json:"to"`
		DX   float64  `json:"dx"`
		DY   float64  `json:"dy"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.From == nil {
		o.fail(`drag needs from: {"from": {"ref": "e4"}, "dx": 120}`)
		return o.result()
	}
	if a.To == nil && a.DX == 0 && a.DY == 0 {
		o.fail(`drag needs a target: to (an element or a point) or dx, dy`)
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	req := mcpReq{Op: "drag", DX: a.DX, DY: a.DY}
	end := func(e *dragEnd, enabled bool) (loc *mcpLoc, at *mcpPoint, desc string, ok bool) {
		if !e.given() && e.X != nil && e.Y != nil {
			return nil, &mcpPoint{*e.X, *e.Y}, fmt.Sprintf("(%g, %g)", *e.X, *e.Y), true
		}
		l, d, ok := s.actionTarget(ctx, &o, e.elementArgs, "drag", enabled)
		return l, nil, d, ok
	}
	loc, at, fromDesc, ok := end(a.From, true)
	if !ok {
		return o.result()
	}
	req.Loc, req.At = loc, at
	done := "Dragged " + fromDesc
	if a.To != nil {
		toLoc, toAt, toDesc, ok := end(a.To, false)
		if !ok {
			return o.result()
		}
		if (req.At == nil) != (toAt == nil) {
			o.fail("drag from and to must both be elements or both be points (x, y)")
			return o.result()
		}
		req.To, req.ToAt = toLoc, toAt
		done += " to " + toDesc + "."
	} else {
		if req.At != nil {
			req.ToAt = &mcpPoint{req.At.X + a.DX, req.At.Y + a.DY}
		}
		done += fmt.Sprintf(" by (%g, %g).", a.DX, a.DY)
	}
	return s.action(ctx, &o, req, done)
}

func (s *mcpSession) toolResize(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		W     float64 `json:"width"`
		H     float64 `json:"height"`
		Scale float64 `json:"scale"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if err := checkWindow(a.W, a.H, a.Scale, true); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	if _, ok := s.send(ctx, &o, mcpReq{Op: "resize", W: a.W, H: a.H, Scale: a.Scale, Snapshot: true}); ok {
		// A rebuild mounts at the new size too.
		if a.W > 0 {
			s.mount.W, s.mount.H, s.mount.SizeSet = a.W, a.H, true
		}
		if a.Scale > 0 {
			s.mount.Scale = a.Scale
		}
		o.head = "Resized the window; the state is kept."
	}
	return o.result()
}

// checkWindow validates the size and scale arguments of resize and open.
func checkWindow(w, h, scale float64, need bool) error {
	if (w != 0) != (h != 0) {
		return errors.New("give both width and height")
	}
	if need && w == 0 && scale == 0 {
		return errors.New(`resize needs width and height, or scale: {"width": 390, "height": 844}`)
	}
	ro := renderOptions{W: defaultWidth, H: defaultHeight, Scale: 1}
	size := ""
	if w != 0 {
		size = fmt.Sprintf("%gx%g", w, h)
	}
	if scale != 0 {
		ro.Scale = scale
	}
	if err := ro.validate(size); err != nil {
		return errors.New(strings.NewReplacer("--size", "size", "--scale", "scale").Replace(err.Error()))
	}
	return nil
}

func (s *mcpSession) toolOpen(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		Preview *string `json:"preview"`
		Dark    *bool   `json:"dark"`
		W       float64 `json:"width"`
		H       float64 `json:"height"`
		Scale   float64 `json:"scale"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if err := checkWindow(a.W, a.H, a.Scale, false); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	if a.Preview == nil && a.Dark == nil && a.W == 0 && a.Scale == 0 {
		rep, ok := s.send(ctx, &o, mcpReq{Op: "info"})
		if ok {
			if len(rep.Names) > 0 {
				o.head = "The package's previews: " + strings.Join(rep.Names, ", ") + `. Open one with {"preview": "Name"}.`
			} else {
				o.head = "The package defines a single screen (Preview or App), no Previews. open can still switch dark mode or the size: {\"dark\": true}."
			}
		}
		return o.result()
	}
	m := s.mount
	if a.Preview != nil {
		if *a.Preview != "" && s.contract != contractPreviews {
			o.fail(fmt.Sprintf("preview %q: %s", *a.Preview, singleScreen(s.target.pkg.Name, s.contract)))
			return o.result()
		}
		m.Preview = *a.Preview
	}
	if a.Dark != nil {
		m.Dark = *a.Dark
	}
	if a.W != 0 {
		m.W, m.H, m.SizeSet = a.W, a.H, true
	}
	if a.Scale != 0 {
		m.Scale = a.Scale
	}
	old := s.mount
	s.mount = m
	s.refs.remount()
	rep, err := s.child.call(ctx, s.mountReq(true))
	if err != nil {
		s.mount = old
		o.fail(err.Error())
		return o.result()
	}
	if rep.MountError != "" {
		o.absorb(rep)
		s.mount = old
		// Mount the previous screen again, so the other tools keep working.
		back, err := s.child.call(ctx, s.mountReq(false))
		if err == nil {
			o.absorb(back)
		}
		o.fail("cannot open that screen: " + cleanMountError(rep.MountError) + "\n\nThe previous screen was mounted again (state reset).")
		return o.result()
	}
	o.absorb(rep)
	o.screen = describeScreen(rep)
	snap := s.withRefs(ctx, &o, deref(rep.YAML))
	o.snapshot = &snap
	o.head = "Mounted the screen afresh (state reset)."
	return o.result()
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (s *mcpSession) toolWait(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		Ms   float64 `json:"ms"`
		Text string  `json:"text"`
		Gone string  `json:"textGone"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	switch {
	case a.Ms < 0 || a.Ms > 600000:
		o.fail("ms must be between 0 and 600000 (10 minutes of app time)")
		return o.result()
	case a.Ms == 0 && a.Text == "" && a.Gone == "":
		o.fail(`wait needs ms, text or textGone: {"ms": 500}`)
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	var did []string
	if a.Ms > 0 {
		did = append(did, fmt.Sprintf("advanced the clock by %g ms", a.Ms))
	}
	if a.Text != "" {
		did = append(did, fmt.Sprintf("%q is shown", a.Text))
	}
	if a.Gone != "" {
		did = append(did, fmt.Sprintf("%q is gone", a.Gone))
	}
	done := "Waited: " + strings.Join(did, "; ") + "."
	return s.action(ctx, &o, mcpReq{Op: "wait", Ms: a.Ms, Text: a.Text, Gone: a.Gone}, done)
}

func (s *mcpSession) toolInspect(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a elementArgs
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	req := mcpReq{Op: "layout"}
	desc := "the window"
	if a.given() {
		e, err := s.element(ctx, &o, a)
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		req.Loc, desc = &e.loc, e.desc
	}
	rep, ok := s.send(ctx, &o, req)
	if !ok {
		return o.result()
	}
	js := rep.Layout
	o.head = "Layout tree of " + desc + " (rect: border box in logical pixels from the window's top-left corner)."
	if rep.Bounds != nil && req.Loc != nil {
		sub, err := layoutSubtree([]byte(js), *rep.Bounds, req.Loc.Role)
		if err != nil {
			o.fail(fmt.Sprintf("cannot find the box of %s in the layout tree: %v", desc, err))
			return o.result()
		}
		js = string(sub)
	}
	o.sections = append(o.sections, "```json\n"+strings.TrimRight(js, "\n")+"\n```")
	return o.result()
}

func (s *mcpSession) toolLint(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	diags, err := s.lint()
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	var b strings.Builder
	if err := lint.WriteJSON(&b, diags); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	switch len(diags) {
	case 0:
		o.head = "tela lint found nothing."
	case 1:
		o.head = "tela lint found 1 problem:"
	default:
		o.head = fmt.Sprintf("tela lint found %d problems:", len(diags))
	}
	o.sections = append(o.sections, "```json\n"+strings.TrimRight(b.String(), "\n")+"\n```")
	return o.result()
}
