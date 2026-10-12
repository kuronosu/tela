package style

import (
	"fmt"
	"testing"
)

// fifteen is a typical class string of a button or card.
const fifteen = "flex flex-col items-center gap-6 p-10 rounded-3xl bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl text-5xl font-bold text-white tracking-tight px-6 py-3"

// BenchmarkParseCold parses a 15-class string that is not in the cache, so
// every iteration pays for the parse and the intern.
func BenchmarkParseCold(b *testing.B) {
	p := NewParser(DefaultTheme())
	strs := make([]string, 4096)
	for i := range strs {
		strs[i] = fmt.Sprintf("%s w-[%dpx]", fifteen, i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Compile(strs[i%len(strs)])
		if i%len(strs) == len(strs)-1 {
			b.StopTimer()
			p = NewParser(DefaultTheme())
			b.StartTimer()
		}
	}
}

// BenchmarkParseCached is the steady state: a build that reuses a string.
func BenchmarkParseCached(b *testing.B) {
	p := NewParser(DefaultTheme())
	p.Compile(fifteen)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Compile(fifteen)
	}
}

func BenchmarkParseCachedParallel(b *testing.B) {
	p := NewParser(DefaultTheme())
	p.Compile(fifteen)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p.Compile(fifteen)
		}
	})
}

// BenchmarkCompute resolves the style of an element with the 15 classes.
func BenchmarkCompute(b *testing.B) {
	cl := Compile(fifteen)
	if cl.Err != nil {
		b.Fatal(cl.Err)
	}
	parent := Compute(testCx, Input{Kind: KindRoot})
	in := Input{Kind: KindDiv, Parent: parent, Classes: []*Classes{cl}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Compute(testCx, in)
	}
}

// BenchmarkComputeLeaf is the common case: a text span with no classes.
func BenchmarkComputeLeaf(b *testing.B) {
	parent := Compute(testCx, Input{Kind: KindRoot})
	in := Input{Kind: KindText, Parent: parent}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Compute(testCx, in)
	}
}

// BenchmarkSuggest times the lazy "did you mean" search over the whole vocabulary.
func BenchmarkSuggest(b *testing.B) {
	p := DefaultParser()
	p.Vocabulary()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Suggest("hover:rounded-xxl")
	}
}
