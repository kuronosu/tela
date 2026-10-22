//go:build teladev

package core

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/devloop"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/remote"
)

// The child runtime of tela dev (ARCHITECTURE section 17, ADR-111, ADR-114):
// what an app built with the teladev tag does when the supervisor started
// it (TELA_DEVLOOP=1). Run uses the supervisor's window through the remote
// backend instead of a native one, seeds the tree with the state the
// previous build saved, answers the supervisor's save and quit requests,
// reports panics of the UI thread and the directories of tela.Assets, and
// tells how much of the state was kept. Without TELA_DEVLOOP the binary
// behaves like a normal build. The control messages are devloop's.

// devRT is the child runtime of this process.
var devRT struct {
	mu sync.Mutex
	// b is the connection to the supervisor, nil when this process is not
	// a tela dev child.
	b *remote.Backend
	// noState is set by --no-state: no capture, no restore, no report.
	noState bool

	// assets are the absolute os.DirFS roots registered with Assets, in
	// order; assetsSent is how many the supervisor has been told about.
	assets     []string
	assetsSent int

	// The state report: expect is set when the process was seeded with a
	// snapshot; the report is sent once, when the seed is over or after
	// devReportFrames frames.
	expect, reported bool
	frames           int
}

// devReportFrames bounds how many frames the report waits for the seed to
// end (core drops it after devSeedFrames frames).
const devReportFrames = 2 * devSeedFrames

// devActive reports whether this process is a tela dev child.
func devActive() bool {
	devRT.mu.Lock()
	defer devRT.mu.Unlock()
	return devRT.b != nil
}

// devBackend returns the supervisor's window as the backend when the
// process is a tela dev child (TELA_DEVLOOP=1), after fetching the state
// of the previous build and registering the app's own. It returns nil, nil
// otherwise.
func devBackend(opts Options) (platform.Backend, error) {
	if os.Getenv(devloop.EnvChild) != "1" {
		return nil, nil
	}
	b, err := remote.DialEnv()
	if err != nil {
		return nil, fmt.Errorf("tela: this process was started by tela dev (%s=1) but cannot reach it: %w", devloop.EnvChild, err)
	}
	noState := os.Getenv(devloop.EnvNoState) == "1"
	devRT.mu.Lock()
	devRT.b = b
	devRT.noState = noState
	devRT.mu.Unlock()
	devSendAssets()
	if noState {
		return b, nil
	}
	if opts.DevSave != nil || opts.DevRestore != nil {
		RegisterDevState("tela.App.DevState", appDevSave(opts.DevSave), appDevRestore(opts.DevRestore))
	}
	ctx, cancel := context.WithTimeout(context.Background(), devloop.FirstFrameTimeout)
	data, err := b.Request(ctx, devloop.KindStateLoad, nil)
	cancel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tela dev: cannot fetch the state of the previous build (%v); the app starts fresh\n", err)
		return b, nil
	}
	SetDevSeed(data)
	devRT.mu.Lock()
	devRT.expect = len(data) > 0
	devRT.mu.Unlock()
	return b, nil
}

// appDevSave adapts App.DevState.Save: nil keeps nothing.
func appDevSave(f func() ([]byte, error)) func() ([]byte, error) {
	return func() ([]byte, error) {
		if f == nil {
			return nil, nil
		}
		return f()
	}
}

// appDevRestore adapts App.DevState.Restore: it is not called when Save
// returned no bytes.
func appDevRestore(f func([]byte) error) func([]byte) error {
	return func(b []byte) error {
		if f == nil || len(b) == 0 {
			return nil
		}
		return f(b)
	}
}

// devRun runs the event loop of the backend. A tela dev child answers the
// supervisor's requests first.
func devRun(b platform.Backend, h *Host) error {
	rb, ok := b.(*remote.Backend)
	if !ok || !devActive() {
		return b.Run(h)
	}
	rb.Handle(devloop.KindQuit, func([]byte) ([]byte, error) {
		rb.Quit() // the supervisor replaced this build: exit 0, nothing to save
		return nil, nil
	})
	devRT.mu.Lock()
	noState := devRT.noState
	devRT.mu.Unlock()
	if !noState {
		rb.Handle(devloop.KindStateSave, func([]byte) ([]byte, error) {
			h.tree.ApplyUpdates() // updates queued from other goroutines are part of the state
			data, _, err := h.tree.CaptureDevState()
			return data, err
		})
	}
	return rb.Run(h)
}

// devAfterFrame sends the state report once the seed of the tree is over,
// asking for the frames it takes when the app is idle.
func devAfterFrame(h *Host) {
	devRT.mu.Lock()
	rb := devRT.b
	due := devRT.expect && !devRT.reported && !devRT.noState
	if due {
		devRT.frames++
		if h.tree.devSeedOpen() && devRT.frames < devReportFrames {
			devRT.mu.Unlock()
			h.win.RequestFrame()
			return
		}
		devRT.reported = true
	}
	devRT.mu.Unlock()
	if !due || rb == nil {
		return
	}
	rep := h.tree.DevSeedReport()
	for i, r := range rep.Remounted {
		if i == 5 {
			fmt.Fprintf(os.Stderr, "tela dev: %d more components reset\n", len(rep.Remounted)-i)
			break
		}
		fmt.Fprintf(os.Stderr, "tela dev: state reset: %s\n", r)
	}
	rb.Notify(devloop.KindStateReport, devSeedCounts(rep).Encode())
}

// devSeedCounts is the supervisor's view of a seed report: the hook slots
// and the engine state kept, and the components that reset.
func devSeedCounts(r DevSeedReport) devloop.StateReport {
	kept := r.Restored + r.Fields + r.Scrolls
	if r.Focus {
		kept++
	}
	return devloop.StateReport{Kept: kept, Reset: len(r.Remounted)}
}

// devSeedOpen reports whether the tree still holds the snapshot it is
// restoring.
func (t *Tree) devSeedOpen() bool {
	d := t.devState()
	return d != nil && d.seed != nil
}

// devRecover is deferred by Run. A panic of the UI thread (a handler, a
// build, an effect) in a tela dev child is printed on standard error as Go
// does, reported to the supervisor with its stack, and ends the process
// with status 2; the supervisor shows it and keeps the window. Panics of
// other goroutines cannot be recovered: the runtime prints them and exits
// with status 2 itself, and the supervisor reports the exit.
func devRecover() {
	if !devActive() {
		return // not a tela dev child: the panic propagates as usual
	}
	v := recover()
	if v == nil {
		return
	}
	stack := debug.Stack()
	fmt.Fprintf(os.Stderr, "panic: %v\n\n%s", v, stack)
	devRT.mu.Lock()
	rb := devRT.b
	devRT.mu.Unlock()
	done := make(chan struct{})
	go func() {
		rb.Notify(devloop.KindPanic, devloop.PanicPayload(v, stack))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
	os.Exit(2)
}

// devAssetAdded records the directory of fsys when it is an os.DirFS (or a
// fs.Sub of one), and tells the supervisor, which watches it.
func devAssetAdded(fsys fs.FS) {
	root := devDirRoot(reflect.ValueOf(fsys))
	if root == "" {
		return // embedded and other file systems rebuild through go list
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	devRT.mu.Lock()
	if !slices.Contains(devRT.assets, root) {
		devRT.assets = append(devRT.assets, root)
	}
	devRT.mu.Unlock()
	devSendAssets()
}

// devSendAssets sends the full list of directories when it grew.
func devSendAssets() {
	devRT.mu.Lock()
	rb := devRT.b
	if rb == nil || len(devRT.assets) == devRT.assetsSent {
		devRT.mu.Unlock()
		return
	}
	devRT.assetsSent = len(devRT.assets)
	dirs := slices.Clone(devRT.assets)
	devRT.mu.Unlock()
	rb.Notify(devloop.KindAssets, devloop.EncodeAssets(dirs))
}

// devDirRoot returns the directory behind an os.DirFS value (an unexported
// string type of package os) or a fs.Sub of one, "" for anything else.
func devDirRoot(v reflect.Value) string {
	for v.IsValid() && v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if !v.IsValid() {
		return ""
	}
	t := v.Type()
	switch {
	case t.Kind() == reflect.String && t.PkgPath() == "os" && t.Name() == "dirFS":
		return v.String()
	case t.Kind() == reflect.Pointer && t.Elem().PkgPath() == "io/fs" && t.Elem().Name() == "subFS" && !v.IsNil():
		e := v.Elem()
		inner := devDirRoot(e.FieldByName("fsys"))
		dir := e.FieldByName("dir")
		if inner == "" || !dir.IsValid() || dir.Kind() != reflect.String {
			return ""
		}
		return filepath.Join(inner, filepath.FromSlash(dir.String()))
	}
	return ""
}
