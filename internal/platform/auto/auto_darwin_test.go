//go:build darwin

package auto

import (
	"errors"
	"os"
	"runtime"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// cocoa.New runs on the main thread only: TestMain keeps the main goroutine
// there and serves onMain while the tests run on other goroutines.

func init() { runtime.LockOSThread() }

var mainCalls = make(chan func())

func TestMain(m *testing.M) {
	done := make(chan int, 1)
	go func() { done <- m.Run() }()
	for {
		select {
		case f := <-mainCalls:
			f()
		case code := <-done:
			os.Exit(code)
		}
	}
}

// onMain runs f on the main thread and returns when it returns.
func onMain(f func()) {
	ch := make(chan struct{})
	mainCalls <- func() { defer close(ch); f() }
	<-ch
}

func TestCocoa(t *testing.T) {
	for _, env := range []string{"", "cocoa"} {
		t.Run("TELA_BACKEND="+env, func(t *testing.T) {
			t.Setenv(EnvBackend, env)
			var b platform.Backend
			var err error
			onMain(func() {
				b, err = New()
				if err == nil {
					// Run's shutdown releases the backend and unlocks the
					// thread.
					b.Quit()
					if rerr := b.Run(platform.HandlerFunc(func(platform.Window, platform.Event) {})); rerr != nil {
						t.Errorf("Run: %v", rerr)
					}
				}
			})
			if errors.Is(err, platform.ErrUnavailable) {
				t.Skipf("no window server session: %v", err)
			}
			if err != nil {
				t.Fatalf("New() with %s=%q: %v", EnvBackend, env, err)
			}
			if b.Name() != "cocoa" {
				t.Fatalf("Name() = %q, want cocoa", b.Name())
			}
			t.Logf("%s=%q gives the %s backend", EnvBackend, env, b.Name())
		})
	}
}

func TestOtherBackendsAreUnavailable(t *testing.T) {
	for _, name := range []string{"wayland", "x11", "win32"} {
		t.Setenv(EnvBackend, name)
		b, err := New()
		if err == nil {
			t.Fatalf("New() with %s=%s = %v, nil; want an error", EnvBackend, name, b)
		}
		if !errors.Is(err, platform.ErrUnavailable) {
			t.Errorf("error %v does not wrap platform.ErrUnavailable", err)
		}
	}
}
