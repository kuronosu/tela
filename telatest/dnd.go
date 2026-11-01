package telatest

import (
	"fmt"
	"image"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// Drag and drop in tests (ADR-211). The headless backend plays the part of
// the operating system: another application's drag is started with data
// and allowed effects and moved with pointer events, and a drag the app
// started from an h.Draggable element is moved the same way, over this
// window, another window of the app, or out of the app. What the app
// answers is what a user would see: the OnDragEnter, OnDragOver,
// OnDragLeave and OnDrop handlers run, and the source's OnDragEnd.
//
// The helpers are small steps that compose, like a user's hand:
//
//	StartDrag(draggable)           press and move until the app starts a drag
//	HoverDrag(target)              move the drag over an element
//	LeaveDrag() / CancelDrag()     leave the window / press Escape
//	DropOn(target) / DropOutside   release over an element / over another application
//
// and the common whole drags in one call: [App.DragTo], [App.DragToWindow],
// [App.DropData] and [App.DropFiles]. The steps that move a drag also start
// another application's drag when given [Carrying].

// DragPayload is the data another application drags: files, text, HTML, a
// PNG image or a custom type, in one or more representations. Build it
// with [DragFiles], [DragText], [DragHTML], [DragPNG] and [DragType], and
// combine representations, best first, with [DragPayload.And].
type DragPayload struct {
	data platform.Data
	err  error
}

// payloadOf builds a payload with the setters of tela.DataTransfer, so the
// types are normalized like the app's own.
func payloadOf(set func(dt *tela.DataTransfer) error) DragPayload {
	var dt tela.DataTransfer
	if err := set(&dt); err != nil {
		return DragPayload{err: err}
	}
	return DragPayload{data: core.TransferData(&dt)}
}

// DragText is plain text (the "text/plain" type).
func DragText(s string) DragPayload {
	return payloadOf(func(dt *tela.DataTransfer) error { dt.SetText(s); return nil })
}

// DragHTML is an HTML fragment (the "text/html" type). Add [DragText] with
// [DragPayload.And] for applications that read only text.
func DragHTML(html string) DragPayload {
	return payloadOf(func(dt *tela.DataTransfer) error { dt.SetHTML(html); return nil })
}

// DragFiles is a list of files, as a file manager drags them: the app
// reads their paths with DataTransfer.Files. The paths must be absolute
// (use t.TempDir()); the files need not exist.
func DragFiles(paths ...string) DragPayload {
	return payloadOf(func(dt *tela.DataTransfer) error { return dt.SetFiles(paths...) })
}

// DragPNG is an image, carried as "image/png".
func DragPNG(img image.Image) DragPayload {
	if img == nil {
		panic("telatest.DragPNG(nil): pass the image to drag")
	}
	return payloadOf(func(dt *tela.DataTransfer) error { return dt.SetImage(img) })
}

// DragType is data of a custom MIME type, such as "application/x-card"
// (HTML's setData). It panics when mime is not a MIME type.
func DragType(mime string, data string) DragPayload {
	return payloadOf(func(dt *tela.DataTransfer) error { dt.SetData(mime, data); return nil })
}

// And returns the payload with the representations of other added after
// its own. It panics when both carry the same type.
//
//	telatest.DragHTML("<b>Hi</b>").And(telatest.DragText("Hi"))
func (p DragPayload) And(other DragPayload) DragPayload {
	if p.err != nil {
		return p
	}
	if other.err != nil {
		return other
	}
	out := p.data.Clone()
	for _, e := range other.data {
		if out.Has(e.Type) {
			panic(fmt.Sprintf("telatest: DragPayload.And: both payloads carry %q; give each type once", e.Type))
		}
		out = append(out, platform.DataEntry{Type: e.Type, Bytes: append([]byte(nil), e.Bytes...)})
	}
	return DragPayload{data: out}
}

// dndConfig is what drag and drop adds to [DragOption]'s Steps.
type dndConfig struct {
	payload    *DragPayload
	allowed    tela.DropEffect
	allowedSet bool
	modifiers  []string
}

// Carrying makes the step start another application's drag of payload
// when no drag is in progress, instead of failing. [App.HoverDrag] and
// [App.DropOn] take it; [App.DropData] and [App.DropFiles] are DropOn with
// it.
func Carrying(payload DragPayload) DragOption {
	return func(c *dragConfig) { c.dnd.payload = &payload }
}

// Allowed sets the effects the other application allows for the drag that
// [Carrying] starts (default: copy, move and link). The app's
// DataTransfer.EffectAllowed in its handlers reports them, and a drop
// refuses an effect outside them.
func Allowed(effects ...tela.DropEffect) DragOption {
	return func(c *dragConfig) {
		c.dnd.allowedSet = true
		c.dnd.allowed = 0
		for _, e := range effects {
			c.dnd.allowed |= e
		}
	}
}

// Modifiers holds keys during the step, with Playwright's names ("Shift",
// "Control", "Alt", "Meta", "ControlOrMeta"). The platform's convention
// picks the default DropEffect from them: Control copies, Shift moves,
// Control+Shift links.
func Modifiers(keys ...string) DragOption {
	return func(c *dragConfig) { c.dnd.modifiers = append(c.dnd.modifiers, keys...) }
}

// dragConfigOf applies opts and resolves the modifiers they hold.
func (a *App) dragConfigOf(opts []DragOption) (cfg dragConfig, mods platform.Modifiers) {
	a.tb.Helper()
	cfg = dragConfig{steps: 12}
	for _, o := range opts {
		o(&cfg)
	}
	cfg.steps = max(cfg.steps, 1)
	for _, name := range cfg.dnd.modifiers {
		m, ok := modifierNamed(name)
		if !ok {
			a.fatalf("Modifiers(%q): not a modifier; use \"Shift\", \"Control\", \"Alt\", \"Meta\" or \"ControlOrMeta\"", name)
		}
		mods |= m.mod
	}
	return cfg, mods
}

// DragInfo describes the drag in progress ([App.ActiveDrag]).
type DragInfo struct {
	// Data is a copy of what the drag carries, with every getter open
	// (Types, Text, HTML, Files, Image, GetData...). Its EffectAllowed is
	// Allowed.
	Data *tela.DataTransfer
	// Allowed are the effects the source allows.
	Allowed tela.DropEffect
	// External: another application's drag. Source is the window of the
	// app that started it, nil for an external drag.
	External bool
	Source   *App
	// Over is the window the drag is over, nil while it is over none.
	Over *App
}

// ActiveDrag returns the drag in progress: the one [App.StartDrag] began
// from an h.Draggable element, or another application's ([Carrying]),
// until a drop, a cancel or [App.DropOutside].
//
//	app.StartDrag(telatest.ByTestID("card-7"))
//	info, _ := app.ActiveDrag()
//	if got := info.Data.GetData("application/x-card"); got != "7" { ... }
func (a *App) ActiveDrag() (DragInfo, bool) {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	return a.dragInfo()
}

func (a *App) dragInfo() (DragInfo, bool) {
	d, ok := a.backend.Drag()
	if !ok {
		return DragInfo{}, false
	}
	dt := core.NewDataTransfer(d.Data)
	dt.EffectAllowed = d.Allowed
	info := DragInfo{Data: dt, Allowed: d.Allowed, External: d.Source == nil}
	if d.Source != nil {
		info.Source = a.view(d.Source)
	}
	if d.Over != nil {
		info.Over = a.view(d.Over)
	}
	return info, true
}

// mustDrag returns the drag in progress or fails the test.
func (a *App) mustDrag(what string) DragInfo {
	a.tb.Helper()
	info, ok := a.dragInfo()
	if !ok {
		a.fatalf("%s: no drag is in progress; start one with StartDrag(draggable), or with the Carrying option (another application's drag)", what)
	}
	return info
}

// forgetPointers drops what the windows knew of the mouse: a drag owns the
// pointer, so after it ends the next pointer action enters again.
func (a *App) forgetPointers() {
	for _, v := range a.sh.views {
		v.hasPos, v.held = false, 0
	}
}

// StartDrag presses the primary button at the center of the element l
// locates and moves the pointer away until the app starts a drag, then
// leaves the drag in progress, owned by the "OS" as a real drag is: the
// app gets no more pointer events until it ends. Continue it with
// [App.HoverDrag], [App.DropOn], [App.DropOutside], [App.LeaveDrag] or
// [App.CancelDrag]. It returns what the drag carries, as [App.ActiveDrag]
// does.
//
// The element must be h.Draggable(true) and its OnDragStart must put data
// in e.DataTransfer; an element without data stays an ordinary drag
// gesture, and StartDrag fails the test.
//
//	app.StartDrag(telatest.ByTestID("card-7"))
//	app.HoverDrag(telatest.ByTestID("column-done"))
//	app.ExpectText("Release to move here") // the drag-over state of the column
//	app.DropOn(telatest.ByTestID("column-done"))
func (a *App) StartDrag(l Locator) DragInfo {
	a.tb.Helper()
	a.mustBeOpen()
	if _, ok := a.backend.Drag(); ok {
		a.fatalf("StartDrag: a drag is already in progress; end it first with DropOn, DropOutside or CancelDrag")
	}
	n, p := a.target(l, "drag", true)
	a.moveTo(p)
	a.down()
	// Move away in small steps toward the middle of the window, so the
	// pointer stays inside it, until the slop is passed.
	vp := a.tree.Viewport()
	var sx, sy float32
	if c := vp.W / 2; p.X < c-4 {
		sx = 1
	} else if p.X > c+4 {
		sx = -1
	}
	if c := vp.H / 2; p.Y < c-4 {
		sy = 1
	} else if p.Y > c+4 {
		sy = -1
	}
	if sx == 0 && sy == 0 {
		sx = 1
	}
	const step, tries = 4, 8
	for i := 1; i <= tries; i++ {
		a.moveTo(platform.Point{X: p.X + sx*step*float32(i), Y: p.Y + sy*step*float32(i)})
		if _, ok := a.backend.Drag(); ok {
			a.forgetPointers()
			info, _ := a.dragInfo()
			return info
		}
	}
	a.up()
	a.fatalf("StartDrag: pressing %s and moving %d px did not start a drag; the element must be h.Draggable(true) and its OnDragStart must put data in e.DataTransfer "+
		"(SetText, SetData, SetFiles...) without calling PreventDefault; an element without data stays a plain drag gesture",
		describeNode(n), step*tries)
	return DragInfo{}
}

// beginExternal starts another application's drag when the options carry
// a payload, and checks the drag state against what the step needs.
func (a *App) beginExternal(what string, cfg dragConfig) {
	a.tb.Helper()
	_, running := a.backend.Drag()
	switch {
	case cfg.dnd.payload == nil && !running:
		a.fatalf("%s: no drag is in progress; start one with StartDrag(draggable), or give Carrying(payload) to drag data from another application", what)
	case cfg.dnd.payload != nil && running:
		a.fatalf("%s: Carrying(...) starts a drag, but one is already in progress; drop it, cancel it (CancelDrag) or leave Carrying out to continue it", what)
	case cfg.dnd.payload == nil:
		if cfg.dnd.allowedSet {
			a.fatalf("%s: Allowed(...) applies to the drag Carrying(...) starts, but the drag in progress is already started", what)
		}
		return
	}
	p := cfg.dnd.payload
	if p.err != nil {
		a.fatalf("%s: the payload is not valid: %v", what, p.err)
	}
	if len(p.data) == 0 {
		a.fatalf("%s: the payload is empty; build it with DragFiles, DragText, DragHTML, DragPNG or DragType", what)
	}
	allowed := tela.DropCopy | tela.DropMove | tela.DropLink
	if cfg.dnd.allowedSet {
		allowed = cfg.dnd.allowed
	}
	if allowed == tela.DropNone || allowed&^tela.DropAll != 0 {
		a.fatalf("%s: Allowed(%v) cannot start a drag; allow at least one of tela.DropCopy, DropMove and DropLink", what, allowed)
	}
	a.backend.StartExternalDrag(p.data, allowed)
}

// pointerOfDrag injects a pointer event the "OS" turns into a move or a
// release of the drag, and settles the app.
func (a *App) pointerOfDrag(act platform.PointerAction, p platform.Point, mods platform.Modifiers) {
	a.tb.Helper()
	e := platform.PointerEvent{Action: act, Pos: p, Mods: mods}
	if act == platform.PointerUp {
		e.Button = platform.ButtonPrimary
	}
	a.win.Inject(e)
	a.settleAfter()
}

// dragMoveTo moves the drag over p of this window, in steps from where it
// is when it is already over this window.
func (a *App) dragMoveTo(p platform.Point, steps int, mods platform.Modifiers) {
	a.tb.Helper()
	if d, ok := a.backend.Drag(); ok && d.Over == a.win && steps > 1 {
		from := d.Pos
		for i := 1; i < steps; i++ {
			f := float32(i) / float32(steps)
			a.pointerOfDrag(platform.PointerMove, platform.Point{X: from.X + (p.X-from.X)*f, Y: from.Y + (p.Y-from.Y)*f}, mods)
		}
	}
	a.pointerOfDrag(platform.PointerMove, p, mods)
}

// HoverDrag moves the drag in progress over the center of the element l
// locates and leaves it there, so the app can show its drag-over state:
// OnDragEnter on the element, OnDragLeave on the one it left, OnDragOver.
// The pointer moves in steps (default 12; [Steps]) when the drag is
// already over this window, and enters it directly otherwise.
//
// With [Carrying] it starts another application's drag first, over no
// window. The window need not be the drag's source: calling HoverDrag on
// the [App] of another window of the app moves a drag between windows.
//
//	app.HoverDrag(telatest.ByTestID("dropzone"), telatest.Carrying(telatest.DragFiles(path)))
//	app.ExpectText("Drop to upload")
//	app.LeaveDrag()
//	app.ExpectNoText("Drop to upload")
func (a *App) HoverDrag(l Locator, opts ...DragOption) {
	a.tb.Helper()
	a.mustBeOpen()
	cfg, mods := a.dragConfigOf(opts)
	_, p := a.target(l, "drag over", false)
	a.beginExternal("HoverDrag", cfg)
	a.dragMoveTo(p, cfg.steps, mods)
}

// LeaveDrag moves the drag in progress out of this window, as the pointer
// leaving it does: OnDragLeave runs. The drag goes on.
func (a *App) LeaveDrag() {
	a.tb.Helper()
	a.mustBeOpen()
	a.mustDrag("LeaveDrag")
	a.win.Inject(platform.PointerEvent{Action: platform.PointerLeave})
	a.settleAfter()
}

// CancelDrag cancels the drag in progress, as the user pressing Escape
// does: OnDragLeave runs on the window it is over, and the source's
// OnDragEnd runs with DropNone.
func (a *App) CancelDrag() {
	a.tb.Helper()
	a.mustBeOpen()
	a.mustDrag("CancelDrag")
	a.backend.CancelDrag()
	a.settleAfter()
	a.forgetPointers()
}

// DropOn releases the drag in progress over the center of the element l
// locates, after moving it there as [App.HoverDrag] does, and returns the
// effect the app accepted: tela.DropNone when it did not take the drop,
// tela.DropCopy, DropMove or DropLink when it did. An app takes a drop
// when its latest OnDragOver on the way called PreventDefault with an
// effect the source allows (a drag with [Carrying] allows copy, move and
// link unless [Allowed] says otherwise), and OnDrop did not set DropNone.
// A refused drop runs OnDragLeave instead of OnDrop. The source's
// OnDragEnd runs with the same effect.
//
//	effect := app.DropOn(telatest.ByRole("region", "Inbox"), telatest.Carrying(telatest.DragText("hello")))
//	if effect != tela.DropCopy { t.Fatalf("the inbox did not copy the text: %v", effect) }
func (a *App) DropOn(l Locator, opts ...DragOption) tela.DropEffect {
	a.tb.Helper()
	a.mustBeOpen()
	cfg, mods := a.dragConfigOf(opts)
	_, p := a.target(l, "drop on", false)
	a.beginExternal("DropOn", cfg)
	a.dragMoveTo(p, cfg.steps, mods)
	before := len(a.win.Drops())
	a.pointerOfDrag(platform.PointerUp, p, mods)
	effect := tela.DropNone
	if drops := a.win.Drops(); len(drops) > before {
		effect = drops[len(drops)-1]
	}
	a.forgetPointers()
	return effect
}

// DropData drops payload from another application on the element l
// locates, and returns the effect the app accepted ([App.DropOn]).
//
//	app.DropData(telatest.ByTestID("editor"), telatest.DragHTML("<b>Hi</b>").And(telatest.DragText("Hi")))
func (a *App) DropData(l Locator, payload DragPayload, opts ...DragOption) tela.DropEffect {
	a.tb.Helper()
	return a.DropOn(l, append([]DragOption{Carrying(payload)}, opts...)...)
}

// DropFiles drops files from a file manager on the element l locates, and
// returns the effect the app accepted ([App.DropOn]). The paths must be
// absolute.
//
//	path := filepath.Join(t.TempDir(), "photo.png")
//	if app.DropFiles(telatest.ByTestID("upload"), path) == tela.DropNone { t.Fatal("refused") }
//	app.ExpectText("photo.png")
func (a *App) DropFiles(l Locator, paths ...string) tela.DropEffect {
	a.tb.Helper()
	return a.DropData(l, DragFiles(paths...))
}

// DragTo drags the element from locates, which must be h.Draggable(true),
// to the element to locates in the same window and drops it there, like
// Playwright's locator.dragTo, through the app's drag and drop handlers
// ([App.StartDrag], [App.HoverDrag], [App.DropOn]). It returns the effect
// the target accepted, tela.DropNone when it refused; the source's
// OnDragEnd got the same one.
//
//	app.DragTo(telatest.ByTestID("card-7"), telatest.ByTestID("column-done"))
//	app.ExpectText("Card 7")
//
// For the gesture drag of OnDrag (sliders, movable panels) use
// [App.Drag].
func (a *App) DragTo(from, to Locator, opts ...DragOption) tela.DropEffect {
	a.tb.Helper()
	a.StartDrag(from)
	a.HoverDrag(to, opts...)
	return a.DropOn(to, opts...)
}

// DragToWindow drags the element from locates to the element to locates in
// another window of the app, window (an App from [App.Windows],
// [App.Window] or [App.WaitForWindow]), and drops it there ([App.DragTo]).
//
//	app.DragToWindow(telatest.ByTestID("tab-1"), app.Window("Detached"), telatest.ByTestID("tabs"))
func (a *App) DragToWindow(from Locator, window *App, to Locator, opts ...DragOption) tela.DropEffect {
	a.tb.Helper()
	if window == nil || window.sh != a.sh {
		a.fatalf("DragToWindow: window is not a window of this app; pass an App from Windows(), Window(title) or WaitForWindow")
	}
	a.StartDrag(from)
	window.HoverDrag(to, opts...)
	return window.DropOn(to, opts...)
}

// DropOutside ends the drag the app started over another application,
// which performs effect: the source's OnDragEnd runs with it. Pass
// tela.DropNone for an application that refuses it. effect must be one the
// drag allows (its DataTransfer.EffectAllowed).
//
//	app.StartDrag(telatest.ByTestID("card-7"))
//	app.DropOutside(tela.DropMove)
//	app.ExpectNoText("Card 7") // OnDragEnd removed the moved card
func (a *App) DropOutside(effect tela.DropEffect) {
	a.tb.Helper()
	a.mustBeOpen()
	info := a.mustDrag("DropOutside")
	if info.External {
		a.fatalf("DropOutside: the drag in progress is another application's; only a drag the app started (StartDrag) can end in another application. Use CancelDrag to end it")
	}
	if effect != tela.DropNone && !info.Allowed.Has(effect) {
		a.fatalf("DropOutside(%v): the drag allows only %v (the source's DataTransfer.EffectAllowed)", effect, info.Allowed)
	}
	a.backend.DropOutside(effect)
	a.settleAfter()
	a.forgetPointers()
}
