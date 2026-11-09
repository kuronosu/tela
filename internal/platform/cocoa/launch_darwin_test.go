//go:build darwin

package cocoa

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The launch tests: application:openURLs: sent to the app delegate
// directly (what AppKit does with Launch Services' Apple Event), a launch
// before Run, and single instance between this
// test process and a helper process (the test binary run again with
// TELA_COCOA_INSTANCE_HELPER set).

// TestOpenURLs: the URLs handed to the delegate are one LaunchEvent, file
// URLs as paths, from a later pass.
func TestOpenURLs(t *testing.T) {
	b := newBackend(t)
	cwd, _ := os.Getwd()
	path := filepath.Join(t.TempDir(), "a b.txt")
	var got []platform.Launch
	inCall := false
	desktopRun(t, b, 20*time.Second, func(w *Window) {
		v := darwin.SendClass(app.userDefaults, darwin.Sel("standardUserDefaults"))
		t.Logf("NSTreatUnknownArgumentsAsOpen = %q", darwin.GoString(darwin.Send(v, darwin.Sel("stringForKey:"), darwin.AutoString("NSTreatUnknownArgumentsAsOpen"))))
		if darwin.SendBool(v, darwin.Sel("boolForKey:"), darwin.AutoString("NSTreatUnknownArgumentsAsOpen")) {
			t.Errorf("NSTreatUnknownArgumentsAsOpen is on: AppKit would open the command-line arguments")
		}
		custom := darwin.SendClass(darwin.GetClass("NSURL"), darwin.Sel("URLWithString:"), darwin.AutoString("tela-test:open?id=3"))
		urls := darwin.NSArray(darwin.FileURL(path, false), custom)
		inCall = true
		darwin.Send(app.delegate, darwin.Sel("application:openURLs:"), app.ns, urls)
		darwin.Send(app.delegate, darwin.Sel("application:openURLs:"), app.ns, darwin.NSArray()) // nothing
		inCall = false
	}, func(w *Window, e platform.Event) bool {
		if le, ok := e.(platform.LaunchEvent); ok {
			if inCall {
				t.Errorf("the LaunchEvent was delivered from inside application:openURLs:")
			}
			if le.Source != platform.LaunchOpened {
				t.Errorf("Launch Services' launch has Source %v, want opened", le.Source)
			}
			t.Logf("launch: %+v", le.Launch)
			got = append(got, le.Launch)
			return true
		}
		return false
	})
	want := platform.Launch{Args: []string{path, "tela-test:open?id=3"}, Dir: cwd}
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Errorf("launches %+v, want one %+v", got, want)
	}
}

// TestLaunchBeforeRun: a launch that reaches the backend before Run (the
// Apple Event of the start comes while [NSApp run] finishes launching) is
// delivered in Run's passes, before or after the first frame: core holds
// the app-scoped events until the first commit (ADR-253), not the
// backend.
func TestLaunchBeforeRun(t *testing.T) {
	b := newBackend(t)
	var order []string
	onMain(func() {
		if _, err := b.NewWindow(platform.WindowOptions{Title: "tela held launch", Size: platform.Size{W: 200, H: 120}}); err != nil {
			t.Errorf("NewWindow: %v", err)
			return
		}
		b.openedLaunch(platform.Launch{Args: []string{"early"}})
		timer := time.AfterFunc(15*time.Second, b.Quit)
		defer timer.Stop()
		_ = b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
			switch e := e.(type) {
			case platform.FrameEvent:
				if len(order) == 0 || order[len(order)-1] != "frame" {
					order = append(order, "frame")
				}
				presentSolid(t, w, color.RGBA{0x50, 0x50, 0x50, 0xff})
			case platform.LaunchEvent:
				order = append(order, fmt.Sprint(e.Launch.Args))
				b.Quit()
			}
		}))
	})
	t.Logf("order: %v", order)
	if !slices.Contains(order, "[early]") {
		t.Errorf("events %v, want the launch delivered", order)
	}
}

const instanceHelperEnv = "TELA_COCOA_INSTANCE_HELPER"

// helperLaunch is what the helper process forwards.
var helperLaunch = platform.Launch{Args: []string{"tela-test:open?id=7", "rel.txt"}, Dir: "/tmp"}

// TestInstanceHelper is the second process of TestClaimInstanceProcesses:
// it claims the instance the first holds and must forward its launch.
func TestInstanceHelper(t *testing.T) {
	id := os.Getenv(instanceHelperEnv)
	if id == "" {
		t.Skip("the helper process of TestClaimInstanceProcesses")
	}
	b := newBackend(t)
	var primary bool
	var err error
	var yields bool
	var firstActive string
	start := time.Now()
	onMain(func() {
		primary, err = b.Desktop().ClaimInstance(id, helperLaunch)
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		yields = darwin.SendBool(app.ns, lsSel.respondsToSelector, lsSel.yieldActivation)
		self := darwin.SendClass(darwin.GetClass("NSRunningApplication"), darwin.Sel("currentApplication"))
		first := darwin.SendClass(darwin.GetClass("NSRunningApplication"), lsSel.withPID, int32(os.Getppid()))
		firstActive = fmt.Sprintf("this process active %v, the first (PID %d) %v",
			darwin.SendBool(self, darwin.Sel("isActive")), os.Getppid(), first != 0 && darwin.SendBool(first, darwin.Sel("isActive")))
	})
	t.Logf("ClaimInstance = %v, %v in %v; yieldActivationToApplication: %v; %s", primary, err, time.Since(start).Round(time.Millisecond), yields, firstActive)
	onMain(func() {
		b.Quit()
		_ = b.Run(platform.HandlerFunc(func(platform.Window, platform.Event) {}))
	})
	if err != nil || primary {
		t.Errorf("the second process's ClaimInstance = %v, %v; want false, nil (forwarded)", primary, err)
	}
}

// TestClaimInstanceProcesses: this process claims an instance; a second
// process (the helper) forwards its launch, which arrives as a
// LaunchEvent; when Run returns the socket is gone.
func TestClaimInstanceProcesses(t *testing.T) {
	if os.Getenv(instanceHelperEnv) != "" {
		t.Skip("in the helper process")
	}
	b := newBackend(t)
	id := fmt.Sprintf("dev.telaui.test.instance.%d", os.Getpid())
	lock, sock, err := instancePaths(os.TempDir(), id)
	t.Logf("TMPDIR %q: lock %s, socket %s (%d bytes), %v", os.TempDir(), lock, sock, len(sock), err)
	var primary, again bool
	var err1, err2 error
	onMain(func() {
		primary, err1 = b.Desktop().ClaimInstance(id, platform.CurrentLaunch())
		again, err2 = b.Desktop().ClaimInstance(id, platform.CurrentLaunch())
	})
	if err1 != nil || !primary || err2 != nil || !again {
		t.Fatalf("ClaimInstance = %v, %v; again %v, %v; want true twice", primary, err1, again, err2)
	}
	if fi, err := os.Lstat(sock); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the socket: %v, %v; want mode 0600", fi, err)
	}

	var out []byte
	var helperErr error
	done := make(chan struct{})
	var helperDone atomic.Bool
	var got []platform.Launch
	var activeAfter string
	var activatedAt, helperSeen time.Time
	desktopRun(t, b, 60*time.Second, func(w *Window) {
		go func() {
			defer close(done)
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstanceHelper$", "-test.v", "-test.count=1")
			cmd.Env = append(os.Environ(), instanceHelperEnv+"="+id)
			out, helperErr = cmd.CombinedOutput()
			helperDone.Store(true)
		}()
	}, func(w *Window, e platform.Event) bool {
		if le, ok := e.(platform.LaunchEvent); ok {
			t.Logf("launch: %+v (%v)", le.Launch, le.Source)
			if le.Source != platform.LaunchForwarded {
				t.Errorf("the helper's launch has Source %v, want forwarded", le.Source)
			}
			got = append(got, le.Launch)
			w.ActivateWithToken(le.Launch.Token) // what desktop.SingleInstance does
			activatedAt = time.Now()
		}
		if !activatedAt.IsZero() && activeAfter == "" && time.Since(activatedAt) > 500*time.Millisecond {
			activeAfter = fmt.Sprintf("NSApp isActive %v, the window key %v",
				darwin.SendBool(app.ns, darwin.Sel("isActive")), darwin.SendBool(w.ns, darwin.Sel("isKeyWindow")))
		}
		if !helperDone.Load() {
			return false
		}
		// The helper exits once its launch was acknowledged, which is
		// before the launch's pass here: wait for it, with a deadline.
		if helperSeen.IsZero() {
			helperSeen = time.Now()
		}
		if len(got) == 0 {
			if time.Since(helperSeen) > 10*time.Second {
				t.Errorf("no LaunchEvent within 10 s of the helper's end")
				return true
			}
			return false
		}
		return activeAfter != ""
	})
	select {
	case <-done:
	case <-time.After(50 * time.Second):
		t.Fatalf("the helper process did not end")
	}
	t.Logf("helper (%v):\n%s", helperErr, out)
	t.Logf("after ActivateWithToken: %s", activeAfter)
	if helperErr != nil {
		t.Errorf("the helper process failed: %v", helperErr)
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], helperLaunch) {
		t.Errorf("launches %+v, want one %+v", got, helperLaunch)
	}
	if _, err := os.Lstat(sock); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the socket after Run: %v; want it removed", err)
	}
	var after error
	onMain(func() { _, after = b.Desktop().ClaimInstance(id, platform.Launch{}) })
	if after == nil {
		t.Errorf("ClaimInstance after Run = nil error")
	}
}
