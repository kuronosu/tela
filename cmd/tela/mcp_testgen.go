package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// The test tool of tela mcp: the acting calls of the session, written as a
// telatest test in the package, checked with go vet and run once. The
// calls come from the recording (mcp_record.go); this file writes the Go.

// testgenMarker begins a file the test tool wrote, so that the tool
// replaces its own files and refuses to overwrite one a person wrote.
const testgenMarker = `// Written by "tela mcp" from a session on this screen`

// testgenInput is what a test is written from.
type testgenInput struct {
	Pkg      string // the package's name
	Func     string // the test function, TestXxx
	Segments []*mcpSegment
	Contract contract
	HasApp   bool // the package declares an App whose settings the mount applies
	AppPtr   bool // the App is a *tela.App
}

// testFuncName turns the name the agent gave into a test function name:
// "checkout flow" and "TestCheckoutFlow" are both TestCheckoutFlow.
func testFuncName(name string) (string, error) {
	name = strings.TrimSpace(name)
	// Only the prefix "Test" of a function name: not the start of a word
	// such as "Testing flow".
	if rest, ok := strings.CutPrefix(name, "Test"); ok {
		if r, _ := utf8.DecodeRuneInString(rest); rest == "" || unicode.IsUpper(r) || r == '_' || r == ' ' || r == '-' {
			name = rest
		}
	}
	var b strings.Builder
	upper := true
	for _, r := range name {
		switch {
		case r == ' ' || r == '-' || r == '_' || r == '.':
			upper = true
			continue
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			return "", fmt.Errorf("the test name %q has %q in it: use letters, digits and spaces, such as \"checkout flow\"", name, r)
		}
		if upper {
			r = unicode.ToUpper(r)
			upper = false
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "", errors.New(`test needs a name: {"name": "checkout flow"}`)
	}
	if r, _ := utf8.DecodeRuneInString(b.String()); unicode.IsDigit(r) {
		return "", fmt.Errorf("the test name %q starts with a digit: Go test functions are named Test followed by a letter or underscore, such as \"flow %s\"", name, name)
	}
	return "Test" + b.String(), nil
}

// testFileName is the default file of a test function: TestCheckoutFlow
// is checkout_flow_test.go.
func testFileName(fn string) string {
	var b strings.Builder
	for i, r := range strings.TrimPrefix(fn, "Test") {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String() + "_test.go"
}

// generateTestSource writes the test for the segments in.
func generateTestSource(in testgenInput) ([]byte, error) {
	var body []string
	checks, steps := 0, 0
	usesTime, usesTela := false, false
	first := true
	for _, seg := range in.Segments {
		if len(seg.steps) == 0 {
			continue
		}
		mount, tela, err := in.mountLines(seg, first)
		if err != nil {
			return nil, err
		}
		usesTela = usesTela || tela
		body = append(body, mount...)
		first = false
		for _, st := range seg.steps {
			lines, t, tl := stepLines(st, seg)
			usesTime, usesTela = usesTime || t, usesTela || tl
			body = append(body, lines...)
			steps++
			ex := expectations(st.before, st.after)
			for _, l := range ex {
				if !strings.HasPrefix(l, "//") {
					checks++
				}
			}
			body = append(body, ex...)
		}
	}
	if steps == 0 {
		return nil, errors.New("nothing to write: the session has no acting call since the screen was mounted")
	}
	var b strings.Builder
	b.WriteString(testgenMarker + ": each action is an action the agent took,\n")
	b.WriteString("// and each check is what the screen showed after it. Edit it freely; the test tool\n")
	b.WriteString("// replaces the file only while it still begins with this comment.\n\n")
	fmt.Fprintf(&b, "package %s\n\nimport (\n\t\"testing\"\n", in.Pkg)
	if usesTime {
		b.WriteString("\t\"time\"\n")
	}
	b.WriteString("\n")
	if usesTela {
		b.WriteString("\t\"github.com/kuronosu/tela\"\n")
	}
	b.WriteString("\t\"github.com/kuronosu/tela/telatest\"\n)\n\n")
	fmt.Fprintf(&b, "// %s replays a session of tela mcp on the screen of the package: %s, %s.\n", in.Func, plural(steps, "action"), plural(checks, "check"))
	fmt.Fprintf(&b, "func %s(t *testing.T) {\n", in.Func)
	for _, l := range body {
		b.WriteString(l + "\n")
	}
	b.WriteString("}\n")
	out, err := format.Source([]byte(b.String()))
	if err != nil {
		return nil, fmt.Errorf("the generated test does not parse (a bug of tela mcp): %v\n%s", err, b.String())
	}
	return out, nil
}

// mountLines are the lines that mount the screen of seg: app := or app =
// telatest.Mount(t, root, options...). tela says whether they use the tela
// package.
func (in testgenInput) mountLines(seg *mcpSegment, first bool) (lines []string, tela bool, err error) {
	var root string
	switch in.Contract {
	case contractPreview:
		root, tela = "tela.C(Preview)", true
	case contractPreviews:
		if seg.preview == "" {
			return nil, false, errors.New("the session does not say which preview it drove")
		}
		root = "Previews[" + strconv.Quote(seg.preview) + "]"
	case contractApp:
		root = "App.Root"
	}
	m := seg.mount
	opts := []string{fmt.Sprintf("telatest.Size(%g, %g)", m.W, m.H)}
	if m.Scale != 1 {
		opts = append(opts, fmt.Sprintf("telatest.Scale(%g)", m.Scale))
	}
	if m.Dark {
		opts = append(opts, "telatest.Dark()")
	}
	if m.Prefer.ReducedMotion {
		opts = append(opts, "telatest.ReducedMotion()")
	}
	if m.Prefer.HighContrast {
		opts = append(opts, "telatest.HighContrast()")
	}
	if sc := textScaleOf(m.Prefer); sc != 1 {
		opts = append(opts, fmt.Sprintf("telatest.TextScale(%g)", sc))
	}
	if in.HasApp {
		app := "App"
		if in.AppPtr {
			app = "*App"
		}
		opts = append(opts, "telatest.FromApp("+app+")")
		if m.SizeSet {
			opts = append(opts, fmt.Sprintf("telatest.Size(%g, %g)", m.W, m.H))
		}
	}
	if m.Prefer.Direction != "" {
		lines = append(lines, fmt.Sprintf("// the session mounted the screen with the direction %q; the test does not reproduce it", m.Prefer.Direction))
	}
	if m.Prefer.LocaleSet {
		lines = append(lines, fmt.Sprintf("// the session set the locale to %q; the test does not reproduce it", m.Prefer.Locale))
	}
	op := ":="
	if !first {
		op = "="
	}
	call := fmt.Sprintf("app %s telatest.Mount(t, %s, %s)", op, root, strings.Join(opts, ", "))
	if len(opts) > 2 {
		call = fmt.Sprintf("app %s telatest.Mount(t, %s,\n%s\n)", op, root, "\t"+strings.Join(opts, ",\n\t")+",")
	}
	return append(lines, call), tela, nil
}

// plural writes a count and its noun.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// fnum writes a number as a Go constant.
func fnum(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

// stepLines writes the action of a step, and says whether it uses the time
// and tela packages.
func stepLines(st mcpStep, seg *mcpSegment) (lines []string, usesTime, usesTela bool) {
	if len(st.lines) > 0 {
		return st.lines, false, false
	}
	r := st.req
	loc := func(l *mcpLoc) string { return l.goExpr() }
	switch r.Op {
	case "click":
		right := ""
		if r.Button == "right" {
			right = ", telatest.Right()"
		}
		switch {
		case r.Loc != nil && r.Double:
			lines = append(lines, fmt.Sprintf("app.DoubleClick(%s)", loc(r.Loc)))
		case r.Loc != nil:
			lines = append(lines, fmt.Sprintf("app.Click(%s%s)", loc(r.Loc), right))
		case r.At != nil:
			lines = append(lines, fmt.Sprintf("app.ClickAt(%s, %s%s)", fnum(r.At.X), fnum(r.At.Y), right))
			if r.Double {
				lines = append(lines, "app.MouseDown()", "app.MouseUp()")
			}
		}
	case "hover":
		if r.At != nil {
			lines = append(lines, fmt.Sprintf("app.MouseMove(%s, %s)", fnum(r.At.X), fnum(r.At.Y)))
		} else if r.Loc != nil {
			lines = append(lines, fmt.Sprintf("app.Hover(%s)", loc(r.Loc)))
		}
	case "fill":
		lines = append(lines, fmt.Sprintf("app.Fill(%s, %s)", loc(r.Loc), strconv.Quote(r.Text)))
		if r.Submit {
			lines = append(lines, `app.Press("Enter")`)
		}
	case "press":
		if r.Loc != nil {
			lines = append(lines, fmt.Sprintf("app.Focus(%s)", loc(r.Loc)))
		}
		lines = append(lines, fmt.Sprintf("app.Press(%s)", strconv.Quote(r.Key)))
	case "scroll":
		if r.Loc != nil {
			lines = append(lines, fmt.Sprintf("app.Scroll(%s, %s, %s)", loc(r.Loc), fnum(r.DX), fnum(r.DY)))
			break
		}
		x, y := st.w/2, st.h/2
		if r.At != nil {
			x, y = r.At.X, r.At.Y
		}
		lines = append(lines, fmt.Sprintf("app.ScrollAt(%s, %s, %s, %s)", fnum(x), fnum(y), fnum(r.DX), fnum(r.DY)))
	case "drag":
		switch {
		case r.At != nil && r.ToAt != nil:
			usesTela = true
			lines = append(lines, fmt.Sprintf("app.DragFromTo(tela.Point{X: %s, Y: %s}, tela.Point{X: %s, Y: %s})", fnum(r.At.X), fnum(r.At.Y), fnum(r.ToAt.X), fnum(r.ToAt.Y)))
		case r.Loc != nil && r.To != nil:
			lines = append(lines, fmt.Sprintf("app.Drag(%s, %s)", loc(r.Loc), loc(r.To)))
		case r.Loc != nil:
			lines = append(lines, fmt.Sprintf("app.DragBy(%s, %s, %s)", loc(r.Loc), fnum(r.DX), fnum(r.DY)))
		}
	case "wait":
		if r.Ms > 0 {
			usesTime = true
			lines = append(lines, fmt.Sprintf("app.Advance(%s * time.Millisecond)", fnum(r.Ms)))
		}
		if r.Text != "" {
			lines = append(lines, fmt.Sprintf("app.ExpectText(%s)", strconv.Quote(r.Text)))
		}
		if r.Gone != "" {
			lines = append(lines, fmt.Sprintf("app.ExpectNoText(%s)", strconv.Quote(r.Gone)))
		}
	case "resize":
		if r.W > 0 {
			lines = append(lines, fmt.Sprintf("app.Resize(%s, %s)", fnum(r.W), fnum(r.H)))
		}
		if r.Scale > 0 {
			lines = append(lines, fmt.Sprintf("app.SetScale(%s)", fnum(r.Scale)))
		}
	}
	return lines, usesTime, usesTela
}

// ---------------------------------------------------------------------
// The tool.

// goTool runs the go command in the module of the package and returns what
// it printed.
func (s *mcpSession) goTool(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.goBin, args...)
	cmd.Dir = s.target.modDir
	cmd.Env = goEnv()
	cmd.Cancel = func() error { return interruptProcess(cmd.Process) }
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimRight(out.String(), "\n"), err
}

// tail keeps the end of long output.
func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

func (s *mcpSession) toolTest(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	fn, err := testFuncName(a.Name)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	pkgDir := filepath.Clean(s.target.pkg.Dir)
	path := a.Path
	if path == "" {
		path = testFileName(fn)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(pkgDir, path)
	}
	path = filepath.Clean(path)
	switch {
	case !strings.HasSuffix(path, "_test.go"):
		o.fail(fmt.Sprintf("path %s: a Go test file ends with _test.go", path))
		return o.result()
	case filepath.Dir(path) != pkgDir:
		o.fail(fmt.Sprintf("path %s: the test must be in the package's directory, %s (a relative path is relative to it)", path, pkgDir))
		return o.result()
	}
	// A file of the tool's is replaced, and kept to put back when the new
	// test does not vet. Lstat, so that a symlink (to a file outside the
	// package, say) is refused and not written through.
	var prev []byte
	var prevMode os.FileMode
	switch fi, err := os.Lstat(path); {
	case err == nil && !fi.Mode().IsRegular():
		o.fail(fmt.Sprintf("%s exists and is not a regular file (a symlink or a directory): give another path, or remove it", path))
		return o.result()
	case err == nil:
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			o.fail(rerr.Error())
			return o.result()
		}
		if !strings.HasPrefix(string(b), testgenMarker) {
			o.fail(fmt.Sprintf("%s exists and was not written by this tool: give another path, or remove it", path))
			return o.result()
		}
		prev, prevMode = b, fi.Mode().Perm()
	case !errors.Is(err, fs.ErrNotExist):
		o.fail(err.Error())
		return o.result()
	}
	if s.rec.interrupted() {
		o.fail("nothing to write: an action was cancelled while it ran, so part of its effect may be on the screen and the recording after it would not match. Call open to mount the screen afresh (state reset) and act again, then call test")
		return o.result()
	}
	if s.rec.count() == 0 {
		o.fail("nothing to write: no acting call (click, fill, press, hover, scroll, drag, wait, resize, prefer) has been made since the screen was mounted or the source was last rebuilt. Act on the screen, then call test")
		return o.result()
	}
	in := testgenInput{Pkg: s.target.pkg.Name, Func: fn, Segments: s.rec.segs, Contract: s.contract, HasApp: s.hasApp}
	// The test file is part of the package's fingerprint, but not of the
	// running instance: writing it must not rebuild and remount the screen.
	// So the session adopts the fingerprint right after each change it makes
	// to the file, and only when nothing else changed before it: a source
	// edit made while go vet or go test run still rebuilds on the next call.
	touch := func(change func() error) error {
		before := s.target.fingerprint()
		synced := s.built && s.builtFP == before
		err := change()
		if synced {
			s.builtFP = s.target.fingerprint()
		}
		return err
	}
	write := func() ([]byte, error) {
		src, err := generateTestSource(in)
		if err != nil {
			return nil, err
		}
		return src, touch(func() error { return os.WriteFile(path, src, 0o644) })
	}
	src, err := write()
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	arg := s.target.arg
	vet, verr := s.goTool(ctx, "vet", arg)
	if verr != nil && !in.AppPtr && strings.Contains(vet, "FromApp") && strings.Contains(vet, "*tela.App") {
		// The package's App is a pointer.
		in.AppPtr = true
		if src, err = write(); err != nil {
			o.fail(err.Error())
			return o.result()
		}
		vet, verr = s.goTool(ctx, "vet", arg)
	}
	if verr != nil {
		if ctx.Err() != nil {
			o.fail("cancelled")
			return o.result()
		}
		if strings.Contains(vet, filepath.Base(path)) {
			kept := "it was not kept"
			if prev != nil {
				touch(func() error { return os.WriteFile(path, prev, prevMode) })
				kept = "the file it would have replaced was put back"
			} else {
				touch(func() error { return os.Remove(path) })
			}
			o.fail(fmt.Sprintf("the test does not compile or vet (go vet %s); %s. This is what it said, and what was written:\n\n%s\n\n```go\n%s\n```", arg, kept, tail(vet, 4000), src))
			return o.result()
		}
		o.notes = append(o.notes, "go vet reports problems in the package that are not in the test:\n"+tail(vet, 2000))
	}
	run := "^" + fn + "$"
	out, terr := s.goTool(ctx, "test", "-count=1", "-run", run, arg)
	if terr != nil {
		if ctx.Err() != nil {
			o.fail("cancelled")
			return o.result()
		}
		o.fail(fmt.Sprintf("wrote %s, but the test fails (go test -run %s %s):\n\n%s\n\nThe file is kept; the screen may have changed since the session, or an action depends on the clock. Edit it, or call test again after acting afresh.\n\n```go\n%s\n```", path, run, arg, tail(out, 4000), src))
		return o.result()
	}
	o.head = fmt.Sprintf("Wrote %s: %s passes (go vet %s and go test -run %s are green).", path, fn, arg, run)
	o.sections = append(o.sections, "```go\n"+string(src)+"```")
	return o.result()
}
