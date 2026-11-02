//go:build darwin

package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
)

// TestExamplesCocoa is phase 14's done criterion: every example runs on
// macOS with snapshots pixel-identical to the headless ones. For each
// directory of examples/ that holds a package main it
//
//  1. builds the example's own package with go build -overlay adding
//     testdata/examplecapture/capture.go to it, whose init follows the
//     window from another goroutine (tela.Post) until its pixels settle,
//     writes them and calls tela.Quit; nothing else in the example
//     changes: its main runs tela.Main on the Cocoa backend;
//  2. runs it with a fresh HOME (and CFFIXED_USER_HOME, for AppKit), so
//     saved settings and data of earlier runs play no part;
//  3. renders the same screen headlessly through telatest, in a test
//     binary of the example's package with a generated file (as tela
//     snapshot does, with its contract code choosing the screen): Mount
//     with FromApp at the window's logical size and scale, in its system
//     settings (appearance, reduced motion, contrast, accent), then Advance
//     by as long as the window was shown, so the reference reaches the
//     steady state the window reached (an overlay scrollbar that faded, a
//     caret that stopped blinking) where tela snapshot's Render draws the
//     first frame of a tree without a clock. The screen is what main shows:
//     App.Root when the package declares `var App` (tela snapshot would
//     render its first preview), else Preview, whose root is main's for the
//     examples that have no App;
//  4. compares the pixels two ways: the window server's capture
//     (CGWindowListCreateImage, what reaches the screen) and the content
//     layer (renderInContext:, what the window holds).
//
// The pass rule is strict: Tela draws every pixel with its own rasterizer
// and embedded fonts, and the cocoa package's TestPresentReadBack shows the
// window server keeps presented pixels exactly, so text antialiasing has no
// reason to differ. The only pixels not compared are
//
//   - the window shape: pixels the window server's capture leaves
//     transparent (its rounded corners), which must lie in the corner bands
//     (cornerBand);
//   - for a TitleBarOverlay window, the rectangle of its standard buttons
//     (read from the window) and one point around it: the OS's traffic
//     lights, where the headless render leaves the room free (the
//     reference's window reports the same rectangle as TitleBarControls,
//     so the app lays its title bar out around the same buttons);
//   - for a Frameless window (its content is the whole frame), the
//     outermost ring of pixels where they stay within a small difference
//     of the reference: the window server's outline of a titled window,
//     which the full-size content view lies under;
//   - for a window that never settled (an endless animation: animate-spin
//     or animate-pulse keep running under reduced motion, which only stops
//     transitions; or a clock of its own, such as the player's, which
//     plays from the start on a goroutine's ticker), the pixels that
//     changed in any read back after the first full one (the window faded
//     in): their value depends on the moment. When they exceed 10% of the
//     window the example fails.
//
// A blinking caret is no exception: the capture waits until it stops,
// visible, 10 s after the focus. The window's size must be the App's, or
// the App's clamped per axis to the screen's visible frame (AppKit keeps a
// window inside it, a few points short of it: the runner's screen has a
// 1024x677 visible frame and gives 1024x642 points of content under a
// 32-point title bar), which is a logged note (axisSize). Everything else is a failure that the log
// describes: the window's size against the App's (a finding, not a mask),
// the differing pixels by channel difference and place, and their regions
// with a sample pixel each. On failure the window's pixels and a
// reference|window|differences panel are written to
// testdata/examples-cocoa (<example>_actual.png, <example>_diff.png; the
// layer's as <example>_layer_*.png).
//
// An example whose screen shows the clock's time differs when the minute
// turns between launching it and rendering the reference: chat writes its
// messages' times (HH:MM) relative to time.Now when it starts, in the
// window and in the reference alike. Such a failure is retried once, within
// one minute (waiting for the next one when the current one has too little
// time left).
//
// It needs a window server session (macos-latest runners have one) and
// skips without it, and in -short mode. It takes about half a minute per
// example.
func TestExamplesCocoa(t *testing.T) {
	skipShort(t)
	if err := darwin.Load(); err != nil {
		t.Skipf("no AppKit: %v", err)
	}
	session := darwin.CGSessionCopyCurrentDictionary()
	if session == 0 {
		t.Skip("no window server session")
	}
	darwin.CFRelease(session)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go command is not in PATH")
	}
	root := repoRoot(t)
	names, err := exampleDirs(filepath.Join(root, "examples"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no examples found")
	}
	capture, err := os.ReadFile(filepath.Join("testdata", "examplecapture", "capture.go"))
	if err != nil {
		t.Fatal(err)
	}
	old, _ := filepath.Glob(filepath.Join(examplesArtifacts, "*.png")) // a previous run's failures
	for _, f := range old {
		os.Remove(f)
	}
	t.Logf("examples: %s", strings.Join(names, ", "))
	var rows []string
	for _, name := range names {
		var row string
		t.Run(name, func(t *testing.T) {
			r := &exampleRun{t: t, goBin: goBin, name: name, capture: capture}
			defer func() { row = r.row() }() // also after a t.Fatal
			r.run()
		})
		rows = append(rows, row)
	}
	t.Logf("summary:\n\t%s", strings.Join(rows, "\n\t"))
}

const (
	// examplesCaptureEnv is capture.go's telaCaptureEnv.
	examplesCaptureEnv = "TELA_EXAMPLE_CAPTURE"
	// examplesArtifacts holds the images of the failures.
	examplesArtifacts = "testdata/examples-cocoa"
	// unstableLimit is the share of the window that may be masked as
	// unstable.
	unstableLimit = 0.10
)

// captureReport is capture.go's telaCaptureReport.
type captureReport struct {
	Error          string   `json:"error"`
	FirstVisibleMS int64    `json:"firstVisibleMs"`
	Captures       int      `json:"captures"`
	Settled        bool     `json:"settled"`
	QuietMS        int64    `json:"quietMs"`
	LastChangeMS   int64    `json:"lastChangeMs"`
	Timeline       []string `json:"timeline"`
	Unstable       int      `json:"unstable"`
	BaseMS         int64    `json:"baseMs"`
	ShownMS        int64    `json:"shownMs"`
	Time           string   `json:"time"`

	Windows []captureWindow `json:"windows"`
	Window  captureWindow   `json:"window"`

	Primary string      `json:"primary"`
	Capture captureRead `json:"capture"`
	Layer   captureRead `json:"layer"`

	Appearance       string `json:"appearance"`
	Dark             bool   `json:"dark"`
	ReduceMotion     bool   `json:"reduceMotion"`
	IncreaseContrast bool   `json:"increaseContrast"`
	Accent           string `json:"accent"`
	Active           bool   `json:"active"`
}

type captureWindow struct {
	Number  int     `json:"number"`
	Title   string  `json:"title"`
	W       float64 `json:"w"`
	H       float64 `json:"h"`
	Scale   float64 `json:"scale"`
	Key     bool    `json:"key"`
	Style   uint64  `json:"styleMask"`
	PhysW   int     `json:"physW"`
	PhysH   int     `json:"physH"`
	FrameW  float64 `json:"frameW"`
	FrameH  float64 `json:"frameH"`
	ScreenW float64 `json:"screenW"`
	ScreenH float64 `json:"screenH"`
	Visible bool    `json:"visible"`
	// Controls is a title bar overlay's standard buttons over the content
	// (x, y, w, h in points, y down), zero for other windows.
	Controls [4]float64 `json:"controls"`
	// Class is the window's Objective-C class: AppKit's own windows (a
	// tray icon's NSStatusBarWindow) are not the app's.
	Class string `json:"class"`
}

// controlsEnv is the reference's exrefControls for a window's controls
// rectangle: "x,y,w,h", or empty for none.
func controlsEnv(c [4]float64) string {
	if c[2] <= 0 || c[3] <= 0 {
		return ""
	}
	return fmt.Sprintf("%g,%g,%g,%g", c[0], c[1], c[2], c[3])
}

// systemWindow reports whether a window is one of AppKit's own, not the
// app's: the status bar window of a tray icon (NSStatusBarWindow) and its
// replicas on other screens (NSStatusItemReplicantWindow and the like).
func (w captureWindow) systemWindow() bool {
	return strings.Contains(w.Class, "StatusBar") || strings.Contains(w.Class, "StatusItem")
}

func (w captureWindow) String() string {
	return fmt.Sprintf("%s %d %q content %gx%g pt at scale %g (%dx%d px), frame %gx%g, screen's visible frame %gx%g, style mask %#x, key %v, visible %v",
		cmp.Or(w.Class, "NSWindow"), w.Number, w.Title, w.W, w.H, w.Scale, w.PhysW, w.PhysH, w.FrameW, w.FrameH, w.ScreenW, w.ScreenH, w.Style, w.Key, w.Visible)
}

type captureRead struct {
	Path string `json:"path"`
	OK   bool   `json:"ok"`
	Note string `json:"note"`
}

// exampleRun is one example's run.
type exampleRun struct {
	t       *testing.T
	goBin   string
	name    string
	capture []byte

	pkg      goPackage
	det      detected
	declared []string
	tmp      string
	app, ref string // the binaries

	// The last attempt's results, for the summary.
	rep        captureReport
	winDiff    *pixelDiff
	layerDiff  *pixelDiff
	exitStatus string
}

func (r *exampleRun) run() {
	t := r.t
	importPath := "github.com/kuronosu/tela/examples/" + r.name
	var err error
	if r.pkg, err = listPackage(r.goBin, importPath); err != nil {
		t.Fatal(err)
	}
	if r.det, err = scanPackage(r.pkg.Dir, r.pkg.GoFiles); err != nil {
		t.Fatal(err)
	}
	if r.declared, err = declaredSizes(r.pkg.Dir, r.pkg.GoFiles); err != nil {
		t.Fatal(err)
	}
	r.tmp = t.TempDir()
	r.build(importPath)

	for attempt := 1; ; attempt++ {
		capDir := r.mkdir(fmt.Sprintf("capture%d", attempt))
		// The span in which the window and the reference read the clock:
		// from launching the example (before its first build, where chat
		// seeds its messages at time.Now) to the reference's end.
		from := time.Now()
		r.runWindow(capDir, r.mkdir(fmt.Sprintf("home%d", attempt)))
		ref := r.runReference(r.mkdir(fmt.Sprintf("refhome%d", attempt)))
		to := time.Now()
		failed := r.compare(capDir, ref, attempt)
		turned := !from.Truncate(time.Minute).Equal(to.Truncate(time.Minute))
		if !failed || attempt == 2 || !turned {
			if failed {
				if turned {
					t.Logf("the minute turned between launching the example (%s) and the reference (%s) again", from.Format(time.TimeOnly), to.Format(time.TimeOnly))
				}
				t.Errorf("the window's pixels differ from the headless reference (see the log above)")
			}
			return
		}
		// Chat writes its messages' times relative to the clock when it
		// starts (HH:MM): a window and a reference that start in different
		// minutes show different times. Run again within one minute: wait
		// for the next one when this one has less time left than the run
		// took, with a margin.
		span := to.Sub(from)
		left := time.Until(to.Truncate(time.Minute).Add(time.Minute))
		t.Logf("the minute turned between launching the example (%s) and the reference (%s), %v apart; this example may show the time (chat writes its messages' times relative to its start)",
			from.Format(time.TimeOnly), to.Format(time.TimeOnly), span.Round(time.Millisecond))
		if left < span+5*time.Second {
			t.Logf("waiting %v for the next minute, to run it again within one", left.Round(time.Millisecond))
			time.Sleep(left + 200*time.Millisecond)
		}
		t.Logf("running it again at %s", time.Now().Format(time.TimeOnly))
	}
}

func (r *exampleRun) mkdir(name string) string {
	d := filepath.Join(r.tmp, name)
	if err := os.Mkdir(d, 0o755); err != nil {
		r.t.Fatal(err)
	}
	return d
}

// build builds the example with capture.go and the reference runner (a
// test binary of the example's package with exampleReferenceSource).
func (r *exampleRun) build(importPath string) {
	t := r.t
	t.Helper()
	capSrc := filepath.Join(r.tmp, "capture.go")
	if err := os.WriteFile(capSrc, r.capture, 0o644); err != nil {
		t.Fatal(err)
	}
	r.app = filepath.Join(r.tmp, r.name)
	start := time.Now()
	ov := r.overlay("capture.json", filepath.Join(r.pkg.Dir, "zz_tela_capture.go"), capSrc)
	r.goRun("go build", "build", "-o", r.app, "-overlay", ov, importPath)
	t.Logf("built %s with capture.go in %v", importPath, time.Since(start).Round(time.Millisecond))

	// The screen main shows: App.Root when the package has an App (the
	// contract code renders it under contractApp), else its Preview.
	c := r.det.contract
	if r.det.hasApp {
		c = contractApp
	}
	runSrc := filepath.Join(r.tmp, generatedFile)
	if err := os.WriteFile(runSrc, exampleReferenceSource(r.pkg.Name, c, r.det.hasApp), 0o644); err != nil {
		t.Fatal(err)
	}
	r.ref = filepath.Join(r.tmp, "reference.test")
	start = time.Now()
	ov = r.overlay("run.json", filepath.Join(r.pkg.Dir, generatedFile), runSrc)
	r.goRun("go test -c", "test", "-c", "-o", r.ref, "-overlay", ov, importPath)
	t.Logf("built the reference runner (contract %v, App %v, declared window sizes %v) in %v", c, r.det.hasApp, r.declared, time.Since(start).Round(time.Millisecond))
}

func (r *exampleRun) overlay(name, file, src string) string {
	js, err := json.Marshal(map[string]map[string]string{"Replace": {file: src}})
	if err != nil {
		r.t.Fatal(err)
	}
	path := filepath.Join(r.tmp, name)
	if err := os.WriteFile(path, js, 0o644); err != nil {
		r.t.Fatal(err)
	}
	return path
}

func (r *exampleRun) goRun(what string, args ...string) {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.goBin, args...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("%s: %v\n%s", what, err, out)
	}
}

// childEnv is the environment of the example and the reference: no TELA_
// variable of the caller (TELA_BACKEND, TELA_DEBUG...) and a fresh home.
func childEnv(home string, extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "TELA_") || k == "HOME" || k == "CFFIXED_USER_HOME" {
			continue
		}
		env = append(env, kv)
	}
	return append(env, append([]string{"HOME=" + home, "CFFIXED_USER_HOME=" + home}, extra...)...)
}

// runWindow runs the example until capture.go quits it and reads the
// report.
func (r *exampleRun) runWindow(capDir, home string) {
	t := r.t
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.app)
	cmd.Dir = home
	cmd.Env = childEnv(home, examplesCaptureEnv+"="+capDir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	err := cmd.Run()
	took := time.Since(start).Round(time.Millisecond)
	r.exitStatus = "0"
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		r.exitStatus = "killed after " + took.String()
		t.Errorf("the example did not exit within %v", took)
	case errors.As(err, &exit):
		r.exitStatus = strconv.Itoa(exit.ExitCode())
		t.Errorf("the example exited with status %d (3: capture.go failed, 4: main did not return after tela.Quit)", exit.ExitCode())
	case err != nil:
		t.Fatalf("running the example: %v", err)
	}
	t.Logf("the example ran for %v, exit status %s", took, r.exitStatus)
	if s := strings.TrimSpace(out.String()); s != "" {
		const limit = 8 << 10
		if len(s) > limit {
			s = s[:limit] + "\n[...]"
		}
		t.Logf("its output:\n%s", s)
	}
	b, err := os.ReadFile(filepath.Join(capDir, "report.json"))
	if err != nil {
		t.Fatalf("no report from capture.go: %v", err)
	}
	r.rep = captureReport{}
	if err := json.Unmarshal(b, &r.rep); err != nil {
		t.Fatalf("report.json: %v\n%s", err, b)
	}
	rep := &r.rep
	if rep.Error != "" {
		t.Fatalf("capture.go: %s", rep.Error)
	}
	t.Logf("system: appearance %s (dark %v), reduce motion %v, increase contrast %v, accent %s, app active %v",
		rep.Appearance, rep.Dark, rep.ReduceMotion, rep.IncreaseContrast, rep.Accent, rep.Active)
	for _, w := range rep.Windows {
		t.Logf("window: %v", w)
	}
	state := fmt.Sprintf("settled: no pixel changed for %dms", rep.QuietMS)
	if !rep.Settled {
		state = fmt.Sprintf("NOT settled: the last change was %dms after it appeared; %d pixels changed after the first full read back (%dms)", rep.LastChangeMS, rep.Unstable, rep.BaseMS)
	}
	t.Logf("the window appeared %dms after start; %d read backs (%s), %s", rep.FirstVisibleMS, rep.Captures, rep.Primary, state)
	t.Logf("changes seen:\n\t%s", strings.Join(rep.Timeline, "\n\t"))
	t.Logf("window server: %s; layer: %s", readNote(rep.Capture), readNote(rep.Layer))
}

func readNote(r captureRead) string {
	if !r.OK {
		return "no pixels (" + r.Note + ")"
	}
	return r.Note
}

// runReference renders the screen headlessly at the window's size, scale
// and appearance.
func (r *exampleRun) runReference(home string) *image.RGBA {
	t := r.t
	t.Helper()
	w := r.rep.Window
	out := filepath.Join(r.tmp, "reference.png")
	advance := time.Duration(r.rep.ShownMS) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.ref, "-test.run", "^"+exampleReferenceTest+"$", "-test.count=1", "-test.v")
	cmd.Dir = r.pkg.Dir // as go test runs it
	cmd.Env = childEnv(home,
		exrefW+"="+strconv.FormatFloat(w.W, 'g', -1, 64),
		exrefH+"="+strconv.FormatFloat(w.H, 'g', -1, 64),
		exrefScale+"="+strconv.FormatFloat(w.Scale, 'g', -1, 64),
		exrefDark+"="+strconv.FormatBool(r.rep.Dark),
		exrefReduced+"="+strconv.FormatBool(r.rep.ReduceMotion),
		exrefContrast+"="+strconv.FormatBool(r.rep.IncreaseContrast),
		exrefAccent+"="+r.rep.Accent,
		exrefAdvance+"="+advance.String(),
		exrefOut+"="+out,
		exrefControls+"="+controlsEnv(w.Controls),
	)
	start := time.Now()
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rendering the reference: %v\n%s", err, b)
	}
	t.Logf("reference: %gx%g at scale %g, dark %v, reduced motion %v, contrast %v, accent %q, clock advanced by %v (as long as the window was shown), rendered in %v",
		w.W, w.H, w.Scale, r.rep.Dark, r.rep.ReduceMotion, r.rep.IncreaseContrast, r.rep.Accent, advance, time.Since(start).Round(time.Millisecond))
	// The reference's own "reference: ..." lines (what it received, its
	// window's state) are logged always; the rest of its output only when
	// there is more than === RUN, --- PASS and PASS.
	var other []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if i := strings.Index(line, "reference: "); i >= 0 {
			t.Logf("the reference process: %s", strings.TrimSpace(line[i+len("reference: "):]))
			continue
		}
		other = append(other, line)
	}
	if s := strings.Join(other, "\n"); len(other) > 3 {
		const limit = 4 << 10
		if len(s) > limit {
			s = s[:limit] + "\n[...]"
		}
		t.Logf("the reference's output:\n%s", s)
	}
	img, err := decodeRGBA(out)
	if err != nil {
		t.Fatalf("the reference: %v", err)
	}
	return img
}

func decodeRGBA(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, nil
	}
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, out.Rect, img, img.Bounds().Min, draw.Src)
	return out, nil
}

// compare checks the window against the declared size and the reference;
// it reports whether the pixels failed (for a retry) and records every
// other failure itself.
func (r *exampleRun) compare(capDir string, ref *image.RGBA, attempt int) (pixelsFailed bool) {
	t := r.t
	t.Helper()
	rep, w := &r.rep, r.rep.Window
	visible := 0
	for _, win := range rep.Windows {
		if win.Visible && !win.systemWindow() {
			visible++
		}
	}
	if visible != 1 {
		t.Errorf("%d visible windows of the app, want 1 (AppKit's status bar windows of a tray icon not counted)", visible)
	}
	switch {
	case len(r.declared) != 1:
		t.Logf("window size: content %gx%g; the package declares %d window sizes (%v): not checked", w.W, w.H, len(r.declared), r.declared)
	default:
		dw, dh, err := parseSize(r.declared[0])
		if err != nil {
			t.Errorf("the declared size %s: %v", r.declared[0], err)
			break
		}
		okW, clampW := axisSize(dw, w.W, w.ScreenW, w.FrameW-w.W)
		okH, clampH := axisSize(dh, w.H, w.ScreenH, w.FrameH-w.H)
		switch {
		case !okW || !okH:
			t.Errorf("the window's content is %gx%g; the App declares %gx%g, the screen's visible frame is %gx%g and the window's frame %gx%g "+
				"(an axis that fits must have the declared size; one that does not must have a frame inside the visible frame and at least %.0f%% of it)",
				w.W, w.H, dw, dh, w.ScreenW, w.ScreenH, w.FrameW, w.FrameH, 100*clampShare)
		case clampW || clampH:
			t.Logf("note: window size: content %gx%g, the App's %gx%g clamped to the screen's visible frame %gx%g (frame %gx%g)",
				w.W, w.H, dw, dh, w.ScreenW, w.ScreenH, w.FrameW, w.FrameH)
		default:
			t.Logf("window size: content %gx%g, as the App declares", w.W, w.H)
		}
	}
	if w.Scale != 1 {
		t.Logf("note: the display's scale is %g; the criterion is stated at scale 1", w.Scale)
	}
	if rep.IncreaseContrast {
		t.Logf("note: the system asks for increased contrast, which the reference does not apply")
	}

	var unstable *image.Gray
	if !rep.Settled {
		img, err := decodeGray(filepath.Join(capDir, "unstable.png"))
		if err != nil {
			t.Errorf("the unstable mask: %v", err)
		} else {
			unstable = img
		}
		if share := float64(rep.Unstable) / float64(max(1, w.PhysW*w.PhysH)); share > unstableLimit {
			t.Errorf("the window never settled: %d pixels (%.1f%%) changed after the first full read back (%dms), more than %.0f%%", rep.Unstable, 100*share, rep.BaseMS, 100*unstableLimit)
		}
	}
	isUnstable := func(x, y int) bool {
		return unstable != nil && image.Pt(x, y).In(unstable.Rect) && unstable.GrayAt(x, y).Y != 0
	}
	const shapeReason, unstableReason = "window shape (transparent in the capture)", "unstable (changed after the window appeared)"
	// A window whose content is its whole frame (a Frameless window: a
	// titled window with a full-size content view) has the window
	// server's one-point outline over its outermost pixels, a native
	// border the headless reference cannot have. Those pixels, a ring of
	// round(scale) pixels and nothing inside it, are not compared while
	// they are within borderTolerance of the reference (the outline is a
	// faint line); one that differs more still counts. A titled window's
	// capture is cropped to its content and keeps the strict rule.
	const borderReason = "window border (the window server's outline on a full-size content window's outermost pixels)"
	const borderTolerance = 24
	fullSize := w.FrameW == w.W && w.FrameH == w.H
	ring := max(1, int(math.Round(w.Scale)))
	onBorder := func(x, y int, c color.RGBA) bool {
		size := ref.Rect.Size()
		if !fullSize || (x >= ring && y >= ring && x < size.X-ring && y < size.Y-ring) {
			return false
		}
		return channelDiff(c, ref.RGBAAt(ref.Rect.Min.X+x, ref.Rect.Min.Y+y)) <= borderTolerance
	}
	// A TitleBarOverlay window shows the OS's traffic lights over the
	// content where the headless render leaves the room free
	// (TitleBarControls): the capture's pixels in the buttons' rectangle,
	// as the window reports it, and one point around it (antialiasing,
	// the buttons' shadow) are not compared. Other windows report none.
	const controlsReason = "window buttons (the OS's traffic lights over a title bar overlay)"
	var controls image.Rectangle
	if cr := w.Controls; cr[2] > 0 && cr[3] > 0 {
		s := w.Scale
		controls = image.Rect(int(math.Floor((cr[0]-1)*s)), int(math.Floor((cr[1]-1)*s)),
			int(math.Ceil((cr[0]+cr[2]+1)*s)), int(math.Ceil((cr[1]+cr[3]+1)*s)))
		t.Logf("title bar overlay: the buttons cover %v points, %v pixels with the margin, not compared in the capture", cr, controls)
	}
	windowMask := func(x, y int, c color.RGBA) string {
		switch {
		case c.A != 0xff:
			return shapeReason
		case image.Pt(x, y).In(controls):
			return controlsReason
		case onBorder(x, y, c):
			return borderReason
		case isUnstable(x, y):
			return unstableReason
		}
		return ""
	}
	layerMask := func(x, y int, _ color.RGBA) string {
		if isUnstable(x, y) {
			return unstableReason
		}
		return ""
	}

	r.winDiff, r.layerDiff = nil, nil
	for _, c := range []struct {
		label, file string
		read        captureRead
		mask        func(x, y int, c color.RGBA) string
		dst         **pixelDiff
		artifact    string
	}{
		{"window server (CGWindowListCreateImage)", "window.png", rep.Capture, windowMask, &r.winDiff, r.name},
		{"layer (renderInContext:)", "layer.png", rep.Layer, layerMask, &r.layerDiff, r.name + "_layer"},
	} {
		if !c.read.OK {
			continue
		}
		img, err := decodeRGBA(filepath.Join(capDir, c.file))
		if err != nil {
			t.Errorf("%s: %v", c.label, err)
			continue
		}
		d := comparePixels(img, ref, c.mask)
		*c.dst = &d
		t.Logf("%s against the reference: %s", c.label, d.summary())
		for _, line := range d.details(12) {
			t.Logf("  %s", line)
		}
		for _, m := range d.masked {
			if m.reason == shapeReason && m.outside > 0 {
				t.Errorf("%s: %d transparent pixels outside the corners (%v): the capture's shape is not the window's rounded corners", c.label, m.outside, m.box)
				pixelsFailed = true
			}
		}
		if !d.ok() {
			pixelsFailed = true
			r.artifacts(c.artifact, attempt, img, ref, c.mask)
		}
	}
	switch {
	case r.winDiff == nil && r.layerDiff == nil:
		t.Fatalf("neither read back has pixels")
	case r.winDiff == nil:
		t.Logf("note: the window server's capture was refused; only the layer was compared")
	}
	return pixelsFailed
}

func decodeGray(path string) (*image.Gray, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	g, ok := img.(*image.Gray)
	if !ok {
		return nil, fmt.Errorf("%s is a %T, want *image.Gray", path, img)
	}
	return g, nil
}

// artifacts writes the window's pixels and the panel of differences for a
// failure.
func (r *exampleRun) artifacts(base string, attempt int, got, want *image.RGBA, mask func(x, y int, c color.RGBA) string) {
	t := r.t
	if attempt > 1 {
		base += fmt.Sprintf("_attempt%d", attempt)
	}
	if err := os.MkdirAll(examplesArtifacts, 0o755); err != nil {
		t.Logf("artifacts: %v", err)
		return
	}
	for _, f := range []struct {
		name string
		img  image.Image
	}{{base + "_actual.png", got}, {base + "_diff.png", diffPanels(got, want, mask)}} {
		var buf bytes.Buffer
		if err := png.Encode(&buf, f.img); err != nil {
			t.Logf("artifacts: %v", err)
			return
		}
		path := filepath.Join(examplesArtifacts, f.name)
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Logf("artifacts: %v", err)
			return
		}
		abs, _ := filepath.Abs(path)
		t.Logf("wrote %s", abs)
	}
}

// row is the example's line of the summary.
func (r *exampleRun) row() string {
	w := r.rep.Window
	verdict := "PASS"
	if r.t.Failed() {
		verdict = "FAIL"
	}
	settled := "settled"
	if !r.rep.Settled {
		settled = fmt.Sprintf("unsettled (%d px masked)", r.rep.Unstable)
	}
	diff := func(d *pixelDiff) string {
		switch {
		case d == nil:
			return "-"
		case d.sizeErr != "":
			return "size differs"
		case d.differing == 0:
			return "identical"
		}
		return fmt.Sprintf("%d px differ (max %d)", d.differing, d.maxDiff)
	}
	return fmt.Sprintf("%-10s %s: %gx%g@%g (declared %s), %s, exit %s; window server: %s; layer: %s",
		r.name, verdict, w.W, w.H, w.Scale, strings.Join(r.declared, ","), settled, r.exitStatus, diff(r.winDiff), diff(r.layerDiff))
}
