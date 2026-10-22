package aria

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// testRef returns a non-zero ElementRef (the zero one is not a valid
// argument of Arrow); the oracle only needs it to be non-zero. It mounts
// nothing: a Mount per case kept 2,400 headless windows and their
// framebuffers alive until the test ended (3.5 GB).
func testRef(testing.TB) tela.ElementRef { return core.NewElementRef() }

// The oracle of the port: testdata/floatingui/cases.json holds random
// rectangles, placements and middleware stacks run through the real
// @floating-ui/core (testdata/floatingui/gen.mjs generates it). Every case
// must give the same coordinates, final placement, middleware data and
// calls of the size middleware.

type oracleCase struct {
	ID            int          `json:"id"`
	Reference     [4]float32   `json:"reference"`
	Floating      [2]float32   `json:"floating"`
	Clip          [4]float32   `json:"clip"`
	ReferenceClip [4]float32   `json:"referenceClip"`
	RTL           bool         `json:"rtl"`
	Arrow         [2]float32   `json:"arrow"`
	Placement     string       `json:"placement"`
	Emulate       bool         `json:"emulate"`
	Middleware    []oracleSpec `json:"middleware"`
	Expect        oracleExpect `json:"expect"`
}

type oracleSpec struct {
	Name    string          `json:"name"`
	Options json.RawMessage `json:"options"`
}

type oracleExpect struct {
	X         float32 `json:"x"`
	Y         float32 `json:"y"`
	Placement string  `json:"placement"`
	Size      []struct {
		AvailableWidth  float32    `json:"availableWidth"`
		AvailableHeight float32    `json:"availableHeight"`
		Reference       [4]float32 `json:"reference"`
		Placement       string     `json:"placement"`
	} `json:"size"`
	Arrow *struct {
		X               *float32 `json:"x"`
		Y               *float32 `json:"y"`
		CenterOffset    float32  `json:"centerOffset"`
		AlignmentOffset *float32 `json:"alignmentOffset"`
	} `json:"arrow"`
	Hide *struct {
		ReferenceHidden *bool `json:"referenceHidden"`
		Escaped         *bool `json:"escaped"`
	} `json:"hide"`
	Limit *[2]float32 `json:"limit"`
}

func loadOracle(t testing.TB) []oracleCase {
	t.Helper()
	data, err := os.ReadFile("testdata/floatingui/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		FloatingUI string       `json:"floatingUI"`
		Cases      []oracleCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Cases) == 0 {
		t.Fatal("no oracle cases")
	}
	return file.Cases
}

// opt is a decoded JSON object of options.
type opt map[string]any

func (o opt) num(k string) (float32, bool) {
	v, ok := o[k].(float64)
	return float32(v), ok
}
func (o opt) f(k string) float32 { v, _ := o.num(k); return v }
func (o opt) str(k string) string {
	s, _ := o[k].(string)
	return s
}
func (o opt) isFalse(k string) bool { b, ok := o[k].(bool); return ok && !b }
func (o opt) isTrue(k string) bool  { b, ok := o[k].(bool); return ok && b }
func (o opt) strs(k string) []string {
	raw, ok := o[k].([]any)
	if !ok {
		return nil
	}
	out := make([]string, len(raw)) // non-nil even when empty
	for i, v := range raw {
		out[i] = v.(string)
	}
	return out
}

// floatingOptions translates the JSON of a case into the public options.
func (c *oracleCase) floatingOptions(t testing.TB, calls *[]sizeCall) []FloatingOption {
	opts := []FloatingOption{Placement(c.Placement)}
	for _, m := range c.Middleware {
		var raw any
		if err := json.Unmarshal(m.Options, &raw); err != nil {
			t.Fatal(err)
		}
		o, _ := raw.(map[string]any)
		op := opt(o)
		switch m.Name {
		case "offset":
			if n, ok := raw.(float64); ok {
				opts = append(opts, Offset(float32(n)))
				break
			}
			oo := OffsetOptions{CrossAxis: op.f("crossAxis")}
			if a, ok := op.num("alignmentAxis"); ok {
				oo.AlignmentAxis, oo.HasAlignmentAxis = a, true
			}
			opts = append(opts, Offset(op.f("mainAxis"), oo))
		case "flip":
			fo := FlipOptions{
				Padding:                   op.f("padding"),
				NoMainAxis:                op.isFalse("mainAxis"),
				NoCrossAxis:               op.isFalse("crossAxis"),
				CrossAxisAlignment:        op.str("crossAxis") == "alignment",
				FallbackPlacements:        op.strs("fallbackPlacements"),
				FallbackStrategy:          op.str("fallbackStrategy"),
				FallbackAxisSideDirection: op.str("fallbackAxisSideDirection"),
				NoFlipAlignment:           op.isFalse("flipAlignment"),
			}
			opts = append(opts, Flip(fo))
		case "shift":
			so := ShiftOptions{NoMainAxis: op.isFalse("mainAxis"), CrossAxis: op.isTrue("crossAxis")}
			if lraw, ok := o["limiter"].(map[string]any); ok {
				l := opt(lraw)
				lo := LimitShiftOptions{NoMainAxis: l.isFalse("mainAxis"), NoCrossAxis: l.isFalse("crossAxis")}
				switch off := lraw["offset"].(type) {
				case float64:
					lo.Offset = float32(off)
				case map[string]any:
					lo.Offset, lo.CrossAxisOffset = opt(off).f("mainAxis"), opt(off).f("crossAxis")
				}
				so.Limiter = LimitShift(lo)
			}
			opts = append(opts, Shift(op.f("padding"), so))
		case "size":
			opts = append(opts, Size(func(a Available) {
				*calls = append(*calls, sizeCall{a.Width, a.Height, [4]float32{a.Rects.Reference.X, a.Rects.Reference.Y, a.Rects.Reference.Width, a.Rects.Reference.Height}, a.Placement})
				if c.Emulate {
					a.MaxWidth(max(a.Width, 0))
					a.MaxHeight(max(a.Height, 0))
				}
			}, SizeOptions{Padding: op.f("padding")}))
		case "arrow":
			opts = append(opts, Arrow(testRef(t), ArrowOptions{Padding: op.f("padding")}))
		case "hide":
			opts = append(opts, Hide(HideOptions{Strategy: op.str("strategy"), Padding: op.f("padding")}))
		case "autoPlacement":
			opts = append(opts, AutoPlacement(AutoPlacementOptions{
				Padding:           op.f("padding"),
				CrossAxis:         op.isTrue("crossAxis"),
				Alignment:         op.str("alignment"),
				AllowedPlacements: op.strs("allowedPlacements"),
				NoAutoAlignment:   op.isFalse("autoAlignment"),
			}))
		default:
			t.Fatalf("case %d: unknown middleware %q", c.ID, m.Name)
		}
	}
	return opts
}

type sizeCall struct {
	w, h      float32
	reference [4]float32
	placement string
}

func (c *oracleCase) env(floating [2]float32) *environment {
	return &environment{
		reference:     rect{c.Reference[0], c.Reference[1], c.Reference[2], c.Reference[3]},
		floating:      rect{width: floating[0], height: floating[1]},
		clip:          rect{c.Clip[0], c.Clip[1], c.Clip[2], c.Clip[3]},
		referenceClip: rect{c.ReferenceClip[0], c.ReferenceClip[1], c.ReferenceClip[2], c.ReferenceClip[3]},
		rtl:           c.RTL,
		arrow:         rect{width: c.Arrow[0], height: c.Arrow[1]},
		hasArrow:      c.Arrow[0] > 0 || c.Arrow[1] > 0,
	}
}

func near(a, b float32) bool {
	if math.IsNaN(float64(a)) || math.IsNaN(float64(b)) {
		return math.IsNaN(float64(a)) && math.IsNaN(float64(b))
	}
	return math.Abs(float64(a-b)) <= 1e-3
}

func TestOracleComputePosition(t *testing.T) {
	cases := loadOracle(t)
	checked, withSize, withArrow, withHide, flipped := 0, 0, 0, 0, 0
	for i := range cases {
		c := &cases[i]
		if c.Emulate {
			continue
		}
		var calls []sizeCall
		cfg := newFloatingConfig(c.floatingOptions(t, &calls))
		pos := computePosition(c.env(c.Floating), cfg.placement, cfg.mws)
		want := c.Expect
		if !near(pos.x, want.X) || !near(pos.y, want.Y) || pos.placement != want.Placement {
			t.Errorf("case %d (%s, %s): got (%g, %g) %s, Floating UI gives (%g, %g) %s", c.ID, c.Placement, describe(c.Middleware), pos.x, pos.y, pos.placement, want.X, want.Y, want.Placement)
			continue
		}
		if pos.placement != c.Placement {
			flipped++
		}
		if len(calls) != len(want.Size) {
			t.Errorf("case %d: size ran %d times, Floating UI %d", c.ID, len(calls), len(want.Size))
			continue
		}
		for j, got := range calls {
			w := want.Size[j]
			if !near(got.w, w.AvailableWidth) || !near(got.h, w.AvailableHeight) || got.placement != w.Placement ||
				!near(got.reference[0], w.Reference[0]) || !near(got.reference[3], w.Reference[3]) {
				t.Errorf("case %d: size call %d got %+v, Floating UI %+v", c.ID, j, got, w)
			}
			withSize++
		}
		if a := want.Arrow; a != nil {
			d := pos.data.arrow
			withArrow++
			if (a.X != nil) != d.hasX || (a.Y != nil) != d.hasY ||
				(a.X != nil && !near(*a.X, d.x)) || (a.Y != nil && !near(*a.Y, d.y)) ||
				!near(a.CenterOffset, d.centerOffset) || (a.AlignmentOffset != nil) != d.hasAlignmentOffset() ||
				(a.AlignmentOffset != nil && !near(*a.AlignmentOffset, d.alignmentOffset)) {
				t.Errorf("case %d: arrow data got %+v, Floating UI x=%v y=%v center=%g align=%v", c.ID, d, ptr(a.X), ptr(a.Y), a.CenterOffset, ptr(a.AlignmentOffset))
			}
		}
		if h := want.Hide; h != nil {
			withHide++
			d := pos.data.hide
			if (h.ReferenceHidden != nil && (!d.hasReference || *h.ReferenceHidden != d.referenceHidden)) ||
				(h.Escaped != nil && (!d.hasEscaped || *h.Escaped != d.escaped)) {
				t.Errorf("case %d: hide data got %+v, Floating UI %+v %+v", c.ID, d, ptr(h.ReferenceHidden), ptr(h.Escaped))
			}
		}
		checked++
	}
	t.Logf("%d cases equal (%d changed placement; %d size calls, %d arrow, %d hide)", checked, flipped, withSize, withArrow, withHide)
	if checked < 1000 || flipped < 100 || withSize < 50 || withArrow < 100 || withHide < 50 {
		t.Errorf("the oracle covers too little: %d cases, %d flipped, %d size, %d arrow, %d hide", checked, flipped, withSize, withArrow, withHide)
	}
}

func ptr[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func describe(m []oracleSpec) string {
	s := ""
	for i, x := range m {
		if i > 0 {
			s += ","
		}
		s += x.Name
	}
	return s
}

// TestOracleSizeLimits: a size middleware that limits the floating element
// (the engine lays it out again and calls the position function again, where
// Floating UI measures it again inside computePosition) ends at the same
// position. The engine's loop is simulated: the element's size is its
// natural size cut by the limits of the last call, until the limits stop
// changing.
//
// Floating UI keeps the data of earlier runs through such a reset (the index
// of flip, the overflows it collected), while the engine starts a fresh
// computation with the new size, so the two can end on different placements
// when flip, autoPlacement, offset or shift run after size (documented in
// ADR-099; size goes last in every example). With size last they agree in
// all but rare cases.
func TestOracleSizeLimits(t *testing.T) {
	cases := loadOracle(t)
	type tally struct{ n, differ, slow int }
	var last, other tally
	for i := range cases {
		c := &cases[i]
		if !c.Emulate {
			continue
		}
		var calls []sizeCall
		cfg := newFloatingConfig(c.floatingOptions(t, &calls))
		var lim [2]float32
		hasLim := false
		var pos position
		iters := 0
		for ; iters < 8; iters++ {
			fl := c.Floating
			if hasLim {
				fl[0], fl[1] = min(fl[0], lim[0]), min(fl[1], lim[1])
			}
			pos = computePosition(c.env(fl), cfg.placement, cfg.mws)
			var next [2]float32
			for _, l := range cfg.sizeLims {
				if l.hasMaxWidth {
					next[0] = l.maxWidth
				}
				if l.hasMaxHeight {
					next[1] = l.maxHeight
				}
			}
			if hasLim && next == lim {
				break
			}
			lim, hasLim = next, true
		}
		sizeIdx, lastOther := -1, -1
		for j, m := range c.Middleware {
			switch m.Name {
			case "size":
				sizeIdx = j
			case "flip", "autoPlacement", "shift", "offset":
				lastOther = j
			}
		}
		tl := &other
		if sizeIdx > lastOther {
			tl = &last
		}
		tl.n++
		if iters >= 3 { // the engine lays out at most twice per frame
			tl.slow++
		}
		w := c.Expect
		if !near(pos.x, w.X) || !near(pos.y, w.Y) || pos.placement != w.Placement {
			tl.differ++
			if sizeIdx > lastOther {
				t.Logf("case %d (%s, %s): engine loop (%g, %g) %s, Floating UI (%g, %g) %s", c.ID, c.Placement, describe(c.Middleware), pos.x, pos.y, pos.placement, w.X, w.Y, w.Placement)
			}
		}
	}
	t.Logf("size last: %d cases, %d end elsewhere than Floating UI's reset loop, %d need more than 2 relayouts", last.n, last.differ, last.slow)
	t.Logf("size first: %d cases, %d end elsewhere, %d need more than 2 relayouts", other.n, other.differ, other.slow)
	if last.n < 50 {
		t.Errorf("only %d cases with size last", last.n)
	}
	if last.differ*20 > last.n {
		t.Errorf("%d of %d cases with size last differ from Floating UI", last.differ, last.n)
	}
}
