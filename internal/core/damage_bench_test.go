package core

import (
	"fmt"
	"testing"
)

// layeredScrollList is a scrolling list of 300 rows that are layers of
// their own (horizontal scroll containers), so a scrolled run holds many
// chunk instances.
func layeredScrollList() Node {
	rows := []Child{Key("list"), Class("h-screen overflow-y-auto")}
	for i := range 300 {
		rows = append(rows, div(Key(fmt.Sprintf("r%d", i)), Class("h-8 px-2 bg-slate-100 text-sm overflow-x-auto"),
			div(Class("w-[1200px]"), Textf("Row %d", i))))
	}
	return div(rows...)
}

// BenchmarkScrollFrameRasterLayers is BenchmarkScrollFrameRaster10k with
// a list whose rows are layers: the damage looks at many chunk instances
// per scrolled run (damager.origin).
func BenchmarkScrollFrameRasterLayers(b *testing.B) {
	tr := newTestTree(b, 800, 600, layeredScrollList(), nil)
	rasterBench(b, tr, 800, 600, 2, false, scrollChange(b, tr))
}
