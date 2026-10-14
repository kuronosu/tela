package main

import (
	"bytes"
	"image"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/golden"
	"github.com/kuronosu/tela/internal/text"
)

// registerCJK adds the test-only CJK subset font (the one the text package's
// goldens use) to c. It is a test dependency only: the example itself embeds
// nothing beyond the default collection and relies on system fonts in a real
// window.
func registerCJK(c *text.Collection) error {
	data, err := os.ReadFile("../../internal/text/testdata/fonts/NotoSansJP-Subset.otf")
	if err != nil {
		return err
	}
	if _, err := c.Register(data, "Noto Sans JP"); err != nil {
		return err
	}
	c.SetScriptFallback(language.Han, "Noto Sans JP")
	return nil
}

// The package's test files are compiled into `tela snapshot` too, so this
// also gives snapshots the CJK subset: they are deterministic and show the
// CJK card the way the goldens do.
func init() {
	if err := registerCJK(fonts); err != nil {
		panic(err)
	}
}

// The size of the showcase's goldens: the page at two columns, just fitting.
var showSize = platform.Size{W: 960, H: 900}

// render rasterizes a display list the way the CLI snapshot does.
func render(dl *gfx.DisplayList, size platform.Size, scale float64) *image.RGBA {
	m := platform.Metrics{Size: size, Scale: scale}
	img := image.NewRGBA(image.Rectangle{Max: m.Physical()})
	raster.Clear(img, image.Rectangle{}, gfx.White)
	raster.Render(img, dl, raster.Options{Scale: float32(scale)})
	return img
}

func reference(size platform.Size, scale float64) *image.RGBA {
	return render(Scene(size.W, size.H), size, scale)
}

func samePixels(t *testing.T, got, want *image.RGBA) {
	t.Helper()
	if got.Rect != want.Rect {
		t.Fatalf("frame bounds = %v, want %v", got.Rect, want.Rect)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		n := 0
		for i := range got.Pix {
			if got.Pix[i] != want.Pix[i] {
				n++
			}
		}
		t.Fatalf("frame differs from the reference in %d bytes", n)
	}
}

// --- helpers to find text and aim the mouse ---

// findPara returns the paragraph whose text contains s.
func findPara(t *testing.T, pg *page, s string) *para {
	t.Helper()
	for _, p := range pg.paras {
		if strings.Contains(p.lay.Text(), s) {
			return p
		}
	}
	t.Fatalf("no paragraph contains %q", s)
	return nil
}

// at returns the page point at the caret of byte offset off of p, nudged
// right by dx logical pixels, on the middle of its line.
func at(p *para, off int, dx float32) platform.Point {
	c := p.lay.Caret(text.Cursor{Offset: off})
	return platform.Point{X: p.x + c.X + dx, Y: p.y + c.Top + c.Height/2}
}

func offsetOf(t *testing.T, p *para, s string) int {
	t.Helper()
	i := strings.Index(p.lay.Text(), s)
	if i < 0 {
		t.Fatalf("%q is not in %q", s, p.lay.Text())
	}
	return i
}

func ptr(a platform.PointerAction, pos platform.Point, ts time.Duration, held platform.Buttons, button platform.PointerButton) platform.PointerEvent {
	return platform.PointerEvent{Action: a, Pos: pos, Time: ts, Buttons: held, Button: button}
}

func down(pos platform.Point, ts time.Duration) platform.PointerEvent {
	return ptr(platform.PointerDown, pos, ts, platform.ButtonPrimary.Mask(), platform.ButtonPrimary)
}

func up(pos platform.Point, ts time.Duration) platform.PointerEvent {
	return ptr(platform.PointerUp, pos, ts, 0, platform.ButtonPrimary)
}

func drag(pos platform.Point, ts time.Duration) platform.PointerEvent {
	return ptr(platform.PointerMove, pos, ts, platform.ButtonPrimary.Mask(), platform.ButtonNone)
}

func hoverAt(pos platform.Point, ts time.Duration) platform.PointerEvent {
	return ptr(platform.PointerMove, pos, ts, 0, platform.ButtonNone)
}

// newState returns an arranged page and its interaction state.
func newState(w, h float32) (*page, *interaction) {
	pg := newPage(fonts)
	pg.arrange(w)
	it := newInteraction(pg)
	it.setView(h)
	return pg, it
}

func (it *interaction) selected() string {
	if it.sel.para == nil {
		return ""
	}
	return it.sel.para.lay.SelectedText(it.sel.sel)
}

// --- the scene ---

func TestGolden(t *testing.T) {
	// One display list per scale would do, but rendering is what is tested.
	for _, tc := range []struct {
		name  string
		scale float64
	}{{"text_x1", 1}, {"text_x2", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			golden.Image(t, tc.name, reference(showSize, tc.scale), golden.MaxDelta(1))
		})
	}
}

// TestGoldenNarrow shows the page reflowed to one column: the same shaped
// paragraphs, broken at a narrow width.
func TestGoldenNarrow(t *testing.T) {
	size := platform.Size{W: 420, H: 1710}
	golden.Image(t, "text_narrow_x1", reference(size, 1), golden.MaxDelta(1))
}

// TestGoldenSelection draws a mouse selection with the app's own painting
// code: a drag across three lines of the Latin paragraph, and a double click
// on an Arabic word (the highlight of right-to-left text).
func TestGoldenSelection(t *testing.T) {
	pg, it := newState(showSize.W, showSize.H)
	latin := findPara(t, pg, "Every paragraph is shaped")
	a := at(latin, offsetOf(t, latin, "single"), 0)
	z := at(latin, offsetOf(t, latin, "kerning,")+4, 0)
	it.pointer(down(a, 0))
	it.pointer(drag(z, 10*time.Millisecond))
	it.pointer(up(z, 20*time.Millisecond))
	if got, want := it.selected(), "single time. Resizing the window only re-breaks the lines -> no reshaping, so kern"; got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
	b := gfx.NewBuilder()
	pg.paint(b, showSize.W, showSize.H, 0, it.sel)
	golden.Image(t, "selection_x1", render(b.Finish(), showSize, 1), golden.MaxDelta(1))

	// Arabic: a double click selects a word.
	pg, it = newState(showSize.W, showSize.H)
	ar := findPara(t, pg, "النص العربي")
	p := at(ar, offsetOf(t, ar, "اليمين")+2, 0)
	it.pointer(down(p, 0))
	it.pointer(up(p, 10*time.Millisecond))
	it.pointer(down(p, 100*time.Millisecond))
	it.pointer(up(p, 110*time.Millisecond))
	if got, want := it.selected(), "اليمين"; got != want {
		t.Fatalf("double click selected %q, want %q", got, want)
	}
	b = gfx.NewBuilder()
	pg.paint(b, showSize.W, showSize.H, 0, it.sel)
	golden.Image(t, "selection_ar_x2", render(b.Finish(), showSize, 2), golden.MaxDelta(1))
}

// TestNoMissingGlyphs guards the content: every character of the page must
// be covered by the embedded fonts or the CJK subset, so that goldens show
// glyphs and never missing-glyph boxes.
func TestNoMissingGlyphs(t *testing.T) {
	pg := newPage(fonts)
	pg.arrange(960)
	for _, p := range pg.paras {
		l := p.lay
		for i := 0; i < l.NumClusters(); i++ {
			cl := l.Cluster(i)
			for g := cl.GlyphStart; g < cl.GlyphEnd; g++ {
				if l.Glyph(g).ID == 0 {
					t.Errorf("no glyph for %q in %q", l.Text()[cl.TextStart:cl.TextEnd], l.Text())
				}
			}
		}
	}
}

// TestSceneSizes checks that the page builds and renders at unusual sizes
// (tiny, portrait, wide, fractional) without panicking.
func TestSceneSizes(t *testing.T) {
	for _, s := range []platform.Size{{W: 1, H: 1}, {W: 60, H: 40}, {W: 200, H: 150}, {W: 480, H: 800}, {W: 1600, H: 500}, {W: 777.5, H: 433.25}} {
		img := reference(s, 1.25)
		if img.Rect.Empty() {
			t.Errorf("size %v: empty image", s)
		}
	}
}

// TestColumns pins the reflow of cards into columns.
func TestColumns(t *testing.T) {
	for _, tc := range []struct {
		w    float32
		cols int
	}{{300, 1}, {420, 1}, {700, 1}, {760, 2}, {960, 2}, {1100, 2}, {1120, 3}, {2000, 3}} {
		if got, _, _ := columnsFor(tc.w); got != tc.cols {
			t.Errorf("columnsFor(%v) = %d, want %d", tc.w, got, tc.cols)
		}
	}
	pg := newPage(fonts)
	for _, w := range []float32{420, 960, 1400} {
		pg.width = 0
		pg.arrange(w)
		cols, _, _ := columnsFor(w)
		xs := map[float32]bool{}
		for _, cd := range pg.cards {
			xs[cd.x] = true
			if cd.x+cd.w > w {
				t.Errorf("width %v: card at x=%v w=%v leaves the window", w, cd.x, cd.w)
			}
		}
		if len(xs) != cols {
			t.Errorf("width %v: cards use %d columns, want %d", w, len(xs), cols)
		}
	}
}

// TestResizeReBreaksOnly is the point of the example: layouts are shaped once
// and only broken again when the width changes.
func TestResizeReBreaksOnly(t *testing.T) {
	pg := newPage(fonts)
	pg.arrange(960)
	layouts := make([]*text.Layout, len(pg.paras))
	for i, p := range pg.paras {
		layouts[i] = p.lay
	}
	tall := render(sceneOf(pg, 960, 900), showSize, 1)

	before := pg.breaks
	pg.arrange(960)
	if pg.breaks != before {
		t.Errorf("arrange at the same width broke lines again (%d calls)", pg.breaks-before)
	}

	heights := map[float32]float32{}
	for _, w := range []float32{600, 420, 1400, 960} {
		pg.arrange(w)
		heights[w] = pg.height
		if pg.breaks == before {
			t.Fatalf("arrange(%v) did not break any paragraph again", w)
		}
		before = pg.breaks
		for i, p := range pg.paras {
			if p.lay != layouts[i] {
				t.Fatalf("arrange(%v) replaced the layout of paragraph %d (%q): it was shaped again", w, i, p.lay.Text())
			}
		}
	}
	if heights[420] <= heights[960] || heights[600] <= heights[960] {
		t.Errorf("a narrower page should be taller: heights %v", heights)
	}
	// Back at 960 the page is pixel-identical to a fresh one.
	samePixels(t, render(sceneOf(pg, 960, 900), showSize, 1), tall)
}

// sceneOf paints an already arranged page.
func sceneOf(pg *page, w, h float32) *gfx.DisplayList {
	b := gfx.NewBuilder()
	pg.paint(b, w, h, 0, selection{})
	return b.Finish()
}

// --- the mouse ---

func TestClickCaretAndDrag(t *testing.T) {
	pg, it := newState(960, 900)
	p := findPara(t, pg, "Every paragraph is shaped")
	start := at(p, offsetOf(t, p, "shaped"), 0)

	if !it.pointer(down(start, 0)) {
		t.Fatal("a press on text did not change the selection")
	}
	if it.sel.para != p || !it.sel.sel.IsCollapsed() {
		t.Fatalf("a single click should leave a caret in the paragraph, got %+v", it.sel)
	}
	if !it.dragging {
		t.Fatal("the press did not start a drag")
	}
	if it.pointer(drag(start, time.Millisecond)) {
		t.Error("a move to the same place changed the selection")
	}
	end := at(p, offsetOf(t, p, "single"), 0)
	if !it.pointer(drag(end, 2*time.Millisecond)) {
		t.Fatal("dragging did not change the selection")
	}
	if got, want := it.selected(), "shaped a "; got != want {
		t.Errorf("dragged selection = %q, want %q", got, want)
	}
	// Dragging back past the anchor selects the other way; the text is the
	// same range either side.
	end = at(p, offsetOf(t, p, "Every")+2, 0)
	it.pointer(drag(end, 3*time.Millisecond))
	if got, want := it.selected(), "ery paragraph is "; got != want {
		t.Errorf("selection after dragging backwards = %q, want %q", got, want)
	}

	it.pointer(up(end, 4*time.Millisecond))
	if it.dragging {
		t.Error("still dragging after the release")
	}
	if it.pointer(drag(start, 5*time.Millisecond)) {
		t.Error("a move without a button changed the selection")
	}
	if got, want := it.selected(), "ery paragraph is "; got != want {
		t.Errorf("selection changed after the release: %q", got)
	}

	// A drag that leaves the window or the paragraph stays in the paragraph.
	it.pointer(down(start, time.Second))
	it.pointer(drag(platform.Point{X: 10000, Y: 10000}, time.Second+time.Millisecond))
	if it.sel.para != p {
		t.Error("the selection left its paragraph")
	}
	if got := it.selected(); !strings.HasSuffix(got, "type designer put them.") {
		t.Errorf("a drag to the far corner should reach the end of the paragraph, selected %q", got)
	}
}

func TestMultiClick(t *testing.T) {
	pg, it := newState(960, 900)
	p := findPara(t, pg, "Every paragraph is shaped")
	other := findPara(t, pg, "The word")
	pos := at(p, offsetOf(t, p, "single")+2, 0)
	click := func(pos platform.Point, ts time.Duration) {
		it.pointer(down(pos, ts))
		it.pointer(up(pos, ts+10*time.Millisecond))
	}

	// 1, 2 and 3 clicks.
	click(pos, 0)
	if !it.sel.sel.IsCollapsed() {
		t.Fatal("one click should be a caret")
	}
	click(pos, 200*time.Millisecond)
	if got, want := it.selected(), "single"; got != want {
		t.Fatalf("double click selected %q, want %q", got, want)
	}
	click(pos, 400*time.Millisecond)
	if got, want := it.selected(), p.lay.Text(); got != want {
		t.Fatalf("triple click selected %q, want the paragraph %q", got, want)
	}
	// A fourth click stays on the paragraph (browsers do the same).
	click(pos, 600*time.Millisecond)
	if got, want := it.selected(), p.lay.Text(); got != want {
		t.Errorf("fourth click selected %q, want the paragraph", got)
	}

	// Too slow: a new single click.
	click(pos, 600*time.Millisecond+doubleClickTime+time.Millisecond+10*time.Millisecond)
	if !it.sel.sel.IsCollapsed() {
		t.Errorf("a click after the double-click time should be single, selected %q", it.selected())
	}
	// Just within the time, and within the slop: double.
	ts := 5 * time.Second
	click(pos, ts)
	click(platform.Point{X: pos.X + doubleClickSlop, Y: pos.Y}, ts+doubleClickTime)
	if it.selected() == "" || it.sel.sel.IsCollapsed() {
		t.Error("a click at the edge of the time and the slop should count as a double click")
	}
	// Too far.
	ts = 10 * time.Second
	click(pos, ts)
	click(platform.Point{X: pos.X + doubleClickSlop + 3, Y: pos.Y}, ts+50*time.Millisecond)
	if !it.sel.sel.IsCollapsed() {
		t.Error("a click far from the first should be single")
	}
	// Another paragraph: single, in that paragraph.
	ts = 15 * time.Second
	click(pos, ts)
	pos2 := at(other, 4, 2)
	click(pos2, ts+50*time.Millisecond)
	if it.sel.para != other || !it.sel.sel.IsCollapsed() {
		t.Errorf("a quick click on another paragraph should be single there, got para %p collapsed=%v", it.sel.para, it.sel.sel.IsCollapsed())
	}
	// A time going backwards (a new backend clock) is not a double click.
	ts = 20 * time.Second
	click(pos2, ts)
	click(pos2, ts-time.Second)
	if !it.sel.sel.IsCollapsed() {
		t.Error("a click that is earlier than the previous one should be single")
	}
}

func TestDoubleClickDragSelectsWords(t *testing.T) {
	pg, it := newState(960, 900)
	p := findPara(t, pg, "Every paragraph is shaped")
	pos := at(p, offsetOf(t, p, "paragraph")+2, 0)
	it.pointer(down(pos, 0))
	it.pointer(up(pos, 10*time.Millisecond))
	it.pointer(down(pos, 100*time.Millisecond))
	it.pointer(drag(at(p, offsetOf(t, p, "single")+2, 0), 120*time.Millisecond))
	if got, want := it.selected(), "paragraph is shaped a single"; got != want {
		t.Errorf("double click and drag selected %q, want whole words %q", got, want)
	}
}

func TestClickOutsideClearsAndIgnoredButtons(t *testing.T) {
	pg, it := newState(960, 900)
	p := findPara(t, pg, "Every paragraph is shaped")
	pos := at(p, 10, 0)
	it.pointer(down(pos, 0))
	it.pointer(up(pos, 10*time.Millisecond))
	if it.sel.para == nil {
		t.Fatal("no selection after a click")
	}

	right := ptr(platform.PointerDown, pos, 20*time.Millisecond, platform.ButtonSecondary.Mask(), platform.ButtonSecondary)
	if it.pointer(right) || it.dragging {
		t.Error("a secondary button press should be ignored")
	}

	if !it.pointer(down(platform.Point{X: 3, Y: 3}, time.Second)) {
		t.Error("a press on the background did not report a change")
	}
	if it.sel.para != nil || it.dragging {
		t.Error("a press on the background should clear the selection")
	}
	if it.pointer(down(platform.Point{X: 3, Y: 3}, 2*time.Second)) {
		t.Error("a second press on the background reported a change")
	}

	// A lost release: a move without the button stops the drag.
	it.pointer(down(pos, 3*time.Second))
	it.pointer(hoverAt(at(p, 40, 0), 3*time.Second+time.Millisecond))
	if it.dragging {
		t.Error("a move without the button should end the drag")
	}
	// Cancel ends it too.
	it.pointer(down(pos, 4*time.Second))
	it.pointer(ptr(platform.PointerCancel, pos, 4*time.Second, 0, platform.ButtonNone))
	if it.dragging {
		t.Error("cancel should end the drag")
	}
}

func TestHoverCursor(t *testing.T) {
	pg, it := newState(960, 900)
	p := findPara(t, pg, "Every paragraph is shaped")
	if it.cursor() != platform.CursorDefault {
		t.Error("the initial cursor should be the default arrow")
	}
	it.pointer(hoverAt(at(p, 10, 0), 0))
	if it.cursor() != platform.CursorText {
		t.Error("the cursor over text should be the I-beam")
	}
	it.pointer(hoverAt(platform.Point{X: 3, Y: 3}, 0))
	if it.cursor() != platform.CursorDefault {
		t.Error("the cursor over the background should be the arrow")
	}
	it.pointer(ptr(platform.PointerEnter, at(p, 10, 0), 0, 0, platform.ButtonNone))
	it.pointer(ptr(platform.PointerLeave, at(p, 10, 0), 0, 0, platform.ButtonNone))
	if it.cursor() != platform.CursorDefault {
		t.Error("the cursor after leaving the window should be the arrow")
	}
	// While dragging, the I-beam stays even outside the text.
	it.pointer(down(at(p, 10, 0), 0))
	it.pointer(drag(platform.Point{X: 3, Y: 3}, time.Millisecond))
	if it.cursor() != platform.CursorText {
		t.Error("the cursor should stay an I-beam during a drag")
	}
}

func TestScrollClamps(t *testing.T) {
	pg, it := newState(960, 400) // the page is about 900 tall
	limit := pg.height - 400
	if limit < 100 {
		t.Fatalf("test needs a page taller than the view, height %v", pg.height)
	}
	if it.wheel(platform.ScrollEvent{Delta: platform.Point{Y: -10}}) {
		t.Error("scrolling up at the top moved the view")
	}
	if !it.wheel(platform.ScrollEvent{Steps: platform.Point{Y: 1}}) || it.scroll != wheelStep {
		t.Errorf("one notch down: scroll = %v, want %v", it.scroll, float32(wheelStep))
	}
	it.wheel(platform.ScrollEvent{Delta: platform.Point{Y: 100000}})
	if it.scroll != limit {
		t.Errorf("scroll = %v, want the limit %v", it.scroll, limit)
	}
	if it.wheel(platform.ScrollEvent{Delta: platform.Point{Y: 5}}) {
		t.Error("scrolling down at the end moved the view")
	}
	// A taller window brings the offset back in range.
	if !it.setView(pg.height - 10) {
		t.Error("growing the window did not move the offset back")
	}
	if it.scroll != 10 {
		t.Errorf("scroll = %v after resizing, want 10", it.scroll)
	}
	// A page that fits does not scroll.
	it.setView(pg.height + 10)
	if it.scroll != 0 || it.wheel(platform.ScrollEvent{Delta: platform.Point{Y: 50}}) {
		t.Error("a page that fits the window scrolled")
	}
}

func TestSelectionFollowsScroll(t *testing.T) {
	pg, it := newState(960, 400)
	p := findPara(t, pg, "Quarterly report")
	it.wheel(platform.ScrollEvent{Delta: platform.Point{Y: 10000}})
	// The pointer position is in window coordinates; the page moves under it.
	pos := at(p, 5, 0)
	pos.Y -= it.scroll
	if pos.Y < 0 || pos.Y > 400 {
		t.Fatalf("test paragraph is not in view: y=%v scroll=%v", pos.Y, it.scroll)
	}
	it.pointer(down(pos, 0))
	if it.sel.para != p {
		t.Fatal("a press at a scrolled position hit the wrong paragraph")
	}
}

// --- the window ---

// start creates a headless backend with a window and the app handler, and
// delivers the first events.
func start(t *testing.T, scale float64, size platform.Size) (*headless.Backend, *headless.Window, *app, *bytes.Buffer) {
	t.Helper()
	be := headless.New(headless.Options{Scale: scale})
	win, err := be.NewWindow(platform.WindowOptions{Title: "Tela — text", AppID: "dev.telaui.text", Size: size})
	if err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	a := newApp(be, win, fonts, &log)
	be.Drain(a)
	return be, win.(*headless.Window), a, &log
}

func TestPresentsScene(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		_, win, a, log := start(t, scale, showSize)
		if win.Presents() != 1 || a.frameCount != 1 {
			t.Fatalf("scale %v: Presents() = %d, frames = %d after the first configure, want 1 and 1", scale, win.Presents(), a.frameCount)
		}
		samePixels(t, win.Frame(), reference(showSize, scale))
		if want := "text: headless configure 960x900 @"; !strings.HasPrefix(log.String(), want) {
			t.Errorf("log = %q, want a line starting with %q", log.String(), want)
		}
	}
}

// TestWindowResizeKeepsLayouts resizes the window and changes its scale:
// frames match a fresh render of the scene and no paragraph is shaped again.
func TestWindowResizeKeepsLayouts(t *testing.T) {
	be, win, a, _ := start(t, 1, showSize)
	first := a.pg.paras[3].lay
	win.SetSize(platform.Size{W: 640, H: 700})
	be.Drain(a)
	if win.Presents() != 2 {
		t.Fatalf("Presents() = %d after a resize, want 2", win.Presents())
	}
	samePixels(t, win.Frame(), reference(platform.Size{W: 640, H: 700}, 1))
	win.SetScale(1.5)
	be.Drain(a)
	samePixels(t, win.Frame(), reference(platform.Size{W: 640, H: 700}, 1.5))
	if a.pg.paras[3].lay != first {
		t.Error("the window shaped a paragraph again on resize")
	}
}

func TestIdleDoesNothing(t *testing.T) {
	be, win, a, _ := start(t, 1, showSize)
	if be.Pending() || win.FrameRequested() {
		t.Fatalf("work pending after the first frame: Pending=%v FrameRequested=%v", be.Pending(), win.FrameRequested())
	}
	for i := 0; i < 5; i++ {
		be.Advance(time.Second)
		if n := be.Drain(a); n != 0 {
			t.Fatalf("idle Drain delivered %d events, want 0", n)
		}
	}
	if win.Presents() != 1 {
		t.Errorf("Presents() = %d while idle, want 1", win.Presents())
	}
}

// TestMouseInWindow drives the app with injected pointer events: a frame is
// requested exactly when the selection changes, the cursor follows the
// pointer and the selection is on screen.
func TestMouseInWindow(t *testing.T) {
	be, win, a, _ := start(t, 1, showSize)
	base := append([]byte(nil), win.Frame().Pix...)
	p := findPara(t, a.pg, "Every paragraph is shaped")
	from := at(p, offsetOf(t, p, "single"), 0)
	to := at(p, offsetOf(t, p, "kerning,"), 0)

	// settle delivers the events and, when a frame was requested, the frame.
	settle := func() int {
		be.Drain(a)
		if win.FrameRequested() {
			be.Advance(be.FrameInterval())
			be.Drain(a)
		}
		return win.Presents()
	}
	presents := win.Presents()

	// Hovering changes the cursor and draws nothing.
	win.Inject(hoverAt(from, be.Now()))
	be.Drain(a)
	if win.FrameRequested() || win.Presents() != presents {
		t.Error("hovering requested a frame")
	}
	if win.Cursor() != platform.CursorText {
		t.Errorf("cursor over text = %v, want the I-beam", win.Cursor())
	}
	win.Inject(hoverAt(platform.Point{X: 3, Y: 3}, be.Now()))
	settle()
	if win.Cursor() != platform.CursorDefault {
		t.Errorf("cursor over the background = %v, want the arrow", win.Cursor())
	}

	// A press draws a caret.
	win.Inject(down(from, be.Now()))
	if got := settle(); got != presents+1 {
		t.Fatalf("Presents() = %d after a press, want %d", got, presents+1)
	}
	if bytes.Equal(base, win.Frame().Pix) {
		t.Error("the caret was not drawn")
	}
	// The same press position again (a drag that does not move) draws nothing.
	win.Inject(drag(from, be.Now()))
	if settle() != presents+1 {
		t.Error("a move that changed nothing requested a frame")
	}
	// Dragging draws the highlight.
	win.Inject(drag(to, be.Now()))
	if got := settle(); got != presents+2 {
		t.Fatalf("Presents() = %d after a drag, want %d", got, presents+2)
	}
	win.Inject(up(to, be.Now()))
	if settle() != presents+2 {
		t.Error("the release requested a frame")
	}
	if got := a.it.selected(); !strings.HasPrefix(got, "single time.") {
		t.Errorf("selected %q", got)
	}
	// The frame is the page painted with that selection, as the golden does.
	b := gfx.NewBuilder()
	a.pg.paint(b, showSize.W, showSize.H, 0, a.it.sel)
	samePixels(t, win.Frame(), render(b.Finish(), showSize, 1))

	// Clicking the background clears it, and everything is idle again.
	win.Inject(down(platform.Point{X: 3, Y: 3}, be.Now()))
	if settle() != presents+3 {
		t.Error("clearing the selection did not redraw")
	}
	samePixels(t, win.Frame(), render(Scene(showSize.W, showSize.H), showSize, 1))
	be.Advance(time.Minute)
	if be.Drain(a) != 0 || win.Presents() != presents+3 || win.FrameRequested() {
		t.Error("work after the last event: not idle")
	}
}

func TestWheelInWindow(t *testing.T) {
	be, win, a, _ := start(t, 1, platform.Size{W: 960, H: 500})
	presents := win.Presents()
	win.Inject(platform.ScrollEvent{Pos: platform.Point{X: 100, Y: 100}, Steps: platform.Point{Y: -1}})
	be.Drain(a)
	if win.Presents() != presents {
		t.Error("scrolling up at the top requested a frame")
	}
	win.Inject(platform.ScrollEvent{Pos: platform.Point{X: 100, Y: 100}, Steps: platform.Point{Y: 2}})
	be.Drain(a)
	be.Advance(be.FrameInterval())
	be.Drain(a)
	if win.Presents() != presents+1 || a.it.scroll != 2*wheelStep {
		t.Errorf("Presents() = %d, scroll = %v after two notches", win.Presents(), a.it.scroll)
	}
	// The frame is the scrolled page.
	b := gfx.NewBuilder()
	a.pg.paint(b, 960, 500, a.it.scroll, selection{})
	samePixels(t, win.Frame(), render(b.Finish(), platform.Size{W: 960, H: 500}, 1))
}

func TestCloseRequest(t *testing.T) {
	be, win, a, _ := start(t, 1, showSize)
	win.RequestClose()
	be.Drain(a)
	if !win.Closed() || !be.Quitting() {
		t.Errorf("Closed() = %v, Quitting() = %v after CloseRequestedEvent, want both true", win.Closed(), be.Quitting())
	}
}
