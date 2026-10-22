//go:build teladev

package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// The cost of the dev state on a large tree: 10,000 keyed rows, each a
// component with two states and a Value, two spans and their texts (about
// 50,000 Elements and 30,000 slots).
//
//	go test -tags teladev -run xxx -bench DevState -benchtime 5x ./internal/core/

func devBenchRow(i int) Node {
	n, _ := UseState(i)
	s, _ := UseState("row")
	v := UseValue(float32(i))
	return div(Class("flex gap-2 h-[20px]"), span(Textf("%d %g", n, v.Get())), span(Text(s)))
}

func devBenchPage() Node {
	ids := make([]int, 10_000)
	for i := range ids {
		ids[i] = i
	}
	return div(Class("flex flex-col"), Map(ids, func(i int) Node { return ComponentProps(devBenchRow, i, Key(i)) }))
}

func devBenchTree(data []byte) *Tree {
	tr := NewTree(Config{Fonts: testFonts})
	tr.SetViewport(gfx.Size{W: 800, H: 600})
	if data != nil {
		tr.SeedDevState(data)
	}
	tr.Mount(Component(devBenchPage))
	return tr
}

// BenchmarkDevStateCapture is CaptureDevState: the walk, the checks and
// the encoding.
func BenchmarkDevStateCapture(b *testing.B) {
	tr := devBenchTree(nil)
	tr.Frame()
	var size int
	for b.Loop() {
		data, _, err := tr.CaptureDevState()
		if err != nil {
			b.Fatal(err)
		}
		size = len(data)
	}
	b.ReportMetric(float64(size), "bytes")
}

// BenchmarkDevStateMount is the first Mount without a snapshot, and
// BenchmarkDevStateSeed with one: the difference is the decoding and the
// seeding.
func BenchmarkDevStateMount(b *testing.B) {
	for b.Loop() {
		devBenchTree(nil)
	}
}

func BenchmarkDevStateSeed(b *testing.B) {
	tr := devBenchTree(nil)
	tr.Frame()
	data, _, err := tr.CaptureDevState()
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		tr := devBenchTree(data)
		if r := tr.DevSeedReport(); r.Restored != 30_000 {
			b.Fatalf("restored %d slots", r.Restored)
		}
	}
}
