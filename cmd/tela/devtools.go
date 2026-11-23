package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/kuronosu/tela/cmd/tela/internal/devtools"
	"github.com/kuronosu/tela/internal/core"
)

const devtoolsUsage = `Usage:

	tela devtools [-addr ADDRESS]

Opens the DevTools window for a running tela dev session (ADR-299): the
element tree of the app with the pick tool, the details of the selected
element (source line, props, hooks, why it rendered, computed style and
box model), the accessibility tree, a frame profiler and the app's log.
The panels follow the app live and survive its reloads. Hovering an
element highlights it in the app.

The DevTools attach to the session's tools socket, which tela dev opens
while it runs. Without -addr they attach to the only session running
(tela dev writes <user cache>/tela/dev/<pid>.json); with several, name one
by its address, which tela dev -v prints. Closing the DevTools leaves the
session running, and the DevTools close when it ends.

tela dev --devtools starts them beside the app.

Flags:

	-addr ADDRESS  the session's tools socket (default: the only session)
`

// runDevtools opens the DevTools of a tela dev session.
func runDevtools(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tela devtools", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	addr := fs.String("addr", "", "")
	err := fs.Parse(args)
	if err == nil && fs.NArg() > 0 {
		err = fmt.Errorf("unexpected argument %q; tela devtools takes only -addr", fs.Arg(0))
	}
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, devtoolsUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "tela devtools: %v\n", err)
		return 2
	}
	c, err := dialTools(*addr)
	if err != nil {
		fmt.Fprintf(stderr, "tela devtools: %v\n", err)
		return 1
	}
	devtools.Run(newDevtoolsSource(c))
	return 0
}

// devtoolsExe is the program tela dev --devtools starts: this one.
func devtoolsExe() string {
	exe, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	return exe
}

// startDevtools starts exe devtools beside a session, as a process of its
// own that attaches to addr, and returns the function that stops it (the
// DevTools also close by themselves when the session ends).
func startDevtools(exe, addr string, stderr io.Writer) (stop func(), err error) {
	cmd := exec.Command(exe, "devtools", "-addr", addr)
	cmd.Stdout, cmd.Stderr = stderr, stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()
	return func() {
		select {
		case <-done:
		default:
			cmd.Process.Kill()
			<-done
		}
	}, nil
}

// devtoolsSource is the session's tools client as the DevTools' Source.
type devtoolsSource struct {
	c     *toolsClient
	notes chan devtools.Note
}

func newDevtoolsSource(c *toolsClient) *devtoolsSource {
	s := &devtoolsSource{c: c, notes: make(chan devtools.Note)}
	go func() {
		defer close(s.notes)
		for n := range c.Notes() {
			s.notes <- devtools.Note{Method: n.Method, Params: n.Params}
		}
	}()
	return s
}

func (s *devtoolsSource) Info(ctx context.Context) (devtools.Info, error) {
	i, err := s.c.Info(ctx)
	return devtools.Info{Pkg: i.Pkg, Dir: i.Dir, PID: i.PID, Started: i.Started, Build: i.Build, Protocol: i.Protocol}, err
}

func (s *devtoolsSource) Windows(ctx context.Context) ([]devtools.Window, error) {
	ws, err := s.c.Windows(ctx)
	out := make([]devtools.Window, len(ws))
	for i, w := range ws {
		out[i] = devtools.Window(w)
	}
	return out, err
}

func (s *devtoolsSource) Subscribe(ctx context.Context, events ...string) error {
	return s.c.Subscribe(ctx, events...)
}

func (s *devtoolsSource) Unsubscribe(ctx context.Context, events ...string) error {
	return s.c.Unsubscribe(ctx, events...)
}

func (s *devtoolsSource) ElementsSince(ctx context.Context, window uint32, version uint64) (core.ElementsDiff, error) {
	return s.c.ElementsSince(ctx, window, version)
}

func (s *devtoolsSource) Element(ctx context.Context, window uint32, id core.ElementID) (core.ElementInfo, error) {
	return s.c.Element(ctx, window, id)
}

func (s *devtoolsSource) Inspect(ctx context.Context, window uint32, id core.ElementID, depth int) (core.InspectNode, error) {
	return s.c.InspectDepth(ctx, window, id, depth)
}

func (s *devtoolsSource) Semantics(ctx context.Context, window uint32) (devtools.Semantics, error) {
	r, err := s.c.Semantics(ctx, window)
	return devtools.Semantics{YAML: r.YAML, Nodes: r.Nodes, Elements: r.Elements}, err
}

func (s *devtoolsSource) Profiles(ctx context.Context, window uint32, since int) ([]core.FrameProfile, error) {
	return s.c.Profiles(ctx, window, since)
}

func (s *devtoolsSource) Logs(ctx context.Context, since uint64) ([]devtools.LogEntry, uint64, error) {
	es, last, err := s.c.Logs(ctx, since)
	out := make([]devtools.LogEntry, len(es))
	for i, e := range es {
		out[i] = devtools.LogEntry(e)
	}
	return out, last, err
}

func (s *devtoolsSource) Pick(ctx context.Context, window uint32, on bool) error {
	return s.c.Pick(ctx, window, on)
}

func (s *devtoolsSource) Highlight(ctx context.Context, window uint32, id core.ElementID) error {
	return s.c.Highlight(ctx, window, id)
}

func (s *devtoolsSource) Notes() <-chan devtools.Note { return s.notes }
func (s *devtoolsSource) Done() <-chan struct{}       { return s.c.Done() }
func (s *devtoolsSource) Close() error                { return s.c.Close() }
