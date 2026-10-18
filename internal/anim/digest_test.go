package anim

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// TestDeterminismDigest pins the bits of the animation math (ADR-041,
// ADR-027): curves, springs, interpolation and keyframes give the same
// results on every architecture, so animated pixels do not depend on the
// CPU. The digest is checked by `GOARCH=arm64 go test -exec
// qemu-aarch64-static ./internal/anim/` too. A change that moves it on
// purpose updates the constant.
func TestDeterminismDigest(t *testing.T) {
	h := sha256.New()
	put := func(vs ...float64) {
		for _, v := range vs {
			var b [8]byte
			binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
			h.Write(b[:])
		}
	}
	putf := func(vs ...float32) {
		for _, v := range vs {
			put(float64(v))
		}
	}
	section := func(name string) { t.Logf("%-10s %x", name, h.Sum(nil)[:6]) }
	for _, c := range []Curve{Ease, EaseIn, EaseOut, EaseInOut, TailwindEaseIn, TailwindEaseOut, TailwindEaseInOut,
		CubicBezier{0.8, 0, 1, 1}, CubicBezier{0.34, 1.56, 0.64, 1}, Steps{5, JumpBoth}} {
		for i := 0; i <= 200; i++ {
			put(c.At(float64(i) / 200))
		}
	}
	section("curves")
	for _, s := range []Spring{DefaultSpring(), {Stiffness: 100, Damping: 20}, {Stiffness: 100, Damping: 45},
		{Stiffness: 170, Damping: 26, Mass: 1.5, Velocity: 120}, SpringForDuration(500*time.Millisecond, 0.3),
		SpringForDuration(800*time.Millisecond, 0), SpringForDurationVelocity(300*time.Millisecond, 0.5, 2)} {
		put(s.Stiffness, s.Damping, s.Mass)
		for i := 0; i <= 120; i++ {
			x, v := s.At(float64(i)/40, 3, 77)
			put(x, v)
		}
		put(float64(s.Duration(0, 100, DefaultRest(100))))
	}
	section("springs")
	red := gfx.OKLCH(0.637, 0.237, 25.331, 1)
	blue := gfx.OKLCH(0.623, 0.214, 259.815, 0.8)
	for _, p := range [][2]gfx.Color{{red, blue}, {gfx.White, gfx.Black}, {gfx.Transparent, blue}, {gfx.RGB8(253, 224, 71), gfx.RGB8(30, 58, 138)}} {
		for i := 0; i <= 50; i++ {
			c := LerpColor(p[0], p[1], float64(i)/50)
			putf(c.R, c.G, c.B, c.A)
		}
	}
	section("colors")
	a := IdentityTransform()
	b := a
	b.TranslateX, b.TranslateY, b.Rotate, b.ScaleX, b.ScaleY = style.PxLP(33), style.PctLP(-25), 133, 1.7, 0.4
	for i := 0; i <= 30; i++ {
		m := LerpTransform(a, b, float64(i)/30)
		putf(m.TranslateX.Px, m.TranslateY.Pct, m.Rotate, m.ScaleX, m.ScaleY)
	}
	section("transform")
	for _, name := range []string{"spin", "ping", "pulse", "bounce"} {
		k, _ := Builtin(name)
		for i := 0; i <= 100; i++ {
			v := k.At(float64(i)/100, nil, 0.9)
			putf(v.Opacity, v.Transform.Rotate, v.Transform.ScaleX, v.Transform.TranslateY.Pct)
		}
	}
	section("keyframes")
	tm := Timing{Duration: 300 * time.Millisecond, Delay: 20 * time.Millisecond, Curve: TailwindDefault}
	tr := Begin(PropOpacity, tm, 0)
	for i := 0; i < 24; i++ {
		now := time.Duration(i) * 14 * time.Millisecond
		e, _ := tr.Progress(now)
		rv := tr.Reverse(tm, now)
		put(e, rv.ReverseFactor, float64(rv.Start), float64(rv.Duration))
		tr = rv
	}
	section("transitions")
	sh := []style.Shadow{{OffsetY: 4, Blur: 6, Spread: -1, Color: gfx.Black.WithAlpha(0.1)}}
	ring := []style.Shadow{{Spread: 3, Color: blue}, {OffsetY: 20, Blur: 25, Spread: -5, Color: gfx.Black.WithAlpha(0.25)}}
	for i := 0; i <= 20; i++ {
		out, _ := LerpShadows(sh, ring, float64(i)/20)
		for _, s := range out {
			putf(s.OffsetX, s.OffsetY, s.Blur, s.Spread, s.Color.R, s.Color.G, s.Color.B, s.Color.A)
		}
	}
	const want = "bc956fb933faafe28474eb329ce3a1260e9031db76582b853f5069240bbd203d"
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		t.Fatalf("digest %s, want %s", got, want)
	}
}
