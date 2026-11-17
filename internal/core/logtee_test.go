package core

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
)

type teed struct {
	mu   sync.Mutex
	msgs []string
}

func (c *teed) add(level slog.Level, msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, level.String()+" "+msg)
}

func (c *teed) get() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.msgs)
}

// The tee sees the dev warnings and the logged errors of a tree, which
// still go where they went; a release tree (no log) gives it the errors.
func TestLogTeeTreeMessages(t *testing.T) {
	var c teed
	SetLogTee(c.add)
	t.Cleanup(func() { SetLogTee(nil) })

	var warns, logs []string
	tr := NewTree(Config{Fonts: testFonts, Dev: true,
		Warn: func(m string) { warns = append(warns, m) }, Log: func(m string) { logs = append(logs, m) }})
	tr.warnf("careful %d", 1)
	tr.warnOnce("k", func() string { return "once" })
	tr.warnOnce("k", func() string { return "twice" })
	tr.logf("broken %s", "x")
	if got := strings.Join(warns, ","); got != "careful 1,once" || strings.Join(logs, ",") != "broken x" {
		t.Errorf("warnings = %v, log = %v", warns, logs)
	}
	if got := strings.Join(c.get(), ","); got != "WARN careful 1,WARN once,ERROR broken x" {
		t.Errorf("tee = %s", got)
	}

	release := NewTree(Config{Fonts: testFonts})
	release.warnf("a dev warning")
	release.logf("an error")
	if got := c.get(); len(got) != 4 || got[3] != "ERROR an error" {
		t.Errorf("a release tree: tee = %v, want its error only", got)
	}
}

// A Host's log keeps writing to its writer, and the tee gets each line.
func TestLogTeeHostWriter(t *testing.T) {
	var buf bytes.Buffer
	if _, ok := hostLog(&buf).(teeWriter); ok {
		t.Error("hostLog wraps the writer without a tee")
	}
	var c teed
	SetLogTee(c.add)
	t.Cleanup(func() { SetLogTee(nil) })
	w := hostLog(&buf)
	if _, err := w.Write([]byte("tela: one\ntela: two\n")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "tela: one\ntela: two\n" {
		t.Errorf("writer = %q", buf.String())
	}
	if got := strings.Join(c.get(), ","); got != "WARN tela: one,WARN tela: two" {
		t.Errorf("tee = %s", got)
	}
}

// A panic goes to the functions of OnPanic with its stack, then panics
// again with the same value.
func TestCrashRecoverPanicsAgain(t *testing.T) {
	var mu sync.Mutex
	var gotWhere string
	var gotV any
	var gotStack []byte
	active := false
	OnPanic(func(where string, v any, stack []byte) {
		mu.Lock()
		defer mu.Unlock()
		if active {
			gotWhere, gotV, gotStack = where, v, stack
		}
		panic("a failing hook is ignored")
	})
	mu.Lock()
	active = true
	mu.Unlock()
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		active = false
	}()

	boom := &struct{ s string }{"boom"}
	func() {
		defer func() {
			if r := recover(); r != boom {
				t.Errorf("recovered %v, want the same value again", r)
			}
		}()
		func() {
			defer crashRecover(PanicGo)
			panic(boom)
		}()
	}()
	mu.Lock()
	defer mu.Unlock()
	if gotWhere != PanicGo || gotV != boom || !strings.Contains(string(gotStack), "TestCrashRecoverPanicsAgain") {
		t.Errorf("hook got %q, %v, stack:\n%s", gotWhere, gotV, gotStack)
	}
	// Without a panic, nothing happens.
	func() { defer crashRecover(PanicUI) }()
}
