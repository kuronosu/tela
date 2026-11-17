package update

import (
	"context"
	"errors"

	"github.com/kuronosu/tela"
)

// Status is where [UseUpdate] is.
type Status int

const (
	// Checking: the manifest is being fetched.
	Checking Status = iota
	// UpToDate: nothing to do (ErrUpToDate).
	UpToDate
	// Downloading: State.Update is being downloaded; State.Done and
	// State.Total say how far.
	Downloading
	// Ready: State.Update is applied and runs from the next start;
	// State.Restart starts it now.
	Ready
	// Failed: State.Err says why. It matches ErrEndOfSupport (show its
	// message and URL), ErrManaged and ErrNotInstalled (nothing to show:
	// a package manager updates the app, or it runs from go run) or an
	// error of the network or the files.
	Failed
)

// State is what [UseUpdate] returns, for a banner.
type State struct {
	Status Status
	// Update is the release offered, from Downloading on.
	Update *Update
	// Done and Total are the bytes downloaded and to download.
	Done, Total int64
	// Err is the failure, when Failed.
	Err error
	// Check checks again (at once, cancelling a download in progress).
	Check func()
}

// Restart starts the applied update now ([Restart]); a no-op unless the
// state is Ready.
func (s State) Restart() {
	if s.Status == Ready {
		_ = Restart()
	}
}

// UseUpdate checks for an update when the component mounts (on a
// goroutine of its own), downloads and applies it, and returns where it
// is, for a banner:
//
//	func (Root) Build() tela.Node {
//	    up := update.UseUpdate(updates) // a package-level update.Config
//	    return Div(Class("flex flex-col h-full"),
//	        If(up.Status == update.Ready && up.Update.Policy == update.ActivateByApp,
//	            Div(Class("flex gap-3 items-center p-2 bg-sky-100"),
//	                Textf("Version %s is ready.", up.Update.Version),
//	                Button(OnClick(up.Restart), Text("Restart")),
//	            ),
//	        ),
//	        Content{},
//	    )
//	}
//
// With the release's policy (or Config.Policy) ActivateNow it restarts
// into the new version as soon as it is applied; with ActivateNextStart
// and ActivateByApp it waits (State.Restart).
//
// Once the component's first frame is presented, UseUpdate marks the
// running version healthy ([MarkHealthy]), ending the trial of a new
// version: an app with a stricter test of its health calls Check,
// Download, Apply and MarkHealthy itself instead. Use it once, near the
// root; cfg is read on mount and on State.Check.
func UseUpdate(cfg Config) State {
	st, setSt := tela.UseState(State{Status: Checking})
	round, setRound := tela.UseState(0)
	tela.UseGo(func(context.Context) { _ = MarkHealthy() }, tela.Once)
	tela.UseGo(func(ctx context.Context) {
		setSt.Set(State{Status: Checking})
		u, err := Check(ctx, cfg)
		if errors.Is(err, ErrUpToDate) {
			setSt.Set(State{Status: UpToDate})
			return
		}
		if err != nil {
			if ctx.Err() == nil {
				setSt.Set(State{Status: Failed, Err: err})
			}
			return
		}
		total := u.Artifact.Size
		setSt.Set(State{Status: Downloading, Update: u, Total: total})
		last := int64(-1)
		err = u.Download(ctx, func(done, total int64) {
			// One state per percent, not per buffer.
			if pc := done * 100 / max(total, 1); pc != last {
				last = pc
				setSt.Set(State{Status: Downloading, Update: u, Done: done, Total: total})
			}
		})
		if err == nil {
			err = u.Apply()
		}
		if err != nil {
			if ctx.Err() == nil {
				setSt.Set(State{Status: Failed, Update: u, Err: err})
			}
			return
		}
		setSt.Set(State{Status: Ready, Update: u, Done: total, Total: total})
		if u.Policy == ActivateNow {
			_ = Restart()
		}
	}, round)
	st.Check = func() { setRound.Update(func(n int) int { return n + 1 }) }
	return st
}
