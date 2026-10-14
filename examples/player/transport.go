package main

import (
	"time"

	"github.com/kuronosu/tela"
)

// Track is a song of the queue.
type Track struct {
	Title, Artist string
	Seconds       int
	// From and To are the Tailwind gradient colors of its cover art
	// (from-violet-500 to-fuchsia-600).
	From, To string
}

var tracks = []Track{
	{"Midnight Drive", "Neon Harbor", 222, "from-violet-500", "to-fuchsia-600"},
	{"Paper Planes", "The Lanterns", 188, "from-sky-400", "to-indigo-600"},
	{"Slow Tide", "Marlowe", 255, "from-teal-400", "to-emerald-700"},
	{"Golden Hour", "Aster & Pine", 177, "from-amber-400", "to-rose-600"},
	{"Static Bloom", "Okabe", 211, "from-rose-400", "to-purple-700"},
	{"Low Orbit", "Neon Harbor", 302, "from-cyan-400", "to-slate-700"},
}

// Timing of the progress clock. The tests change them to run faster.
var (
	// tickEvery is how often the clock goroutine advances the progress.
	tickEvery = 100 * time.Millisecond
	// timeScale is how many seconds of music pass per second of real time.
	timeScale = 1.0
)

// Transport is the state of the player: the queue position, whether it
// plays, and the progress. They are [tela.Value]s, not UseState, because a
// goroutine and several components need them:
//
//   - Elapsed (seconds, fractional) is read only inside WidthFn and
//     TranslateFn: when it changes, the progress bar is laid out again and
//     nothing is built.
//   - Secs (whole seconds) changes once per second and is the only thing
//     the time label reads, in a TextFn, so its paragraph is laid out once
//     per second instead of once per tick.
//   - Track and Playing are read in component bodies: a change rebuilds
//     the components that show them.
//
// A *Transport is created once per player (see useTransport) and is
// comparable, so it can sit in the props of CP components.
type Transport struct {
	Track   *tela.Value[int]
	Playing *tela.Value[bool]
	Elapsed *tela.Value[float64]
	Secs    *tela.Value[int]
}

// useTransport is a custom hook: it calls hooks, so it must run in a
// component body, in the same order on every build. It also starts the
// clock goroutine, which stops when the player unmounts.
func useTransport(startAt float64) *Transport {
	tr := tela.UseMemo(func() *Transport { return &Transport{} }, nil)
	// UseValue must be called on every build, in order, even though the
	// Transport itself is created once.
	tr.Track = tela.UseValue(0)
	tr.Playing = tela.UseValue(true)
	tr.Elapsed = tela.UseValue(startAt)
	tr.Secs = tela.UseValue(int(startAt))

	tela.UseEffect(func() func() {
		done := make(chan struct{})
		go func() {
			tick := time.NewTicker(tickEvery)
			defer tick.Stop()
			last := time.Now()
			for {
				select {
				case now := <-tick.C:
					dt := now.Sub(last).Seconds() * timeScale
					last = now
					// Post runs on the UI thread, in order with the Value
					// updates, so Get sees the state left by the previous ones.
					tela.Post(func() { tr.advance(dt) })
				case <-done:
					return
				}
			}
		}()
		return func() { close(done) }
	}, nil)
	return tr
}

// Current returns the track that is selected.
func (tr *Transport) Current() Track { return tracks[tr.Track.Get()] }

// advance moves the progress by dt seconds while playing and goes to the
// next track at the end. It runs on the UI thread.
func (tr *Transport) advance(dt float64) {
	if !tr.Playing.Get() {
		return
	}
	e := tr.Elapsed.Get() + dt
	if e >= float64(tr.Current().Seconds) {
		tr.Next()
		return
	}
	tr.Elapsed.Set(e)
	if s := int(e); s != tr.Secs.Get() {
		tr.Secs.Set(s)
	}
}

// Select plays track i from its start.
func (tr *Transport) Select(i int) {
	tr.Track.Set(i)
	tr.Playing.Set(true)
	tr.Seek(0)
}

// Next plays the following track, wrapping around.
func (tr *Transport) Next() { tr.Select((tr.Track.Get() + 1) % len(tracks)) }

// Prev restarts the track, or goes to the previous one when it has just
// started (the convention of every player).
func (tr *Transport) Prev() {
	if tr.Elapsed.Get() > 3 {
		tr.Seek(0)
		return
	}
	tr.Select((tr.Track.Get() + len(tracks) - 1) % len(tracks))
}

// Toggle plays or pauses.
func (tr *Transport) Toggle() { tr.Playing.Update(func(p bool) bool { return !p }) }

// Seek moves the progress to a fraction (0 to 1) of the current track.
func (tr *Transport) Seek(frac float64) {
	frac = min(max(frac, 0), 0.999)
	e := frac * float64(tr.Current().Seconds)
	tr.Elapsed.Set(e)
	tr.Secs.Set(int(e))
}
