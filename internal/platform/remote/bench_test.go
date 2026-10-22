package remote

import (
	"fmt"
	"image"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// waitReleased waits until the host released every slot of w: the host
// has copied the last Present out of shared memory.
func (w *Window) waitReleased() {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	for {
		busy := false
		for _, x := range w.busy {
			busy = busy || x
		}
		if !busy {
			return
		}
		b.mu.Unlock()
		select {
		case <-w.released:
		case <-time.After(time.Second):
		}
		b.mu.Lock()
	}
}

var benchSizes = []platform.Size{{W: 1920, H: 1080}, {W: 2880, H: 1800}}

// BenchmarkPresent measures one full-frame Present through shared memory:
// the child's copy into a slot, the message, and the host's copy out of
// it (until the slot is released). It runs the child on the benchmark
// goroutine (calling Present outside the child's Run, which is fine for a
// window whose ConfigureEvent was delivered).
func BenchmarkPresent(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(fmt.Sprintf("%vx%v", size.W, size.H), func(b *testing.B) {
			p := newPair(&testing.T{}, HostOptions{}, headless.Options{})
			defer p.host.Close()
			a := p.dial("bench").start()
			w := a.newWindow(platform.WindowOptions{Size: size})
			a.configured()
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			pattern(fb, fb.Rect, 1)
			rw := w.(*Window)
			b.SetBytes(int64(len(fb.Pix)))
			b.ResetTimer()
			for range b.N {
				a.ui(func() {
					if err := w.Present(fb, nil); err != nil {
						b.Fatal(err)
					}
				})
				rw.waitReleased()
			}
		})
	}
}

// BenchmarkPresentToScreen measures a full frame from the child's Present
// until the host presented it on its real (headless) window, which copies
// it once more.
func BenchmarkPresentToScreen(b *testing.B) {
	for _, size := range benchSizes {
		b.Run(fmt.Sprintf("%vx%v", size.W, size.H), func(b *testing.B) {
			p := newPair(&testing.T{}, HostOptions{}, headless.Options{})
			defer p.host.Close()
			a := p.dial("bench").start()
			w := a.newWindow(platform.WindowOptions{Size: size})
			a.configured()
			hw := p.window(0)
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			pattern(fb, fb.Rect, 1)
			b.SetBytes(int64(len(fb.Pix)))
			b.ResetTimer()
			for range b.N {
				n := hw.Presents()
				a.ui(func() {
					if err := w.Present(fb, nil); err != nil {
						b.Fatal(err)
					}
				})
				for hw.Presents() == n {
					time.Sleep(10 * time.Microsecond)
				}
			}
		})
	}
}

// BenchmarkEventLatency measures an input event from its injection into the
// host's window until the child's handler receives it.
func BenchmarkEventLatency(b *testing.B) {
	p := newPair(&testing.T{}, HostOptions{}, headless.Options{})
	defer p.host.Close()
	a := p.dial("bench")
	var got atomic.Int64
	a.react = func(_ *app, _ platform.Window, e platform.Event) {
		if pe, ok := e.(platform.PointerEvent); ok {
			got.Store(int64(pe.Time))
		}
	}
	a.start()
	a.newWindow(platform.WindowOptions{Size: platform.Size{W: 64, H: 64}})
	a.configured()
	hw := p.window(0)
	b.ResetTimer()
	for i := range b.N {
		hw.Inject(platform.PointerEvent{Action: platform.PointerMove, Time: time.Duration(i + 1)})
		for got.Load() != int64(i+1) {
		}
	}
}

// BenchmarkResample measures the forced-scale resampling of a full frame
// (1440x900 at scale 2 to 2880x1800 and back).
func BenchmarkResample(b *testing.B) {
	for _, k := range []samplerKey{
		{src: image.Pt(1440, 900), dst: image.Pt(2880, 1800), stretch: true},
		{src: image.Pt(2880, 1800), dst: image.Pt(1440, 900), stretch: true},
	} {
		b.Run(fmt.Sprintf("%vto%v", k.src, k.dst), func(b *testing.B) {
			s := newSampler(k)
			src := image.NewRGBA(image.Rectangle{Max: k.src})
			dst := image.NewRGBA(image.Rectangle{Max: k.dst})
			b.SetBytes(int64(len(dst.Pix)))
			for range b.N {
				s.resample(dst, src, []image.Rectangle{dst.Rect})
			}
		})
	}
}
