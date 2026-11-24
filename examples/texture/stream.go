package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kuronosu/tela"
)

// Mode says who draws the scene.
type Mode string

const (
	// Auto is the GPU that draws Tela's windows (Texture.Device); software
	// where Tela draws with the CPU.
	Auto Mode = "auto"
	// Other is a GPU that does not draw Tela's windows.
	Other Mode = "other"
	// Software is the CPU.
	Software Mode = "software"
)

// painter draws frames of the scene and submits them. One goroutine uses it.
type painter interface {
	// paint draws the scene and submits it as a frame, waiting for a free
	// buffer (until ctx is done).
	paint(ctx context.Context, u *uniforms) error
	// about says what it draws with.
	about() string
	// close frees what it has, once Tela let go of all of it: while its last
	// frame is on screen it waits for another painter's, or for the texture
	// to close.
	close()
}

// openPainter returns the painter of a mode, for a texture that a device
// shows, and a note when it is not the one the mode asks for.
func openPainter(tex *tela.Texture, dev tela.TextureDevice, mode Mode) (painter, string) {
	switch {
	case mode == Software:
		return newSoftware(tex), ""
	case mode == Auto && !dev.GPU:
		return newSoftware(tex), "Tela draws with the CPU (build with -tags telagpu)"
	}
	p, err := newGPU(tex, dev, mode == Other)
	if err != nil {
		return newSoftware(tex), softwareNote(err)
	}
	return p, ""
}

func softwareNote(err error) string { return "Drawn in software: " + err.Error() }

// stream draws the scene until ctx is done or the texture is closed: a frame
// for each frame of the window, and none while paused, unless the size
// changes. NextFrame blocks while the texture is out of
// sight, so there is no timer, and nothing runs at rest.
func stream(ctx context.Context, tex *tela.Texture, ctl *controls, mode Mode, report func(Status)) {
	ft, err := tex.NextFrame(ctx) // the window is open: its device is known
	if err != nil {
		return
	}
	dev := tex.Device()
	p, note := openPainter(tex, dev, mode)
	defer func() { go p.close() }() // it waits for Tela
	// use makes another painter draw from now on. The frame on screen is
	// the old one's, which closes when it is replaced.
	use := func(next painter, why string) {
		go p.close()
		p, note = next, why
	}
	m := newMeter(tex)
	for {
		moving := ctl.advance(ft.Time)
		size := tex.Size()
		if size.Width > 0 {
			u := sceneAt(ctl.now(), size.Width, size.Height)
			start := time.Now()
			err := p.paint(ctx, &u)
			switch {
			case err == nil:
			case ctx.Err() != nil || errors.Is(err, tela.ErrTextureClosed):
				return
			case p.about() == softwareName:
				report(Status{Note: err.Error()})
				return
			default:
				use(newSoftware(tex), softwareNote(err))
				continue
			}
			m.frame(ft, time.Since(start))
		}
		if m.due(ft.Time) || !moving {
			// Tela may have lost its GPU, and draws with the CPU since.
			if now := tex.Device(); now.GPU != dev.GPU || now.UUID != dev.UUID {
				dev = now
				next, why := openPainter(tex, dev, mode)
				use(next, why)
				continue
			}
			report(m.status(dev, p, note, size, ft, moving))
		}
		if !moving {
			for again := false; !again; again = ctl.playing() || tex.Size() != size {
				if !ctl.wait(ctx) {
					return
				}
			}
		}
		if ft, err = tex.NextFrame(ctx); err != nil {
			return
		}
	}
}

// controls is what the window tells the loop, and the scene's clock. Safe
// from any goroutine.
type controls struct {
	mu     sync.Mutex
	paused bool
	speed  float64
	at     time.Duration // the scene's clock
	last   time.Time     // the time of the frame it was advanced to
	wake   chan struct{} // what a paused loop waits for: something changed
}

func newControls(at time.Duration) *controls {
	return &controls{speed: 1, at: at, wake: make(chan struct{}, 1)}
}

// set pauses or plays, at a speed.
func (c *controls) set(paused bool, speed float64) {
	c.mu.Lock()
	c.paused, c.speed = paused, speed
	c.mu.Unlock()
	c.changed()
}

// changed makes a paused loop look at whether it should draw.
func (c *controls) changed() {
	select {
	case c.wake <- struct{}{}:
	default: // it will
	}
}

// wait blocks until changed is called; it reports false when ctx is done.
func (c *controls) wait(ctx context.Context) bool {
	select {
	case <-c.wake:
		return true
	case <-ctx.Done():
		return false
	}
}

// advance moves the clock to the frame at t, and reports whether it moves.
func (c *controls) advance(t time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.paused && !c.last.IsZero() {
		// The time out of sight, or paused, is not the scene's.
		dt := min(max(t.Sub(c.last), 0), 100*time.Millisecond)
		c.at += time.Duration(float64(dt) * c.speed)
	}
	c.last = t
	return !c.paused
}

func (c *controls) playing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.paused
}

func (c *controls) now() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

// Status is what the window says of the frames.
type Status struct {
	// Device is what Tela draws with, Painter what the scene is drawn with,
	// and Note why that is not what was asked for.
	Device, Painter, Note string
	// Still is a picture that is drawn once (Demo.Still), without a loop;
	// Moving a scene that is not paused.
	Still, Moving bool
	// FPS is the frames shown per second, of a display that shows Refresh.
	FPS, Refresh float64
	Size         tela.TextureSize
	// Draw is the CPU's time for a frame, and Age the time from its
	// submission to its presentation.
	Draw, Age      time.Duration
	Shown, Dropped uint64
}

// badge is the label over the scene.
func (s Status) badge() string {
	switch {
	case s.Still:
		return "Still"
	case !s.Moving:
		return "Paused"
	case s.FPS == 0:
		return "Starting" // nothing measured yet
	}
	return fmt.Sprintf("%.0f fps", s.FPS)
}

// painter is the line on what draws the scene.
func (s Status) painter() string {
	if s.Note == "" {
		return s.Painter
	}
	return s.Painter + ". " + s.Note
}

// frames is the line of numbers.
func (s Status) frames() string {
	if s.Size.Width == 0 {
		return ""
	}
	out := fmt.Sprintf("%d×%d at scale %g", s.Size.Width, s.Size.Height, s.Size.Scale)
	if !s.Moving {
		return out
	}
	if s.Refresh > 0 {
		out += fmt.Sprintf(", %.0f Hz", s.Refresh)
	}
	return out + fmt.Sprintf(", %.1f ms to draw, %.1f ms to the screen, %d shown, %d dropped", ms(s.Draw), ms(s.Age), s.Shown, s.Dropped)
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// reportEvery is how often the numbers change while the scene moves.
const reportEvery = 500 * time.Millisecond

// meter measures the frames between two reports.
type meter struct {
	tex   *tela.Texture
	start time.Time
	shown uint64
	drawn time.Duration
	n     int
}

func newMeter(tex *tela.Texture) *meter { return &meter{tex: tex} }

// frame counts a frame that took d to draw.
func (m *meter) frame(ft tela.FrameTime, d time.Duration) {
	if m.start.IsZero() {
		m.start = ft.Time
	}
	m.drawn += d
	m.n++
}

// due reports whether it is time to report. The first one is soon: what
// draws the scene is known by then.
func (m *meter) due(t time.Time) bool {
	return m.n == 1 && m.shown == 0 || !m.start.IsZero() && t.Sub(m.start) >= reportEvery
}

// status is the report, and starts the next one.
func (m *meter) status(dev tela.TextureDevice, p painter, note string, size tela.TextureSize, ft tela.FrameTime, moving bool) Status {
	st := m.tex.Stats()
	s := Status{
		Device: "the CPU", Painter: p.about(), Note: note, Moving: moving, Size: size,
		Age: st.Age, Shown: st.Shown, Dropped: st.Dropped,
	}
	if dev.GPU {
		s.Device = dev.Name
		if dev.RenderNode != "" {
			s.Device += " (" + dev.RenderNode + ")"
		}
	}
	if ft.Refresh > 0 {
		s.Refresh = float64(time.Second) / float64(ft.Refresh)
	}
	if d := ft.Time.Sub(m.start); d > 0 {
		s.FPS = float64(st.Shown-m.shown) / d.Seconds()
	}
	if m.n > 0 {
		s.Draw = m.drawn / time.Duration(m.n)
	}
	*m = meter{tex: m.tex, start: ft.Time, shown: st.Shown}
	return s
}
