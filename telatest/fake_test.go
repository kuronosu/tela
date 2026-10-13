package telatest_test

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// fakeTB is a testing.TB that records failures instead of failing, so the
// tests can check what telatest reports. Fatal stops the goroutine that
// called it, like the real one.
type fakeTB struct {
	testing.TB // nil: the methods telatest uses are overridden below

	mu       sync.Mutex
	name     string
	errors   []string
	fatal    string
	logs     []string
	cleanups []func()
	failed   bool
}

func (f *fakeTB) Helper()      {}
func (f *fakeTB) Name() string { return f.name }
func (f *fakeTB) Failed() bool { return f.failed }
func (f *fakeTB) Fail()        { f.failed = true }
func (f *fakeTB) FailNow()     { f.failed = true; runtime.Goexit() }
func (f *fakeTB) Cleanup(fn func()) {
	f.cleanups = append(f.cleanups, fn)
}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = true
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}

func (f *fakeTB) Error(args ...any) { f.Errorf("%s", fmt.Sprint(args...)) }

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.failed = true
	f.fatal = fmt.Sprintf(format, args...)
	f.mu.Unlock()
	runtime.Goexit()
}

func (f *fakeTB) Fatal(args ...any) { f.Fatalf("%s", fmt.Sprint(args...)) }

func (f *fakeTB) Logf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logs = append(f.logs, fmt.Sprintf(format, args...))
}

func (f *fakeTB) Log(args ...any) { f.Logf("%s", fmt.Sprint(args...)) }

// failures returns every error and the fatal message.
func (f *fakeTB) failures() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	all := append([]string(nil), f.errors...)
	if f.fatal != "" {
		all = append(all, f.fatal)
	}
	return strings.Join(all, "\n---\n")
}

// run calls body on its own goroutine (so a Fatal stops only it) with a fake
// TB, then runs the cleanups there, and returns the TB.
func run(t *testing.T, body func(tb testing.TB)) *fakeTB {
	t.Helper()
	f := &fakeTB{name: t.Name()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			for i := len(f.cleanups) - 1; i >= 0; i-- {
				f.cleanups[i]()
			}
		}()
		body(f)
	}()
	<-done
	return f
}

// mustFail runs body and checks the fake TB failed with a message that
// contains every one of the substrings.
func mustFail(t *testing.T, body func(tb testing.TB), want ...string) {
	t.Helper()
	f := run(t, body)
	if !f.failed {
		t.Fatalf("the test did not fail; want a failure containing %q", want)
	}
	got := f.failures()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("the failure does not contain %q:\n%s", w, got)
		}
	}
}

// mustPass runs body and checks the fake TB did not fail.
func mustPass(t *testing.T, body func(tb testing.TB)) *fakeTB {
	t.Helper()
	f := run(t, body)
	if f.failed {
		t.Fatalf("the test failed:\n%s", f.failures())
	}
	return f
}
