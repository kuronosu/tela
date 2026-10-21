package layout

import (
	"hash/fnv"
	"math"
	"math/rand/v2"
	"testing"
)

// wantLayoutDigest is layoutDigest() recorded on amd64 (GOAMD64=v1, no
// fused multiply-adds). Layout output feeds the rasterizer, so it must be
// bit-identical on every architecture (ADR-027); check with
//
//	GOARCH=arm64 go test -short -exec qemu-aarch64-static -run Deterministic ./internal/layout/
//	GOAMD64=v3 go test -run Deterministic ./internal/layout/
const wantLayoutDigest uint64 = 0xacb86b7e328e6863

// layoutDigest hashes the unrounded and rounded layouts of the Taffy
// fixtures, Tela's cases and random trees with fractional sizes.
func layoutDigest(t *testing.T) uint64 {
	h := fnv.New64a()
	put := func(vs ...float32) {
		for _, v := range vs {
			b := math.Float32bits(v)
			h.Write([]byte{byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24)})
		}
	}
	var hashTree func(n *testNode)
	hashTree = func(n *testNode) {
		for _, l := range []*Layout{n.data.Layout(), n.data.RoundedLayout()} {
			put(l.Location.X, l.Location.Y, l.Size.W, l.Size.H, l.FirstBaseline,
				l.ScrollableOverflow.X0, l.ScrollableOverflow.Y0, l.ScrollableOverflow.X1, l.ScrollableOverflow.Y1,
				l.Margin.Left, l.Margin.Right, l.Margin.Top, l.Margin.Bottom, l.Padding.Left, l.Padding.Top)
		}
		for _, c := range n.children {
			hashTree(c)
		}
	}
	for _, set := range []fixtureSet{taffyFixtures, telaCases} {
		for _, name := range set.files(t) {
			ft, err := buildFixture(set.source(t, name), 800, 513)
			if err != nil || len(ft.unsupported) > 0 {
				continue
			}
			var e Engine
			e.Compute(ft.viewport, DefiniteSpace(800), DefiniteSpace(513))
			Round(ft.viewport)
			hashTree(ft.viewport)
		}
	}
	rng := rand.New(rand.NewPCG(5, 5))
	for i := 0; i < 200; i++ {
		root := randomTree(rng, 4)
		root.style.Padding = EdgesAll(Px(float32(rng.Float64() * 7)))
		var e Engine
		w := float32(100 + rng.Float64()*500)
		e.Compute(root, DefiniteSpace(w), DefiniteSpace(w*0.7))
		Round(root)
		hashTree(root)
	}
	return h.Sum64()
}

func TestLayoutDeterministic(t *testing.T) {
	if got := layoutDigest(t); got != wantLayoutDigest {
		t.Errorf("layoutDigest = %#x, want %#x (the same on every architecture)", got, wantLayoutDigest)
	}
}
