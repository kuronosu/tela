package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The film tool of tela mcp: the frames of the motion an action starts (a
// transition, a spring, a Presence exit, a spinner), on the app's clock, as
// a contact sheet the agent can look at, and optionally an animated PNG.
// The instance captures them with telatest's App.Film (mcp_runner.go); the
// pictures are made here (film.go).

// filmActions are the tools whose request a film can start with, and what a
// bare string means for them.
var filmActions = map[string]string{
	"click": "ref", "hover": "ref", "fill": "", "press": "key", "scroll": "", "drag": "",
}

// filmAction reads the action argument of film: an object with one
// key, the name of an acting tool, whose value is that tool's arguments, or
// for click, hover and press a string: the ref, or the key.
func filmAction(raw map[string]json.RawMessage) (tool string, args json.RawMessage, err error) {
	if len(raw) != 1 {
		return "", nil, fmt.Errorf(`action is one tool with its arguments, such as {"click": "e5"} or {"press": "Escape"} or {"fill": {"ref": "e7", "text": "ana"}}; it has %d keys`, len(raw))
	}
	for tool, val := range raw {
		short, ok := filmActions[tool]
		if !ok {
			names := make([]string, 0, len(filmActions))
			for k := range filmActions {
				names = append(names, k)
			}
			slices.Sort(names)
			return "", nil, fmt.Errorf("action %q: a film starts with one of %s", tool, strings.Join(names, ", "))
		}
		var str string
		if json.Unmarshal(val, &str) == nil {
			if short == "" {
				return "", nil, fmt.Errorf("action %s takes its arguments as an object, such as {%q: {...}}", tool, tool)
			}
			b, _ := json.Marshal(map[string]string{short: str})
			return tool, b, nil
		}
		return tool, val, nil
	}
	return "", nil, nil
}

func (s *mcpSession) toolFilm(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		Span   float64                    `json:"span"`
		Every  float64                    `json:"every"`
		Action map[string]json.RawMessage `json:"action"`
		APNG   string                     `json:"apng"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	span, every := time.Duration(a.Span*float64(time.Millisecond)), time.Duration(a.Every*float64(time.Millisecond))
	if a.Span <= 0 {
		o.fail(`film needs span, the milliseconds of the app's clock to film: {"span": 600, "action": {"click": "e5"}}`)
		return o.result()
	}
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
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	// The animation goes inside the module: an agent's path must not reach
	// other files.
	apng := ""
	if a.APNG != "" {
		if apng, err = filmPath(s.target.modDir, a.APNG); err != nil {
			o.fail(err.Error())
			return o.result()
		}
		if err = checkAPNGTarget(apng); err != nil {
			o.fail(err.Error())
			return o.result()
		}
	}
	// The action is the tool's own, run to learn the request it would send
	// (the element resolved, the checks made) without sending it.
	req := mcpReq{Op: "film", Ms: float64(span) / float64(time.Millisecond), Every: float64(every) / float64(time.Millisecond), Snapshot: true}
	desc := "the app as it is"
	if tool != "" {
		var inner mcpReq
		for _, d := range s.defs() {
			if d.tool.Name != tool {
				continue
			}
			if err := checkArgNames(&d, toolArgs); err != nil {
				o.fail(err.Error())
				return o.result()
			}
			s.capture, s.captured = &inner, false
			// Whatever the tool does, the session stops capturing: a panic
			// that is recovered above must not leave later calls capturing.
			res := func() mcpResult {
				defer func() { s.capture = nil }()
				return d.run(s, ctx, toolArgs)
			}()
			if res.IsError {
				return res
			}
		}
		if !s.captured {
			o.fail("the action did not produce a request to film")
			return o.result()
		}
		req.Inner = &inner
		desc = "after " + tool
	}
	before := s.snapshotBefore(ctx, &o)
	rep, ok := s.send(ctx, &o, req)
	if !ok {
		return o.result()
	}
	if len(rep.Film) == 0 {
		o.fail("the instance sent no frames")
		return o.result()
	}
	var film []filmFrame
	for _, f := range rep.Film {
		fr, err := decodeFilmFrame(time.Duration(f.At)*time.Millisecond, f.PNG)
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		film = append(film, fr)
	}
	sheet := contactSheet(film)
	data, err := encodePNG(sheet)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	sb := sheet.Bounds()
	shown := min(len(film), filmSheetFrames)
	o.head = fmt.Sprintf("Filmed %d frames, 0 to %d ms every %d ms of the app's clock (%s). The contact sheet shows %d of them, %dx%d pixels, each captioned with its time; the snapshot below is of the app once settled.", len(film), span.Milliseconds(), every.Milliseconds(), desc, shown, sb.Dx(), sb.Dy())
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
	// The test tool replays the action and then the clock the film advanced
	// (a film of nothing is time passing): the snapshot is of the app once
	// settled after the span, so its checks follow the wait, not the action.
	if req.Inner != nil {
		s.rec.add(mcpStep{req: *req.Inner, before: before, after: before, w: s.curW, h: s.curH}, s.curPreview)
	}
	s.record(mcpReq{Op: "wait", Ms: req.Ms}, before)
	return o.result()
}

// The arguments of film and test, shared with the attached server's stubs.
var (
	filmProps = []schemaProp{
		{"span", "number", "Milliseconds of the app's clock to film, after the action", nil},
		{"every", "number", "Milliseconds between frames (default 50; at least 1)", nil},
		{"action", "object", `One tool and its arguments that starts the motion: {"click": "e5"} (a ref), {"hover": "e3"}, {"press": "Escape"}, or the tool's arguments as an object: {"fill": {"ref": "e7", "text": "ana"}}, {"scroll": {"ref": "e12", "dy": 400}}, {"drag": {"from": {"ref": "e4"}, "dx": 120}}`, nil},
		{"apng", "string", "Also write the frames as an animated PNG to this .png path inside the module (relative to its directory)", nil},
	}
	testProps = []schemaProp{
		{"name", "string", `The test's name: "checkout flow" and "TestCheckoutFlow" are both TestCheckoutFlow`, nil},
		{"path", "string", "The file, in the package's directory (default: the name in snake case plus _test.go, such as checkout_flow_test.go)", nil},
	}
)

const (
	filmDesc = `See motion: capture frames every few milliseconds of the app's clock over a span, as a contact sheet (a grid of the frames, each captioned with its time), so a transition, a spring, a Presence exit or a spinner can be judged. With action, the film starts at that action, so the motion it triggers is captured ({"click": "e5"}, {"hover": "e3"}, {"press": "Escape"}, {"fill": {"ref": "e7", "text": "ana"}}, scroll, drag: the arguments of those tools); without it, the app as it is (a spinner, a timer). The clock is the app's, so the frames are deterministic. Returns the sheet as an image and the settled snapshot; apng also writes every frame as an animated PNG to that path. At most 60 frames (span / every) and 30000 ms. Example: {"span": 600, "every": 50, "action": {"click": "e5"}}`
	testDesc = `Write the acting calls of this session (click, fill, press, hover, scroll, drag, wait, resize, prefer, film actions) as a Go telatest test in the package, then run go vet and the test and report the result. Each action is an app.Click(telatest.ByRole("button", "Save"))-style call (refs become role and name, with Nth when several elements look alike; never node identities), the mount has the session's size, scale, dark mode and preferences, wait is app.Advance, and each action is followed by checks of what changed in the snapshot (an element that appeared, a state that flipped, a text shown or gone, a field's value), not a copy of the whole snapshot, so the test breaks when the behavior does. The recording starts at the latest mount; a change of the source starts it over. A file this tool wrote is replaced; any other is refused. Example: {"name": "checkout flow"}`
)

// mcpFilmDefs are the film and test tools, listed after the tools every
// server shares.
func mcpFilmDefs() []mcpToolDef {
	act := func(title string) *mcpToolAnnotate { return &mcpToolAnnotate{Title: title} }
	def := func(name, desc string, ann *mcpToolAnnotate, props []schemaProp, run func(*mcpSession, context.Context, json.RawMessage) mcpResult, required ...string) mcpToolDef {
		return mcpToolDef{tool: mcpTool{Name: name, Description: desc, InputSchema: schemaOf(props, required...), Annotations: ann}, props: propNames(props), run: run}
	}
	return []mcpToolDef{
		def("film", filmDesc, act("Film motion"), filmProps, (*mcpSession).toolFilm, "span"),
		def("test", testDesc, act("Write a test"), testProps, (*mcpSession).toolTest, "name"),
	}
}

// checkAPNGTarget refuses an apng path that holds a file this tool did not
// write (a golden, an asset): a new path, or an animated PNG of a former
// film, which it replaces.
func checkAPNGTarget(path string) error {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("apng %s: %v", path, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("apng %s exists and is not a regular file (a symlink or a directory): give another path, or remove it", path)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		var chunks []pngChunk
		if chunks, err = splitPNG(data); err == nil {
			for _, c := range chunks {
				if c.Type == "tEXt" && string(c.Data) == apngSoftware {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("apng %s exists and was not written by the film tool: give a new path (the film replaces only an animated PNG it wrote), or remove the file", path)
}

// filmPath resolves the apng argument of film: a .png file inside the
// module's directory (a relative path is relative to it).
func filmPath(modDir, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(modDir, path)
	}
	path = filepath.Clean(path)
	rel, err := filepath.Rel(filepath.Clean(modDir), path)
	switch {
	case err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)):
		return "", fmt.Errorf("apng %s: the animation is written inside the module, %s", path, modDir)
	case !strings.HasSuffix(strings.ToLower(path), ".png"):
		return "", fmt.Errorf("apng %s: an animated PNG is a .png file", path)
	}
	return path, nil
}
