package core

import "testing"

// The cost of the overlays (debug.go) on the 10k-element scroll frame of
// scroll_bench_test.go. With none on, the numbers are those of
// BenchmarkScrollFrameRaster10k and BenchmarkIdleFrame10k: the overlays
// cost nothing when off.

func benchDebugScroll(b *testing.B, flags DebugFlags) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, scrollList(setters), nil)
	tr.SetDebug(flags)
	rasterBench(b, tr, 800, 600, 2, false, scrollChange(b, tr))
}

func BenchmarkDebugScrollFrameRaster10kOff(b *testing.B)       { benchDebugScroll(b, 0) }
func BenchmarkDebugScrollFrameRaster10kLayout(b *testing.B)    { benchDebugScroll(b, DebugLayout) }
func BenchmarkDebugScrollFrameRaster10kLayers(b *testing.B)    { benchDebugScroll(b, DebugLayers) }
func BenchmarkDebugScrollFrameRaster10kSemantics(b *testing.B) { benchDebugScroll(b, DebugSemantics) }
func BenchmarkDebugScrollFrameRaster10kFlash(b *testing.B)     { benchDebugScroll(b, DebugPaintFlash) }
func BenchmarkDebugScrollFrameRaster10kFPS(b *testing.B)       { benchDebugScroll(b, DebugFPS) }
func BenchmarkDebugScrollFrameRaster10kAll(b *testing.B)       { benchDebugScroll(b, DebugAll) }

// BenchmarkDebugIdleFrame10k is a frame with nothing dirty, with and
// without the idle overlays: the application's frame costs the same.
func BenchmarkDebugIdleFrame10k(b *testing.B) {
	for _, tc := range []struct {
		name  string
		flags DebugFlags
	}{{"off", 0}, {"layout", DebugLayout}, {"layers", DebugLayers}, {"semantics", DebugSemantics}} {
		b.Run(tc.name, func(b *testing.B) {
			setters := make([]func(string), benchRows)
			tr := newTestTree(b, 800, 600, bigList(benchRows, setters), nil)
			tr.SetDebug(tc.flags)
			tr.Frame()
			b.ReportAllocs()
			for b.Loop() {
				tr.Frame()
			}
		})
	}
}
