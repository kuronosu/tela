package win32

import (
	"image"
	"slices"
	"testing"
)

// Renamed message box buttons are widened to their texts (ADR-259): one
// width for the row, right-aligned with the box's gaps and margin, and
// the box grows when the row would pass the left margin.
func TestFitButtonRow(t *testing.T) {
	// A Yes/No/Cancel box: 75 px buttons 6 px apart, 11 px right margin,
	// in a 300 px client area.
	row := []image.Rectangle{image.Rect(52, 100, 127, 123), image.Rect(133, 100, 208, 123), image.Rect(214, 100, 289, 123)}
	for _, tc := range []struct {
		name string
		need []int
		want []image.Rectangle
		grow int
	}{
		{"short texts keep the layout", []int{40, 60, 75}, row, 0},
		// 3*90 + 2*6 = 282, plus two 11 px margins: 304, 4 more than 300.
		{"a long one widens the row", []int{40, 90, 60}, []image.Rectangle{
			image.Rect(11, 100, 101, 123), image.Rect(107, 100, 197, 123), image.Rect(203, 100, 293, 123)}, 4},
		// 3*80 + 2*6 = 252 fits: the row keeps its right edge.
		{"a wider row that fits", []int{80, 40, 40}, []image.Rectangle{
			image.Rect(37, 100, 117, 123), image.Rect(123, 100, 203, 123), image.Rect(209, 100, 289, 123)}, 0},
		{"the box grows", []int{120, 40, 40}, []image.Rectangle{
			image.Rect(11, 100, 131, 123), image.Rect(137, 100, 257, 123), image.Rect(263, 100, 383, 123)}, 94},
	} {
		got, grow := fitButtonRow(row, tc.need, 300)
		if !slices.Equal(got, tc.want) || grow != tc.grow {
			t.Errorf("%s: %v grow %d, want %v grow %d", tc.name, got, grow, tc.want, tc.grow)
		}
	}
	if got, grow := fitButtonRow(nil, nil, 300); got != nil || grow != 0 {
		t.Errorf("no buttons: %v, %d", got, grow)
	}
}

func TestPlainLabel(t *testing.T) {
	for in, want := range map[string]string{"&Sí": "Sí", "Save && close": "Save & close", "Cancelar": "Cancelar", "x&": "x"} {
		if got := plainLabel(in); got != want {
			t.Errorf("plainLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
