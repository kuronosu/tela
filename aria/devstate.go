//go:build teladev

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// Codecs for tela dev (ADR-113, ADR-309). The values and hook stores of
// this package hold their state in unexported fields, which tela dev
// cannot keep, so a reload reset them silently: a Selection in the app's
// own state, the uncontrolled choice of a radio group or a calendar, the
// selection and focused item of a collection, a slider's values, a color.
// The GobEncode and GobDecode methods below give tela dev what to keep:
// core.DevEncode checks it as it checks hook state, and a store of a
// UseOnce (the collection, the slider, the color, the toggle group's
// choice, the rover, the calendar) is decoded into the store the next
// build's initializer made, before the hook reads it; a store marked
// started then takes the options of the first build as a later build does,
// keeping what was restored.
//
// What follows the pointer or the platform (a press, a hover, a drag, the
// typeahead's search, timers), what every build makes again from kept
// state (boolState, choice) and handles (refs, setters, callbacks) have no
// codec: they reset, and tela dev -v lists them.
//
// Compiled only with the teladev build tag: release builds do not have
// these methods.

// devSelection is what a Selection keeps.
type devSelection[K comparable] struct {
	Set, All bool
	Keys     []K
}

func (s Selection[K]) GobEncode() ([]byte, error) {
	var d devSelection[K]
	if s.s != nil {
		d.Set, d.All, d.Keys = true, s.s.all, s.s.keys
	}
	return core.DevEncode(d)
}

func (s *Selection[K]) GobDecode(b []byte) error {
	var d devSelection[K]
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	switch {
	case !d.Set:
		*s = Selection[K]{}
	case d.All:
		*s = All[K]()
	default:
		*s = SelectionOf(d.Keys...)
	}
	return nil
}

// devOptional is what an optional keeps: the uncontrolled value of a
// radio group, a calendar's date and its focused date (useChoice).
type devOptional[K comparable] struct {
	K  K
	OK bool
}

func (o optional[K]) GobEncode() ([]byte, error) { return core.DevEncode(devOptional[K]{o.k, o.ok}) }

func (o *optional[K]) GobDecode(b []byte) error {
	var d devOptional[K]
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	*o = optional[K]{d.K, d.OK}
	return nil
}

// devColl is what a collection keeps (list boxes, menus, selects, tabs,
// combo boxes): its uncontrolled selection, its range anchor and the
// focused item, and whether the focus was inside it (the engine keeps the
// focused element itself).
type devColl[K comparable] struct {
	Sel                     Selection[K]
	Focused, Anchor, Cur    K
	HasFocused, HasAnchor   bool
	HasCur, Within, Visible bool
}

func (c *collState[K]) GobEncode() ([]byte, error) {
	return core.DevEncode(devColl[K]{
		Sel: c.sel, Focused: c.focused, Anchor: c.anchor, Cur: c.current,
		HasFocused: c.hasFocused, HasAnchor: c.hasAnchor, HasCur: c.hasCurrent, Within: c.within, Visible: c.visible,
	})
}

func (c *collState[K]) GobDecode(b []byte) error {
	var d devColl[K]
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	c.sel = d.Sel
	if !c.sel.IsSet() {
		c.sel = SelectionOf[K]()
	}
	c.focused, c.hasFocused = d.Focused, d.HasFocused
	c.anchor, c.hasAnchor = d.Anchor, d.HasAnchor
	c.current, c.hasCurrent = d.Cur, d.HasCur
	c.within, c.visible = d.Within, d.Visible
	c.started = true // the restored selection and focus, not the defaults
	return nil
}

// devSlider is what a slider keeps: its uncontrolled values.
type devSlider struct{ Values []float64 }

func (s *sliderState) GobEncode() ([]byte, error) { return core.DevEncode(devSlider{s.values}) }

func (s *sliderState) GobDecode(b []byte) error {
	var d devSlider
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	if len(d.Values) > 0 {
		s.values, s.started = d.Values, true
	}
	return nil
}

// devColor is what a ColorState keeps: the color shown, as HSB (so a gray
// keeps its hue), and the HSL saturation a lightness of 0 or 100 hides.
type devColor struct {
	Shown HSB
	HSLS  float64
}

func (s *ColorState) GobEncode() ([]byte, error) { return core.DevEncode(devColor{s.shown, s.hslS}) }

func (s *ColorState) GobDecode(b []byte) error {
	var d devColor
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	s.shown, s.hslS, s.started = d.Shown, d.HSLS, true
	return nil
}

// devSelChoice is what a toggle group keeps: its uncontrolled selection.
type devSelChoice[K comparable] struct{ Kept Selection[K] }

func (s *selChoice[K]) GobEncode() ([]byte, error) { return core.DevEncode(devSelChoice[K]{s.kept}) }

func (s *selChoice[K]) GobDecode(b []byte) error {
	var d devSelChoice[K]
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	s.kept, s.started = d.Kept, true
	if !s.kept.IsSet() {
		s.kept = SelectionOf[K]()
	}
	return nil
}

// devRover is what a roving group keeps: the item that is its tab stop.
type devRover[K comparable] struct {
	Focused    K
	HasFocused bool
}

func (r *rover[K]) GobEncode() ([]byte, error) {
	return core.DevEncode(devRover[K]{r.focused, r.hasFocused})
}

func (r *rover[K]) GobDecode(b []byte) error {
	var d devRover[K]
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	r.focused, r.hasFocused = d.Focused, d.HasFocused
	return nil
}

// devCal is what a calendar keeps besides its value and focused date
// (useChoice's state): the first month on display and a range being
// chosen.
type devCal struct {
	Start, Anchor, WinLo, WinHi Date
	HasAnchor                   bool
}

func (c *cal) GobEncode() ([]byte, error) {
	return core.DevEncode(devCal{Start: c.start, Anchor: c.anchor, WinLo: c.winLo, WinHi: c.winHi, HasAnchor: c.hasAnchor})
}

func (c *cal) GobDecode(b []byte) error {
	var d devCal
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	c.start, c.anchor, c.winLo, c.winHi, c.hasAnchor = d.Start, d.Anchor, d.WinLo, d.WinHi, d.HasAnchor
	return nil
}

// devCtxPoint and devCtxSeen are what an open context menu keeps: the
// point it opened at and the opening it belongs to.
type devCtxPoint struct {
	Gen uint64
	At  tela.Rect
}

func (p ctxPoint) GobEncode() ([]byte, error) { return core.DevEncode(devCtxPoint{p.gen, p.at}) }

func (p *ctxPoint) GobDecode(b []byte) error {
	var d devCtxPoint
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	*p = ctxPoint{gen: d.Gen, at: d.At}
	return nil
}

type devCtxSeen struct {
	Open, App      bool
	Gen, AppGen    uint64
	Extra          int
	AppAt, AppFrom tela.Rect
}

func (s ctxSeen) GobEncode() ([]byte, error) {
	return core.DevEncode(devCtxSeen{Open: s.open, App: s.app, Gen: s.gen, AppGen: s.appGen, Extra: s.extra, AppAt: s.appAt, AppFrom: s.appFrom})
}

func (s *ctxSeen) GobDecode(b []byte) error {
	var d devCtxSeen
	if err := core.DevDecode(b, &d); err != nil {
		return err
	}
	*s = ctxSeen{open: d.Open, app: d.App, gen: d.Gen, appGen: d.AppGen, extra: d.Extra, appAt: d.AppAt, appFrom: d.AppFrom}
	return nil
}
