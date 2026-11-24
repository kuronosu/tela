package style

import "testing"

// Tailwind 4.3's transition and transition-colors list fill and stroke;
// stroke-width transitions with transition-all only.
func TestTransitionsOfSVGPaints(t *testing.T) {
	const both = TransitionFill | TransitionStroke
	for _, c := range []struct {
		classes string
		want    TransitionProps // of the SVG paints
	}{
		{"transition", both},
		{"transition-colors", both},
		{"transition-all", both | TransitionStrokeWidth},
		{"transition-opacity", 0},
		{"transition-none", 0},
		{"transition-[fill]", TransitionFill},
		{"transition-[stroke,stroke-width]", TransitionStroke | TransitionStrokeWidth},
	} {
		if got := style(t, c.classes).Transition.Properties & (both | TransitionStrokeWidth); got != c.want {
			t.Errorf("%s transitions %b of the SVG paints, want %b", c.classes, got, c.want)
		}
	}
}

// PaintsSet: the SVG paints that the element's own classes set.
func TestPaintsSet(t *testing.T) {
	const both = TransitionFill | TransitionStroke
	parent := style(t, "fill-red-500 stroke-red-500 stroke-3")
	for _, c := range []struct {
		classes string
		want    TransitionProps
	}{
		{"", 0},
		{"fill-red-500", TransitionFill},
		{"fill-none", TransitionFill},
		{"fill-current stroke-current/50", both},
		{"stroke-2", TransitionStrokeWidth},
		{"stroke-[1.5]", TransitionStrokeWidth},
		{"stroke-[2px] stroke-primary", TransitionStroke | TransitionStrokeWidth},
	} {
		if got := styleIn(t, testCx, KindDiv, parent, c.classes).PaintsSet; got != c.want {
			t.Errorf("%q sets %b, want %b", c.classes, got, c.want)
		}
	}
}
