package main

import (
	"math"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/text"
)

// Multi-click detection, as on every desktop: a press within doubleClickTime
// of the previous one and within doubleClickSlop logical pixels of it, on the
// same paragraph, counts one more click.
const (
	doubleClickTime = 500 * time.Millisecond
	doubleClickSlop = 5
	wheelStep       = 48 // logical pixels per wheel notch
)

// interaction is the state machine behind the mouse: selection, multi-click
// counting, hover cursor and scrolling. It has no clock, no timers and no
// window: it is fed pointer events (with their own timestamps) and reports
// whether the picture changed, so the app can request a frame only then.
type interaction struct {
	pg *page

	sel      selection
	dragging bool

	// Multi-click state.
	clicks   int
	lastPara *para
	lastTime time.Duration
	lastPos  platform.Point
	hover    *para
	scroll   float32 // vertical scroll offset, logical px
	viewH    float32 // height of the window, logical px
}

func newInteraction(pg *page) *interaction { return &interaction{pg: pg} }

// cursor is the pointer shape for the current hover: the I-beam over text.
func (it *interaction) cursor() platform.Cursor {
	if it.hover != nil || it.dragging {
		return platform.CursorText
	}
	return platform.CursorDefault
}

// setView records the window height and keeps the scroll offset in range
// (after a resize the page height changes). It reports a change of offset.
func (it *interaction) setView(h float32) bool {
	it.viewH = h
	return it.scrollTo(it.scroll)
}

// scrollTo moves the view to offset y, clamped to the page, and reports
// whether it moved.
func (it *interaction) scrollTo(y float32) bool {
	limit := max(0, it.pg.height-it.viewH)
	y = min(max(y, 0), limit)
	if y == it.scroll {
		return false
	}
	it.scroll = y
	return true
}

// wheel handles a scroll event and reports whether the view moved.
func (it *interaction) wheel(e platform.ScrollEvent) bool {
	dy := e.Delta.Y
	if e.Steps.Y != 0 {
		dy = e.Steps.Y * wheelStep
	}
	return it.scrollTo(it.scroll + dy)
}

// pointer handles a pointer event and reports whether the selection or the
// hover changed in a way that needs a new frame.
func (it *interaction) pointer(e platform.PointerEvent) (redraw bool) {
	if e.Kind != platform.PointerMouse && e.Kind != platform.PointerPen && e.Kind != platform.PointerTouch {
		return false
	}
	x, y := e.Pos.X, e.Pos.Y+it.scroll
	p := it.pg.paraAt(x, y)
	it.hover = p

	switch e.Action {
	case platform.PointerDown:
		if e.Button != platform.ButtonPrimary {
			return false
		}
		return it.press(p, x, y, e)
	case platform.PointerMove:
		if !it.dragging {
			return false
		}
		if !e.Buttons.Has(platform.ButtonPrimary) { // the release was lost
			it.dragging = false
			return false
		}
		next := it.sel.para.lay.ExtendSelection(it.sel.sel, x-it.sel.para.x, y-it.sel.para.y)
		if next == it.sel.sel {
			return false
		}
		it.sel.sel = next
		return true
	case platform.PointerUp, platform.PointerCancel:
		it.dragging = false
	case platform.PointerLeave:
		it.hover = nil
	}
	return false
}

// press starts a selection at (x, y) in page coordinates: a caret for a
// single click, the word for a double click, the paragraph for a triple one.
func (it *interaction) press(p *para, x, y float32, e platform.PointerEvent) bool {
	if p == nil {
		it.clicks, it.lastPara = 0, nil
		it.dragging = false
		changed := it.sel.para != nil
		it.sel = selection{}
		return changed
	}
	near := math.Abs(float64(e.Pos.X-it.lastPos.X)) <= doubleClickSlop && math.Abs(float64(e.Pos.Y-it.lastPos.Y)) <= doubleClickSlop
	if p == it.lastPara && near && e.Time-it.lastTime <= doubleClickTime && e.Time >= it.lastTime {
		it.clicks = min(it.clicks+1, 3)
	} else {
		it.clicks = 1
	}
	it.lastPara, it.lastTime, it.lastPos = p, e.Time, e.Pos

	lx, ly := x-p.x, y-p.y
	prev := it.sel
	switch it.clicks {
	case 1:
		it.sel = selection{p, text.CaretSelection(p.lay.HitTest(lx, ly))}
	case 2:
		it.sel = selection{p, p.lay.SelectWordAt(lx, ly)}
	default:
		it.sel = selection{p, p.lay.SelectParagraphAt(lx, ly)}
	}
	it.dragging = true
	return it.sel != prev
}
