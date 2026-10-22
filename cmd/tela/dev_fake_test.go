package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime/debug"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/devloop"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/remote"
)

// The fake children of the supervisor's tests are this test binary,
// re-executed with envFake set to a mode: TestMain runs fakeChild instead of
// the tests. A fake child speaks the remote protocol and the devloop kinds
// directly (the role of the teladev runtime, task T3), so the supervisor is
// tested without a real Tela app.
//
// It shows one solid color: R is its build generation, G its counter (its
// state: kept across reloads), B the clicks this process received itself,
// plus 0x80 when it sees the system in dark mode. A primary press
// increments the counter; the key "p" makes it panic; a close request
// closes the window and quits (exit 0).
//
// Modes: "ok"; "slow" (the first frame waits fakeSlow); "noframe" (never
// presents); "crash" (exits with status 3 before connecting its window);
// "panic" (reports a panic and exits with status 2 at startup); "nosave"
// (no state.save handler); "late" (the state report comes 150 ms after
// start) and "noreport" (none); "stdio" checks that its standard output
// and error are pipes and writes fakeLines numbered lines to each. The key
// "g" panics in another goroutine.
const envFake = "TELA_DEV_TEST_FAKE"

const (
	fakeSlow  = 400 * time.Millisecond
	fakeLines = 500
)

func TestMain(m *testing.M) {
	if mode, ok := os.LookupEnv(envFake); ok {
		os.Exit(fakeChild(mode))
	}
	os.Exit(m.Run())
}

func fakeChild(mode string) int {
	b, err := remote.DialEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake:", err)
		return 4
	}
	gen, _ := strconv.Atoi(os.Getenv(remote.EnvTag))
	noState := os.Getenv(devloop.EnvNoState) == "1"
	if os.Getenv(devloop.EnvChild) != "1" {
		b.Notify("fake.error", []byte(devloop.EnvChild+" is not set"))
	}
	if os.Getenv("TELA_DEV") != "1" && os.Getenv("FAKE_EXPECT_DEV") != "" {
		b.Notify("fake.error", []byte("TELA_DEV is not 1"))
	}
	count := 0
	if !noState {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		data, err := b.Request(ctx, devloop.KindStateLoad, nil)
		cancel()
		if err != nil {
			b.Notify("fake.error", []byte("state.load: "+err.Error()))
		} else if len(data) > 0 {
			count, _ = strconv.Atoi(string(data))
			report := devloop.StateReport{Kept: 1, Reset: 2}.Encode()
			switch mode {
			case "late":
				// Like the real runtime: once the state is restored, a
				// few frames after the first one.
				time.AfterFunc(150*time.Millisecond, func() { b.Notify(devloop.KindStateReport, report) })
			case "noreport":
			default:
				b.Notify(devloop.KindStateReport, report)
			}
		}
	}
	switch mode {
	case "crash":
		return 3
	case "panic":
		b.Notify(devloop.KindPanic, devloop.PanicPayload("startup boom", debug.Stack()))
		fmt.Fprintln(os.Stderr, "panic: startup boom")
		return 2
	}
	if mode == "stdio" {
		// The supervisor forwards its children's output: they never write
		// to the terminal themselves (ADR-119).
		for _, f := range []*os.File{os.Stdout, os.Stderr} {
			if fi, err := f.Stat(); err != nil || fi.Mode()&os.ModeNamedPipe == 0 {
				b.Notify("fake.error", []byte(fmt.Sprintf("%s is not a pipe (%v)", f.Name(), err)))
			}
		}
		for i := range fakeLines {
			fmt.Fprintf(os.Stdout, "out %d\n", i)
			fmt.Fprintf(os.Stderr, "err %d\n", i)
		}
	}
	if mode != "nosave" {
		b.Handle(devloop.KindStateSave, func([]byte) ([]byte, error) {
			if noState {
				b.Notify("fake.error", []byte("state.save under --no-state"))
			}
			return []byte(strconv.Itoa(count)), nil
		})
	}
	b.Handle(devloop.KindQuit, func([]byte) ([]byte, error) {
		b.Quit()
		return nil, nil
	})
	w, err := b.NewWindow(platform.WindowOptions{Title: "fake " + strconv.Itoa(gen), Size: platform.Size{W: 100, H: 80}})
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake:", err)
		return 4
	}
	clicks := 0
	dark := b.Settings().DarkMode
	dirty, first := true, true
	var fb *image.RGBA
	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ConfigureEvent:
			dirty = true
		case platform.SettingsEvent:
			dark = e.Settings.DarkMode
			dirty = true
			w.RequestFrame()
		case platform.PointerEvent:
			if e.Action == platform.PointerDown && e.Button == platform.ButtonPrimary {
				count++
				clicks++
				dirty = true
				w.RequestFrame()
			}
		case platform.KeyEvent:
			if e.Action == platform.KeyPress && e.Key == "g" {
				// A panic in another goroutine: nobody can recover it, the
				// runtime prints it and exits with status 2.
				go func() { panic("goroutine boom") }()
			}
			if e.Action == platform.KeyPress && e.Key == "p" {
				b.Notify(devloop.KindPanic, devloop.PanicPayload("boom", debug.Stack()))
				fmt.Fprintln(os.Stderr, "panic: boom")
				os.Exit(2)
			}
		case platform.CloseRequestedEvent:
			w.Close()
			b.Quit()
		case platform.FrameEvent:
			if !dirty || mode == "noframe" {
				return
			}
			if first && mode == "slow" {
				time.Sleep(fakeSlow)
			}
			first, dirty = false, false
			p := w.Metrics().Physical()
			if fb == nil || fb.Rect.Max != p {
				fb = image.NewRGBA(image.Rectangle{Max: p})
			}
			c := color.RGBA{R: uint8(gen), G: uint8(count), B: uint8(clicks), A: 255}
			if dark {
				c.B |= 0x80
			}
			for i := 0; i < len(fb.Pix); i += 4 {
				fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = c.R, c.G, c.B, c.A
			}
			if err := w.Present(fb, nil); err != nil {
				b.Notify("fake.error", []byte(err.Error()))
			}
		}
	}))
	if err != nil {
		return 1
	}
	return 0
}

// fakeBuilder "builds" by returning the test binary in a fake mode.
type fakeBuilder struct {
	exe string

	mu        sync.Mutex
	modes     map[int]string        // generation -> mode (default "ok")
	fail      map[int]string        // generation -> compiler output of a failed build
	block     map[int]chan struct{} // generation -> released when closed
	cancelled []int
	builds    int
}

func (f *fakeBuilder) build(ctx context.Context, gen int) buildResult {
	f.mu.Lock()
	f.builds++
	mode := f.modes[gen]
	fail, failed := f.fail[gen]
	block := f.block[gen]
	f.mu.Unlock()
	res := buildResult{gen: gen, exe: f.exe, args: []string{"-test.run=^$"}, start: time.Now()}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			f.mu.Lock()
			f.cancelled = append(f.cancelled, gen)
			f.mu.Unlock()
			res.err, res.end = ctx.Err(), time.Now()
			return res
		}
	}
	if mode == "" {
		mode = "ok"
	}
	res.env = []string{envFake + "=" + mode}
	if failed {
		res.err, res.output = errors.New("exit status 1"), fail
	}
	res.end = time.Now()
	return res
}

func (f *fakeBuilder) set(gen int, mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modes[gen] = mode
}
