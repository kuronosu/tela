package remote

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// The child of TestSubprocess is this test binary, re-executed with
// envChild set: TestMain runs childMain instead of the tests. It covers the
// real cross-process path: descriptor passing (Linux, macOS) or named
// mappings (Windows), and a child that exits.

const envChild = "TELA_REMOTE_TEST_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(envChild) != "" {
		if err := childMain(); err != nil {
			fmt.Fprintln(os.Stderr, "child:", err)
			os.Exit(3)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// childMain is a small app: it presents pattern 1 on its first frame,
// pattern 2 (at the new size) after a resize or a pointer press, answers
// "ping", and quits on a close request.
func childMain() error {
	b, err := DialEnv()
	if err != nil {
		return err
	}
	b.Handle("ping", func(p []byte) ([]byte, error) {
		return []byte("pong " + string(p) + " " + strconv.Itoa(os.Getpid())), nil
	})
	w, err := b.NewWindow(platform.WindowOptions{Title: "subprocess", Size: platform.Size{W: 64, H: 48}})
	if err != nil {
		return err
	}
	var fb *image.RGBA
	seed := byte(1)
	draw := false
	return b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ConfigureEvent:
			if fb != nil {
				seed = 2
			}
			draw = true
		case platform.PointerEvent:
			if e.Action == platform.PointerDown {
				seed, draw = 2, true
				w.RequestFrame()
			}
		case platform.FrameEvent:
			if !draw {
				return
			}
			draw = false
			p := w.Metrics().Physical()
			if fb == nil || fb.Rect.Max != p {
				fb = image.NewRGBA(image.Rectangle{Max: p})
			}
			pattern(fb, fb.Rect, seed)
			if err := w.Present(fb, nil); err != nil {
				b.Notify("error", []byte(err.Error()))
			}
		case platform.CloseRequestedEvent:
			w.Close()
			b.Quit()
		}
	}))
}

func TestSubprocess(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a process")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	p := newPair(t, HostOptions{}, headless.Options{Scale: 2})
	p.host.Handle("error", func(c *Child, payload []byte) ([]byte, error) {
		t.Errorf("child error: %s", payload)
		return nil, nil
	})
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), envChild+"=1", EnvAddr+"="+p.host.Addr(), EnvTag+"=sub")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			cmd.Process.Kill()
			<-exited
		}
	})

	c := p.child(0)
	if c.PID() != cmd.Process.Pid || c.Tag() != "sub" {
		t.Errorf("child PID %d tag %q, want %d %q", c.PID(), c.Tag(), cmd.Process.Pid, "sub")
	}
	hw := p.window(0)
	want := image.NewRGBA(image.Rect(0, 0, 128, 96))
	pattern(want, want.Rect, 1)
	waitFrame(t, hw, "the subprocess's first frame", want)
	if hw.Title() != "subprocess" {
		t.Errorf("title %q", hw.Title())
	}

	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	reply, err := c.Request(ctx, "ping", []byte("hi"))
	if want := fmt.Sprintf("pong hi %d", cmd.Process.Pid); err != nil || string(reply) != want {
		t.Errorf("ping: %q, %v; want %q", reply, err, want)
	}

	// A resize reallocates the shared buffers in the child.
	hw.Resize(platform.Size{W: 80, H: 40})
	want = image.NewRGBA(image.Rect(0, 0, 160, 80))
	pattern(want, want.Rect, 2)
	waitFrame(t, hw, "the frame after a resize", want)

	hw.Inject(platform.CloseRequestedEvent{})
	select {
	case err := <-exited:
		if err != nil {
			t.Errorf("child exited with %v", err)
		}
		exited <- err
	case <-time.After(waitTimeout):
		t.Fatal("child did not exit after a close request")
	}
	select {
	case <-c.Done():
		if err := c.Err(); err != nil {
			t.Errorf("disconnect error %v", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("no disconnect after the child exited")
	}
	waitFor(t, "the real window closed", hw.Closed)
}

// TestSubprocessKilled checks that a child that dies without closing
// anything leaves the window and its last frame.
func TestSubprocessKilled(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a process")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	p := newPair(t, HostOptions{}, headless.Options{})
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), envChild+"=1", EnvAddr+"="+p.host.Addr())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := p.child(0)
	hw := p.window(0)
	want := image.NewRGBA(image.Rect(0, 0, 64, 48))
	pattern(want, want.Rect, 1)
	waitFrame(t, hw, "first frame", want)
	cmd.Process.Kill()
	cmd.Wait()
	select {
	case <-c.Done():
	case <-time.After(waitTimeout):
		t.Fatal("no disconnect after the child was killed")
	}
	if hw.Closed() {
		t.Fatalf("the window closed with the child")
	}
	if f := p.hostFrame(hw); f == nil || string(f.Pix) != string(want.Pix) {
		t.Errorf("the last frame is gone")
	}
}
