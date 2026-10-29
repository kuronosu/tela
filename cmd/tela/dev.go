package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/kuronosu/tela/cmd/tela/internal/term"
	"github.com/kuronosu/tela/cmd/tela/internal/watch"
	"github.com/kuronosu/tela/internal/platform/auto"
	"github.com/kuronosu/tela/internal/platform/remote"
)

func init() {
	// tela dev runs a platform event loop, which must stay on the main OS
	// thread (Cocoa requires it, Win32 requires the thread that created
	// the windows), like tela.Run does. Harmless for the other commands.
	runtime.LockOSThread()
}

const devUsage = `Usage:

	tela dev [--size WxH] [--scale N] [--dark] [--preview NAME] [--no-state] [-v] [pkg]

Runs a package's app in a window that never closes, rebuilds it in the
background when its sources change and swaps the running process for the
new build, keeping the state of its components (ADR-111). Go cannot replace
code in a running process, so the app runs as a child of tela dev, which
owns the window; each build is a new child that takes over the window
without a visible close and reopen.

<pkg> is a package path or a relative directory; it defaults to ".". A
package main is built as is. Any other package, or a package with
--preview, gets a generated main that runs its screen, as tela snapshot
renders it: func Preview(), a Previews variable or the Root of a
package-level var App (whose Width, Height, RootClass, ColorScheme and the
rest apply to the window).

The app is rebuilt when any non-test .go file of its module changes, or
go.mod, go.sum, a file the module's packages embed, or a file under a
directory the app registered with tela.Assets(os.DirFS(...)). Builds use
-tags teladev (the dev-loop runtime) with CGO_ENABLED=0; release builds are
unaffected. A save during a build restarts it. To build faster, dev builds
also skip the version-control stamp (-buildvcs=false), the symbol table and
the DWARF debug information (-ldflags=-s -w: panics keep their file:line,
but a debugger cannot attach) and inlining in the package built
(-gcflags=-l); a -buildvcs, -ldflags or -gcflags set in GOFLAGS is used
instead.

Before a swap the input is held; it reaches the new build once it shows its
first frame. A build error, or a crash of the app, is printed here and the
window keeps the last good frame; the next good build takes over.

On Windows, Microsoft Defender scans each new build on its first run, which
can make every reload about twice as slow. When the build of a reload takes
more than 400ms to start, tela dev prints once the directories to exclude
from real-time scanning (its build directory, tela-dev in the temporary
directory, and the Go build cache, go env GOCACHE) and the
PowerShell command to run as administrator (Add-MpPreference
-ExclusionPath ...). It never changes the settings itself.

Flags:

	--size WxH     the window's logical size (default: the app's)
	--scale N      render at scale N instead of the display's
	--dark         the app sees the system in dark mode (App.ColorScheme
	               still wins)
	--preview NAME run the preview NAME of the package's Previews
	--no-state     do not keep state across reloads
	-v             print the timing of every step of a reload

Keys (when standard input is a terminal):

	r   rebuild and reload, keeping state
	R   rebuild and reload with fresh state
	q   quit (so does closing the window, or Ctrl+C)
`

// devOptions are the arguments of tela dev.
type devOptions struct {
	Pkg     string
	W, H    float64 // --size, when SizeSet
	SizeSet bool
	Scale   float64 // --scale; 0 means the window's
	Dark    bool
	Preview string
	NoState bool
	Verbose bool

	noTerminal bool // tests: leave standard input alone
}

// parseDevArgs parses the arguments of tela dev. Flags and the package may
// come in any order; the package defaults to ".".
func parseDevArgs(args []string) (devOptions, error) {
	var o devOptions
	var size string
	fs := flag.NewFlagSet("tela dev", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&size, "size", "", "")
	fs.Float64Var(&o.Scale, "scale", 0, "")
	fs.BoolVar(&o.Dark, "dark", false, "")
	fs.StringVar(&o.Preview, "preview", "", "")
	fs.BoolVar(&o.NoState, "no-state", false, "")
	fs.BoolVar(&o.Verbose, "v", false, "")
	fs.BoolVar(&o.Verbose, "verbose", false, "")
	pkgs, err := parsePositionals("dev", fs, args)
	if err != nil {
		return o, err
	}
	switch len(pkgs) {
	case 0:
		o.Pkg = "."
	case 1:
		o.Pkg = pkgs[0]
	default:
		return o, fmt.Errorf("expected at most one <pkg>, got %d (%s)", len(pkgs), strings.Join(pkgs, ", "))
	}
	if size != "" {
		w, h, err := parseSize(size)
		if err != nil {
			return o, err
		}
		o.W, o.H, o.SizeSet = w, h, true
	}
	if o.Scale != 0 && (!(o.Scale > 0) || math.IsInf(o.Scale, 0) || o.Scale > 64) {
		return o, fmt.Errorf("invalid --scale %v: use a number > 0 and <= 64, for example 1, 2 or 1.25", o.Scale)
	}
	if o.SizeSet && o.Scale > 0 {
		px, py := physicalSide(o.W, o.Scale), physicalSide(o.H, o.Scale)
		if px > maxPhysicalSide || py > maxPhysicalSide || px*py > maxPhysicalPix {
			return o, fmt.Errorf("a %gx%g window at scale %g is %dx%d pixels, which is too large; lower --size or --scale", o.W, o.H, o.Scale, px, py)
		}
	}
	return o, nil
}

func runDev(args []string, stdout, stderr io.Writer) int {
	opts, err := parseDevArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, devUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "tela dev: %v\n", err)
		return 2
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintln(stderr, "tela dev: the go command was not found in PATH; install Go (https://go.dev/dl) and try again")
		return 1
	}
	code, err := devMain(goBin, opts, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "tela dev: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	return code
}

// devMain wires the real pieces: the target's builder, the watcher, the
// native backend with a remote host, the terminal and the signals; the
// supervisor does the rest.
func devMain(goBin string, opts devOptions, stdout, stderr io.Writer) (int, error) {
	target, err := resolveDevTarget(goBin, opts)
	if err != nil {
		return 1, err
	}
	tmp, exclude, err := newBuildDir()
	if err != nil {
		return 1, err
	}
	defer os.RemoveAll(tmp)
	b, err := newGoBuilder(goBin, target, tmp)
	if err != nil {
		return 1, err
	}
	b.env = append(b.env, buildTempEnv(tmp, target.gotmpdir)...)

	// What to watch: the module (and local replacements) of the target,
	// refreshed after each build together with the embedded files.
	set := newWatchSet(tmp)
	if deps, err := listDeps(goBin, target); err == nil {
		set.setDeps(deps)
	} else {
		set.setDeps(depSet{modules: []string{target.modDir}})
		fmt.Fprintf(stderr, "tela dev: warning: %v\n", err)
	}
	w, err := watch.New(watch.Options{Filter: set.relevant, SkipDir: set.skipDir}, set.roots()...)
	if err != nil {
		return 1, err
	}
	defer w.Close()

	backend, err := auto.New()
	if err != nil {
		return 1, err
	}

	// The terminal: single keys, restored on every exit path (the
	// supervisor's goroutines restore it too if they panic).
	var restore func()
	if !opts.noTerminal && term.IsTerminal(os.Stdin.Fd()) {
		if st, err := term.Cbreak(os.Stdin.Fd()); err == nil {
			restore = func() { st.Restore() }
			defer restore()
		}
	}

	s := newSupervisor(supervisorConfig{
		opts:     opts,
		builder:  b,
		log:      stderr,
		stdout:   stdout,
		stderr:   stderr,
		env:      os.Environ(),
		restore:  restore,
		overlay:  true,
		watchSet: set,
		setRoots: func(roots []string) { w.SetRoots(roots) },
		refreshDeps: func() (depSet, error) {
			return listDeps(goBin, target)
		},
		describe:   target.describe(),
		watching:   watchDescription(w),
		slowStart:  defaultSlowStart(),
		exclusions: exclusions(exclude, target),
	})
	host, err := remote.NewHost(backend, s.hostOptions())
	if err != nil {
		return 1, err
	}
	defer host.Close()
	s.attach(host, backend.Quit)

	go s.forward(w)
	if restore != nil {
		go s.readKeys(os.Stdin)
	}
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	go func() {
		for range sigs {
			s.push(evSignal{})
		}
	}()

	done := make(chan int, 1)
	go func() { done <- s.run() }()
	runErr := host.Run()
	s.push(evHostDone{err: runErr})
	code := <-done
	return code, nil
}

// watchDescription says how the watcher works, for the start line.
func watchDescription(w *watch.Watcher) string {
	if w.Native() {
		return ""
	}
	return " (polling for changes)"
}
