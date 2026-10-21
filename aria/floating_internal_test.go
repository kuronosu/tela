package aria

import (
	"math"
	"reflect"
	"testing"

	"github.com/kuronosu/tela/internal/core"
)

func TestPlacementHelpers(t *testing.T) {
	for _, p := range allPlacements {
		if got := getOppositePlacement(getOppositePlacement(p)); got != p {
			t.Errorf("the opposite of the opposite of %s is %s", p, got)
		}
		if !validPlacement(p) {
			t.Errorf("%s is not valid", p)
		}
	}
	if got := getExpandedPlacements("bottom-start"); !reflect.DeepEqual(got, []string{"bottom-end", "top-start", "top-end"}) {
		t.Errorf("expanded placements of bottom-start: %v", got)
	}
	// Floating UI's fallbackAxisSideDirection lists, for a vertical side in
	// left-to-right and right-to-left elements.
	if got := getOppositeAxisPlacements("bottom-start", true, "start", false); !reflect.DeepEqual(got, []string{"left-start", "right-start", "left-end", "right-end"}) {
		t.Errorf("opposite axis placements: %v", got)
	}
	if got := getOppositeAxisPlacements("top", false, "start", true); !reflect.DeepEqual(got, []string{"right", "left"}) {
		t.Errorf("opposite axis placements in rtl: %v", got)
	}
	if validPlacement("bottom-middle") || validPlacement("") || validPlacement("Bottom") {
		t.Errorf("invalid placements accepted")
	}
}

func TestClosest(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"bottm-start", "bottom-start"},
		{"Top", "top"},
		{"botom", "bottom"},
		{"left-strt", "left-start"},
		{"center", ""},
	} {
		if got := closest(tc.in, allPlacements); got != tc.want {
			t.Errorf("closest(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPlaceWithoutAnchor: an anchor that is not displayed leaves the element
// where it is (a result the engine ignores) and does not panic.
func TestPlaceWithoutAnchor(t *testing.T) {
	cfg := newFloatingConfig([]FloatingOption{Offset(4), Flip(), Shift(8)})
	res := cfg.place(core.FloatInput{}, &floatingPublish{})
	if !math.IsNaN(float64(res.X)) || !math.IsNaN(float64(res.Y)) || res.HasMaxWidth || res.HasMaxHeight {
		t.Errorf("without an anchor: %+v", res)
	}
}

func TestAvailableZeroValue(t *testing.T) {
	var a Available
	a.MaxWidth(10) // no engine behind it: nothing to limit, nothing to crash
	a.MaxHeight(10)
}

// TestSizeLimitsAreRemoved: a run whose apply does not ask for a limit
// leaves none.
func TestSizeLimitsAreRemoved(t *testing.T) {
	limit := true
	cfg := newFloatingConfig([]FloatingOption{Size(func(a Available) {
		if limit {
			a.MaxHeight(a.Height)
		}
	})})
	env := &environment{
		reference: rect{100, 50, 40, 20},
		floating:  rect{width: 100, height: 400},
		clip:      rect{0, 0, 400, 300}, referenceClip: rect{0, 0, 400, 300},
	}
	computePosition(env, "bottom", cfg.mws)
	if l := cfg.sizeLims[0]; !l.hasMaxHeight || l.maxHeight != 230 || l.hasMaxWidth {
		t.Errorf("limits %+v, want a max height of 230", *l)
	}
	limit = false
	computePosition(env, "bottom", cfg.mws)
	if l := cfg.sizeLims[0]; l.hasMaxHeight {
		t.Errorf("the limit stayed: %+v", *l)
	}
}
