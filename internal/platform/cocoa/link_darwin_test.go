//go:build darwin

package cocoa

import (
	"image/color"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// TestPacing animates a window for a second with each pacing source this
// system has (the one the backend chooses is logged), checks that the
// frames come at about the display's rate, and that once nothing is
// requested no frame, no link tick and no timer follow (0% CPU when idle).
func TestPacing(t *testing.T) {
	for _, kind := range []linkKind{linkView, linkCV, linkTimer} {
		t.Run(kind.String(), func(t *testing.T) { testPacing(t, kind) })
	}
}

func testPacing(t *testing.T, kind linkKind) {
	b := newBackend(t)
	auto := b.linkKind
	switch kind {
	case linkView:
		if auto != linkView {
			t.Skipf("NSView displayLinkWithTarget:selector: is missing (macOS 13?); the backend uses %v", auto)
		}
	case linkCV:
		if err := darwin.LoadCoreVideo(); err != nil {
			t.Skipf("CoreVideo: %v", err)
		}
	}
	b.linkKind = kind

	const animate = time.Second
	var (
		w                   *Window
		frames              []time.Duration
		idleFrames          int
		ticksAtStop, ticks  int64
		stalls              int64
		running, armed      bool
		maxFPS              int
		startTicks          int64
		animStart, lastSeen time.Duration
	)
	onMain(func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: "tela pacing", Size: platform.Size{W: 160, H: 120}})
		if err != nil {
			t.Errorf("NewWindow: %v", err)
			return
		}
		w = pw.(*Window)
		time.AfterFunc(20*time.Second, b.Quit) // a failure, not a hang
		state, forced := 0, false
		_ = b.Run(platform.HandlerFunc(func(ew platform.Window, e platform.Event) {
			switch e := e.(type) {
			case platform.FrameEvent:
				presentSolid(t, ew, color.RGBA{0x30, 0x80, 0x40, 0xff})
				switch state {
				case 0:
					// The first Present shows the window; the link paces
					// only windows on screen.
					if w.shown {
						state, animStart, startTicks = 1, e.Time, w.ticks.Load()
						pool := darwin.PoolPush()
						if screen := darwin.Send(w.ns, lsel.screen); screen != 0 {
							maxFPS = darwin.SendInt(screen, darwin.Sel("maximumFramesPerSecond"))
						}
						darwin.PoolPop(pool)
					}
					ew.RequestFrame()
				case 1:
					frames = append(frames, e.Time)
					if e.Time-animStart < animate {
						ew.RequestFrame()
						return
					}
					state, lastSeen = 2, e.Time
					ticksAtStop = w.ticks.Load()
					time.AfterFunc(300*time.Millisecond, b.Wake)
				default:
					if forced {
						forced = false // the frame of a configure, not a request
					} else {
						idleFrames++
					}
				}
			case platform.ConfigureEvent:
				forced = state == 2
			case platform.WakeEvent:
				if state == 2 {
					running, armed = w.linkRunning, b.timerArmed
					ticks = w.ticks.Load() - ticksAtStop
					stalls = w.stalls.Load()
					b.Quit()
				}
			}
		}))
	})
	if w == nil {
		return
	}
	t.Logf("pacing %v (the backend chose %v); screen maximumFramesPerSecond %d", kind, auto, maxFPS)
	if len(frames) < 2 {
		t.Fatalf("got %d frames while animating, want a steady stream", len(frames))
	}
	span := frames[len(frames)-1] - frames[0]
	rate := float64(len(frames)-1) / span.Seconds()
	var worst time.Duration
	for i := 1; i < len(frames); i++ {
		worst = max(worst, frames[i]-frames[i-1])
	}
	t.Logf("%d frames in %v: %.1f Hz, longest gap %v; %d link ticks while animating, %d stand-ins by the timer",
		len(frames), span.Round(time.Millisecond), rate, worst.Round(time.Millisecond), ticksAtStop-startTicks, stalls)
	t.Logf("after the last frame (at %v): %d frames, %d link ticks in 300 ms; link running %v, timer armed %v",
		lastSeen.Round(time.Millisecond), idleFrames, ticks, running, armed)
	if rate < 20 || rate > 400 {
		t.Errorf("frame rate %.1f Hz while animating, want about the display's rate (20 to 400 Hz)", rate)
	}
	if kind != linkTimer && ticksAtStop == startTicks {
		t.Errorf("the %v never ticked: the frames were paced by the timer", kind)
	} else if kind != linkTimer && stalls > int64(len(frames))/2 {
		t.Errorf("the timer stood in for the display link %d times in %d frames: the link does not tick", stalls, len(frames))
	}
	if idleFrames != 0 {
		t.Errorf("%d frames arrived after the app stopped requesting them", idleFrames)
	}
	if running || armed {
		t.Errorf("idle: link running %v, timer armed %v; want neither (0%% CPU when idle)", running, armed)
	}
	if ticks > 2 {
		t.Errorf("the display link ticked %d times in 300 ms after the last frame; want it stopped", ticks)
	}
}
