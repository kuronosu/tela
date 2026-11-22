package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"slices"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/lint"
)

// The tools of tela mcp --attach (mcp_attach.go).

// attachToolDef is a tool of the attached server.
type attachToolDef struct {
	mcpToolDef
	run func(s *attachSession, ctx context.Context, args json.RawMessage) mcpResult
}

// attachArgs is what every tool takes besides its own arguments.
type attachArgs struct {
	Window uint32 `json:"window"`
}

// list implements mcpTools.
func (s *attachSession) list() []mcpTool {
	var out []mcpTool
	for _, d := range attachDefs {
		out = append(out, d.tool)
	}
	return out
}

// call implements mcpTools.
func (s *attachSession) call(ctx context.Context, name string, args json.RawMessage) (mcpResult, bool) {
	for _, d := range attachDefs {
		if d.tool.Name != name {
			continue
		}
		if err := checkArgNames(&d.mcpToolDef, args); err != nil {
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

var windowProp = schemaProp{"window", "integer", "ID of the window, from windows (default: the first window the app shows)", nil}

var attachDefs = func() []attachToolDef {
	ro := func(title string) *mcpToolAnnotate {
		return &mcpToolAnnotate{Title: title, ReadOnlyHint: true, IdempotentHint: true}
	}
	act := func(title string) *mcpToolAnnotate { return &mcpToolAnnotate{Title: title} }
	def := func(name, desc string, ann *mcpToolAnnotate, props []schemaProp, run func(*attachSession, context.Context, json.RawMessage) mcpResult, required ...string) attachToolDef {
		props = append(slices.Clone(props), windowProp)
		return attachToolDef{mcpToolDef: mcpToolDef{tool: mcpTool{Name: name, Description: desc, InputSchema: schemaOf(props, required...), Annotations: ann}, props: propNames(props)}, run: run}
	}
	point := []schemaProp{
		{"x", "number", "X in logical pixels from the window's left edge (instead of an element)", nil},
		{"y", "number", "Y in logical pixels from the window's top edge", nil},
	}
	unavailable := func(name, why string) func(*attachSession, context.Context, json.RawMessage) mcpResult {
		return func(*attachSession, context.Context, json.RawMessage) mcpResult {
			var o mcpOut
			o.fail(name + " is not available while attached to tela dev: " + why)
			return o.result()
		}
	}
	const resizeWhy = "the window is the person's, on the real desktop, and tela dev does not resize it. Resize it by hand, or use the headless server (tela mcp ./pkg), which keeps a window of its own"
	const openWhy = "there is one running app, not previews to mount. Use the headless server (tela mcp ./pkg) for another preview, scheme or size"
	const preferWhy = "the running app reads the system's own preferences (dark mode, reduced motion, text scale...); change them in the desktop's settings, or use the headless server (tela mcp ./pkg), which sets them live"
	return []attachToolDef{
		def("windows", `List the windows the running app shows: ID, title, logical size, display scale and which one has the keyboard focus. Every other tool takes one of these IDs as window; without it, the first window. Example: {}`,
			ro("Windows"), nil, (*attachSession).toolWindows),
		def("snapshot", `Show the structure of the real window as an aria snapshot (Playwright's YAML): roles, accessible names, states and texts, with [ref=eN] on each element you can act on and [src=file:line], the source line that made it. Cheap: use it first and after every edit (tela dev rebuilds the app when the source is saved, and the result says so). Example: {}`,
			ro("Aria snapshot"), nil, (*attachSession).toolSnapshot),
		def("screenshot", `The frame the window presents, as a PNG of its real pixels (at the display's scale: the answer says it); with a ref, only that element with an 8 px margin. Use it to judge the looks. Example: {} or {"ref": "e5"}`,
			ro("Screenshot"), elementProps("element"), (*attachSession).toolScreenshot),
		def("click", `Click an element with real pointer events at its painted position (hover, press, release; focus moves to it), wait until the app has stopped drawing, and return the new snapshot. The element is scrolled into view first, and a click where another element covers it (a dialog, a popover) is refused with what covers it. Name it by its ref (or role and name, text), or give x and y. button "right" opens a context menu; double: true double-clicks. A disabled element cannot be clicked. Example: {"ref": "e5"}`,
			act("Click"), append(append(elementProps("element"), point...),
				schemaProp{"button", "string", `"left" (default) or "right"`, []string{"left", "right"}},
				schemaProp{"double", "boolean", "Double-click", nil}), (*attachSession).toolClick),
		def("hover", `Move the pointer over an element (or to x, y), so its hover: styles and tooltips apply, and return the new snapshot. Example: {"ref": "e3"}`,
			act("Hover"), append(elementProps("element"), point...), (*attachSession).toolHover),
		def("fill", `Replace the text of a text field the way a person does: click it, select all, type the text (OnChange fires for each character); submit: true presses Enter afterwards. Returns the new snapshot. Example: {"ref": "e7", "text": "ana@example.com"}`,
			act("Fill a field"), append(elementProps("field"),
				schemaProp{"text", "string", "The new value", nil},
				schemaProp{"submit", "boolean", "Press Enter after filling", nil}), (*attachSession).toolFill, "text"),
		def("press", `Press a key or a combo, Playwright's syntax ("Enter", "Tab", "Shift+Tab", "Escape", "ArrowDown", "Control+a", "a"), on the focused element of the window; with a ref of a text field, it clicks the field first to focus it (another element cannot be focused without activating it: Tab to it). Returns the new snapshot. Example: {"key": "Tab"}`,
			act("Press a key"), append([]schemaProp{{"key", "string", `Key or combo, such as "Enter" or "Shift+Tab"`, nil}}, elementProps("text field to focus first")...), (*attachSession).toolPress, "key"),
		def("scroll", `Scroll the mouse wheel over an element by dx, dy logical pixels (positive: right, down): the nearest scrollable container under it moves. Without an element the wheel goes to x, y, or to the center of the window. Returns the new snapshot. Example: {"ref": "e12", "dy": 400} or {"dy": 400}`,
			act("Scroll"), append(append(elementProps("element"), point...),
				schemaProp{"dx", "number", "Horizontal distance in logical pixels (positive: right)", nil},
				schemaProp{"dy", "number", "Vertical distance in logical pixels (positive: down)", nil}), (*attachSession).toolScroll),
		def("drag", `Drag with the primary button: press on from, move in steps, release on to (an element or a point), or by dx, dy from the start. For sliders, reordering, resizable panes. Returns the new snapshot. Example: {"from": {"ref": "e4"}, "dx": 120} or {"from": {"ref": "e4"}, "to": {"ref": "e9"}}`,
			act("Drag"), []schemaProp{
				{"from", "element", "Where the drag starts: {\"ref\": \"e4\"} (or role and name, text), or {\"x\": 10, \"y\": 20}", nil},
				{"to", "element", "Where it ends: an element or a point", nil},
				{"dx", "number", "Without to: horizontal distance from the start", nil},
				{"dy", "number", "Without to: vertical distance from the start", nil},
			}, (*attachSession).toolDrag, "from"),
		def("resize", `Not available while attached to tela dev: the window is the person's. Use the headless server for this.`,
			act("Resize the window"), []schemaProp{
				{"width", "number", "Logical width in pixels", nil},
				{"height", "number", "Logical height in pixels", nil},
				{"scale", "number", "Display scale", nil},
			}, unavailable("resize", resizeWhy)),
		def("open", `Not available while attached to tela dev: there is one running app. Use the headless server for previews.`,
			act("Open a screen"), []schemaProp{
				{"preview", "string", "Name of a preview", nil},
				{"width", "number", "Logical width in pixels", nil},
				{"height", "number", "Logical height in pixels", nil},
				{"scale", "number", "Display scale", nil},
			}, unavailable("open", openWhy)),
		def("wait", `Wait in real time (the real app's clock runs by itself): ms milliseconds (at most 60000), and/or up to 10 s until a text appears (text) or disappears (textGone), as for data loaded by a goroutine. Returns the new snapshot. Example: {"text": "Loaded"}`,
			act("Wait"), []schemaProp{
				{"ms", "number", "Milliseconds to wait", nil},
				{"text", "string", "Wait until the window shows this text", nil},
				{"textGone", "string", "Wait until the window no longer shows this text", nil},
			}, (*attachSession).toolWait),
		def("inspect", `Show the layout tree of the running app as JSON (tela inspect --layout): each box's rect in logical pixels, margin, border, padding, classes and computed style, with its source line. With an element (ref), only its box and what it contains. Use it when something is misplaced, misaligned or has the wrong size. Example: {"ref": "e5"}`,
			ro("Layout tree"), elementProps("element"), (*attachSession).toolInspect),
		def("state", `Show why a component rendered and what it holds: its props, its hooks (kind, call site, current value), how many times it built and the reason of each of its last builds. With an element (its ref, or role and name, text), the component that rendered it (the nearest one above it); without, the window's components that have props or hooks, in tree order (max, default 40). Example: {"ref": "e5"}`,
			ro("Component state"), append(elementProps("element"),
				schemaProp{"max", "integer", "Without an element: the most components to list (1 to 200, default 40)", nil}),
			(*attachSession).toolState),
		def("logs", `List what tela dev has logged since the last logs call, with levels: the dev warnings and errors of Tela (unknown classes, missing keys, misused hooks: "dev") and what the app wrote on its standard error ("stderr"; standard output is not kept). Every other result already carries the dev warnings it caused. Example: {"level": "warn"}`,
			ro("Logs"), []schemaProp{
				{"level", "string", `Only this level and above: "info" (everything, the default), "warn" or "error"`, []string{"info", "warn", "error"}},
				{"all", "boolean", "Everything the session kept, not only what is new since the last call", nil},
			}, (*attachSession).toolLogs),
		def("perf", `Show how long the last frames of the real window took, per phase of the pipeline (input, animate, build, style, layout, paint, semantics, raster, present, effects), with what each built, styled, laid out and repainted, and a summary. With action: true, the frames the last click, press, scroll... caused. Example: {"action": true}`,
			ro("Frame profile"), []schemaProp{
				{"frames", "integer", "The last N frames (1 to 200, default 10)", nil},
				{"action", "boolean", "Only the frames the last acting call caused", nil},
			}, (*attachSession).toolPerf),
		def("prefer", `Not available while attached to tela dev: the app reads the system's own preferences. Use the headless server for this.`,
			act("Set preferences"), []schemaProp{
				{"dark", "boolean", "Dark mode", nil},
				{"reducedMotion", "boolean", "Reduced motion", nil},
				{"highContrast", "boolean", "High contrast", nil},
				{"textScale", "number", "Text scale", nil},
				{"locale", "string", "A BCP 47 tag", nil},
				{"direction", "string", `"rtl", "ltr" or "default"`, []string{"ltr", "rtl", "default"}},
			}, unavailable("prefer", preferWhy)),
		def("lint", `Check the package's Go source for what dev mode reports at run time, without running it: unknown Tailwind classes (with a suggestion), ARIA roles and tokens, duplicate or missing keys, misused hooks. Prints tela lint -json. It lints the directory of the package tela dev runs. Example: {}`,
			ro("Lint"), nil, (*attachSession).toolLint),
	}
}()

// ---------------------------------------------------------------------
// Targets.

// attachTarget is the element a tool acts on.
type attachTarget struct {
	win      toolsWindow
	line     *attachLine
	desc     string
	disabled bool
}

// resolve finds the element an element argument names, in the window the
// call is about. A ref names its element and window; the others are looked
// up in a fresh snapshot of the window.
func (s *attachSession) resolve(ctx context.Context, winArg uint32, a elementArgs) (attachTarget, error) {
	ws, err := s.conn.Windows(ctx)
	if err != nil {
		return attachTarget{}, err
	}
	w, err := pickWindow(ws, winArg)
	if err != nil {
		return attachTarget{}, err
	}
	switch {
	case a.TestID != "":
		return attachTarget{}, fmt.Errorf("testId is not available while attached: the aria snapshot of a real window has no test ids; name the element by its ref, or by role and name, or text")
	case a.Name != "" && a.Role == "":
		return attachTarget{}, fmt.Errorf(`name needs a role: {"role": "button", "name": "Save"}; or use the element's ref from the snapshot`)
	case !a.given():
		return attachTarget{}, fmt.Errorf(`give the element: its ref from the latest snapshot ({"ref": "e5"}), or role and name, or text`)
	}
	if a.Ref != "" {
		ref := strings.TrimSpace(a.Ref)
		if winArg == 0 {
			if owner, ok := s.findRef(ref, w.ID); ok && owner != w.ID {
				if w, err = pickWindow(ws, owner); err != nil {
					return attachTarget{}, errRefGone(ref, "its window is not open any more")
				}
			}
		}
		snap, err := s.snapshot(ctx, w)
		if err != nil {
			return attachTarget{}, err
		}
		aw := s.win(w.ID)
		if line := snap.byRef[ref]; line != nil {
			e := aw.refs.current[ref]
			return attachTarget{win: w, line: line, desc: e.desc + " [ref=" + ref + "]", disabled: e.disabled}, nil
		}
		if _, err := aw.refs.lookup(ref); err != nil {
			return attachTarget{}, err
		}
		return attachTarget{}, errRefGone(ref, "the element was removed, or is hidden")
	}
	snap, err := s.snapshot(ctx, w)
	if err != nil {
		return attachTarget{}, err
	}
	var found []*attachLine
	var desc string
	for i := range snap.lines {
		l := &snap.lines[i]
		if !l.ok || l.node == 0 {
			continue
		}
		switch {
		case a.Role != "":
			if l.parsed.Role == a.Role && (a.Name == "" || l.parsed.Name == a.Name) {
				found = append(found, l)
			}
		default:
			hay := l.parsed.Name + " " + strings.TrimPrefix(l.parsed.Rest, ":")
			if a.Exact && (l.parsed.Name == a.Text || strings.TrimSpace(strings.TrimPrefix(l.parsed.Rest, ":")) == a.Text) ||
				!a.Exact && strings.Contains(strings.ToLower(hay), strings.ToLower(a.Text)) {
				found = append(found, l)
			}
		}
	}
	if a.Role != "" {
		desc = a.Role
		if a.Name != "" {
			desc += " " + fmt.Sprintf("%q", a.Name)
		}
	} else {
		desc = "the element with the text " + fmt.Sprintf("%q", a.Text)
	}
	switch {
	case len(found) == 0:
		return attachTarget{}, fmt.Errorf("no element matches %s; take a snapshot and use a ref", desc)
	case a.Nth != nil && (*a.Nth < 0 || *a.Nth >= len(found)):
		return attachTarget{}, fmt.Errorf("nth %d: %s matches %d elements", *a.Nth, desc, len(found))
	case a.Nth == nil && len(found) > 1:
		return attachTarget{}, fmt.Errorf("%s matches %d elements; give nth (0-based) or a ref from the snapshot", desc, len(found))
	}
	line := found[0]
	if a.Nth != nil {
		line = found[*a.Nth]
	}
	return attachTarget{win: w, line: line, desc: line.parsed.Describe(), disabled: line.parsed.Disabled}, nil
}

// revealTries bounds the scrolls into view of one target: a scroll that
// is still under way after them (a container that keeps moving) is
// acted on where it is.
const revealTries = 3

// point returns where to act on the element, in logical pixels of its
// window, with Playwright's actionability: the element is scrolled into
// view (its scroll containers move as little as needed, and the session
// waits for them), and the point is one of its visible part that hit
// testing sends to it or to a descendant: its center, else the center of
// a quarter. A point where another element takes the pointer (a dialog,
// a popover, a sticky header over it) makes the element covered.
func (s *attachSession) point(ctx context.Context, t attachTarget) (x, y float32, err error) {
	info, err := s.conn.Element(ctx, t.win.ID, t.line.el)
	if err != nil {
		return 0, 0, err
	}
	if info.Bounds == nil || info.Bounds.X1 <= info.Bounds.X0 || info.Bounds.Y1 <= info.Bounds.Y0 {
		return 0, 0, fmt.Errorf("%s has no painted box (it has no size, or is not drawn)", t.desc)
	}
	var vis *gfx.Rect
	for i := 0; ; i++ {
		s.frameCursor(ctx, t.win.ID)
		start := time.Now()
		r, err := s.conn.Reveal(ctx, t.win.ID, t.line.el)
		if unsupported(err) {
			return s.center(t, *info.Bounds) // an app of an older Tela
		}
		if err != nil {
			return 0, 0, err
		}
		vis = r.Visible
		if !r.Moved || i == revealTries-1 {
			break
		}
		s.settle(ctx, t.win.ID, start) // the scroll's frames
	}
	if vis == nil || vis.Empty() {
		return 0, 0, fmt.Errorf("%s is not visible even scrolled into view: an ancestor that clips its overflow (overflow-hidden, overflow-clip) or the window's edge cuts it whole", t.desc)
	}
	pts := actionPoints(*vis)
	var first toolsHit
	for i, p := range pts {
		h, err := s.conn.Hit(ctx, t.win.ID, p.X, p.Y, t.line.el)
		if err != nil {
			return 0, 0, err
		}
		if h.Within {
			return p.X, p.Y, nil
		}
		if i == 0 {
			first = h
		}
	}
	c := pts[0]
	return 0, 0, fmt.Errorf("%s is covered by %s at (%g, %g), and at the other points of its visible part: close or move what covers it, or act on that instead (an element with pointer-events-none lets the pointer through to what is under it)",
		t.desc, s.describeHit(ctx, t.win, first), round2(float64(c.X)), round2(float64(c.Y)))
}

// actionPoints are the points of visible rect r an action tries, in
// order: its center, then the centers of its quarters.
func actionPoints(r gfx.Rect) []gfx.Point {
	c := r.Center()
	dx, dy := r.Width()/4, r.Height()/4
	return []gfx.Point{c, {X: c.X - dx, Y: c.Y - dy}, {X: c.X + dx, Y: c.Y - dy}, {X: c.X - dx, Y: c.Y + dy}, {X: c.X + dx, Y: c.Y + dy}}
}

// describeHit names what a hit found, as the snapshot shows it (with its
// ref), else by its element's kind.
func (s *attachSession) describeHit(ctx context.Context, w toolsWindow, h toolsHit) string {
	if h.Node != 0 {
		if snap, err := s.snapshot(ctx, w); err == nil {
			for i := range snap.lines {
				l := &snap.lines[i]
				if l.node != h.Node || !l.ok {
					continue
				}
				d := l.parsed.Describe()
				if ref, ok := s.win(w.ID).refs.byNode[h.Node]; ok {
					d += " [ref=" + ref + "]"
				}
				return d
			}
		}
	}
	if h.ID != 0 {
		if info, err := s.conn.Element(ctx, w.ID, h.ID); err == nil && info.Kind != "" {
			return "a " + info.Kind + " element"
		}
	}
	return "another element"
}

// unsupported reports the error of a call the app (or tela dev) does not
// answer: one of an older Tela.
func unsupported(err error) bool {
	var ce *toolsCallError
	return errors.As(err, &ce) && ce.Code == rpcMethodNotFound
}

// center returns the middle of bounds b of the element, for an app that
// cannot scroll it into view or hit-test (an older Tela).
func (s *attachSession) center(t attachTarget, b gfx.Rect) (x, y float32, err error) {
	x, y = (b.X0+b.X1)/2, (b.Y0+b.Y1)/2
	if x < 0 || y < 0 || x >= t.win.Width || y >= t.win.Height {
		return 0, 0, fmt.Errorf("%s is at (%g, %g), outside the window's %gx%g: scroll it into view first", t.desc, round2(float64(x)), round2(float64(y)), round2(float64(t.win.Width)), round2(float64(t.win.Height)))
	}
	return x, y, nil
}

// target resolves the element of an acting tool and its position. enabled
// makes a disabled element an error.
func (s *attachSession) target(ctx context.Context, o *mcpOut, winArg uint32, a elementArgs, verb string, enabled bool) (t attachTarget, x, y float32, ok bool) {
	t, err := s.resolve(ctx, winArg, a)
	if err != nil {
		o.fail(err.Error())
		return t, 0, 0, false
	}
	if enabled && t.disabled {
		o.fail(fmt.Sprintf("cannot %s %s: it is disabled (the snapshot marks it [disabled])", verb, t.desc))
		return t, 0, 0, false
	}
	x, y, err = s.point(ctx, t)
	if err != nil {
		o.fail(err.Error())
		return t, 0, 0, false
	}
	return t, x, y, true
}

// atPoint checks a point of the window the tool was given.
func atPoint(w toolsWindow, x, y float64) error {
	if x < 0 || y < 0 || x >= float64(w.Width) || y >= float64(w.Height) {
		return fmt.Errorf("the point (%g, %g) is outside the window's %gx%g logical pixels", x, y, round2(float64(w.Width)), round2(float64(w.Height)))
	}
	return nil
}

// act runs an input on window w, waits until the app has settled and
// answers with the new snapshot.
//
// The frames the action caused (those after the frame the app was at when
// it began, a scroll into view during it included) are what perf
// {"action": true} lists, and the result sums them up.
func (s *attachSession) act(ctx context.Context, o *mcpOut, w toolsWindow, done string, do func() error) mcpResult {
	from := s.frameCursor(ctx, w.ID)
	start := time.Now()
	o.screen = describeWindow(w)
	if err := do(); err != nil {
		o.fail(err.Error())
		s.collectLogs(ctx, o)
		return o.result()
	}
	_, settled := s.settle(ctx, w.ID, start)
	aw := s.win(w.ID)
	if ps, err := s.conn.Profiles(ctx, w.ID, from); err == nil {
		aw.action = ps
	}
	o.frames = framesLine(profileFrames(aw.action))
	snap, err := s.snapshot(ctx, w)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	o.head = done
	if !settled {
		o.notes = append(o.notes, fmt.Sprintf("The app was still drawing frames after %v (an animation?): the snapshot is of that moment.", s.max))
	}
	o.snapshot = &snap.yaml
	s.collectLogs(ctx, o)
	return o.result()
}

// ---------------------------------------------------------------------
// The tools.

func (s *attachSession) toolWindows(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	if !s.begin(ctx, &o) {
		return o.result()
	}
	ws, err := s.conn.Windows(ctx)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if len(ws) == 0 {
		o.head = "The app shows no window yet (tela dev is starting it, or it crashed)."
		return o.result()
	}
	var b strings.Builder
	first := true
	for _, w := range ws {
		fmt.Fprintf(&b, "- window %d %q: %gx%g logical pixels, scale %g", w.ID, w.Title, round2(float64(w.Width)), round2(float64(w.Height)), w.Scale)
		if w.Focused {
			b.WriteString(", focused")
		}
		switch {
		case !w.Active:
			b.WriteString(", not shown by the running app (the last frame of a build that crashed)")
		case first:
			b.WriteString(", the default")
			first = false
		}
		if !w.Visible {
			b.WriteString(", hidden")
		}
		b.WriteString("\n")
	}
	o.head = fmt.Sprintf("%d window(s) of the app tela dev runs (%s, build %d); pass an ID as window:", len(ws), s.info.Pkg, s.info.Build)
	o.sections = append(o.sections, strings.TrimRight(b.String(), "\n"))
	return o.result()
}

func (s *attachSession) toolSnapshot(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a attachArgs
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	w, err := s.window(ctx, a.Window)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	snap, err := s.snapshot(ctx, w)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	o.screen = describeWindow(w)
	o.snapshot = &snap.yaml
	s.collectLogs(ctx, &o)
	return o.result()
}

func (s *attachSession) toolScreenshot(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
		elementArgs
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	w, err := s.window(ctx, a.Window)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	var t attachTarget
	var bounds *core.ElementInfo
	if a.given() {
		if t, err = s.resolve(ctx, a.Window, a.elementArgs); err != nil {
			o.fail(err.Error())
			return o.result()
		}
		w = t.win
		info, err := s.conn.Element(ctx, w.ID, t.line.el)
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		bounds = &info
	}
	img, err := s.conn.Screenshot(ctx, w.ID)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	o.screen = describeWindow(w)
	b := img.Bounds()
	scale := 1.0
	if w.Width > 0 {
		scale = float64(b.Dx()) / float64(w.Width)
	}
	head := fmt.Sprintf("Screenshot of window %d, the frame it presents: %dx%d real pixels (%gx%g logical at scale %g).", w.ID, b.Dx(), b.Dy(), round2(float64(w.Width)), round2(float64(w.Height)), w.Scale)
	if bounds != nil {
		if bounds.Bounds == nil {
			o.fail(t.desc + " has no painted box to cut out")
			return o.result()
		}
		r := *bounds.Bounds
		const margin = 8
		x0, y0 := int((float64(r.X0)-margin)*scale), int((float64(r.Y0)-margin)*scale)
		x1, y1 := int((float64(r.X1)+margin)*scale+0.999), int((float64(r.Y1)+margin)*scale+0.999)
		crop := image.Rect(x0, y0, x1, y1).Add(b.Min).Intersect(b)
		sub, ok := img.(interface {
			SubImage(image.Rectangle) image.Image
		})
		if !ok || crop.Empty() {
			o.fail(t.desc + " is outside the frame the window presents")
			return o.result()
		}
		img = sub.SubImage(crop)
		head = fmt.Sprintf("Screenshot of %s of window %d at (%g, %g), %gx%g logical pixels, with an 8 px margin: %dx%d real pixels (scale %g).", t.desc, w.ID, round2(float64(r.X0)), round2(float64(r.Y0)), round2(float64(r.X1-r.X0)), round2(float64(r.Y1-r.Y0)), crop.Dx(), crop.Dy(), w.Scale)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	o.head = head
	o.images = append(o.images, mcpContent{Type: "image", Data: base64.StdEncoding.EncodeToString(buf.Bytes()), MimeType: "image/png"})
	s.collectLogs(ctx, &o)
	return o.result()
}

func (s *attachSession) toolClick(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
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
	if !a.given() && (a.X != nil) != (a.Y != nil) {
		o.fail("give both x and y")
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	verb, count := "Clicked", 1
	if a.Double {
		verb, count = "Double-clicked", 2
	} else if a.Button == "right" {
		verb = "Right-clicked"
	}
	var w toolsWindow
	var x, y float32
	var desc string
	if !a.given() && a.X != nil {
		var err error
		if w, err = s.window(ctx, a.Window); err == nil {
			err = atPoint(w, *a.X, *a.Y)
		}
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		x, y, desc = float32(*a.X), float32(*a.Y), fmt.Sprintf("at (%g, %g)", *a.X, *a.Y)
	} else {
		t, cx, cy, ok := s.target(ctx, &o, a.Window, a.elementArgs, "click", true)
		if !ok {
			return o.result()
		}
		w, x, y, desc = t.win, cx, cy, t.desc
	}
	return s.act(ctx, &o, w, verb+" "+desc+".", func() error {
		return s.conn.Clicks(ctx, w.ID, x, y, a.Button, count)
	})
}

func (s *attachSession) toolHover(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
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
	var w toolsWindow
	var x, y float32
	var desc string
	if !a.given() && a.X != nil && a.Y != nil {
		var err error
		if w, err = s.window(ctx, a.Window); err == nil {
			err = atPoint(w, *a.X, *a.Y)
		}
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		x, y, desc = float32(*a.X), float32(*a.Y), fmt.Sprintf("(%g, %g)", *a.X, *a.Y)
	} else {
		t, cx, cy, ok := s.target(ctx, &o, a.Window, a.elementArgs, "hover", false)
		if !ok {
			return o.result()
		}
		w, x, y, desc = t.win, cx, cy, t.desc
	}
	return s.act(ctx, &o, w, "The pointer is over "+desc+".", func() error {
		return s.conn.Pointer(ctx, w.ID, "move", x, y, "")
	})
}

func (s *attachSession) toolFill(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
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
	a.Text = "" // the value is the text: the field is named by ref, or role and name
	if !s.begin(ctx, &o) {
		return o.result()
	}
	t, x, y, ok := s.target(ctx, &o, a.Window, a.elementArgs, "fill", true)
	if !ok {
		return o.result()
	}
	done := fmt.Sprintf("Filled %s with %q (clicked it, selected all, typed).", t.desc, *a.Value)
	if a.Submit {
		done += " Pressed Enter."
	}
	return s.act(ctx, &o, t.win, done, func() error {
		w := t.win.ID
		if err := s.conn.Click(ctx, w, x, y); err != nil {
			return err
		}
		if err := s.conn.Key(ctx, w, "ControlOrMeta+a", ""); err != nil {
			return err
		}
		if *a.Value == "" {
			if err := s.conn.Key(ctx, w, "Backspace", ""); err != nil {
				return err
			}
		} else if err := s.conn.Text(ctx, w, *a.Value); err != nil {
			return err
		}
		if a.Submit {
			return s.conn.Key(ctx, w, "Enter", "")
		}
		return nil
	})
}

// focusOnClick are the roles whose elements a click focuses without
// doing anything else.
var focusOnClick = []string{"textbox", "searchbox", "combobox", "spinbutton", "slider"}

func (s *attachSession) toolPress(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
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
	done := fmt.Sprintf("Pressed %s.", a.Key)
	var w toolsWindow
	var x, y float32
	focus := false
	if a.given() {
		t, cx, cy, ok := s.target(ctx, &o, a.Window, a.elementArgs, "focus", false)
		if !ok {
			return o.result()
		}
		if !slices.Contains(focusOnClick, t.line.parsed.Role) {
			o.fail(fmt.Sprintf("cannot focus %s without activating it: a click on it does what a person's click does. Press the key on the focused element (Tab and Shift+Tab move the focus), or click it", t.desc))
			return o.result()
		}
		w, x, y, focus = t.win, cx, cy, true
		done = fmt.Sprintf("Clicked %s to focus it and pressed %s.", t.desc, a.Key)
	} else {
		var err error
		if w, err = s.window(ctx, a.Window); err != nil {
			o.fail(err.Error())
			return o.result()
		}
	}
	return s.act(ctx, &o, w, done, func() error {
		if focus {
			if err := s.conn.Click(ctx, w.ID, x, y); err != nil {
				return err
			}
		}
		return s.conn.Key(ctx, w.ID, a.Key, "")
	})
}

func (s *attachSession) toolScroll(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
		elementArgs
		DX float64  `json:"dx"`
		DY float64  `json:"dy"`
		X  *float64 `json:"x"`
		Y  *float64 `json:"y"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.DX == 0 && a.DY == 0 {
		o.fail(`scroll needs a distance: {"ref": "e12", "dy": 400} (positive dy scrolls down), or {"dy": 400} for the window`)
		return o.result()
	}
	if !a.given() && (a.X == nil) != (a.Y == nil) {
		o.fail("give both x and y")
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	var w toolsWindow
	var x, y float32
	var desc string
	switch {
	case a.given():
		t, cx, cy, ok := s.target(ctx, &o, a.Window, a.elementArgs, "scroll", false)
		if !ok {
			return o.result()
		}
		w, x, y, desc = t.win, cx, cy, t.desc
	default:
		var err error
		if w, err = s.window(ctx, a.Window); err != nil {
			o.fail(err.Error())
			return o.result()
		}
		if a.X != nil {
			if err := atPoint(w, *a.X, *a.Y); err != nil {
				o.fail(err.Error())
				return o.result()
			}
			x, y, desc = float32(*a.X), float32(*a.Y), fmt.Sprintf("the point (%g, %g)", *a.X, *a.Y)
		} else {
			x, y, desc = w.Width/2, w.Height/2, "the window (at its center)"
		}
	}
	return s.act(ctx, &o, w, fmt.Sprintf("Scrolled by (%g, %g) over %s.", a.DX, a.DY, desc), func() error {
		return s.conn.Scroll(ctx, w.ID, x, y, float32(a.DX), float32(a.DY))
	})
}

// dragSteps is how many moves a drag is made of.
const dragSteps = 8

func (s *attachSession) toolDrag(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
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
	w, err := s.window(ctx, a.Window)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	// end resolves the from or to: an element, or a point of the window.
	// Both ends are in the window of the first.
	winArg := a.Window
	end := func(e *dragEnd, enabled bool) (x, y float32, desc string, ok bool) {
		if !e.given() && e.X != nil && e.Y != nil {
			if err := atPoint(w, *e.X, *e.Y); err != nil {
				o.fail(err.Error())
				return 0, 0, "", false
			}
			return float32(*e.X), float32(*e.Y), fmt.Sprintf("(%g, %g)", *e.X, *e.Y), true
		}
		t, cx, cy, ok := s.target(ctx, &o, winArg, e.elementArgs, "drag", enabled)
		if !ok {
			return 0, 0, "", false
		}
		w, winArg = t.win, t.win.ID
		return cx, cy, t.desc, true
	}
	x0, y0, fromDesc, ok := end(a.From, true)
	if !ok {
		return o.result()
	}
	done := "Dragged " + fromDesc
	var x1, y1 float32
	if a.To != nil {
		var toDesc string
		if x1, y1, toDesc, ok = end(a.To, false); !ok {
			return o.result()
		}
		done += " to " + toDesc + "."
	} else {
		x1, y1 = x0+float32(a.DX), y0+float32(a.DY)
		done += fmt.Sprintf(" by (%g, %g).", a.DX, a.DY)
	}
	return s.act(ctx, &o, w, done, func() error {
		id := w.ID
		if err := s.conn.Pointer(ctx, id, "down", x0, y0, ""); err != nil {
			return err
		}
		// letGo releases the button where the pointer is when the drag
		// stops early (a move failed, the call was cancelled): the app
		// would keep a button held, and the session would keep dragging
		// with every later move. Its own context: ctx may be done.
		x, y := x0, y0
		letGo := func(err error) error {
			uctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			s.conn.Pointer(uctx, id, "up", x, y, "")
			return err
		}
		for i := 1; i <= dragSteps; i++ {
			f := float32(i) / dragSteps
			mx, my := x0+(x1-x0)*f, y0+(y1-y0)*f
			if err := s.conn.Pointer(ctx, id, "move", mx, my, ""); err != nil {
				return letGo(err)
			}
			x, y = mx, my
			select {
			case <-ctx.Done():
				return letGo(ctx.Err())
			case <-time.After(8 * time.Millisecond):
			}
		}
		if err := s.conn.Pointer(ctx, id, "up", x1, y1, ""); err != nil {
			return letGo(err)
		}
		return nil
	})
}

func (s *attachSession) toolWait(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
		Ms   float64 `json:"ms"`
		Text string  `json:"text"`
		Gone string  `json:"textGone"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	switch {
	case a.Ms < 0 || a.Ms > float64(attachWaitMax/time.Millisecond):
		o.fail("ms must be between 0 and 60000 (a minute of real time)")
		return o.result()
	case a.Ms == 0 && a.Text == "" && a.Gone == "":
		o.fail(`wait needs ms, text or textGone: {"ms": 500}`)
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	w, err := s.window(ctx, a.Window)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	var did []string
	if a.Ms > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(a.Ms * float64(time.Millisecond))):
		}
		did = append(did, fmt.Sprintf("waited %g ms of real time", a.Ms))
	}
	var snap *attachSnap
	deadline := time.Now().Add(attachWaitText)
	for a.Text != "" || a.Gone != "" {
		if snap, err = s.snapshot(ctx, w); err != nil {
			o.fail(err.Error())
			return o.result()
		}
		if (a.Text == "" || strings.Contains(snap.yaml, a.Text)) && (a.Gone == "" || !strings.Contains(snap.yaml, a.Gone)) {
			break
		}
		if ctx.Err() != nil {
			break
		}
		if time.Now().After(deadline) {
			what := fmt.Sprintf("%q to go", a.Gone)
			if a.Text != "" && !strings.Contains(snap.yaml, a.Text) {
				what = fmt.Sprintf("%q to appear", a.Text)
			}
			o.fail(fmt.Sprintf("timed out after %v waiting for %s; the snapshot below is the window now", attachWaitText, what))
			o.snapshot = &snap.yaml
			return o.result()
		}
		select {
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
	}
	if a.Text != "" {
		did = append(did, fmt.Sprintf("%q is shown", a.Text))
	}
	if a.Gone != "" {
		did = append(did, fmt.Sprintf("%q is gone", a.Gone))
	}
	if snap == nil {
		if snap, err = s.snapshot(ctx, w); err != nil {
			o.fail(err.Error())
			return o.result()
		}
	}
	o.screen = describeWindow(w)
	o.head = "Waited: " + strings.Join(did, "; ") + "."
	o.snapshot = &snap.yaml
	s.collectLogs(ctx, &o)
	return o.result()
}

func (s *attachSession) toolInspect(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
		elementArgs
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	w, err := s.window(ctx, a.Window)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	var id core.ElementID
	desc := "window " + fmt.Sprint(w.ID)
	if a.given() {
		t, err := s.resolve(ctx, a.Window, a.elementArgs)
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		w, id, desc = t.win, t.line.el, t.desc
	}
	node, err := s.conn.Inspect(ctx, w.ID, id)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	js, err := json.MarshalIndent(node, "", "  ")
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	o.head = "Layout tree of " + desc + " (rect: border box in logical pixels from the window's top-left corner)."
	o.sections = append(o.sections, "```json\n"+string(js)+"\n```")
	return o.result()
}

// attachComponent is a component element as writeComponent shows it.
func attachComponent(e core.ElementInfo) mcpComponent {
	c := mcpComponent{Name: e.Kind, Key: e.Key, Source: e.Site.Rel(), Builds: e.Builds}
	for _, p := range e.Props {
		c.Props = append(c.Props, struct{ Name, Value string }{p.Name, p.Value})
	}
	for _, h := range e.Hooks {
		c.Hooks = append(c.Hooks, struct {
			Index               int
			Kind, Source, Value string
		}{h.Index, h.Kind, h.Site.Rel(), h.Value})
	}
	for _, r := range e.Recent {
		rr := struct {
			Frame   int
			Reasons []string
		}{Frame: r.Frame}
		for _, why := range r.Reasons {
			rr.Reasons = append(rr.Reasons, why.String())
		}
		c.Renders = append(c.Renders, rr)
	}
	return c
}

func (s *attachSession) toolState(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
		elementArgs
		Max int `json:"max"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.Max < 0 || a.Max > 200 {
		o.fail("max must be between 1 and 200 components")
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	w, err := s.window(ctx, a.Window)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	var el core.ElementID
	if a.given() {
		t, err := s.resolve(ctx, a.Window, a.elementArgs)
		if err != nil {
			o.fail(err.Error())
			return o.result()
		}
		w, el = t.win, t.line.el
	}
	els, err := s.conn.Elements(ctx, w.ID, true)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	var comps []mcpComponent
	total := 0
	if el != 0 {
		byID := make(map[core.ElementID]core.ElementInfo, len(els))
		for _, e := range els {
			byID[e.ID] = e
		}
		for id := el; id != 0; id = byID[id].Parent {
			if e, ok := byID[id]; ok && e.Component {
				comps = append(comps, attachComponent(e))
				break
			}
		}
	} else {
		limit := a.Max
		if limit == 0 {
			limit = 40
		}
		for _, e := range els {
			if e.Component && (len(e.Props) > 0 || len(e.Hooks) > 0) {
				total++
				if len(comps) < limit {
					comps = append(comps, attachComponent(e))
				}
			}
		}
	}
	if len(comps) == 0 {
		o.head = "No component of the window has props or hooks."
		if el != 0 {
			o.head = "No component rendered that element."
		}
		return o.result()
	}
	var b strings.Builder
	for i := range comps {
		writeComponent(&b, &comps[i], el != 0)
	}
	if el != 0 {
		o.head = "The component that rendered the element (the nearest one above it); newest render first."
	} else {
		o.head = fmt.Sprintf("%d components with props or hooks, in tree order; give a ref for the one that rendered an element.", total)
		if total > len(comps) {
			o.head += fmt.Sprintf(" The first %d are shown: max raises the limit.", len(comps))
		}
	}
	o.sections = append(o.sections, strings.TrimRight(b.String(), "\n"))
	return o.result()
}

// logLevel names a slog level as the logs tool does.
func logLevel(l int) string {
	switch {
	case l >= 8:
		return "error"
	case l >= 4:
		return "warn"
	}
	return "info"
}

func (s *attachSession) toolLogs(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
		Level string `json:"level"`
		All   bool   `json:"all"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	rank := map[string]int{"": 0, "info": 0, "warn": 1, "error": 2}
	if _, ok := rank[a.Level]; !ok {
		o.fail(fmt.Sprintf(`level %q: use "info" (everything), "warn" (warnings and errors) or "error"`, a.Level))
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	since := s.logRead
	if a.All {
		since = 0
	}
	es, last, err := s.conn.Logs(ctx, since)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	s.logRead, s.logSeen = max(s.logRead, last), max(s.logSeen, last)
	var lines []string
	for _, e := range es {
		lvl := logLevel(int(e.Level))
		if rank[lvl] < rank[a.Level] {
			continue
		}
		src := e.Source
		if src == "app" {
			src = "dev"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", lvl, src, e.Msg))
	}
	if len(lines) == 0 {
		o.head = "No new log entries since the last logs call."
		if a.All {
			o.head = "The log is empty."
		}
		if a.Level != "" && a.Level != "info" {
			o.head = strings.TrimSuffix(o.head, ".") + " at level " + a.Level + " or above."
		}
		return o.result()
	}
	const show = 100
	earlier := 0
	if len(lines) > show {
		earlier = len(lines) - show
		lines = lines[earlier:]
	}
	text := strings.Join(lines, "\n")
	if earlier > 0 {
		text = fmt.Sprintf("(%d earlier entries left out)\n", earlier) + text
	}
	o.head = fmt.Sprintf("%d log entries: dev warnings and errors of Tela (dev), and what the app wrote on its standard error (stderr).", len(lines))
	o.sections = append(o.sections, "```\n"+text+"\n```")
	return o.result()
}

func (s *attachSession) toolPerf(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		attachArgs
		Frames int  `json:"frames"`
		Action bool `json:"action"`
	}
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if a.Frames < 0 || a.Frames > 200 {
		o.fail("frames must be between 1 and 200")
		return o.result()
	}
	if a.Frames == 0 {
		a.Frames = 10
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	w, err := s.window(ctx, a.Window)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	ps, err := s.conn.Profiles(ctx, w.ID, 0)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	now := 0
	if len(ps) > 0 {
		now = ps[len(ps)-1].Frame
	}
	what := fmt.Sprintf("the last %d requested", a.Frames)
	if a.Action {
		ps = s.win(w.ID).action
		what = "the frames the last acting call caused"
	} else if len(ps) > a.Frames {
		ps = ps[len(ps)-a.Frames:]
	}
	if len(ps) == 0 {
		o.head = fmt.Sprintf("No frames in %s (the app is at frame %d: a window that changes nothing draws none).", what, now)
		return o.result()
	}
	fs := profileFrames(ps)
	o.head = perfSummary(fs, what)
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "- frame %d: %s", f.Number, millis(f.Duration))
		if parts := topPhases(f.Phases, 4); len(parts) > 0 {
			b.WriteString(" (" + strings.Join(parts, ", ") + ")")
		}
		fmt.Fprintf(&b, "; built %d, styled %d, laid out %d, repainted %d px\n", f.Built, f.Styled, f.LaidOut, f.Damage)
	}
	o.sections = append(o.sections, "```\n"+strings.TrimRight(b.String(), "\n")+"\n```\nTimes are the real window's on this machine while tela dev instruments it (the instrumentation adds a little); compare frames with each other, and read the counts as the exact part.")
	return o.result()
}

// profileFrames turns the frame profiles of the app into the frames the
// perf summary reads.
func profileFrames(ps []core.FrameProfile) []mcpFrame {
	fs := make([]mcpFrame, len(ps))
	for i, p := range ps {
		f := mcpFrame{Number: p.Frame, Duration: p.Duration, Phases: map[string]time.Duration{}, Built: p.Built, Styled: p.Styled, LaidOut: p.LaidOut, Damage: p.Damage}
		for ph, d := range p.Phases {
			if d > 0 {
				f.Phases[core.Phase(ph).String()] = d
			}
		}
		fs[i] = f
	}
	return fs
}

func (s *attachSession) toolLint(ctx context.Context, args json.RawMessage) mcpResult {
	var o mcpOut
	var a attachArgs
	if err := decodeArgs(args, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	if !s.begin(ctx, &o) {
		return o.result()
	}
	// The package's directory, from a tela dev of this Tela; an older one
	// gives only the package as it was given, relative to where it ran.
	opts := lint.Options{Patterns: []string{s.info.Pkg}}
	pkg := s.info.Pkg
	if s.info.Dir != "" {
		opts = lint.Options{Dir: s.info.Dir, Patterns: []string{"."}, AbsPaths: true}
		pkg = s.info.Dir
	} else if pkg == "" {
		pkg, opts.Patterns = ".", []string{"."}
	}
	res, err := lint.Run(opts)
	if err != nil {
		o.fail(fmt.Sprintf("tela lint %s: %v (the package is the one tela dev was given; run tela mcp --attach in the same directory)", pkg, err))
		return o.result()
	}
	var b strings.Builder
	if err := lint.WriteJSON(&b, res.Diagnostics); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	switch n := len(res.Diagnostics); n {
	case 0:
		o.head = "tela lint found nothing."
	case 1:
		o.head = "tela lint found 1 problem:"
	default:
		o.head = fmt.Sprintf("tela lint found %d problems:", n)
	}
	o.sections = append(o.sections, "```json\n"+strings.TrimRight(b.String(), "\n")+"\n```")
	return o.result()
}
