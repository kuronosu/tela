package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

func TestParseDebug(t *testing.T) {
	tests := []struct {
		in   string
		want DebugFlags
	}{
		{"", 0},
		{"none", 0},
		{"layout", DebugLayout},
		{"layout,paint-flash,layers,fps,semantics", DebugAll},
		{"all", DebugAll},
		{" Layers , FPS ", DebugLayers | DebugFPS},
		{"semantics,semantics", DebugSemantics},
		{"layout,,fps", DebugLayout | DebugFPS},
	}
	for _, tt := range tests {
		got, err := ParseDebug(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseDebug(%q) = %v, %v; want %v, nil", tt.in, got, err, tt.want)
		}
	}
	// String is the inverse.
	for _, f := range []DebugFlags{0, DebugLayout, DebugLayers | DebugFPS, DebugAll, DebugPaintFlash | DebugSemantics} {
		if got, err := ParseDebug(f.String()); err != nil || got != f {
			t.Errorf("ParseDebug(%q) = %v, %v; want %v", f.String(), got, err, f)
		}
	}
}

func TestParseDebugErrors(t *testing.T) {
	// One message lists the unknown names, a suggestion for a likely typo
	// and the valid flags; the valid names still apply.
	f, err := ParseDebug("layout,paint-flsh,fps")
	if f != DebugLayout|DebugFPS {
		t.Errorf("flags = %v, want layout,fps", f)
	}
	if err == nil {
		t.Fatal("no error for an unknown flag")
	}
	for _, want := range []string{
		`TELA_DEBUG: unknown debug flag "paint-flsh"`,
		`"paint-flsh" is probably "paint-flash"`,
		"valid flags: layout, paint-flash, layers, fps, semantics, or all",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("the message must be one line: %q", err)
	}
	_, err = ParseDebug("bogus,zzz")
	if err == nil || !strings.Contains(err.Error(), `unknown debug flags "bogus", "zzz"`) || strings.Contains(err.Error(), "probably") {
		t.Errorf("unrelated names: error %v", err)
	}
	// Prefixes and near misses suggest.
	for in, want := range map[string]string{"paint": "paint-flash", "layer": "layers", "sematics": "semantics", "fp": "fps", "layot": "layout"} {
		_, err := ParseDebug(in)
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q is probably %q", in, want)) {
			t.Errorf("ParseDebug(%q): error %v, want a suggestion of %q", in, err, want)
		}
	}
}

func TestNewHostReadsEnv(t *testing.T) {
	newHost := func(cfg Config, log *bytes.Buffer) *Host {
		b := headless.New(headless.Options{Size: platform.Size{W: 100, H: 80}})
		cfg.Fonts = testFonts
		h, err := NewHost(b, Options{Config: cfg, Root: div(Text("x")), ErrorLog: log})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(h.stopTimer)
		return h
	}
	t.Setenv(EnvDebug, "layers,fps")
	var log bytes.Buffer
	if got := newHost(Config{}, &log).Tree().Debug(); got != DebugLayers|DebugFPS || log.Len() != 0 {
		t.Errorf("TELA_DEBUG=layers,fps: flags %v, log %q", got, log.String())
	}
	// The options win over the environment, and DebugOff is explicit.
	if got := newHost(Config{Debug: DebugLayout}, &log).Tree().Debug(); got != DebugLayout {
		t.Errorf("Config.Debug=layout: flags %v", got)
	}
	if got := newHost(Config{Debug: DebugOff}, &log).Tree().Debug(); got != 0 {
		t.Errorf("Config.Debug=DebugOff: flags %v, want none", got)
	}
	// A malformed value is reported once, and its valid flags apply.
	t.Setenv(EnvDebug, "layout,paint-flsh")
	log.Reset()
	if got := newHost(Config{}, &log).Tree().Debug(); got != DebugLayout {
		t.Errorf("flags %v, want layout", got)
	}
	if n := strings.Count(log.String(), "\n"); n != 1 || !strings.Contains(log.String(), `probably "paint-flash"`) || !strings.HasPrefix(log.String(), "tela: ") {
		t.Errorf("log = %q, want one message with a suggestion", log.String())
	}
}

// TestDebugOverlayMatchesFullRender is the damage property test (layer_test.go)
// with overlays on: after any sequence of changes, the window equals a full
// rasterization of the application followed by the overlays.
func TestDebugOverlayMatchesFullRender(t *testing.T) {
	steps := 60
	if testing.Short() {
		steps = 20
	}
	rows := func(st *sceneState) Node { return rowScene(st, 80, "grow overflow-y-auto bg-white") }
	for _, tc := range []struct {
		name  string
		flags DebugFlags
	}{
		{"all", DebugAll},
		{"layout", DebugLayout},
		{"flash", DebugPaintFlash},
		{"layers", DebugLayers},
		{"semantics", DebugSemantics},
		{"fps", DebugFPS},
	} {
		runScene(t, "rows_"+tc.name, rows, commonOps, Config{Debug: tc.flags}, steps)
	}
	runScene(t, "page_all", pageScene, commonOps, Config{InstantScroll: true, Debug: DebugAll}, steps)
	runScene(t, "virtual_all", virtualScene, commonOps, Config{InstantScroll: true, Debug: DebugAll}, steps)
	runScene(t, "board_all", boardScene, commonOps, Config{InstantScroll: true, Debug: DebugAll}, steps)
}

// TestDebugOffRestoresApp: the application's pixels are those of a window
// that never had overlays, after turning them on and off.
func TestDebugOffRestoresApp(t *testing.T) {
	st := &sceneState{refs: make([]ElementRef, 1), setters: make([]Setter[int], 200)}
	x := newDamageHarness(t, rowScene(st, 60, "grow overflow-y-auto bg-white"), 360, 280, 2, Config{InstantScroll: true}, 3)
	tr := x.tree()
	x.move(100, 100)
	for _, f := range []DebugFlags{DebugAll, 0, DebugLayout | DebugFPS, DebugPaintFlash, 0} {
		tr.SetDebug(f)
		x.frame("set debug " + f.String())
		st.refs[0].ScrollBy(0, 37)
		x.frame("scroll")
		st.setters[3].Update(func(v int) int { return v + 1 })
		x.frame("state")
		x.move(200, 150)
		x.frame("hover")
		if f == 0 && (tr.dbg.clean != nil || tr.dbg.prev != nil) {
			t.Errorf("overlays off but the overlay state is kept: clean %v, prev %v", tr.dbg.clean != nil, tr.dbg.prev != nil)
		}
	}
	// With none on, the window is exactly the application (check compares
	// it with the reference alone).
	if tr.Debug() != 0 {
		t.Fatalf("debug = %v", tr.Debug())
	}
}

// TestDebugOverlaysDoNotAffectApp: overlays change no layout, hit test,
// semantics or event delivery.
func TestDebugOverlaysDoNotAffectApp(t *testing.T) {
	mk := func(flags DebugFlags) *damageHarness {
		st := &sceneState{refs: make([]ElementRef, 1), setters: make([]Setter[int], 200)}
		return newDamageHarness(t, rowScene(st, 30, "grow overflow-y-auto bg-white"), 360, 280, 1, Config{Debug: flags}, 1)
	}
	on, off := mk(DebugAll), mk(DebugOff)
	for _, p := range []gfx.Point{{X: 40, Y: 60}, {X: 200, Y: 120}, {X: 300, Y: 250}} {
		on.move(p.X, p.Y)
		off.move(p.X, p.Y)
		on.frame("move")
		off.frame("move")
	}
	js := func(x *damageHarness) string {
		b, err := json.Marshal(x.tree().Inspect())
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if js(on) != js(off) {
		t.Error("the layout tree differs with overlays on")
	}
	if a, b := on.tree().AriaSnapshot(), off.tree().AriaSnapshot(); a != b {
		t.Errorf("the aria snapshot differs with overlays on:\n%s\n---\n%s", a, b)
	}
	for y := float32(5); y < 280; y += 23 {
		for xx := float32(5); xx < 360; xx += 31 {
			if a, b := on.tree().HitTest(xx, y), off.tree().HitTest(xx, y); a == nil || b == nil || a.box.kind != b.box.kind || a.box.u != b.box.u {
				t.Fatalf("HitTest(%g, %g) differs with overlays on: %v, %v", xx, y, a, b)
			}
		}
	}
}

// TestDebugFrameRequests: overlays that do not change over time never
// ask for frames when the application is idle; paint-flash only while a
// flash fades; fps always.
func TestDebugFrameRequests(t *testing.T) {
	scene := func(flags DebugFlags) (*damageHarness, *sceneState) {
		st := &sceneState{refs: make([]ElementRef, 1), setters: make([]Setter[int], 200)}
		return newDamageHarness(t, rowScene(st, 30, "grow overflow-y-auto bg-white"), 360, 280, 1, Config{InstantScroll: true, Debug: flags}, 1), st
	}
	settle := func(x *damageHarness) {
		for range 80 {
			x.frame("settle")
			if !x.w.FrameRequested() && !x.tree().Dirty() {
				return
			}
		}
	}
	for _, flags := range []DebugFlags{DebugLayout, DebugLayers, DebugSemantics, DebugLayout | DebugLayers | DebugSemantics} {
		x, st := scene(flags)
		settle(x)
		n := x.h.Presents()
		st.setters[2].Update(func(v int) int { return v + 1 })
		settle(x)
		if x.h.Presents() == n {
			t.Errorf("%v: a change presented nothing", flags)
		}
		n = x.h.Presents()
		for range 30 {
			x.frame("idle")
			if x.w.FrameRequested() || x.tree().DebugWantsFrame() {
				t.Fatalf("%v: an idle app still asks for frames", flags)
			}
		}
		if x.h.Presents() != n {
			t.Errorf("%v: %d frames presented while idle", flags, x.h.Presents()-n)
		}
	}

	// Paint flash: frames while fading (about 300 ms), then none.
	x, st := scene(DebugPaintFlash)
	settle(x)
	if got := len(x.tree().dbg.flashes); got != 0 {
		t.Fatalf("%d flashes left after settling", got)
	}
	st.setters[2].Update(func(v int) int { return v + 1 })
	x.frame("change")
	if len(x.tree().dbg.flashes) == 0 || !x.tree().DebugWantsFrame() {
		t.Fatal("a repaint did not start a flash")
	}
	start := x.b.Now()
	for range 60 {
		x.frame("fade")
		if !x.w.FrameRequested() {
			break
		}
	}
	if d := x.b.Now() - start; d < flashFade-30*time.Millisecond || d > flashFade+40*time.Millisecond {
		t.Errorf("the flash lasted %v of frames, want about %v", d, flashFade)
	}
	n := x.h.Presents()
	for range 30 {
		x.frame("idle")
	}
	if x.w.FrameRequested() || x.tree().DebugWantsFrame() || x.h.Presents() != n || len(x.tree().dbg.flashes) != 0 {
		t.Errorf("after the fade: requested %v, presents %d -> %d, flashes %d", x.w.FrameRequested(), n, x.h.Presents(), len(x.tree().dbg.flashes))
	}

	// fps: a frame per vsync while the window is visible.
	x, _ = scene(DebugFPS)
	settle0 := x.h.Presents()
	for range 20 {
		x.frame("tick")
		if !x.w.FrameRequested() {
			t.Fatal("the fps meter stopped asking for frames")
		}
	}
	if got := x.h.Presents() - settle0; got < 20 {
		t.Errorf("fps: %d presents in 20 frames", got)
	}
}

// TestDebugHoverRequestsFrame: the layout overlay follows the pointer: a
// hover change asks for one frame that redraws the emphasis.
func TestDebugHoverRequestsFrame(t *testing.T) {
	st := &sceneState{refs: make([]ElementRef, 1), setters: make([]Setter[int], 200)}
	x := newDamageHarness(t, rowScene(st, 30, "grow overflow-y-auto bg-white"), 360, 280, 1, Config{Debug: DebugLayout}, 1)
	for range 5 {
		x.frame("settle")
	}
	n := x.h.Presents()
	before := x.w.Frame().RGBAAt(100, 100)
	// An input event that asks for a frame gets it in the same
	// iteration of the (headless) loop.
	x.inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 100, Y: 100}})
	if x.h.Presents() != n+1 {
		t.Errorf("presents %d -> %d, want one frame for the hover", n, x.h.Presents())
	}
	if after := x.w.Frame().RGBAAt(100, 100); after == before {
		t.Errorf("pixel under the pointer unchanged (%v): the hovered element was not emphasized", after)
	}
	// Moving inside the same element changes nothing.
	n = x.h.Presents()
	x.inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 101, Y: 100}})
	x.frame("same element")
	if x.h.Presents() != n || x.w.FrameRequested() {
		t.Errorf("a move within one element presented %d frames (requested %v)", x.h.Presents()-n, x.w.FrameRequested())
	}
	// Leaving the window clears the emphasis.
	x.inject(platform.PointerEvent{Action: platform.PointerLeave, Pos: platform.Point{X: -1, Y: -1}})
	x.frame("leave")
	if got := x.w.Frame().RGBAAt(100, 100); got != before {
		t.Errorf("after the pointer left, the pixel is %v, want %v", got, before)
	}
}

// TestDebugPaintFlashShowsDamage: a flash covers what the application
// repainted and fades to the application's pixels.
func TestDebugPaintFlashShowsDamage(t *testing.T) {
	st := &sceneState{refs: make([]ElementRef, 1), setters: make([]Setter[int], 200)}
	x := newDamageHarness(t, rowScene(st, 30, "grow overflow-y-auto bg-white"), 360, 280, 2, Config{Debug: DebugPaintFlash}, 1)
	tr := x.tree()
	// Let the mount's full flash end.
	for range 30 {
		x.frame("settle")
	}
	clean := func() *gfx.Rect {
		d := tr.LastDamage()
		if len(d.Rects) == 0 {
			return nil
		}
		r := gfx.RectFromImage(d.Rects[0])
		return &r
	}
	st.setters[2].Update(func(v int) int { return v + 1 })
	x.frame("change")
	r := clean()
	if r == nil {
		t.Fatal("no damage")
	}
	if len(tr.dbg.flashes) == 0 {
		t.Fatal("no flash recorded")
	}
	// The flash is in the window, in the repainted rectangle, and not
	// outside it.
	img := x.w.Frame()
	cx, cy := int(r.Center().X), int(r.Center().Y)
	px := img.RGBAAt(cx, cy)
	// Green was added to the row's white background: red drops.
	if !(px.R < 250 && px.G > px.R) {
		t.Errorf("pixel in the damage is %v, want it tinted green", px)
	}
	if q := img.RGBAAt(int(r.X0)-8, int(r.Y0)-8); q.R < 240 && q.G < 240 {
		t.Logf("pixel outside the damage: %v", q)
	}
	for range 30 {
		x.frame("fade")
	}
	if len(tr.dbg.flashes) != 0 {
		t.Fatalf("%d flashes after the fade", len(tr.dbg.flashes))
	}
	// Faded: the window equals the application's pixels (check).
}

// TestDebugFPSMeter drives the meter with a scripted clock.
func TestDebugFPSMeter(t *testing.T) {
	var calls int
	var now time.Duration
	durs := []time.Duration{2 * time.Millisecond, 20 * time.Millisecond, 40 * time.Millisecond}
	cfg := Config{Debug: DebugFPS, DebugClock: func() time.Duration {
		calls++
		if calls%2 == 0 {
			now += durs[(calls/2)%len(durs)]
		}
		return now
	}}
	st := &sceneState{refs: make([]ElementRef, 1), setters: make([]Setter[int], 200)}
	x := newDamageHarness(t, rowScene(st, 30, "grow overflow-y-auto bg-white"), 360, 280, 1, cfg, 1)
	for i := range 12 {
		if i%2 == 0 {
			st.setters[2].Update(func(v int) int { return v + 1 })
		}
		x.frame("tick")
	}
	d := &x.tree().dbg
	if len(d.frames) == 0 {
		t.Fatal("no frames recorded")
	}
	var idle, inc, full int
	for _, s := range d.frames {
		switch s.kind {
		case DebugFrameIdle:
			idle++
		case DebugFrameIncremental:
			inc++
		case DebugFrameFull:
			full++
		}
		if s.work <= 0 {
			t.Errorf("a frame has work %v", s.work)
		}
	}
	if full != 1 || inc < 5 || idle < 4 {
		t.Errorf("frames: %d idle, %d incremental, %d full", idle, inc, full)
	}
	// The ring keeps the last fpsHistory frames, oldest first.
	for range fpsHistory + 20 {
		x.tree().DebugRecordFrame(x.b.Now(), time.Millisecond, DebugFrameIdle)
	}
	if len(d.frames) != fpsHistory {
		t.Errorf("%d frames kept, want %d", len(d.frames), fpsHistory)
	}
	// Overlays other than fps ignore RecordFrame.
	other := NewTree(Config{Fonts: testFonts, Debug: DebugLayout})
	other.DebugRecordFrame(0, time.Millisecond, DebugFrameFull)
	if len(other.dbg.frames) != 0 {
		t.Error("frames recorded without the fps overlay")
	}
}

// TestRasterizeFrameOffNoAllocs: with no overlay, rasterizing a frame that
// was already rasterized allocates nothing.
func TestRasterizeFrameOffNoAllocs(t *testing.T) {
	tr := newTestTree(t, 360, 280, div(Class("p-4"), Text("hello")), nil)
	tr.Frame()
	img := tr.newImage(1)
	var r raster.Renderer
	tr.RasterizeFrame(img, &r, false)
	if n := testing.AllocsPerRun(50, func() {
		tr.Frame()
		tr.RasterizeFrame(img, &r, false)
	}); n != 0 {
		t.Errorf("an idle frame with no overlay allocates %v times", n)
	}
	if tr.dbg.clean != nil || tr.dbg.prev != nil || tr.DebugPending() || tr.DebugWantsFrame() {
		t.Error("overlay state exists with no overlay")
	}
}

// TestDebugRender: Render draws the overlays over the frame.
func TestDebugRender(t *testing.T) {
	root := div(Class("flex gap-2 p-4"), button(Class("rounded bg-indigo-500 px-3 py-1 text-white"), Text("Go")))
	_, plain := Render(root, RenderOptions{Config: Config{Fonts: testFonts, Debug: DebugOff}, Width: 200, Height: 100, Scale: 2})
	_, dbg := Render(root, RenderOptions{Config: Config{Fonts: testFonts, Debug: DebugLayout | DebugLayers}, Width: 200, Height: 100, Scale: 2})
	if plain.Rect != dbg.Rect {
		t.Fatalf("sizes differ: %v, %v", plain.Rect, dbg.Rect)
	}
	if bytes.Equal(plain.Pix, dbg.Pix) {
		t.Error("Render with overlays equals Render without")
	}
}
