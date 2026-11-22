package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// film and test over the real windows of tela mcp --attach. film takes the
// frames the window presents in real time, after an action or not; test
// writes the acting calls of the session on the app's first window as a
// telatest test of the package tela dev runs, which mounts the screen
// headlessly. The attached server lists them after its own tools, in the
// order of the headless list.

const (
	attachFilmDesc = `See motion on the real window: take the frame the window presents every few milliseconds of real time over a span, as a contact sheet (a grid of the frames, each captioned with the time it was taken), so a transition, a spring, a Presence exit or a spinner can be judged. With action, the film starts at that action ({"click": "e5"}, {"hover": "e3"}, {"press": "Escape"}, {"fill": {"ref": "e7", "text": "ana"}}, scroll, drag: the arguments of those tools); without it, the window as it is. The clock is the wall's: a frame is taken when its time comes and the window answers, so the times are close to, not exactly, the ones asked, and the motion depends on this machine's speed (the headless server films on the app's own clock). Returns the sheet as an image and, after an action, its result; apng also writes every frame as an animated PNG to that path, in the module of tela dev's package. At most 60 frames (span / every) and 30000 ms. Example: {"span": 600, "every": 50, "action": {"click": "e5"}}`
	attachTestDesc = `Write the acting calls of this session on the app's first window (click, fill, press, hover, scroll, drag, wait, film actions) as a Go telatest test in the package tela dev runs, then run go vet and the test and report the result. The test mounts the screen headlessly (the package's App, or its Preview or preview), at the window's size and scale, as the person's app was when the recording started: the recording starts when tela dev builds the app, so act from the state of a fresh start for a test that passes. Refs become role and name (with Nth when several elements look alike), waits become app.Advance, and each action is followed by checks of what changed in the snapshot. A file this tool wrote is replaced; any other is refused. Example: {"name": "checkout flow"}`
)

func init() {
	for _, t := range []struct {
		name, desc string
		props      []schemaProp
		required   string
		run        func(*attachSession, context.Context, json.RawMessage) mcpResult
	}{
		{"film", attachFilmDesc, filmProps, "span", (*attachSession).toolFilm},
		{"test", attachTestDesc, testProps, "name", (*attachSession).toolTest},
	} {
		props := append(slices.Clone(t.props), windowProp)
		attachDefs = append(attachDefs, attachToolDef{
			mcpToolDef: mcpToolDef{tool: mcpTool{Name: t.name, Description: t.desc, InputSchema: schemaOf(props, t.required), Annotations: &mcpToolAnnotate{Title: t.name}}, props: propNames(props)},
			run:        t.run,
		})
	}
}

// attachFilm is a film the film tool asks of the acting tool it runs: act
// takes the frames right after the tool's input (filmWindow).
type attachFilm struct {
	span, every time.Duration
	frames      []filmFrame
	err         error
	ran         bool
}

// filmWindow takes the frame window w presents at 0 and every every of
// real time up to span, the last at span: as close to those times as the
// window answers, each captioned with the time it was taken.
func (s *attachSession) filmWindow(ctx context.Context, w toolsWindow, span, every time.Duration) ([]filmFrame, error) {
	t0 := time.Now()
	var frames []filmFrame
	for at := time.Duration(0); ; at += every {
		at = min(at, span)
		if d := time.Until(t0.Add(at)); d > 0 {
			select {
			case <-ctx.Done():
				return frames, ctx.Err()
			case <-time.After(d):
			}
		}
		img, err := s.conn.Screenshot(ctx, w.ID)
		if err != nil {
			return frames, err
		}
		taken := time.Since(t0)
		if n := len(frames); n > 0 && taken <= frames[n-1].At {
			taken = frames[n-1].At + time.Millisecond // the animated PNG needs times that move on
		}
		frames = append(frames, filmFrame{At: taken, Img: toRGBA(img)})
		if at >= span {
			return frames, nil
		}
	}
}

// moduleDir returns the directory of the module that has dir: the first
// directory up from it with a go.mod.
func moduleDir(dir string) (string, bool) {
	for d := filepath.Clean(dir); ; {
		if fi, err := os.Stat(filepath.Join(d, "go.mod")); err == nil && fi.Mode().IsRegular() {
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

func (s *attachSession) toolFilm(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
		Span   float64                    `json:"span"`
		Every  float64                    `json:"every"`
		Action map[string]json.RawMessage `json:"action"`
		APNG   string                     `json:"apng"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.Span <= 0 {
		o.fail(`film needs span, the milliseconds of real time to film: {"span": 600, "action": {"click": "e5"}}`)
		return o.result()
	}
	span, every := time.Duration(a.Span*float64(time.Millisecond)), time.Duration(a.Every*float64(time.Millisecond))
	every, _, err := checkFilm(span, every)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	var tool string
	var toolArgs json.RawMessage
	if len(a.Action) > 0 {
		if tool, toolArgs, err = filmAction(a.Action); err != nil {
			o.fail(err.Error())
			return o.result()
		}
		if a.Window != 0 {
			// The film's window is the action's, unless it names one.
			var m map[string]json.RawMessage
			if json.Unmarshal(toolArgs, &m) == nil && m != nil && m["window"] == nil {
				m["window"], _ = json.Marshal(a.Window)
				toolArgs, _ = json.Marshal(m)
			}
		}
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	apng := ""
	if a.APNG != "" {
		mod, ok := "", false
		if s.info.Dir != "" {
			mod, ok = moduleDir(s.info.Dir)
		}
		if !ok {
			o.fail("apng: the module of tela dev's package is not known (a tela dev of an older Tela does not say where its package is); film without apng")
			return o.result()
		}
		if apng, err = filmPath(mod, a.APNG); err == nil {
			err = checkAPNGTarget(apng)
		}
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
	}
	var film []filmFrame
	var acted *mcpResult
	desc := "the window as it is"
	if tool != "" {
		var def *attachToolDef
		for i := range attachDefs {
			if attachDefs[i].tool.Name == tool {
				def = &attachDefs[i]
			}
		}
		if def == nil {
			o.fail("action " + tool + ": no such tool")
			return o.result()
		}
		if err := checkArgNames(&def.mcpToolDef, toolArgs); err != nil {
			o.fail(err.Error())
			return o.result()
		}
		f := &attachFilm{span: span, every: every}
		s.filming = f
		res := func() mcpResult {
			defer func() { s.filming = nil }()
			return def.run(s, ctx, toolArgs)
		}()
		if res.IsError {
			return res
		}
		if !f.ran {
			o.fail("the action ran no input to film")
			return o.result()
		}
		if f.err != nil {
			o.notes = append(o.notes, "The film stopped early: "+f.err.Error())
		}
		film, acted, desc = f.frames, &res, "after "+tool
	} else {
		w, err := s.window(ctx, a.Window)
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		o.screen = describeWindow(w)
		if film, err = s.filmWindow(ctx, w, span, every); err != nil {
			o.fail(err.Error())
			return o.result()
		}
	}
	if len(film) == 0 {
		o.fail("the window gave no frame")
		return o.result()
	}
	sheet := contactSheet(film)
	data, err := encodePNG(sheet)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	sb := sheet.Bounds()
	o.head = fmt.Sprintf("Filmed %d frames over %d ms of real time (%s), each captioned with the time it was taken: the window answers each screenshot in its own time, so they are near the %d ms asked between them. The contact sheet shows %d of them, %dx%d pixels.",
		len(film), film[len(film)-1].At.Milliseconds(), desc, every.Milliseconds(), min(len(film), filmSheetFrames), sb.Dx(), sb.Dy())
	o.images = append(o.images, mcpContent{Type: "image", Data: base64.StdEncoding.EncodeToString(data), MimeType: "image/png"})
	if apng != "" {
		out, err := encodeAPNG(film)
		if err == nil {
			err = writeFileAtomic(apng, out)
		}
		if err != nil {
			o.notes = append(o.notes, "The animated PNG was not written: "+err.Error())
		} else {
			o.head += fmt.Sprintf("\nWrote the animated PNG, all %d frames, to %s (%d KB).", len(film), apng, (len(out)+1023)/1024)
		}
	}
	if acted != nil {
		var texts []string
		for _, c := range acted.Content {
			if c.Type == "text" {
				texts = append(texts, c.Text)
			}
		}
		o.sections = append(o.sections, "### The action\n"+strings.Join(texts, "\n"))
	} else {
		s.collectLogs(ctx, &o)
	}
	return o.result()
}

func (s *attachSession) toolTest(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
		testArgs
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
	if s.info.Dir == "" {
		o.fail("tela dev does not say where its package is (a tela dev of an older Tela): update tela, or use the headless server (tela mcp ./pkg)")
		return o.result()
	}
	if s.rec.interrupted() {
		o.fail("nothing to write: an action failed part-way, or acted on another window than the app's first, so a test of the app's screen cannot follow the recording. Save a source file (tela dev builds the app again and the recording starts over) and act again, then call test")
		return o.result()
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		o.fail("the go command is not on the PATH: " + err.Error())
		return o.result()
	}
	pkg, err := listPackageIn(goBin, s.info.Dir, ".")
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	det, err := scanPackage(pkg.Dir, pkg.GoFiles)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	mod, ok := moduleDir(pkg.Dir)
	if !ok {
		o.fail(pkg.Dir + " is not in a module (no go.mod above it)")
		return o.result()
	}
	c, preview, err := attachContract(pkg.Name, det, s.info.Preview)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	for _, seg := range s.rec.segs {
		if seg.preview == "" {
			seg.preview = preview
		}
	}
	arg := "."
	if rel, err := filepath.Rel(mod, pkg.Dir); err == nil && rel != "." {
		arg = "./" + filepath.ToSlash(rel)
	}
	st := sessionTest{goBin: goBin, modDir: mod, pkgDir: pkg.Dir, pkgName: pkg.Name, arg: arg, rec: &s.rec, contract: c, hasApp: det.hasApp}
	return writeSessionTest(ctx, &o, st, fn, a.Path)
}

// attachContract is the screen of the package tela dev runs, as a test
// mounts it: what tela dev runs (a package main runs its own main, which
// a test can mount when it shows the package's var App; another package,
// or --preview, runs its Preview, its Previews' preview, or its App's
// Root), and for Previews the preview's name (tela dev's --preview, or the
// first name in byte order, as tela dev picks it).
func attachContract(pkgName string, det detected, preview string) (contract, string, error) {
	if pkgName == "main" && preview == "" {
		if !det.hasApp {
			return contractNone, "", errors.New("tela dev runs this package's main, whose screen a test cannot mount: declare the app as a package-level var App = tela.App{Root: ...} that main passes to tela.Main, or run tela dev with --preview")
		}
		return contractApp, "", nil
	}
	switch det.contract {
	case contractNone:
		return contractNone, "", errors.New("the package has no screen a test can mount: " + contractHelp)
	case contractPreviews:
		if preview == "" {
			names := slices.Clone(det.previews)
			slices.Sort(names)
			if len(names) == 0 {
				return contractNone, "", errors.New("tela dev does not say which preview it runs (run it with --preview NAME)")
			}
			preview = names[0]
		}
	}
	return det.contract, preview, nil
}
