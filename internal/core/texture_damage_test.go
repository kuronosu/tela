package core

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
)

// noiseImage returns a w x h image of premultiplied pixels of random colors
// in blocks, some translucent.
func noiseImage(rng *rand.Rand, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	block := 1 + rng.IntN(8)
	var c color.RGBA
	for y := range h {
		for x := range w {
			if x%block == 0 {
				r := rand.New(rand.NewPCG(uint64(x/block), uint64(y/block)))
				a := uint8(255)
				if rng.IntN(16) == 0 {
					a = 128
				}
				p := func() uint8 { return uint8(int(r.IntN(256)) * int(a) / 255) }
				c = color.RGBA{p(), p(), p(), a}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// textureScene shows textures under what an element can have around it and
// over it: rounded corners, object-fit, opacity, transforms, filters, blend
// modes, masks, a backdrop filter over them, a list that scrolls and a layer
// moved by a Value.
func textureScene(st *sceneState, texs *[]*Texture) Node {
	comp := func() Node {
		st.refs[0] = UseElementRef()
		st.values[0] = UseValue[float32](0)
		st.values[1] = UseValue[float32](1)
		v, set := UseState(0)
		st.setters[0] = set
		x, o := st.values[0], st.values[1]
		const n = 9
		ts := make([]*Texture, n)
		for i := range ts {
			ts[i] = UseTexture()
		}
		*texs = ts
		cell := func(i int, class string, over ...Child) Node {
			return div(append([]Child{Class("relative m-1 h-20 w-24 shrink-0"), texture(ts[i], Class(class))}, over...)...)
		}
		list := []Child{AttachRef(st.refs[0]), Key("list"), Class("h-28 overflow-y-auto bg-white")}
		for i := range 12 {
			if i == 3 {
				list = append(list, texture(ts[8], Key("tex"), Class("m-2 h-16 w-32 rounded-xl object-contain bg-slate-200")))
			}
			list = append(list, div(Key(i), Class("h-7 px-2 text-sm odd:bg-slate-50 hover:bg-sky-100"), Textf("Row %d", i)))
		}
		kids := []Child{Class("h-screen overflow-y-auto bg-linear-to-br from-white to-indigo-200"),
			div(Class("flex flex-wrap"),
				cell(0, "size-full rounded-2xl object-cover border-2 border-indigo-500 shadow-lg"),
				cell(1, "size-full object-contain object-left-top opacity-60 bg-amber-200"),
				cell(2, "size-full rotate-12 rounded-lg object-none"),
				cell(3, "size-full blur-[2px] object-cover"),
				cell(4, "size-full mix-blend-multiply p-2"),
				cell(5, "size-full mask-linear-to-r mask-from-50% rounded-full"),
				cell(6, "size-full object-cover", div(Class("absolute inset-3 rounded-xl border border-white/50 bg-white/20 p-1 text-xs backdrop-blur-sm"), Text("glass"))),
			),
			div(list...),
			div(Class("m-2 h-16 w-40 rounded-xl overflow-hidden"),
				Style(style.TranslateFn(func() (float32, float32) { return x.Get(), 0 }), style.OpacityFn(func() float32 { return o.Get() })),
				texture(ts[7], Class("size-full object-cover hover:scale-110 transition"))),
			div(Class("h-40 px-4 text-sm text-slate-500"), Text("Below")),
		}
		if v%4 == 3 {
			kids = append(kids, texture(ts[7], Key("again"), Class("h-12 w-full"))) // a texture shown twice
		}
		return div(kids...)
	}
	return Component(comp)
}

// Incremental frames are bit-identical to full ones while frames of the
// textures arrive.
func TestTextureDamageMatchesFullRender(t *testing.T) {
	t.Parallel()
	steps := 120
	if testing.Short() {
		steps = 30
	}
	var texs []*Texture
	ops := func(x *damageHarness, st *sceneState) string {
		if x.rng.IntN(3) == 0 {
			return commonOps(x, st)
		}
		what := "textures"
		for range 1 + x.rng.IntN(4) {
			i := x.rng.IntN(len(texs))
			img := noiseImage(x.rng, 1+x.rng.IntN(90), 1+x.rng.IntN(60))
			if err := texs[i].SubmitImage(img, nil); err != nil {
				x.t.Fatal(err)
			}
			what += fmt.Sprintf(" %d:%v", i, img.Rect.Size())
		}
		return what
	}
	root := func(st *sceneState) Node { return textureScene(st, &texs) }
	runSceneWith(t, "texture", root, ops, Config{InstantScroll: true}, steps, sceneScales(0), nil)
	runSceneWith(t, "texture_smooth", root, ops, Config{}, steps, sceneScales(4), nil)
}

// TestTextureRace runs producers, readers and a NextFrame loop on goroutines
// while the UI thread draws, hides and unmounts: for the race detector, and
// for the account of the releases.
func TestTextureRace(t *testing.T) {
	t.Parallel()
	for _, quit := range []bool{false, true} {
		a := texApp{class: "w-40 h-20 bg-black rounded-xl object-cover"}
		b := headless.New(headless.Options{Size: platform.Size{W: 200, H: 160}})
		h, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: a.root()})
		if err != nil {
			t.Fatal(err)
		}
		imp := &fakeTextures{}
		h.setTextureImporter(imp)
		b.Drain(h)
		tex := a.tex
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		var calls, released atomic.Int64
		run := func(f func(rng *rand.Rand)) {
			wg.Add(1)
			seed := uint64(calls.Load())
			go func() {
				defer wg.Done()
				rng := rand.New(rand.NewPCG(seed, 1))
				for ctx.Err() == nil {
					f(rng)
					time.Sleep(time.Duration(rng.IntN(200)) * time.Microsecond)
				}
			}()
		}
		check := func(err error) {
			if err != nil && !errors.Is(err, ErrTextureClosed) {
				t.Error(err)
			}
		}
		// Pixels written again as soon as they are released.
		pool := make(chan *image.RGBA, 8)
		for range 4 {
			pool <- image.NewRGBA(image.Rect(0, 0, 32, 16))
		}
		run(func(rng *rand.Rand) {
			select {
			case img := <-pool:
				for i := range img.Pix {
					img.Pix[i] = 255 - byte(i)&3
				}
				calls.Add(1)
				check(tex.SubmitImage(img, func() { released.Add(1); pool <- img }))
			default:
			}
		})
		run(func(rng *rand.Rand) { check(tex.SubmitImage(solid(1+rng.IntN(20), 1+rng.IntN(20), texRed), nil)) })
		// Buffers with fences that signal, fail and are cancelled.
		for range 2 {
			run(func(rng *rand.Rand) {
				buf, err := tex.Import(Dmabuf{Width: 8, Height: 8, Fourcc: fourccAR24, Modifier: 42, Planes: []DmabufPlane{{FD: 3, Stride: 32}}})
				if err != nil {
					check(err)
					return
				}
				f := newChanFence()
				calls.Add(1)
				check(tex.Submit(buf, TextureFrame{Fence: f, Release: func() { released.Add(1) }}))
				switch rng.IntN(3) {
				case 0:
					f.ch <- errors.New("failed")
				default:
					time.Sleep(time.Duration(rng.IntN(300)) * time.Microsecond)
					f.ch <- nil
				}
				check(buf.Close())
			})
		}
		run(func(*rand.Rand) {
			_, err := tex.NextFrame(ctx)
			if err != nil && ctx.Err() == nil {
				check(err)
			}
		})
		run(func(rng *rand.Rand) {
			_, _, _ = tex.Size(), tex.Stats(), tex.Device()
			if rng.IntN(5) == 0 {
				tex.OnResize(func(TextureSize) {})
			}
		})
		modes := []string{"", "none", "", "gone"}
		for i := range 120 {
			switch {
			case i%10 == 5:
				a.mode.Set(modes[i/10%len(modes)])
			case i%30 == 7:
				WindowHandle{h.Tree()}.Minimize()
			case i%30 == 11:
				WindowHandle{h.Tree()}.Focus()
			}
			b.Drain(h)
			b.Advance(b.FrameInterval())
			b.Drain(h)
			time.Sleep(100 * time.Microsecond)
		}
		if quit {
			h.End()
		} else {
			a.closed.Set(true)
			b.Drain(h)
			b.Advance(b.FrameInterval())
			b.Drain(h)
		}
		time.Sleep(time.Millisecond) // the producers meet a closed texture
		cancel()
		wg.Wait()
		waitFor(t, "every Release", func() bool { return released.Load() == calls.Load() })
		time.Sleep(time.Millisecond)
		if released.Load() != calls.Load() {
			t.Errorf("%d Releases for %d calls", released.Load(), calls.Load())
		}
		imp.mu.Lock()
		for _, c := range imp.imported {
			waitFor(t, "the buffers to be freed", func() bool { _, closed := c.state(); return closed == 1 })
		}
		imp.mu.Unlock()
		if st := tex.Stats(); st.Submitted != st.Shown+st.Dropped {
			t.Errorf("stats %+v: a closed texture has nothing pending", st)
		}
		h.stopTimer()
	}
}

// BenchmarkTextureFrame is a frame of a 1280x720 texture, at its own size
// and scaled, in a window of 800x450 at scale 2, through the CPU path:
// SubmitImage, the window's frame and the rasterizer. "copy" has SubmitImage
// copy the pixels (no Release).
func BenchmarkTextureFrame(b *testing.B) {
	for _, bc := range []struct {
		name, class string
		copy        bool
	}{
		{"1to1", "w-[640px] h-[360px]", false},
		{"1to1-copy", "w-[640px] h-[360px]", true},
		{"rounded-cover", "w-[700px] h-[400px] rounded-2xl object-cover", false},
	} {
		b.Run(bc.name, func(b *testing.B) {
			var tex *Texture
			tr := NewTree(Config{Fonts: testFonts})
			tr.SetViewport(gfx.Size{W: 800, H: 450})
			tr.SetScale(2)
			tr.Mount(Component(func() Node {
				tex = UseTexture()
				return div(Class("p-4 bg-slate-100 h-screen"), texture(tex, Class(bc.class)))
			}))
			fb := image.NewRGBA(image.Rect(0, 0, 1600, 900))
			var r raster.Renderer
			imgs := []*image.RGBA{solid(1280, 720, texRed), solid(1280, 720, texGreen), solid(1280, 720, texBlue)}
			release := func() {}
			if bc.copy {
				release = nil
			}
			frame := func(i int) {
				if err := tex.SubmitImage(imgs[i%len(imgs)], release); err != nil {
					b.Fatal(err)
				}
				tr.ApplyUpdates()
				tr.Frame()
				tr.RasterizeFrame(fb, &r, false)
				tr.texturesPresented()
			}
			for i := range 4 {
				frame(i)
			}
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				frame(i)
				i++
			}
			if st := tex.Stats(); st.Dropped != 0 {
				b.Fatalf("stats %+v", st)
			}
		})
	}
}
